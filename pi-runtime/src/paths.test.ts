import { chmod, mkdtemp, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, sep } from "node:path";
import { describe, expect, it } from "vitest";
import {
  ensureProjectPaths,
  ensureRuntimePaths,
  projectId,
  projectPaths,
  resolveRuntimePaths,
} from "./paths.js";

describe("Pi data isolation", () => {
  it("keeps every path under the explicit data directory", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-"));
    const paths = resolveRuntimePaths(root);
    await ensureRuntimePaths(paths);
    const project = projectPaths(paths, "../越界/故事");
    await ensureProjectPaths(project);

    for (const path of Object.values({ ...paths, ...project })) {
      expect(path === root || path.startsWith(root + sep)).toBe(true);
    }
    expect(paths.agentDir.includes(".pi")).toBe(false);
    expect(paths.socketPath).toBe(join(root, "run", "pi.sock"));
    expect((await stat(paths.root)).mode & 0o777).toBe(0o700);
    expect((await stat(project.projectPiDir)).mode & 0o777).toBe(0o700);
  });

  it("normalizes project names before hashing", () => {
    expect(projectId("Cafe\u0301")).toBe(projectId("Café"));
    expect(projectId("项目甲")).not.toBe(projectId("项目乙"));
  });

  it("repairs an overly broad root mode", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-"));
    await chmod(root, 0o755);
    await ensureRuntimePaths(resolveRuntimePaths(root));
    expect((await stat(root)).mode & 0o777).toBe(0o700);
  });
});
