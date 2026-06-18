package main

// DefaultPromptsEN holds English versions of every prompt template.
// Used when a project's language is set to "en".
var DefaultPromptsEN = PromptsConfig{
	OutlineGeneration: `You are a professional novel-planning editor. Generate a novel outline that satisfies the constraints below.

Return JSON in exactly this structure:
{
  "title": "Novel title",
  "core_prompt": "Core writing prompt (a system-level guideline that will steer every later chapter)",
  "story_synopsis": "Synopsis of the story",
  "chapters": [
    {"num": 1, "title": "Chapter title", "outline": "Outline for this chapter"},
    ...
  ]
}

[Story type] {{.StoryType}}
[Chapter count] {{.ChapterCount}}
[Words per chapter] {{.TargetWords}}
[Writing style] {{.WritingStyle}}
[Narrative POV] {{.WritingPOV}}
[Synopsis] {{.StorySynopsis}}

Rules:
1. The outline must cover the full story arc, from inciting incident to resolution.
2. Each chapter outline must describe concrete plot beats, not vague summaries.
3. Each chapter outline must list the characters who appear; mark each new character with "first appearance" in the chapter they debut, and ensure they do not appear in any earlier chapter.
4. One-time events such as first meetings and identity reveals must happen in exactly one chapter — never repeat them.
5. core_prompt should bundle the directives that guide the whole novel, including writing style and narrative POV, and must require a consistent POV throughout.
6. Output strict JSON only. No extra prose.`,

	ChapterWriting: `Write the prose for chapter {{.ChapterNum}} of the novel "{{.Title}}".

[Core writing prompt]
{{.CorePrompt}}

[Synopsis]
{{.StorySynopsis}}

[Story-so-far (rolling recap of recent chapters — continue from this state strictly)]
{{.HistorySummary}}

{{.PreviousEnding}}{{.Foreshadows}}{{.OutlineConstraints}}[Task for this chapter]
Chapter title: "{{.ChapterTitle}}"
Outline: {{.ChapterOutline}}

[Writing style] {{.WritingStyle}}
[Narrative POV] {{.WritingPOV}}
{{.CharacterContext}}
{{.WorldviewContext}}
Writing rules:
1. Strictly continue from the character states, timeline, and established facts in the story-so-far. Do not contradict them.
2. Stay inside this chapter's outline. Do not borrow material from later chapters.
3. Do NOT preemptively introduce characters, first meetings, identity reveals, or other one-time events that the outline assigns to later chapters — and do not hint at or spoil them.
4. One-time events already played out (first meetings, identity reveals, relationships established) must be treated as established facts and never re-enacted in this chapter.
5. Do not re-summarise the story-so-far. Open straight into this chapter's scene. If a previous-chapter ending is provided, your opening must seamlessly continue its setting, time, and mood without re-establishing what's already there.
6. Each character's dialogue must match their established voice; do not let everyone sound alike.
7. Drive the plot with concrete action, sensory detail, and dialogue. Avoid abstract, summarising narration.
8. Close on a natural cliffhanger or emotional hook. Do not write meta lines like "to be continued".
9. Keep the narrative POV strictly consistent: follow [Narrative POV] throughout; do not switch person or viewpoint subject unless the POV spec explicitly allows alternation.
10. Target length: about {{.TargetWords}} words.
11. Output ONLY the chapter prose — no chapter title, chapter number, outline recap, author notes, dividers, or meta lines such as "Chapter X", "(Chapter X text)", "End of chapter", "To be continued", "Here is the revised chapter", "Below is the full text". Do not add any preamble before the prose or any summary after it.`,

	ChapterRevision: `You are the author of this novel. Revise chapter {{.ChapterNum}} "{{.ChapterTitle}}" according to the feedback below.

[Core writing prompt]
{{.CorePrompt}}

[Story-so-far]
{{.HistorySummary}}

[Writing style] {{.WritingStyle}}
[Narrative POV] {{.WritingPOV}}
{{.CharacterContext}}
{{.WorldviewContext}}
[Current chapter text]
{{.OriginalContent}}

[Revision feedback]
{{.UserFeedback}}

Revision rules (strict):
1. This is a "revision", not a "rewrite". Change only what the feedback requires; leave everything else exactly as written (wording, paragraph structure).
2. The revised chapter must remain consistent with the story-so-far and the unchanged portions (names, timeline, established facts).
3. Do not alter the chapter's overall plot direction unless the feedback explicitly requests it.
4. Keep the narrative POV strictly consistent: follow [Narrative POV] throughout; do not switch person or viewpoint subject unless the POV spec explicitly allows alternation.
5. Output the full revised chapter prose (including the unchanged portions). No chapter title, chapter number, author notes, dividers, or meta lines such as "Chapter X", "(Chapter X text)", "End of chapter", "To be continued", "Here is the revised chapter", "Below is the full text". Do not add any preamble before the prose or any summary after it.`,

	ChapterSummary: `You are a precise novel narrative-state analyst. You distil literary text into the narrative elements and psychological beats that downstream chapters need.

Compress the chapter below into a structured summary of 250 words or fewer.

Use exactly this format:

[Chapter core] One sentence describing what happens (or the protagonist's current state).
[Character beats] Characters that appear and how their relationships move. Explicitly note one-time events such as "A and B meet for the first time" or "B's identity is revealed". If nothing changes, write "no new progress".
[Psychological arc] The protagonist's current mental state, emotional tone, and any pivotal internal turn.
[State changes] What changed about the protagonist (outward: appearance/clothing/behaviour; inward: attitude/perception) compared to the previous chapter. If nothing changed, write "carries over from previous chapter".
[Key details] One or two details with the highest narrative continuation value that later chapters may reference.
[Emotional palette] Two or three words capturing the chapter's mood.

[Chapter text]
{{.ChapterContent}}`,

	FactCheck: `You are a strict novel fact-checker. Your task is to detect objective factual contradictions in the chapter.

Check whether the chapter below contradicts the story-so-far or the outline arc.

[Story-so-far]
{{.HistorySummary}}

[Chapter outline]
{{.ChapterOutline}}

{{.OutlineConstraints}}[Chapter under review]
{{.ChapterContent}}

Scope (only the following count as problems, nothing else):
1. Character names or honorifics inconsistent with prior text.
2. Timeline contradictions (e.g. previous text ended at night, this chapter inexplicably reverts to morning of the same day).
3. Facts that directly contradict established events (a dead character reappearing without explanation, a destroyed object intact again).
4. Character abilities or identity directly clashing with established setting.
5. Premature introduction of characters, first meetings, identity reveals, or other events that the outline assigns to later chapters.
6. One-time events already played out in prior chapters being re-enacted as new in this chapter.

Notes:
- Style, pacing, scene-length choices, and plot plausibility are subjective issues — always PASS them.
- New information that neither the story-so-far nor the outline mentions is not a contradiction.
- Only solid objective contradictions warrant FAIL. When in doubt, PASS.

Return JSON only (no other text):
{"result": "PASS", "issues": []}
or
{"result": "FAIL", "issues": ["concrete contradiction 1", "concrete contradiction 2"]}`,

	OutlineRevision: `You are a novel-planning editor. The user gave revision feedback on the outline. Revise accordingly.

[Current outline]
{{.CurrentOutline}}

[User feedback]
{{.UserFeedback}}

[Locked chapters (must not be changed)]
{{.LockedChapters}}

Return the revised full outline as JSON:
{
  "title": "Novel title",
  "core_prompt": "Core writing prompt",
  "story_synopsis": "Synopsis",
  "chapters": [
    {"num": 1, "title": "Chapter title", "outline": "Outline for this chapter"},
    ...
  ]
}

Rules:
1. Locked chapter contents may not be changed; only unlocked chapters may be edited.
2. Keep the total chapter count and numbering unchanged unless the feedback explicitly requires adding or removing chapters.
3. Return chapters unrelated to the feedback verbatim. Do not refactor them while you're at it.
4. Output strict JSON only. No extra prose.`,

	ForeshadowPlanning: `You are a senior narrative architect who specialises in foreshadow design. Design a foreshadow plan for the novel outline below.

[Title] {{.Title}}
[Core writing prompt] {{.CorePrompt}}
[Synopsis] {{.StorySynopsis}}

[Full outline]
{{.Outline}}

Design 3 to 8 foreshadows following these principles:
1. Each foreshadow should serve the main plot or character arc, not exist for mystery's sake.
2. Each foreshadow has a clear "plant point" (chapter it is seeded) and "payoff point" (chapter where it is expected to be resolved).
3. Foreshadows may interconnect into a web of clues.
4. Vary the types: objects, hinted dialogue, environmental detail, contradictions in behaviour, unexplained phenomena, etc.
5. Spread payoff points across different chapters; do not cluster them.
6. Foreshadows can begin as early as chapter 1, but most should be planted in the middle and paid off in the latter half.

Return JSON:
{
  "foreshadows": [
    {
      "name": "Short label (under 10 words)",
      "description": "Detailed description: how it is planted, what it hints at, what the 'oh-I-see' feeling should be when it pays off",
      "plant_chapter": chapter_number,
      "target_chapter": expected_payoff_chapter
    }
  ]
}

Output strict JSON only.`,

	ForeshadowUpdate: `You are a strict foreshadow tracker. Update the foreshadow system based on the just-completed chapter.

[Title] {{.Title}}

[Current foreshadows]
{{.Foreshadows}}

[Chapter info]
Chapter number: {{.ChapterNum}}
Chapter title: "{{.ChapterTitle}}"

[Chapter text]
{{.ChapterContent}}

[Story-so-far]
{{.HistorySummary}}

For each foreshadow, decide whether its state changed in this chapter:

1. First time it is hinted/planted in this chapter → status = "planted".
2. New clue or progress in this chapter → status = "progressing".
3. Fully revealed/resolved in this chapter → status = "resolved".
4. Not present in this chapter → keep the existing status.
5. Distinguish "true resolution" from "mere progress": only mark resolved when the mystery is fully unveiled.

Return JSON:
{
  "updates": [
    {
      "id": foreshadow_id,
      "status": "new state if changed",
      "event": "one-sentence description of what this chapter did with this foreshadow",
      "resolution": "how it was resolved, if status = resolved"
    }
  ]
}

Only return foreshadows whose state changed. Omit any foreshadow not touched in this chapter.
Output strict JSON only.`,

	ContentAnalysis: `You are a professional novel analysis editor. Analyse the existing novel text, extract story metadata, and produce per-chapter outline + summary entries.

Return JSON in this structure:
{
  "title": "Novel title",
  "story_type": "Genre (fantasy/urban/sci-fi/mystery, etc.)",
  "core_prompt": "Core writing prompt (system-level guideline for downstream chapters)",
  "story_synopsis": "Synopsis",
  "writing_style": "Writing-style description",
  "writing_pov": "Narrative POV (e.g. third-person limited, first-person heroine, alternating first-person leads)",
  "chapters": [
    {
      "num": 1,
      "title": "Chapter title",
      "outline": "Chapter outline (what happens, 100-200 words)",
      "summary": "Structured summary (for downstream story-so-far, under 200 words: core events, psychological arc, state changes, key details)"
    }
  ]
}

Requirements:
1. Detect chapter boundaries (common formats: "Chapter X", "# Chapter X", blank-line separators, etc.).
2. For each chapter produce: outline (what happens) and summary (structured story-so-far for downstream chapters).
3. summary should retain continuation-relevant state: core events, psychological arc, key details, emotional palette.
4. Extract story metadata: genre, writing style, narrative POV, character settings, worldview.
5. Generate core_prompt and story_synopsis to guide downstream writing.

[Existing novel text]
{{.ExistingContent}}

Output strict JSON only.`,

	ContinuationOutlineGeneration: `You are a professional novel-planning editor. Based on existing chapters' outlines and summaries, produce the outline for the next chapters.

[Title] {{.Title}}
[Story type] {{.StoryType}}
[Core writing prompt] {{.CorePrompt}}
[Synopsis] {{.StorySynopsis}}
[Writing style] {{.WritingStyle}}
[Narrative POV] {{.WritingPOV}}

[Existing chapters]
{{.ExistingOutline}}

Produce outlines for {{.NewChapterCount}} more chapters, starting at chapter {{.StartNum}}.

Return JSON:
{
  "chapters": [
    {"num": {{.StartNum}}, "title": "Chapter title", "outline": "Outline for this chapter"},
    ...
  ]
}

Rules:
1. The outlines must continue the existing storyline coherently.
2. Each outline should describe concrete plot beats, not vague summaries.
3. List the characters appearing in each chapter; mark new characters with "first appearance" in their debut chapter.
4. One-time events already used in prior chapters (first meeting, identity reveal, etc.) must not be re-scheduled.
5. Output strict JSON only.`,

	TransitionSmoothing: `You are a senior novel editor in charge of polishing chapter-to-chapter transitions. Below are the end of the previous chapter and the opening of the current chapter. Decide whether the opening naturally continues from the previous ending.

[Previous chapter ending]
{{.PrevTail}}

[Opening of current chapter (chapter {{.ChapterNum}} "{{.ChapterTitle}}")]
{{.Opening}}

[Chapter outline (for context only — do not expand it)]
{{.ChapterOutline}}

Rules (strict):
1. If the opening already continues naturally from the previous ending (scene transition, timeline, character state, emotional tone all coherent), output exactly the single word NO_CHANGE and nothing else.
2. If the transition is rough (abrupt scene jump, re-establishing what already happened, character-state break), rewrite the opening above so it seamlessly continues from the previous ending.
3. The rewrite is "minimal": keep every plot beat and piece of information in the opening, similar length to the original, only adjust the bridging beats, transitional sentences, and necessary detail.
4. Output only the rewritten opening prose — no title, explanation, prefix/suffix marker, or previous-chapter content. Do not continue past the opening.`,

	OutlineConsistencyCheck: `You are a strict novel-planning editor. Before drafting this chapter's prose, check whether this chapter's outline already conflicts with the actual prior storyline.

[Story-so-far (already happened, cannot be changed)]
{{.HistorySummary}}

{{.PreviousEnding}}[Outline under check]
Chapter {{.ChapterNum}} "{{.ChapterTitle}}": {{.ChapterOutline}}

Checklist (objective conflicts only):
1. Outline schedules a "first meeting" between characters who already know each other in prior text.
2. Outline assumes a precondition (character state, location, possessed item, knowledge) that contradicts prior text.
3. Outline schedules an event that has already happened in prior text.

Rules:
- If no conflict: conflict = false, revised_outline left empty.
- If there is a conflict: conflict = true, and provide a revised outline for this chapter that keeps its original plot goals, characters, and role in the overall arc — only the minimum changes needed to make it compatible with prior text (e.g. change "first meeting" to "reunion").
- Do not expand new plot. Do not change the chapter's length tier. When unsure, treat as no conflict.

Return JSON only (no other text):
{"conflict": false, "issues": [], "revised_outline": ""}
or
{"conflict": true, "issues": ["conflict description"], "revised_outline": "revised outline for this chapter"}`,

	ForeshadowOutlineConsistency: `You are a strict narrative-consistency editor. Check whether the foreshadow plan matches the full chapter outline.

[Title] {{.Title}}
[Full outline]
{{.Outline}}

[Foreshadow list]
{{.Foreshadows}}

[Summaries of confirmed chapters]
{{.AcceptedSummaries}}

Checklist (objective issues only):
1. Each active foreshadow (not resolved/abandoned) has reasonable planting space in its plant_chapter outline.
2. The target_chapter outline has plot space to pay off that foreshadow (logical fit, not word-for-word match).
3. Foreshadow description structurally contradicts the outline (cannot be achieved on this story path).
4. plant_chapter / target_chapter exceed the actual chapter count.
5. Confirmed-chapter summaries clearly contradict the foreshadow plan.

Return JSON only (no other text):
{
  "has_conflicts": false,
  "conflicts": [],
  "summary": "one-sentence summary"
}
or
{
  "has_conflicts": true,
  "conflicts": [
    {
      "foreshadow_id": 1,
      "foreshadow_name": "short name",
      "conflict_type": "missing_payoff|weak_payoff|missing_plant|structural|out_of_range",
      "description": "specific conflict",
      "suggested_fix": "revise_outline|adjust_foreshadow|abandon"
    }
  ],
  "summary": "one-sentence summary"
}

When unsure, treat as no conflict.`,

	WritingConflictAnalysis: `You are a senior novel editor. Fact-checking has failed repeatedly for this chapter. Diagnose the root cause and recommend next steps.

[Chapter]
Chapter {{.ChapterNum}} "{{.ChapterTitle}}"

[Chapter outline]
{{.ChapterOutline}}

[Story-so-far]
{{.HistorySummary}}

{{.OutlineConstraints}}{{.Foreshadows}}[Accumulated fact-check failures]
{{.FailedIssues}}

[Current draft excerpt (reference)]
{{.ContentExcerpt}}

Tasks:
1. Decide whether failures come from irreconcilable contradictions among outline, foreshadows, and prior story.
2. If reconcilable without changing outline/foreshadows: provide extra_constraints text to inject into the writing prompt so the next draft can pass fact-check.
3. If not reconcilable: explain why and whether the user should edit the outline or adjust foreshadows.

Return JSON only (no other text):
{
  "reconcilable": true,
  "summary": "one-sentence root cause",
  "root_cause": "foreshadow_outline|outline_history|foreshadow_history|mixed|other",
  "extra_constraints": "full constraint text (required when reconcilable is true)",
  "suggested_actions": [
    {"id": "edit_outline", "label": "Edit chapter outline", "description": "..."},
    {"id": "adjust_foreshadow", "label": "Adjust foreshadows", "description": "..."},
    {"id": "force_review", "label": "Keep draft for manual review", "description": "..."}
  ]
}

When reconcilable is false, leave extra_constraints empty. suggested_actions must include edit_outline, adjust_foreshadow, and force_review.`,

	SettingsReconciliation: `You are a professional novel-consistency editor. The user changed the story settings, but some chapters are already confirmed. Check whether the new settings are consistent with the existing chapters, and auto-adjust the settings to remain compatible.

[User's new settings]
Story type: {{.NewType}}
Writing style: {{.NewWritingStyle}}
Narrative POV: {{.NewWritingPOV}}
Synopsis: {{.NewStorySynopsis}}

[Summaries of existing confirmed chapters]
{{.ExistingSummaries}}

Return the adjusted settings as JSON:
{
  "type": "...",
  "writing_style": "...",
  "writing_pov": "...",
  "story_synopsis": "...",
  "explanation": "Describe what was adjusted and why"
}

Adjustment principles:
1. Existing chapters cannot be changed; the settings must be compatible with them.
2. Preserve the user's intent as much as possible.
3. Where conflicts are irreconcilable, prefer existing content and adjust the new settings minimally.
4. Non-conflicting parts keep the user's new settings.`,

	BookDiagnosis: `You are a senior editor-in-chief for serialised fiction, specialising in full-novel reviews after the manuscript is complete.

[Task]
Read the materials below and produce a "Full-Novel Optimisation Diagnostic Report". Only diagnose this round — do not rewrite prose.

{{.ModeNote}}

=== Settings and style ===
{{.SettingsText}}

=== Chapter summary index ===
{{.SummaryIndex}}

=== Full novel text ===
{{.FullText}}

[Output format (strict)]
## 1. Overall assessment (under 200 words)
## 2. Structure and pacing (point out dragging sections, peak sections, lull sections — anchor every issue to a chapter number)
## 3. Characterisation and dialogue (flat archetypes, inconsistent voice, completeness of protagonist arc)
## 4. Setting and logic faults (timeline, power level, geography, foreshadow misses or wrong payoffs)
## 5. Style and AI fingerprints (cliches, parallel-clause pile-ups, emotion labelling, overly written dialogue)
## 6. Prioritised fix list (P0/P1/P2; every entry must contain: chapter number, issue type, one-line description, suggested fix)
- P0 = logic/setting error that blocks reading
- P1 = style/pacing problem with clear quality impact
- P2 = polish

[Constraints]
- No vague generalities. Every issue must anchor to a specific chapter.
- Do not output rewritten prose.
- When unsure, mark "needs close re-read".`,

	BookConsistencyCheck: `You are a strict novel fact-checker. Check the entire novel for consistency with its settings.

{{.VolumeNote}}

=== Settings ===
{{.SettingsText}}

=== Chapter summary index (whole novel) ===
{{.SummaryIndex}}

=== Prose (this volume) ===
{{.FullText}}

[Audit dimensions]
1. Timeline contradictions (age, season, event order)
2. Character-setting contradictions (appearance, abilities, address, relationships)
3. Inconsistent geography / organisations / props
4. Foreshadows: planted-but-never-paid, wrong payoffs, one-time events re-enacted (e.g. a first meeting written twice)
5. Transition breaks between chapters (previous ending and current opening do not match)

[Output format]
Use a Markdown table:
| Severity | Chapter | Original excerpt (<= 30 words) | Contradiction description | Suggested fix (minimum change) |

Severity: critical / major / minor
Do not rewrite the prose, only describe the fix.`,

	BookRoadmap: `You are a senior novel editor. Based on the diagnostic and consistency reports below, produce an executable revision task list.

[Diagnostic report]
{{.DiagnosisReport}}

[Consistency report]
{{.ConsistencyReport}}

[Requirements]
1. Merge duplicates and sort by chapter number.
2. At most 3 revision items per chapter; anything beyond goes to round two.
3. type takes values: logic, transition, style, rhythm, dialogue, polish (AI-flavour removal).
4. priority takes values: P0 / P1 / P2.
5. feedback must be ready-to-use revision instructions (50 to 150 words) emphasising minimum changes.
6. **Merge all issues for the same chapter into ONE task** (at most one items entry per chapter).
7. Suggested execution order: transitions -> P0 logic -> style polish.

[Output format]
JSON only, nothing else:
{"items": [{"chapter_num": 1, "type": "logic", "priority": "P0", "feedback": "concrete revision instruction", "selected": true}]}`,
}
