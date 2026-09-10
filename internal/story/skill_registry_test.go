package story

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"showmethestory/internal/config"
	"testing"
)

func testSkillMarkdown(body string) []byte {
	return []byte("---\nid: test-skill\nname: Test Skill\ndescription: test\ncategory: writing\nlang: en\napplies_to: [chapter.generate]\n---\n\n" + body)
}

func testSkillFiles(body string) map[string][]byte {
	manifest := []byte(`{"schema_version":1,"id":"test-skill","name":"Test Skill","description":"test","category":"writing","languages":["en"],"applies_to":["chapter.generate"],"entrypoint":"SKILL.md"}`)
	return map[string][]byte{"skill.json": manifest, "SKILL.md": testSkillMarkdown(body)}
}

func TestInstallSkillAndInvalidateValidationOnContentChange(t *testing.T) {
	dir := t.TempDir()
	first, err := InstallSkillFiles(dir, testSkillFiles("first"), false)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "test-skill" || first.ContentHash == "" {
		t.Fatalf("unexpected installed skill: %+v", first)
	}
	if err := SetSkillValidation(dir, first.ID, &SkillValidationReport{Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	loaded := LoadGlobalSkills(dir)
	if len(loaded) != 1 || loaded[0].Validation == nil || loaded[0].Validation.Status != "passed" {
		t.Fatalf("validation not loaded: %+v", loaded)
	}
	second, err := InstallSkillFiles(dir, testSkillFiles("changed"), true)
	if err != nil {
		t.Fatal(err)
	}
	if second.ContentHash == first.ContentHash || second.Validation != nil {
		t.Fatalf("changed content must invalidate validation: %+v", second)
	}
}

func TestSkillPackageRequiresVersionedManifest(t *testing.T) {
	if _, err := InstallSkillFiles(t.TempDir(), map[string][]byte{"SKILL.md": testSkillMarkdown("body")}, false); err == nil {
		t.Fatal("expected missing skill.json rejection")
	}
	files := testSkillFiles("body")
	files["skill.json"] = []byte(`{"id":"test-skill","name":"Test Skill","entrypoint":"SKILL.md","applies_to":["chapter.generate"]}`)
	if _, err := InstallSkillFiles(t.TempDir(), files, false); err == nil {
		t.Fatal("expected missing schema_version rejection")
	}
}

func TestSkillZipRejectsTraversalAndSymlink(t *testing.T) {
	makeZip := func(name string, mode os.FileMode) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		h.SetMode(mode)
		w, _ := zw.CreateHeader(h)
		_, _ = w.Write([]byte("x"))
		_ = zw.Close()
		return buf.Bytes()
	}
	if _, err := FilesFromSkillZip(makeZip("../escape.md", 0644)); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if _, err := FilesFromSkillZip(makeZip("link.md", os.ModeSymlink|0777)); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestResolveSkillsUsesEnabledLanguageAndScope(t *testing.T) {
	skills := []Skill{{ID: "a", Lang: "en", AppliesTo: []string{SkillScopeChapterGenerate}}, {ID: "b", Lang: "en", AppliesTo: []string{SkillScopeChapterPolish}}, {ID: "c", Lang: "zh", AppliesTo: []string{SkillScopeChapterGenerate}}}
	sc := &config.SkillConfig{EnabledSkills: map[string]bool{"a": true, "b": true, "c": true}}
	got := ResolveSkills(skills, sc, SkillScopeChapterGenerate, "en")
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("unexpected skills: %+v", got)
	}
}

func TestInstallStandardPackageAndResources(t *testing.T) {
	dir := t.TempDir()
	manifest := []byte(`{"schema_version":1,"id":"standard","name":"Standard","description":"d","category":"writing","languages":["en"],"applies_to":["chapter.generate"],"entrypoint":"SKILL.md"}`)
	s, err := InstallSkillFiles(dir, map[string][]byte{"skill.json": manifest, "SKILL.md": []byte("instructions"), "references/example.md": []byte("example")}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Resources) != 1 || s.Resources[0] != "references/example.md" {
		t.Fatalf("unexpected resources: %+v", s.Resources)
	}
	if _, err := os.Stat(filepath.Join(SkillLibraryDir(dir), "standard", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
