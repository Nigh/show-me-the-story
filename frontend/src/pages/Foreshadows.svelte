<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import {
    progress, taskRunning, addToast, showConfirm,
    foreshadowSuggestions, foreshadowShowSuggestions
  } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  let viewMode = 'list'; // list | timeline | markdown
  let roadmapMarkdown = '';
  let roadmapPath = '';
  let loadingRoadmap = false;

  let showForm = false;
  let editing = null;
  let form = { name: '', description: '', plant_chapter: 1, target_chapter: 0, status: 'planted', resolution: '' };

  $: statusMeta = {
    planted:     { label: $t('fs.status.planted'),     cls: 'badge-info' },
    progressing: { label: $t('fs.status.progressing'), cls: 'badge-warning' },
    resolved:    { label: $t('fs.status.resolved'),    cls: 'badge-success' },
    abandoned:   { label: $t('fs.status.abandoned'),   cls: 'badge-ghost' },
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
  $: outlineReport = $progress?.last_foreshadow_outline_report;

  async function runOutlineCheck() {
    try {
      await api('POST', '/api/foreshadows/outline-check');
      addToast($t('fs.outlineConflict.recheck'), 'info');
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  function gotoOutline() {
    window.location.hash = '#outline';
  }

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
      addToast($t('fs.suggestions.suggestStarted'), 'info');
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
      addToast($t('fs.form.required'), 'error');
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
        addToast($t('fs.form.saved.update'), 'success');
      } else {
        await api('POST', '/api/foreshadows', {
          name: form.name.trim(),
          description: form.description.trim(),
          plant_chapter: form.plant_chapter,
          target_chapter: form.target_chapter,
        });
        addToast($t('fs.form.saved.create'), 'success');
      }
      showForm = false;
      roadmapMarkdown = '';
      await refreshProgress();
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  function deleteForeshadow(fs) {
    showConfirm($t('fs.deleteConfirm', { name: fs.name }), async () => {
      try {
        await api('DELETE', '/api/foreshadows/' + fs.id);
        addToast($t('fs.deleted'), 'success');
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
      addToast($t('fs.suggestions.pickRequired'), 'error');
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
      addToast($t('fs.suggestions.confirmed', { n: payload.length }), 'success');
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
      addToast($t('fs.copy.done'), 'success');
    } catch (e) {
      addToast($t('fs.copy.failed'), 'error');
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
        <h2 class="card-title text-base">{$t('fs.title')}</h2>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={suggestForeshadows}>
            {$t('fs.designAi')}
          </button>
          <button class="btn btn-outline btn-sm" disabled={$taskRunning} on:click={openCreate}>
            {$t('fs.addManual')}
          </button>
          <button class="btn btn-ghost btn-sm" on:click={() => switchView('markdown')}>
            {$t('fs.viewRoadmap')}
          </button>
        </div>
      </div>
      <div class="flex flex-wrap gap-2 text-sm">
        <span class="badge badge-ghost">{$t('fs.stats.total', { n: foreshadows.length })}</span>
        <span class="badge badge-info badge-outline">{$t('fs.stats.active', { n: activeCount })}</span>
        <span class="badge badge-success badge-outline">{$t('fs.stats.resolved', { n: resolvedCount })}</span>
        {#if overdueList.length > 0}
          <span class="badge badge-error">{$t('fs.stats.overdue', { n: overdueList.length })}</span>
        {/if}
      </div>
      <p class="text-xs text-base-content/50">
        {@html $t('fs.hint', { file: '<code class="text-xs">Foreshadows.md</code>' })}
      </p>
    </div>
  </div>

  {#if outlineReport?.has_conflicts}
    <div class="card bg-warning/10 border border-warning/30 shadow-sm">
      <div class="card-body py-4 gap-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h3 class="font-semibold text-warning">{$t('fs.outlineConflict.title')}</h3>
          <div class="flex gap-2">
            <button class="btn btn-ghost btn-xs" disabled={$taskRunning} on:click={runOutlineCheck}>{$t('fs.outlineConflict.recheck')}</button>
            <button class="btn btn-warning btn-xs" disabled={$taskRunning} on:click={gotoOutline}>{$t('fs.outlineConflict.gotoOutline')}</button>
          </div>
        </div>
        {#if outlineReport.summary}
          <p class="text-sm">{$t('fs.outlineConflict.summary')}：{outlineReport.summary}</p>
        {/if}
        <div class="space-y-2 max-h-56 overflow-y-auto text-sm">
          {#each (outlineReport.conflicts || []) as c}
            <div class="rounded-lg bg-base-300/50 p-3">
              <div class="font-medium">#{c.foreshadow_id} {c.foreshadow_name}</div>
              <div class="text-base-content/70 mt-1">{c.description}</div>
              <div class="text-xs text-base-content/50 mt-1">{$t('fs.outlineConflict.suggestedFix')}：{c.suggested_fix}</div>
            </div>
          {/each}
        </div>
      </div>
    </div>
  {/if}

  <!-- AI 建议确认 -->
  {#if $foreshadowShowSuggestions && $foreshadowSuggestions.length > 0}
    <div class="card bg-base-200 border border-primary/30 shadow-sm">
      <div class="card-body py-4 gap-3">
        <h3 class="font-semibold">{$t('fs.suggestions.title', { n: $foreshadowSuggestions.length })}</h3>
        <p class="text-sm text-base-content/60">{$t('fs.suggestions.hint')}</p>
        <div class="space-y-2 max-h-72 overflow-y-auto">
          {#each $foreshadowSuggestions as s, i}
            <label class="flex gap-3 p-3 rounded-lg bg-base-300/50 cursor-pointer">
              <input type="checkbox" class="checkbox checkbox-sm mt-1" bind:checked={s._selected} />
              <div class="min-w-0 flex-1">
                <div class="font-medium">{s.name}</div>
                <div class="text-sm text-base-content/70 mt-1">{s.description}</div>
                <div class="text-xs text-base-content/50 mt-1">
                  {$t('fs.suggestions.line', { plant: s.plant_chapter, target: s.target_chapter })}
                </div>
              </div>
            </label>
          {/each}
        </div>
        <div class="flex gap-2">
          <button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={confirmSuggestions}>{$t('fs.suggestions.adopt')}</button>
          <button class="btn btn-ghost btn-sm" on:click={dismissSuggestions}>{$t('fs.suggestions.dismiss')}</button>
        </div>
      </div>
    </div>
  {/if}

  <!-- 超期告警 -->
  {#if overdueList.length > 0}
    <div class="alert alert-warning py-3 text-sm">
      <div>
        <div class="font-medium">{$t('fs.overdue.title')}</div>
        <ul class="list-disc list-inside mt-1">
          {#each overdueList as fs}
            <li>{$t('fs.overdue.line', { id: fs.id, name: fs.name, target: fs.target_chapter })}</li>
          {/each}
        </ul>
      </div>
    </div>
  {/if}

  <!-- 视图切换 -->
  <div class="tabs tabs-boxed bg-base-200 w-fit">
    <button class="tab tab-sm" class:tab-active={viewMode === 'list'} on:click={() => viewMode = 'list'}>{$t('fs.tabs.list')}</button>
    <button class="tab tab-sm" class:tab-active={viewMode === 'timeline'} on:click={() => viewMode = 'timeline'}>{$t('fs.tabs.timeline')}</button>
    <button class="tab tab-sm" class:tab-active={viewMode === 'markdown'} on:click={() => switchView('markdown')}>{$t('fs.tabs.markdown')}</button>
  </div>

  {#if foreshadows.length === 0}
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body items-center text-center py-12 text-base-content/50">
        <p>{$t('fs.empty.title')}</p>
        <p class="text-sm">{$t('fs.empty.hint')}</p>
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
                <button class="btn btn-ghost btn-xs" disabled={$taskRunning} on:click={() => openEdit(fs)}>{$t('common.edit')}</button>
                <button class="btn btn-ghost btn-xs text-error" disabled={$taskRunning} on:click={() => deleteForeshadow(fs)}>{$t('common.delete')}</button>
              </div>
            </div>
            <p class="text-sm text-base-content/70">{fs.description}</p>
            <div class="text-xs text-base-content/50 flex flex-wrap gap-x-4 gap-y-1">
              <span>{$t('fs.plant', { n: fs.plant_chapter })}</span>
              {#if fs.target_chapter > 0}
                <span>{$t('fs.target', { n: fs.target_chapter })}</span>
              {/if}
            </div>
            {#if fs.events?.length}
              <div class="text-xs mt-1">
                <div class="text-base-content/50 mb-1">{$t('fs.events.title')}</div>
                <ul class="space-y-0.5">
                  {#each fs.events as ev}
                    <li class="text-base-content/70">{$t('fs.events.line', { chapter: ev.chapter, note: ev.note })}</li>
                  {/each}
                </ul>
              </div>
            {/if}
            {#if fs.resolution}
              <div class="text-xs text-success/80">{$t('fs.resolution', { text: fs.resolution })}</div>
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
            <h3 class="font-medium text-sm">{$t('fs.timeline.chapter', { num: row.num })}</h3>
            {#if row.plant.length}
              <div class="text-xs">
                <span class="text-info">{$t('fs.timeline.plant')}</span>
                {#each row.plant as f}
                  <span class="badge badge-sm badge-outline ml-1">#{f.id} {f.name}</span>
                {/each}
              </div>
            {/if}
            {#if row.target.length}
              <div class="text-xs">
                <span class="text-warning">{$t('fs.timeline.target')}</span>
                {#each row.target as f}
                  <span class="badge badge-sm badge-outline ml-1">#{f.id} {f.name}</span>
                {/each}
              </div>
            {/if}
            {#if row.events.length}
              <div class="text-xs space-y-1">
                <span class="text-base-content/50">{$t('fs.timeline.events')}</span>
                {#each row.events as item}
                  <div class="pl-2 text-base-content/70">{$t('fs.timeline.eventLine', { id: item.foreshadow.id, name: item.foreshadow.name, note: item.event.note })}</div>
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
            {#if roadmapPath}{$t('fs.markdown.file', { name: roadmapPath.split('/').pop() })}{/if}
          </span>
          <div class="flex gap-2">
            <button class="btn btn-ghost btn-xs" disabled={loadingRoadmap} on:click={loadRoadmap}>{$t('common.refresh')}</button>
            <button class="btn btn-ghost btn-xs" disabled={!roadmapMarkdown} on:click={copyRoadmap}>{$t('common.copy')}</button>
            <button class="btn btn-ghost btn-xs" disabled={!roadmapMarkdown} on:click={downloadRoadmap}>{$t('common.download')}</button>
          </div>
        </div>
        {#if loadingRoadmap}
          <div class="flex justify-center py-8"><span class="loading loading-spinner loading-md"></span></div>
        {:else}
          <pre class="text-xs whitespace-pre-wrap bg-base-300/50 rounded-lg p-4 max-h-[480px] overflow-y-auto">{roadmapMarkdown || $t('fs.markdown.empty')}</pre>
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- 编辑/创建弹窗 -->
{#if showForm}
  <div class="modal modal-open">
    <div class="modal-box max-w-lg">
      <h3 class="font-bold text-lg">{editing ? $t('fs.form.edit') : $t('fs.form.create')}</h3>
      <div class="form-control gap-3 mt-4">
        <span class="label py-0"><span class="label-text">{$t('fs.form.name')}</span></span>
        <input class="input input-bordered input-sm" bind:value={form.name} disabled={$taskRunning} />
        <span class="label py-0"><span class="label-text">{$t('fs.form.description')}</span></span>
        <textarea class="textarea textarea-bordered text-sm" rows="3" bind:value={form.description} disabled={$taskRunning}></textarea>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <span class="label py-0"><span class="label-text">{$t('fs.form.plant')}</span></span>
            <input type="number" min="1" class="input input-bordered input-sm w-full" bind:value={form.plant_chapter} disabled={$taskRunning} />
          </div>
          <div>
            <span class="label py-0"><span class="label-text">{$t('fs.form.target')}</span></span>
            <input type="number" min="0" class="input input-bordered input-sm w-full" bind:value={form.target_chapter} disabled={$taskRunning} />
          </div>
        </div>
        {#if editing}
          <div>
            <span class="label py-0"><span class="label-text">{$t('fs.form.status')}</span></span>
            <select class="select select-bordered select-sm w-full" bind:value={form.status} disabled={$taskRunning}>
              {#each Object.entries(statusMeta) as [val, meta]}
                <option value={val}>{meta.label}</option>
              {/each}
            </select>
          </div>
          <div>
            <span class="label py-0"><span class="label-text">{$t('fs.form.resolution')}</span></span>
            <input class="input input-bordered input-sm w-full" bind:value={form.resolution} disabled={$taskRunning} />
          </div>
        {/if}
      </div>
      <div class="modal-action">
        <button class="btn btn-ghost btn-sm" on:click={() => showForm = false}>{$t('common.cancel')}</button>
        <button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={saveForm}>{$t('common.save')}</button>
      </div>
    </div>
    <div class="modal-backdrop" on:click={() => showForm = false} on:keydown={() => {}} role="presentation"></div>
  </div>
{/if}
