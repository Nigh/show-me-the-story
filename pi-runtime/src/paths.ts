import { createHash } from "node:crypto";
import { chmod, mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";

export interface RuntimePaths {
  root: string;
  agentDir: string;
  authPath: string;
  modelsPath: string;
  modelsStorePath: string;
  projectsDir: string;
  stagingDir: string;
  rollbackDir: string;
  runDir: string;
  socketPath: string;
}

export interface ProjectPaths {
  projectRoot: string;
  workspace: string;
  projectPiDir: string;
  sessionsDir: string;
  pluginDataDir: string;
  scratchDir: string;
}

export function resolveRuntimePaths(dataDir: string): RuntimePaths {
  const root = resolve(dataDir);
  const agentDir = join(root, "agent");
  const runDir = join(root, "run");
  return {
    root,
    agentDir,
    authPath: join(agentDir, "auth.json"),
    modelsPath: join(agentDir, "models.json"),
    modelsStorePath: join(agentDir, "models-store.json"),
    projectsDir: join(root, "projects"),
    stagingDir: join(root, "staging"),
    rollbackDir: join(root, "rollback"),
    runDir,
    socketPath: join(runDir, "pi.sock"),
  };
}

export function projectId(name: string): string {
  return createHash("sha256")
    .update(name.normalize("NFC"))
    .digest("hex")
    .slice(0, 16);
}

export function projectPaths(paths: RuntimePaths, name: string): ProjectPaths {
  const projectRoot = join(paths.projectsDir, projectId(name));
  const workspace = join(projectRoot, "workspace");
  return {
    projectRoot,
    workspace,
    projectPiDir: join(workspace, ".pi"),
    sessionsDir: join(projectRoot, "sessions"),
    pluginDataDir: join(projectRoot, "plugin-data"),
    scratchDir: join(projectRoot, "scratch"),
  };
}

async function ensurePrivateDirectory(path: string): Promise<void> {
  await mkdir(path, { recursive: true, mode: 0o700 });
  await chmod(path, 0o700);
}

export async function ensureRuntimePaths(paths: RuntimePaths): Promise<void> {
  for (const path of [
    paths.root,
    paths.agentDir,
    paths.projectsDir,
    paths.stagingDir,
    paths.rollbackDir,
    paths.runDir,
  ]) {
    await ensurePrivateDirectory(path);
  }
}

export async function ensureProjectPaths(paths: ProjectPaths): Promise<void> {
  for (const path of [
    paths.projectRoot,
    paths.workspace,
    paths.projectPiDir,
    paths.sessionsDir,
    paths.pluginDataDir,
    paths.scratchDir,
  ]) {
    await ensurePrivateDirectory(path);
  }
}
