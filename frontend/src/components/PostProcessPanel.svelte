<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { postprocess, taskRunning, addToast, confirmModal, progress } from '../lib/stores.js';
  import { renderMarkdown } from '../lib/markdown.js';

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

  const typeLabels = {
    logic: '逻辑', transition: '衔接', style: '文风', rhythm: '节奏',
    dialogue: '对话', polish: '润色',
  };
  const statusLabels = {
    pending: '待执行', running: '执行中', done: '已完成',
    failed: '失败', skipped: '无变化',
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
      addToast('工单已保存', 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }

  function runDiagnose() {
    confirmModal.set({
      message: '将依次运行：全书诊断 → 一致性核查 → 生成优化路线图。耗时取决于全书篇幅与模型速度，是否开始？',
      onConfirm: async () => {
        try {
          await api('POST', '/api/postprocess/diagnose');
          addToast('全书优化分析已启动', 'info');
        } catch (e) { addToast(e.message, 'error'); }
      },
    });
  }

  async function runConsistency() {
    try {
      await api('POST', '/api/postprocess/consistency');
      addToast('全书一致性核查已启动', 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function runRoadmap() {
    try {
      await api('POST', '/api/postprocess/roadmap');
      addToast('路线图重新生成已启动', 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  function runExecute() {
    const pending = roadmapLocal.filter(r => r.selected && r.status === 'pending');
    const chapterCount = new Set(pending.map(r => r.chapter_num)).size;
    if (pending.length === 0) {
      addToast('请至少勾选一条待执行的工单', 'error');
      return;
    }
    const mergeHint = pending.length > chapterCount
      ? `（${pending.length} 条工单将按章合并为 ${chapterCount} 次编辑）`
      : '';
    confirmModal.set({
      message: `将处理 ${chapterCount} 章${mergeHint}，每章一次性完成全部修改意见，可随时停止。是否开始？`,
      onConfirm: async () => {
        try {
          if (dirty) await saveRoadmap();
          await api('POST', '/api/postprocess/execute', { execute_options: optsLocal });
          addToast('全书优化执行已启动', 'info');
        } catch (e) { addToast(e.message, 'error'); }
      },
    });
  }

  function clearAll() {
    confirmModal.set({
      message: '将清空诊断报告、核查报告和优化工单，是否继续？',
      onConfirm: async () => {
        try {
          const res = await api('DELETE', '/api/postprocess');
          postprocess.set(res);
          addToast('已清空全书优化数据', 'info');
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
        <h2 class="card-title text-base flex-1">全书优化</h2>
        {#if pp?.bundle_mode}
          <span class="badge badge-sm badge-ghost" title="诊断时使用的材料模式">
            {pp.bundle_mode === 'summary_only' ? '摘要模式' : '全文模式'}
          </span>
        {/if}
        {#if pp?.estimated_tokens}
          <span class="text-xs text-base-content/40">约 {pp.estimated_tokens.toLocaleString()} tokens</span>
        {/if}
        {#if pp?.volume_count > 1}
          <span class="text-xs text-base-content/40">{pp.volume_count} 卷核查</span>
        {/if}
      </div>

      <p class="text-xs text-base-content/50">
        完稿后通读诊断 → 一致性核查 → 生成可执行工单 → 逐章最小化修订。建议使用大上下文模型（配置页可设上下文预算）。
      </p>

      <div class="flex gap-2 flex-wrap">
        <button class="btn btn-primary btn-sm" on:click={runDiagnose} disabled={$taskRunning}>🔍 开始全书分析</button>
        <button class="btn btn-ghost btn-sm" on:click={runConsistency} disabled={$taskRunning || !pp?.diagnosis_report}>🧪 重新核查</button>
        <button class="btn btn-ghost btn-sm" on:click={runRoadmap} disabled={$taskRunning || (!pp?.diagnosis_report && !pp?.consistency_report)}>📋 重新生成路线图</button>
        <button class="btn btn-ghost btn-sm btn-error" on:click={clearAll} disabled={$taskRunning}>清空</button>
      </div>

      {#if pp?.diagnosis_report || pp?.consistency_report}
        <div class="tabs tabs-boxed tabs-sm w-fit">
          <button class="tab {reportTab === 'diagnosis' ? 'tab-active' : ''}" on:click={() => reportTab = 'diagnosis'}>诊断报告</button>
          <button class="tab {reportTab === 'consistency' ? 'tab-active' : ''}" on:click={() => reportTab = 'consistency'}>核查报告</button>
        </div>
        <div class="bg-base-300 rounded-lg p-3 max-h-64 overflow-y-auto text-sm">
          {#if reportTab === 'diagnosis' && diagnosisHtml}
            <div class="md-body">{@html diagnosisHtml}</div>
          {:else if reportTab === 'consistency' && consistencyHtml}
            <div class="md-body">{@html consistencyHtml}</div>
          {:else}
            <p class="text-base-content/40 text-center py-4">暂无报告</p>
          {/if}
        </div>
      {/if}

      {#if roadmapLocal.length > 0}
        <div class="divider my-0 text-xs">优化工单（{roadmapLocal.length} 条，待执行 {pendingCount}）</div>

        <div class="flex gap-3 flex-wrap items-center text-xs">
          <label class="flex items-center gap-1.5 cursor-pointer">
            <input type="checkbox" class="checkbox checkbox-xs" bind:checked={optsLocal.run_smooth_transitions_first} on:change={markDirty} />
            执行前先优化章节衔接
          </label>
          <label class="flex items-center gap-1.5 cursor-pointer">
            <input type="checkbox" class="checkbox checkbox-xs" bind:checked={optsLocal.include_polish} on:change={markDirty} />
            修订时附加去 AI 味
          </label>
          <div class="flex-1"></div>
          <button class="btn btn-ghost btn-xs" on:click={() => selectAllPending(true)} disabled={$taskRunning}>全选待执行</button>
          <button class="btn btn-ghost btn-xs" on:click={() => selectAllPending(false)} disabled={$taskRunning}>全不选</button>
          <button class="btn btn-ghost btn-xs" on:click={resetFailed} disabled={$taskRunning}>重置失败项</button>
          {#if dirty}
            <button class="btn btn-primary btn-xs" on:click={saveRoadmap} disabled={$taskRunning}>保存工单</button>
          {/if}
          <button class="btn btn-success btn-sm" on:click={runExecute} disabled={$taskRunning || selectedPending === 0}>
            ▶ 执行选中（{selectedChapterCount} 章 / {selectedPending} 条）
          </button>
        </div>

        <div class="overflow-x-auto max-h-80 overflow-y-auto rounded-lg border border-base-300">
          <table class="table table-xs table-zebra">
            <thead class="sticky top-0 bg-base-200 z-10">
              <tr>
                <th class="w-8"></th>
                <th>章</th>
                <th>类型</th>
                <th>优先级</th>
                <th class="min-w-[200px]">修改意见</th>
                <th class="min-w-[5.5rem] w-28">状态</th>
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
                  <td class="whitespace-nowrap">第{item.chapter_num}章</td>
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
                      <button class="btn btn-ghost btn-xs" on:click={() => diffItem = item}>对比</button>
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
      <h3 class="font-bold text-base mb-2">第 {diffItem.chapter_num} 章修改对比（节选前 500 字）</h3>
      <div class="grid grid-cols-2 gap-3 text-sm">
        <div>
          <div class="text-xs text-base-content/50 mb-1">修改前</div>
          <div class="bg-base-300 rounded p-3 whitespace-pre-wrap max-h-64 overflow-y-auto font-serif">{diffItem.diff_original || '—'}</div>
        </div>
        <div>
          <div class="text-xs text-base-content/50 mb-1">修改后</div>
          <div class="bg-base-300 rounded p-3 whitespace-pre-wrap max-h-64 overflow-y-auto font-serif">{diffItem.diff_revised || '—'}</div>
        </div>
      </div>
      <div class="modal-action">
        <button class="btn btn-sm" on:click={() => diffItem = null}>关闭</button>
      </div>
    </div>
    <form method="dialog" class="modal-backdrop"><button on:click={() => diffItem = null}>close</button></form>
  </dialog>
{/if}
