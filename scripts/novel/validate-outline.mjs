import fs from "node:fs";

const outlinePath = process.argv[2];
const arcMapPath = process.argv[3];
if (!outlinePath || !arcMapPath) {
  console.error("Usage: node scripts/novel/validate-outline.mjs <outline-json> <arc-map-json>");
  process.exit(2);
}

const outline = JSON.parse(fs.readFileSync(outlinePath, "utf8"));
const arcMap = JSON.parse(fs.readFileSync(arcMapPath, "utf8"));
const errors = [];
const warnings = [];

function requireString(value, label, minLength) {
  if (typeof value !== "string" || value.trim().length < minLength) {
    errors.push(`${label} must be a string with at least ${minLength} characters`);
  }
}

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

if (!Array.isArray(arcMap.arcs)) {
  errors.push("arc map must contain arcs[]");
} else {
  if (arcMap.arcs.length !== 80) {
    errors.push(`arc count=${arcMap.arcs.length}, expected 80`);
  }
  let expectedStart = 1;
  for (const arc of arcMap.arcs) {
    if (arc.start !== expectedStart) {
      errors.push(`arc ${arc.arc} starts at ${arc.start}, expected ${expectedStart}`);
    }
    if (arc.end < arc.start) {
      errors.push(`arc ${arc.arc} has invalid range ${arc.start}-${arc.end}`);
    }
    for (const key of ["title", "cultivation", "ground_line", "high_line", "burden_bearer", "end_state"]) {
      requireString(arc[key], `arc ${arc.arc} ${key}`, 4);
    }
    expectedStart = arc.end + 1;
  }
  if (expectedStart !== 2401) {
    errors.push(`arc map ends at ${expectedStart - 1}, expected 2400`);
  }
}

const allText = Array.isArray(outline.chapters)
  ? outline.chapters.map((chapter) => `${chapter.title}\n${chapter.outline}`).join("\n")
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
if (Array.isArray(outline.chapters)) {
  for (const [start, end, anchor] of partRanges) {
    const partText = outline.chapters.slice(start - 1, end).map((chapter) => `${chapter.title}\n${chapter.outline}`).join("\n");
    if (!partText.includes(anchor)) {
      errors.push(`part ${start}-${end} does not mention "${anchor}"`);
    }
  }
}

if (Array.isArray(outline.chapters)) {
  const lengths = outline.chapters.map((chapter) => [...chapter.outline].length);
  const min = Math.min(...lengths);
  const max = Math.max(...lengths);
  const avg = Math.round(lengths.reduce((sum, value) => sum + value, 0) / lengths.length);
  if (min < 180) warnings.push(`shortest outline has ${min} chars`);
  if (max > 1200) warnings.push(`longest outline has ${max} chars`);
  console.log(JSON.stringify({
    outline: outlinePath,
    chapters: outline.chapters.length,
    outline_length: { min, max, avg },
    warnings,
    errors,
  }, null, 2));
}

if (errors.length > 0) {
  process.exit(1);
}
