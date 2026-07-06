package main

import "strings"

func RenderPrompt(template string, data map[string]string) string {
	result := template
	for key, value := range data {
		result = strings.ReplaceAll(result, "{{."+key+"}}", value)
	}
	return result
}

// DefaultPromptsZH is the Chinese default prompt set.
// EN version lives in prompts_en.go.
var DefaultPromptsZH = PromptsConfig{
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
【每章正文字数】{{.TargetWords}}
【写作风格】{{.WritingStyle}}
【叙述视角】{{.WritingPOV}}
【故事梗概】{{.StorySynopsis}}

【已登记角色】
{{.CharacterList}}

注意：
1. 大纲需要覆盖完整的故事弧线，从开端到结局
2. 每章 outline 字段须为 {{.OutlineMinWords}}–{{.OutlineMaxWords}} 字（不含章节标题），包含具体情节发展，禁止笼统描述或一两句话敷衍
3. 每章大纲须依次包含：开场场景/地点；本章核心冲突或目标；关键转折或信息点；出场人物（及作用）；章末走向或悬念钩子
4. 优先使用【已登记角色】中的人物；仅因剧情需要方可新增未登记角色，须在其首次出场章节标注「首次登场」并附一行身份或与主角关系说明，且不得出现在更早章节
5. 初遇、身份揭示等一次性事件只能安排在一个章节中发生，避免重复
6. core_prompt 应包含指导整部小说写作的核心提示词，包括写作风格与叙述视角，并明确要求全书视角统一
7. 若【故事类型】【写作风格】【叙述视角】【故事梗概】等字段已由用户提供且非空，JSON 中对应字段请原样返回，不要改写或扩写
8. 请严格以JSON格式输出，不要添加任何额外文字`,

	ChapterWriting: `请为小说《{{.Title}}》创作第 {{.ChapterNum}} 章的正文。

【核心写作提示词】
{{.CorePrompt}}

【故事梗概】
{{.StorySynopsis}}

【前情提要（滚动最近章节进展，请严格承接状态）】
{{.HistorySummary}}

{{.PreviousEnding}}{{.Foreshadows}}{{.Memory}}{{.OutlineConstraints}}【本章创作任务】
章节标题：《{{.ChapterTitle}}》
核心大纲：{{.ChapterOutline}}

【写作风格】{{.WritingStyle}}
【叙述视角】{{.WritingPOV}}
{{.CharacterContext}}
{{.WorldviewContext}}
创作要求：
1. 严格承接前情提要中的人物状态、时间线和已发生事件，不得与之矛盾
2. 只写本章大纲范围内的情节，不要提前透支后续章节的内容
3. 严禁让按章节脉络安排在后续章节才登场或发生的人物、初遇、身份揭示等事件提前出现，也不得以任何形式暗示或剧透
4. 前文已发生的一次性事件（初次见面、身份揭示、关系确立等）只能作为既成事实延续，绝不能在本章重新发生一遍
5. 不要复述前情，开篇直接进入本章场景；若提供了上一章结尾原文，开头必须自然承接其场景、时间与情绪，不要重新铺垫已有内容
6. 人物对话要符合各自的性格设定，避免所有角色说话腔调雷同
7. 多用具体的动作、感官细节和对话推进情节，少用抽象的总结性叙述
8. 章节结尾留出自然的悬念或情绪钩子，但不要写"欲知后事如何"之类的套话
9. 全书叙述视角必须严格统一：按【叙述视角】要求写作，不得擅自切换人称或视角主体（若设定为交替视角，须按既定规则切换）
10. 正文字数须严格控制在 {{.TargetWordsMin}}–{{.TargetWordsMax}} 字（目标 {{.TargetWords}} 字）。超出上限不可接受；只写本章大纲范围内的情节，宁可精简描写也不要写入后续章节的内容
11. 只输出小说正文：禁止出现章节标题、章节号、大纲复述、作者说明、分隔线，以及「第X章」「（第X章正文）」「本章完」「待续」「以下为修订后的第X章完整正文」「以下是第X章」等任何元信息或说明性文字。正文前不要有任何引导语，正文后不要有任何总结语`,

	ChapterRevision: `你是这部小说的作者，现在需要根据修改意见修订第 {{.ChapterNum}} 章《{{.ChapterTitle}}》。

【核心写作提示词】
{{.CorePrompt}}

【前情提要】
{{.HistorySummary}}

【写作风格】{{.WritingStyle}}
【叙述视角】{{.WritingPOV}}
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
4. 全书叙述视角必须严格统一：按【叙述视角】要求写作，不得擅自切换人称或视角主体
5. 输出修改后的完整章节正文（包含未修改的部分）：禁止出现章节标题、章节号、作者说明、分隔线，以及「第X章」「（第X章正文）」「本章完」「待续」「以下为修订后的第X章完整正文」「以下是第X章」等任何元信息或说明性文字。正文前不要有任何引导语，正文后不要有任何总结语`,

	ChapterSegmentRevision: `你是这部小说的作者。用户在第 {{.ChapterNum}} 章《{{.ChapterTitle}}》中框选了原文片段并提出了修改意见，请只针对这部分做最小化修订，其余章节内容不会被改动。

【核心写作提示词】
{{.CorePrompt}}

【前情提要】
{{.HistorySummary}}

【写作风格】{{.WritingStyle}}
【叙述视角】{{.WritingPOV}}
{{.CharacterContext}}
{{.WorldviewContext}}
【用户引用的原文片段】
{{.QuotedText}}

【待修订的原段落】
{{.SegmentOriginal}}

【修改意见】
{{.UserFeedback}}

修订要求（必须严格遵守）：
1. 这是"定向局部修订"：只重写上面【待修订的原段落】中的内容，不要输出章节中的其他段落
2. 保留原段落中的情节进展、人物动作和关键信息，仅针对【修改意见】做必要修改；除非修改意见明确要求，不要扩写、删减或改变情节走向
3. 修订后必须与前情提要及章节中未修改的部分保持事实一致（人名、时间线、设定）
4. 全书叙述视角必须严格统一：按【叙述视角】要求写作，不得擅自切换人称或视角主体
5. 若【待修订的原段落】包含多个自然段（以空行分隔），输出时必须保持相同的段落数量和顺序，并用空行分隔
6. 只输出修订后的段落正文：禁止出现章节标题、章节号、引用片段复述、作者说明、分隔线，以及「第X章」「（第X章正文）」「本章完」「待续」「以下是修订后的段落」等任何元信息或说明性文字。正文前不要有任何引导语，正文后不要有任何总结语`,

	ChapterSummary: `你是一位精准的小说叙事状态分析师，擅长从文学性文本中提取关键叙事要素和人物心理轨迹。你的摘要将作为后续章节创作的前情提要，因此必须保留可延续的状态信息。

请将以下章节压缩为结构化摘要（总字数控制在250字以内）。

请严格按以下格式输出：

【本章核心】一句话概括本章发生了什么（或主角处于什么状态）。
【人物动态】本章出场人物及其关系进展，特别标注初次见面、身份揭示、关系确立等一次性事件（如"A与B初次相识"），无则写"无新进展"。
【心理轨迹】主角当前的心理状态、情绪基调、有无关键的心理转折点。
【状态变化】本章相比上一章，主角在外在（外貌/穿着/行为）或内在（态度/认知）上发生了什么具体变化。如无明显变化则写"延续上章状态"。
【关键细节】提取1-2个最具叙事延续价值的细节，后续章节可能会引用。
【情绪色调】用2-3个词概括本章的整体情绪氛围。

【章节正文】
{{.ChapterContent}}`,

	FactCheck: `你是一位严谨的小说事实核查员。你的任务是检查小说章节中的客观事实矛盾。

请核查以下小说章节与前情提要、章节脉络之间是否存在事实矛盾。

【前情提要】
{{.HistorySummary}}

【本章大纲】
{{.ChapterOutline}}

{{.OutlineConstraints}}{{.Memory}}【待核查章节】
{{.ChapterContent}}

核查范围（仅限以下客观矛盾，其他一概不算问题）：
1. 角色姓名、称呼前后不一致
2. 时间线倒错（如前文已是夜晚，本章无缘由地变回同日清晨）
3. 与前情明确矛盾的事实（如已死亡角色无解释地出现、已损毁物品完好如初）
4. 角色能力/身份与已确立设定直接冲突
5. 提前引入按章节脉络安排在后续章节才登场或发生的人物、初遇、身份揭示等事件
6. 前文已发生的一次性事件（初次见面、身份揭示等）在本章作为新事件重复发生

注意：
- 文风、节奏、详略取舍、剧情合理性等主观问题不属于事实错误，必须判 PASS
- 前情提要和章节脉络都未提及的新信息不算矛盾
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

【已登记角色】
{{.CharacterList}}

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
4. 未锁定章节的 outline 须为 {{.OutlineMinWords}}–{{.OutlineMaxWords}} 字，包含具体情节要素（场景、冲突、转折、人物、章末钩子）；优先使用【已登记角色】
5. 请严格以JSON格式输出，不要添加任何额外文字`,

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
  "writing_pov": "叙述视角描述（如：第三人称限知、第一人称女主视角、第一人称交替男女主视角等）",
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
4. 提取故事元数据：故事类型、写作风格、叙述视角、角色设定、世界观设定
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
【叙述视角】{{.WritingPOV}}

【已有章节】
{{.ExistingOutline}}

【已登记角色】
{{.CharacterList}}

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
2. 每章 outline 字段须为 {{.OutlineMinWords}}–{{.OutlineMaxWords}} 字，包含具体情节发展，禁止笼统描述
3. 每章大纲须包含：开场场景；核心冲突；关键转折；出场人物及作用；章末走向或钩子
4. 优先使用【已登记角色】；新增角色须标注「首次登场」并附一行说明
5. 已有章节中发生过的初遇、身份揭示等一次性事件不得在新章节中重复安排
6. 请严格以JSON格式输出，不要添加任何额外文字`,

	OutlineCharacterCheck: `你是一位严谨的小说设定编辑。请检查完整章节大纲中出现的人物，与角色管理中已登记的角色列表是否一致。

【小说标题】{{.Title}}

【已登记角色】
{{.RegisteredCharacters}}

【完整大纲】
{{.Outline}}

【已确认章节摘要（辅助判断人物是否已在正文中出现）】
{{.AcceptedSummaries}}

任务：
1. 找出在大纲中出场、但不在【已登记角色】列表中的人物（含标注「首次登场」或未标注的新名字）
2. 忽略群体称谓（如「村民」「守卫们」）和未具名的「某人/神秘人」，除非大纲给了明确专名
3. 不要重复报告已在【已登记角色】中的人物

请以JSON格式返回（不要输出任何其他文字）：
{
  "has_suggestions": true,
  "summary": "简要说明",
  "suggestions": [
    {
      "name": "人物名",
      "chapter_num": 5,
      "description": "从大纲提取的一句话描述",
      "role": "与主角关系或叙事作用（可选）"
    }
  ]
}

若无未登记人物，返回：
{"has_suggestions": false, "summary": "大纲人物与已登记角色一致", "suggestions": []}`,

	TransitionSmoothing: `你是一位资深小说编辑，负责优化章节之间的衔接。下面给出上一章的结尾和本章的开头片段，请判断本章开头是否自然承接上一章结尾。

【上一章结尾】
{{.PrevTail}}

【本章（第{{.ChapterNum}}章《{{.ChapterTitle}}》）开头片段】
{{.Opening}}

【本章大纲（仅供理解剧情，不要据此扩写）】
{{.ChapterOutline}}

处理规则（必须严格遵守）：
1. 如果本章开头已经自然承接上一章结尾（场景过渡、时间线、人物状态、情绪基调连贯），只输出 NO_CHANGE 这一个词，不要输出任何其他文字
2. 如果衔接生硬（如场景突兀跳转、重复铺垫已发生内容、人物状态断裂），请重写上面的"本章开头片段"，使其无缝承接上一章结尾
3. 重写是"最小化修改"：保留开头片段中的全部情节和信息，篇幅与原片段相近，只调整承接方式、过渡句和必要细节
4. 只输出重写后的开头片段正文，不要输出标题、解释说明、前后缀标记或上一章内容，不要续写开头片段之外的新内容`,

	OutlineConsistencyCheck: `你是一位严谨的小说策划编辑。在创作本章正文之前，请检查本章大纲是否已与实际写出的前文剧情冲突。

【前情提要（已发生剧情，不可更改）】
{{.HistorySummary}}

{{.PreviousEnding}}【待检查的本章大纲】
第{{.ChapterNum}}章《{{.ChapterTitle}}》：{{.ChapterOutline}}

检查要点（仅限以下客观冲突）：
1. 大纲安排的"初次见面/初识"事件，相关人物在前文是否已经认识
2. 大纲假设的前置条件（人物状态、所在地点、持有物品、信息知晓情况）是否与前文实际情况一致
3. 大纲安排的事件是否在前文已经发生过

处理规则：
- 没有冲突时，conflict 为 false，revised_outline 留空
- 有冲突时，conflict 为 true，并给出修订后的本章大纲：保持本章原有的情节目标、出场人物和在全书中的作用，只做使其与已发生剧情兼容的最小修改（例如把"初次见面"改为"再次相遇"）
- 不要扩写新剧情，不要改变本章篇幅定位，拿不准是否冲突时一律视为不冲突

请以JSON格式返回（不要输出任何其他文字）：
{"conflict": false, "issues": [], "revised_outline": ""}
或
{"conflict": true, "issues": ["冲突描述"], "revised_outline": "修订后的本章大纲"}`,

	ForeshadowOutlineConsistency: `你是一位严谨的小说叙事一致性编辑。请检查伏笔计划与完整章节大纲是否一致。

【小说标题】{{.Title}}
【完整大纲】
{{.Outline}}

【伏笔列表】
{{.Foreshadows}}

【已确认章节摘要】
{{.AcceptedSummaries}}

检查要点（仅限客观可判定的问题）：
1. 每条未回收、未放弃的伏笔，其 plant_chapter 是否在大纲对应章节中有合理的埋设空间
2. target_chapter 对应章节的大纲是否包含回收该伏笔的情节空间（不要求逐字对应，但逻辑上应能承接）
3. 伏笔描述是否与大纲主线结构性矛盾（按现有大纲不可能实现）
4. plant_chapter / target_chapter 是否超出实际章节总数
5. 已确认章节摘要是否与伏笔的埋设/回收计划明显冲突

请以JSON格式返回（不要输出任何其他文字）：
{
  "has_conflicts": false,
  "conflicts": [],
  "summary": "一句话总结"
}
或
{
  "has_conflicts": true,
  "conflicts": [
    {
      "foreshadow_id": 1,
      "foreshadow_name": "伏笔简称",
      "conflict_type": "missing_payoff|weak_payoff|missing_plant|structural|out_of_range",
      "description": "具体冲突描述",
      "suggested_fix": "revise_outline|adjust_foreshadow|abandon"
    }
  ],
  "summary": "一句话总结"
}

无冲突时 has_conflicts 必须为 false 且 conflicts 为空数组。拿不准时视为无冲突。`,

	WritingConflictAnalysis: `你是一位资深小说编辑。章节正文在事实核查环节已连续多次失败，请分析根本原因并给出处理建议。

【本章信息】
第{{.ChapterNum}}章《{{.ChapterTitle}}》

【本章大纲】
{{.ChapterOutline}}

【前情提要】
{{.HistorySummary}}

{{.OutlineConstraints}}{{.Foreshadows}}【事实核查累计失败项】
{{.FailedIssues}}

【当前章节正文节选（供参考）】
{{.ContentExcerpt}}

分析任务：
1. 判断失败是否由大纲、伏笔、前情之间的不可调和矛盾导致
2. 若可在不改大纲/伏笔的前提下调和：给出一段可直接注入写作 prompt 的「补充约束」（extra_constraints），指导 AI 写出能通过事实核查的正文
3. 若不可调和：说明原因，并建议用户应修改大纲还是调整伏笔等

返回 JSON（不要输出任何其他文字）：
{
  "reconcilable": true,
  "summary": "一句话总结根因",
  "root_cause": "foreshadow_outline|outline_history|foreshadow_history|mixed|other",
  "extra_constraints": "补充约束全文（reconcilable 为 true 时必填）",
  "suggested_actions": [
    {"id": "edit_outline", "label": "修改本章大纲", "description": "说明应如何改大纲"},
    {"id": "adjust_foreshadow", "label": "调整伏笔", "description": "说明应如何改伏笔"},
    {"id": "force_review", "label": "保留当前稿进入审核", "description": "接受当前版本，人工后续处理"}
  ]
}

reconcilable 为 false 时 extra_constraints 留空；suggested_actions 至少包含 edit_outline、adjust_foreshadow、force_review 三项。`,

	SettingsReconciliation: `你是一位专业的小说一致性审查编辑。用户修改了故事设定，但已有部分已确认章节。请检查新设定与已有内容的一致性，并自动调整设定使其兼容。

【用户的新设定】
故事类型：{{.NewType}}
写作风格：{{.NewWritingStyle}}
叙述视角：{{.NewWritingPOV}}
故事梗概：{{.NewStorySynopsis}}

【已有已确认章节摘要】
{{.ExistingSummaries}}

请以JSON格式返回调整后的设定：
{
  "type": "...",
  "writing_style": "...",
  "writing_pov": "...",
  "story_synopsis": "...",
  "explanation": "说明做了哪些调整及原因"
}

调整原则：
1. 已有章节内容不可更改，设定必须与之兼容
2. 尽量保留用户修改的意图
3. 如有不可调和矛盾，以已有内容为准微调新设定
4. 不冲突的部分直接保留用户新设定`,

	BookDiagnosis: `你是一位资深网文总编辑，擅长长篇完稿后的通读审阅。

【任务】
通读下方材料，输出《全书优化诊断报告》。本轮只诊断，不改写正文。

{{.ModeNote}}

=== 设定与风格 ===
{{.SettingsText}}

=== 章节摘要索引 ===
{{.SummaryIndex}}

=== 全书正文 ===
{{.FullText}}

【输出格式（严格遵守）】
## 一、总评（200字内）
## 二、结构与节奏（标出拖沓段、高潮段、断档段，定位到章节号）
## 三、人设与台词（角色是否脸谱化、口吻是否统一、主角弧光是否完整）
## 四、设定与逻辑硬伤（时间线、战力、地理、伏笔未收/误收）
## 五、文风与 AI 痕迹（套话、排比堆砌、情绪标签化、对话书面化）
## 六、优先修改清单（P0/P1/P2，每条必须包含：章节号、问题类型、一句话描述、建议改法）
- P0 = 影响阅读的逻辑/设定错误
- P1 = 明显影响质感的文风/节奏问题
- P2 = 锦上添花

【约束】
- 不要泛泛而谈，每条问题必须能定位到具体章节
- 不要输出改写后的正文
- 拿不准的问题标注「需精读复核」`,

	BookConsistencyCheck: `你是一位严谨的小说事实核查员。请核查整部小说与设定之间的一致性。

{{.VolumeNote}}

=== 设定 ===
{{.SettingsText}}

=== 章节摘要索引（全书） ===
{{.SummaryIndex}}

=== 正文（本卷） ===
{{.FullText}}

【核查维度】
1. 时间线矛盾（年龄、季节、事件先后）
2. 人物设定矛盾（外貌、能力、称呼、关系）
3. 地理/组织/道具前后不一致
4. 伏笔：已埋未收、误收、重复发生的一次性事件（如初遇写了两次）
5. 章间衔接断裂（上一章结尾与本章开头对不上）

【输出格式】
用 Markdown 表格输出：
| 严重度 | 章节 | 原文摘录（≤30字）| 矛盾说明 | 建议修法（最小改动）|

严重度：致命 / 重要 / 轻微
不要改写全文，只给修法。`,

	BookRoadmap: `你是一位资深小说编辑。请根据以下诊断与核查报告，生成可执行的修改工单。

【诊断报告】
{{.DiagnosisReport}}

【核查报告】
{{.ConsistencyReport}}

【要求】
1. 合并去重，按章节号排序
2. 每章最多 3 条修改项，超出标为二轮
3. type 取值：logic（逻辑）、transition（衔接）、style（文风）、rhythm（节奏）、dialogue（对话）、polish（去AI味润色）
4. priority 取值：P0 / P1 / P2
5. feedback 必须可直接作为修订意见（50–150字），强调最小改动
6. **同一章节的所有问题合并为一条工单**（每章最多 1 条 items），不要在同一章输出多条
7. 建议执行顺序：衔接类 → P0 逻辑 → 文风润色

【输出格式】
只输出 JSON，不要其他文字：
{"items": [{"chapter_num": 1, "type": "logic", "priority": "P0", "feedback": "具体修改意见", "selected": true}]}`,

	MemoryUpdate: `你是一位精准的小说叙事记忆管理员。你的任务是从最新完成的章节中提取关键叙事细节，维护一份跨章节的长期记忆库。

记忆库的目的是弥补前情提要（仅覆盖最近5章）的信息缺口——记录那些大纲和摘要未体现、但对后续写作有延续价值的具体细节。

【小说标题】{{.Title}}
【本章编号】第{{.ChapterNum}}章
【本章标题】《{{.ChapterTitle}}》

【本章大纲】
{{.ChapterOutline}}

【本章正文】
{{.ChapterContent}}

【已有记忆库】
{{.ExistingMemory}}

【记忆库 token 上限】{{.MemoryMaxTokens}}

提取规则：
1. 只提取**大纲中未体现的**具体叙事细节——大纲已有的高层情节描述不需要记忆
2. 重点记忆以下类型：
   - character：角色的口头禅、习惯动作、外貌细节、情绪微妙变化
   - location：具体地名、场景布置、环境特征
   - item：重要道具、信物、物品的外观和来历
   - event：具体对话中的关键承诺、约定、信息交换
   - promise：角色对他人或自己的承诺、未完成的事项
   - other：其他有延续价值的细节
3. 每条记忆用一句话概括，附带该细节在原文章节中的大致段落序号（从1开始，按段落分隔计算）
4. 如果已有记忆中的某条因本章内容而过时或被推翻，在 updates 中标记删除
5. 如果记忆总数超出 token 上限（约 {{.MemoryMaxTokens}} tokens），在 response 中合并或删除最不重要的条目

请以JSON格式返回：
{
  "new_memories": [
    {"content": "记忆内容描述", "category": "分类", "position": 段落序号}
  ],
  "updates": [
    {"id": 已有记忆ID, "action": "delete", "reason": "删除原因"}
  ]
}

只返回有变化的内容。如果本章没有值得记忆的新细节，返回 {"new_memories": [], "updates": []}。
请严格以JSON格式输出，不要添加任何额外文字。`,
}
