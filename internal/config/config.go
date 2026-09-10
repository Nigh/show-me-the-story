package config

import (
	"encoding/json"
	"fmt"
	"os"
	"showmethestory/internal/fsutil"
	"showmethestory/internal/i18n"
)

type APIConfig struct {
	APIKey              string `json:"api_key"`
	BaseURL             string `json:"base_url"`
	URLStrict           bool   `json:"url_strict,omitempty"` // true = 不自动插入 /v1，仅补 /chat/completions
	Model               string `json:"model"`
	MaxTokens           int    `json:"max_tokens,omitempty"` // 0 = 模型默认；新建默认 32768；Agent 调用建议 ≥ 8192
	HTTPTimeoutSeconds  int    `json:"http_timeout_seconds"`
	ContextBudgetTokens int    `json:"context_budget_tokens"` // 模型上下文预算，默认 300000
}

type Config struct {
	ProjectFormatVersion int           `json:"project_format_version"`
	CreatedWithVersion   string        `json:"created_with_version,omitempty"`
	Language             string        `json:"language"` // "zh" 或 "en"，影响 AI 提示词与生成内容
	Story                StoryConfig   `json:"story"`
	Prompts              PromptsConfig `json:"prompts"`
	SkillConfig          *SkillConfig  `json:"skill_config,omitempty"`
}

type StoryConfig struct {
	Type                  string `json:"type"`
	Title                 string `json:"title"`
	TargetWordsPerChapter int    `json:"target_words_per_chapter"`
	WritingStyle          string `json:"writing_style"`
	WritingPOV            string `json:"writing_pov"` // 叙述视角，如第一人称女主、第三人称限知等
}

type PromptsConfig struct {
	ChapterWriting                string `json:"chapter_writing"`
	ChapterRevision               string `json:"chapter_revision"`
	ChapterSegmentRevision        string `json:"chapter_segment_revision"`
	ChapterSummary                string `json:"chapter_summary"`
	FactCheck                     string `json:"fact_check"`
	OutlineRevision               string `json:"outline_revision"`
	ForeshadowPlanning            string `json:"foreshadow_planning"`
	ForeshadowUpdate              string `json:"foreshadow_update"`
	ContinuationOutlineGeneration string `json:"continuation_outline_generation"`
	SettingsReconciliation        string `json:"settings_reconciliation"`
	TransitionSmoothing           string `json:"transition_smoothing"`
	OutlineConsistencyCheck       string `json:"outline_consistency_check"`
	ForeshadowOutlineConsistency  string `json:"foreshadow_outline_consistency"`
	OutlineCharacterCheck         string `json:"outline_character_check"`
	WritingConflictAnalysis       string `json:"writing_conflict_analysis"`
	BookDiagnosis                 string `json:"book_diagnosis"`
	BookConsistencyCheck          string `json:"book_consistency_check"`
	BookRoadmap                   string `json:"book_roadmap"`
	MemoryUpdate                  string `json:"memory_update"`
	HistoryCompression            string `json:"history_compression,omitempty"`
	ImportMetaAnalysis            string `json:"import_meta_analysis"`
	ImportChapterAnalysis         string `json:"import_chapter_analysis"`
}

// DefaultContextBudgetTokens is the fallback context budget when the model's
// real context window cannot be fetched.
const DefaultContextBudgetTokens = 300000

// DefaultMaxTokens is the max_tokens written into a freshly created api.json.
const DefaultMaxTokens = 32768

// DefaultHTTPTimeoutSeconds is the HTTP client timeout for API calls.
const DefaultHTTPTimeoutSeconds = 600

// ProjectFormatVersion is the only on-disk project layout this binary writes.
const ProjectFormatVersion = 4

func DefaultAPIConfig() *APIConfig {
	return &APIConfig{
		MaxTokens:           DefaultMaxTokens,
		HTTPTimeoutSeconds:  DefaultHTTPTimeoutSeconds,
		ContextBudgetTokens: DefaultContextBudgetTokens,
	}
}

func DefaultConfig() *Config {
	return DefaultConfigForLang(i18n.LangZH)
}

func DefaultConfigForLang(lang string) *Config {
	lang = i18n.NormalizeLanguage(lang)
	cfg := &Config{
		ProjectFormatVersion: ProjectFormatVersion,
		Language:             lang,
		Story: StoryConfig{
			TargetWordsPerChapter: 5000,
		},
		SkillConfig: &SkillConfig{
			EnabledSkills: make(map[string]bool),
		},
	}
	cfg.Prompts.ApplyDefaults(lang)
	return cfg
}

func LoadAPIConfig(path string) (*APIConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultAPIConfig()
			if saveErr := saveAPIConfig(path, cfg); saveErr != nil {
				return nil, fmt.Errorf("创建默认API配置文件失败: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("读取API配置文件失败: %w", err)
	}

	var cfg APIConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析API配置文件失败: %w", err)
	}

	if cfg.HTTPTimeoutSeconds <= 0 {
		cfg.HTTPTimeoutSeconds = DefaultHTTPTimeoutSeconds
	}
	// ContextBudgetTokens <= 0 is filled in by llm.EnsureContextBudget at
	// startup (needs an API round-trip, so it lives outside this package).

	return &cfg, nil
}

func saveAPIConfig(path string, cfg *APIConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data)
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			if saveErr := SaveConfig(path, cfg); saveErr != nil {
				return nil, fmt.Errorf("创建默认配置文件失败: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if cfg.Story.TargetWordsPerChapter <= 0 {
		cfg.Story.TargetWordsPerChapter = 5000
	}

	cfg.Language = i18n.NormalizeLanguage(cfg.Language)

	// 保存 applyDefaults 前的 prompts 状态，用于判断是否有字段被填充
	oldPrompts := cfg.Prompts
	cfg.Prompts.ApplyDefaults(cfg.Language)
	// 如果有字段被填充（从空变为默认值），写回磁盘
	if cfg.Prompts != oldPrompts {
		SaveConfig(path, &cfg)
	}

	if cfg.SkillConfig == nil {
		cfg.SkillConfig = &SkillConfig{
			EnabledSkills: make(map[string]bool),
		}
	} else {
		cfg.SkillConfig.ApplyDefaults()
	}

	return &cfg, nil
}

type SkillConfig struct {
	EnabledSkills map[string]bool `json:"enabled_skills"`
}

func (sc *SkillConfig) ApplyDefaults() {
	if sc.EnabledSkills == nil {
		sc.EnabledSkills = make(map[string]bool)
	}
}

func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data)
}

// ApplyDefaults fills empty fields with language-specific defaults without
// overwriting customized prompts.
func (p *PromptsConfig) ApplyDefaults(lang string) {
	defaults := DefaultPromptsForLang(lang)
	if p.ChapterWriting == "" {
		p.ChapterWriting = defaults.ChapterWriting
	}
	if p.ChapterRevision == "" {
		p.ChapterRevision = defaults.ChapterRevision
	}
	if p.ChapterSegmentRevision == "" {
		p.ChapterSegmentRevision = defaults.ChapterSegmentRevision
	}
	if p.ChapterSummary == "" {
		p.ChapterSummary = defaults.ChapterSummary
	}
	if p.FactCheck == "" {
		p.FactCheck = defaults.FactCheck
	}
	if p.OutlineRevision == "" {
		p.OutlineRevision = defaults.OutlineRevision
	}
	if p.ForeshadowPlanning == "" {
		p.ForeshadowPlanning = defaults.ForeshadowPlanning
	}
	if p.ForeshadowUpdate == "" {
		p.ForeshadowUpdate = defaults.ForeshadowUpdate
	}
	if p.ContinuationOutlineGeneration == "" {
		p.ContinuationOutlineGeneration = defaults.ContinuationOutlineGeneration
	}
	if p.SettingsReconciliation == "" {
		p.SettingsReconciliation = defaults.SettingsReconciliation
	}
	if p.TransitionSmoothing == "" {
		p.TransitionSmoothing = defaults.TransitionSmoothing
	}
	if p.OutlineConsistencyCheck == "" {
		p.OutlineConsistencyCheck = defaults.OutlineConsistencyCheck
	}
	if p.ForeshadowOutlineConsistency == "" {
		p.ForeshadowOutlineConsistency = defaults.ForeshadowOutlineConsistency
	}
	if p.OutlineCharacterCheck == "" {
		p.OutlineCharacterCheck = defaults.OutlineCharacterCheck
	}
	if p.WritingConflictAnalysis == "" {
		p.WritingConflictAnalysis = defaults.WritingConflictAnalysis
	}
	if p.BookDiagnosis == "" {
		p.BookDiagnosis = defaults.BookDiagnosis
	}
	if p.BookConsistencyCheck == "" {
		p.BookConsistencyCheck = defaults.BookConsistencyCheck
	}
	if p.BookRoadmap == "" {
		p.BookRoadmap = defaults.BookRoadmap
	}
	if p.MemoryUpdate == "" {
		p.MemoryUpdate = defaults.MemoryUpdate
	}
	if p.HistoryCompression == "" {
		p.HistoryCompression = defaults.HistoryCompression
	}
	if p.ImportMetaAnalysis == "" {
		p.ImportMetaAnalysis = defaults.ImportMetaAnalysis
	}
	if p.ImportChapterAnalysis == "" {
		p.ImportChapterAnalysis = defaults.ImportChapterAnalysis
	}
}

func DefaultPromptsForLang(lang string) PromptsConfig {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return DefaultPromptsEN
	}
	return DefaultPromptsZH
}
