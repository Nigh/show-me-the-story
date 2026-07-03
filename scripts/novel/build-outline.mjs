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

const arcMap = JSON.parse(fs.readFileSync(arcMapPath, "utf8"));
if (!Array.isArray(arcMap.arcs)) {
  throw new Error("outline-2400-arc-map.json must contain arcs[]");
}

const chapters = [];
for (const arc of arcMap.arcs) {
  const batchPath = path.join(batchDir, `arc-${String(arc.arc).padStart(3, "0")}.json`);
  const batch = JSON.parse(fs.readFileSync(batchPath, "utf8"));
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
