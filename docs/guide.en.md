# User guide

[Back to README](../README.en.md) · [中文指南](guide.zh.md)

This guide follows the writing process. For your first project, complete a small batch manually before enabling auto-confirm or combining several skills.

## Contents

- [Installation and API setup](#setup)
- [Your first story](#first-story)
- [Batch planning and endings](#planning)
- [Review, editing, and revision](#revision)
- [Add story knowledge and correct errors](#knowledge)
- [Facts, settings, and foreshadowing](#consistency)
- [Import and continuation](#import)
- [Final proofreading](#completion)
- [Skills and assistant](#skills)
- [Saving, backups, and migration](#data)
- [Troubleshooting](#troubleshooting)

<a id="setup"></a>
## 1. Installation and API setup

### Launch and projects

Extract a release, start the executable, and open `http://localhost:48090`. Closing the browser does not stop a backend task. Stop the task and wait for saving before exiting the program.

The project name identifies its directory; the novel title identifies the work. They can differ. Choose Chinese or English when creating the project. Project language determines prompts, prose, and applicable skills and should not be changed after creation. You can switch the interface language independently.

Data defaults to the working directory at launch. To use a fixed location, pass an existing directory and verify the startup log. If the port is occupied, set `PORT`, for example in PowerShell:

```powershell
$env:PORT = "48091"
.\show-me-the-story.exe "D:\Novels"
```

Then open `http://localhost:48091`.

### Connect a model

Enter and save these fields in Configuration:

| Field | What to enter |
|---|---|
| API address | Your provider’s OpenAI-compatible Chat Completions address |
| Strict URL mode | Enable when the provider’s path must not have `/v1` inserted; `/chat/completions` is still appended |
| Model | The exact provider model identifier, not an arbitrary display name |
| API key | Credentials for that service; local services may or may not require one |
| Maximum output tokens | Enough for a chapter or outline, within the model’s supported output limit |
| Context budget | The actual model window; a larger number does not increase the model’s capacity |
| Timeout | Allow enough time for long chapter generation |

A bare domain normally receives `/v1/chat/completions`; an existing path generally receives `/chat/completions`. A complete endpoint can be used directly. Connection testing uses a short request. Passing it does not verify long-output limits, context capacity, or a suitable timeout. Save the configuration after testing.

API configuration is shared across projects. Changing it affects future calls in every project. Drafting, checks, summaries, revisions, planning, and assistant operations can incur charges. Generating one chapter usually involves several requests.

<a id="first-story"></a>
## 2. Your first story: recommended order

### Step 1: Establish essential settings

Save the genre, target chapter length, style, and point of view in Configuration. First decide who wants what, what blocks them, and which rules make the story distinctive.

Example:

> Fantasy mystery. Third-person limited, following only the heroine. She investigates her sister’s disappearance. City plants grow under moonlight and become dormant in sunlight. Keep dialogue concise and reveal clues through action.

Add the protagonist, key supporting characters, and essential worldview entries. You do not need every subplot before starting. Motivation and ability limits often help consistency more than lengthy appearance descriptions. You may ask AI to generate initial settings, but review the result.

### Step 2: Plan a small first batch

Start with 3–6 chapters in Outline. Explain the events that must happen, the amount of progress, and where the batch should stop; a genre label alone is insufficient.

Example:

> The heroine finds a moonlight seed left by her sister and searches an abandoned greenhouse for cultivation records. Establish the investigation and her partnership in the first three chapters. By chapter six, reveal that the records were altered. Do not reveal her sister’s actual location yet.

Long-term direction could say that the sister ultimately turns out to have joined the night watch. That does not require this batch to reach the revelation.

Review chapter goals, character introductions, clue order, and premature reveals. If character suggestions await approval, check them before adding them to your character list. When the page offers Confirm outline, confirm before writing. An outline does not generate prose automatically.

### Step 3: Generate and review one chapter

Generate the current chapter in Writing. Wait for prose, summary, and related checks to finish. Streaming text or an active task indicator means the operation is still underway.

Review in this order:

1. Did the chapter accomplish its outline goal without prematurely consuming future events?
2. Are character knowledge, abilities, and actions plausible within the story?
3. Did it violate a rule, misuse knowledge, or repeat earlier content?
4. Do dialogue, point of view, and style match your intent?

Revise problems first; accept the chapter when satisfied. Acceptance advances the writing position and makes the prose eligible for setting synchronization.

### Step 4: Continue and reassess

Repeat generation, review, and acceptance. After a small batch, plan the next one and use planning review when needed. Avoid filling a distant future with detailed outlines before testing the story’s direction.

Once results are stable, auto-confirm can continue writing to the end of the current plan. It skips manual acceptance; it does not certify the author’s approval. To correct errors, disable auto-confirm and wait for the active task or stop it before editing.

<a id="planning"></a>
## 3. Batch planning and endings

Each batch requires a chapter count (1–36) and synopsis. Long-term direction and ending purpose are additional inputs. The batch synopsis also informs prose generation for its chapters.

| Operation | When to use it | Effect |
|---|---|---|
| Append a batch | You need more chapters | Starts after the highest chapter number and preserves existing batches |
| Replan this batch | The final complete batch is entirely pending and has no prose | Replaces that batch after confirmation; count and synopsis may change |
| Edit or revise outlines | Events, pace, or details need adjustment | Does not automatically rewrite existing prose |
| Planning review | Some chapters are already accepted | Uses story context to assess future planning; inspect results and logs |
| Set ending purpose | This batch should bring the work toward closure | Guides planning and ending behavior but does not formally complete the book |

Do not delete outlines or chapters merely to request wording changes. Once a batch has begun writing, edit relevant outlines or revise prose instead of trying to replace the entire batch.

Ending purposes include ongoing serialization, completing the book, and closing this book while leaving sequel space. A closed ending should resolve major outcomes. An open ending should still address the present main plot while deliberately leaving questions open. Custom endings should identify mandatory resolutions and intentional loose ends.

After the planned ending, accept the chapters, check foreshadowing, and mark the work complete manually. Appending after a planned completion requires confirmation. Finishing a batch alone does not finish the book.

<a id="revision"></a>
## 4. Review, editing, and revision

Chapters generally move through pending → writing → review → accepted. Text visible after a failed generation may be a draft rather than a completed chapter.

### Choose the right operation

| Need | Recommended operation |
|---|---|
| A typo or exact replacement | Select the paragraph and edit manually |
| Improve a particular exchange or description | Paragraph AI revision with facts to preserve |
| Adjust a chapter’s wording or local events | Chapter revision with explicit scope and goals |
| Correct a story rule and retain it for future writing | Add story knowledge, then revise using it |
| Improve phrasing and voice | Enable an applicable polishing skill and use polishing |
| Clean up language after completion | Final proofreading |

An ordinary revision of the current review chapter may also revise subsequent outlines. Targeted revisions of other chapters focus on that chapter. The new revision using selected settings always targets the selected chapter, without automatically rewriting other outlines or prose.

Use specific feedback:

> Preserve the discovery of the key. Replace the guard’s voluntary confession with the heroine’s deduction from the duty roster. Keep third-person limited and introduce no new characters.

Select text and use the quote-to-feedback action to insert lines beginning with `> `. This prioritizes revision of the quoted paragraphs. If exact matching fails or the returned paragraph count does not match, the system may fall back to a whole-chapter revision. Avoid restricting feedback to quoted paragraphs when errors occur throughout the chapter.

Paragraph tools appear after selecting a paragraph. Changes affecting linked facts require confirmation. If the content version is stale, reload or reselect the chapter, read the new version, and submit again. Confirmation does not revise other chapters automatically.

<a id="knowledge"></a>
## 5. Add story knowledge and correct errors

### Where knowledge belongs

Knowledge is part of the novel’s settings and does not have to reflect reality. In Configuration → Worldview, create an entry with the Story knowledge category. Name and description are required; tags can include relevant terms and aliases.

Use one entry for a closely related group of rules. State the scope, limits, and exceptions. Example:

> Name: Moonlight coffee
>
> Description: In this novel, the Arabica tree is fictional. Its fruit ripens under moonlight. The seeds can be brewed directly after peeling and need no roasting. Sunlight makes the plant dormant but does not kill it. Ordinary coffee outside the city does not follow these rules.
>
> Tags: Arabica tree, moonlight coffee, seeds, greenhouse

“Something is wrong with the coffee” is not a reusable setting. Write the correct rule, then put chapter-specific directions in revision feedback.

### The shortest correction workflow

1. Disable auto-confirm, wait for the active task, and select the latest chapter containing the error.
2. Click Add story knowledge in the prose action area.
3. Enter a name, description, and optional tags.
4. Click Save and revise this chapter.
5. If linked facts exist, review the confirmation and authorize correction of facts conflicting with the selected settings.
6. Wait for completion, inspect related prose and the summary, and continue when satisfied.

Knowledge is saved before revision begins. A revision failure does not remove the saved entry. Select the existing entry to retry rather than creating duplicates.

### Add or edit knowledge in Configuration first

You can also create or edit the entry in Worldview, return to Writing, select the chapter, open Add story knowledge, select the saved entry, and click Revise using selected settings.

Feedback is optional. You can add a direction such as “Preserve the plot; correct only the plant’s growth process.”

Existing geography, history, or rule entries can also be selected. There is no need to duplicate them under Story knowledge. That category additionally indicates an author-maintained rule: proposed changes from prose synchronization require approval.

Save alone does not revise prose. Submit worldview to AI sends a message to the assistant; it is not an automatic chapter rewrite.

### Scope and limitations

- Selected entries enter this revision request in full, without requiring keyword matches or being clipped by the retrieval budget. If they exceed the model’s total window, select fewer entries or shorten them and retry.
- Later ordinary writing retrieves relevant entries rather than the entire library. Use names and tags that actually occur in the prose to improve retrieval.
- Entries belong to this project, not every project.
- Revision targets the currently selected chapter. Check earlier prose, conflicting settings, and future outlines separately.
- Summary, fact, and setting synchronization use the existing workflow after revision. Watch for pending synchronization and proposed setting changes. The model can still miss a correction.
- If entries conflict, clarify scope and exceptions or update the conflicting entry before requesting revision.

<a id="consistency"></a>
## 6. Facts, settings, and foreshadowing

| Information | Meaning | Author’s role |
|---|---|---|
| Worldview, characters, organizations, relations | World rules and entity information | Maintain the story’s reference material |
| Story knowledge | Explicit fictional or realistic rules | Explain or correct model understanding |
| Extracted prose facts | Events and details recognized in written chapters | Check continuity and sources, not real-world truth |
| Summary | Main progress of a chapter | Review quickly and check updates after revision |
| Foreshadowing | Clues planned for planting, advancement, and resolution | Monitor state and intended resolution chapter |

The facts/settings panel inside the prose card is collapsed by default; the summary remains visible. Expand fact markers to inspect and jump to source paragraphs. Changed sources need review. Extraction may miss facts, so an unmarked passage is not necessarily consequence-free.

Accepted prose can drive setting evolution. Conflicts with author settings or insufficient evidence produce proposals to accept or ignore. Changes to Story knowledge require approval even if the model calls them plot evolution.

If synchronization fails, inspect the log and retry through the knowledge panel. Saved prose and synchronized knowledge are separate outcomes; pending items can remain after a task ends.

Foreshadows can be suggested by AI or created manually. Check descriptions, planting and resolution chapters, and planted/progressing/resolved/abandoned states. Before completion, resolve or retire unused threads so they are not treated as future obligations.

Long-form context has a budget. Recent summaries, hierarchical distant summaries, relevant facts/settings, and limited future outlines provide context together. The model does not reread the complete novel for every request.

<a id="import"></a>
## 7. Import and continuation

### Continue existing text

1. Create an empty project in the source text’s language.
2. Choose Import existing content in Outline and paste the text.
3. Review the local chapter-splitting preview. If headings are recognized incorrectly, normalize them before previewing again.
4. Start import and wait for metadata and per-chapter analysis.
5. Review imported chapters, summaries, characters, and worldview. After interruption, resume analysis rather than importing the same text again.
6. Enter the next batch’s synopsis and count, plan it, and start writing.

Imported chapters serve as accepted history and do not need to be generated again. Text import is not a full project restore: conversations, skill choices, and every manually maintained setting cannot necessarily be reconstructed from prose.

### Continue a completed work

Before proofreading has modified prose, use the resume-serialization workflow when the page permits it. Once proofreading changes prose, the original project no longer returns to normal writing.

Instead, enter a new project name in the proofreading page and create a continuation project. It inherits frozen settings, facts, outlines, summaries, and foreshadow snapshots, not the proofread prose. Chapter numbering continues; progress counts only newly written chapters.

Inherited foreshadows are independent snapshots. Changes in the continuation do not update the predecessor. If proofreading changed major plot details or rules, check and supplement inherited settings and summaries. Do not assume proofread prose was re-extracted into writing knowledge.

<a id="completion"></a>
## 8. Completion, proofreading, and exports

1. Accept planned chapters, check the main outcome and foreshadows, and mark the work complete.
2. Open final proofreading, download the original text and all outlines, and acknowledge the backup.
3. Enter style preferences and run automatic proofreading if needed.
4. Generate the interactive report for logic, structure, character, and foreshadowing issues.
5. Click chapter/paragraph references to inspect the prose. Mark manually handled issues resolved, or ignored when you reject the suggestion.
6. Review before/after changes, undo proofreading by chapter if needed, and export the final text, outlines, and report.

Automatic proofreading targets spelling, grammar, repetition, dialogue, and voice. It does not restructure the plot. The report lists issues requiring author judgment; it serves a different purpose.

Proofreading processes chapters individually. Failed chapters can be retried; cancellation does not undo previously saved chapters. Per-chapter proofreading undo is not a general undo feature for every writing operation.

Text, outline, and report exports are readable deliverables, not full project backups.

<a id="skills"></a>
## 9. Skills and the assistant

### Skills

All skills are disabled by default. Establish a basic workflow before enabling a few skills matching the project language. Scope determines whether a skill applies to writing, polishing, outlining, or proofreading. Enabling one does not inject it into every request; logs show active skills.

Install by pasting Markdown, uploading Markdown or ZIP, or selecting a folder. User skills live in the shared `skills/<id>/` directory and are enabled separately for each project. A standard package contains `skill.json` and `SKILL.md`, optionally safe text resources. Scripts and binaries are not executed.

Custom skills can be checked by AI. Unchecked skills may be enabled; those judged to need optimization or to fail cannot be enabled. Optimization produces a copy and checks it again. Content changes invalidate previous checks.

Project-specific rules such as “magic consumes memories” belong in worldview knowledge. Reusable writing methods belong in skills. Avoid combining contradictory instructions.

### Assistant

Prefer page buttons for core workflows. The assistant is useful for queries, drafting multiple settings, explaining problems, and precise natural-language changes:

> Inspect chapter 4 and settings about moonlight plants. Explain conflicts without editing anything.

> Revise chapter 4 using the Moonlight coffee entry to fix harvesting. Preserve the investigation’s outcome and do not change other chapters.

> Suggest pacing changes for the final batch while keeping the chapter count.

Revision and deletion are different actions. For deletion, check the scope the assistant repeats. After an asynchronous operation begins, inspect task status and logs before requesting another write. Dedicated pages are clearer for proofreading, importing, and graph interaction.

<a id="data"></a>
## 10. Saving, backups, and migration

Common files:

```text
<data directory>/
├── api.json                 # Shared API configuration, potentially including keys
├── skills/                  # Installed user skills
└── storys/<project>/
    ├── config.json          # Language, story settings, prompts, skill selections
    ├── progress.json        # Outlines, state, summaries, facts, foreshadows
    ├── chapters/            # NNNNNN.json: authoritative prose
    ├── settings.json       # Entities, worldview, relations, provenance
    ├── sessions/            # Assistant conversations
    ├── import.json          # Import checkpoint, created when needed
    └── postprocess.json     # Proofreading state, created when needed
```

The program may also generate Markdown chapter copies and other auxiliary files. Editing those copies does not update prose in the application; use the page editor.

Prose and progress are saved together using the temporary `progress.json.rollback` journal. A failed write restores the previous files; after an abnormal exit, reopening the project first recovers any uncommitted save. Do not delete a retained journal or open the project with an older application. If recovery fails, close the program and copy the entire project before resolving permissions, disk space, or sync software problems and retrying. Missing or damaged chapters prevent opening and identify the chapter and path; restore a complete backup if repair is not possible. This is not arbitrary version history, and only one program may write a project at a time.

Editing an outline or confirming foreshadows starts a consistency-check task when foreshadows need checking. Other AI tasks and project switching remain unavailable during the check; the stop button cancels it. A failed check does not undo the saved outline or foreshadows. Use the consistency-check button to retry later.

For a reliable backup: stop AI tasks → wait for saving → close the program → copy the entire data directory to a dated backup location. On another computer, point the program at the copied directory. When backing up one project alone, also preserve required user skills separately. Do not publish API keys with a shared work.

Alternatively, use backup and restoration in the project list:

1. Wait for AI work to finish, return to the project list, and click **Back up ZIP** for the project. Save the downloaded file.
2. Under **Restore a project backup**, select the ZIP and enter an unused project name.
3. Click **Restore as new project**, then open it from the list and check prose, settings, and proofreading state. The original project remains unchanged.

ZIP includes project prose, progress, configuration, settings, conversations, import checkpoints, and proofreading state. It excludes the data directory's `api.json` and `skills/`. Project write requests are serialized during backup/restoration. Restoration validates archive paths, sizes, JSON, v4 format, and chapter integrity before publishing the new project; failures leave no partial project. Limits: ZIP 256 MiB, extracted total 512 MiB, each file 64 MiB, 20,000 entries. For larger projects, close the application and copy the directory. Text import is not a substitute for this full snapshot.

If long-term summary generation fails, a warning identifies temporary local summaries, which are retried on next use along with affected parent summaries. Cancellation does not commit new summary checkpoints. Task status is reconciled with the server on reconnect and while running, so a missed completion event does not leave the interface permanently busy.

This program only loads v4 projects. Older projects are labeled with the appropriate program version and are not migrated automatically. The format number is a storage protocol, not an upgrade switch.

<a id="troubleshooting"></a>
## 11. Troubleshooting

| Symptom | What to do |
|---|---|
| Generate or save is disabled | Check active tasks, outline confirmation, chapter state, and completion requirements |
| Progress appears missing after refresh | Wait for server restoration and verify the selected project and data directory before creating anything |
| API test fails | Check endpoint, strict mode, model identifier, key, and service status |
| 401/403/404 | Check credentials, permissions, and endpoint paths before retrying |
| Short test passes but a chapter fails | Check output limits, context window, timeout, and streaming compatibility |
| Context limit exceeded | Use the real window; reduce input or selected knowledge, or configure a model with sufficient capacity |
| Streaming stops early | Inspect logs; partial output is not a completed response. Check chapter state before retrying |
| Writing conflict appears | Read the reason and use the page to adjust outlines, retry, or keep the draft for review |
| Knowledge saved but prose unchanged | Saving is separate from revision; select the target chapter and saved entries, then revise |
| Knowledge error survives revision | Check clarity, contradictory entries, and quoted-paragraph scope; specify the remaining error |
| Content revision conflict / 409 | Reload, review current prose, and confirm again instead of reusing stale approval |
| Knowledge synchronization fails | Keep the prose, inspect errors, retry synchronization, and review setting proposals |
| Storage error appears | Resolve permissions, disk space, or file locks; task completion is not proof of successful saving |
| Cannot replace the final batch | The entire final batch must be pending with no prose; revise existing content instead |
| Cannot resume writing after proofreading | Create a continuation project and review inherited information |
| Older project will not open | Verify directory and format; use the corresponding release without changing format markers |

Logs describe substeps, not just generated prose. For a reproducible problem, record the operation, final error, model configuration with credentials removed, and steps to reproduce.

[Back to README](../README.en.md)
