package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type APIConfig struct {
	APIKey               string `json:"api_key"`
	BaseURL              string `json:"base_url"`
	Model                string `json:"model"`
	HTTPTimeoutSeconds   int    `json:"http_timeout_seconds"`
	ContextBudgetTokens  int    `json:"context_budget_tokens"` // 全书优化上下文预算，默认 900000
}

type Config struct {
	Story       StoryConfig   `json:"story"`
	Prompts     PromptsConfig `json:"prompts"`
	SkillConfig *SkillConfig  `json:"skill_config,omitempty"`
}

type StoryConfig struct {
	Type                  string `json:"type"`
	Title                 string `json:"title"`
	ChapterCount          int    `json:"chapter_count"`
	TargetWordsPerChapter int    `json:"target_words_per_chapter"`
	WritingStyle          string `json:"writing_style"`
	StorySynopsis         string `json:"story_synopsis"`
}

type PromptsConfig struct {
	OutlineGeneration             string `json:"outline_generation"`
	ChapterWriting                string `json:"chapter_writing"`
	ChapterRevision               string `json:"chapter_revision"`
	ChapterSummary                string `json:"chapter_summary"`
	FactCheck                     string `json:"fact_check"`
	OutlineRevision               string `json:"outline_revision"`
	ForeshadowPlanning            string `json:"foreshadow_planning"`
	ForeshadowUpdate              string `json:"foreshadow_update"`
	ContentAnalysis               string `json:"content_analysis"`
	ContinuationOutlineGeneration string `json:"continuation_outline_generation"`
	SettingsReconciliation        string `json:"settings_reconciliation"`
	TransitionSmoothing           string `json:"transition_smoothing"`
	OutlineConsistencyCheck       string `json:"outline_consistency_check"`
	BookDiagnosis                 string `json:"book_diagnosis"`
	BookConsistencyCheck          string `json:"book_consistency_check"`
	BookRoadmap                   string `json:"book_roadmap"`
}

func DefaultAPIConfig() *APIConfig {
	return &APIConfig{
		HTTPTimeoutSeconds:  300,
		ContextBudgetTokens: defaultContextBudgetTokens,
	}
}

func DefaultConfig() *Config {
	cfg := &Config{
		Story: StoryConfig{
			ChapterCount:          30,
			TargetWordsPerChapter: 2500,
		},
		SkillConfig: &SkillConfig{
			EnabledSkills: make(map[string]bool),
		},
	}
	cfg.Prompts.applyDefaults()
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
		cfg.HTTPTimeoutSeconds = 300
	}
	if cfg.ContextBudgetTokens <= 0 {
		cfg.ContextBudgetTokens = defaultContextBudgetTokens
	}

	return &cfg, nil
}

func saveAPIConfig(path string, cfg *APIConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			if saveErr := saveConfig(path, cfg); saveErr != nil {
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

	if cfg.Story.ChapterCount <= 0 {
		cfg.Story.ChapterCount = 30
	}
	if cfg.Story.TargetWordsPerChapter <= 0 {
		cfg.Story.TargetWordsPerChapter = 2500
	}

	cfg.Prompts.applyDefaults()

	if cfg.SkillConfig == nil {
		cfg.SkillConfig = &SkillConfig{
			EnabledSkills: make(map[string]bool),
		}
	} else {
		cfg.SkillConfig.applyDefaults()
	}

	return &cfg, nil
}

func saveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func (p *PromptsConfig) applyDefaults() {
	if p.OutlineGeneration == "" {
		p.OutlineGeneration = DefaultPrompts.OutlineGeneration
	}
	if p.ChapterWriting == "" {
		p.ChapterWriting = DefaultPrompts.ChapterWriting
	}
	if p.ChapterRevision == "" {
		p.ChapterRevision = DefaultPrompts.ChapterRevision
	}
	if p.ChapterSummary == "" {
		p.ChapterSummary = DefaultPrompts.ChapterSummary
	}
	if p.FactCheck == "" {
		p.FactCheck = DefaultPrompts.FactCheck
	}
	if p.OutlineRevision == "" {
		p.OutlineRevision = DefaultPrompts.OutlineRevision
	}
	if p.ForeshadowPlanning == "" {
		p.ForeshadowPlanning = DefaultPrompts.ForeshadowPlanning
	}
	if p.ForeshadowUpdate == "" {
		p.ForeshadowUpdate = DefaultPrompts.ForeshadowUpdate
	}
	if p.ContentAnalysis == "" {
		p.ContentAnalysis = DefaultPrompts.ContentAnalysis
	}
	if p.ContinuationOutlineGeneration == "" {
		p.ContinuationOutlineGeneration = DefaultPrompts.ContinuationOutlineGeneration
	}
	if p.SettingsReconciliation == "" {
		p.SettingsReconciliation = DefaultPrompts.SettingsReconciliation
	}
	if p.TransitionSmoothing == "" {
		p.TransitionSmoothing = DefaultPrompts.TransitionSmoothing
	}
	if p.OutlineConsistencyCheck == "" {
		p.OutlineConsistencyCheck = DefaultPrompts.OutlineConsistencyCheck
	}
	if p.BookDiagnosis == "" {
		p.BookDiagnosis = DefaultPrompts.BookDiagnosis
	}
	if p.BookConsistencyCheck == "" {
		p.BookConsistencyCheck = DefaultPrompts.BookConsistencyCheck
	}
	if p.BookRoadmap == "" {
		p.BookRoadmap = DefaultPrompts.BookRoadmap
	}
}
