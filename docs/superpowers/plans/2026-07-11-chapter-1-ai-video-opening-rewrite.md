# Chapter 1 AI-Video Opening Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a review-ready new draft of Chapter 1, “被替换的人”, that opens with AI-driven redundancy, dramatizes one live misjudgment affecting ordinary workers, introduces the short-video breathing guidance without overt cultivation, and ends with Lin Yan actively pursuing paid work.

**Architecture:** Keep all work in the existing isolated draft directory. First lock continuity and chapter-boundary contracts, then rewrite Chapter 1 from scratch, then pass it through an independent semantic review and a fresh mechanical verification. The system chapter and `progress.json` remain byte-identical to their backups until the user explicitly approves a later synchronization task.

**Tech Stack:** Markdown prose, JSON outline data, POSIX shell, `rg`, `jq`, Perl sentence scanning, SHA-256 checks, independent literary review.

## Global Constraints

- The chapter title is exactly `第 1 章 被替换的人`.
- The first prose sentence is exactly `林砚被裁的理由，是他亲手教会的那套工具写的。`.
- The main dramatic location is one concentrated layoff event, not a full-day itinerary.
- AI has no subjective malice and provides no early evidence of a platform conspiracy.
- The live incident is a false batch fraud judgment caused by station data being uploaded together after a network outage.
- Lin Yan pauses the freeze to protect workers’ same-day income, but does not accept personal liability or repair the model for free.
- Bai Yan enters as the station lead with a concrete demand and later offers paid appeal-material work.
- Hospital and rent pressure appear only as short messages and together occupy no more than one tenth of the chapter.
- The short video contains no `修仙`, `灵气`, `炼气`, `吐纳`, `功法`, realm names, mysterious account behavior, or personalized message.
- The first bodily response is weak: shaking and cold sweat ease slightly; hunger, fatigue, and dizziness remain.
- No wound healing, warm current, emitted light, reversed steam, visible energy, interface, evidence number, or platform investigation appears.
- Lin Yan does not identify the experience as cultivation and does not investigate the video.
- The last action is Lin Yan replying `把资料和价格发我。` to Bai Yan’s paid-work offer.
- Expected length is 6000–9000 Chinese characters, but scene completion governs and there is no minimum word-count gate.
- Subsequent ordinary chapters use 4000–8000 Chinese characters as a working range; major conflicts and volume endings may exceed 10000 naturally.
- Use the “锋利通俗型” style: short action/dialogue-driven paragraphs, limited black humor, no slogan, no author summary, no process padding.
- Do not modify `/Users/caoye/storys/被逼急了，只好修仙/Chapter_01.md` or `/Users/caoye/storys/被逼急了，只好修仙/progress.json`.
- Do not call `confirm_outline` or import any chapter into the running application.

---

### Task 1: Lock the New Continuity Contract

**Files:**
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01-before-ai-video-opening.md`
- Modify: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/continuity.md`
- Modify: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/outlines.json`
- Create: `/Users/caoye/Documents/xs/show-me-the-story/.superpowers/sdd/chapter-1-ai-video-task-1-report.md`

**Interfaces:**
- Consumes: `docs/superpowers/specs/2026-07-11-chapter-1-ai-video-opening-design.md`, the current Chapter 1 draft, and existing Chapters 2–5 continuity boundaries.
- Produces: a frozen pre-rewrite snapshot plus explicit Chapter 1–4 boundary rules consumed by the prose writer.

- [ ] **Step 1: Snapshot the current draft and record baseline hashes**

Run:

```bash
set -e
root='/Users/caoye/storys/被逼急了，只好修仙'
draft="$root/drafts/rewrite-opening-20260710/Chapter_01.md"
snapshot="$root/drafts/rewrite-opening-20260710/Chapter_01-before-ai-video-opening.md"
test ! -e "$snapshot"
cp "$draft" "$snapshot"
cmp -s "$draft" "$snapshot"
shasum -a 256 "$snapshot" "$root/Chapter_01.md" "$root/progress.json"
wc -m -l "$snapshot"
```

Expected: the snapshot is byte-identical to the current 9,129-character draft; current system hashes are recorded in the Task 1 report.

- [ ] **Step 2: Replace the Chapter 1 outline and repair Chapters 2–5 bridges**

Update the Chapter 1 object in `outlines.json` to carry this exact event order:

```text
标题：被替换的人
1. 林砚在裁员谈话中看见 AI 岗位评估，报告认定其工作 83% 可自动完成，附件却是他半年提交的纠错记录。
2. 周启明要求他签署款项结清文件，谈话被站点批量冻结告警打断。
3. 白雁来电说明同站点几十名骑手当天收入即将被冻结，要求先保住钱再调查。
4. 林砚从集中时间戳和定位聚集看出网络中断后的补传问题，以应急权限暂停冻结。
5. 周启明要求林砚用个人账号确认全部订单正常并补齐模型规则；林砚拒绝个人背责，只提交复核方向。
6. 林砚拒签虚假结清文件并离开公司，没有拿到确定到账日期。
7. 医院补缴和朱阿姨催租两条消息落下，林砚开始搜索日结工作。
8. 他因手抖误停在普通呼吸放松视频上，无意识跟做一次，只感到手抖和冷汗略缓。
9. 白雁提出按件付费整理旧账申诉材料，林砚回复“把资料和价格发我。”
```

Do not add a hospital visit, rent negotiation, visible supernatural event, video investigation, or platform-conspiracy clue.

Make these exact bridge repairs in the remaining objects:

```text
Chapter 2: change “不重写林砚第一次到院” to the first actual hospital arrival. The chapter discovers Qin Sumei’s hidden examination and cost pressure. Bai Yan’s appeal work may remain pending, but no payment is already earned.
Chapter 3: remove the first appearance of the breathing video. After the wage-gap confrontation, Bai Yan sends the first bounded batch of appeal materials; Lin Yan sees repeated judgment patterns but does not yet possess cultivation perception.
Chapter 4: make Apartment 202 the first face-to-face rent negotiation. Afterward Lin Yan deliberately replays the already-saved breathing video for the first time; the attempt remains short and brings discomfort if he forces the rhythm.
Chapter 5: preserve one cautious offline attempt after food and rest. Use “呼吸练习” rather than establishing “吐纳” as a confirmed method, and do not search for cultivation.
```

No Chapter 2–5 object may say the video first appears in Chapter 3, the first practice occurs in Chapter 4 without acknowledging Chapter 1’s accidental attempt, the first hospital arrival happened in Chapter 1, or a prior face-to-face rent negotiation happened in Chapter 1.

- [ ] **Step 3: Replace the conflicting continuity rules for Chapters 1–5**

Update `使用边界`, `压力与欠款状态`, `信息权限与章节交接`, and `异常可见度` so they express these binding boundaries in the document’s existing Chinese style:

```text
篇幅：第一章预计 6000 至 9000 字，后续普通章节以 4000 至 8000 字为主；均不设最低值，场景完成即停。
第 1 章：只出现医院与房租消息，不到医院，不与朱阿姨当面谈租。白雁提供申诉材料工作线索，但尚未结算收入。
第 2 章：林砚第一次真正到院，发现秦素梅隐瞒检查与费用压力；不得写成再次到院。
第 3 章：工资缺口交锋后接收白雁第一批有限申诉材料；不得再次把呼吸视频写成第一次出现。
第 4 章：在 202 完成第一次当面房租谈判；随后才第一次主动重播第一章已收藏的视频，不得声称此前已有房租宽限谈判。
第 5 章：吃饭、休息后只进行一次谨慎的离线呼吸尝试；来源仍不明，不确认方法，不搜索修仙。
视频连续性：第一章只完成一次无意识跟练和极弱缓解；第三章不重复发现，第四章才第一次主动重播，第五章才进行离线对照。
搞钱连续性：第一章只有白雁的报价邀请；第二章不得把申诉钱写成已经到账。
```

Delete or rewrite every old sentence asserting that Chapter 1 spans two days, contains the first hospital arrival, contains the first rent negotiation, or has no abnormal response. Delete or rewrite every old sentence asserting that Chapter 3 first shows the video or Chapter 4 contains the first practice.

- [ ] **Step 4: Validate the JSON and boundary text**

Run:

```bash
set -e
root='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710'
jq empty "$root/outlines.json"
for p in '被替换的人' '白雁' '集中.*补传' '把资料和价格发我' '第 2 章.*第一次真正到院' '第 4 章.*第一次当面房租谈判' '第一章只完成一次无意识跟练'; do
  rg -q "$p" "$root/outlines.json" "$root/continuity.md"
done
if rg -n '第 1 章跨两天|第 1 章.*首次到院|第 1 章.*第一次房租谈判|第 1 至 3 章没有超常现象|第 3 章末.*呼吸视频进入|第 4 章首次练习|承接已发生的首次到院|不重演第一次房租|不重写林砚第一次到院' "$root/continuity.md" "$root/outlines.json"; then exit 1; fi
echo 'continuity_contract=PASS'
```

Expected: `jq` exits 0, every required boundary is found, and forbidden Chapter 1 boundary claims have no matches.

- [ ] **Step 5: Write the Task 1 report**

Record the snapshot hash, system chapter hash, `progress.json` hash, changed outline-object indexes 1–5, JSON validation output, and the exact Chapter 1–5 boundary text in `chapter-1-ai-video-task-1-report.md`.

Expected: a fresh reviewer can verify continuity without reconstructing prior conversation history.

### Task 2: Rewrite Chapter 1 from Scratch

**Files:**
- Modify: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md`
- Create: `/Users/caoye/Documents/xs/show-me-the-story/.superpowers/sdd/chapter-1-ai-video-task-2-report.md`

**Interfaces:**
- Consumes: the updated `continuity.md`, Chapter 1 outline object, frozen snapshot, and confirmed design spec.
- Produces: a complete standalone Chapter 1 draft ready for independent literary review.

- [ ] **Step 1: Write the opening layoff exchange**

Start with exactly:

```text
# 第 1 章 被替换的人

林砚被裁的理由，是他亲手教会的那套工具写的。
```

Within the first page, show the 83% automation assessment and reveal that its supporting material is Lin Yan’s own correction history. Keep Zhou Qiming tired, defensive, and self-protective rather than cruel. Do not include a normal morning, dashboard walkthrough, commute, or retrospective desk history.

- [ ] **Step 2: Dramatize the false batch freeze inside the meeting**

Use one incoming alert and one call from Bai Yan. Establish all of the following through dialogue and action:

```text
- Dozens of workers from the same station are classified as coordinated fraud.
- Their same-day earnings are about to be frozen.
- Bai Yan accepts investigation but refuses punishment before verification.
- Lin Yan notices the shared timestamps and locations came from queued uploads after a station network outage.
- Zhou Qiming asks Lin Yan to click a personal “normal” confirmation because the system cannot carry responsibility.
```

Do not explain the model architecture, enumerate fields, or turn the scene into a technical incident report.

- [ ] **Step 3: Give Lin Yan a bounded, active solution**

Lin Yan must pause the batch action with existing emergency authority, state the two records that need rechecking, and refuse both permanent personal confirmation and unpaid model-rule repair. His choice must protect workers before protecting his severance claim.

The scene outcome is partial: the money is not taken, but the case is not fully cleared. Zhou Qiming still proceeds with the layoff and the company does not admit wrongdoing.

- [ ] **Step 4: Compress the exit and money pressure**

Resolve the layoff with one refusal to sign the false “all payments settled” statement. Remove equipment checklists, multi-person handover, account-export procedure, commute, and repeated balance calculations.

After Lin Yan leaves, include only:

```text
- one hospital-account message concerning Qin Sumei;
- one rent-deadline message from Zhu Auntie;
- one immediate decision to search for same-day paid work.
```

The two pressures together must remain under one tenth of the final chapter by manual review.

- [ ] **Step 5: Introduce the breathing video without announcing cultivation**

While Lin Yan searches a short-video feed for day work, let shaking cause an accidental pause on a generic breathing-relaxation clip. The clip may direct posture, shoulder relaxation, and breath length, but it must not contain a face addressing him, occult vocabulary, a mysterious account, or a personalized instruction.

After one attempt, show only weak relief: typing becomes steadier and cold sweat eases, while hunger, fatigue, and standing dizziness remain. Lin Yan attributes the change to sitting and slowing his breathing.

- [ ] **Step 6: End on paid work, not reflection**

Bai Yan sends a concrete offer to organize old appeal materials for pay, with work volume and price still open. End with Lin Yan’s active reply:

```text
“把资料和价格发我。”
```

Do not add a concluding aphorism, author summary, proof that the video vanished, or a second mysterious video.

- [ ] **Step 7: Run the mechanical chapter checks**

Run:

```bash
set -e
f='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md'
root='/Users/caoye/storys/被逼急了，只好修仙'

test "$(sed -n '1p' "$f")" = '# 第 1 章 被替换的人'
test -z "$(sed -n '2p' "$f")"
test "$(sed -n '3p' "$f")" = '林砚被裁的理由，是他亲手教会的那套工具写的。'

for p in '百分之八十三|83%' '周启明' '白雁' '网络.*中断|中断.*网络' '补传' '暂停.*冻结|冻结.*暂停' '秦素梅' '朱阿姨' '把资料和价格发我'; do
  rg -q "$p" "$f"
done

test "$(rg -o '这份我不签|我不签' "$f" | wc -l | tr -d ' ')" -eq 1

if tail -n +2 "$f" | rg -n 'EV-|证据编号|本章|作者|读者|修仙|炼气|灵气|吐纳|功法|境界|天道|闻雪楼|流量池|平台阴谋|神秘账号|伤口.*愈合|暖流|发光|蒸汽.*逆'; then exit 1; fi

if rg -n '设备交还|工牌和钥匙|考勤申请|报销申请状态|公交|逐站|九点零七|九点十二|九点二十八|九点三十二|九点四十七' "$f"; then exit 1; fi

test -z "$(perl -CSD -Mutf8 -ne 'while(/([^。！？\n]{12,}[。！？])/g){$c{$1}++} END{for $s (sort {$c{$b}<=>$c{$a}} keys %c){print "$c{$s}\t$s\n" if $c{$s}>1}}' "$f")"

cmp -s "$root/Chapter_01.md" "$root/backup-opening-1-5-before-grounded-rewrite-20260710/Chapter_01.md"
cmp -s "$root/progress.json" "$root/backup-opening-1-5-before-grounded-rewrite-20260710/progress.json"

wc -m -l "$f"
shasum -a 256 "$f"
echo 'chapter_mechanical_checks=PASS'
```

Expected: all assertions pass, the only output is length/hash plus `chapter_mechanical_checks=PASS`; length is reported rather than used as a hard gate.

- [ ] **Step 8: Perform the writer’s semantic self-review**

Read the chapter once without editing and answer these exact questions in the Task 2 report:

```text
1. Does the first page contain a conflict, an irony, and an immediate decision pressure?
2. Does the live incident change the layoff conversation rather than merely interrupt it?
3. Does Lin Yan protect workers without looking omniscient or saintly?
4. Can a non-technical reader understand the false judgment without learning system internals?
5. Are hospital and rent only motives for money, not repeated scenes?
6. Could the video still be mistaken for ordinary relaxation content?
7. Is the bodily response weaker than hunger and exhaustion?
8. Does the final paid-work reply create a stronger next-chapter question than a reflective ending?
```

If any answer is `no`, revise the named scene and rerun Step 7 before reporting completion.

### Task 3: Independent Literary Review and Fix Loop

**Files:**
- Create: `/Users/caoye/Documents/xs/show-me-the-story/.superpowers/sdd/chapter-1-ai-video-review-package.md`
- Create: `/Users/caoye/Documents/xs/show-me-the-story/.superpowers/sdd/chapter-1-ai-video-review.md`
- Modify if required: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md`
- Append if required: `/Users/caoye/Documents/xs/show-me-the-story/.superpowers/sdd/chapter-1-ai-video-task-2-report.md`

**Interfaces:**
- Consumes: the complete draft, design spec, Task 1 continuity report, Task 2 writer report, and frozen pre-rewrite snapshot.
- Produces: an `Approved` semantic review or a fixed and re-reviewed draft with no Critical or Important findings.

- [ ] **Step 1: Build a reviewer package**

The package must contain paths and SHA-256 values for all consumed files, the full new Chapter 1, and a unified diff against `Chapter_01-before-ai-video-opening.md`. It must explicitly state that the reviewer does not edit files.

Expected package headings:

```text
## Binding Spec
## Continuity Contract
## Writer Report
## Baseline and Current Hashes
## Unified Diff
## Full Current Draft
```

- [ ] **Step 2: Dispatch an independent semantic reviewer**

Require this review rubric:

```text
- Does the chapter sell this specific novel within the first page?
- Is AI replacement dramatized through conflict rather than explained as an essay?
- Is the batch-freeze incident understandable, plausible, and consequential?
- Does Bai Yan sound like a person with leverage and responsibilities, not a victim-shaped plot device?
- Does Lin Yan make bounded choices without becoming omniscient, heroic, or rhetorically perfect?
- Does Zhou Qiming retain a self-protective human motive?
- Are hospital and rent compressed enough to avoid repeating the old draft?
- Is the breathing video ordinary on first reading and meaningful only in retrospect?
- Is the bodily response genuinely weak?
- Is the prose sharp and popular rather than flat, procedural, literary-summary-heavy, or slogan-driven?
- Does the ending create demand for Chapter 2?
- Are Chapters 2 and 4 still left with their first hospital and rent scenes?
```

Output format:

```text
1. Spec Compliance
2. Strengths
3. Issues: Critical / Important / Minor, each with current draft line references
4. Assessment: Approved or Needs fixes
```

- [ ] **Step 3: Fix all Critical and Important findings**

Return the complete finding list to the original prose writer. Restrict changes to the named paragraphs and require the writer to append a `Fix Round N` section containing changed lines, rationale, fresh hash, and the complete Step 7 command output.

Do not fix a reviewer finding by expanding process, adding another pressure scene, strengthening the supernatural response, or exposing platform intent.

- [ ] **Step 4: Re-review until the semantic gate passes**

Use the same independent reviewer for each fix round. `Approved` requires zero Critical and zero Important findings. Record Minor findings in the review file for the final coordinator check; do not silently discard them.

### Task 4: Final Verification and User Handoff

**Files:**
- Modify: `/Users/caoye/Documents/xs/show-me-the-story/.superpowers/sdd/progress.md`
- Read only: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md`
- Read only: `/Users/caoye/storys/被逼急了，只好修仙/Chapter_01.md`
- Read only: `/Users/caoye/storys/被逼急了，只好修仙/progress.json`

**Interfaces:**
- Consumes: the reviewer-approved draft and all fresh verification evidence.
- Produces: a user-reviewable draft link and a clear statement that the running novel system remains unchanged.

- [ ] **Step 1: Rerun the complete mechanical verification**

Run Task 2 Step 7 exactly from a fresh shell. Do not reuse writer or reviewer output.

Expected: `chapter_mechanical_checks=PASS`, current draft hash printed, and both system-file comparisons exit 0.

- [ ] **Step 2: Verify review and continuity status**

Run:

```bash
set -e
review='/Users/caoye/Documents/xs/show-me-the-story/.superpowers/sdd/chapter-1-ai-video-review.md'
continuity='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/continuity.md'
rg -q 'Assessment: Approved|\*\*Approved\*\*' "$review"
rg -q '第 2 章.*第一次真正到院' "$continuity"
rg -q '第 4 章.*第一次当面房租谈判' "$continuity"
echo 'semantic_and_continuity_gates=PASS'
```

Expected: `semantic_and_continuity_gates=PASS`.

- [ ] **Step 3: Update the progress ledger**

Append one line containing the current draft’s first eight SHA-256 characters, character count, review result, and `awaiting user acceptance`. Do not mark system synchronization complete.

- [ ] **Step 4: Present the draft and stop**

Report:

```text
- final character count and SHA-256;
- one-paragraph summary of the new dramatic engine;
- independent review result and any remaining Minor notes;
- clickable path to the draft;
- confirmation that system Chapter_01.md and progress.json remain byte-identical to backups.
```

Stop for user review. Do not write the draft into the application, do not generate Chapter 2, and do not ask the user to review multiple chapters at once.
