<script>
  import { currentPage } from './lib/router.js';
  import { progress, taskRunning, contextPage, toastStore, currentProject } from './lib/stores.js';
  import { connectSSE } from './lib/sse.js';
  import { api } from './lib/api.js';
  import { onMount } from 'svelte';
  import Projects from './pages/Projects.svelte';
  import Config from './pages/Config.svelte';
  import Outline from './pages/Outline.svelte';
  import Writing from './pages/Writing.svelte';
  import Relations from './pages/Relations.svelte';
  import Skills from './pages/Skills.svelte';
  import Foreshadows from './pages/Foreshadows.svelte';
  import ChatPanel from './components/ChatPanel.svelte';
  import ConfirmModal from './components/ConfirmModal.svelte';

  let chatPanel;

  $: $contextPage = $currentPage;

  onMount(async () => {
    connectSSE();
    // Check if a project is already selected
    try {
      const cur = await api('GET', '/api/projects/current');
      if (cur.name) {
        currentProject.set(cur.name);
        try { const p = await api('GET', '/api/progress'); progress.set(p); } catch (e) {}
      }
    } catch (e) {}
  });

  $: phaseNames = { outline: '大纲阶段', writing: '写作阶段' };
  $: phase = $progress ? (phaseNames[$progress.phase] || $progress.phase) : '未开始';
  $: chapterStats = (() => {
    const chs = $progress?.chapters || [];
    if (chs.length === 0) return '';
    const accepted = chs.filter(c => c.status === 'accepted').length;
    return `${accepted}/${chs.length} 章`;
  })();

  async function sendToChat(text) {
    if (chatPanel) await chatPanel.sendMessageToChat(text);
  }

  function backToProjects() {
    currentProject.set(null);
  }
</script>

<div class="flex flex-col h-screen bg-base-300 text-base-content overflow-hidden">
  <!-- Header -->
  <header class="navbar bg-base-200 border-b border-base-content/10 px-6 min-h-[46px] shrink-0 gap-4">
    <span class="text-lg font-semibold">AI 小说生成器</span>
    {#if $currentProject}
      <span class="badge badge-sm badge-outline">{$currentProject}</span>
      <button
        class="btn btn-ghost btn-xs gap-1"
        on:click={backToProjects}
        disabled={$taskRunning}
        title={$taskRunning ? 'AI 任务进行中，暂不能切换项目' : '关闭当前项目，返回项目列表（可切换或新建项目）'}
      >
        ⇄ 切换 / 新建项目
      </button>
      <span class="badge badge-sm" class:badge-primary={$progress}>{phase}</span>
      {#if chapterStats}
        <span class="badge badge-sm badge-ghost">{chapterStats}</span>
      {/if}
      {#if $taskRunning}
        <span class="badge badge-sm badge-warning gap-1">
          <span class="loading loading-spinner loading-xs"></span>
          AI 思考中
        </span>
      {/if}
    {/if}
  </header>

  {#if !$currentProject}
    <!-- Project selection -->
    <main class="flex-1 overflow-y-auto p-6">
      <Projects />
    </main>
  {:else}
    <div class="flex flex-1 overflow-hidden">
      <!-- Left: Nav + Content -->
      <div class="flex flex-col w-1/2 min-w-[320px] border-r border-base-content/10 shrink-0">
        <!-- Nav -->
        <nav class="flex bg-base-200 border-b border-base-content/10 px-3 py-2 shrink-0 gap-1">
          {#each [
            ['config', '⚙️', '配置'],
            ['outline', '📝', '大纲'],
            ['writing', '✍️', '写作'],
            ['foreshadows', '🔗', '伏笔'],
            ['relations', '🕸️', '图谱'],
            ['skills', '🧩', '技能']
          ] as [page, icon, label]}
            <button
              class="btn btn-sm text-sm px-4 gap-1.5 {$currentPage === page ? 'btn-primary' : 'btn-ghost'}"
              on:click={() => window.location.hash = '#' + page}
            >
              <span class="text-xs">{icon}</span>{label}
            </button>
          {/each}
        </nav>

        <!-- Content -->
        <main class="flex-1 overflow-y-auto p-4">
          {#if $currentPage === 'config'}
            <Config {sendToChat} />
          {:else if $currentPage === 'outline'}
            <Outline {sendToChat} />
          {:else if $currentPage === 'writing'}
            <Writing {sendToChat} />
          {:else if $currentPage === 'foreshadows'}
            <Foreshadows />
          {:else if $currentPage === 'relations'}
            <Relations />
          {:else if $currentPage === 'skills'}
            <Skills />
          {/if}
        </main>
      </div>

      <!-- Right: Chat Panel -->
      <div class="flex-1 bg-base-200 overflow-hidden">
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
