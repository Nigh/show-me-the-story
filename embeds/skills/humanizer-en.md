---
id: humanizer-en
name: English De-AI Polish
description: Strip the typical AI fingerprints from generated English prose and replace them with natural writing.
category: polish
lang: en
source: builtin
---

# English De-AI Polish Rules

## 1. Forbidden patterns (rewrite every occurrence)

These are the dead giveaways of ChatGPT / Claude / Gemini English fiction. Every instance must go.

1. **The "delve / tapestry / navigate / embark / underscore" cluster** — these verbs almost never appear in published trade fiction. Replace with plain choices ("look at", "world", "find a way", "set out", "show").
2. **"It's important to note that..." / "It's worth mentioning..."** — non-fiction register leaking into fiction. Cut entirely.
3. **"In a world where..." openings** — back-cover-blurb language. Drop and start with a concrete scene.
4. **"As such / thereby / henceforth / heretofore / ergo"** — pseudo-formal connectives. Use a full stop or "so".
5. **"Furthermore / Moreover / Additionally / In addition / In conclusion"** — essay transitions. Cut or replace with a beat.
6. **"With bated breath" / "as if time stood still" / "the calm before the storm" / "a chill ran down [their] spine" / "her heart skipped a beat" / "every fibre of [their] being"** — top-tier clichés. Rewrite with one concrete body cue.
7. **"A sense of [foreboding / unease / dread / wonder / belonging]"** — vague abstract noun. Replace with the specific sensation ("the room smelled wrong").
8. **"A mix of [emotion] and [emotion]"** / "a complex mix of emotions" / "a whirlwind of emotions" — label removal. Show one cue and trust the reader.
9. **Triadic parallelism on every other paragraph** — "the wind, the rain, and the sky"; "she was tired, hungry, and afraid". One triad per chapter, not per paragraph.
10. **Symbolic last sentence in every scene** — landing on light, silence, distance, fate, or "destiny". Allow at most one per chapter, and only if earned.
11. **Em-dash overuse** — the most reliable LLM tell. Halve them. Replace with a comma, full stop, or paragraph break.
12. **"It was as if..." analogies** — replace with a direct sensory beat, not a comparison.
13. **Adverb crutches in narration** — "obviously / clearly / certainly / truly / really / quite / rather / somewhat / arguably / undoubtedly". Delete on sight.
14. **Said-bookisms** — "exclaimed / expostulated / opined / queried / interjected / vocalised / articulated". Use "said", or replace the tag with an action beat.
15. **Adverbial dialogue tags** — "she said sadly / he said angrily". Show the sadness in the body or the line; use "said".
16. **Filtering through perception verbs** — "he saw the door open" → "the door opened"; "she felt the cold" → "the cold bit through her sleeve".
17. **Stage-direction summary** — "Then they laughed and continued their journey." Render the moment in beats; do not narrate forward.
18. **Mirror passages** — characters describing themselves to themselves in a window/blade/pool reflection. Cut unless plot-critical.
19. **Restating the obvious in dialogue** — "We need to get to the castle, Father, as you said earlier." Cut.
20. **Curtain-call epilogues** — "And so it was that...", "Little did they know...", "Thus ended...". Remove ceremonial framing.
21. **"Of course! Here is the chapter..." / "Certainly! Below is..." preface** — strip any meta acknowledgement, apology, or "let me know if you'd like changes" coda.
22. **Sentence-initial "Indeed" / "Truly" / "Verily"** — cut.
23. **Generic body language tics** — "a knot in her stomach", "a shiver down his spine", "butterflies", "goosebumps", "blood ran cold". Use specific, situation-bound detail.
24. **Smile/nod/sigh tic** — tally per scene; if any of these appears more than twice, swap variations or cut.
25. **Telegraphic narrator synonyms** — "the protagonist / the man / the figure / the boy / the girl / the woman / the stranger" used to dodge repetition. Use the name or "he/she/they". Vary only when POV or knowledge state demands it.
26. **Compound noun clichés** — "a sense of foreboding", "the weight of expectation", "the gravity of the situation", "the air was thick with tension". Replace with a sensory image.
27. **Run-on lyric lists** — "of dust, of memory, of half-forgotten promises". Pick one.
28. **"In that moment / At that moment / In an instant / Just then / Suddenly"** — beat-padding. Remove; let action carry the timing.
29. **"Could not help but..."** — verbose negation. Use the verb directly ("she smiled" not "she could not help but smile").
30. **"Began to..." / "Started to..."** — replace with the simple past unless the start matters ("she began to run" → "she ran").

## 2. High-frequency phrase swap table

These are the phrases that score highest on AI-fiction classifiers right now. Replace on contact.

| AI default | Better |
|---|---|
| a sense of unease / dread / foreboding | the room felt off / she didn't like the door / something was wrong |
| a knot formed in her stomach | her stomach tightened (or a specific image) |
| in that moment | (cut) |
| with bated breath | she didn't breathe |
| as if time stood still | (cut) |
| her heart skipped a beat | (use a body cue) |
| a chill ran down her spine | (use a body cue) |
| eyes filled with [tears / fear / wonder] | (cut, use action) |
| a hint of a smile | (just write the smile) |
| in the depths of [his eyes / the forest] | inside / deep in |
| a wave of [relief / sadness / nausea] | (use a specific cue) |
| an air of mystery | (show it) |
| a kaleidoscope / whirlwind of emotions | (pick one emotion, one cue) |
| the weight of the world (on his shoulders) | (specific worry) |
| pierced the silence | broke the quiet |
| crystal clear | clear |
| razor sharp | sharp |
| stark contrast | the contrast |
| breathtaking view | (describe what is breathtaking) |
| ethereal beauty | (cut "ethereal", describe) |
| time seemed to slow | (cut, use beats) |
| the world around [her] faded | (cut) |
| every fibre of his being | (just the feeling) |
| an unspoken understanding passed between them | (one beat; cut the sentence) |
| meticulously / painstakingly / intricately | carefully (or cut) |
| a tapestry of [anything] | (cut, name the thing) |
| navigated the [situation / conversation] | (use a concrete verb) |
| delved into | looked into / went into |
| embarked on a journey | set out / left |
| underscored / underlined the importance | mattered / showed |
| in a world where | (cut, open on a scene) |
| little did [they] know | (cut) |
| moreover / furthermore / additionally | (cut or use a comma) |
| as such / thereby / hence | so |
| however | but (in fiction prose) |
| utilise | use |
| commence | start / begin |
| ascertain | find out |
| endeavour | try |
| obtain | get |
| reside | live |
| inquire | ask |

## 3. Style rules

- **Contractions**: use them in dialogue ("don't", "can't", "I'm"). Use them in close-POV narration. Drop only for distant or archaic voice.
- **Anglo-Saxon over Latinate**: "use" > "utilise"; "show" > "demonstrate"; "buy" > "purchase"; "start" > "commence"; "end" > "conclude"; "help" > "facilitate".
- **Concrete nouns beat abstract nouns**: "fear" → "the cold lock on the back door". "Beauty" → "the way her hair caught the kitchen light".
- **Vary sentence length**: short, short, longer with embedded clause; then a beat. Aim for visible rhythm changes every paragraph.
- **One image per paragraph**. Cut anything that doesn't earn its place.
- **Cut every adverb in dialogue tags.** Replace with action or a stronger verb in the line itself.
- **No filtering in close POV**: collapse "he saw / she heard / they felt" most of the time.
- **Concrete sensory grounding**: every scene needs at least two of sound, smell, texture, weight, temperature. Not just visual.
- **Dialogue economy**: cut greetings, acknowledgements, and "as you know" exchanges unless silence would be wrong.

## 4. Format rules

- **Quote marks**: use straight or smart quotes consistently. American: double quotes for speech (`"Hello," she said.`). British: typically single quotes (`'Hello,' she said.`). Pick one and hold it across the project.
- **Comma inside quotes (US) vs outside (UK)**: follow the project register; don't mix.
- **Em-dash** without surrounding spaces in US (`—`); with thin spaces in UK is acceptable. Don't render as `--` or `- -`.
- **Ellipsis**: three dots `...` or the Unicode `…`. Never four. Used sparingly; not as a generic pause marker.
- **Dialogue tags**: comma before the closing quote, lowercase tag: `"I know," she said.` Not `"I know." She said.` unless it's a separate sentence with an action beat.
- **One space after a full stop**, not two.
- **Paragraph breaks on speaker change**: every new speaker gets a new paragraph.
- **No bullet lists, headings, or markdown formatting in fiction prose**. Polish never introduces them.
- **Output only the revised chapter prose.** No commentary, no diff markers, no "Here is the polished version" preface, no concluding remarks.
