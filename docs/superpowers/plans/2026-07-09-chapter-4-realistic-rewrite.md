# 第 4 章现实克制型样章实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不覆盖系统现有正文的前提下，产出约 15,000 字的第 4 章现实克制型样章。

**Architecture:** 先锁定第 3、4、5 章的连续性边界，再按“催租与给粥、停电与推送、尝试吐纳、授权诱导、匿名示警”五个连续场景重写。成稿保存为独立样章文件，通过机械禁词检查和人工风格检查后交用户审阅。

**Tech Stack:** Markdown、本地 `progress.json`、`rg`、`wc`、show-me-the-story 本地项目文件。

## Global Constraints

- 保留既定剧情结果，不改变第 3、5 章的必要衔接。
- 采用有限第三人称，林砚只能理解眼前异常。
- 主体语言朴素、具体、连续，少解释、多潜台词。
- 正文目标约 15,000 字，可因场景完整性自然浮动，不凑字。
- 不出现正文证据编号、章节元叙述、作者说明、写作指令或尚未解禁的高阶世界观名词。
- 样章先写入独立文件，不调用章节编辑接口，不覆盖系统中的已验收第 4 章。

---

### Task 1: 锁定连续性边界

**Files:**
- Read: `/Users/caoye/storys/被逼急了，只好修仙/Chapter_03.md`
- Read: `/Users/caoye/storys/被逼急了，只好修仙/Chapter_04.md`
- Read: `/Users/caoye/storys/被逼急了，只好修仙/Chapter_05.md`
- Read: `/Users/caoye/storys/被逼急了，只好修仙/progress.json`

**Interfaces:**
- Consumes: 当前已验收正文、章节 4 大纲、章节 3 与 5 的衔接信息。
- Produces: 样章必须保留的入场状态、离场状态、角色信息权限与能力上限。

- [ ] **Step 1: 提取第 4 章大纲与第 3、5 章衔接段**

Run: 使用结构化 JSON 读取第 4 章大纲，并用 `sed` 读取第 3 章结尾和第 5 章开头。

Expected: 明确林砚进入第 4 章时的身体、债务、家庭和修行状态，以及第 5 章依赖的授权与异常线索。

- [ ] **Step 2: 列出不可改动的事实**

Expected: 至少覆盖朱阿姨宽限一天、剩粥、停电、三分钟吐纳、身体不适、未签授权、匿名“别签”提醒七项事实。

### Task 2: 重写样章

**Files:**
- Create: `/Users/caoye/storys/被逼急了，只好修仙/drafts/第004章-三分钟吐纳不是救命符-现实克制样章.md`

**Interfaces:**
- Consumes: Task 1 的连续性边界和已批准的改写设计。
- Produces: 完整 Markdown 样章，含标题和正文，不含系统摘要或后台编号。

- [ ] **Step 1: 写催租与给粥场景**

Expected: 朱阿姨通过催缴单、账本、动作和说话习惯表现自身压力；林砚的窘迫通过回避、身体反应和有限回应呈现。

- [ ] **Step 2: 写停电与精准推送场景**

Expected: 推送出现得过分及时，但林砚只能怀疑平台标签和推荐逻辑，不能看穿幕后力量。

- [ ] **Step 3: 写吐纳尝试与身体失控场景**

Expected: 练气既有微弱真实反馈，也有缺氧、恐慌和误判；林砚因求生而尝试，不因聪明而天然免疫诱惑。

- [ ] **Step 4: 写授权诱导与匿名示警场景**

Expected: 林砚在最想相信的时候停下签署，先检查权限和推荐痕迹；“别签”只制造疑问，不解释全局。

- [ ] **Step 5: 完成结尾钩子**

Expected: 结尾同时保留欠租、母亲病情和异常修行入口三重压力，避免观点总结。

### Task 3: 风格与连续性复核

**Files:**
- Verify: `/Users/caoye/storys/被逼急了，只好修仙/drafts/第004章-三分钟吐纳不是救命符-现实克制样章.md`

**Interfaces:**
- Consumes: Task 2 样章。
- Produces: 满足设计文档验收标准的可审阅版本。

- [ ] **Step 1: 检查篇幅和禁词**

Run:

```bash
wc -m '/Users/caoye/storys/被逼急了，只好修仙/drafts/第004章-三分钟吐纳不是救命符-现实克制样章.md'
rg -n 'EV-|证据编号|本章|第3001道|道外|众生席|百年协议|科技天道|作者|读者|写作' '/Users/caoye/storys/被逼急了，只好修仙/drafts/第004章-三分钟吐纳不是救命符-现实克制样章.md'
```

Expected: 字符数接近 15,000；禁词扫描无命中。

- [ ] **Step 2: 检查模板句与碎段密度**

Run: 搜索“不是……而是……”“他终于明白”“这才是”等惯用总结句，并抽读开头、中段、结尾。

Expected: 不形成连续模板节拍；短段只服务于压力骤停或转折。

- [ ] **Step 3: 核对剧情节点与人物声音**

Expected: 七项不可改动事实齐全；朱阿姨、林砚和匿名提醒具有不同语言来源，人物不替作者讲道理。

### Task 4: 交付样章

**Files:**
- Deliver: `/Users/caoye/storys/被逼急了，只好修仙/drafts/第004章-三分钟吐纳不是救命符-现实克制样章.md`

**Interfaces:**
- Consumes: Task 3 通过复核的样章。
- Produces: 用户可阅读、可比较、尚未覆盖系统正文的第 4 章样章。

- [ ] **Step 1: 汇报篇幅、风格变化和文件位置**

Expected: 明确说明系统第 4 章未被覆盖，等待用户确认后再写入。
