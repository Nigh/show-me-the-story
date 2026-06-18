package main

// messageCatalog holds localized UI/log/agent status strings (key → zh/en template).
// Templates use fmt.Sprintf verbs (%s, %d, %v). Frontend mirrors keys with {0},{1},… placeholders.
var messageCatalog = map[string]map[string]string{
	// ---- Task / handler logs ----
	"log.autoconfirm_on": {
		LangZH: "已开启自动确认模式：每章生成完成后将自动确认并继续生成下一章",
		LangEN: "Auto-confirm enabled: each chapter is automatically confirmed once generation completes, then the next chapter begins.",
	},
	"log.autoconfirm_off": {
		LangZH: "已关闭自动确认模式",
		LangEN: "Auto-confirm disabled",
	},
	"log.outline_cleared_pending": {
		LangZH: "已自动清除旧的大纲（pending 章节）",
		LangEN: "Cleared previous outline (pending chapters)",
	},
	"log.outline_generating": {
		LangZH: "正在生成小说大纲...",
		LangEN: "Generating novel outline...",
	},
	"log.outline_generate_cancelled": {
		LangZH: "大纲生成已取消",
		LangEN: "Outline generation cancelled",
	},
	"log.outline_generate_failed": {
		LangZH: "大纲生成失败: %s",
		LangEN: "Outline generation failed: %s",
	},
	"log.outline_generate_done": {
		LangZH: "大纲生成完成！",
		LangEN: "Outline generation complete.",
	},
	"log.outline_confirmed": {
		LangZH: "大纲已确认，进入写作阶段。",
		LangEN: "Outline confirmed. Entering writing phase.",
	},
	"log.outline_revising": {
		LangZH: "正在根据意见修订大纲...",
		LangEN: "Revising outline based on feedback...",
	},
	"log.outline_revise_cancelled": {
		LangZH: "大纲修订已取消",
		LangEN: "Outline revision cancelled",
	},
	"log.outline_revise_failed": {
		LangZH: "大纲修订失败: %s",
		LangEN: "Outline revision failed: %s",
	},
	"log.outline_revised": {
		LangZH: "大纲已修订。",
		LangEN: "Outline revised.",
	},
	"log.chapter_writing": {
		LangZH: "正在创作第 %d 章...",
		LangEN: "Writing chapter %d...",
	},
	"log.chapter_write_cancelled": {
		LangZH: "章节创作已取消",
		LangEN: "Chapter writing cancelled",
	},
	"log.chapter_write_conflict_pause": {
		LangZH: "章节创作因事实核查冲突暂停，等待你选择处理方向",
		LangEN: "Chapter writing paused due to fact-check conflict — choose how to proceed",
	},
	"log.chapter_write_failed": {
		LangZH: "章节创作失败: %s",
		LangEN: "Chapter writing failed: %s",
	},
	"log.chapter_write_done": {
		LangZH: "第 %d 章《%s》创作完成！",
		LangEN: "Chapter %d \"%s\" complete!",
	},
	"log.chapter_autoconfirm_failed": {
		LangZH: "自动确认失败: %s",
		LangEN: "Auto-confirm failed: %s",
	},
	"log.chapter_autoconfirmed": {
		LangZH: "第 %d 章《%s》已自动确认。",
		LangEN: "Chapter %d \"%s\" auto-confirmed.",
	},
	"log.all_chapters_done": {
		LangZH: "全部章节已创作完成！",
		LangEN: "All chapters generated.",
	},
	"log.autowrite_cancelled": {
		LangZH: "任务已取消，停止自动续写",
		LangEN: "Task cancelled; auto-continue stopped",
	},
	"log.chapter_kept_review": {
		LangZH: "第 %d 章已保留当前稿并进入审核。",
		LangEN: "Chapter %d kept as draft and moved to review.",
	},
	"log.chapter_confirmed": {
		LangZH: "第 %d 章已确认。",
		LangEN: "Chapter %d confirmed.",
	},
	"log.chapter_revising": {
		LangZH: "正在根据意见修改当前章节...",
		LangEN: "Revising current chapter based on feedback...",
	},
	"log.chapter_revise_cancelled": {
		LangZH: "章节修订已取消",
		LangEN: "Chapter revision cancelled",
	},
	"log.chapter_revise_failed": {
		LangZH: "章节修订失败: %s",
		LangEN: "Chapter revision failed: %s",
	},
	"log.chapter_revised": {
		LangZH: "章节已修订。",
		LangEN: "Chapter revised.",
	},
	"log.chapter_specific_revising": {
		LangZH: "正在定向修订第 %d 章...",
		LangEN: "Targeted revision of chapter %d...",
	},
	"log.smooth_transitions_cancelled": {
		LangZH: "章节衔接优化已取消（已完成部分不会丢失）",
		LangEN: "Transition smoothing cancelled (completed parts are preserved)",
	},
	"log.smooth_transitions_failed": {
		LangZH: "章节衔接优化失败: %s",
		LangEN: "Transition smoothing failed: %s",
	},
	"log.chapter_deleted": {
		LangZH: "已删除第 %d 章。",
		LangEN: "Deleted chapter %d.",
	},
	"log.outline_deleted": {
		LangZH: "大纲已删除。",
		LangEN: "Outline deleted.",
	},
	"log.chapter_outline_updated": {
		LangZH: "第 %d 章大纲已更新。",
		LangEN: "Chapter %d outline updated.",
	},
	"log.settings_reconciling": {
		LangZH: "正在协调新设定与已有内容...",
		LangEN: "Reconciling new settings with existing content...",
	},
	"log.settings_reconcile_cancelled": {
		LangZH: "设定协调已取消",
		LangEN: "Settings reconciliation cancelled",
	},
	"log.settings_reconcile_failed": {
		LangZH: "设定协调失败: %s",
		LangEN: "Settings reconciliation failed: %s",
	},
	"log.settings_reconcile_done": {
		LangZH: "设定协调完成！",
		LangEN: "Settings reconciliation complete.",
	},
	"log.delete_file_failed": {
		LangZH: "删除文件 %s 失败: %v",
		LangEN: "Failed to delete file %s: %v",
	},
	"log.chapters_deleted_from": {
		LangZH: "已从第 %d 章删除到末尾，共删除 %d 章。",
		LangEN: "Deleted from chapter %d to end (%d chapters).",
	},
	"log.foreshadow_roadmap_save_failed": {
		LangZH: "伏笔路线图保存失败: %v",
		LangEN: "Failed to save foreshadow roadmap: %v",
	},
	"log.foreshadow_suggesting": {
		LangZH: "正在分析大纲，设计伏笔方案...",
		LangEN: "Analysing outline and designing foreshadow plan...",
	},
	"log.foreshadow_suggest_cancelled": {
		LangZH: "伏笔建议已取消",
		LangEN: "Foreshadow suggestions cancelled",
	},
	"log.foreshadow_suggest_failed": {
		LangZH: "伏笔建议生成失败: %s",
		LangEN: "Foreshadow suggestion failed: %s",
	},
	"log.foreshadow_suggest_done": {
		LangZH: "伏笔建议生成完成，共 %d 条",
		LangEN: "Foreshadow suggestions ready (%d items)",
	},
	"log.continue_analyzing": {
		LangZH: "正在分析已有内容...",
		LangEN: "Analysing existing content...",
	},
	"log.continue_analyze_cancelled": {
		LangZH: "内容分析已取消",
		LangEN: "Content analysis cancelled",
	},
	"log.continue_analyze_failed": {
		LangZH: "内容分析失败: %s",
		LangEN: "Content analysis failed: %s",
	},
	"log.continue_analyze_done": {
		LangZH: "内容分析完成，发现 %d 章",
		LangEN: "Content analysis complete — found %d chapters",
	},
	"log.continue_import_done": {
		LangZH: "续写导入完成，已进入大纲阶段。",
		LangEN: "Continuation import complete — entered outline phase.",
	},
	"log.continuation_outline_generating": {
		LangZH: "正在生成续写大纲...",
		LangEN: "Generating continuation outline...",
	},
	"log.continuation_outline_cancelled": {
		LangZH: "续写大纲生成已取消",
		LangEN: "Continuation outline cancelled",
	},
	"log.continuation_outline_failed": {
		LangZH: "续写大纲生成失败: %s",
		LangEN: "Continuation outline failed: %s",
	},
	"log.continuation_outline_done": {
		LangZH: "续写大纲生成完成！",
		LangEN: "Continuation outline complete.",
	},
	"log.chapter_polish_cancelled": {
		LangZH: "章节润色已取消",
		LangEN: "Chapter polish cancelled",
	},
	"log.chapter_polish_failed": {
		LangZH: "章节润色失败: %s",
		LangEN: "Chapter polish failed: %s",
	},
	"log.postprocess_diagnose_cancelled": {
		LangZH: "全书优化分析已取消",
		LangEN: "Full-book analysis cancelled",
	},
	"log.postprocess_diagnose_failed": {
		LangZH: "全书优化分析失败: %s",
		LangEN: "Full-book analysis failed: %s",
	},
	"log.postprocess_consistency_cancelled": {
		LangZH: "全书一致性核查已取消",
		LangEN: "Consistency check cancelled",
	},
	"log.postprocess_consistency_failed": {
		LangZH: "全书一致性核查失败: %s",
		LangEN: "Consistency check failed: %s",
	},
	"log.postprocess_roadmap_cancelled": {
		LangZH: "路线图生成已取消",
		LangEN: "Roadmap generation cancelled",
	},
	"log.postprocess_roadmap_failed": {
		LangZH: "路线图生成失败: %s",
		LangEN: "Roadmap generation failed: %s",
	},
	"log.postprocess_execute_cancelled": {
		LangZH: "全书优化执行已取消（已完成项不会丢失）",
		LangEN: "Optimisation execution cancelled (completed items are preserved)",
	},
	"log.postprocess_execute_failed": {
		LangZH: "全书优化执行失败: %s",
		LangEN: "Optimisation execution failed: %s",
	},
	"log.child_task_start_failed": {
		LangZH: "无法启动子任务 %s：主任务已结束",
		LangEN: "Cannot start child task %s: parent task already ended",
	},
	"log.save_session_failed": {
		LangZH: "保存会话失败: %v",
		LangEN: "Failed to save chat session: %v",
	},
	"log.chat_cancelled": {
		LangZH: "助理对话已取消",
		LangEN: "Assistant chat cancelled",
	},
	"log.chat_failed": {
		LangZH: "助理回复失败: %v",
		LangEN: "Assistant reply failed: %v",
	},
	"log.chat_done": {
		LangZH: "助理回复完成",
		LangEN: "Assistant reply complete.",
	},
	"log.project_deleted": {
		LangZH: "项目「%s」已删除",
		LangEN: "Project \"%s\" deleted",
	},
	"log.project_created": {
		LangZH: "项目「%s」创建成功",
		LangEN: "Project \"%s\" created",
	},

	// ---- Writing pipeline logs ----
	"log.chapter_start": {
		LangZH: "开始创作第 %d 章: 《%s》",
		LangEN: "Starting chapter %d: \"%s\"",
	},
	"log.outline_check_failed": {
		LangZH: "大纲一致性检查失败: %v（按原大纲继续）",
		LangEN: "Outline consistency check failed: %v (continuing with original outline)",
	},
	"log.outline_auto_revised": {
		LangZH: "本章大纲已自动修订以匹配当前剧情",
		LangEN: "Chapter outline auto-revised to match current story",
	},
	"log.outline_consistent": {
		LangZH: "本章大纲与当前剧情一致 ✓",
		LangEN: "Chapter outline consistent with story ✓",
	},
	"log.prose_done": {
		LangZH: "正文撰写完毕，共 %d 字",
		LangEN: "Prose complete — %d characters",
	},
	"log.summary_done": {
		LangZH: "摘要提炼完成",
		LangEN: "Summary extraction complete",
	},
	"log.factcheck_retry": {
		LangZH: "[事实核查] 发现问题，正在重新生成第 %d 章（第 %d 次重试）...",
		LangEN: "[Fact-check] Issues found — regenerating chapter %d (retry %d)...",
	},
	"log.factcheck_details": {
		LangZH: "核查详情: %s",
		LangEN: "Check details: %s",
	},
	"log.factcheck_max_retries": {
		LangZH: "[事实核查] 已达最大重试次数，正在分析冲突根因...",
		LangEN: "[Fact-check] Max retries reached — analysing conflict root cause...",
	},
	"log.conflict_analyze_failed": {
		LangZH: "冲突分析失败: %v，保留当前版本",
		LangEN: "Conflict analysis failed: %v — keeping current version",
	},
	"log.conflict_retry": {
		LangZH: "检测到可调和冲突，正在按补充约束进行最后一次尝试...",
		LangEN: "Resolvable conflict detected — final attempt with extra constraints...",
	},
	"log.factcheck_constraint_pass": {
		LangZH: "[事实核查] 补充约束尝试通过 ✓",
		LangEN: "[Fact-check] Constrained retry passed ✓",
	},
	"log.factcheck_pass": {
		LangZH: "[事实核查] 通过 ✓",
		LangEN: "[Fact-check] Passed ✓",
	},
	"log.chapter_write_complete": {
		LangZH: "第 %d 章创作完成！",
		LangEN: "Chapter %d writing complete!",
	},
	"log.outline_conflict": {
		LangZH: "第 %d 章大纲与当前剧情冲突: %s",
		LangEN: "Chapter %d outline conflicts with story: %s",
	},
	"log.chapter_modifying": {
		LangZH: "正在修改第 %d 章《%s》...",
		LangEN: "Revising chapter %d \"%s\"...",
	},
	"log.prose_revised": {
		LangZH: "正文修改完毕，共 %d 字",
		LangEN: "Revision complete — %d characters",
	},
	"log.subsequent_outline_failed": {
		LangZH: "后续大纲修订失败: %v（不影响当前章节）",
		LangEN: "Subsequent outline revision failed: %v (current chapter unaffected)",
	},
	"log.subsequent_outline_done": {
		LangZH: "后续大纲修订完成",
		LangEN: "Subsequent outline revision complete",
	},
	"log.chapter_specific_revising_long": {
		LangZH: "正在对第 %d 章《%s》进行定向修订（不影响其他章节）...",
		LangEN: "Targeted revision of chapter %d \"%s\" (other chapters untouched)...",
	},
	"log.prose_specific_revised": {
		LangZH: "正文修订完毕，共 %d 字",
		LangEN: "Targeted revision complete — %d characters",
	},
	"log.chapter_specific_done": {
		LangZH: "第 %d 章定向修订完成（其余章节未受影响）。",
		LangEN: "Chapter %d targeted revision complete (other chapters untouched).",
	},
	"log.fatal_no_retry": {
		LangZH: "致命错误: %v，不再重试",
		LangEN: "Fatal error: %v — not retrying",
	},
	"log.content_gen_retry": {
		LangZH: "正文生成失败: %v。第 %d 次重试，等待 %ds...",
		LangEN: "Prose generation failed: %v. Retry %d, waiting %ds...",
	},
	"log.summary_retry": {
		LangZH: "摘要提炼失败: %v。第 %d 次重试，等待 %ds...",
		LangEN: "Summary extraction failed: %v. Retry %d, waiting %ds...",
	},
	"log.factcheck_api_retry": {
		LangZH: "事实核查失败: %v。第 %d 次重试，等待 %ds...",
		LangEN: "Fact-check failed: %v. Retry %d, waiting %ds...",
	},
	"log.smooth_start": {
		LangZH: "开始章节衔接优化，共 %d 章待检查",
		LangEN: "Starting transition smoothing — %d chapters to check",
	},
	"log.smooth_natural": {
		LangZH: "第 %d 章衔接自然，无需修改",
		LangEN: "Chapter %d transition is smooth — no change needed",
	},
	"log.smooth_optimized": {
		LangZH: "第 %d 章开头已优化并保存",
		LangEN: "Chapter %d opening optimised and saved",
	},
	"log.smooth_done": {
		LangZH: "章节衔接优化完成：检查 %d 章，优化 %d 章",
		LangEN: "Transition smoothing complete: checked %d, optimised %d",
	},
	"log.outline_generate_summary": {
		LangZH: "大纲生成完成，共 %d 章，标题: 《%s》",
		LangEN: "Outline complete — %d chapters, title: \"%s\"",
	},
	"log.outline_revise_summary": {
		LangZH: "大纲已修订，共 %d 章",
		LangEN: "Outline revised — %d chapters",
	},
	"log.reconcile_pending_outline_failed": {
		LangZH: "待定章节大纲重新生成失败: %v（设定已更新）",
		LangEN: "Failed to regenerate pending outlines: %v (settings updated)",
	},
	"log.reconcile_done_explain": {
		LangZH: "设定协调完成。%s",
		LangEN: "Settings reconciliation complete. %s",
	},
	"log.continuation_outline_summary": {
		LangZH: "续写大纲生成完成，新增 %d 章，总计 %d 章",
		LangEN: "Continuation outline complete — added %d chapters (total %d)",
	},
	"log.foreshadow_outline_check_failed": {
		LangZH: "伏笔-大纲一致性检查失败: %v",
		LangEN: "Foreshadow/outline consistency check failed: %v",
	},
	"log.foreshadow_outline_report_save_failed": {
		LangZH: "保存伏笔-大纲检查报告失败: %v",
		LangEN: "Failed to save foreshadow/outline check report: %v",
	},
	"log.foreshadow_outline_check_pass": {
		LangZH: "伏笔与大纲一致性检查通过 ✓",
		LangEN: "Foreshadow/outline consistency check passed ✓",
	},
	"log.foreshadow_plan_parsed": {
		LangZH: "伏笔方案解析完成，共 %d 条",
		LangEN: "Foreshadow plan parsed — %d items",
	},
	"log.foreshadow_status_updated": {
		LangZH: "伏笔状态更新完成，处理 %d 条变更",
		LangEN: "Foreshadow status updated — %d changes",
	},
	"log.foreshadow_sync_failed": {
		LangZH: "伏笔状态更新失败: %v（不影响本章）",
		LangEN: "Foreshadow sync failed: %v (chapter unaffected)",
	},
	"log.foreshadow_sync_summary": {
		LangZH: "伏笔状态已更新（活跃: %d, 已回收: %d）",
		LangEN: "Foreshadow status updated (active: %d, resolved: %d)",
	},
	"log.postprocess_material": {
		LangZH: "全书材料：约 %d 字，预估 %d tokens，诊断模式：%s",
		LangEN: "Book material: ~%d chars, ~%d tokens, diagnosis mode: %s",
	},
	"log.postprocess_consistency_single": {
		LangZH: "开始全书一致性核查（单卷）...",
		LangEN: "Starting full-book consistency check (single volume)...",
	},
	"log.postprocess_consistency_multi": {
		LangZH: "正文较长，分 %d 卷进行一致性核查...",
		LangEN: "Long text — consistency check split into %d volumes...",
	},
	"log.postprocess_roadmap_items": {
		LangZH: "已生成 %d 条优化工单",
		LangEN: "Generated %d optimisation roadmap items",
	},
	"log.postprocess_smooth_preface": {
		LangZH: "前置步骤：优化章节衔接...",
		LangEN: "Preface: optimising chapter transitions...",
	},
	"log.postprocess_smooth_skip": {
		LangZH: "章节衔接优化跳过或失败: %v",
		LangEN: "Transition smoothing skipped or failed: %v",
	},
	"log.postprocess_batch_failed": {
		LangZH: "第 %d 章工单失败: %v",
		LangEN: "Chapter %d roadmap batch failed: %v",
	},
	"log.postprocess_batch_done": {
		LangZH: "第 %d 章已完成（合并 %d 条意见）",
		LangEN: "Chapter %d done (merged %d items)",
	},
	"log.postprocess_batch_skip": {
		LangZH: "第 %d 章内容无变化，已跳过",
		LangEN: "Chapter %d unchanged — skipped",
	},
	"log.postprocess_execute_summary": {
		LangZH: "全书优化执行完成：处理 %d 章（共 %d 条工单），有效修改 %d 章",
		LangEN: "Optimisation execution complete: %d chapters (%d items), %d modified",
	},
	"log.api_fatal": {
		LangZH: "致命错误: %v，不再重试",
		LangEN: "Fatal error: %v — not retrying",
	},
	"log.api_retry": {
		LangZH: "API调用失败: %v。第 %d 次重试，等待 %ds...",
		LangEN: "API call failed: %v. Retry %d, waiting %ds...",
	},
	"log.api_stream_retry": {
		LangZH: "流式API调用失败: %v。第 %d 次重试，等待 %ds...",
		LangEN: "Streaming API call failed: %v. Retry %d, waiting %ds...",
	},

	// ---- Agent tool status messages ----
	"agent.task_cancelled": {
		LangZH: "任务已取消",
		LangEN: "Task cancelled",
	},
	"agent.api_failed": {
		LangZH: "Agent API 调用失败: %v",
		LangEN: "Agent API call failed: %v",
	},
	"agent.max_steps": {
		LangZH: "已达到最大工具调用步骤限制。",
		LangEN: "Reached the maximum tool-call step limit.",
	},
	"agent.tool_exec_error": {
		LangZH: "工具执行错误: %v",
		LangEN: "Tool execution error: %v",
	},
	"agent.unknown_tool": {
		LangZH: "未知工具: %s",
		LangEN: "Unknown tool: %s",
	},
	"agent.confirm_required": {
		LangZH: "⚠️ 操作未执行：「%s」是不可逆的危险操作。请先向用户复述影响范围并获得明确同意，确认后携带 confirm=true 重新调用。如果用户的本意是修改内容而非删除，请改用对应的修订工具。",
		LangEN: "⚠️ Not executed: \"%s\" is irreversible. Restate the impact to the user and get explicit consent, then retry with confirm=true. If the user meant to edit content, use the revise tool instead.",
	},
	"agent.no_characters": {
		LangZH: "暂无角色数据",
		LangEN: "No character data",
	},
	"agent.characters_not_found": {
		LangZH: "没有找到匹配的角色",
		LangEN: "No matching characters found",
	},
	"agent.character_not_found": {
		LangZH: "未找到角色: %s",
		LangEN: "Character not found: %s",
	},
	"agent.no_worldview": {
		LangZH: "暂无世界观数据",
		LangEN: "No worldview data",
	},
	"agent.worldview_not_found": {
		LangZH: "没有找到匹配的世界观条目",
		LangEN: "No matching worldview entries",
	},
	"agent.no_organizations": {
		LangZH: "暂无组织数据",
		LangEN: "No organization data",
	},
	"agent.chapter_not_found": {
		LangZH: "未找到第%d章",
		LangEN: "Chapter %d not found",
	},
	"agent.no_outline": {
		LangZH: "暂无大纲",
		LangEN: "No outline yet",
	},
	"agent.no_foreshadows": {
		LangZH: "暂无伏笔",
		LangEN: "No foreshadows yet",
	},
	"agent.search_keyword_required": {
		LangZH: "请提供搜索关键词",
		LangEN: "Search keyword required",
	},
	"agent.search_no_results": {
		LangZH: "未找到相关内容",
		LangEN: "No matching content found",
	},
	"agent.character_created": {
		LangZH: "角色「%s」创建成功 (ID: %s)",
		LangEN: "Character \"%s\" created (ID: %s)",
	},
	"agent.character_updated": {
		LangZH: "角色「%s」已更新",
		LangEN: "Character \"%s\" updated",
	},
	"agent.character_deleted": {
		LangZH: "角色「%s」已删除",
		LangEN: "Character \"%s\" deleted",
	},
	"agent.worldview_created": {
		LangZH: "世界观条目「%s」创建成功 (ID: %s)",
		LangEN: "Worldview entry \"%s\" created (ID: %s)",
	},
	"agent.worldview_updated": {
		LangZH: "世界观条目「%s」已更新",
		LangEN: "Worldview entry \"%s\" updated",
	},
	"agent.worldview_deleted": {
		LangZH: "世界观条目「%s」已删除",
		LangEN: "Worldview entry \"%s\" deleted",
	},
	"agent.config_saved_reconciling": {
		LangZH: "故事配置已保存，正在自动协调已有内容...",
		LangEN: "Story config saved — reconciling existing content...",
	},
	"agent.config_saved": {
		LangZH: "故事配置已保存",
		LangEN: "Story config saved",
	},
	"agent.outline_task_started": {
		LangZH: "大纲生成任务已启动，请等待完成。",
		LangEN: "Outline generation started — please wait.",
	},
	"agent.outline_confirmed": {
		LangZH: "大纲已确认，现在进入写作阶段。",
		LangEN: "Outline confirmed — entering writing phase.",
	},
	"agent.outline_revise_started": {
		LangZH: "大纲修订任务已启动，请等待完成。",
		LangEN: "Outline revision started — please wait.",
	},
	"agent.outline_deleted": {
		LangZH: "大纲已删除。",
		LangEN: "Outline deleted.",
	},
	"agent.chapter_outline_updated": {
		LangZH: "第 %d 章大纲已更新。",
		LangEN: "Chapter %d outline updated.",
	},
	"agent.chapter_task_started": {
		LangZH: "第 %d 章生成任务已启动，请等待完成。",
		LangEN: "Chapter %d generation started — please wait.",
	},
	"agent.chapter_confirmed": {
		LangZH: "第 %d 章《%s》已确认。",
		LangEN: "Chapter %d \"%s\" confirmed.",
	},
	"agent.chapter_revise_started": {
		LangZH: "第 %d 章修订任务已启动（仅修改该章，不影响其他章节），请等待完成。",
		LangEN: "Chapter %d revision started (this chapter only) — please wait.",
	},
	"agent.chapter_deleted": {
		LangZH: "已删除第 %d 章。",
		LangEN: "Deleted chapter %d.",
	},
	"agent.chapters_bulk_delete_confirm": {
		LangZH: "⚠️ 操作未执行：这将永久删除第 %d 章到末尾共 %d 章的全部内容。请先向用户复述此影响范围并获得明确同意，确认后携带 confirm=true 重新调用。如果用户的本意是修改章节内容，请改用 revise_chapter。",
		LangEN: "⚠️ Not executed: this would permanently delete chapters %d through end (%d chapters). Restate the impact and get consent, then retry with confirm=true. To edit content, use revise_chapter.",
	},
	"agent.chapters_deleted_from": {
		LangZH: "已从第 %d 章删除到末尾，共删除 %d 章。",
		LangEN: "Deleted from chapter %d to end (%d chapters).",
	},
	"agent.organization_created": {
		LangZH: "组织「%s」创建成功 (ID: %s)",
		LangEN: "Organization \"%s\" created (ID: %s)",
	},
	"agent.organization_updated": {
		LangZH: "组织「%s」已更新",
		LangEN: "Organization \"%s\" updated",
	},
	"agent.organization_deleted": {
		LangZH: "组织「%s」已删除",
		LangEN: "Organization \"%s\" deleted",
	},
	"agent.organization_not_found": {
		LangZH: "未找到组织: %s",
		LangEN: "Organization not found: %s",
	},
	"agent.relation_created": {
		LangZH: "关系创建成功 (ID: %s)",
		LangEN: "Relation created (ID: %s)",
	},
	"agent.relation_updated": {
		LangZH: "关系已更新 (ID: %s)",
		LangEN: "Relation updated (ID: %s)",
	},
	"agent.relation_deleted": {
		LangZH: "关系已删除",
		LangEN: "Relation deleted",
	},
	"agent.relation_not_found": {
		LangZH: "未找到关系: %s",
		LangEN: "Relation not found: %s",
	},
	"agent.foreshadow_suggest_started": {
		LangZH: "伏笔建议生成任务已启动，请等待完成。",
		LangEN: "Foreshadow suggestion task started — please wait.",
	},
	"agent.foreshadow_created": {
		LangZH: "伏笔「%s」创建成功 (ID: %d)",
		LangEN: "Foreshadow \"%s\" created (ID: %d)",
	},
	"agent.foreshadow_updated": {
		LangZH: "伏笔「%s」已更新",
		LangEN: "Foreshadow \"%s\" updated",
	},
	"agent.foreshadow_deleted": {
		LangZH: "伏笔「%s」已删除",
		LangEN: "Foreshadow \"%s\" deleted",
	},
	"agent.foreshadow_not_found": {
		LangZH: "伏笔 %d 不存在",
		LangEN: "Foreshadow %d not found",
	},
	"agent.skill_toggled": {
		LangZH: "技能「%s」已%s",
		LangEN: "Skill \"%s\" %s",
	},
	"agent.progress_reset": {
		LangZH: "进度已重置。",
		LangEN: "Progress reset.",
	},
}
