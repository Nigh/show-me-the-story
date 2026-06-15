<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import {
    progress, taskRunning, addToast, showConfirm,
    foreshadowSuggestions, foreshadowShowSuggestions
  } from '../lib/stores.js';

  let viewMode = 'list'; // list | timeline | markdown
  let roadmapMarkdown = '';
  let roadmapPath = '';
  let loadingRoadmap = false;

  let showForm = false;
  let editing = null;
  let form = { name: '', description: '', plant_chapter: 1, target_chapter: 0, status: 'planted', resolution: '' };

  const statusMeta = {
    planted:     { label: '已埋设', cls: 'badge-info' },
    progressing: { label: '推进中', cls: 'badge-warning' },
    resolved:    { label: '已回收', cls: 'badge-success' },
    abandoned:   { label: '已放弃', cls: 'badge-ghost' },
  };

  $: foreshadows = $progress?.foreshadows || [];
  $: totalChapters = ($progress?.chapters || []).length;
  $: activeCount = foreshadows.filter(f => f.status === 'planted' || f.status === 'progressing').length;
  $: resolvedCount = foreshadows.filter(f => f.status === 'resolved').length;
  $: currentChapter = ($progress?.current_chapter_index ?? 0) + 1;
  $: overdueList = foreshadows.filter(f =>
    (f.status === 'planted' || f.status === 'progressing') &&
    f.target_chapter > 0 && currentChapter > f.target_chapter
  );

  $: timelineChapters = buildTimeline(foreshadows, totalChapters);

  function buildTimeline(items, chapterCount) {
    const maxFromItems = items.reduce((m, f) => {
      let n = m;
      if (f.plant_chapter > n) n = f.plant_chapter;
      if (f.target_chapter > n) n = f.target_chapter;
      (f.events || []).forEach(ev => { if (ev.chapter > n) n = ev.chapter; });
      return n;
    }, 0);
    const max = Math.max(chapterCount, maxFromItems);
    const rows = [];
    for (let i = 1; i <= max; i++) {
      const row = { num: i, plant: [], target: [], events: [] };
      items.forEach(f => {
        if (f.plant_chapter === i) row.plant.push(f);
        if (f.target_chapter === i) row.target.push(f);
        (f.events || []).forEach(ev => {
          if (ev.chapter === i) row.events.push({ foreshadow: f, event: ev });
        });
      });
      if (row.plant.length || row.target.length || row.events.length) rows.push(row);
    }
    return rows;
  }

  onMount(async () => {
    if ($foreshadowShowSuggestions && $foreshadowSuggestions.length > 0) {
      viewMode = 'list';
    }
  });

  async function loadRoadmap() {
    loadingRoadmap = true;
    try {
      const res = await api('GET', '/api/foreshadows/roadmap');
      roadmapMarkdown = res.markdown || '';
      roadmapPath = res.path || 'Foreshadows.md';
    } catch (e) {
      addToast(e.message, 'error');
    } finally {
      loadingRoadmap = false;
    }
  }

  async function switchView(mode) {
    viewMode = mode;
    if (mode === 'markdown' && !roadmapMarkdown) await loadRoadmap();
  }

  async function refreshProgress() {
    try {
      progress.set(await api('GET', '/api/progress'));
    } catch (e) {}
  }

  async function suggestForeshadows() {
    try {
      await api('POST', '/api/foreshadows/suggest');
      addToast('AI 正在分析大纲并设计伏笔方案…', 'info');
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  function openCreate() {
    editing = null;
    form = { name: '', description: '', plant_chapter: 1, target_chapter: 0, status: 'planted', resolution: '' };
    showForm = true;
  }

  function openEdit(fs) {
    editing = fs;
    form = {
      name: fs.name,
      description: fs.description,
      plant_chapter: fs.plant_chapter || 1,
      target_chapter: fs.target_chapter || 0,
      status: fs.status || 'planted',
      resolution: fs.resolution || '',
    };
    showForm = true;
  }

  async function saveForm() {
    if (!form.name.trim() || !form.description.trim()) {
      addToast('请填写名称和描述', 'error');
      return;
    }
    try {
      if (editing) {
        await api('PUT', '/api/foreshadows/' + editing.id, {
          name: form.name.trim(),
          description: form.description.trim(),
          plant_chapter: form.plant_chapter,
          target_chapter: form.target_chapter,
          status: form.status,
          resolution: form.resolution.trim(),
        });
        addToast('伏笔已更新', 'success');
      } else {
        await api('POST', '/api/foreshadows', {
          name: form.name.trim(),
          description: form.description.trim(),
          plant_chapter: form.plant_chapter,
          target_chapter: form.target_chapter,
        });
        addToast('伏笔已创建', 'success');
      }
      showForm = false;
      roadmapMarkdown = '';
      await refreshProgress();
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  function deleteForeshadow(fs) {
    showConfirm(`确定删除伏笔「${fs.name}」？`, async () => {
      try {
        await api('DELETE', '/api/foreshadows/' + fs.id);
        addToast('伏笔已删除', 'success');
        roadmapMarkdown = '';
        await refreshProgress();
      } catch (e) {
        addToast(e.message, 'error');
      }
    });
  }

  async function confirmSuggestions() {
    const selected = $foreshadowSuggestions.filter(s => s._selected !== false);
    if (selected.length === 0) {
      addToast('请至少选择一条建议', 'error');
      return;
    }
    try {
      const payload = selected.map(s => ({
        name: s.name,
        description: s.description,
        plant_chapter: s.plant_chapter,
        target_chapter: s.target_chapter,
        events: [],
      }));
      await api('POST', '/api/foreshadows/confirm', { foreshadows: payload });
      foreshadowSuggestions.set([]);
      foreshadowShowSuggestions.set(false);
      roadmapMarkdown = '';
      addToast(`已确认 ${payload.length} 条伏笔`, 'success');
      await refreshProgress();
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  function dismissSuggestions() {
    foreshadowSuggestions.set([]);
    foreshadowShowSuggestions.set(false);
  }

  async function copyRoadmap() {
    if (!roadmapMarkdown) await loadRoadmap();
    try {
      await navigator.clipboard.writeText(roadmapMarkdown);
      addToast('路线图已复制到剪贴板', 'success');
    } catch (e) {
      addToast('复制失败', 'error');
    }
  }

  function downloadRoadmap() {
    if (!roadmapMarkdown) return;
    const blob = new Blob([roadmapMarkdown], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'Foreshadows.md';
    a.click();
    URL.revokeObjectURL(url);
  }
</script>

<div class="space-y-4">
  <!-- 统计与操作 -->
  <div class="card bg-base-200 shadow-sm">
    <div class="card-body py-4 gap-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h2 class="card-title text-base">伏笔系统</h2>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={suggestForeshadows}>
            AI 设计伏笔
          </button>
          <button class="btn btn-outline btn-sm" disabled={$taskRunning} on:click={openCreate}>
            手动添加
          </button>
          <button class="btn btn-ghost btn-sm" on:click={() => switchView('markdown')}>
            查看路线图
          </button>
        </div>
      </div>
      <div class="flex flex-wrap gap-2 text-sm">
        <span class="badge badge-ghost">共 {foreshadows.length} 条</span>
        <span class="badge badge-info badge-outline">活跃 {activeCount}</span>
        <span class="badge badge-success badge-outline">已回收 {resolvedCount}</span>
        {#if overdueList.length > 0}
          <span class="badge badge-error">超期 {overdueList.length}</span>
        {/if}
      </div>
      <p class="text-xs text-base-content/50">
        写作时活跃伏笔会自动注入 AI 提示词；每章生成或修订后会更新状态，并同步写入项目目录 <code class="text-xs">Foreshadows.md</code>。
      </p>
    </div>
  </div>

  <!-- AI 建议确认 -->
  {#if $foreshadowShowSuggestions && $foreshadowSuggestions.length > 0}
    <div class="card bg-base-200 border border-primary/30 shadow-sm">
      <div class="card-body py-4 gap-3">
        <h3 class="font-semibold">AI 伏笔建议（{$foreshadowSuggestions.length} 条）</h3>
        <p class="text-sm text-base-content/60">勾选要采纳的方案，确认后将写入项目并开始追踪。</p>
        <div class="space-y-2 max-h-72 overflow-y-auto">
          {#each $foreshadowSuggestions as s, i}
            <label class="flex gap-3 p-3 rounded-lg bg-base-300/50 cursor-pointer">
              <input type="checkbox" class="checkbox checkbox-sm mt-1" bind:checked={s._selected} />
              <div class="min-w-0 flex-1">
                <div class="font-medium">{s.name}</div>
                <div class="text-sm text-base-content/70 mt-1">{s.description}</div>
                <div class="text-xs text-base-content/50 mt-1">
                  埋设第 {s.plant_chapter} 章 → 预计第 {s.target_chapter} 章回收
                </div>
              </div>
            </label>
          {/each}
        </div>
        <div class="flex gap-2">
          <button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={confirmSuggestions}>确认采纳</button>
          <button class="btn btn-ghost btn-sm" on:click={dismissSuggestions}>暂不采纳</button>
        </div>
      </div>
    </div>
  {/if}

  <!-- 超期告警 -->
  {#if overdueList.length > 0}
    <div class="alert alert-warning py-3 text-sm">
      <div>
        <div class="font-medium">以下伏笔已超过预计回收章节：</div>
        <ul class="list-disc list-inside mt-1">
          {#each overdueList as fs}
            <li>#{fs.id} {fs.name}（预计第 {fs.target_chapter} 章）</li>
          {/each}
        </ul>
      </div>
    </div>
  {/if}

  <!-- 视图切换 -->
  <div class="tabs tabs-boxed bg-base-200 w-fit">
    <button class="tab tab-sm" class:tab-active={viewMode === 'list'} on:click={() => viewMode = 'list'}>列表</button>
    <button class="tab tab-sm" class:tab-active={viewMode === 'timeline'} on:click={() => viewMode = 'timeline'}>章节时间线</button>
    <button class="tab tab-sm" class:tab-active={viewMode === 'markdown'} on:click={() => switchView('markdown')}>路线图文档</button>
  </div>

  {#if foreshadows.length === 0}
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body items-center text-center py-12 text-base-content/50">
        <p>尚无伏笔记录</p>
        <p class="text-sm">点击「AI 设计伏笔」根据大纲自动生成方案，或手动添加。</p>
      </div>
    </div>
  {:else if viewMode === 'list'}
    <div class="grid gap-3">
      {#each foreshadows as fs}
        <div class="card bg-base-200 shadow-sm">
          <div class="card-body py-4 gap-2">
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div>
                <span class="text-xs text-base-content/40 mr-2">#{fs.id}</span>
                <span class="font-semibold">{fs.name}</span>
                <span class="badge badge-sm ml-2 {statusMeta[fs.status]?.cls || 'badge-ghost'}">
                  {statusMeta[fs.status]?.label || fs.status}
                </span>
              </div>
              <div class="flex gap-1">
                <button class="btn btn-ghost btn-xs" disabled={$taskRunning} on:click={() => openEdit(fs)}>编辑</button>
                <button class="btn btn-ghost btn-xs text-error" disabled={$taskRunning} on:click={() => deleteForeshadow(fs)}>删除</button>
              </div>
            </div>
            <p class="text-sm text-base-content/70">{fs.description}</p>
            <div class="text-xs text-base-content/50 flex flex-wrap gap-x-4 gap-y-1">
              <span>埋设：第 {fs.plant_chapter} 章</span>
              {#if fs.target_chapter > 0}
                <span>预计回收：第 {fs.target_chapter} 章</span>
              {/if}
            </div>
            {#if fs.events?.length}
              <div class="text-xs mt-1">
                <div class="text-base-content/50 mb-1">进展记录</div>
                <ul class="space-y-0.5">
                  {#each fs.events as ev}
                    <li class="text-base-content/70">第 {ev.chapter} 章：{ev.note}</li>
                  {/each}
                </ul>
              </div>
            {/if}
            {#if fs.resolution}
              <div class="text-xs text-success/80">回收方式：{fs.resolution}</div>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {:else if viewMode === 'timeline'}
    <div class="space-y-3">
      {#each timelineChapters as row}
        <div class="card bg-base-200 shadow-sm">
          <div class="card-body py-3 gap-2">
            <h3 class="font-medium text-sm">第 {row.num} 章</h3>
            {#if row.plant.length}
              <div class="text-xs">
                <span class="text-info">🔵 埋设</span>
                {#each row.plant as f}
                  <span class="badge badge-sm badge-outline ml-1">#{f.id} {f.name}</span>
                {/each}
              </div>
            {/if}
            {#if row.target.length}
              <div class="text-xs">
                <span class="text-warning">🎯 预计回收</span>
                {#each row.target as f}
                  <span class="badge badge-sm badge-outline ml-1">#{f.id} {f.name}</span>
                {/each}
              </div>
            {/if}
            {#if row.events.length}
              <div class="text-xs space-y-1">
                <span class="text-base-content/50">📌 进展</span>
                {#each row.events as item}
                  <div class="pl-2 text-base-content/70">#{item.foreshadow.id} {item.foreshadow.name}：{item.event.note}</div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body py-4 gap-3">
        <div class="flex flex-wrap gap-2 justify-between items-center">
          <span class="text-sm text-base-content/60">
            {#if roadmapPath}文件：{roadmapPath.split('/').pop()}{/if}
          </span>
          <div class="flex gap-2">
            <button class="btn btn-ghost btn-xs" disabled={loadingRoadmap} on:click={loadRoadmap}>刷新</button>
            <button class="btn btn-ghost btn-xs" disabled={!roadmapMarkdown} on:click={copyRoadmap}>复制</button>
            <button class="btn btn-ghost btn-xs" disabled={!roadmapMarkdown} on:click={downloadRoadmap}>下载</button>
          </div>
        </div>
        {#if loadingRoadmap}
          <div class="flex justify-center py-8"><span class="loading loading-spinner loading-md"></span></div>
        {:else}
          <pre class="text-xs whitespace-pre-wrap bg-base-300/50 rounded-lg p-4 max-h-[480px] overflow-y-auto">{roadmapMarkdown || '暂无内容'}</pre>
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- 编辑/创建弹窗 -->
{#if showForm}
  <div class="modal modal-open">
    <div class="modal-box max-w-lg">
      <h3 class="font-bold text-lg">{editing ? '编辑伏笔' : '添加伏笔'}</h3>
      <div class="form-control gap-3 mt-4">
        <label class="label py-0"><span class="label-text">名称</span></label>
        <input class="input input-bordered input-sm" bind:value={form.name} disabled={$taskRunning} />
        <label class="label py-0"><span class="label-text">描述</span></label>
        <textarea class="textarea textarea-bordered text-sm" rows="3" bind:value={form.description} disabled={$taskRunning}></textarea>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="label py-0"><span class="label-text">埋设章节</span></label>
            <input type="number" min="1" class="input input-bordered input-sm w-full" bind:value={form.plant_chapter} disabled={$taskRunning} />
          </div>
          <div>
            <label class="label py-0"><span class="label-text">预计回收章节</span></label>
            <input type="number" min="0" class="input input-bordered input-sm w-full" bind:value={form.target_chapter} disabled={$taskRunning} />
          </div>
        </div>
        {#if editing}
          <div>
            <label class="label py-0"><span class="label-text">状态</span></label>
            <select class="select select-bordered select-sm w-full" bind:value={form.status} disabled={$taskRunning}>
              {#each Object.entries(statusMeta) as [val, meta]}
                <option value={val}>{meta.label}</option>
              {/each}
            </select>
          </div>
          <div>
            <label class="label py-0"><span class="label-text">回收方式</span></label>
            <input class="input input-bordered input-sm w-full" bind:value={form.resolution} disabled={$taskRunning} />
          </div>
        {/if}
      </div>
      <div class="modal-action">
        <button class="btn btn-ghost btn-sm" on:click={() => showForm = false}>取消</button>
        <button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={saveForm}>保存</button>
      </div>
    </div>
    <div class="modal-backdrop" on:click={() => showForm = false} on:keydown={() => {}} role="presentation"></div>
  </div>
{/if}
