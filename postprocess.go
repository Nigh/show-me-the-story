package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	postprocessVolumeSplitRunes = 150000
	defaultContextBudgetTokens  = 300000
	diffExcerptRunes            = 500
)

// Roadmap item types and statuses.
const (
	RoadmapTypeLogic      = "logic"
	RoadmapTypeTransition = "transition"
	RoadmapTypeStyle      = "style"
	RoadmapTypeRhythm     = "rhythm"
	RoadmapTypeDialogue   = "dialogue"
	RoadmapTypePolish     = "polish"

	RoadmapStatusPending = "pending"
	RoadmapStatusRunning = "running"
	RoadmapStatusDone    = "done"
	RoadmapStatusFailed  = "failed"
	RoadmapStatusSkipped = "skipped"
)

type RoadmapItem struct {
	ID           string `json:"id"`
	ChapterNum   int    `json:"chapter_num"`
	Type         string `json:"type"`
	Priority     string `json:"priority"`
	Feedback     string `json:"feedback"`
	Selected     bool   `json:"selected"`
	Status       string `json:"status"`
	DiffOriginal string `json:"diff_original,omitempty"`
	DiffRevised  string `json:"diff_revised,omitempty"`
	Error        string `json:"error,omitempty"`
}

type PostProcessExecuteOptions struct {
	RunSmoothTransitionsFirst bool `json:"run_smooth_transitions_first"`
	IncludePolish             bool `json:"include_polish"`
}

type PostProcessState struct {
	DiagnosisReport   string                     `json:"diagnosis_report,omitempty"`
	ConsistencyReport string                     `json:"consistency_report,omitempty"`
	Roadmap           []RoadmapItem              `json:"roadmap,omitempty"`
	BundleMode        string                     `json:"bundle_mode,omitempty"`
	VolumeCount       int                        `json:"volume_count,omitempty"`
	TotalBookRunes    int                        `json:"total_book_runes,omitempty"`
	EstimatedTokens   int                        `json:"estimated_tokens,omitempty"`
	DiagnosedAt       string                     `json:"diagnosed_at,omitempty"`
	ConsistencyAt     string                     `json:"consistency_at,omitempty"`
	RoadmapAt         string                     `json:"roadmap_at,omitempty"`
	ExecuteOptions    *PostProcessExecuteOptions `json:"execute_options,omitempty"`
	LastExecuteAt     string                     `json:"last_execute_at,omitempty"`
}

type PostProcessBundle struct {
	SettingsText    string
	SummaryIndex    string
	FullText        string
	TotalRunes      int
	EstimatedTokens int
	Mode            string // full | summary_only
	VolumeCount     int
}

func getContextBudget(apiCfg *APIConfig) int {
	if apiCfg != nil && apiCfg.ContextBudgetTokens > 0 {
		return apiCfg.ContextBudgetTokens
	}
	return defaultContextBudgetTokens
}

func estimateTokensFromRunes(runes int) int {
	return int(float64(runes) * 1.5)
}

func LoadPostProcess(path string) (*PostProcessState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PostProcessState{
				ExecuteOptions: &PostProcessExecuteOptions{
					RunSmoothTransitionsFirst: true,
				},
			}, nil
		}
		return nil, fmt.Errorf("读取全书优化文件失败: %w", err)
	}
	var pp PostProcessState
	if err := json.Unmarshal(data, &pp); err != nil {
		return nil, fmt.Errorf("解析全书优化文件失败: %w", err)
	}
	if pp.ExecuteOptions == nil {
		pp.ExecuteOptions = &PostProcessExecuteOptions{RunSmoothTransitionsFirst: true}
	}
	return &pp, nil
}

func SavePostProcess(path string, pp *PostProcessState) error {
	data, err := json.MarshalIndent(pp, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化全书优化状态失败: %w", err)
	}
	return writeFileAtomic(path, data)
}

func isBookFullyAccepted(state *Progress) bool {
	if state == nil || len(state.Chapters) == 0 {
		return false
	}
	for _, ch := range state.Chapters {
		if ch.Status != StatusAccepted || ch.Content == "" {
			return false
		}
	}
	return true
}

func buildFullBookText(state *Progress) string {
	var sb strings.Builder
	title := state.Title
	if title == "" {
		title = "未命名"
	}
	sb.WriteString(fmt.Sprintf("《%s》\n", title))
	for _, ch := range state.Chapters {
		if ch.Content == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n\n第 %d 章　%s\n\n%s", ch.Num, ch.Title, ch.Content))
	}
	return sb.String()
}

func buildSummaryIndex(state *Progress) string {
	var sb strings.Builder
	for _, ch := range state.Chapters {
		if ch.Content == "" {
			continue
		}
		summary := ch.Summary
		if summary == "" {
			summary = "（无摘要）"
		}
		sb.WriteString(fmt.Sprintf("第%d章《%s》| %s\n", ch.Num, ch.Title, summary))
	}
	return sb.String()
}

func buildAllSettingsText(cfg *Config, settings *ProjectSettings, state *Progress) string {
	var sb strings.Builder
	title := preferUserValue(cfg.Story.Title, state.Title)
	sb.WriteString(fmt.Sprintf("标题：%s\n", title))
	sb.WriteString(fmt.Sprintf("类型：%s\n", cfg.Story.Type))
	sb.WriteString(fmt.Sprintf("写作风格：%s\n", cfg.Story.WritingStyle))
	if cfg.Story.WritingPOV != "" {
		sb.WriteString(fmt.Sprintf("叙述视角：%s\n", cfg.Story.WritingPOV))
	}
	synopsis := preferUserValue(cfg.Story.StorySynopsis, state.StorySynopsis)
	sb.WriteString(fmt.Sprintf("梗概：%s\n", synopsis))
	if state.CorePrompt != "" {
		sb.WriteString(fmt.Sprintf("核心提示词：%s\n", state.CorePrompt))
	}

	if settings != nil && len(settings.Characters) > 0 {
		sb.WriteString("\n【角色设定】\n")
		for _, c := range settings.Characters {
			sb.WriteString(fmt.Sprintf("· %s", c.Name))
			if c.Age != "" {
				sb.WriteString(fmt.Sprintf("（%s）", c.Age))
			}
			sb.WriteString("\n")
			if c.Personality != "" {
				sb.WriteString(fmt.Sprintf("  性格：%s\n", c.Personality))
			}
			if c.Background != "" {
				sb.WriteString(fmt.Sprintf("  背景：%s\n", c.Background))
			}
			if c.Abilities != "" {
				sb.WriteString(fmt.Sprintf("  能力：%s\n", c.Abilities))
			}
		}
	}

	if settings != nil && len(settings.Worldview) > 0 {
		sb.WriteString("\n【世界观】\n")
		for _, w := range settings.Worldview {
			sb.WriteString(fmt.Sprintf("· %s（%s）：%s\n", w.Name, w.Category, w.Description))
		}
	}

	if settings != nil && len(settings.Organizations) > 0 {
		sb.WriteString("\n【组织】\n")
		for _, o := range settings.Organizations {
			sb.WriteString(fmt.Sprintf("· %s（%s）：%s\n", o.Name, o.Type, o.Description))
		}
	}

	if len(state.Foreshadows) > 0 {
		sb.WriteString("\n【伏笔】\n")
		for _, fs := range state.Foreshadows {
			sb.WriteString(fmt.Sprintf("· %s [埋设第%d章→预计第%d章回收] 状态:%s — %s\n",
				fs.Name, fs.PlantChapter, fs.TargetChapter, fs.Status, fs.Description))
		}
	}

	return sb.String()
}

func buildPostProcessBundle(apiCfg *APIConfig, cfg *Config, settings *ProjectSettings, state *Progress) *PostProcessBundle {
	settingsText := buildAllSettingsText(cfg, settings, state)
	summaryIndex := buildSummaryIndex(state)
	fullText := buildFullBookText(state)

	settingsRunes := len([]rune(settingsText))
	summaryRunes := len([]rune(summaryIndex))
	fullRunes := len([]rune(fullText))
	totalRunes := settingsRunes + summaryRunes + fullRunes

	budget := getContextBudget(apiCfg)
	usable := int(float64(budget) * 0.65) // 留 35% 给系统提示与输出

	mode := "full"
	fixedCost := estimateTokensFromRunes(settingsRunes + summaryRunes)
	fullCost := estimateTokensFromRunes(fullRunes)
	if fixedCost+fullCost > usable {
		mode = "summary_only"
	}

	volumeCount := 1
	if fullRunes > postprocessVolumeSplitRunes {
		volumeCount = (fullRunes + postprocessVolumeSplitRunes - 1) / postprocessVolumeSplitRunes
	}

	return &PostProcessBundle{
		SettingsText:    settingsText,
		SummaryIndex:    summaryIndex,
		FullText:        fullText,
		TotalRunes:      totalRunes,
		EstimatedTokens: estimateTokensFromRunes(totalRunes),
		Mode:            mode,
		VolumeCount:     volumeCount,
	}
}

func splitTextByRunes(text string, maxRunes int) []string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}
	var parts []string
	for i := 0; i < len(runes); i += maxRunes {
		end := i + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		parts = append(parts, string(runes[i:end]))
	}
	return parts
}

func excerptForDiff(content string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

func roadmapPriorityOrder(p string) int {
	switch p {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	default:
		return 3
	}
}

func sortRoadmapItems(items []RoadmapItem) {
	// simple bubble-free sort via indices
	type keyed struct {
		item RoadmapItem
		key  int
		idx  int
	}
	var keyedItems []keyed
	for i, it := range items {
		key := roadmapPriorityOrder(it.Priority)*100000 + it.ChapterNum*10
		switch it.Type {
		case RoadmapTypeTransition:
			key += 0
		case RoadmapTypeLogic:
			key += 1
		case RoadmapTypePolish, RoadmapTypeStyle:
			key += 5
		default:
			key += 3
		}
		keyedItems = append(keyedItems, keyed{item: it, key: key, idx: i})
	}
	for i := 0; i < len(keyedItems); i++ {
		for j := i + 1; j < len(keyedItems); j++ {
			if keyedItems[j].key < keyedItems[i].key {
				keyedItems[i], keyedItems[j] = keyedItems[j], keyedItems[i]
			}
		}
	}
	for i, k := range keyedItems {
		items[i] = k.item
	}
}

type roadmapEntry struct {
	ChapterNum int    `json:"chapter_num"`
	Type       string `json:"type"`
	Priority   string `json:"priority"`
	Feedback   string `json:"feedback"`
	Selected   *bool  `json:"selected"`
}

func parseRoadmapJSON(raw string) ([]RoadmapItem, error) {
	cleaned := cleanJSONResponse(raw)

	var wrapper struct {
		Items []roadmapEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(cleaned), &wrapper); err == nil && len(wrapper.Items) > 0 {
		return mapRoadmapEntries(wrapper.Items), nil
	}

	var arr []roadmapEntry
	if err := json.Unmarshal([]byte(cleaned), &arr); err != nil {
		return nil, fmt.Errorf("解析路线图 JSON 失败: %w", err)
	}
	return mapRoadmapEntries(arr), nil
}

func mapRoadmapEntries(entries []roadmapEntry) []RoadmapItem {
	var items []RoadmapItem
	for i, e := range entries {
		if e.ChapterNum <= 0 || strings.TrimSpace(e.Feedback) == "" {
			continue
		}
		typ := e.Type
		if typ == "" {
			typ = RoadmapTypeStyle
		}
		pri := e.Priority
		if pri == "" {
			pri = "P1"
		}
		selected := true
		if e.Selected != nil {
			selected = *e.Selected
		}
		items = append(items, RoadmapItem{
			ID:         fmt.Sprintf("rm_%d", i+1),
			ChapterNum: e.ChapterNum,
			Type:       typ,
			Priority:   pri,
			Feedback:   strings.TrimSpace(e.Feedback),
			Selected:   selected,
			Status:     RoadmapStatusPending,
		})
	}
	return items
}

// DiagnoseBookAction 全书诊断（根据上下文预算自动选择全文或摘要模式）。
func DiagnoseBookAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, settings *ProjectSettings, state *Progress, logger *LogBroadcaster) (string, error) {
	if err := validateAPIConfig(apiCfg); err != nil {
		return "", err
	}
	if !isBookFullyAccepted(state) {
		return "", fmt.Errorf("全书尚未完成（需所有章节已确认）")
	}

	bundle := buildPostProcessBundle(apiCfg, cfg, settings, state)
	logger.InfoKey("log.postprocess_material",
		bundle.TotalRunes, bundle.EstimatedTokens, bundle.Mode)

	fullTextBlock := bundle.FullText
	modeNote := ""
	if bundle.Mode == "summary_only" {
		fullTextBlock = "（正文过长，本轮诊断仅依据章节摘要索引，执行阶段将按章读取正文）"
		modeNote = "注意：因上下文预算限制，本次诊断基于摘要索引而非全文，标为「需精读复核」的问题请在执行前人工确认。"
	}

	userPrompt := RenderPrompt(cfg.Prompts.BookDiagnosis, map[string]string{
		"SettingsText": bundle.SettingsText,
		"SummaryIndex": bundle.SummaryIndex,
		"FullText":     fullTextBlock,
		"ModeNote":     modeNote,
	})
	systemPrompt := SystemPromptFor(cfg.Language, "book_diagnosis")

	resp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if resp == "" {
		return "", fmt.Errorf("全书诊断调用失败或被取消")
	}
	return strings.TrimSpace(resp), nil
}

// ConsistencyCheckBookAction 全书一致性核查（超长书按卷分段核查后合并报告）。
func ConsistencyCheckBookAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, settings *ProjectSettings, state *Progress, logger *LogBroadcaster) (string, error) {
	if err := validateAPIConfig(apiCfg); err != nil {
		return "", err
	}
	if !isBookFullyAccepted(state) {
		return "", fmt.Errorf("全书尚未完成（需所有章节已确认）")
	}

	bundle := buildPostProcessBundle(apiCfg, cfg, settings, state)
	volumes := splitTextByRunes(bundle.FullText, postprocessVolumeSplitRunes)

	if len(volumes) == 1 {
		logger.InfoKey("log.postprocess_consistency_single")
		return runConsistencyCheckVolume(ctx, apiCfg, cfg, bundle.SettingsText, bundle.SummaryIndex, volumes[0], 1, 1, logger)
	}

	logger.InfoKey("log.postprocess_consistency_multi", len(volumes))
	var reports []string
	for i, vol := range volumes {
		if ctx.Err() != nil {
			return "", fmt.Errorf("任务已取消")
		}
		logger.StepInfo(i+1, len(volumes), fmt.Sprintf("正在核查第 %d/%d 卷...", i+1, len(volumes)))
		report, err := runConsistencyCheckVolume(ctx, apiCfg, cfg, bundle.SettingsText, bundle.SummaryIndex, vol, i+1, len(volumes), logger)
		if err != nil {
			return "", err
		}
		reports = append(reports, fmt.Sprintf("### 第 %d/%d 卷\n\n%s", i+1, len(volumes), report))
	}
	return strings.Join(reports, "\n\n---\n\n"), nil
}

func runConsistencyCheckVolume(ctx context.Context, apiCfg *APIConfig, cfg *Config, settingsText, summaryIndex, volumeText string, volIdx, volTotal int, logger *LogBroadcaster) (string, error) {
	volNote := ""
	if volTotal > 1 {
		volNote = fmt.Sprintf("（全书第 %d/%d 卷，请只核查本卷范围内的问题，跨卷矛盾在卷末标注「可能跨卷」）", volIdx, volTotal)
	}
	userPrompt := RenderPrompt(cfg.Prompts.BookConsistencyCheck, map[string]string{
		"SettingsText": settingsText,
		"SummaryIndex": summaryIndex,
		"FullText":     volumeText,
		"VolumeNote":   volNote,
	})
	systemPrompt := SystemPromptFor(cfg.Language, "book_consistency_check")

	resp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if resp == "" {
		return "", fmt.Errorf("一致性核查调用失败或被取消")
	}
	return strings.TrimSpace(resp), nil
}

// BuildRoadmapAction 根据诊断与核查报告生成可执行工单。
func BuildRoadmapAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, diagnosisReport, consistencyReport string, logger *LogBroadcaster) ([]RoadmapItem, error) {
	if err := validateAPIConfig(apiCfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(diagnosisReport) == "" && strings.TrimSpace(consistencyReport) == "" {
		return nil, fmt.Errorf("缺少诊断或核查报告，无法生成路线图")
	}

	userPrompt := RenderPrompt(cfg.Prompts.BookRoadmap, map[string]string{
		"DiagnosisReport":   diagnosisReport,
		"ConsistencyReport": consistencyReport,
	})
	systemPrompt := SystemPromptFor(cfg.Language, "book_roadmap")

	resp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if resp == "" {
		return nil, fmt.Errorf("路线图生成调用失败或被取消")
	}

	items, err := parseRoadmapJSON(resp)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("路线图解析结果为空，请检查 AI 输出格式")
	}
	sortRoadmapItems(items)
	logger.SuccessKey("log.postprocess_roadmap_items", len(items))
	return items, nil
}

type chapterRoadmapBatch struct {
	ChapterNum int
	Indices    []int
}

// groupPendingRoadmapByChapter 将待执行工单按章节分组，同一章的多条工单合并为一次编辑。
func groupPendingRoadmapByChapter(roadmap []RoadmapItem) []chapterRoadmapBatch {
	byChapter := make(map[int][]int)
	var order []int
	for i, item := range roadmap {
		if !item.Selected || item.Status != RoadmapStatusPending {
			continue
		}
		if _, ok := byChapter[item.ChapterNum]; !ok {
			order = append(order, item.ChapterNum)
		}
		byChapter[item.ChapterNum] = append(byChapter[item.ChapterNum], i)
	}
	var batches []chapterRoadmapBatch
	for _, num := range order {
		batches = append(batches, chapterRoadmapBatch{
			ChapterNum: num,
			Indices:    byChapter[num],
		})
	}
	return batches
}

func roadmapTypeLabel(typ string) string {
	switch typ {
	case RoadmapTypeLogic:
		return "逻辑"
	case RoadmapTypeTransition:
		return "衔接"
	case RoadmapTypeStyle:
		return "文风"
	case RoadmapTypeRhythm:
		return "节奏"
	case RoadmapTypeDialogue:
		return "对话"
	case RoadmapTypePolish:
		return "润色"
	default:
		return typ
	}
}

// mergeChapterRoadmapFeedback 合并同章多条工单的修改意见；若全部为润色类则走润色流程。
func mergeChapterRoadmapFeedback(items []RoadmapItem, opts *PostProcessExecuteOptions, hasPolishSkills bool) (polishOnly bool, feedback string) {
	if len(items) == 0 {
		return false, ""
	}
	allPolish := true
	for _, item := range items {
		if item.Type != RoadmapTypePolish {
			allPolish = false
			break
		}
	}
	if allPolish {
		return true, ""
	}

	var parts []string
	for n, item := range items {
		parts = append(parts, fmt.Sprintf("【意见 %d · %s/%s】\n%s",
			n+1, roadmapTypeLabel(item.Type), item.Priority, strings.TrimSpace(item.Feedback)))
	}
	feedback = strings.Join(parts, "\n\n")

	needPolishAppend := false
	if opts != nil && opts.IncludePolish && hasPolishSkills {
		needPolishAppend = true
	}
	hasNonLogic := false
	for _, item := range items {
		if item.Type == RoadmapTypePolish {
			needPolishAppend = true
		}
		if item.Type != RoadmapTypeLogic {
			hasNonLogic = true
		}
	}
	if needPolishAppend && hasNonLogic && hasPolishSkills {
		feedback += "\n\n【附加文风要求】修改完成后顺带去除 AI 套话，对话口语化，不改变情节。"
	}
	return false, feedback
}

func applyBatchResultToRoadmapItems(items []*RoadmapItem, execErr error, diffOriginal, diffRevised string) {
	if execErr != nil {
		for _, item := range items {
			item.Status = RoadmapStatusFailed
			item.Error = execErr.Error()
		}
		return
	}
	status := RoadmapStatusDone
	if diffOriginal == diffRevised {
		status = RoadmapStatusSkipped
	}
	for _, item := range items {
		item.DiffOriginal = diffOriginal
		item.DiffRevised = diffRevised
		item.Status = status
	}
}

// FullPostProcessAnalyzeAction 完整分析流水线：诊断 → 核查 → 路线图。
func FullPostProcessAnalyzeAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, settings *ProjectSettings, state *Progress, pp *PostProcessState, postprocessPath string, logger *LogBroadcaster) error {
	bundle := buildPostProcessBundle(apiCfg, cfg, settings, state)
	pp.BundleMode = bundle.Mode
	pp.VolumeCount = bundle.VolumeCount
	pp.TotalBookRunes = bundle.TotalRunes
	pp.EstimatedTokens = bundle.EstimatedTokens

	logger.StepInfo(1, 3, "正在进行全书诊断...")
	diagnosis, err := DiagnoseBookAction(ctx, apiCfg, cfg, settings, state, logger)
	if err != nil {
		return err
	}
	pp.DiagnosisReport = diagnosis
	pp.DiagnosedAt = time.Now().Format(time.RFC3339)
	logger.PostProcessReport("diagnosis", diagnosis)

	logger.StepInfo(2, 3, "正在进行全书一致性核查...")
	consistency, err := ConsistencyCheckBookAction(ctx, apiCfg, cfg, settings, state, logger)
	if err != nil {
		return err
	}
	pp.ConsistencyReport = consistency
	pp.ConsistencyAt = time.Now().Format(time.RFC3339)
	logger.PostProcessReport("consistency", consistency)

	logger.StepInfo(3, 3, "正在生成优化路线图...")
	roadmap, err := BuildRoadmapAction(ctx, apiCfg, cfg, diagnosis, consistency, logger)
	if err != nil {
		return err
	}
	pp.Roadmap = roadmap
	pp.RoadmapAt = time.Now().Format(time.RFC3339)
	logger.PostProcessRoadmap(pp)

	return SavePostProcess(postprocessPath, pp)
}

// ExecuteRoadmapAction 执行已勾选的优化工单。
func ExecuteRoadmapAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, settings *ProjectSettings, state *Progress, pp *PostProcessState, progressPath, postprocessPath string, skills []Skill, logger *LogBroadcaster) error {
	if err := validateAPIConfig(apiCfg); err != nil {
		return err
	}
	opts := pp.ExecuteOptions
	if opts == nil {
		opts = &PostProcessExecuteOptions{RunSmoothTransitionsFirst: true}
	}

	if opts.RunSmoothTransitionsFirst {
		logger.InfoKey("log.postprocess_smooth_preface")
		if err := SmoothTransitionsAction(ctx, apiCfg, cfg, state, progressPath, logger); err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("任务已取消")
			}
			logger.WarnKey("log.postprocess_smooth_skip", err)
		}
	}

	polishSkills := GetEnabledSkillsByCategory(skills, cfg.SkillConfig, "polish")
	hasPolishSkills := len(polishSkills) > 0

	batches := groupPendingRoadmapByChapter(pp.Roadmap)
	if len(batches) == 0 {
		return fmt.Errorf("没有待执行的已选工单")
	}

	selectedCount := 0
	for _, batch := range batches {
		selectedCount += len(batch.Indices)
	}

	executed := 0
	for step, batch := range batches {
		if ctx.Err() != nil {
			return fmt.Errorf("任务已取消")
		}

		for _, idx := range batch.Indices {
			pp.Roadmap[idx].Status = RoadmapStatusRunning
		}
		_ = SavePostProcess(postprocessPath, pp)

		var batchItems []RoadmapItem
		for _, idx := range batch.Indices {
			batchItems = append(batchItems, pp.Roadmap[idx])
		}

		label := fmt.Sprintf("正在处理第 %d 章", batch.ChapterNum)
		if len(batch.Indices) > 1 {
			label += fmt.Sprintf("（合并 %d 条工单）", len(batch.Indices))
		}
		label += "..."
		logger.StepInfo(step+1, len(batches), label)

		chapterIdx := -1
		for i, ch := range state.Chapters {
			if ch.Num == batch.ChapterNum {
				chapterIdx = i
				break
			}
		}
		if chapterIdx == -1 {
			errMsg := "章节不存在"
			for _, idx := range batch.Indices {
				pp.Roadmap[idx].Status = RoadmapStatusFailed
				pp.Roadmap[idx].Error = errMsg
				logger.PostProcessItemDone(pp.Roadmap[idx])
			}
			_ = SavePostProcess(postprocessPath, pp)
			continue
		}

		diffOriginal := excerptForDiff(state.Chapters[chapterIdx].Content, diffExcerptRunes)
		polishOnly, mergedFeedback := mergeChapterRoadmapFeedback(batchItems, opts, hasPolishSkills)

		var execErr error
		if polishOnly {
			if !hasPolishSkills {
				execErr = fmt.Errorf("没有启用的润色技能")
			} else {
				execErr = PolishChapterAction(ctx, apiCfg, cfg, state, chapterIdx, polishSkills, progressPath, logger)
			}
		} else {
			execErr = ReviseSpecificChapterAction(ctx, apiCfg, cfg, state, progressPath, batch.ChapterNum, mergedFeedback, settings, logger)
		}

		if execErr == nil {
			state.Chapters[chapterIdx].Status = StatusAccepted
			_ = SaveProgress(progressPath, state)
		}

		diffRevised := excerptForDiff(state.Chapters[chapterIdx].Content, diffExcerptRunes)

		var ptrs []*RoadmapItem
		for _, idx := range batch.Indices {
			ptrs = append(ptrs, &pp.Roadmap[idx])
		}
		applyBatchResultToRoadmapItems(ptrs, execErr, diffOriginal, diffRevised)

		if execErr != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("任务已取消")
			}
			logger.WarnKey("log.postprocess_batch_failed", batch.ChapterNum, execErr)
		} else if diffOriginal != diffRevised {
			executed++
			logger.SuccessKey("log.postprocess_batch_done", batch.ChapterNum, len(batch.Indices))
		} else {
			logger.InfoKey("log.postprocess_batch_skip", batch.ChapterNum)
		}

		for _, idx := range batch.Indices {
			logger.PostProcessItemDone(pp.Roadmap[idx])
		}
		_ = SavePostProcess(postprocessPath, pp)
	}

	pp.LastExecuteAt = time.Now().Format(time.RFC3339)
	logger.SuccessKey("log.postprocess_execute_summary", len(batches), selectedCount, executed)
	return SavePostProcess(postprocessPath, pp)
}
