package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"showmethestory/internal/config"
)

const (
	projectCompatibilitySupported = "supported"
	projectCompatibilityLegacy    = "legacy_incompatible"
	projectCompatibilityUnknown   = "unknown_incompatible"
)

// projectCompatibilityError prevents project selection before any project file
// is created or rewritten.
type projectCompatibilityError struct {
	compatibility string
}

func (e *projectCompatibilityError) Error() string {
	return "项目格式不兼容: " + e.compatibility
}

// detectProjectCompatibility only reads project files. Unmarked projects are
// rejected unless their split v3 chapter layout can be verified, because v2
// stores prose inline in progress.json and v3 would otherwise read it as empty.
func detectProjectCompatibility(projectDir string) (string, error) {
	configData, err := os.ReadFile(filepath.Join(projectDir, "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return projectCompatibilityUnknown, nil
		}
		return "", fmt.Errorf("读取项目配置失败: %w", err)
	}
	var configProbe struct {
		ProjectFormatVersion int `json:"project_format_version"`
	}
	if err := json.Unmarshal(configData, &configProbe); err != nil {
		return projectCompatibilityUnknown, nil
	}
	if configProbe.ProjectFormatVersion != 0 {
		if configProbe.ProjectFormatVersion == config.ProjectFormatVersion {
			return projectCompatibilitySupported, nil
		}
		return projectCompatibilityUnknown, nil
	}

	progressData, err := os.ReadFile(filepath.Join(projectDir, "progress.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return projectCompatibilityUnknown, nil
		}
		return "", fmt.Errorf("读取项目进度失败: %w", err)
	}
	var progressProbe struct {
		Chapters []struct {
			Num     int             `json:"num"`
			Content json.RawMessage `json:"content"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(progressData, &progressProbe); err != nil {
		return projectCompatibilityUnknown, nil
	}
	for _, chapter := range progressProbe.Chapters {
		if len(chapter.Content) != 0 {
			return projectCompatibilityLegacy, nil
		}
	}

	chaptersDir := filepath.Join(projectDir, "chapters")
	if info, err := os.Stat(chaptersDir); err != nil || !info.IsDir() {
		return projectCompatibilityUnknown, nil
	}
	for _, chapter := range progressProbe.Chapters {
		if chapter.Num <= 0 {
			return projectCompatibilityUnknown, nil
		}
		path := filepath.Join(chaptersDir, fmt.Sprintf("%06d.json", chapter.Num))
		data, err := os.ReadFile(path)
		if err != nil {
			return projectCompatibilityUnknown, nil
		}
		var chapterProbe struct {
			Num int `json:"num"`
		}
		if json.Unmarshal(data, &chapterProbe) != nil || chapterProbe.Num != chapter.Num {
			return projectCompatibilityUnknown, nil
		}
	}
	return projectCompatibilitySupported, nil
}

func ensureProjectCompatible(projectDir string) error {
	compatibility, err := detectProjectCompatibility(projectDir)
	if err != nil {
		return err
	}
	if compatibility != projectCompatibilitySupported {
		return &projectCompatibilityError{compatibility: compatibility}
	}
	return nil
}

func isProjectCompatibilityError(err error) bool {
	var compatibilityErr *projectCompatibilityError
	return errors.As(err, &compatibilityErr)
}
