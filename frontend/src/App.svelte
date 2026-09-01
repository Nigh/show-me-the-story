<script>
  import { currentPage } from './lib/router.js';
  import { progress, taskRunning, contextPage, toastStore, currentProject, projectLanguage, currentChatSession } from './lib/stores.js';
  import { connectSSE } from './lib/sse.js';
  import { api } from './lib/api.js';
  import { onMount, tick } from 'svelte';
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
  import StorageErrorModal from './components/StorageErrorModal.svelte';

  let chatPanel;

  let chatPanelContainer;

  const navItems = [
    ['config', '⚙️', 'nav.config'],
    ['outline', '📝', 'nav.outline'],
    ['writing', '✍️', 'nav.writing'],
    ['foreshadows', '🔗', 'nav.foreshadows'],
    ['memory', '🧠', 'nav.memory'],
    ['relations', '🕸️', 'nav.relations'],
    ['skills', '🧩', 'nav.skills']
  ];

  let mobileNavigationOpen = false;
  let mobileWorkspaceOpen = true;
  let mobileAssistantOpen = false;
  let accordionProject;
  let appVersion = '';
  let latestVersion = '';
  let hasUpdate = false;
  const releasesURL = 'https://github.com/Nigh/show-me-the-story/releases';
  const latestReleaseURL = 'https://github.com/Nigh/show-me-the-story/releases/latest';

  $: $contextPage = $currentPage;
  $: currentNavItem = navItems.find(([page]) => page === $currentPage) || navItems[0];

  // Accordion state is intentionally session-only and starts fresh for each project.
  $: if ($currentProject !== accordionProject) {
    accordionProject = $currentProject;
    mobileNavigationOpen = false;
    mobileWorkspaceOpen = true;
    mobileAssistantOpen = false;
  }

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
    await revealAssistant();
    if (chatPanel) {
      await chatPanel.sendMessageToChat(text);
      await chatPanel.reveal();
    }
  }

  function isMobileLayout() {
    return typeof window !== 'undefined' && window.matchMedia('(max-width: 63.999rem)').matches;
  }

  async function revealAssistant() {
    mobileAssistantOpen = true;
    await tick();
    if (isMobileLayout() && chatPanelContainer) {
      chatPanelContainer.scrollIntoView({ block: 'nearest' });
    }
    if (chatPanel) await chatPanel.reveal();
  }

  async function toggleAssistant() {
    if (mobileAssistantOpen) {
      mobileAssistantOpen = false;
      return;
    }
    await revealAssistant();
  }

  async function toggleWorkspace() {
    mobileWorkspaceOpen = !mobileWorkspaceOpen;
    if (mobileWorkspaceOpen) {
      await tick();
      // Canvas-based pages use the window resize event to remeasure after reveal.
      window.dispatchEvent(new Event('resize'));
    }
  }

  async function navigateTo(page) {
    mobileNavigationOpen = false;
    mobileWorkspaceOpen = true;
    window.location.hash = '#' + page;
    await tick();
    window.dispatchEvent(new Event('resize'));
  }

  function backToProjects() {
    currentProject.set(null);
  }

  function toggleLocale() {
    setLocale($uiLocale === 'en' ? 'zh' : 'en');
  }
</script>

<div class="app-shell flex flex-col bg-base-300 text-base-content overflow-hidden">
  <!-- Header -->
  <header class="app-header navbar bg-base-200 border-b border-base-content/10 px-6 min-h-[46px] shrink-0 gap-4">
    <span class="app-title text-lg font-semibold">{$t('app.title')}</span>
    {#if appVersion}
      <span class="header-meta badge badge-xs badge-ghost font-mono">{appVersion}</span>
    {/if}
    {#if hasUpdate}
      <a href={latestReleaseURL} target="_blank" rel="noopener" class="header-meta badge badge-xs badge-warning gap-0.5 no-underline">
        {$t('app.newVersion')}
      </a>
    {/if}
    {#if $currentProject}
      <span class="header-meta header-project badge badge-sm badge-outline">{$currentProject}</span>
      <span class="header-meta badge badge-sm badge-accent uppercase" title={$projectLanguage === 'en' ? 'English' : '中文'}>
        {$projectLanguage === 'en' ? 'EN' : 'ZH'}
      </span>
      <button
        class="header-meta header-action btn btn-ghost btn-xs gap-1"
        on:click={backToProjects}
        disabled={$taskRunning}
        title={$taskRunning ? $t('app.switchProject.disabled') : $t('app.switchProject.tooltip')}
      >
        {$t('app.switchProject')}
      </button>
      <span class="header-meta badge badge-sm" class:badge-primary={$progress}>{phase}</span>
      {#if chapterStats}
        <span class="header-meta badge badge-sm badge-ghost">{chapterStats}</span>
      {/if}
      {#if $taskRunning}
        <span class="header-meta badge badge-sm badge-warning gap-1">
          <span class="loading loading-spinner loading-xs"></span>
          {$t('app.aiThinking')}
          <TaskTokenBadge className="badge badge-xs badge-warning font-mono border-0" />
        </span>
      {/if}
    {/if}
    <span class="header-spacer flex-1"></span>
    <button
      class="header-locale btn btn-ghost btn-xs gap-1"
      on:click={toggleLocale}
      title={$t('app.uiLang.label')}
    >
      {$uiLocale === 'en' ? $t('app.uiLang.en') : $t('app.uiLang.zh')}
    </button>
  </header>

  {#if !$currentProject}
    <!-- Project selection -->
    <main class="project-main flex-1 overflow-y-auto p-6">
      <Projects />
    </main>
  {:else}
    <div class="workspace-shell flex flex-1 overflow-hidden">
      <button
        type="button"
        class="mobile-section-toggle"
        data-mobile-toggle="navigation"
        aria-expanded={mobileNavigationOpen}
        aria-controls="mobile-navigation-panel"
        title={mobileNavigationOpen ? $t('app.mobile.collapse') : $t('app.mobile.expand')}
        on:click={() => mobileNavigationOpen = !mobileNavigationOpen}
      >
        <span aria-hidden="true">☰</span>
        <span class="font-semibold">{$t('app.mobile.navigation')}</span>
        <span class="mobile-section-summary">{$t(currentNavItem[2])}</span>
        <span class:rotate-180={mobileNavigationOpen} class="mobile-section-chevron" aria-hidden="true">⌄</span>
      </button>

      <!-- Left: vertical nav -->
      <nav
        id="mobile-navigation-panel"
        class="mobile-nav flex flex-col w-44 shrink-0 bg-base-200 border-r border-base-content/10 py-3 px-2 gap-0.5"
        class:mobile-panel-collapsed={!mobileNavigationOpen}
        data-mobile-body="navigation"
        aria-label={$t('app.mobile.navigation')}
      >
        {#each navItems as [page, icon, labelKey]}
          <button
            class="btn btn-sm justify-start w-full gap-2 px-3 text-sm {$currentPage === page ? 'btn-primary font-medium' : 'btn-ghost'}"
            on:click={() => navigateTo(page)}
          >
            <span class="text-xs">{icon}</span>{$t(labelKey)}
          </button>
        {/each}
      </nav>

      <button
        type="button"
        class="mobile-section-toggle"
        data-mobile-toggle="workspace"
        aria-expanded={mobileWorkspaceOpen}
        aria-controls="mobile-workspace-panel"
        title={mobileWorkspaceOpen ? $t('app.mobile.collapse') : $t('app.mobile.expand')}
        on:click={toggleWorkspace}
      >
        <span aria-hidden="true">📖</span>
        <span class="font-semibold">{$t('app.mobile.workspace')}</span>
        <span class="mobile-section-summary">{$t(currentNavItem[2])}</span>
        <span class:rotate-180={mobileWorkspaceOpen} class="mobile-section-chevron" aria-hidden="true">⌄</span>
      </button>

      <!-- Center: page content -->
      <main
        id="mobile-workspace-panel"
        class="mobile-workspace flex-1 min-w-0 overflow-y-auto p-4 border-r border-base-content/10"
        class:mobile-panel-collapsed={!mobileWorkspaceOpen}
        data-mobile-body="workspace"
      >
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

      <button
        type="button"
        class="mobile-section-toggle"
        data-mobile-toggle="assistant"
        aria-expanded={mobileAssistantOpen}
        aria-controls="mobile-assistant-panel"
        title={mobileAssistantOpen ? $t('app.mobile.collapse') : $t('app.mobile.expand')}
        on:click={toggleAssistant}
      >
        <span aria-hidden="true">💬</span>
        <span class="font-semibold">{$t('app.mobile.assistant')}</span>
        <span class="mobile-section-summary">
          {#if $taskRunning}{$t('app.aiThinking')}{:else}{$currentChatSession?.title || $t('chat.session.placeholder')}{/if}
        </span>
        <span class:rotate-180={mobileAssistantOpen} class="mobile-section-chevron" aria-hidden="true">⌄</span>
      </button>

      <!-- Right: Chat Panel -->
      <div
        id="mobile-assistant-panel"
        bind:this={chatPanelContainer}
        class="mobile-chat-panel flex-1 min-w-0 bg-base-200 overflow-hidden"
        class:mobile-panel-collapsed={!mobileAssistantOpen}
        data-mobile-body="assistant"
      >
        <ChatPanel bind:this={chatPanel} contextPage={$currentPage} />
      </div>
    </div>
  {/if}

  <!-- Toasts -->
  <div class="app-toasts fixed top-5 right-5 z-50 flex flex-col gap-2">
    {#each $toastStore as t (t.id)}
      <div class="alert alert-sm {t.type === 'success' ? 'alert-success' : t.type === 'error' ? 'alert-error' : 'alert-info'} toast-enter shadow-lg max-w-sm">
        <span>{t.msg}</span>
      </div>
    {/each}
  </div>

  <ConfirmModal />
  <StorageErrorModal />
</div>
