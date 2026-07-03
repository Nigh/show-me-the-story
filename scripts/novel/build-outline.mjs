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
