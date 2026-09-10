<script>
  import { onMount } from 'svelte';
  import logo from '../../../docs/show-me-the-story.webp';
  import { api, apiFetch } from '../lib/api.js';
  import { currentProject, projects, addToast, showConfirm, taskRunning, progress, config, settings, chatSessions, currentChatSession, projectLanguage } from '../lib/stores.js';
  import { t, setLocale } from '../lib/i18n/index.js';

  let newProjectName = '';
  let newProjectLang = 'zh';
  let creating = false;
  let backupBusy = false;
  let restoreName = '';
  let restoreFiles;
  let restoreInput;

  onMount(loadProjects);

  function phaseLabel(p) {
    if (p === 'completed') return $t('projects.phase.completed');
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
    const project = $projects.find((item) => item.name === name);
    if (project?.compatibility !== 'supported') {
      const message = project.compatible_app_line
        ? $t('projects.incompatible.message', { format: project.project_format, line: project.compatible_app_line, version: project.recommended_app_version })
        : $t('projects.incompatible.unknown');
      showConfirm(message, () => {});
      return;
    }
    try {
      await api('POST', '/api/projects/select', { name });
      config.set(null);
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

  async function backupProject(name) {
    backupBusy = true;
    try {
      const response = await apiFetch('/api/projects/' + encodeURIComponent(name) + '/backup');
      const url = URL.createObjectURL(await response.blob());
      const link = document.createElement('a');
      link.href = url;
      link.download = `${name}-${new Date().toISOString().slice(0, 10)}.zip`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) { addToast(e.message, 'error'); }
    finally { backupBusy = false; }
  }

  async function restoreProject() {
    if (!restoreName.trim() || !restoreFiles?.length) return;
    backupBusy = true;
    try {
      await apiFetch('/api/projects/restore?name=' + encodeURIComponent(restoreName.trim()), {
        method: 'POST', headers: { 'Content-Type': 'application/zip' }, body: restoreFiles[0],
      });
      restoreName = '';
      restoreInput.value = '';
      restoreFiles = null;
      await loadProjects();
      addToast($t('projects.restore.done'), 'success');
    } catch (e) { addToast(e.message, 'error'); }
    finally { backupBusy = false; }
  }

  function handleKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      createProject();
    }
  }
</script>

<div class="flex items-center justify-center min-h-[60vh]">
  <div class="w-full max-w-xl space-y-6">
    <!-- Title -->
    <div class="text-center">
      <img src={logo} alt={$t('app.title')} class="w-36 h-36 object-contain mx-auto mb-4" />
      <h2 class="text-2xl font-bold mb-1">{$t('projects.title')}</h2>
      <p class="text-sm text-base-content/65">{$t('projects.subtitle')}</p>
    </div>

    <!-- Create new project -->
    <div class="card bg-base-200">
      <div class="card-body p-4">
        <h3 class="card-title text-base">{$t('projects.create')}</h3>
        <input
          type="text"
          class="input w-full"
          aria-label={$t('projects.create.placeholder')}
          bind:value={newProjectName}
          placeholder={$t('projects.create.placeholder')}
          on:keydown={handleKeydown}
          disabled={creating}
        />
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <span class="text-xs text-base-content/65">{$t('projects.create.lang')}</span>
            <div class="tabs tabs-box tabs-sm" role="tablist">
              <button type="button" role="tab" class="tab tab-sm" class:tab-active={newProjectLang === 'zh'} aria-selected={newProjectLang === 'zh'} disabled={creating} on:click={() => newProjectLang = 'zh'}>中文</button>
              <button type="button" role="tab" class="tab tab-sm" class:tab-active={newProjectLang === 'en'} aria-selected={newProjectLang === 'en'} disabled={creating} on:click={() => newProjectLang = 'en'}>EN</button>
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
        <p class="text-xs text-base-content/60 mt-1">{$t('projects.create.langHint')}</p>
      </div>
    </div>

    <div class="card bg-base-200">
      <div class="card-body p-4">
        <h3 class="card-title text-base">{$t('projects.restore.title')}</h3>
        <p class="text-sm text-base-content/65">{$t('projects.restore.hint')}</p>
        <input class="input w-full" aria-label={$t('projects.restore.name')} placeholder={$t('projects.restore.name')} bind:value={restoreName} disabled={backupBusy || $taskRunning} />
        <input type="file" accept=".zip,application/zip" class="file-input w-full" aria-label={$t('projects.restore.file')} bind:this={restoreInput} bind:files={restoreFiles} disabled={backupBusy || $taskRunning} />
        <button class="btn btn-primary btn-sm self-end" on:click={restoreProject} disabled={backupBusy || $taskRunning || !restoreName.trim() || !restoreFiles?.length}>
          {$t('projects.restore.button')}
        </button>
      </div>
    </div>

    <!-- Project list -->
    <div class="card bg-base-200">
      <div class="card-body p-4">
        <h3 class="card-title text-base">{$t('projects.list')} <span class="text-xs font-normal text-base-content/60">({$projects.length})</span></h3>
        {#if $projects.length === 0}
          <p class="text-sm text-base-content/60 py-4 text-center">{$t('projects.empty')}</p>
        {:else}
          <div class="space-y-1.5">
            {#each $projects as p}
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <!-- svelte-ignore a11y-no-static-element-interactions -->
              <div
                class="flex items-center gap-3 bg-base-300 rounded-lg p-3 transition-colors group {p.compatibility === 'supported' ? 'cursor-pointer hover:bg-base-300/80' : 'opacity-60'}"
                class:ring-1={$currentProject === p.name}
                class:ring-primary={$currentProject === p.name}
                on:click={() => selectProject(p.name)}
              >
                <div class="w-9 h-9 rounded-lg bg-primary/20 text-primary flex items-center justify-center text-sm font-bold shrink-0">
                  {(p.name || '?')[0]}
                </div>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium truncate flex items-center gap-2">
                    <span class="truncate" title={p.name}>{p.name}</span>
                    <span class="badge badge-accent badge-xs uppercase shrink-0">{(p.language || 'zh') === 'en' ? 'EN' : 'ZH'}</span>
                    {#if p.compatibility !== 'supported'}
                      <span class="badge badge-warning badge-xs">{$t('projects.incompatible.badge')}</span>
                    {/if}
                  </div>
                  <div class="text-xs text-base-content/60 truncate">
                    {#if p.compatibility !== 'supported'}
                      {$t('projects.incompatible.hint')}
                    {:else if p.title}
                      {$t('projects.bookTitle', { title: p.title })}
                      {#if p.phase || p.book_status === 'completed'}
                        · {phaseLabel(p.book_status === 'completed' ? 'completed' : p.phase)}
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
                    class="btn btn-error btn-outline btn-xs shrink-0"
                    on:click|stopPropagation={() => deleteProject(p.name)}
                    disabled={$taskRunning}
                  >
                    {$t('common.delete')}
                  </button>
                {/if}
                <button class="btn btn-outline btn-xs shrink-0" on:click|stopPropagation={() => backupProject(p.name)} disabled={backupBusy || $taskRunning || p.compatibility !== 'supported'}>
                  {$t('projects.backup')}
                </button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>
