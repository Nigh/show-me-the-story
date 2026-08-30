import { open } from "node:fs/promises";
import {
  createAgentSession,
  DefaultResourceLoader,
  type ModelRuntime,
  SessionManager,
  SettingsManager,
  type AgentSession,
} from "@earendil-works/pi-coding-agent";
import { ensureProjectPaths, projectId, projectPaths, type RuntimePaths } from "./paths.js";
import type { SessionSummary } from "./protocol.js";

export interface SessionHandle {
  readonly projectId: string;
  summary(restored: boolean): SessionSummary;
  dispose(): void;
}

export interface StorySessionFactory {
  open(projectName: string): Promise<{ handle: SessionHandle; restored: boolean }>;
}

export class StorySessionRegistry {
  private active?: { handle: SessionHandle; summary: SessionSummary };
  private queue: Promise<void> = Promise.resolve();
  private closed = false;

  constructor(private readonly factory: StorySessionFactory) {}

  select(projectName: string): Promise<SessionSummary> {
    const selection = this.queue.then(async () => {
      if (this.closed) {
        throw new Error("story session registry is closed");
      }
      const id = projectId(projectName);
      if (this.active?.handle.projectId === id) {
        return copySummary(this.active.summary);
      }

      const replacement = await this.factory.open(projectName);
      if (this.closed) {
        replacement.handle.dispose();
        throw new Error("story session registry is closed");
      }
      const summary = replacement.handle.summary(replacement.restored);
      const previous = this.active;
      this.active = { handle: replacement.handle, summary: copySummary(summary) };
      previous?.handle.dispose();
      return copySummary(summary);
    });
    this.queue = selection.then(
      () => undefined,
      () => undefined,
    );
    return selection;
  }

  current(): SessionSummary | undefined {
    return this.active === undefined ? undefined : copySummary(this.active.summary);
  }

  dispose(): void {
    this.closed = true;
    this.active?.handle.dispose();
    this.active = undefined;
  }
}

class PiSessionHandle implements SessionHandle {
  constructor(
    readonly projectId: string,
    private readonly session: AgentSession,
  ) {}

  summary(restored: boolean): SessionSummary {
    return {
      project_id: this.projectId,
      session_id: this.session.sessionId,
      message_count: this.session.messages.length,
      restored,
    };
  }

  dispose(): void {
    this.session.dispose();
  }
}

export class PiStorySessionFactory implements StorySessionFactory {
  constructor(
    private readonly paths: RuntimePaths,
    private readonly modelRuntime: ModelRuntime,
  ) {}

  async open(projectName: string): Promise<{ handle: SessionHandle; restored: boolean }> {
    if (projectName.trim() === "") {
      throw new Error("project name is required");
    }

    const id = projectId(projectName);
    const project = projectPaths(this.paths, projectName);
    await ensureProjectPaths(project);

    const settings = SettingsManager.create(project.workspace, this.paths.agentDir, {
      projectTrusted: true,
    });
    const resources = new DefaultResourceLoader({
      cwd: project.workspace,
      agentDir: this.paths.agentDir,
      settingsManager: settings,
      noExtensions: true,
      noSkills: true,
      noPromptTemplates: true,
      noThemes: true,
      noContextFiles: true,
    });
    await resources.reload();
    assertResourcesDisabled(resources);

    const restored = (await SessionManager.list(project.workspace, project.sessionsDir)).length > 0;
    const sessionManager = restored
      ? SessionManager.continueRecent(project.workspace, project.sessionsDir)
      : await createPersistedSessionManager(project.workspace, project.sessionsDir);
    const { session } = await createAgentSession({
      cwd: project.workspace,
      agentDir: this.paths.agentDir,
      modelRuntime: this.modelRuntime,
      settingsManager: settings,
      resourceLoader: resources,
      sessionManager,
      noTools: "all",
    });

    try {
      if (session.getActiveToolNames().length !== 0) {
        throw new Error("Pi session isolation failed: tools are active");
      }
      assertResourcesDisabled(resources);
    } catch (error) {
      session.dispose();
      throw error;
    }

    return { handle: new PiSessionHandle(id, session), restored };
  }
}

async function createPersistedSessionManager(
  workspace: string,
  sessionsDir: string,
): Promise<SessionManager> {
  const seed = SessionManager.create(workspace, sessionsDir);
  const sessionFile = seed.getSessionFile();
  if (sessionFile === undefined) {
    throw new Error("Pi did not allocate a persistent session path");
  }

  const placeholder = await open(sessionFile, "wx", 0o600);
  await placeholder.close();
  return SessionManager.open(sessionFile, sessionsDir, workspace);
}

function assertResourcesDisabled(resources: DefaultResourceLoader): void {
  if (
    resources.getExtensions().extensions.length !== 0 ||
    resources.getSkills().skills.length !== 0 ||
    resources.getPrompts().prompts.length !== 0 ||
    resources.getThemes().themes.length !== 0 ||
    resources.getAgentsFiles().agentsFiles.length !== 0
  ) {
    throw new Error("Pi session isolation failed: resources are loaded");
  }
}

function copySummary(summary: SessionSummary): SessionSummary {
  return { ...summary };
}
