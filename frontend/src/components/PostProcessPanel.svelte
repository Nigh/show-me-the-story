<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { postprocess, taskRunning, addToast, confirmModal, progress } from '../lib/stores.js';
  import { renderMarkdown } from '../lib/markdown.js';
  import { t } from '../lib/i18n/index.js';

  $: bookComplete = (() => {
    const chs = $progress?.chapters || [];
    return chs.length > 0 && chs.every(c => c.status === 'accepted' && c.content);
  })();

  $: pp = $postprocess?.state;
  $: opts = pp?.execute_options || { run_smooth_transitions_first: true, include_polish: false };

  let reportTab = 'diagnosis';
  let diffItem = null;
  let roadmapLocal = [];
  let optsLocal = { run_smooth_transitions_first: true, include_polish: false };
  let dirty = false;

  $: typeLabels = {
    logic: $t('pp.type.logic'),
    transition: $t('pp.type.transition'),
    style: $t('pp.type.style'),
    rhythm: $t('pp.type.rhythm'),
    dialogue: $t('pp.type.dialogue'),
    polish: $t('pp.type.polish'),
  };
  $: statusLabels = {
    pending: $t('pp.status.pending'),
    running: $t('pp.status.running'),
    done: $t('pp.status.done'),
    failed: $t('pp.status.failed'),
    skipped: $t('pp.status.skipped'),
  };
  const statusCls = {
    pending: 'badge-ghost', running: 'badge-warning', done: 'badge-success',
    failed: 'badge-error', skipped: 'badge-ghost',
  };

  async function loadPostprocess() {
    try {
      postprocess.set(await api('GET', '/api/postprocess'));
    } catch (e) { /* ignore */ }
  }

  onMount(loadPostprocess);

  $: if (pp?.roadmap && !dirty) {
    roadmapLocal = pp.roadmap.map(r => ({ ...r }));
  }
  $: if (pp?.execute_options) {
    optsLocal = { ...pp.execute_options };
  }

  function markDirty() { dirty = true; }

  function selectAllPending(val) {
    roadmapLocal = roadmapLocal.map(r =>
      r.status === 'pending' ? { ...r, selected: val } : r
    );
    markDirty();
  }

  function resetFailed() {
    roadmapLocal = roadmapLocal.map(r =>
      (r.status === 'failed' || r.status === 'skipped') ? { ...r, status: 'pending', error: '', diff_original: '', diff_revised: '' } : r
    );
    markDirty();
  }

  async function saveRoadmap() {
    try {
      const res = await api('PUT', '/api/postprocess/roadmap', {
        roadmap: roadmapLocal,
        execute_options: optsLocal,
      });
      postprocess.set(res);
      dirty = false;
      addToast($t('pp.toast.saved'), 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }

  function runDiagnose() {
    confirmModal.set({
      message: $t('pp.confirm.diagnose'),
      onConfirm: async () => {
        try {
          await api('POST', '/api/postprocess/diagnose');
          addToast($t('pp.toast.diagnoseStarted'), 'info');
        } catch (e) { addToast(e.message, 'error'); }
      },
    });
  }

  async function runConsistency() {
    try {
      await api('POST', '/api/postprocess/consistency');
      addToast($t('pp.toast.consistencyStarted'), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function runRoadmap() {
    try {
      await api('POST', '/api/postprocess/roadmap');
      addToast($t('pp.toast.roadmapStarted'), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  function runExecute() {
    const pending = roadmapLocal.filter(r => r.selected && r.status === 'pending');
    const chapterCount = new Set(pending.map(r => r.chapter_num)).size;
    if (pending.length === 0) {
      addToast($t('pp.toast.pickRequired'), 'error');
      return;
    }
    const mergeHint = pending.length > chapterCount
      ? $t('pp.confirm.execute.merge', { items: pending.length, chapters: chapterCount })
      : '';
    confirmModal.set({
      message: $t('pp.confirm.execute', { chapters: chapterCount, merge: mergeHint }),
      onConfirm: async () => {
        try {
          if (dirty) await saveRoadmap();
          await api('POST', '/api/postprocess/execute', { execute_options: optsLocal });
          addToast($t('pp.toast.executeStarted'), 'info');
        } catch (e) { addToast(e.message, 'error'); }
      },
    });
  }

  function clearAll() {
    confirmModal.set({
      message: $t('pp.confirm.clear'),
      onConfirm: async () => {
        try {
          const res = await api('DELETE', '/api/postprocess');
          postprocess.set(res);
          addToast($t('pp.toast.cleared'), 'info');
        } catch (e) { addToast(e.message, 'error'); }
      },
    });
  }

  $: diagnosisHtml = pp?.diagnosis_report ? renderMarkdown(pp.diagnosis_report) : '';
  $: consistencyHtml = pp?.consistency_report ? renderMarkdown(pp.consistency_report) : '';
  $: pendingCount = roadmapLocal.filter(r => r.status === 'pending').length;
  $: selectedPending = roadmapLocal.filter(r => r.selected && r.status === 'pending').length;
  $: selectedChapterCount = new Set(
    roadmapLocal.filter(r => r.selected && r.status === 'pending').map(r => r.chapter_num)
  ).size;
</script>

{#if bookComplete}
  <div class="card bg-base-200 shadow-sm">
    <div class="card-body p-4 gap-3">
      <div class="flex items-center gap-2 flex-wrap">
        <h2 class="card-title text-base flex-1">{$t('pp.title')}</h2>
        {#if pp?.bundle_mode}
          <span class="badge badge-sm badge-ghost" title={$t('pp.mode.tooltip')}>
            {pp.bundle_mode === 'summary_only' ? $t('pp.mode.summary') : $t('pp.mode.full')}
          </span>
        {/if}
        {#if pp?.estimated_tokens}
          <span class="text-xs text-base-content/40">{$t('pp.tokens', { n: pp.estimated_tokens.toLocaleString() })}</span>
        {/if}
        {#if pp?.volume_count > 1}
          <span class="text-xs text-base-content/40">{$t('pp.volumes', { n: pp.volume_count })}</span>
        {/if}
      </div>

      <p class="text-xs text-base-content/50">{$t('pp.intro')}</p>

      <div class="flex gap-2 flex-wrap">
        <button class="btn btn-primary btn-sm" on:click={runDiagnose} disabled={$taskRunning}>{$t('pp.btn.diagnose')}</button>
        <button class="btn btn-ghost btn-sm" on:click={runConsistency} disabled={$taskRunning || !pp?.diagnosis_report}>{$t('pp.btn.consistency')}</button>
        <button class="btn btn-ghost btn-sm" on:click={runRoadmap} disabled={$taskRunning || (!pp?.diagnosis_report && !pp?.consistency_report)}>{$t('pp.btn.roadmap')}</button>
        <button class="btn btn-ghost btn-sm btn-error" on:click={clearAll} disabled={$taskRunning}>{$t('pp.btn.clear')}</button>
      </div>

      {#if pp?.diagnosis_report || pp?.consistency_report}
        <div class="tabs tabs-boxed tabs-sm w-fit">
          <button class="tab {reportTab === 'diagnosis' ? 'tab-active' : ''}" on:click={() => reportTab = 'diagnosis'}>{$t('pp.tab.diagnosis')}</button>
          <button class="tab {reportTab === 'consistency' ? 'tab-active' : ''}" on:click={() => reportTab = 'consistency'}>{$t('pp.tab.consistency')}</button>
        </div>
        <div class="bg-base-300 rounded-lg p-3 max-h-64 overflow-y-auto text-sm">
          {#if reportTab === 'diagnosis' && diagnosisHtml}
            <div class="md-body">{@html diagnosisHtml}</div>
          {:else if reportTab === 'consistency' && consistencyHtml}
            <div class="md-body">{@html consistencyHtml}</div>
          {:else}
            <p class="text-base-content/40 text-center py-4">{$t('pp.report.empty')}</p>
          {/if}
        </div>
      {/if}

      {#if roadmapLocal.length > 0}
        <div class="divider my-0 text-xs">{$t('pp.roadmap.title', { total: roadmapLocal.length, pending: pendingCount })}</div>

        <div class="flex gap-3 flex-wrap items-center text-xs">
          <label class="flex items-center gap-1.5 cursor-pointer">
            <input type="checkbox" class="checkbox checkbox-xs" bind:checked={optsLocal.run_smooth_transitions_first} on:change={markDirty} />
            {$t('pp.opts.smoothFirst')}
          </label>
          <label class="flex items-center gap-1.5 cursor-pointer">
            <input type="checkbox" class="checkbox checkbox-xs" bind:checked={optsLocal.include_polish} on:change={markDirty} />
            {$t('pp.opts.includePolish')}
          </label>
          <div class="flex-1"></div>
          <button class="btn btn-ghost btn-xs" on:click={() => selectAllPending(true)} disabled={$taskRunning}>{$t('pp.btn.selectAll')}</button>
          <button class="btn btn-ghost btn-xs" on:click={() => selectAllPending(false)} disabled={$taskRunning}>{$t('pp.btn.selectNone')}</button>
          <button class="btn btn-ghost btn-xs" on:click={resetFailed} disabled={$taskRunning}>{$t('pp.btn.resetFailed')}</button>
          {#if dirty}
            <button class="btn btn-primary btn-xs" on:click={saveRoadmap} disabled={$taskRunning}>{$t('pp.btn.saveRoadmap')}</button>
          {/if}
          <button class="btn btn-success btn-sm" on:click={runExecute} disabled={$taskRunning || selectedPending === 0}>
            {$t('pp.btn.execute', { chapters: selectedChapterCount, items: selectedPending })}
          </button>
        </div>

        <div class="overflow-x-auto max-h-80 overflow-y-auto rounded-lg border border-base-300">
          <table class="table table-xs table-zebra">
            <thead class="sticky top-0 bg-base-200 z-10">
              <tr>
                <th class="w-8"></th>
                <th>{$t('pp.col.chapter')}</th>
                <th>{$t('pp.col.type')}</th>
                <th>{$t('pp.col.priority')}</th>
                <th class="min-w-[200px]">{$t('pp.col.feedback')}</th>
                <th class="min-w-[5.5rem] w-28">{$t('pp.col.status')}</th>
                <th class="w-14 shrink-0"></th>
              </tr>
            </thead>
            <tbody>
              {#each roadmapLocal as item, i}
                <tr>
                  <td>
                    {#if item.status === 'pending'}
                      <input type="checkbox" class="checkbox checkbox-xs" bind:checked={item.selected} on:change={() => { roadmapLocal[i] = item; markDirty(); }} disabled={$taskRunning} />
                    {/if}
                  </td>
                  <td class="whitespace-nowrap">{$t('pp.chapter.label', { n: item.chapter_num })}</td>
                  <td>{typeLabels[item.type] || item.type}</td>
                  <td><span class="badge badge-xs {item.priority === 'P0' ? 'badge-error' : item.priority === 'P1' ? 'badge-warning' : 'badge-ghost'}">{item.priority}</span></td>
                  <td>
                    {#if item.status === 'pending'}
                      <textarea class="textarea textarea-xs w-full min-h-[2.5rem]" bind:value={item.feedback} on:input={markDirty} disabled={$taskRunning}></textarea>
                    {:else}
                      <span class="text-base-content/70 line-clamp-2" title={item.feedback}>{item.feedback}</span>
                    {/if}
                  </td>
                  <td class="align-top min-w-[5.5rem] w-28">
                    <div class="flex flex-col gap-1">
                      <span class="badge badge-xs whitespace-nowrap w-fit {statusCls[item.status] || 'badge-ghost'}">{statusLabels[item.status] || item.status}</span>
                      {#if item.error}
                        <span class="text-error text-[10px] leading-snug break-words" title={item.error}>{item.error}</span>
                      {/if}
                    </div>
                  </td>
                  <td>
                    {#if item.diff_original || item.diff_revised}
                      <button class="btn btn-ghost btn-xs" on:click={() => diffItem = item}>{$t('pp.diff.btn')}</button>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </div>
{/if}

{#if diffItem}
  <dialog class="modal modal-open">
    <div class="modal-box max-w-4xl">
      <h3 class="font-bold text-base mb-2">{$t('pp.diff.title', { n: diffItem.chapter_num })}</h3>
      <div class="grid grid-cols-2 gap-3 text-sm">
        <div>
          <div class="text-xs text-base-content/50 mb-1">{$t('pp.diff.before')}</div>
          <div class="bg-base-300 rounded p-3 whitespace-pre-wrap max-h-64 overflow-y-auto font-serif">{diffItem.diff_original || '—'}</div>
        </div>
        <div>
          <div class="text-xs text-base-content/50 mb-1">{$t('pp.diff.after')}</div>
          <div class="bg-base-300 rounded p-3 whitespace-pre-wrap max-h-64 overflow-y-auto font-serif">{diffItem.diff_revised || '—'}</div>
        </div>
      </div>
      <div class="modal-action">
        <button class="btn btn-sm" on:click={() => diffItem = null}>{$t('common.close')}</button>
      </div>
    </div>
    <form method="dialog" class="modal-backdrop"><button on:click={() => diffItem = null}>close</button></form>
  </dialog>
{/if}
