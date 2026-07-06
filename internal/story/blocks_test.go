package story

import "testing"

func chWith(content string) *ChapterState {
	ch := &ChapterState{Num: 1, Title: "t", Content: content}
	SyncChapterBlocks(ch)
	return ch
}

func TestSyncChapterBlocksStableIDs(t *testing.T) {
	ch := chWith("段落一。\n\n“对话。”\n\n---\n\n段落二。")
	if len(ch.Blocks) != 4 {
		t.Fatalf("期望 4 个 block，得到 %d", len(ch.Blocks))
	}
	if ch.Blocks[1].Type != BlockDialogue || ch.Blocks[2].Type != BlockSceneBreak {
		t.Fatalf("类型标注错误: %+v", ch.Blocks)
	}
	ids := []int{ch.Blocks[0].ID, ch.Blocks[1].ID, ch.Blocks[2].ID, ch.Blocks[3].ID}

	// Rewrite one paragraph: others keep IDs, changed one gets a new ID.
	ch.Content = "段落一。\n\n“改过的对话。”\n\n---\n\n段落二。"
	SyncChapterBlocks(ch)
	if ch.Blocks[0].ID != ids[0] || ch.Blocks[2].ID != ids[2] || ch.Blocks[3].ID != ids[3] {
		t.Fatal("未变更段落的 ID 应保持稳定")
	}
	if ch.Blocks[1].ID == ids[1] {
		t.Fatal("变更段落应获得新 ID")
	}
}

func TestSyncSingleNewlineFallback(t *testing.T) {
	ch := chWith("第一行。\n第二行。\n第三行。")
	if len(ch.Blocks) != 3 || ch.BlockSep != "\n" {
		t.Fatalf("单换行回退失败: %d blocks, sep=%q", len(ch.Blocks), ch.BlockSep)
	}
	rebuildContentFromBlocks(ch)
	if ch.Content != "第一行。\n第二行。\n第三行。" {
		t.Fatalf("重建正文改变了换行格式: %q", ch.Content)
	}
}

func TestBlockCRUD(t *testing.T) {
	ch := chWith("甲。\n\n乙。\n\n丙。")
	id2 := ch.Blocks[1].ID

	if err := UpdateBlock(ch, id2, "乙（改）。"); err != nil {
		t.Fatal(err)
	}
	if ch.Content != "甲。\n\n乙（改）。\n\n丙。" {
		t.Fatalf("update 后正文错误: %q", ch.Content)
	}

	newID, err := InsertBlockAfter(ch, id2, "乙点五。")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Content != "甲。\n\n乙（改）。\n\n乙点五。\n\n丙。" {
		t.Fatalf("insert 后正文错误: %q", ch.Content)
	}

	if err := DeleteBlock(ch, newID); err != nil {
		t.Fatal(err)
	}
	if ch.Content != "甲。\n\n乙（改）。\n\n丙。" {
		t.Fatalf("delete 后正文错误: %q", ch.Content)
	}

	if err := UpdateBlock(ch, 9999, "x"); err != ErrBlockNotFound {
		t.Fatal("不存在的 block 应报错")
	}
}

func TestUpdateBlockMultiParagraphSplits(t *testing.T) {
	ch := chWith("甲。\n\n乙。")
	id1 := ch.Blocks[0].ID
	if err := UpdateBlock(ch, id1, "甲一。\n\n甲二。"); err != nil {
		t.Fatal(err)
	}
	if len(ch.Blocks) != 3 {
		t.Fatalf("含空行的编辑应拆成多个 block，得到 %d", len(ch.Blocks))
	}
	if ch.Content != "甲一。\n\n甲二。\n\n乙。" {
		t.Fatalf("拆分后正文错误: %q", ch.Content)
	}
}

func TestBlocksPersistRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/progress.json"
	p := &Progress{Phase: "writing", Chapters: []ChapterState{
		{Num: 1, Title: "A", Content: "一。\n\n二。", Status: StatusAccepted},
	}}
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	origIDs := []int{p.Chapters[0].Blocks[0].ID, p.Chapters[0].Blocks[1].ID}

	resetCache(path)
	loaded, err := LoadProgress(path)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.Chapters[0].Blocks
	if len(got) != 2 || got[0].ID != origIDs[0] || got[1].ID != origIDs[1] {
		t.Fatalf("blocks 未随章节文件持久化: %+v", got)
	}
}
