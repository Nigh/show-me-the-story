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
  "story_synopsis": "故事梗概",
  "chapters": [
    {"num": 1, "title": "章节标题", "outline": "本章大纲"},
    ...
  ]
}

【故事类型】{{.StoryType}}
【章节数量】{{.ChapterCount}}
【每章字数】{{.TargetWords}}
【写作风格】{{.WritingStyle}}
【故事梗概】{{.StorySynopsis}}

注意：
1. 大纲需要覆盖完整的故事弧线，从开端到结局
2. 每章大纲应包含具体的情节发展，而非笼统的描述
3. core_prompt 应包含指导整部小说写作的核心提示词，包括写作风格等
4. 请严格以JSON格式输出，不要添加任何额外文字`,

	ChapterWriting: `请为小说《{{.Title}}》创作第 {{.ChapterNum}} 章的正文。

【核心写作提示词】
{{.CorePrompt}}

【故事梗概】
{{.StorySynopsis}}

【前情提要（滚动最近章节进展，请严格承接状态）】
{{.HistorySummary}}

{{.Foreshadows}}【本章创作任务】
章节标题：《{{.ChapterTitle}}》
核心大纲：{{.ChapterOutline}}

【写作风格】{{.WritingStyle}}
{{.CharacterContext}}
{{.WorldviewContext}}
创作要求：
1. 严格承接前情提要中的人物状态、时间线和已发生事件，不得与之矛盾
2. 只写本章大纲范围内的情节，不要提前透支后续章节的内容
3. 不要复述前情，开篇直接进入本章场景
4. 人物对话要符合各自的性格设定，避免所有角色说话腔调雷同
5. 多用具体的动作、感官细节和对话推进情节，少用抽象的总结性叙述
6. 章节结尾留出自然的悬念或情绪钩子，但不要写"欲知后事如何"之类的套话
7. 字数 {{.TargetWords}} 字左右
8. 只输出小说正文，不要输出标题、章节号、大纲复述或任何解释说明`,

	ChapterRevision: `你是这部小说的作者，现在需要根据修改意见修订第 {{.ChapterNum}} 章《{{.ChapterTitle}}》。

【核心写作提示词】
{{.CorePrompt}}

【前情提要】
{{.HistorySummary}}

【写作风格】{{.WritingStyle}}
{{.CharacterContext}}
{{.WorldviewContext}}
【本章原文】
{{.OriginalContent}}

【修改意见】
{{.UserFeedback}}

修订要求（必须严格遵守）：
1. 这是"修订"而不是"重写"：仅针对修改意见涉及的部分做必要修改，其余内容保持原文不变（包括措辞、段落结构）
2. 修改后必须与前情提要及未修改部分保持事实一致（人名、时间线、设定）
3. 不要改变本章的整体情节走向，除非修改意见明确要求
4. 输出修改后的完整章节正文（包含未修改的部分），不要输出标题、解释、修改说明或差异标记`,

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

	FactCheck: `你是一位严谨的小说事实核查员。你的任务是检查小说章节中的客观事实矛盾。

请核查以下小说章节与前情提要之间是否存在事实矛盾。

【前情提要】
{{.HistorySummary}}

【待核查章节】
{{.ChapterContent}}

核查范围（仅限以下客观矛盾，其他一概不算问题）：
1. 角色姓名、称呼前后不一致
2. 时间线倒错（如前文已是夜晚，本章无缘由地变回同日清晨）
3. 与前情明确矛盾的事实（如已死亡角色无解释地出现、已损毁物品完好如初）
4. 角色能力/身份与已确立设定直接冲突

注意：
- 文风、节奏、详略取舍、剧情合理性等主观问题不属于事实错误，必须判 PASS
- 前情提要未提及的新信息不算矛盾
- 只有确凿的客观矛盾才判 FAIL，拿不准时一律判 PASS

请以JSON格式返回（不要输出任何其他文字）：
{"result": "PASS", "issues": []}
或
{"result": "FAIL", "issues": ["具体矛盾描述1", "具体矛盾描述2"]}`,

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
  "story_synopsis": "故事梗概",
  "chapters": [
    {"num": 1, "title": "章节标题", "outline": "本章大纲"},
    ...
  ]
}

注意：
1. 已锁定的章节内容不可修改，只能修改未锁定的章节
2. 保持章节总数和编号不变，除非用户意见明确要求增删章节
3. 与用户意见无关的章节保持原样返回，不要顺手改写
4. 请严格以JSON格式输出，不要添加任何额外文字`,

	ForeshadowPlanning: `你是一位资深的小说叙事架构师，擅长设计伏笔系统。请根据以下小说大纲，设计一组伏笔（foreshadowing）方案。

【小说标题】{{.Title}}
【核心写作提示词】{{.CorePrompt}}
【故事梗概】{{.StorySynopsis}}

【完整大纲】
{{.Outline}}

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
  "story_synopsis": "故事梗概",
  "writing_style": "写作风格描述",
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
5. 生成 core_prompt 和 story_synopsis，用于指导后续创作

【已有小说文本】
{{.ExistingContent}}

请严格以JSON格式输出，不要添加任何额外文字。`,

	ContinuationOutlineGeneration: `你是一位专业的小说策划编辑。请根据已有章节的大纲和摘要，为后续章节生成大纲。

【小说标题】{{.Title}}
【故事类型】{{.StoryType}}
【核心写作提示词】{{.CorePrompt}}
【故事梗概】{{.StorySynopsis}}
【写作风格】{{.WritingStyle}}

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
故事梗概：{{.NewStorySynopsis}}

【已有已确认章节摘要】
{{.ExistingSummaries}}

请以JSON格式返回调整后的设定：
{
  "type": "...",
  "writing_style": "...",
  "story_synopsis": "...",
  "explanation": "说明做了哪些调整及原因"
}

调整原则：
1. 已有章节内容不可更改，设定必须与之兼容
2. 尽量保留用户修改的意图
3. 如有不可调和矛盾，以已有内容为准微调新设定
4. 不冲突的部分直接保留用户新设定`,
}
