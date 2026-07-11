# 第 1 章跨日多冲突重写实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把拖拉的单日流程稿重写成跨两天、由职场、裁员、医院和房租四类冲突共同推进的第 1 章。

**Architecture:** 先保存当前已审稿作为对照，再更新连续性账本和第 1、2、4 章大纲接缝。正文从头重组，只保留会改变人物处境的场景，使用自然时间跳切略过等待、重复手续和通勤，完成后进行规格与文学质量双重审查。

**Tech Stack:** Markdown、JSON、`jq`、`rg`、Perl、本地小说项目文件。

## Global Constraints

- 第 1 章跨越两天，不按分钟记录完整工作日。
- 依次包含 AI 误分工单与指标上涨、裁员拒签、医院费用与母子隐瞒、房租谈判、次日工资未到账后的现实选择。
- 四类冲突必须互相改变下一场的资源、关系或期限；完整拒签只能发生一次。
- 林砚当日离开公司，设备、权限和交接只呈现必要后果，不展开多轮流程。
- 第 1 章第一次进入医院并确认费用缺口；第 2 章从后续检查变化开始，不复演首次到院。
- 第 1 章完成第一次房租碰撞；第 4 章必须出现新的期限、条件或关系变化，不复演同一催租对话。
- 使用现实克制型有限第三人称；少解释、多动作和潜台词。
- AI 只是管理权工具，不是有意识反派；公司、医院、房东彼此独立。
- 第 1 章不出现修仙、吐纳、灰线、视频入口、平台授权或后期世界观。
- 12,000 至 16,000 字仅作参考区间，不设硬性最低字数；场景完成后立即收束，不为达到数字补写流程、通勤、心理、环境或观点。
- 用户确认前只修改独立草稿与重写辅助文件，不覆盖系统 `Chapter_01.md` 或 `progress.json`。

---

### Task 1: 保存对照稿并更新接缝

**Files:**
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01-before-multiconflict.md`
- Modify: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/continuity.md`
- Modify: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/outlines.json`

**Interfaces:**
- Consumes: 当前第 1 章审稿、已批准多冲突设计。
- Produces: 可比较的旧草稿快照，以及与跨日第 1 章一致的第 1、2、4 章接缝。

- [ ] **Step 1: 保存当前草稿并校验**

Run:

```bash
src='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md'
dst='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01-before-multiconflict.md'
cp "$src" "$dst"
cmp "$src" "$dst"
```

Expected: `cmp` 无输出且退出码为 0。

- [ ] **Step 2: 更新连续性账本**

`continuity.md` 必须明确：第 1 章跨两天；第一天完成离职、首次到院和第一次房租谈判；第二天早晨工资仍未到账，章尾落在林砚的现实选择。第 2 章不得重写首次到院，第 4 章不得重写第一次房租碰撞。将旧的“第 1 章结束于去医院途中”和 15,000 字硬下限全部删除。

Run:

```bash
rg -n '跨两天|首次到院|第一次房租|第二天早晨|12,000 至 16,000' '/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/continuity.md'
if rg -n '去医院途中|15,000 至 20,000|至少 15,000' '/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/continuity.md'; then exit 1; fi
```

Expected: 新接缝均命中，旧接缝和旧下限无命中。

- [ ] **Step 3: 更新第 1、2、4 章大纲**

第 1 章大纲写明跨两天四冲突及次日选择；第 2 章从随后两至三天的检查变化、床位限制、母子隐瞒拆穿和房租宽限到期开始；第 4 章使用新的房租条件或关系后果，再进入普通呼吸视频。三条大纲均保持 300 至 500 字，第 3、5 章不改。

Run:

```bash
f='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/outlines.json'
jq -e 'length == 5 and all(.[]; ((.outline | length) >= 300) and ((.outline | length) <= 500))' "$f"
jq -e '([.[] | select(.num == 1 or .num == 2 or .num == 4)] | length) == 3' "$f"
if jq -r '.[] | select(.num == 1 or .num == 2 or .num == 4) | .outline' "$f" | rg -n '推荐日志|平台阴谋|异常授权|匿名提醒|陈婶|侯三'; then exit 1; fi
```

Expected: 两个 `jq` 命令输出 `true`，禁入线索无命中。

### Task 2: 重写第 1 章

**Files:**
- Modify: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md`

**Interfaces:**
- Consumes: 更新后的 `continuity.md`、第 1 章大纲、对照稿。
- Produces: 跨两天的多冲突第 1 章，为第 2 章后续医院变化提供新入场状态。

- [ ] **Step 1: 从头重组正文**

采用以下场景顺序：

1. 第一天上午：一个 AI 误分工单和一个上涨指标迅速建立职场压力。
2. 第一天中午：周启明通知裁员，林砚在医院缴费压力下犹豫，最终拒绝签署“已收齐款项”。完整拒签只出现一次。
3. 第一天下午至傍晚：跳切到医院。林砚第一次当面发现秦素梅隐瞒检查安排和费用；只建立缺口，不展开完整医院调查。
4. 第一夜：跳切到老小区 202。朱阿姨要求明确房租期限并展示自己的扣款压力，只给短暂宽限。
5. 第二天早晨：工资仍未到账，公司权限关闭，医院和房租期限并存。林砚作出一个具体的下一步选择，立即收束。

删除或压缩：逐项任务交接、多个离职同事的完整收拾过程、设备核验细目、权限遗漏清单、贷款广告、公交逐站过程、反复材料点评和资金盘点。

- [ ] **Step 2: 运行机械检查**

Run:

```bash
f='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md'
wc -m "$f"
test "$(sed -n '1p' "$f")" = '# 第 1 章：裁员当天，房租也到期了'
test -z "$(sed -n '2p' "$f")"
if tail -n +2 "$f" | rg -n 'EV-|证据编号|本章|作者|读者|修仙|炼气|灵气|气感|吐纳|灰线|灰雾|天道|闻雪楼|流量池|吞噬|视频入口|平台授权'; then exit 1; fi
for p in '周启明' '秦素梅' '朱阿姨' '上月.*十项|此前.*十项|原来.*十项' '十二项' '这份我不签' '第二天'; do rg -q "$p" "$f"; done
test "$(rg -o '这份我不签' "$f" | wc -l | tr -d ' ')" -eq 1
```

Expected: 章节格式正确，必含事件命中，完整拒签一次，禁入内容无命中。`wc -m` 只报告篇幅，不作为硬性通过门槛。

- [ ] **Step 3: 检查节奏与复读**

Run:

```bash
perl -CSD -Mutf8 -ne 'while(/([^。！？\n]{12,}[。！？])/g){$c{$1}++} END{for $s (sort {$c{$b}<=>$c{$a}} keys %c){print "$c{$s}\t$s\n" if $c{$s}>1}}' '/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md'
if rg -n '九点零七|九点十二|九点二十八|九点三十二|九点三十五|九点四十|九点四十七|三点半|四点零六' '/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md'; then exit 1; fi
```

Expected: 无 12 字以上原样复读；分钟级时间链无命中。人工抽读确认四类冲突均改变下一场处境，且没有用新冲突数量制造机械模板。

### Task 3: 双重审查与用户验收

**Files:**
- Verify: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md`
- Compare: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01-before-multiconflict.md`

**Interfaces:**
- Consumes: Task 2 完整稿。
- Produces: 规格符合、节奏通过且尚未覆盖系统的用户审阅稿。

- [ ] **Step 1: 规格审查**

逐项核对跨两天、四类冲突、单次拒签、无前期修仙、住房空间、人物信息权限、第 2/4 章接缝和系统未覆盖。

- [ ] **Step 2: 文学质量审查**

阅读全文并比较对照稿，重点检查：是否真正提速；多冲突是否自然递进而非堆灾难；医院和房租人物是否有自己的利益；是否仍有流程凑字、通勤凑字、作者总结、碎段和理想化正确主角。

- [ ] **Step 3: 用户验收**

报告最终篇幅、删除或合并的拖慢段落、审查结果和草稿路径。用户确认前不进入第 2 章，不覆盖系统正文。
