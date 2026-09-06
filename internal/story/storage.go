package story

// v3 project storage: progress metadata lives in project.json (chapter list
// WITHOUT prose content), each chapter's prose lives in chapters/NNNNNN.json.
// LoadProgress/SaveProgress keep their v2 signatures (path = project.json) so
// every caller stays untouched; the split happens inside.

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"showmethestory/internal/fsutil"
	"showmethestory/internal/prose"
	"strings"
	"sync"
)

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

// saveChapterFiles writes chapter files whose content changed since the last
// load/save, removes orphaned chapter files, and refreshes WordCount.
func saveChapterFiles(progressPath string, p *Progress) error {
	dir := chaptersDir(progressPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建章节目录失败: %w", err)
	}

	cache := cacheFor(progressPath)

	for i := range p.Chapters {
		ch := &p.Chapters[i]
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
			return fmt.Errorf("序列化第 %d 章失败: %w", ch.Num, err)
		}
		if err := fsutil.WriteFileAtomic(chapterFilePath(progressPath, ch.Num), data); err != nil {
			return fmt.Errorf("保存第 %d 章失败: %w", ch.Num, err)
		}
		contentHashCache.Lock()
		cache[ch.Num] = h
		contentHashCache.Unlock()
	}

	return nil
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
func loadChapterContents(progressPath string, p *Progress) {
	cache := cacheFor(progressPath)
	for i := range p.Chapters {
		ch := &p.Chapters[i]
		data, err := os.ReadFile(chapterFilePath(progressPath, ch.Num))
		if err != nil {
			ch.Content = ""
			continue
		}
		var cf chapterFile
		if err := json.Unmarshal(data, &cf); err != nil {
			ch.Content = ""
			continue
		}
		ch.Content = cf.Content
		ch.Blocks = cf.Blocks
		ch.NextBlockID = cf.NextBlockID
		ch.BlockSep = cf.BlockSep
		if len(ch.Blocks) == 0 && ch.Content != "" {
			SyncChapterBlocks(ch)
		}
		if ch.WordCount == 0 && ch.Content != "" {
			ch.WordCount = prose.CountProseUnits(ch.Content)
		}
		contentHashCache.Lock()
		cache[ch.Num] = HashContent(ch.Content)
		contentHashCache.Unlock()
	}
}

// ResetProgressFiles removes project.json and the chapters directory.
func ResetProgressFiles(progressPath string) error {
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
			m.Snippet = extractSnippet(p, m.Chapter, m.Position, 100)
			entries[i] = m
		}
		cp.MemoryEntries = entries
	}
	return &cp
}
