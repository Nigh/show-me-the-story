import { fileURLToPath } from "node:url";
import { isAbsolute, relative, resolve } from "node:path";
import { SettingsManager } from "@earendil-works/pi-coding-agent";
import { LoginBroker } from "./login-broker.js";
import { createPiModelRuntime, ModelService } from "./models.js";
import { ensureRuntimePaths, resolveRuntimePaths } from "./paths.js";
import { createRuntimeServer, listenOnPrivateSocket } from "./server.js";
import { PiStorySessionFactory, StorySessionRegistry } from "./sessions.js";

export interface RuntimeOptions {
  dataDir: string;
  socketPath: string;
}

export function parseRuntimeOptions(args: string[]): RuntimeOptions {
  let dataDir: string | undefined;
  let socketPath: string | undefined;
  for (let index = 0; index < args.length; index += 1) {
    const option = args[index];
    const value = args[index + 1];
    if (option !== "--data-dir" && option !== "--socket") {
      throw new Error(`unknown runtime option: ${option ?? ""}`);
    }
    if (value === undefined || value.startsWith("--")) {
      throw new Error(`${option} requires an absolute path`);
    }
    if (option === "--data-dir") {
      if (dataDir !== undefined) {
        throw new Error("--data-dir may only be provided once");
      }
      dataDir = value;
    } else {
      if (socketPath !== undefined) {
        throw new Error("--socket may only be provided once");
      }
      socketPath = value;
    }
    index += 1;
  }

  if (dataDir === undefined) {
    throw new Error("--data-dir is required");
  }
  if (socketPath === undefined) {
    throw new Error("--socket is required");
  }
  if (!isAbsolute(dataDir) || !isAbsolute(socketPath)) {
    throw new Error("--data-dir and --socket must be absolute paths");
  }
  return { dataDir: resolve(dataDir), socketPath: resolve(socketPath) };
}

export function validateSocketPath(socketPath: string, runDir: string): string {
  const socket = resolve(socketPath);
  const runtimeDirectory = resolve(runDir);
  const child = relative(runtimeDirectory, socket);
  if (child === "" || child === ".." || child.startsWith(`..${process.platform === "win32" ? "\\" : "/"}`) || isAbsolute(child)) {
    throw new Error("socket path is outside the Pi runtime run directory");
  }
  return socket;
}

export function startLoginSweeper(
  logins: Pick<LoginBroker, "sweepExpired">,
  intervalMs = 60_000,
): () => void {
  const timer = setInterval(() => logins.sweepExpired(), intervalMs);
  timer.unref();
  return () => clearInterval(timer);
}

export async function startRuntime(options: RuntimeOptions): Promise<() => Promise<void>> {
  const paths = resolveRuntimePaths(options.dataDir);
  await ensureRuntimePaths(paths);
  const socketPath = validateSocketPath(options.socketPath, paths.runDir);
  const settings = SettingsManager.create(paths.root, paths.agentDir, { projectTrusted: true });
  const modelRuntime = await createPiModelRuntime(paths);
  const models = new ModelService(modelRuntime, settings, paths);
  const logins = new LoginBroker(modelRuntime);
  const stopLoginSweeper = startLoginSweeper(logins);
  const sessions = new StorySessionRegistry(new PiStorySessionFactory(paths, modelRuntime));
  const server = createRuntimeServer({ models, logins, sessions });

  let closeSocket: (() => Promise<void>) | undefined;
  try {
    closeSocket = await listenOnPrivateSocket(server, socketPath);
  } catch (error) {
    stopLoginSweeper();
    logins.dispose();
    sessions.dispose();
    throw error;
  }

  let closed = false;
  return async () => {
    if (closed) {
      return;
    }
    closed = true;
    const closingSocket = closeSocket!();
    stopLoginSweeper();
    logins.dispose();
    sessions.dispose();
    await closingSocket;
  };
}

async function main(): Promise<void> {
  const shutdown = await startRuntime(parseRuntimeOptions(process.argv.slice(2)));
  let shuttingDown = false;
  const onSignal = () => {
    if (shuttingDown) {
      return;
    }
    shuttingDown = true;
    void shutdown().catch(() => {
      process.exitCode = 1;
    });
  };
  process.once("SIGTERM", onSignal);
  process.once("SIGINT", onSignal);
}

const entryPath = process.argv[1];
if (entryPath !== undefined && resolve(entryPath) === fileURLToPath(import.meta.url)) {
  void main().catch((error: unknown) => {
    const message = error instanceof Error ? error.message : "unknown startup error";
    process.stderr.write(`Pi runtime failed to start: ${message}\n`);
    process.exitCode = 1;
  });
}
