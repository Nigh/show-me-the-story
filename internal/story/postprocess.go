package story

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"showmethestory/internal/config"
	"showmethestory/internal/fsutil"
)

type PostProcessState struct {
	SchemaVersion       int                 `json:"schema_version,omitempty"`
	BackupAcknowledged  bool                `json:"backup_acknowledged,omitempty"`
	ContentModified     bool                `json:"content_modified,omitempty"`
	Issues              []ProofreadIssue    `json:"issues,omitempty"`
	Revisions           []ProofreadRevision `json:"revisions,omitempty"`
	ApplyErrors         map[int]string      `json:"apply_errors,omitempty"`
	ProofreadAnalyzedAt string              `json:"proofread_analyzed_at,omitempty"`
	ProofreadAppliedAt  string              `json:"proofread_applied_at,omitempty"`
	AuthorRequirements  string              `json:"author_requirements,omitempty"`
}

const ProofreadSchemaVersion = 1

func NewProofreadState() *PostProcessState {
	return &PostProcessState{SchemaVersion: ProofreadSchemaVersion, ApplyErrors: map[int]string{}}
}

func LoadPostProcess(path string) (*PostProcessState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewProofreadState(), nil
		}
		return nil, fmt.Errorf("读取完稿校订文件失败: %w", err)
	}
	var pp PostProcessState
	if err := json.Unmarshal(data, &pp); err != nil {
		return nil, fmt.Errorf("解析完稿校订文件失败: %w", err)
	}
	if pp.SchemaVersion != ProofreadSchemaVersion {
		return nil, fmt.Errorf("不支持的完稿校订文件版本: %d", pp.SchemaVersion)
	}
	if pp.ApplyErrors == nil {
		pp.ApplyErrors = map[int]string{}
	}
	return &pp, nil
}

func SavePostProcess(path string, pp *PostProcessState) error {
	data, err := json.MarshalIndent(pp, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化完稿校订状态失败: %w", err)
	}
	return fsutil.WriteFileAtomic(path, data)
}

func IsBookFullyAccepted(state *Progress) bool {
	if state == nil || state.BookStatus != BookStatusCompleted || len(state.Chapters) == 0 {
		return false
	}
	for _, ch := range state.Chapters {
		if ch.Status != StatusAccepted || ch.Content == "" {
			return false
		}
	}
	return true
}

func buildAllSettingsText(cfg *config.Config, settings *ProjectSettings, state *Progress) string {
	var sb strings.Builder
	title := preferUserValue(cfg.Story.Title, state.Title)
	fmt.Fprintf(&sb, "标题：%s\n类型：%s\n写作风格：%s\n", title, cfg.Story.Type, cfg.Story.WritingStyle)
	if cfg.Story.WritingPOV != "" {
		fmt.Fprintf(&sb, "叙述视角：%s\n", cfg.Story.WritingPOV)
	}
	fmt.Fprintf(&sb, "梗概：%s\n", BookSynopsis(cfg, state))
	if state.CorePrompt != "" {
		fmt.Fprintf(&sb, "核心提示词：%s\n", state.CorePrompt)
	}
	if settings != nil && len(settings.Characters) > 0 {
		sb.WriteString("\n【角色设定】\n")
		for _, c := range settings.Characters {
			fmt.Fprintf(&sb, "· %s（%s）\n  性格：%s\n  背景：%s\n  能力：%s\n", c.Name, c.Age, c.Personality, c.Background, c.Abilities)
		}
	}
	if settings != nil && len(settings.Worldview) > 0 {
		sb.WriteString("\n【世界观】\n")
		for _, item := range settings.Worldview {
			fmt.Fprintf(&sb, "· %s（%s）：%s\n", item.Name, item.Category, item.Description)
		}
	}
	if settings != nil && len(settings.Organizations) > 0 {
		sb.WriteString("\n【组织】\n")
		for _, org := range settings.Organizations {
			fmt.Fprintf(&sb, "· %s（%s）：%s\n", org.Name, org.Type, org.Description)
		}
	}
	if len(state.Foreshadows) > 0 {
		sb.WriteString("\n【伏笔】\n")
		for _, fs := range state.Foreshadows {
			fmt.Fprintf(&sb, "· %s [埋设第%d章→预计第%d章回收] 状态:%s — %s\n", fs.Name, fs.PlantChapter, fs.TargetChapter, fs.Status, fs.Description)
		}
	}
	return sb.String()
}
