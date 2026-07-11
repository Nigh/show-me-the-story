# 《被逼急了，只好修仙》前五章合理性重写实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从头重写第 1 至 5 章，在不提前暴露平台阴谋的前提下，建立可信的都市困境、住房空间和隐蔽修仙入口。

**Architecture:** 先备份当前系统状态并建立独立重写目录，再按章节顺序写作，每章完成机械检查、因果复核和用户验收后才能进入下一章。五章全部通过后，停止本地服务，原子更新大纲与标题，重启后通过章节编辑接口同步正文并验证磁盘和服务内存一致。

**Tech Stack:** Markdown、JSON、`jq`、`rg`、Perl、本地 REST API `http://127.0.0.1:48090`、show-me-the-story 本地服务。

## Global Constraints

- 第 1 至 5 章全部从头写，旧正文只作人物和现实事件素材。
- 使用贴近林砚的有限第三人称，不提前理解平台、天道或收割机制。
- 前五章不调查、不怀疑、不暗示平台阴谋；平台表现为普通内容载体。
- 第 1 至 3 章无超常能力；第 4 章首次出现可作普通解释的异常；第 5 章离线复现后仅确认方法值得观察。
- 林砚租住老小区 502，朱阿姨住同楼 202；陈婶住 501，侯三在小区门岗，但二人前五章不承担剧情任务。
- 医院、公司、房东和短视频各自按现实利益运行，不提前串成同一幕后网络。
- 每个信息必须有来源，每个角色必须有独立动机和合理信息权限。
- 每章目标 15,000 至 20,000 字，允许因场景完整自然超出，不用重复心理、环境或观点凑字。
- 每个独立草稿第一行固定为 `# 第 N 章：标题`，第二行为空行，正文从第三行开始，供最终同步脚本稳定剥离标题。
- 正文不出现后台证据编号、章节元叙述、作者说明或写作指令。
- 用户逐章确认前只写独立草稿，不覆盖系统已验收正文。

---

### Task 1: 备份与连续性账本

**Files:**
- Create directory: `/Users/caoye/storys/被逼急了，只好修仙/backup-opening-1-5-before-grounded-rewrite-20260710`
- Copy: `/Users/caoye/storys/被逼急了，只好修仙/progress.json`
- Copy: `/Users/caoye/storys/被逼急了，只好修仙/settings.json`
- Copy: `/Users/caoye/storys/被逼急了，只好修仙/Chapter_01.md` through `Chapter_05.md`
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/continuity.md`
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/outlines.json`

**Interfaces:**
- Consumes: 当前系统正文、已批准设计、当前 `progress.json`。
- Produces: 可恢复备份、五章统一事实账本、最终系统同步用标题与大纲补丁。

- [ ] **Step 1: 创建并校验备份**

Run:

```bash
backup='/Users/caoye/storys/被逼急了，只好修仙/backup-opening-1-5-before-grounded-rewrite-20260710'
mkdir -p "$backup"
cp '/Users/caoye/storys/被逼急了，只好修仙/progress.json' "$backup/progress.json"
cp '/Users/caoye/storys/被逼急了，只好修仙/settings.json' "$backup/settings.json"
for n in 01 02 03 04 05; do cp "/Users/caoye/storys/被逼急了，只好修仙/Chapter_${n}.md" "$backup/Chapter_${n}.md"; done
cmp '/Users/caoye/storys/被逼急了，只好修仙/progress.json' "$backup/progress.json"
cmp '/Users/caoye/storys/被逼急了，只好修仙/settings.json' "$backup/settings.json"
```

Expected: `cmp` 无输出且退出码为 0，五个章节文件均存在于备份目录。

- [ ] **Step 2: 写连续性账本**

`continuity.md` 必须明确以下事实：

- 第 1 章开始时林砚仍在职，秦素梅已住院，房租逾期。
- 第 1 章结束时林砚拒签但没有拿到钱。
- 第 2 章医院线不出现超常现象，绿色通道与修仙无关。
- 第 3 章孙苗只能让林砚短暂查看职责内工资表，不能提供完整后台。
- 第 3 章末普通呼吸视频首次进入视野，林砚未跟练。
- 第 4 章朱阿姨住 202，林砚住 502；无停电、陈婶、侯三、授权页或匿名提醒。
- 第 5 章只验证身体反应，不检查平台记录。

Expected: 任一章节写手只读该文件即可回答人物位置、已知信息、欠款状态和修仙可见度。

- [ ] **Step 3: 写五章标题与 300 至 500 字大纲补丁**

`outlines.json` 必须是五元素 JSON 数组，每项只包含 `num`、`title`、`outline`。标题依次为《裁员当天，房租也到期了》《病床不能等工资》《工资表少了一个月》《三分钟吐纳不是救命符》《关掉视频以后》。每条大纲必须对应设计文档中的章节结构，不出现推荐日志、平台阴谋、异常授权、匿名提醒、陈婶救场或侯三递线索。

Run:

```bash
jq -e 'length == 5 and all(.[]; (.num >= 1 and .num <= 5) and ((.title | length) > 0) and ((.outline | length) >= 300) and ((.outline | length) <= 500))' '/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/outlines.json'
```

Expected: 输出 `true`。

### Task 2: 重写第 1 章

**Files:**
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md`

**Interfaces:**
- Consumes: `continuity.md` 与第 1 章新大纲。
- Produces: 从在职到拒签离开的完整现实开篇，为第 2 章医院入场提供时间与资金状态。

- [ ] **Step 1: 写完整正文**

必须包含：AI 工具上线后人手减少与指标上涨；周启明要求林砚签署未到账确认；医院缴费消息与房租催缴先后到来；林砚拒签并保存自己合法接触的材料；离开公司时仍没有钱和成熟计划。

不得包含：灰线、灰雾、吐纳、修仙、灵气、气感、平台授权、视频引导或主角立即看穿系统全局。

- [ ] **Step 2: 运行章节检查**

Run:

```bash
f='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md'
test "$(wc -m < "$f" | tr -d ' ')" -ge 15000
if tail -n +2 "$f" | rg -n 'EV-|证据编号|本章|作者|读者|修仙|炼气|灵气|气感|吐纳|灰线|灰雾|天道|闻雪楼|流量池|吞噬'; then exit 1; fi
rg -q '周启明' "$f"
rg -q '秦素梅' "$f"
rg -q '朱阿姨' "$f"
```

Expected: 退出码为 0；正文达到目标长度且无超常词和元叙述。

- [ ] **Step 3: 用户验收第 1 章**

交付时报告篇幅、现实事件链和检查结果。用户提出的修改在该章内闭环后才能开始第 2 章。

### Task 3: 重写第 2 章

**Files:**
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_02.md`

**Interfaces:**
- Consumes: 已确认第 1 章离场状态、`continuity.md` 与第 2 章新大纲。
- Produces: 医院现实压力、母子互相隐瞒与普通呼吸常识，为第 3 章返回公司提供明确时间窗口。

- [ ] **Step 1: 写完整正文**

必须包含：缴费与床位的现实规则；罗青职责内帮助；秦素梅隐瞒疼痛，林砚隐瞒失业；第三方绿色通道条款不清但与修仙无关；护士让林砚坐下并正常呼吸。

不得包含：罗青泄露后台、医院与平台共享数据、呼吸产生能力、灵疗、修行潜力或任何高层观察。

- [ ] **Step 2: 运行章节检查**

Run:

```bash
f='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_02.md'
test "$(wc -m < "$f" | tr -d ' ')" -ge 15000
if tail -n +2 "$f" | rg -n 'EV-|证据编号|本章|作者|读者|修仙|炼气|灵气|气感|吐纳|灰线|灰雾|天道|闻雪楼|流量池|吞噬|平台.*医院|医院.*平台'; then exit 1; fi
rg -q '罗青' "$f"
rg -q '秦素梅' "$f"
```

Expected: 退出码为 0；医院规则和人物选择可以独立成立，无修仙效果或平台串联。

- [ ] **Step 3: 用户验收第 2 章**

用户确认人物信息权限、医院流程和母子关系后才能开始第 3 章。

### Task 4: 重写第 3 章

**Files:**
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_03.md`

**Interfaces:**
- Consumes: 已确认第 2 章离场状态、`continuity.md` 与第 3 章新大纲。
- Produces: 有限工资缺口事实和正常内容推荐链，为第 4 章跟练提供自然入口。

- [ ] **Step 1: 写完整正文**

必须包含：林砚回公司取物并询问结算；孙苗只让他短暂查看职责内工资表；交涉没有当场获胜；周启明有现实压力但仍主动向下转移风险；夜间内容从劳动纠纷、便宜饭、失眠自然滑向普通呼吸视频。

不得包含：孙苗发送完整后台、平台精准说出林砚困境、视频出现修仙词、林砚当晚跟练或产生异常。

- [ ] **Step 2: 运行章节检查**

Run:

```bash
f='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_03.md'
test "$(wc -m < "$f" | tr -d ' ')" -ge 15000
if tail -n +2 "$f" | rg -n 'EV-|证据编号|本章|作者|读者|修仙|炼气|灵气|气感|吐纳|灰线|灰雾|天道|闻雪楼|流量池|吞噬|负债.*病痛.*呼吸'; then exit 1; fi
rg -q '孙苗' "$f"
rg -q '呼吸' "$f"
```

Expected: 退出码为 0；视频入口由正常观看行为产生，结尾前无跟练和异常。

- [ ] **Step 3: 用户验收第 3 章**

用户确认公司交涉、孙苗边界和推荐链均自然后才能开始第 4 章。

### Task 5: 重写第 4 章

**Files:**
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_04.md`

**Interfaces:**
- Consumes: 已确认第 3 章结尾出现的普通呼吸视频、`continuity.md` 与第 4 章新大纲。
- Produces: 首次仍可作普通解释的身体异常，为第 5 章离线复现提供林砚记住的呼吸顺序。

- [ ] **Step 1: 写完整正文**

必须包含：林砚在 202 与朱阿姨谈租金；朱阿姨既限定时间又给剩粥；林砚回 502 后因失眠重开普通呼吸视频；古法节奏混在正常练习中；他追逐凉意而练过头，独自停止、开窗、喝水并恢复；视频保持正常可用；他只记下节奏与身体反应。

不得包含：停电、公共水池、陈婶、侯三、外包人员、平台授权、收益补贴、匿名提醒、视频删除、记录异常、平台怀疑或“修仙已确认”。

- [ ] **Step 2: 运行章节检查**

Run:

```bash
f='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_04.md'
test "$(wc -m < "$f" | tr -d ' ')" -ge 15000
if tail -n +2 "$f" | rg -n 'EV-|证据编号|本章|作者|读者|修仙|炼气|灵气|气感|天道|闻雪楼|流量池|吞噬|停电|公共水池|陈婶|侯三|外包|授权|补贴|匿名|别签|看日志|视频.*删|推荐日志'; then exit 1; fi
rg -q '202' "$f"
rg -q '502' "$f"
rg -q '南瓜粥' "$f"
rg -q '呼吸' "$f"
```

Expected: 退出码为 0；异常只发生在身体内部，平台和住房逻辑保持普通。

- [ ] **Step 3: 用户验收第 4 章**

用户确认呼吸引导隐蔽、独处处理合理且朱阿姨关系成立后才能开始第 5 章。

### Task 6: 重写第 5 章

**Files:**
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_05.md`

**Interfaces:**
- Consumes: 第 4 章记录的呼吸顺序和身体状态、`continuity.md` 与第 5 章新大纲。
- Produces: 离线重复结果、暂定“吐纳”称呼和极小的超常可能，为第 6 章继续现实主线提供稳定边界。

- [ ] **Step 1: 写完整正文**

必须包含：林砚先处理结算、医院和房租安排；在吃饭、休息和关闭视频后保守复现；普通深呼吸无异常，特定顺序才出现相似微凉；他查询公开呼吸安全资料并看到“吐纳”一词；在普通沟通中用缩短节奏稳住手抖和心跳；结尾只露出极小的感知变化。

不得包含：查看推荐日志、调查平台、权限异常、视频删除、平台与现实困境关联、修仙确认、识破谎言或取得直接经济收益。

- [ ] **Step 2: 运行章节检查**

Run:

```bash
f='/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_05.md'
test "$(wc -m < "$f" | tr -d ' ')" -ge 15000
if tail -n +2 "$f" | rg -n 'EV-|证据编号|本章|作者|读者|推荐日志|观看记录|权限异常|空白条目|匿名账号|别签|看日志|视频.*删|平台.*困境|困境.*平台|流量池|吞噬|闻雪楼|科技天道|确认.*修仙|识破.*谎'; then exit 1; fi
rg -q '吐纳' "$f"
rg -q '关掉' "$f"
```

Expected: 退出码为 0；林砚只确认可重复的身体变化，没有平台推断和能力越阶。

- [ ] **Step 3: 用户验收第 5 章**

用户确认方法验证、现实行动顺序和能力上限后进入五章联合复核。

### Task 7: 五章联合复核

**Files:**
- Verify: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_01.md` through `Chapter_05.md`
- Update: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/continuity.md`
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/summaries.json`

**Interfaces:**
- Consumes: 五章用户确认稿。
- Produces: 可同步系统的连续正文和最终事实账本。

- [ ] **Step 1: 检查长句复读**

Run:

```bash
perl -CSD -Mutf8 -ne 'while(/([^。！？\n]{12,}[。！？])/g){$c{$1}++} END{for $s (sort {$c{$b}<=>$c{$a}} keys %c){print "$c{$s}\t$s\n" if $c{$s}>1}}' /Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_0[1-5].md
```

Expected: 无 12 字以上句子在两章中原样重复。人物口头禅如确需重复，必须在人工复核中说明。

- [ ] **Step 2: 检查住房与功能性配角**

Run:

```bash
if rg -n '公共水池|共用水池|整栋出租楼|远程断电|夜查租户|私人保安|陈婶|侯三' /Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_0[1-5].md; then exit 1; fi
rg -n '202|502' '/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_04.md'
```

Expected: 禁止项无命中；第 4 章明确 202 与 502 的空间关系。

- [ ] **Step 3: 检查修仙可见度阶梯**

Run:

```bash
if tail -n +2 /Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_0[1-3].md | rg -n '修仙|炼气|灵气|气感|吐纳|灰线|灰雾|天道'; then exit 1; fi
if tail -n +2 '/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_04.md' | rg -n '修仙|炼气|灵气|气感|天道'; then exit 1; fi
rg -q '吐纳' '/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/Chapter_05.md'
```

Expected: 第 1 至 3 章无超常词；第 4 章只有无法命名的身体异常；第 5 章才将“吐纳”作为公开资料中的暂定称呼。

- [ ] **Step 4: 人工因果审计**

逐章回答并补入 `continuity.md`：每条关键信息由谁提供、为何能知道、为何此刻出现；每次角色帮助付出什么边界；每章结尾留下的未解决问题是否在下一章真实存在。

Expected: 没有连续巧合解谜、功能性配角救场、主角信息越权或平台阴谋泄漏。

- [ ] **Step 5: 写系统章节摘要**

`summaries.json` 写成五元素 JSON 数组，每项只包含 `num` 和 `summary`。每条摘要 100 至 180 字，只记录本章已经发生的事实，不评价人物、不写后台信息、不预告后期阴谋。

Run:

```bash
jq -e 'length == 5 and all(.[]; (.num >= 1 and .num <= 5) and ((.summary | length) >= 100) and ((.summary | length) <= 180))' '/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/summaries.json'
```

Expected: 输出 `true`。

### Task 8: 用户总验收后同步系统

**Files:**
- Modify: `/Users/caoye/storys/被逼急了，只好修仙/progress.json`
- Regenerate: `/Users/caoye/storys/被逼急了，只好修仙/Chapter_01.md` through `Chapter_05.md`
- Keep: `/Users/caoye/storys/被逼急了，只好修仙/drafts/rewrite-opening-20260710/*`

**Interfaces:**
- Consumes: 用户总验收后的五章正文与 `outlines.json`。
- Produces: 磁盘、服务内存、章节 Markdown、大纲标题完全一致的系统状态。

- [ ] **Step 1: 再次备份同步前状态**

Run:

```bash
cp '/Users/caoye/storys/被逼急了，只好修仙/progress.json' '/Users/caoye/storys/被逼急了，只好修仙/backup-progress-before-opening-sync-20260710.json'
cmp '/Users/caoye/storys/被逼急了，只好修仙/progress.json' '/Users/caoye/storys/被逼急了，只好修仙/backup-progress-before-opening-sync-20260710.json'
```

Expected: `cmp` 无输出且退出码为 0。

- [ ] **Step 2: 停止服务并原子更新前五章标题、大纲与正文**

先停止当前服务：

```bash
pid=$(lsof -ti tcp:48090 | head -n 1)
kill "$pid"
while kill -0 "$pid" 2>/dev/null; do sleep 1; done
```

使用下面的原子更新脚本读取 `outlines.json`、`summaries.json` 和五个草稿，将 `progress.json` 中对应章节的 `title`、`outline`、`content`、`summary` 更新为用户确认版本。正文写入时去掉 Markdown 标题行。

Run:

```bash
root='/Users/caoye/storys/被逼急了，只好修仙'
rewrite="$root/drafts/rewrite-opening-20260710"
stage=$(mktemp -d)
for n in 01 02 03 04 05; do sed '1,2d' "$rewrite/Chapter_${n}.md" > "$stage/Chapter_${n}.txt"; done
tmp=$(mktemp "$root/progress.json.XXXXXX")
jq \
  --slurpfile outlines "$rewrite/outlines.json" \
  --slurpfile summaries "$rewrite/summaries.json" \
  --rawfile c1 "$stage/Chapter_01.txt" \
  --rawfile c2 "$stage/Chapter_02.txt" \
  --rawfile c3 "$stage/Chapter_03.txt" \
  --rawfile c4 "$stage/Chapter_04.txt" \
  --rawfile c5 "$stage/Chapter_05.txt" '
  .chapters |= map(
    if (.num >= 1 and .num <= 5) then
      . as $chapter
      | ($outlines[0][] | select(.num == $chapter.num)) as $meta
      | ($summaries[0][] | select(.num == $chapter.num)) as $sum
      | .title = $meta.title
      | .outline = $meta.outline
      | .summary = $sum.summary
      | .content = (if .num == 1 then $c1 elif .num == 2 then $c2 elif .num == 3 then $c3 elif .num == 4 then $c4 else $c5 end)
    else . end
  )' "$root/progress.json" > "$tmp"
jq -e '[.chapters[] | select(.num >= 1 and .num <= 5)] | length == 5 and all(.[]; .status == "accepted" and (.content | length >= 15000))' "$tmp"
mv "$tmp" "$root/progress.json"
rm -rf "$stage"
```

Expected: 第 5 章标题为《关掉视频以后》，第 1 至 5 章大纲均为新版，正文不包含 Markdown 标题行。

- [ ] **Step 3: 重启服务并等待健康响应**

Run:

```bash
nohup '/Users/caoye/.local/bin/show-me-the-story-local' '/Users/caoye' >'/tmp/show-me-the-story-local.log' 2>&1 &
for i in $(seq 1 30); do curl -fsS 'http://127.0.0.1:48090/api/chapter/current' >/dev/null && break; sleep 1; done
curl -fsS 'http://127.0.0.1:48090/api/chapter/current' | jq '{num,title,status}'
```

Expected: HTTP 请求成功；写作前沿仍指向原来的待写章节，不倒退到第 1 至 5 章。

- [ ] **Step 4: 让服务重新生成章节 Markdown**

对第 1 至 5 章分别调用 `POST /api/chapter/edit`，使用 `replace_text` 将正文第一句替换为相同文本。该无内容变化编辑会从服务内存重新保存 `progress.json` 并调用 `SaveChapterMarkdown`，从而让五个 `Chapter_XX.md` 使用新版标题、摘要和正文。

Run:

```bash
root='/Users/caoye/storys/被逼急了，只好修仙'
for n in 1 2 3 4 5; do
  first=$(jq -r --argjson n "$n" '.chapters[] | select(.num == $n) | .content | split("\n") | map(select(length > 0))[0]' "$root/progress.json")
  jq -n --argjson n "$n" --arg text "$first" '{num:$n,operation:"replace_text",old_text:$text,new_text:$text}' \
    | curl -fsS -X POST 'http://127.0.0.1:48090/api/chapter/edit' -H 'Content-Type: application/json' --data-binary @- \
    | jq -e '.success == true'
done
```

Expected: 五次响应均包含 `"success":true`，`Chapter_05.md` 首行为新版标题。

- [ ] **Step 5: 最终一致性验证**

Run:

```bash
jq -e '[.chapters[] | select(.num >= 1 and .num <= 5)] | length == 5 and all(.[]; .status == "accepted" and (.content | length >= 15000))' '/Users/caoye/storys/被逼急了，只好修仙/progress.json'
jq -r '.chapters[] | select(.num >= 1 and .num <= 5) | [.num,.title,.status,(.content|length)] | @tsv' '/Users/caoye/storys/被逼急了，只好修仙/progress.json'
head -n 1 '/Users/caoye/storys/被逼急了，只好修仙/Chapter_05.md'
```

Expected: `jq` 输出 `true`；五章均为 `accepted`、正文不少于 15,000 字；第 5 章标题为《关掉视频以后》。
