<script>
  import { currentPage } from './lib/router.js';
  import { progress, taskRunning, contextPage, toastStore, currentProject, projectLanguage } from './lib/stores.js';
  import { connectSSE } from './lib/sse.js';
  import { api } from './lib/api.js';
  import { onMount } from 'svelte';
  import { t, uiLocale, setLocale } from './lib/i18n/index.js';
  import TaskTokenBadge from './components/TaskTokenBadge.svelte';
  import Projects from './pages/Projects.svelte';
  import Config from './pages/Config.svelte';
  import Outline from './pages/Outline.svelte';
  import Writing from './pages/Writing.svelte';
  import Relations from './pages/Relations.svelte';
  import Skills from './pages/Skills.svelte';
  import Foreshadows from './pages/Foreshadows.svelte';
  import Memory from './pages/Memory.svelte';
  import ChatPanel from './components/ChatPanel.svelte';
  import ConfirmModal from './components/ConfirmModal.svelte';

  let chatPanel;

  let appVersion = '';
  let latestVersion = '';
  let hasUpdate = false;
  const releasesURL = 'https://github.com/Nigh/show-me-the-story/releases';
  const latestReleaseURL = 'https://github.com/Nigh/show-me-the-story/releases/latest';

  $: $contextPage = $currentPage;

  onMount(async () => {
    connectSSE();
    // Fetch app version
    try {
      const ver = await api('GET', '/api/version');
      appVersion = ver.version || 'dev';
    } catch (e) {}
    // Check for updates (skip for dev builds)
    if (appVersion && appVersion !== 'dev') {
      try {
        const resp = await fetch('https://api.github.com/repos/Nigh/show-me-the-story/releases/latest');
        if (resp.ok) {
          const data = await resp.json();
          latestVersion = data.tag_name || '';
          if (latestVersion && latestVersion !== appVersion) {
            hasUpdate = true;
          }
        }
      } catch (e) {}
    }
    // Check if a project is already selected
    try {
      const cur = await api('GET', '/api/projects/current');
      if (cur.name) {
        currentProject.set(cur.name);
        if (cur.language) {
          projectLanguage.set(cur.language);
          // First time opening this project this session: align UI with project language.
          // Subsequent toggles persist in localStorage.
          setLocale(cur.language);
        }
        try { const p = await api('GET', '/api/progress'); progress.set(p); } catch (e) {}
      }
    } catch (e) {}
  });

  $: phase = $progress
    ? ($progress.phase === 'outline' ? $t('app.phase.outline')
        : $progress.phase === 'writing' ? $t('app.phase.writing')
        : $progress.phase)
    : $t('app.phase.unstarted');
  $: chapterStats = (() => {
    const chs = $progress?.chapters || [];
    if (chs.length === 0) return '';
    const accepted = chs.filter(c => c.status === 'accepted').length;
    return $t('app.chapters.count', { accepted, total: chs.length });
  })();

  async function sendToChat(text) {
    if (chatPanel) await chatPanel.sendMessageToChat(text);
  }

  function backToProjects() {
    currentProject.set(null);
  }

  function toggleLocale() {
    setLocale($uiLocale === 'en' ? 'zh' : 'en');
  }
</script>

<div class="flex flex-col h-screen bg-base-300 text-base-content overflow-hidden">
  <!-- Header -->
  <header class="navbar bg-base-200 border-b border-base-content/10 px-6 min-h-[46px] shrink-0 gap-4">
    <span class="text-lg font-semibold">{$t('app.title')}</span>
    {#if appVersion}
      <span class="badge badge-xs badge-ghost font-mono">{appVersion}</span>
    {/if}
    {#if hasUpdate}
      <a href={latestReleaseURL} target="_blank" rel="noopener" class="badge badge-xs badge-warning gap-0.5 no-underline">
        {$t('app.newVersion')}
      </a>
    {/if}
    {#if $currentProject}
      <span class="badge badge-sm badge-outline">{$currentProject}</span>
      <span class="badge badge-sm badge-accent uppercase" title={$projectLanguage === 'en' ? 'English' : '中文'}>
        {$projectLanguage === 'en' ? 'EN' : 'ZH'}
      </span>
      <button
        class="btn btn-ghost btn-xs gap-1"
        on:click={backToProjects}
        disabled={$taskRunning}
        title={$taskRunning ? $t('app.switchProject.disabled') : $t('app.switchProject.tooltip')}
      >
        {$t('app.switchProject')}
      </button>
      <span class="badge badge-sm" class:badge-primary={$progress}>{phase}</span>
      {#if chapterStats}
        <span class="badge badge-sm badge-ghost">{chapterStats}</span>
      {/if}
      {#if $taskRunning}
        <span class="badge badge-sm badge-warning gap-1">
          <span class="loading loading-spinner loading-xs"></span>
          {$t('app.aiThinking')}
          <TaskTokenBadge className="badge badge-xs badge-warning font-mono border-0" />
        </span>
      {/if}
    {/if}
    <span class="flex-1"></span>
    <button
      class="btn btn-ghost btn-xs gap-1"
      on:click={toggleLocale}
      title={$t('app.uiLang.label')}
    >
      {$uiLocale === 'en' ? $t('app.uiLang.en') : $t('app.uiLang.zh')}
    </button>
  </header>

  {#if !$currentProject}
    <!-- Project selection -->
    <main class="flex-1 overflow-y-auto p-6">
      <Projects />
    </main>
  {:else}
    <div class="flex flex-1 overflow-hidden">
      <!-- Left: vertical nav -->
      <nav class="flex flex-col w-44 shrink-0 bg-base-200 border-r border-base-content/10 py-3 px-2 gap-0.5">
        {#each [
          ['config', '⚙️', 'nav.config'],
          ['outline', '📝', 'nav.outline'],
          ['writing', '✍️', 'nav.writing'],
          ['foreshadows', '🔗', 'nav.foreshadows'],
          ['memory', '🧠', 'nav.memory'],
          ['relations', '🕸️', 'nav.relations'],
          ['skills', '🧩', 'nav.skills']
        ] as [page, icon, labelKey]}
          <button
            class="btn btn-sm justify-start w-full gap-2 px-3 text-sm {$currentPage === page ? 'btn-primary font-medium' : 'btn-ghost'}"
            on:click={() => window.location.hash = '#' + page}
          >
            <span class="text-xs">{icon}</span>{$t(labelKey)}
          </button>
        {/each}
      </nav>

      <!-- Center: page content -->
      <main class="flex-1 min-w-0 overflow-y-auto p-4 border-r border-base-content/10">
        {#if $currentPage === 'config'}
          <Config {sendToChat} />
        {:else if $currentPage === 'outline'}
          <Outline {sendToChat} />
        {:else if $currentPage === 'writing'}
          <Writing {sendToChat} />
        {:else if $currentPage === 'foreshadows'}
          <Foreshadows />
        {:else if $currentPage === 'memory'}
          <Memory />
        {:else if $currentPage === 'relations'}
          <Relations />
        {:else if $currentPage === 'skills'}
          <Skills />
        {/if}
      </main>

      <!-- Right: Chat Panel -->
      <div class="flex-1 min-w-0 bg-base-200 overflow-hidden">
        <ChatPanel bind:this={chatPanel} contextPage={$currentPage} />
      </div>
    </div>
  {/if}

  <!-- Toasts -->
  <div class="fixed top-5 right-5 z-50 flex flex-col gap-2">
    {#each $toastStore as t (t.id)}
      <div class="alert alert-sm {t.type === 'success' ? 'alert-success' : t.type === 'error' ? 'alert-error' : 'alert-info'} toast-enter shadow-lg max-w-sm">
        <span>{t.msg}</span>
      </div>
    {/each}
  </div>

  <ConfirmModal />
</div>
