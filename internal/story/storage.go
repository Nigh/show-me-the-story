package story

// Project storage: progress metadata lives in progress.json (without prose),
// and chapter prose lives in chapters/NNNNNN.json.

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"showmethestory/internal/fsutil"
	"showmethestory/internal/prose"
	"strings"
	"sync"
)

// ponytail: serialize project file transactions in this process; use per-project
// locks if the application ever supports concurrent project writers.
var progressStorageMu sync.Mutex

// ChapterLoadError preserves the chapter and path for localized HTTP diagnostics.
type ChapterLoadError struct {
	Num  int
	Path string
	Err  error
}

func (e *ChapterLoadError) Error() string {
	return fmt.Sprintf("chapter %d (%s): %v", e.Num, e.Path, e.Err)
}
func (e *ChapterLoadError) Unwrap() error { return e.Err }

// Num=0 identifies progress metadata. Positive numbers identify chapter files;
// the journal cannot supply arbitrary filesystem paths. Nil Data means absent.
type progressFile struct {
	Num  int    `json:"num"`
	Data []byte `json:"data"`
}

func (f *progressFile) UnmarshalJSON(data []byte) error {
	var raw struct {
		Num  *int            `json:"num"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Num == nil || len(raw.Data) == 0 {
		return errors.New("incomplete rollback file")
	}
	f.Num = *raw.Num
	return json.Unmarshal(raw.Data, &f.Data)
}

type progressJournal struct {
	Version int            `json:"version"`
	Files   []progressFile `json:"files"`
}

func progressFilePath(path string, num int) string {
	if num == 0 {
		return path
	}
	return chapterFilePath(path, num)
}

// Recover an interrupted transaction before exposing or saving project state.
// Keep the journal until every restore succeeds, so recovery is retryable.
func recoverProgress(path string, write func(string, []byte) error) error {
	journalPath := path + ".rollback"
	data, err := os.ReadFile(journalPath)
	if os.IsNotExist(err) {
		return nil
	}
	failure := func(err error) error {
		return &fsutil.SaveError{Path: path, Stage: "recover_progress", BackupPath: journalPath, Err: err}
	}
	if err != nil {
		return failure(err)
	}
	var journal progressJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return failure(err)
	}
	if journal.Version != 1 || len(journal.Files) == 0 {
		return failure(errors.New("invalid progress rollback journal"))
	}
	seen := make(map[int]bool, len(journal.Files))
	for _, file := range journal.Files {
		if file.Num < 0 || seen[file.Num] {
			return failure(errors.New("invalid rollback file number"))
		}
		seen[file.Num] = true
	}
	if !seen[0] {
		return failure(errors.New("rollback journal has no progress metadata"))
	}
	resetCache(path)
	for _, file := range journal.Files {
		target := progressFilePath(path, file.Num)
		if file.Data == nil {
			err = os.Remove(target)
			if os.IsNotExist(err) {
				err = nil
			}
		} else {
			err = write(target, file.Data)
		}
		if err != nil {
			return failure(err)
		}
	}
	if err := os.Remove(journalPath); err != nil {
		return failure(err)
	}
	return nil
}

// Journal old bytes before the first replacement. Removing the journal commits
// the transaction; a crash before that point restores the entire previous save.
func commitProgressFiles(path string, files []progressFile, write func(string, []byte) error) error {
	journal := progressJournal{Version: 1}
	for _, file := range files {
		target := progressFilePath(path, file.Num)
		data, err := os.ReadFile(target)
		if err != nil && !os.IsNotExist(err) {
			return &fsutil.SaveError{Path: target, Stage: "read_original", OriginalPreserved: true, Err: err}
		}
		journal.Files = append(journal.Files, progressFile{Num: file.Num, Data: data})
	}
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	journalPath := path + ".rollback"
	if err := write(journalPath, data); err != nil {
		return err
	}
	for _, file := range files {
		if err = write(progressFilePath(path, file.Num), file.Data); err != nil {
			break
		}
	}
	if err == nil {
		err = os.Remove(journalPath)
	}
	if err != nil {
		restoreErr := recoverProgress(path, write)
		backupPath := ""
		if restoreErr != nil {
			backupPath = journalPath
		}
		return &fsutil.SaveError{Path: path, Stage: "commit_progress", OriginalPreserved: restoreErr == nil,
			BackupPath: backupPath, Err: err, RestoreErr: restoreErr}
	}
	return nil
}

// chapterFile is the on-disk shape of chapters/NNNNNN.json.
type chapterFile struct {
	Num         int     `json:"num"`
	Title       string  `json:"title"`
	Content     string  `json:"content"`
	Blocks      []Block `json:"blocks,omitempty"`
	NextBlockID int     `json:"next_block_id,omitempty"`
	BlockSep    string  `json:"block_sep,omitempty"`
}

func chaptersDir(progressPath string) string {
	return filepath.Join(filepath.Dir(progressPath), "chapters")
}

func chapterFilePath(progressPath string, num int) string {
	return filepath.Join(chaptersDir(progressPath), fmt.Sprintf("%06d.json", num))
}

// contentHashCache tracks the last-saved content hash per project per chapter,
// so SaveProgress only rewrites chapter files whose prose actually changed.
// ponytail: process-global map, entries never evicted; bounded by number of
// projects opened in one process lifetime, which is tiny.
var contentHashCache = struct {
	sync.Mutex
	m map[string]map[int]uint64
}{m: map[string]map[int]uint64{}}

func HashContent(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func cacheFor(progressPath string) map[int]uint64 {
	contentHashCache.Lock()
	defer contentHashCache.Unlock()
	c, ok := contentHashCache.m[progressPath]
	if !ok {
		c = map[int]uint64{}
		contentHashCache.m[progressPath] = c
	}
	return c
}

func resetCache(progressPath string) {
	contentHashCache.Lock()
	delete(contentHashCache.m, progressPath)
	contentHashCache.Unlock()
}

// Prepare changed chapter files without overwriting any originals.
func prepareChapterFiles(progressPath string, p *Progress) ([]progressFile, error) {
	cache := cacheFor(progressPath)
	var files []progressFile
	seenNumbers := make(map[int]bool, len(p.Chapters))

	for i := range p.Chapters {
		ch := &p.Chapters[i]
		if ch.Num <= 0 || seenNumbers[ch.Num] {
			return nil, fmt.Errorf("invalid or duplicate chapter number: %d", ch.Num)
		}
		seenNumbers[ch.Num] = true
		h := HashContent(ch.Content)
		contentHashCache.Lock()
		prev, seen := cache[ch.Num]
		contentHashCache.Unlock()
		if seen && prev == h {
			continue
		}
		ch.WordCount = prose.CountProseUnits(ch.Content)
		SyncChapterBlocks(ch)
		data, err := json.MarshalIndent(chapterFile{
			Num: ch.Num, Title: ch.Title, Content: ch.Content,
			Blocks: ch.Blocks, NextBlockID: ch.NextBlockID, BlockSep: ch.BlockSep,
		}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("序列化第 %d 章失败: %w", ch.Num, err)
		}
		files = append(files, progressFile{Num: ch.Num, Data: data})
	}

	return files, nil
}

// Prune only after metadata commits, so failed saves retain old chapter files.
func cleanupChapterFiles(progressPath string, p *Progress) {
	dir := chaptersDir(progressPath)
	cache := cacheFor(progressPath)
	live := make(map[int]bool, len(p.Chapters))
	for _, ch := range p.Chapters {
		live[ch.Num] = true
	}

	// Remove orphaned chapter files (e.g. outline regenerated with fewer chapters).
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var num int
		if _, err := fmt.Sscanf(name, "%d.json", &num); err != nil {
			continue
		}
		if !live[num] {
			fsutil.Delete(filepath.Join(dir, name))
			contentHashCache.Lock()
			delete(cache, num)
			contentHashCache.Unlock()
		}
	}
}

// loadChapterContents fills Content for each chapter from its chapter file
// and primes the dirty-check cache.
func loadChapterContents(progressPath string, p *Progress) error {
	cache := make(map[int]uint64, len(p.Chapters))
	seen := make(map[int]bool, len(p.Chapters))
	for i := range p.Chapters {
		ch := &p.Chapters[i]
		path := chapterFilePath(progressPath, ch.Num)
		failure := func(err error) error { return &ChapterLoadError{Num: ch.Num, Path: path, Err: err} }
		if ch.Num <= 0 || seen[ch.Num] {
			return failure(errors.New("invalid or duplicate chapter number"))
		}
		seen[ch.Num] = true
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) && ch.Status == StatusPending && ch.WordCount == 0 && ch.Summary == "" && ch.Content == "" && ch.ContentRev == "" {
				continue // Older v4 projects may omit files for wholly unwritten chapters.
			}
			return failure(err)
		}
		var cf struct {
			chapterFile
			Content *string `json:"content"`
		}
		if err := json.Unmarshal(data, &cf); err != nil {
			return failure(err)
		}
		if cf.Num != ch.Num || cf.Content == nil {
			return failure(errors.New("chapter number or content field is invalid"))
		}
		if ch.WordCount > 0 && *cf.Content == "" {
			return failure(errors.New("chapter is empty but metadata records existing prose"))
		}
		ch.Content = *cf.Content
		ch.Blocks = cf.Blocks
		ch.NextBlockID = cf.NextBlockID
		ch.BlockSep = cf.BlockSep
		if len(ch.Blocks) == 0 && ch.Content != "" {
			SyncChapterBlocks(ch)
		}
		if ch.WordCount == 0 && ch.Content != "" {
			ch.WordCount = prose.CountProseUnits(ch.Content)
		}
		cache[ch.Num] = HashContent(ch.Content)
	}
	contentHashCache.Lock()
	contentHashCache.m[progressPath] = cache
	contentHashCache.Unlock()
	return nil
}

// ResetProgressFiles removes project.json and the chapters directory.
func ResetProgressFiles(progressPath string) error {
	progressStorageMu.Lock()
	defer progressStorageMu.Unlock()
	if err := recoverProgress(progressPath, fsutil.WriteFileAtomic); err != nil {
		return err
	}
	if err := fsutil.Delete(progressPath); err != nil {
		return err
	}
	resetCache(progressPath)
	return os.RemoveAll(chaptersDir(progressPath))
}

// ProgressView returns a copy of p with prose content removed from chapters
// (word count + content revision kept) and memory snippets resolved, for API
// responses.
func ProgressView(p *Progress) *Progress {
	cp := *p
	cp.Chapters = make([]ChapterState, len(p.Chapters))
	for i, ch := range p.Chapters {
		if ch.Content != "" {
			if ch.WordCount == 0 {
				ch.WordCount = prose.CountProseUnits(ch.Content)
			}
			ch.ContentRev = fmt.Sprintf("%x", HashContent(ch.Content))
		} else {
			ch.WordCount = 0
		}
		ch.Content = ""
		ch.Blocks = nil
		ch.NextBlockID = 0
		ch.BlockSep = ""
		cp.Chapters[i] = ch
	}
	if len(p.MemoryEntries) > 0 {
		entries := make([]MemoryEntry, len(p.MemoryEntries))
		for i, m := range p.MemoryEntries {
			for _, ref := range m.References {
				if ReferenceLive(p, ref) {
					r := []rune(ref.Quote)
					if len(r) > 100 {
						r = r[:100]
					}
					m.Snippet = string(r)
					break
				}
			}
			m.References = nil // Evidence is loaded on demand via the facts endpoint.
			entries[i] = m
		}
		cp.MemoryEntries = entries
	}
	return &cp
}
