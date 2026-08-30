import { mkdtemp, readFile, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { readJsonIfExists, writeJsonAtomic } from "./files.js";

describe("private JSON files", () => {
  it("writes JSON atomically with owner-only permissions", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-json-"));
    const path = join(root, "agent", "models.json");

    await writeJsonAtomic(path, { providers: { local: { name: "Local" } } });

    expect(JSON.parse(await readFile(path, "utf8"))).toEqual({
      providers: { local: { name: "Local" } },
    });
    expect((await stat(path)).mode & 0o777).toBe(0o600);
  });

  it("returns undefined only when the file is absent", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-json-"));
    expect(await readJsonIfExists(join(root, "missing.json"))).toBeUndefined();
  });
});
