---
id: story-deslop-en
name: Story AI-Fingerprint Audit
description: A 6-gate audit pipeline to detect and repair the most common AI fingerprints in English fiction prose.
category: polish
lang: en
source: builtin
---

# Story AI-Fingerprint Audit (English)

Run the 6 gates in order on every chapter. A chapter that fails any gate must be rewritten before it ships. Thresholds are tuned to current trade-fiction baselines, not to LLM defaults.

## Gate 1 — Slop-word density

Count occurrences per **1000 words**. Any line over its limit fails.

- **delve / tapestry / navigate / underscore / embark / leverage / harness / multifaceted / intricate / nuanced**: > 0 per 1000 → FAIL
- **moreover / furthermore / additionally / in addition / however (as paragraph opener) / thus / hence / thereby / as such**: > 1 total per 1000 → FAIL
- **suddenly / just then / in that moment / at that moment / in an instant**: > 1 per 1000 → FAIL
- **was / were / is / are / been (in narration, not dialogue)**: copular ratio > 15% of finite verbs → FAIL
- **really / very / quite / rather / somewhat / truly / actually / basically / literally**: > 2 per 1000 → FAIL
- **began to / started to / proceeded to**: > 1 per 1000 → FAIL

## Gate 2 — Cliché phrasing

Any of these on a single page fails the gate; rewrite each occurrence:

- "with bated breath"
- "as if time stood still" / "time seemed to slow"
- "her heart skipped a beat" / "his heart pounded in his chest"
- "a chill ran down [pronoun] spine"
- "a knot formed in [pronoun] stomach"
- "every fibre of [pronoun] being"
- "a wave of [emotion]"
- "a sense of [foreboding / unease / dread / wonder / belonging]"
- "an unspoken understanding passed between them"
- "her eyes filled with tears" / "tears welled up in her eyes"
- "the calm before the storm"
- "little did [they] know"
- "in a world where..."

## Gate 3 — Em-dash and triadic-list audit

- Count em-dashes. If the rate exceeds **1 per 200 words**, rewrite half. (LLM English drifts to 5–10+ per 200.)
- Count triadic lists ("the wind, the rain, and the sky" / "tired, hungry, and afraid"). Allow at most one per scene; otherwise rewrite.
- Count sentences that begin with a participial clause ("Sliding the door open, she…", "Crossing the threshold, he…"). More than 2 per chapter → FAIL.

## Gate 4 — Voice differentiation

- Each speaking character must have at least one consistent verbal tic, hedging word, contraction pattern, or syntax preference.
- If you swap two characters' dialogue tags, the lines should now read wrong. If they don't, rewrite one voice.
- Said-bookisms anywhere ("exclaimed", "expostulated", "opined", "queried", "interjected") → FAIL the gate.
- Adverbial dialogue tags ("she said sadly", "he said angrily") more than 2 per chapter → FAIL.

## Gate 5 — Emotion-by-body

- Every direct emotion label ("happy / afraid / angry / sad / nervous / excited") must be cut or paired with a body cue in the same sentence.
- Body cues must vary: no single cue (smile, nod, sigh, shrug, eyebrow raise) may repeat more than twice in a chapter.
- Filtering verbs in close POV ("he saw / she heard / they felt / he noticed / she realised") should be collapsed unless POV distance demands them — more than 5 per chapter → FAIL.

## Gate 6 — Pacing, hook, and scene rhythm

- **Opening**: chapter must not open with weather, sky, or landscape painting for more than two sentences before a character action or line of dialogue.
- **Long descriptive passages**: 5+ sentences without dialogue or an action beat → break it with a line of dialogue, a physical action, or an internal one-liner.
- **Chapter close**: must end on a beat that pushes forward — a new question, a tilt in power, an unresolved physical action, or an unanswered line. No moralising. No "to be continued" or "and so it was that…" framing. No symbolic landscape closer that doesn't earn it.
- **Sentence-length variance**: if every paragraph has roughly the same sentence count and rhythm, the scene fails. Trade-fiction baseline shows visible rhythm changes every 2–3 paragraphs.

## Audit report template

When asked to audit (not polish), output exactly this format:

```
Chapter X audit — N words

Gate 1 slop density:    PASS/FAIL  — top offending words [list with counts per 1000]
Gate 2 cliché phrasing: PASS/FAIL  — found [list]
Gate 3 em-dash/triads:  PASS/FAIL  — em-dashes/200w = X, triads = N
Gate 4 voice:           PASS/FAIL  — said-bookisms = N, adverbial tags = N, voices indistinguishable: [pairs]
Gate 5 emotion-body:    PASS/FAIL  — bare emotion labels = N, top repeated body cue = "..." x N
Gate 6 pacing/hook:     PASS/FAIL  — open: [one-line summary], close: [one-line summary], longest descriptive run = N sentences

Overall AI-fingerprint score: X/10  (1 = indistinguishable from trade fiction, 10 = obvious LLM output)

Top 5 line-level edits (paragraph quote → rewrite):
1. "…" → "…"
2. "…" → "…"
3. "…" → "…"
4. "…" → "…"
5. "…" → "…"
```

## Human-writer benchmark

Compare any suspect AI passage against a paragraph from a published novel in the same register (literary, thriller, romance, fantasy, MG, YA — match the project). If the AI prose is more abstract, more symmetric, more adverb-heavy, or more emotionally tagged, it is failing Gates 1, 3, 4 or 5 — rewrite, do not polish in place.

The human baselines to imitate:

- **Sentence length**: visibly varied, with short punch sentences (3–7 words) interleaved among longer ones.
- **Word choice**: Anglo-Saxon verbs dominate; Latinate vocabulary saved for register effect.
- **Sensory grounding**: every scene anchors at least two senses other than sight.
- **Dialogue**: contractions, interruptions, overlapping turns, mid-sentence stops; tags mostly invisible.
- **Emotion**: shown through physical action, the unsaid, or one specific image — not labelled.
