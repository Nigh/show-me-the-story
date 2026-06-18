<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { currentProject, projects, addToast, showConfirm, taskRunning, progress, config, settings, chatSessions, currentChatSession, projectLanguage } from '../lib/stores.js';
  import { t, setLocale } from '../lib/i18n/index.js';

  let newProjectName = '';
  let newProjectLang = 'zh';
  let creating = false;

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
