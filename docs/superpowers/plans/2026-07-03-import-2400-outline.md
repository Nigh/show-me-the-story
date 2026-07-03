# 2400 Chapter Outline Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the approved novel master spec into a validated 2400-chapter outline and import it into the local `show-me-the-story` project without calling the external model API.

**Architecture:** Use the approved master spec as the source of truth, author an 80-arc map, expand each arc into one batch file, merge batches into the `import_outline` JSON payload, validate structure and story anchors deterministically, then import through the existing Go backend endpoint. The app remains the source of persistence; do not edit `/Users/caoye/storys/.../progress.json` directly.

**Tech Stack:** Markdown spec, JSON outline artifacts, Node.js validation/build scripts, existing Go HTTP API (`/api/projects/select`, `/api/config`, `/api/outline/import`, `/api/outline/summary`), optional MCP sidecar `import_outline`.

---

## File Structure

- Read: `docs/superpowers/specs/2026-07-03-novel-master-outline-design.md`
- Create: `scripts/novel/build-outline.mjs`
- Create: `scripts/novel/validate-outline.mjs`
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-arc-map.json`
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/batches/arc-001.json` through `docs/novel/outlines/bi-ji-ji-le-2400/batches/arc-080.json`
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400.json`
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-review.md`
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-import-result.json`

## Artifact Schemas

Arc map file:

```json
{
  "title": "被逼急了，只好修仙",
  "chapter_count": 2400,
  "arcs": [
    {
      "arc": 1,
      "part": 1,
      "start": 1,
      "end": 30,
      "title": "裁员当天，仙缘刷屏",
      "cultivation": "识气初醒",
      "ground_line": "林砚被裁员、催租、医院缴费压垮，刷到三分钟炼气。",
      "high_line": "平台把他标入低信用高转化样本。",
      "burden_bearer": "匿名账号第一次提醒别签。",
      "end_state": "林砚第一次确认气不是幻觉，平台开始记录他。"
    }
  ]
}
```

Batch file:

```json
{
  "arc": 1,
  "chapters": [
    {
      "num": 1,
      "title": "裁员当天，房租也到期了",
      "outline": "核心事件：林砚被公司裁员并被扣薪，回到出租屋又遇到房东催租和医院催费，现实压力集中爆发。\\n修仙推进：手机刷到“三分钟炼气”短视频，林砚暂时只当作荒诞骗局。\\n人物推进：林砚的逃避、秦素梅的隐忍、周启明的压榨同时立住。\\n高层/伏笔：匿名账号留下“这一轮别再签”的评论，林砚看不懂。\\n后果钩子：林砚为了省一顿饭钱，半夜照着视频开始吐纳。"
    }
  ]
}
```

Final import payload:

```json
{
  "title": "被逼急了，只好修仙",
  "core_prompt": "长篇都市硬修仙群像反收割小说。林砚从第 50 火种纪元第 50 年入局，前两部不知道纪元真相，第三部揭示第五十轮后半场。主线坚持修仙感知、千门识局、三十六计破局、员选式分析、易经思维、群众路线，并最终走向道外不称王、薪火公约与世界大同。",
  "story_synopsis": "林砚被房租、裁员、医院、网贷、物业、相亲和老板压榨逼到墙角，通过短视频误入修仙。他先用识气搞钱救家人，随后发现修仙平台、宗门、香火、魔法、科技、妖魔、外星文明、平衡世界和多维宇宙都在争夺众生命门。前 49 个火种纪元曾有人点火又失败，林砚在第 50 轮后半场接住残火，最终成为所有宇宙至强但不称王，将百年协议改写为薪火公约，把大道拆成公共火种。",
  "chapters": []
}
```

Every chapter outline must contain these exact labels:

- `核心事件：`
- `修仙推进：`
- `人物推进：`
- `高层/伏笔：`
- `后果钩子：`

## 80-Arc Map Contract

Author `outline-2400-arc-map.json` with these contiguous ranges and arc titles. Parts 1-6 use 30 chapters per arc, Part 7 uses 35 chapters per arc, and Part 8 uses 25 chapters per arc.

| Arc | Part | Chapters | Title |
| ---: | ---: | --- | --- |
| 1 | 1 | 1-30 | 裁员当天，仙缘刷屏 |
| 2 | 1 | 31-60 | 网贷灰线与相亲局 |
| 3 | 1 | 61-90 | 第一桶灵气钱 |
| 4 | 1 | 91-120 | 炼气流量池 |
| 5 | 1 | 121-150 | 修行 MCN |
| 6 | 1 | 151-180 | 第一批火种 |
| 7 | 1 | 181-210 | 气运反噬 |
| 8 | 1 | 211-240 | 母亲守命门 |
| 9 | 1 | 241-270 | 城市代理祁照 |
| 10 | 1 | 271-300 | 不签命门 |
| 11 | 2 | 301-330 | 筑基丹是贷款 |
| 12 | 2 | 331-360 | 第一座小阵 |
| 13 | 2 | 361-390 | 互助组变组织 |
| 14 | 2 | 391-420 | 假火种 |
| 15 | 2 | 421-450 | 外门任务 |
| 16 | 2 | 451-480 | 城市灵脉局 |
| 17 | 2 | 481-510 | 立基反噬 |
| 18 | 2 | 511-540 | 火种规则 |
| 19 | 2 | 541-570 | 城市代理战 |
| 20 | 2 | 571-600 | 火种不归宗 |
| 21 | 3 | 601-630 | 外门黑名单 |
| 22 | 3 | 631-660 | 师徒债 |
| 23 | 3 | 661-690 | 传法资格 |
| 24 | 3 | 691-720 | 金丹拍卖会 |
| 25 | 3 | 721-750 | 假丹害人 |
| 26 | 3 | 751-780 | 我丹不是金丹 |
| 27 | 3 | 781-810 | 丹劫与夺心 |
| 28 | 3 | 811-840 | 宗门论法 |
| 29 | 3 | 841-870 | 前人失败真相 |
| 30 | 3 | 871-900 | 结我不称师 |
| 31 | 4 | 901-930 | 梦里有人喊救命 |
| 32 | 4 | 931-960 | 元神出窍 |
| 33 | 4 | 961-990 | 拜我者安 |
| 34 | 4 | 991-1020 | 夺舍公司 |
| 35 | 4 | 1021-1050 | 旧王复苏 |
| 36 | 4 | 1051-1080 | 梦境城市 |
| 37 | 4 | 1081-1110 | 元神不是神 |
| 38 | 4 | 1111-1140 | 记忆审判 |
| 39 | 4 | 1141-1170 | 火种梦网 |
| 40 | 4 | 1171-1200 | 出元神不受拜 |
| 41 | 5 | 1201-1230 | 跨城求助 |
| 42 | 5 | 1231-1260 | 众念入身 |
| 43 | 5 | 1261-1290 | 节点自治 |
| 44 | 5 | 1291-1320 | 假群众 |
| 45 | 5 | 1321-1350 | 妖魔欲潮 |
| 46 | 5 | 1351-1380 | 群众不是神 |
| 47 | 5 | 1381-1410 | 众念分流阵 |
| 48 | 5 | 1411-1440 | 跨城大会 |
| 49 | 5 | 1441-1470 | 化众念劫 |
| 50 | 5 | 1471-1500 | 星火过江河 |
| 51 | 6 | 1501-1530 | 魔法交换生 |
| 52 | 6 | 1531-1560 | 真名贷款 |
| 53 | 6 | 1561-1590 | 算道接口 |
| 54 | 6 | 1591-1620 | 身体协议 |
| 55 | 6 | 1621-1650 | 妖魔同桌 |
| 56 | 6 | 1651-1680 | 血脉不是命 |
| 57 | 6 | 1681-1710 | 星海评级 |
| 58 | 6 | 1711-1740 | 文明债券 |
| 59 | 6 | 1741-1770 | 万道互译 |
| 60 | 6 | 1771-1800 | 合万道不吞万道 |
| 61 | 7 | 1801-1835 | 平衡世界来信 |
| 62 | 7 | 1836-1870 | 无签字权者 |
| 63 | 7 | 1871-1905 | 越界试炼 |
| 64 | 7 | 1906-1940 | 平衡法庭 |
| 65 | 7 | 1941-1975 | 协议旧账 |
| 66 | 7 | 1976-2010 | 多维观测者 |
| 67 | 7 | 2011-2045 | 越界劫 |
| 68 | 7 | 2046-2080 | 代理人战争 |
| 69 | 7 | 2081-2115 | 第一张众生席 |
| 70 | 7 | 2116-2150 | 协议未改，桌已裂 |
| 71 | 8 | 2151-2175 | 天道化身 |
| 72 | 8 | 2176-2200 | 万道王座 |
| 73 | 8 | 2201-2225 | 众生席被毁 |
| 74 | 8 | 2226-2250 | 道外劫起 |
| 75 | 8 | 2251-2275 | 诸天失衡 |
| 76 | 8 | 2276-2300 | 道外不是神 |
| 77 | 8 | 2301-2325 | 拆大道 |
| 78 | 8 | 2326-2350 | 薪火公约 |
| 79 | 8 | 2351-2375 | 不称王 |
| 80 | 8 | 2376-2400 | 世界大同 |

## Task 1: Create Builder And Validator Scripts

**Files:**
- Create: `scripts/novel/build-outline.mjs`
- Create: `scripts/novel/validate-outline.mjs`

- [ ] **Step 1: Create the script directory**

Run:

```bash
mkdir -p scripts/novel
```

Expected: command exits with status 0.

- [ ] **Step 2: Create `build-outline.mjs`**

Write this exact file:

```js
import fs from "node:fs";
import path from "node:path";

const workDir = process.argv[2];
if (!workDir) {
  console.error("Usage: node scripts/novel/build-outline.mjs <work-dir>");
  process.exit(2);
}

const arcMapPath = path.join(workDir, "outline-2400-arc-map.json");
const batchDir = path.join(workDir, "batches");
const outPath = path.join(workDir, "outline-2400.json");

function isObject(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function expectedArcContract(arcNum) {
  if (arcNum >= 1 && arcNum <= 60) {
    const start = (arcNum - 1) * 30 + 1;
    return { part: Math.ceil(arcNum / 10), start, end: start + 29 };
  }
  if (arcNum >= 61 && arcNum <= 70) {
    const start = 1801 + (arcNum - 61) * 35;
    return { part: 7, start, end: start + 34 };
  }
  if (arcNum >= 71 && arcNum <= 80) {
    const start = 2151 + (arcNum - 71) * 25;
    return { part: 8, start, end: start + 24 };
  }
  return null;
}

const arcMap = JSON.parse(fs.readFileSync(arcMapPath, "utf8"));
if (!isObject(arcMap)) {
  throw new Error("outline-2400-arc-map.json must be an object");
}
if (!Array.isArray(arcMap.arcs)) {
  throw new Error("outline-2400-arc-map.json must contain arcs[]");
}
if (arcMap.arcs.length !== 80) {
  throw new Error(`outline-2400-arc-map.json: arcs.length=${arcMap.arcs.length}, expected 80`);
}

function requireInteger(value, label) {
  if (!Number.isInteger(value)) {
    throw new Error(`${label} must be an integer`);
  }
}

let expectedStart = 1;
for (let i = 0; i < arcMap.arcs.length; i += 1) {
  const arc = arcMap.arcs[i];
  if (!arc || typeof arc !== "object" || Array.isArray(arc)) {
    throw new Error(`arc index ${i} must be an object`);
  }

  requireInteger(arc.arc, `arc index ${i} arc`);
  requireInteger(arc.part, `arc ${arc.arc} part`);
  requireInteger(arc.start, `arc ${arc.arc} start`);
  requireInteger(arc.end, `arc ${arc.arc} end`);

  const expectedArc = i + 1;
  const contract = expectedArcContract(arc.arc);
  if (arc.arc !== expectedArc) {
    throw new Error(`arc index ${i} has arc=${arc.arc}, expected ${expectedArc}`);
  }
  if (!contract) {
    throw new Error(`arc index ${i} has arc=${arc.arc}, expected 1..80`);
  }
  if (arc.part !== contract.part) {
    throw new Error(`arc ${arc.arc} has part=${arc.part}, expected ${contract.part}`);
  }
  if (arc.start !== contract.start) {
    throw new Error(`arc ${arc.arc} starts at ${arc.start}, expected ${contract.start}`);
  }
  if (arc.end !== contract.end) {
    throw new Error(`arc ${arc.arc} ends at ${arc.end}, expected ${contract.end}`);
  }
  if (arc.end < arc.start) {
    throw new Error(`arc ${arc.arc} has invalid range ${arc.start}-${arc.end}`);
  }
  expectedStart = arc.end + 1;
}
if (expectedStart !== 2401) {
  throw new Error(`arc map ends at ${expectedStart - 1}, expected 2400`);
}

const chapters = [];
for (const arc of arcMap.arcs) {
  const batchPath = path.join(batchDir, `arc-${String(arc.arc).padStart(3, "0")}.json`);
  const batch = JSON.parse(fs.readFileSync(batchPath, "utf8"));
  if (!isObject(batch)) {
    throw new Error(`${batchPath}: batch must be an object`);
  }
  if (batch.arc !== arc.arc) {
    throw new Error(`${batchPath}: arc=${batch.arc}, expected ${arc.arc}`);
  }
  if (!Array.isArray(batch.chapters)) {
    throw new Error(`${batchPath}: chapters[] missing`);
  }
  const expectedCount = arc.end - arc.start + 1;
  if (batch.chapters.length !== expectedCount) {
    throw new Error(`${batchPath}: ${batch.chapters.length} chapters, expected ${expectedCount}`);
  }
  for (let num = arc.start; num <= arc.end; num += 1) {
    const chapter = batch.chapters[num - arc.start];
    if (!chapter || chapter.num !== num) {
      throw new Error(`${batchPath}: chapter index ${num - arc.start} has num=${chapter?.num}, expected ${num}`);
    }
    chapters.push(chapter);
  }
}
if (chapters.length !== 2400) {
  throw new Error(`merged chapters length=${chapters.length}, expected 2400`);
}

const payload = {
  title: arcMap.title || "被逼急了，只好修仙",
  core_prompt: arcMap.core_prompt,
  story_synopsis: arcMap.story_synopsis,
  chapters,
};

fs.writeFileSync(outPath, `${JSON.stringify(payload, null, 2)}\n`);
console.log(JSON.stringify({
  output: outPath,
  chapters: chapters.length,
  first: chapters[0]?.title,
  last: chapters[chapters.length - 1]?.title,
}, null, 2));
```

- [ ] **Step 3: Create `validate-outline.mjs`**

Write this exact file:

```js
import fs from "node:fs";

const outlinePath = process.argv[2];
const arcMapPath = process.argv[3];
if (!outlinePath || !arcMapPath) {
  console.error("Usage: node scripts/novel/validate-outline.mjs <outline-json> <arc-map-json>");
  process.exit(2);
}

const errors = [];
const warnings = [];

function readJson(filePath, label) {
  try {
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch (error) {
    errors.push(`${label} could not be read or parsed: ${error.message}`);
    return undefined;
  }
}

const outline = readJson(outlinePath, "outline");
const arcMap = readJson(arcMapPath, "arc map");

function isObject(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function expectedArcContract(arcNum) {
  if (arcNum >= 1 && arcNum <= 60) {
    const start = (arcNum - 1) * 30 + 1;
    return { part: Math.ceil(arcNum / 10), start, end: start + 29 };
  }
  if (arcNum >= 61 && arcNum <= 70) {
    const start = 1801 + (arcNum - 61) * 35;
    return { part: 7, start, end: start + 34 };
  }
  if (arcNum >= 71 && arcNum <= 80) {
    const start = 2151 + (arcNum - 71) * 25;
    return { part: 8, start, end: start + 24 };
  }
  return null;
}

function requireString(value, label, minLength) {
  if (typeof value !== "string" || value.trim().length < minLength) {
    errors.push(`${label} must be a string with at least ${minLength} characters`);
  }
}

function requireInteger(value, label) {
  if (!Number.isInteger(value)) {
    errors.push(`${label} must be an integer`);
    return false;
  }
  return true;
}

function chapterText(chapter) {
  if (!isObject(chapter)) {
    return "";
  }
  return `${typeof chapter.title === "string" ? chapter.title : ""}\n${typeof chapter.outline === "string" ? chapter.outline : ""}`;
}

if (!isObject(outline)) {
  errors.push("outline must be an object");
} else {
  requireString(outline.title, "title", 1);
  requireString(outline.core_prompt, "core_prompt", 80);
  requireString(outline.story_synopsis, "story_synopsis", 160);

  if (!Array.isArray(outline.chapters)) {
    errors.push("chapters must be an array");
  } else {
    if (outline.chapters.length !== 2400) {
      errors.push(`chapters.length=${outline.chapters.length}, expected 2400`);
    }
    const seen = new Set();
    const requiredLabels = ["核心事件：", "修仙推进：", "人物推进：", "高层/伏笔：", "后果钩子："];
    for (let i = 0; i < outline.chapters.length; i += 1) {
      const chapter = outline.chapters[i];
      const expectedNum = i + 1;
      if (!isObject(chapter)) {
        errors.push(`chapter ${expectedNum} must be an object`);
        continue;
      }
      if (chapter.num !== expectedNum) {
        errors.push(`chapter index ${i} has num=${chapter.num}, expected ${expectedNum}`);
      }
      if (seen.has(chapter.num)) {
        errors.push(`duplicate chapter num ${chapter.num}`);
      }
      seen.add(chapter.num);
      requireString(chapter.title, `chapter ${expectedNum} title`, 1);
      requireString(chapter.outline, `chapter ${expectedNum} outline`, 180);
      for (const label of requiredLabels) {
        if (typeof chapter.outline !== "string" || !chapter.outline.includes(label)) {
          errors.push(`chapter ${expectedNum} outline missing label ${label}`);
        }
      }
    }
  }
}

if (!isObject(arcMap)) {
  errors.push("arc map must be an object");
} else {
  if (!Array.isArray(arcMap.arcs)) {
    errors.push("arc map must contain arcs[]");
  } else {
    if (arcMap.arcs.length !== 80) {
      errors.push(`arc count=${arcMap.arcs.length}, expected 80`);
    }
    let expectedStart = 1;
    for (let i = 0; i < arcMap.arcs.length; i += 1) {
      const arc = arcMap.arcs[i];
      const expectedArc = i + 1;
      if (!isObject(arc)) {
        errors.push(`arc index ${i} must be an object`);
        continue;
      }

      const arcLabel = Number.isInteger(arc.arc) ? `arc ${arc.arc}` : `arc index ${i}`;
      const hasArc = requireInteger(arc.arc, `${arcLabel} arc`);
      const hasPart = requireInteger(arc.part, `${arcLabel} part`);
      const hasStart = requireInteger(arc.start, `${arcLabel} start`);
      const hasEnd = requireInteger(arc.end, `${arcLabel} end`);
      const contract = hasArc ? expectedArcContract(arc.arc) : null;

      if (hasArc) {
        if (arc.arc < 1 || arc.arc > 80) {
          errors.push(`arc index ${i} has arc=${arc.arc}, expected 1..80`);
        }
        if (arc.arc !== expectedArc) {
          errors.push(`arc index ${i} has arc=${arc.arc}, expected ${expectedArc}`);
        }
        if (contract) {
          if (hasPart && arc.part !== contract.part) {
            errors.push(`arc ${arc.arc} has part=${arc.part}, expected ${contract.part}`);
          }
          if (hasStart && arc.start !== contract.start) {
            errors.push(`arc ${arc.arc} starts at ${arc.start}, expected ${contract.start}`);
          }
          if (hasEnd && arc.end !== contract.end) {
            errors.push(`arc ${arc.arc} ends at ${arc.end}, expected ${contract.end}`);
          }
        }
      }
      if (hasStart && arc.start !== expectedStart) {
        errors.push(`${arcLabel} starts at ${arc.start}, expected ${expectedStart}`);
      }
      if (hasStart && hasEnd) {
        if (arc.end < arc.start) {
          errors.push(`${arcLabel} has invalid range ${arc.start}-${arc.end}`);
        } else {
          expectedStart = arc.end + 1;
        }
      }
      requireString(arc.title, `${arcLabel} title`, 2);
      for (const key of ["cultivation", "ground_line", "high_line", "burden_bearer", "end_state"]) {
        requireString(arc[key], `${arcLabel} ${key}`, 4);
      }
    }
    if (expectedStart !== 2401) {
      errors.push(`arc map ends at ${expectedStart - 1}, expected 2400`);
    }
  }
}

const chapters = isObject(outline) && Array.isArray(outline.chapters) ? outline.chapters : [];
const allText = chapters.length > 0
  ? chapters.map(chapterText).join("\n")
  : "";
const requiredAnchors = [
  "房租", "裁员", "医院", "网贷", "短视频", "识气", "立基", "结我丹", "出元神",
  "化众念", "合万道", "越界", "道外", "火种纪元", "百年协议", "薪火公约",
  "魔法", "科技", "妖魔", "星海", "平衡世界", "多维", "不称王"
];
for (const anchor of requiredAnchors) {
  const count = allText.split(anchor).length - 1;
  if (count === 0) {
    errors.push(`required anchor "${anchor}" never appears`);
  }
}

const partRanges = [
  [1, 300, "识气"],
  [301, 600, "立基"],
  [601, 900, "结我丹"],
  [901, 1200, "出元神"],
  [1201, 1500, "化众念"],
  [1501, 1800, "合万道"],
  [1801, 2150, "越界"],
  [2151, 2400, "道外"],
];
if (chapters.length > 0) {
  for (const [start, end, anchor] of partRanges) {
    const partText = chapters.slice(start - 1, end).map(chapterText).join("\n");
    if (!partText.includes(anchor)) {
      errors.push(`part ${start}-${end} does not mention "${anchor}"`);
    }
  }
}

const lengths = chapters
  .filter((chapter) => isObject(chapter) && typeof chapter.outline === "string")
  .map((chapter) => [...chapter.outline].length);
const outlineLength = { min: null, max: null, avg: null };
if (lengths.length > 0) {
  outlineLength.min = Math.min(...lengths);
  outlineLength.max = Math.max(...lengths);
  outlineLength.avg = Math.round(lengths.reduce((sum, value) => sum + value, 0) / lengths.length);
  if (outlineLength.min < 180) warnings.push(`shortest outline has ${outlineLength.min} chars`);
  if (outlineLength.max > 1200) warnings.push(`longest outline has ${outlineLength.max} chars`);
}

console.log(JSON.stringify({
  outline: outlinePath,
  chapters: chapters.length,
  outline_length: outlineLength,
  warnings,
  errors,
}, null, 2));

if (errors.length > 0) {
  process.exit(1);
}
```

- [ ] **Step 4: Commit scripts**

Run:

```bash
git add scripts/novel/build-outline.mjs scripts/novel/validate-outline.mjs
git commit -m "chore: add novel outline build and validation scripts"
```

Expected: commit succeeds and includes only the two script files.

## Task 2: Create The 80-Arc Map

**Files:**
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-arc-map.json`

- [ ] **Step 1: Create artifact directories**

Run:

```bash
mkdir -p docs/novel/outlines/bi-ji-ji-le-2400/batches
```

Expected: command exits with status 0.

- [ ] **Step 2: Author the arc map JSON**

Create `outline-2400-arc-map.json` using the 80 ranges in the “80-Arc Map Contract”. Every arc object must include `arc`, `part`, `start`, `end`, `title`, `cultivation`, `ground_line`, `high_line`, `burden_bearer`, and `end_state`.

Use this exact top-level metadata:

```json
{
  "title": "被逼急了，只好修仙",
  "chapter_count": 2400,
  "core_prompt": "长篇都市硬修仙群像反收割小说。林砚从第 50 火种纪元第 50 年入局，前两部不知道纪元真相，第三部揭示第五十轮后半场。主线坚持修仙感知、千门识局、三十六计破局、员选式分析、易经思维、群众路线，并最终走向道外不称王、薪火公约与世界大同。",
  "story_synopsis": "林砚被房租、裁员、医院、网贷、物业、相亲和老板压榨逼到墙角，通过短视频误入修仙。他先用识气搞钱救家人，随后发现修仙平台、宗门、香火、魔法、科技、妖魔、外星文明、平衡世界和多维宇宙都在争夺众生命门。前 49 个火种纪元曾有人点火又失败，林砚在第 50 轮后半场接住残火，最终成为所有宇宙至强但不称王，将百年协议改写为薪火公约，把大道拆成公共火种。",
  "arcs": []
}
```

For arc descriptions, use the approved spec as binding context. Encode the hidden-timeline rule explicitly: Parts 1-2 may use phrases like “这一轮”“F50”“后半场样本” as伏笔, but must not let林砚 understand“第 50 火种纪元” before Part 3.

- [ ] **Step 3: Validate arc map shape**

Run:

```bash
node -e 'const fs=require("fs"); const p="docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-arc-map.json"; const m=JSON.parse(fs.readFileSync(p,"utf8")); console.log({arcs:m.arcs.length, first:m.arcs[0], last:m.arcs.at(-1)}); if(m.arcs.length!==80) process.exit(1);'
```

Expected: prints `arcs: 80`, first arc starts at 1, last arc ends at 2400.

- [ ] **Step 4: Commit arc map**

Run:

```bash
git add docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-arc-map.json
git commit -m "docs: add 2400 chapter outline arc map"
```

Expected: commit succeeds and includes only the arc map.

## Task 3: Author 80 Chapter Batch Files

**Files:**
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/batches/arc-001.json` through `docs/novel/outlines/bi-ji-ji-le-2400/batches/arc-080.json`

- [ ] **Step 1: Author each batch from its arc**

For every arc in `outline-2400-arc-map.json`, create the corresponding batch file. The file name must be `arc-NNN.json` where `NNN` is the zero-padded arc number. Each file must contain exactly the chapters in that arc range.

Every chapter must:

- Use sequential `num`.
- Have a concrete chapter title.
- Use the required five labels in `outline`.
- Push at least two of these: plot, cultivation, character choice, high-level伏笔, consequence.
- Preserve the hidden timeline reveal: chapters 1-600 do not let林砚 understand the Fireseed纪元 structure; chapters 661-700 reveal it.
- Keep修仙力量 visible in every arc, not just background.
- Include群像 actions so wins do not all come from林砚.

- [ ] **Step 2: Run local batch range check**

Run:

```bash
node <<'NODE'
const fs = require('fs');
const path = require('path');
const work = 'docs/novel/outlines/bi-ji-ji-le-2400';
const map = JSON.parse(fs.readFileSync(path.join(work, 'outline-2400-arc-map.json'), 'utf8'));
for (const arc of map.arcs) {
  const file = path.join(work, 'batches', `arc-${String(arc.arc).padStart(3, '0')}.json`);
  const batch = JSON.parse(fs.readFileSync(file, 'utf8'));
  const expected = arc.end - arc.start + 1;
  if (batch.arc !== arc.arc) throw new Error(`${file}: wrong arc`);
  if (!Array.isArray(batch.chapters) || batch.chapters.length !== expected) throw new Error(`${file}: wrong count`);
  for (let n = arc.start; n <= arc.end; n++) {
    const chapter = batch.chapters[n - arc.start];
    if (!chapter || chapter.num !== n) throw new Error(`${file}: expected chapter ${n}`);
  }
}
console.log(`checked ${map.arcs.length} batch files`);
NODE
```

Expected: `checked 80 batch files`.

- [ ] **Step 3: Commit batch files**

Run:

```bash
git add docs/novel/outlines/bi-ji-ji-le-2400/batches
git commit -m "docs: add 2400 chapter outline batches"
```

Expected: commit succeeds and includes 80 batch JSON files.

## Task 4: Build And Validate Final Import Payload

**Files:**
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400.json`
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-review.md`

- [ ] **Step 1: Build final JSON**

Run:

```bash
node scripts/novel/build-outline.mjs docs/novel/outlines/bi-ji-ji-le-2400
```

Expected: output reports `"chapters": 2400`, with first and last chapter titles.

- [ ] **Step 2: Validate final JSON**

Run:

```bash
node scripts/novel/validate-outline.mjs \
  docs/novel/outlines/bi-ji-ji-le-2400/outline-2400.json \
  docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-arc-map.json
```

Expected: JSON output contains `"errors": []`.

- [ ] **Step 3: Create review summary**

Run:

```bash
node <<'NODE' > docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-review.md
const fs = require('fs');
const outline = JSON.parse(fs.readFileSync('docs/novel/outlines/bi-ji-ji-le-2400/outline-2400.json','utf8'));
const arcs = JSON.parse(fs.readFileSync('docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-arc-map.json','utf8')).arcs;
console.log('# 《被逼急了，只好修仙》2400 章大纲审阅摘要');
console.log('');
console.log(`总章数：${outline.chapters.length}`);
console.log('');
for (const arc of arcs) {
  const first = outline.chapters[arc.start - 1];
  const last = outline.chapters[arc.end - 1];
  console.log(`## Arc ${String(arc.arc).padStart(3,'0')}：${arc.title}（${arc.start}-${arc.end}）`);
  console.log('');
  console.log(`- 修为阶段：${arc.cultivation}`);
  console.log(`- 地面线：${arc.ground_line}`);
  console.log(`- 高层线：${arc.high_line}`);
  console.log(`- 负重者：${arc.burden_bearer}`);
  console.log(`- 章首：第 ${first.num} 章《${first.title}》`);
  console.log(`- 章尾：第 ${last.num} 章《${last.title}》`);
  console.log('');
}
NODE
```

Expected: review file has one section per arc and reports 2400 total chapters.

- [ ] **Step 4: Spot-check key reveal points**

Run:

```bash
node <<'NODE'
const fs = require('fs');
const outline = JSON.parse(fs.readFileSync('docs/novel/outlines/bi-ji-ji-le-2400/outline-2400.json','utf8'));
for (const n of [1, 50, 300, 600, 661, 700, 900, 1200, 1500, 1800, 2150, 2250, 2350, 2400]) {
  const ch = outline.chapters[n - 1];
  console.log(`第${ch.num}章《${ch.title}》`);
  console.log(ch.outline.split('\\n').slice(0, 2).join('\\n'));
  console.log('');
}
NODE
```

Expected: chapters 1-600 contain伏笔 but not林砚 understanding the Fireseed纪元 structure; chapters 661-700 reveal the 第 50 火种纪元 truth; chapters 2151-2400 move from道外 to薪火公约 and世界大同.

- [ ] **Step 5: Commit final outline artifacts**

Run:

```bash
git add docs/novel/outlines/bi-ji-ji-le-2400/outline-2400.json docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-review.md
git commit -m "docs: add validated 2400 chapter outline"
```

Expected: commit succeeds and includes only the final JSON and review markdown.

## Task 5: Import Outline Through The Go Backend

**Files:**
- Read: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400.json`
- Create: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-import-result.json`

- [ ] **Step 1: Verify server is reachable**

Run:

```bash
curl -sS http://127.0.0.1:48090/api/version
```

Expected: JSON with a `version` field.

- [ ] **Step 2: Select the project and preflight existing outline state**

Run:

```bash
node <<'NODE'
const base = 'http://127.0.0.1:48090';
const project = '被逼急了，只好修仙';
async function req(method, path, body) {
  const res = await fetch(`${base}${path}`, {
    method,
    headers: {'Content-Type':'application/json'},
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error(`${method} ${path} ${res.status}: ${text}`);
  return data;
}
await req('POST', '/api/projects/select', {name: project});
const current = await req('GET', '/api/projects/current');
const outline = await req('GET', '/api/outline/summary');
const locked = (outline.chapters || []).filter((ch) => ['writing', 'review', 'accepted'].includes(ch.status));
console.log(JSON.stringify({current, existing_chapters: (outline.chapters || []).length, locked: locked.length}, null, 2));
if (locked.length > 0) process.exit(1);
NODE
```

Expected: selected project is `被逼急了，只好修仙`; `locked` is 0. Pending chapters are safe to replace.

- [ ] **Step 3: Update project story config to 2400 chapters and import**

Run:

```bash
node <<'NODE' > docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-import-result.json
const fs = require('fs');
const base = 'http://127.0.0.1:48090';
const outlinePath = 'docs/novel/outlines/bi-ji-ji-le-2400/outline-2400.json';
const outline = JSON.parse(fs.readFileSync(outlinePath, 'utf8'));
async function req(method, path, body, timeoutMs = 120000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(`${base}${path}`, {
      method,
      headers: {'Content-Type':'application/json'},
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: controller.signal,
    });
    const text = await res.text();
    const data = text ? JSON.parse(text) : null;
    if (!res.ok) throw new Error(`${method} ${path} ${res.status}: ${text}`);
    return data;
  } finally {
    clearTimeout(timer);
  }
}
await req('POST', '/api/projects/select', {name: '被逼急了，只好修仙'});
const cfg = await req('GET', '/api/config');
cfg.story = {
  ...(cfg.story || {}),
  title: outline.title,
  chapter_count: 2400,
  story_synopsis: outline.story_synopsis,
  type: '都市硬修仙群像反收割诸天道博弈长篇',
  writing_pov: '第三人称限知，主要跟随林砚视角，群像章节可临时贴近对应角色但不得全知剧透',
  writing_style: '剧情紧凑，现实压迫扎根，修仙能力具体落地，群像人物有难处、有选择、有后果。方法论藏进场景、账本、合同、投票、复盘和斗法，不口号化。',
};
await req('PUT', '/api/config', cfg);
const imported = await req('POST', '/api/outline/import', outline, 120000);
const status = await req('GET', '/api/status');
const summary = await req('GET', '/api/outline/summary');
console.log(JSON.stringify({
  imported_phase: imported.phase,
  imported_chapters: imported.chapters?.length,
  status_phase: status.phase,
  status_total_chapters: status.total_chapters,
  summary_total_chapters: summary.total_chapters,
  first: summary.chapters?.[0],
  last: summary.chapters?.[summary.chapters.length - 1],
}, null, 2));
NODE
```

Expected: output JSON reports `imported_chapters: 2400`, `status_total_chapters: 2400`, and first/last chapter objects.

- [ ] **Step 4: Commit import result artifact**

Run:

```bash
git add docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-import-result.json
git commit -m "docs: record 2400 chapter outline import result"
```

Expected: commit succeeds and includes only the import result JSON. The project data under `/Users/caoye/storys` is not part of the repository commit.

## Task 6: Final Verification

**Files:**
- Read: `docs/novel/outlines/bi-ji-ji-le-2400/outline-2400-import-result.json`

- [ ] **Step 1: Verify backend outline count**

Run:

```bash
curl -sS http://127.0.0.1:48090/api/status | jq '{phase,current_chapter,total_chapters,ai_availability}'
```

Expected: `phase` is `outline`, `current_chapter` is `0`, `total_chapters` is `2400`. `ai_availability` may remain unavailable because import does not require the external API.

- [ ] **Step 2: Verify outline summary first and last chapters**

Run:

```bash
curl -sS http://127.0.0.1:48090/api/outline/summary | jq '{title,total_chapters,first:.chapters[0],last:.chapters[-1]}'
```

Expected: title is `被逼急了，只好修仙`; `total_chapters` is `2400`; first and last chapters match `outline-2400.json`.

- [ ] **Step 3: Run repository checks that are unaffected by content import**

Run:

```bash
go test ./...
npm run build --prefix frontend
```

Expected: Go tests pass; frontend build succeeds.

- [ ] **Step 4: Report outcome**

Report to the user:

- The spec used.
- The generated outline artifact path.
- The import result path.
- The project selected.
- The new chapter count.
- Whether app AI remains disabled due to API balance.
- The browser refresh instruction if the UI still shows the old count.

## Self-Review Checklist

- Spec coverage: the plan references the approved master spec and carries forward title, time line,境界体系,八部结构,天道体系,人物原则, and结局原则 into the artifact schemas and validator anchors.
- Import safety: the plan imports only through `/api/outline/import`; it does not edit story data files directly.
- Hidden reveal safety: the plan requires Parts 1-2 to avoid revealing Fireseed纪元 structure and requires the reveal around chapters 661-700.
- API independence: no step calls external model APIs; all generation is Codex-authored and imported through local backend.
- Validation: build and validation scripts enforce 2400 sequential chapters, arc continuity, required labels, and core story anchors.
