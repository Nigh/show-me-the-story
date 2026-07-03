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
      for (const key of ["title", "cultivation", "ground_line", "high_line", "burden_bearer", "end_state"]) {
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
