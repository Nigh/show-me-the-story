package story

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"showmethestory/internal/fsutil"
	"strings"
	"testing"
)

func TestProgressTransactionRollsBackEveryFile(t *testing.T) {
	for _, failNum := range []int{2, 0} {
		t.Run(fmt.Sprintf("fail_%d", failNum), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "progress.json")
			p := &Progress{Title: "old", Chapters: []ChapterState{{Num: 1, Content: "original", Status: StatusAccepted}}}
			if err := SaveProgress(path, p); err != nil {
				t.Fatal(err)
			}
			oldChapter, err := os.ReadFile(chapterFilePath(path, 1))
			if err != nil {
				t.Fatal(err)
			}
			oldMeta, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			files := []progressFile{{Num: 1, Data: []byte(`{"num":1,"content":"new"}`)}, {Num: 2, Data: []byte(`{"num":2,"content":"new chapter"}`)}, {Num: 0, Data: []byte(`{"title":"new"}`)}}
			failed := false
			err = commitProgressFiles(path, files, func(target string, data []byte) error {
				if !failed && target == progressFilePath(path, failNum) {
					failed = true
					return errors.New("injected disk failure")
				}
				return fsutil.WriteFileAtomic(target, data)
			})
			saveErr, ok := fsutil.AsSaveError(err)
			if !ok || !saveErr.OriginalPreserved {
				t.Fatalf("missing safe rollback diagnostic: %v", err)
			}
			for target, want := range map[string][]byte{path: oldMeta, chapterFilePath(path, 1): oldChapter} {
				got, err := os.ReadFile(target)
				if err != nil || string(got) != string(want) {
					t.Fatal("partial save survived rollback", target, err)
				}
			}
			if _, err := os.Stat(chapterFilePath(path, 2)); !os.IsNotExist(err) {
				t.Fatal("new chapter survived rollback", err)
			}
			if _, err := os.Stat(path + ".rollback"); !os.IsNotExist(err) {
				t.Fatal("completed rollback retained journal", err)
			}
			// A retry must rewrite the new prose rather than trust stale hashes.
			p.Chapters[0].Content = "retry"
			if err := SaveProgress(path, p); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadProgress(path)
			if err != nil || loaded.Chapters[0].Content != "retry" {
				t.Fatal("retry lost prose", err)
			}
		})
	}
}

func TestProgressRecoveryAfterInterruptedOrFailedRollback(t *testing.T) {
	for _, interrupted := range []bool{true, false} {
		t.Run(fmt.Sprint(interrupted), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "progress.json")
			p := &Progress{Title: "old", Chapters: []ChapterState{{Num: 1, Content: "original", Status: StatusAccepted}}}
			if err := SaveProgress(path, p); err != nil {
				t.Fatal(err)
			}
			oldChapter, err := os.ReadFile(chapterFilePath(path, 1))
			if err != nil {
				t.Fatal(err)
			}
			oldMeta, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if interrupted {
				journal, err := json.Marshal(progressJournal{Version: 1, Files: []progressFile{{Num: 1, Data: oldChapter}, {Num: 0, Data: oldMeta}}})
				if err != nil {
					t.Fatal(err)
				}
				if err := fsutil.WriteFileAtomic(path+".rollback", journal); err != nil {
					t.Fatal(err)
				}
				if err := fsutil.WriteFileAtomic(chapterFilePath(path, 1), []byte(`{"num":1,"content":"uncommitted"}`)); err != nil {
					t.Fatal(err)
				}
				if err := fsutil.WriteFileAtomic(path, []byte(`{"title":"uncommitted"}`)); err != nil {
					t.Fatal(err)
				}
			} else {
				err = commitProgressFiles(path, []progressFile{{Num: 1, Data: []byte(`{"num":1,"content":"uncommitted"}`)}, {Num: 0, Data: []byte(`{}`)}}, func(target string, data []byte) error {
					if target == path || string(data) == string(oldChapter) {
						return errors.New("disk unavailable")
					}
					return fsutil.WriteFileAtomic(target, data)
				})
				saveErr, ok := fsutil.AsSaveError(err)
				if !ok || saveErr.OriginalPreserved || saveErr.RestoreErr == nil || saveErr.BackupPath != path+".rollback" {
					t.Fatalf("missing retained journal diagnostic: %v", err)
				}
			}
			resetCache(path)
			loaded, err := LoadProgress(path)
			if err != nil || loaded.Title != "old" || loaded.Chapters[0].Content != "original" {
				t.Fatal("recovery did not restore a complete save", err)
			}
			if _, err := os.Stat(path + ".rollback"); !os.IsNotExist(err) {
				t.Fatal("recovered journal retained", err)
			}
		})
	}
}

func TestInvalidRecoveryJournalBlocksLoadAndSave(t *testing.T) {
	for _, journal := range []string{`{`, `{"version":2,"files":[{"num":0,"data":null}]}`, `{"version":1,"files":[{"num":-1,"data":null},{"num":0,"data":null}]}`, `{"version":1,"files":[{"num":0,"data":null},{"num":0,"data":null}]}`, `{"version":1,"files":[{"num":1,"data":null}]}`, `{"version":1,"files":[{"num":0}]}`, `{"version":1,"files":[{"data":null}]}`} {
		path := filepath.Join(t.TempDir(), "progress.json")
		if err := os.WriteFile(path, []byte(`{"title":"original"}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+".rollback", []byte(journal), 0644); err != nil {
			t.Fatal(err)
		}
		if got, err := LoadProgress(path); err == nil || got != nil {
			t.Fatal("invalid recovery allowed load")
		}
		if err := SaveProgress(path, &Progress{}); err == nil {
			t.Fatal("invalid recovery allowed save")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != `{"title":"original"}` {
			t.Fatal("invalid journal changed original", err)
		}
	}
}

func TestLoadProgressRejectsUnreadableChapters(t *testing.T) {
	for _, damage := range []string{"missing", "invalid JSON", "directory", "wrong number", "missing content", "empty content"} {
		t.Run(damage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "progress.json")
			p := &Progress{Chapters: []ChapterState{{Num: 1, Content: "original prose", Status: StatusAccepted}}}
			if err := SaveProgress(path, p); err != nil {
				t.Fatal(err)
			}
			chapterPath := chapterFilePath(path, 1)
			if err := os.Remove(chapterPath); err != nil {
				t.Fatal(err)
			}
			var damaged []byte
			switch damage {
			case "directory":
				if err := os.Mkdir(chapterPath, 0755); err != nil {
					t.Fatal(err)
				}
			case "invalid JSON":
				damaged = []byte(`{"num":1,"content":"recoverable prose`)
			case "wrong number":
				damaged = []byte(`{"num":2,"content":"other chapter"}`)
			case "missing content":
				damaged = []byte(`{"num":1}`)
			case "empty content":
				damaged = []byte(`{"num":1,"content":""}`)
			}
			if damaged != nil {
				if err := os.WriteFile(chapterPath, damaged, 0644); err != nil {
					t.Fatal(err)
				}
			}
			resetCache(path) // A fresh process must not turn read errors into empty prose.
			got, err := LoadProgress(path)
			if err == nil || got != nil || !strings.Contains(err.Error(), chapterPath) {
				t.Fatalf("loaded damaged chapter: progress=%v error=%v", got, err)
			}
			if damaged != nil {
				data, err := os.ReadFile(chapterPath)
				if err != nil || string(data) != string(damaged) {
					t.Fatal("damaged original changed", err)
				}
			}
		})
	}
}

func TestLoadProgressAllowsOnlyUnwrittenMissingChapter(t *testing.T) {
	for _, status := range []string{StatusPending, StatusWriting, StatusReview, StatusAccepted} {
		t.Run(status, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "progress.json")
			data, err := json.Marshal(&Progress{Chapters: []ChapterState{{Num: 1, Status: status}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}
			got, err := LoadProgress(path)
			if status == StatusPending {
				if err != nil || got.Chapters[0].Content != "" {
					t.Fatal("unwritten chapter rejected", err)
				}
			} else if err == nil || !errors.Is(err, os.ErrNotExist) {
				t.Fatal("missing written chapter accepted", err)
			}
		})
	}
}

func TestStorageRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")

	p := &Progress{
		Phase: "writing",
		Title: "测试书",
		Chapters: []ChapterState{
			{Num: 1, Title: "第一章", Outline: "开局", Content: "第一段。\n\n第二段。", Summary: "摘要1", Status: StatusAccepted},
			{Num: 2, Title: "第二章", Outline: "冲突", Content: "", Status: StatusPending},
		},
		CurrentChapterIndex: 1,
		MemoryEntries: []MemoryEntry{
			{ID: 1, Content: "主角捡到怀表", Category: "item", References: []MemoryReference{{Chapter: 1, BlockID: 2, Quote: "第二段。"}}},
		},
	}

	if err := SaveProgress(path, p); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	// Metadata file must not contain prose content.
	meta, _ := os.ReadFile(path)
	if strings.Contains(string(meta), "第一段。") {
		t.Fatal("progress.json 不应包含正文")
	}

	// Chapter file exists with content.
	if _, err := os.Stat(chapterFilePath(path, 1)); err != nil {
		t.Fatalf("章节文件缺失: %v", err)
	}

	// Roundtrip restores content.
	resetCache(path)
	loaded, err := LoadProgress(path)
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if loaded.Chapters[0].Content != "第一段。\n\n第二段。" {
		t.Fatalf("正文丢失: %q", loaded.Chapters[0].Content)
	}
	if loaded.Chapters[0].WordCount == 0 {
		t.Fatal("WordCount 未计算")
	}
	if loaded.Chapters[1].Content != "" {
		t.Fatal("空章节不应有正文")
	}
}

func TestSaveProgressDirtyCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	p := &Progress{Phase: "writing", Chapters: []ChapterState{
		{Num: 1, Title: "A", Content: "原文", Status: StatusAccepted},
	}}
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	ch1 := chapterFilePath(path, 1)
	st1, _ := os.Stat(ch1)

	// Save again without changes: chapter file must not be rewritten.
	// Corrupt-proof check: delete the file; unchanged content should be skipped
	// only when hash cache says so — so instead compare mtime via rewrite marker.
	os.Chtimes(ch1, st1.ModTime(), st1.ModTime())
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	st2, _ := os.Stat(ch1)
	if !st2.ModTime().Equal(st1.ModTime()) {
		t.Fatal("未变更的章节文件被重写")
	}

	// Change content: file must be rewritten.
	p.Chapters[0].Content = "改过的正文"
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(ch1)
	var cf chapterFile
	json.Unmarshal(data, &cf)
	if cf.Content != "改过的正文" {
		t.Fatalf("章节文件未更新: %q", cf.Content)
	}
}

func TestSaveProgressOrphanCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	p := &Progress{Phase: "outline", Chapters: []ChapterState{
		{Num: 1, Content: "a", Status: StatusPending},
		{Num: 2, Content: "b", Status: StatusPending},
	}}
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	p.Chapters = p.Chapters[:1]
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(chapterFilePath(path, 2)); !os.IsNotExist(err) {
		t.Fatal("孤儿章节文件未清理")
	}
}

func TestProgressViewStripsContent(t *testing.T) {
	p := &Progress{Chapters: []ChapterState{
		{Num: 1, Content: "正文内容", Status: StatusAccepted},
	}}
	v := ProgressView(p)
	if v.Chapters[0].Content != "" {
		t.Fatal("view 不应包含正文")
	}
	if v.Chapters[0].ContentRev == "" || v.Chapters[0].WordCount == 0 {
		t.Fatal("view 应包含 content_rev 与 word_count")
	}
	if p.Chapters[0].Content == "" {
		t.Fatal("原始 state 被破坏")
	}
}
