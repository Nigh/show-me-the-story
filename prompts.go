package main

import "strings"

func RenderPrompt(template string, data map[string]string) string {
	result := template
	for key, value := range data {
		result = strings.ReplaceAll(result, "{{."+key+"}}", value)
	}
	return result
}

var DefaultPrompts = PromptsConfig{
	OutlineGeneration: `你是一位专业的小说策划编辑。请根据以下约束生成小说大纲。

请以JSON格式返回，结构如下：
{
  "title": "小说标题",
  "core_prompt": "核心写作提示词（用于指导后续各章创作的系统级提示）",
  "core_requirements": "核心写作要求",
  "chapters": [
    {"num": 1, "title": "章节标题", "outline": "本章大纲"},
    ...
  ]
}

【故事类型】{{.StoryType}}
【章节数量】{{.ChapterCount}}
【每章字数】{{.TargetWords}}
【写作风格】{{.WritingStyle}}
【角色设定】{{.CharacterSetting}}
【世界观】{{.WorldSetting}}
【核心写作要求】{{.CoreRequirements}}

注意：
1. 大纲需要覆盖完整的故事弧线，从开端到结局
2. 每章大纲应包含具体的情节发展，而非笼统的描述
3. core_prompt 应包含指导整部小说写作的核心提示词，包括写作风格等
4. 请严格以JSON格式输出，不要添加任何额外文字`,

	ChapterWriting: `请为小说《{{.Title}}》创作第 {{.ChapterNum}} 章的正文。

【核心写作提示词】
{{.CorePrompt}}

【核心写作要求】
{{.CoreRequirements}}

【前情提要（滚动最近章节进展，请严格承接状态）】
{{.HistorySummary}}

{{.Foreshadows}}【本章创作任务】
章节标题：《{{.ChapterTitle}}》
核心大纲：{{.ChapterOutline}}

【写作风格】{{.WritingStyle}}
【角色设定】{{.CharacterSetting}}
{{.CharacterContext}}【世界观】{{.WorldSetting}}
{{.WorldviewContext}}
请直接输出小说正文，字数{{.TargetWords}}字左右。`,

	ChapterSummary: `你是一位精准的小说叙事状态分析师，擅长从文学性文本中提取关键叙事要素和人物心理轨迹。你的摘要将作为后续章节创作的前情提要，因此必须保留可延续的状态信息。

请将以下章节压缩为结构化摘要（总字数控制在200字以内）。

请严格按以下格式输出：

【本章核心】一句话概括本章发生了什么（或主角处于什么状态）。
【心理轨迹】主角当前的心理状态、情绪基调、有无关键的心理转折点。
【状态变化】本章相比上一章，主角在外在（外貌/穿着/行为）或内在（态度/认知）上发生了什么具体变化。如无明显变化则写"延续上章状态"。
【关键细节】提取1-2个最具叙事延续价值的细节，后续章节可能会引用。
【情绪色调】用2-3个词概括本章的整体情绪氛围。

【章节正文】
{{.ChapterContent}}`,

	FactCheck: `你是一位严谨的小说事实核查员。你的任务是检查小说章节中的事实错误，包括但不限于：角色名字一致性、时间线连续性、设定一致性。你必须非常严格，任何事实错误都必须指出。

请核查以下小说章节的事实准确性。

【前情提要】
{{.HistorySummary}}

【待核查章节】
{{.ChapterContent}}

请以JSON格式返回：
{"result": "PASS或FAIL", "issues": ["问题1", "问题2"]}

如无问题，result为PASS，issues为空数组。`,

	OutlineRevision: `你是一位小说策划编辑。用户对当前大纲提出了修改意见，请根据用户意见修订大纲。

【当前大纲】
{{.CurrentOutline}}

【用户意见】
{{.UserFeedback}}

【已确认章节（不可修改）】
{{.LockedChapters}}

请以JSON格式返回修订后的完整大纲：
{
  "title": "小说标题",
  "core_prompt": "核心写作提示词",
  "core_requirements": "核心写作要求",
  "chapters": [
    {"num": 1, "title": "章节标题", "outline": "本章大纲"},
    ...
  ]
}

注意：已锁定的章节内容不可修改，只能修改未锁定的章节。请严格以JSON格式输出。`,

	ForeshadowPlanning: `你是一位资深的小说叙事架构师，擅长设计伏笔系统。请根据以下小说大纲，设计一组伏笔（foreshadowing）方案。

【小说标题】{{.Title}}
【核心写作提示词】{{.CorePrompt}}
【核心写作要求】{{.CoreRequirements}}

【完整大纲】
{{.Outline}}

【角色设定】
{{.CharacterSetting}}

【世界观】
{{.WorldSetting}}

请设计 3-8 条伏笔，遵循以下原则：
1. 伏笔应服务于故事主线和人物弧线，而非为了悬疑而悬疑
2. 每条伏笔应有明确的"埋设点"（在哪章埋下）和"回收点"（预计在哪章回收）
3. 伏笔之间可以相互关联，形成线索网络
4. 伏笔类型多样化：可以是物件、对话中的暗示、环境细节、人物行为的矛盾、未解释的现象等
5. 回收点应分散在不同章节，避免扎堆回收
6. 伏笔从第1章即可开始埋设，但大部分应在故事中段埋设、后半段回收

请以JSON格式返回：
{
  "foreshadows": [
    {
      "name": "伏笔简称（10字以内）",
      "description": "伏笔的详细描述：埋设方式、暗示内容、预期回收时读者应产生的'原来如此'的顿悟感",
      "plant_chapter": 埋设章节编号,
      "target_chapter": 预计回收章节编号
    }
  ]
}

请严格以JSON格式输出，不要添加任何额外文字。`,

	ForeshadowUpdate: `你是一位严谨的小说伏笔追踪员。你的任务是根据最新完成的章节内容，更新伏笔系统的状态。

【小说标题】{{.Title}}

【当前伏笔列表】
{{.Foreshadows}}

【本章信息】
章节编号：第{{.ChapterNum}}章
章节标题：《{{.ChapterTitle}}》

【本章正文】
{{.ChapterContent}}

【前情提要】
{{.HistorySummary}}

请分析本章内容，判断每条伏笔在本章中的状态变化：

1. 如果伏笔在本章被首次提及/埋设，status 设为 "planted"
2. 如果伏笔在本章有新的线索/推进，status 设为 "progressing"
3. 如果伏笔在本章被完全揭示/回收，status 设为 "resolved"
4. 如果伏笔在本章没有出现，保持原状态不变
5. 注意区分"真正回收"和"仅仅是推进"——只有当伏笔的谜底被完全揭开时才算 resolved

请以JSON格式返回：
{
  "updates": [
    {
      "id": 伏笔ID,
      "status": "新状态（如果变化）",
      "event": "本章对该伏笔做了什么（如果有的话，一句话描述）",
      "resolution": "如果resolved，描述回收方式"
    }
  ]
}

只返回有变化的伏笔。如果某条伏笔在本章完全没有被提及，不要包含在返回结果中。
请严格以JSON格式输出，不要添加任何额外文字。`,

	ContentAnalysis: `你是一位专业的小说分析编辑。请分析以下已有小说文本，提取故事元数据、为每章生成大纲和摘要。

请以JSON格式返回，结构如下：
{
  "title": "小说标题",
  "story_type": "故事类型（如：奇幻/都市/科幻/悬疑等）",
  "core_prompt": "核心写作提示词（用于指导后续各章创作的系统级提示）",
  "core_requirements": "核心写作要求",
  "writing_style": "写作风格描述",
  "character_setting": "角色设定",
  "world_setting": "世界观设定",
  "chapters": [
    {
      "num": 1,
      "title": "章节标题",
      "outline": "本章内容概要（描述本章发生了什么，100-200字）",
      "summary": "结构化摘要（用于后续创作的前情提要，200字以内，包含核心事件、心理轨迹、状态变化、关键细节）"
    }
  ]
}

分析要求：
1. 从文本中识别章节边界（支持"第X章"、"# Chapter X"、空行分隔等常见格式）
2. 为每章生成：outline（本章内容概要）和 summary（用于后续创作的结构化摘要）
3. summary 需保留可延续的状态信息：核心事件、心理轨迹、关键细节、情绪色调
4. 提取故事元数据：故事类型、写作风格、角色设定、世界观设定
5. 生成 core_prompt 和 core_requirements，用于指导后续创作

【已有小说文本】
{{.ExistingContent}}

请严格以JSON格式输出，不要添加任何额外文字。`,

	ContinuationOutlineGeneration: `你是一位专业的小说策划编辑。请根据已有章节的大纲和摘要，为后续章节生成大纲。

【小说标题】{{.Title}}
【故事类型】{{.StoryType}}
【核心写作提示词】{{.CorePrompt}}
【核心写作要求】{{.CoreRequirements}}
【写作风格】{{.WritingStyle}}
【角色设定】{{.CharacterSetting}}
【世界观】{{.WorldSetting}}

【已有章节】
{{.ExistingOutline}}

请为后续 {{.NewChapterCount}} 章生成大纲，从第 {{.StartNum}} 章开始。

请以JSON格式返回：
{
  "chapters": [
    {"num": {{.StartNum}}, "title": "章节标题", "outline": "本章大纲"},
    ...
  ]
}

注意：
1. 大纲需要承接已有章节的故事线，保持连贯性
2. 每章大纲应包含具体的情节发展，而非笼统的描述
3. 请严格以JSON格式输出，不要添加任何额外文字`,

	SettingsReconciliation: `你是一位专业的小说一致性审查编辑。用户修改了故事设定，但已有部分已确认章节。请检查新设定与已有内容的一致性，并自动调整设定使其兼容。

【用户的新设定】
故事类型：{{.NewType}}
写作风格：{{.NewWritingStyle}}
角色设定：{{.NewCharacterSetting}}
世界观：{{.NewWorldSetting}}
核心写作要求：{{.NewCoreRequirements}}

【已有已确认章节摘要】
{{.ExistingSummaries}}

请以JSON格式返回调整后的设定：
{
  "type": "...",
  "writing_style": "...",
  "character_setting": "...",
  "world_setting": "...",
  "core_requirements": "...",
  "explanation": "说明做了哪些调整及原因"
}

调整原则：
1. 已有章节内容不可更改，设定必须与之兼容
2. 尽量保留用户修改的意图
3. 如有不可调和矛盾，以已有内容为准微调新设定
4. 不冲突的部分直接保留用户新设定`,
}
