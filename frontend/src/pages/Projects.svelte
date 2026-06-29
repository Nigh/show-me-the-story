<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { currentProject, projects, addToast, showConfirm, taskRunning, progress, config, settings, chatSessions, currentChatSession, projectLanguage } from '../lib/stores.js';
  import { t, setLocale } from '../lib/i18n/index.js';

  let newProjectName = '';
  let newProjectLang = 'zh';
  let creating = false;

  // 导入工程相关状态
  let showImportDialog = false;
  let importSourcePath = '';
  let importName = '';
  let importMode = 'copy'; // 'copy' | 'link'
  let importOnConflict = 'error'; // 'error' | 'rename' | 'overwrite'
  let importing = false;

  onMount(loadProjects);

  function phaseLabel(p) {
    if (p === 'outline') return $t('app.phase.outline');
    if (p === 'writing') return $t('app.phase.writing');
    return p || '';
  }

  async function loadProjects() {
    try {
      const list = await api('GET', '/api/projects');
      projects.set(Array.isArray(list) ? list : []);
    } catch (e) {
      projects.set([]);
    }
  }

  async function selectProject(name) {
    try {
      await api('POST', '/api/projects/select', { name });
      currentProject.set(name);
      // Reload all project data
      try { progress.set(await api('GET', '/api/progress')); } catch (e) {}
      try {
        const cfg = await api('GET', '/api/config');
        config.set(cfg);
        if (cfg && cfg.language) {
          projectLanguage.set(cfg.language);
          setLocale(cfg.language);
        }
      } catch (e) {}
      try { settings.set(await api('GET', '/api/settings')); } catch (e) {}
      try { chatSessions.set(await api('GET', '/api/chat/sessions')); } catch (e) {}
      currentChatSession.set(null);
      addToast($t('projects.toast.switched', { name }), 'success');
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  async function createProject() {
    const name = newProjectName.trim();
    if (!name) {
      addToast($t('projects.toast.needName'), 'error');
      return;
    }
    creating = true;
    try {
      await api('POST', '/api/projects', { name, language: newProjectLang });
      newProjectName = '';
      await loadProjects();
      await selectProject(name);
    } catch (e) {
      addToast(e.message, 'error');
    } finally {
      creating = false;
    }
  }

  async function deleteProject(name) {
    showConfirm($t('projects.confirm.delete', { name }), async () => {
      try {
        await api('DELETE', '/api/projects/' + encodeURIComponent(name));
        await loadProjects();
        addToast($t('projects.toast.deleted'), 'success');
      } catch (e) {
        addToast(e.message, 'error');
      }
    });
  }

  function handleKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      createProject();
    }
  }

  function openImportDialog() {
    showImportDialog = true;
    importSourcePath = '';
    importName = '';
    importMode = 'copy';
    importOnConflict = 'error';
  }

  function closeImportDialog() {
    showImportDialog = false;
  }

  async function importProject() {
    if (!importSourcePath.trim()) {
      addToast($t('projects.import.needPath'), 'error');
      return;
    }

    importing = true;
    try {
      const result = await api('POST', '/api/projects/import', {
        source_path: importSourcePath.trim(),
        name: importName.trim() || undefined,
        mode: importMode,
        on_conflict: importOnConflict
      });

      await loadProjects();
      addToast($t('projects.import.success', { name: result.name }), 'success');
      closeImportDialog();

      // 自动切换到导入的工程
      if (result.name) {
        await selectProject(result.name);
      }
    } catch (e) {
      addToast(e.message, 'error');
    } finally {
      importing = false;
    }
  }
</script>

<div class="flex items-center justify-center min-h-[60vh]">
  <div class="w-full max-w-xl space-y-6">
    <!-- Title -->
    <div class="text-center">
      <div class="text-5xl mb-4">📚</div>
      <h2 class="text-2xl font-bold mb-1">{$t('projects.title')}</h2>
      <p class="text-sm text-base-content/50">{$t('projects.subtitle')}</p>
    </div>

    <!-- Create new project -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4">
        <h3 class="card-title text-sm">{$t('projects.create')}</h3>
        <input
          type="text"
          class="input input-sm w-full"
          bind:value={newProjectName}
          placeholder={$t('projects.create.placeholder')}
          on:keydown={handleKeydown}
          disabled={creating}
        />
        <div class="flex items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <span class="text-xs text-base-content/50">{$t('projects.create.lang')}</span>
            <div class="join">
              <button
                type="button"
                class="btn btn-sm join-item {newProjectLang === 'zh' ? 'btn-primary' : 'btn-ghost'}"
                disabled={creating}
                on:click={() => newProjectLang = 'zh'}
              >中文</button>
              <button
                type="button"
                class="btn btn-sm join-item {newProjectLang === 'en' ? 'btn-primary' : 'btn-ghost'}"
                disabled={creating}
                on:click={() => newProjectLang = 'en'}
              >EN</button>
            </div>
          </div>
          <button
            class="btn btn-primary btn-sm"
            on:click={createProject}
            disabled={creating || !newProjectName.trim()}
          >
            {#if creating}
              <span class="loading loading-spinner loading-xs"></span>
            {:else}
              {$t('projects.create.button')}
            {/if}
          </button>
        </div>
        <p class="text-xs text-base-content/40 mt-1">{$t('projects.create.langHint')}</p>
      </div>
    </div>

    <!-- Import project -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4">
        <h3 class="card-title text-sm">{$t('projects.import')}</h3>
        <p class="text-sm text-base-content/70">{$t('projects.import.hint')}</p>
        <div class="card-actions justify-end">
          <button
            class="btn btn-secondary btn-sm"
            on:click={openImportDialog}
            disabled={$taskRunning}
          >
            {$t('projects.import.button')}
          </button>
        </div>
      </div>
    </div>

    <!-- Project list -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4">
        <h3 class="card-title text-sm">{$t('projects.list')} <span class="text-xs font-normal text-base-content/40">({$projects.length})</span></h3>
        {#if $projects.length === 0}
          <p class="text-sm text-base-content/40 py-4 text-center">{$t('projects.empty')}</p>
        {:else}
          <div class="space-y-1.5">
            {#each $projects as p}
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <!-- svelte-ignore a11y-no-static-element-interactions -->
              <div
                class="flex items-center gap-3 bg-base-300 rounded-lg p-3 cursor-pointer hover:bg-base-300/80 transition-colors group"
                class:ring-1={$currentProject === p.name}
                class:ring-primary={$currentProject === p.name}
                on:click={() => selectProject(p.name)}
              >
                <div class="w-9 h-9 rounded-lg bg-primary/20 text-primary flex items-center justify-center text-sm font-bold shrink-0">
                  {(p.name || '?')[0]}
                </div>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium truncate flex items-center gap-2">
                    <span>{p.name}</span>
                    <span class="badge badge-accent badge-xs uppercase">{(p.language || 'zh') === 'en' ? 'EN' : 'ZH'}</span>
                  </div>
                  <div class="text-xs text-base-content/40 truncate">
                    {#if p.title}
                      {$t('projects.bookTitle', { title: p.title })}
                      {#if p.phase}
                        · {phaseLabel(p.phase)}
                      {/if}
                    {:else}
                      {$t('projects.emptyProject')}
                    {/if}
                  </div>
                </div>
                {#if $currentProject === p.name}
                  <span class="badge badge-primary badge-xs">{$t('projects.current')}</span>
                {:else}
                  <button
                    class="btn btn-ghost btn-xs text-error opacity-0 group-hover:opacity-100 transition-opacity"
                    on:click|stopPropagation={() => deleteProject(p.name)}
                    disabled={$taskRunning}
                  >
                    {$t('common.delete')}
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>

{#if showImportDialog}
  <dialog class="modal modal-open" open>
    <div class="modal-box">
      <h3 class="font-bold text-lg">{$t('projects.import.dialog.title')}</h3>
      <div class="py-4 space-y-4">
        <div class="form-control">
          <label class="label">
            <span class="label-text">{$t('projects.import.dialog.sourcePath')}</span>
          </label>
          <input
            type="text"
            bind:value={importSourcePath}
            placeholder="C:\path\to\old\project"
            class="input input-bordered w-full"
            disabled={importing}
          />
        </div>
        <div class="form-control">
          <label class="label">
            <span class="label-text">{$t('projects.import.dialog.projectName')}</span>
          </label>
          <input
            type="text"
            bind:value={importName}
            placeholder={$t('projects.import.dialog.projectNamePlaceholder')}
            class="input input-bordered w-full"
            disabled={importing}
          />
        </div>
        <div class="form-control">
          <label class="label">
            <span class="label-text">{$t('projects.import.dialog.importMode')}</span>
          </label>
          <select bind:value={importMode} class="select select-bordered w-full" disabled={importing}>
            <option value="copy">{$t('projects.import.dialog.modeCopy')}</option>
            <option value="link">{$t('projects.import.dialog.modeLink')}</option>
          </select>
          <label class="label">
            <span class="label-text-alt">
              {#if importMode === 'copy'}
                {$t('projects.import.dialog.modeCopyHint')}
              {:else}
                {$t('projects.import.dialog.modeLinkHint')}
              {/if}
            </span>
          </label>
        </div>
        <div class="form-control">
          <label class="label">
            <span class="label-text">{$t('projects.import.dialog.conflictStrategy')}</span>
          </label>
          <select bind:value={importOnConflict} class="select select-bordered w-full" disabled={importing}>
            <option value="error">{$t('projects.import.dialog.conflictError')}</option>
            <option value="rename">{$t('projects.import.dialog.conflictRename')}</option>
            <option value="overwrite">{$t('projects.import.dialog.conflictOverwrite')}</option>
          </select>
        </div>
      </div>
      <div class="modal-action">
        <button
          class="btn"
          on:click={closeImportDialog}
          disabled={importing}
        >
          {$t('common.cancel')}
        </button>
        <button
          class="btn btn-primary"
          on:click={importProject}
          disabled={importing || !importSourcePath.trim()}
        >
          {#if importing}
            <span class="loading loading-spinner"></span>
          {/if}
          {$t('projects.import.dialog.confirm')}
        </button>
      </div>
    </div>
    <form method="dialog" class="modal-backdrop">
      <button on:click={closeImportDialog} disabled={importing}>close</button>
    </form>
  </dialog>
{/if}
