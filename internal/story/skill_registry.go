package story

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"showmethestory/internal/fsutil"
	"sort"
	"strings"
)

const (
	maxSkillFiles     = 100
	maxSkillFileSize  = 2 << 20
	maxSkillTotalSize = 10 << 20
)

var skillIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type SkillManifest struct {
	SchemaVersion int      `json:"schema_version"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Category      string   `json:"category"`
	Languages     []string `json:"languages"`
	AppliesTo     []string `json:"applies_to"`
	EntryPoint    string   `json:"entrypoint"`
}

type SkillRegistryRecord struct {
	ContentHash string                 `json:"content_hash"`
	Validation  *SkillValidationReport `json:"validation,omitempty"`
}

type SkillRegistryState struct {
	Skills map[string]SkillRegistryRecord `json:"skills"`
}

func SkillLibraryDir(progDir string) string { return filepath.Join(progDir, "skills") }

func safeSkillRelativePath(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	name = strings.TrimPrefix(name, "./")
	clean := filepath.ToSlash(filepath.Clean(name))
	if name == "" || clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || filepath.IsAbs(name) || strings.Contains(clean, ":") {
		return "", fmt.Errorf("unsafe skill path %q", name)
	}
	parts := strings.Split(clean, "/")
	if len(parts) > 8 {
		return "", fmt.Errorf("skill path is too deep: %s", clean)
	}
	allowed := map[string]bool{".md": true, ".txt": true, ".json": true}
	if !allowed[strings.ToLower(filepath.Ext(clean))] {
		return "", fmt.Errorf("unsupported skill file: %s", clean)
	}
	return clean, nil
}

func FilesFromSkillZip(data []byte) (map[string][]byte, error) {
	if len(data) > maxSkillTotalSize {
		return nil, fmt.Errorf("skill archive exceeds %d bytes", maxSkillTotalSize)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	total := 0
	for _, f := range zr.File {
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symbolic links are not allowed")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if len(files) >= maxSkillFiles {
			return nil, fmt.Errorf("skill contains too many files")
		}
		name, err := safeSkillRelativePath(f.Name)
		if err != nil {
			return nil, err
		}
		if f.UncompressedSize64 > maxSkillFileSize {
			return nil, fmt.Errorf("skill file is too large: %s", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, readErr := io.ReadAll(io.LimitReader(rc, maxSkillFileSize+1))
		closeErr := rc.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(b) > maxSkillFileSize {
			return nil, fmt.Errorf("skill file is too large: %s", name)
		}
		if _, exists := files[name]; exists {
			return nil, fmt.Errorf("duplicate skill file: %s", name)
		}
		total += len(b)
		if total > maxSkillTotalSize {
			return nil, fmt.Errorf("expanded skill exceeds limit")
		}
		files[name] = b
	}
	return normalizeSkillRoot(files), nil
}

func normalizeSkillRoot(files map[string][]byte) map[string][]byte {
	var root string
	for name := range files {
		p := strings.Split(filepath.ToSlash(name), "/")
		if len(p) < 2 {
			return files
		}
		if root == "" {
			root = p[0]
		} else if root != p[0] {
			return files
		}
	}
	out := map[string][]byte{}
	for name, b := range files {
		out[strings.TrimPrefix(filepath.ToSlash(name), root+"/")] = b
	}
	return out
}

func normalizeSkillFiles(files map[string][]byte) (SkillManifest, map[string][]byte, error) {
	if len(files) == 0 || len(files) > maxSkillFiles {
		return SkillManifest{}, nil, fmt.Errorf("empty or oversized skill package")
	}
	cleaned := map[string][]byte{}
	total := 0
	for name, b := range normalizeSkillRoot(files) {
		clean, err := safeSkillRelativePath(name)
		if err != nil {
			return SkillManifest{}, nil, err
		}
		if len(b) > maxSkillFileSize {
			return SkillManifest{}, nil, fmt.Errorf("skill file is too large: %s", clean)
		}
		total += len(b)
		if total > maxSkillTotalSize {
			return SkillManifest{}, nil, fmt.Errorf("skill exceeds total size limit")
		}
		cleaned[clean] = b
	}
	raw, ok := cleaned["skill.json"]
	if !ok {
		return SkillManifest{}, nil, fmt.Errorf("skill.json is required")
	}
	var manifest SkillManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, nil, fmt.Errorf("invalid skill.json: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return manifest, nil, fmt.Errorf("unsupported skill schema version")
	}
	if !skillIDPattern.MatchString(manifest.ID) {
		return manifest, nil, fmt.Errorf("invalid skill id %q", manifest.ID)
	}
	if manifest.Name == "" || manifest.EntryPoint == "" {
		return manifest, nil, fmt.Errorf("skill name and entrypoint are required")
	}
	entry, err := safeSkillRelativePath(manifest.EntryPoint)
	if err != nil {
		return manifest, nil, err
	}
	manifest.EntryPoint = entry
	if _, ok := cleaned[entry]; !ok {
		return manifest, nil, fmt.Errorf("entrypoint %s not found", entry)
	}
	if strings.TrimSpace(string(cleaned[entry])) == "" {
		return manifest, nil, fmt.Errorf("skill entrypoint is empty")
	}
	allowed := map[string]bool{}
	for _, s := range AllowedSkillScopes {
		allowed[s] = true
	}
	if len(manifest.AppliesTo) == 0 {
		return manifest, nil, fmt.Errorf("applies_to is required")
	}
	for _, s := range manifest.AppliesTo {
		if !allowed[s] {
			return manifest, nil, fmt.Errorf("unsupported applies_to scope %q", s)
		}
	}
	for i, l := range manifest.Languages {
		manifest.Languages[i] = strings.ToLower(strings.TrimSpace(l))
		if manifest.Languages[i] != "zh" && manifest.Languages[i] != "en" {
			return manifest, nil, fmt.Errorf("unsupported language %q", l)
		}
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	cleaned["skill.json"] = mb
	return manifest, cleaned, nil
}

func ValidateSkillFiles(input map[string][]byte) (SkillManifest, []string, error) {
	manifest, files, err := normalizeSkillFiles(input)
	if err != nil {
		return manifest, nil, err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return manifest, names, nil
}

func skillFilesHash(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		h.Write([]byte(n))
		h.Write([]byte{0})
		h.Write(files[n])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func registryPath(progDir string) string {
	return filepath.Join(SkillLibraryDir(progDir), ".registry.json")
}
func loadRegistry(progDir string) SkillRegistryState {
	st := SkillRegistryState{Skills: map[string]SkillRegistryRecord{}}
	b, err := os.ReadFile(registryPath(progDir))
	if err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Skills == nil {
		st.Skills = map[string]SkillRegistryRecord{}
	}
	return st
}
func saveRegistry(progDir string, st SkillRegistryState) error {
	if err := os.MkdirAll(SkillLibraryDir(progDir), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(registryPath(progDir), b)
}

func InstallSkillFiles(progDir string, input map[string][]byte, overwrite bool) (Skill, error) {
	manifest, files, err := normalizeSkillFiles(input)
	if err != nil {
		return Skill{}, err
	}
	root := SkillLibraryDir(progDir)
	target := filepath.Join(root, manifest.ID)
	if _, err := os.Stat(target); err == nil && !overwrite {
		return Skill{}, fmt.Errorf("skill %s already exists", manifest.ID)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return Skill{}, err
	}
	stage, err := os.MkdirTemp(root, ".install-")
	if err != nil {
		return Skill{}, err
	}
	defer os.RemoveAll(stage)
	for name, b := range files {
		path := filepath.Join(stage, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return Skill{}, err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return Skill{}, err
		}
	}
	backup := ""
	if _, err := os.Stat(target); err == nil {
		backup = target + ".backup"
		_ = os.RemoveAll(backup)
		if err := os.Rename(target, backup); err != nil {
			return Skill{}, err
		}
	}
	if err := os.Rename(stage, target); err != nil {
		if backup != "" {
			_ = os.Rename(backup, target)
		}
		return Skill{}, err
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	hash := skillFilesHash(files)
	st := loadRegistry(progDir)
	rec := st.Skills[manifest.ID]
	if rec.ContentHash != hash {
		rec.Validation = nil
	}
	rec.ContentHash = hash
	st.Skills[manifest.ID] = rec
	if err := saveRegistry(progDir, st); err != nil {
		return Skill{}, err
	}
	return loadSkillPackage(target, manifest.ID, rec)
}

func loadSkillPackage(dir, id string, rec SkillRegistryRecord) (Skill, error) {
	mb, err := os.ReadFile(filepath.Join(dir, "skill.json"))
	if err != nil {
		return Skill{}, err
	}
	var m SkillManifest
	if err = json.Unmarshal(mb, &m); err != nil {
		return Skill{}, err
	}
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(m.EntryPoint)))
	if err != nil {
		return Skill{}, err
	}
	s := Skill{ID: m.ID, Name: m.Name, Description: m.Description, Category: m.Category, Languages: m.Languages, AppliesTo: m.AppliesTo, EntryPoint: m.EntryPoint, Content: string(body), Source: "user", ContentHash: rec.ContentHash, Validation: rec.Validation}
	if parsed, parseErr := parseSkillFile(string(body), "user"); parseErr == nil {
		s.Content = parsed.Content
	}
	if len(m.Languages) == 1 {
		s.Lang = m.Languages[0]
	}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
		if e == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			rel = filepath.ToSlash(rel)
			if rel != "skill.json" && rel != m.EntryPoint {
				s.Resources = append(s.Resources, rel)
				if ext := strings.ToLower(filepath.Ext(rel)); ext == ".md" || ext == ".txt" {
					if b, readErr := os.ReadFile(path); readErr == nil {
						chunk := "\n\n## " + rel + "\n\n" + string(b)
						if len(s.ReferenceContent)+len(chunk) <= 64<<10 {
							s.ReferenceContent += chunk
						}
					}
				}
			}
		}
		return nil
	})
	sort.Strings(s.Resources)
	return s, nil
}

func LoadGlobalSkills(progDir string) []Skill {
	root := SkillLibraryDir(progDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	st := loadRegistry(progDir)
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if s, err := loadSkillPackage(filepath.Join(root, e.Name()), e.Name(), st.Skills[e.Name()]); err == nil {
			out = append(out, s)
		}
	}
	return out
}

func SetSkillValidation(progDir, id string, report *SkillValidationReport) error {
	st := loadRegistry(progDir)
	rec, ok := st.Skills[id]
	if !ok {
		return fmt.Errorf("skill not found")
	}
	if report != nil {
		report.ContentHash = rec.ContentHash
	}
	rec.Validation = report
	st.Skills[id] = rec
	return saveRegistry(progDir, st)
}

func DeleteGlobalSkill(progDir, id string) error {
	if !skillIDPattern.MatchString(id) {
		return fmt.Errorf("invalid skill id")
	}
	target := filepath.Join(SkillLibraryDir(progDir), id)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	st := loadRegistry(progDir)
	delete(st.Skills, id)
	return saveRegistry(progDir, st)
}

func NextOptimizedSkillID(progDir, base string) string {
	candidate := base + "-optimized"
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(SkillLibraryDir(progDir), candidate)); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-optimized-%d", base, n)
	}
}
