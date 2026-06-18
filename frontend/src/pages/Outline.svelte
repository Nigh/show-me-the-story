<script>
  import { api } from '../lib/api.js';
  import { progress, streamingContent, streamingChapterIdx, taskRunning, addToast, showConfirm, continueAnalysis } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  $: p = $progress;
  $: chapters = p?.chapters || [];
  $: hasOutline = chapters.length > 0;
  $: hasAccepted = chapters.some(c => c.status === 'accepted');
  $: inOutlinePhase = p?.phase === 'outline';
  $: pendingCount = chapters.filter(c => c.status === 'pending').length;

  $: statusMeta = {
    pending:  { label: $t('outline.status.pending'),  cls: 'badge-ghost' },
    writing:  { label: $t('outline.status.writing'),  cls: 'badge-warning' },
    review:   { label: $t('outline.status.review'),   cls: 'badge-info' },
    accepted: { label: $t('outline.status.accepted'), cls: 'badge-success' },
  };

  let reviseFeedback = '';
  let showRevise = false;

  // 内联编辑
  let editingNum = -1;
  let editTitle = '';
  let editOutline = '';

  // 导入续写
  let showImport = false;
  let importContent = '';
  let continuationCount = 5;

  async function generateOutline() {
    try {
      await api('POST', '/api/outline/generate');
      addToast($t('outline.toasts.outlineStarted'), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function confirmOutline() {
    showConfirm($t('outline.toasts.confirmAsk'), async () => {
      try {
        await api('POST', '/api/outline/confirm');
        progress.set(await api('GET', '/api/progress'));
        addToast($t('outline.toasts.outlineConfirmed'), 'success');
        window.location.hash = '#writing';
      } catch (e) { addToast(e.message, 'error'); }
    });
  }

  async function reviseOutline() {
    const fb = reviseFeedback.trim();
    if (!fb) { addToast($t('outline.toasts.reviseFeedbackRequired'), 'error'); return; }
    try {
      await api('POST', '/api/outline/revise', { feedback: fb });
      addToast($t('outline.toasts.reviseStarted'), 'info');
      reviseFeedback = '';
      showRevise = false;
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function deleteOutline() {
    showConfirm($t('outline.toasts.deleteConfirm', { n: chapters.length }), async () => {
      try {
        await api('DELETE', '/api/outline');
        progress.set(await api('GET', '/api/progress'));
        addToast($t('outline.toasts.deleted'), 'success');
      } catch (e) { addToast(e.message, 'error'); }
    });
  }

  async function generateContinuation() {
    try {
      await api('POST', '/api/outline/generate-continuation', { chapter_count: Number(continuationCount) || 5 });
      addToast($t('outline.toasts.continuationStarted'), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  function startEdit(ch) {
    editingNum = ch.num;
    editTitle = ch.title;
    editOutline = ch.outline;
  }

  function cancelEdit() {
    editingNum = -1;
  }

  async function saveEdit() {
    if (!editTitle.trim() || !editOutline.trim()) { addToast($t('outline.toasts.editRequired'), 'error'); return; }
    try {
      await api('PUT', '/api/outline/' + editingNum, { title: editTitle.trim(), outline: editOutline.trim() });
      progress.set(await api('GET', '/api/progress'));
      addToast($t('outline.toasts.editSaved', { num: editingNum }), 'success');
      editingNum = -1;
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function importExisting() {
    const content = importContent.trim();
    if (!content) { addToast($t('outline.toasts.importContentRequired'), 'error'); return; }
    try {
      await api('POST', '/api/continue/import', { content });
      addToast($t('outline.toasts.importStarted'), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function confirmImport() {
    if (!$continueAnalysis) return;
    try {
      await api('POST', '/api/continue/confirm', $continueAnalysis);
      progress.set(await api('GET', '/api/progress'));
      continueAnalysis.set(null);
      showImport = false;
      importContent = '';
      addToast($t('outline.toasts.importDone'), 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }
</script>

<div class="space-y-3">
  {#if !hasOutline}
    <!-- 空状态 -->
    <div class="text-center py-14 text-base-content/50">
      <div class="text-5xl mb-3">📝</div>
      <p class="text-base mb-1">{$t('outline.empty.title')}</p>
      <p class="text-sm text-base-content/35 mb-6">{$t('outline.empty.hint')}</p>
      <div class="flex justify-center gap-2">
        <button class="btn btn-primary btn-sm" on:click={generateOutline} disabled={$taskRunning}>{$t('outline.btn.generate')}</button>
        <button class="btn btn-ghost btn-sm" on:click={() => showImport = !showImport} disabled={$taskRunning}>{$t('outline.btn.import')}</button>
      </div>
    </div>

    {#if showImport}
      <div class="card bg-base-200 shadow-sm">
        <div class="card-body p-4 gap-2">
          <h3 class="card-title text-base">{$t('outline.import.title')}</h3>
          <p class="text-xs text-base-content/50">{$t('outline.import.hint')}</p>
          <textarea class="textarea w-full h-48 text-sm font-serif" bind:value={importContent} placeholder={$t('outline.import.placeholder')} disabled={$taskRunning}></textarea>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-xs" on:click={() => { showImport = false; importContent = ''; }}>{$t('common.cancel')}</button>
            <button class="btn btn-primary btn-xs" on:click={importExisting} disabled={$taskRunning || !importContent.trim()}>{$t('outline.import.start')}</button>
          </div>
        </div>
      </div>
    {/if}

    {#if $continueAnalysis}
      <div class="card bg-base-200 shadow-sm border border-primary/30">
        <div class="card-body p-4 gap-2">
          <h3 class="card-title text-base">{$t('outline.analysis.title')}</h3>
          <div class="grid grid-cols-2 gap-2">
            <div>
              <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.title')}</span>
              <input type="text" class="input input-sm w-full" bind:value={$continueAnalysis.title} disabled={$taskRunning} />
            </div>
            <div>
              <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.type')}</span>
              <input type="text" class="input input-sm w-full" bind:value={$continueAnalysis.story_type} disabled={$taskRunning} />
            </div>
          </div>
          <div>
            <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.synopsis')}</span>
            <textarea class="textarea textarea-sm w-full h-20 text-sm" bind:value={$continueAnalysis.story_synopsis} disabled={$taskRunning}></textarea>
          </div>
          <div>
            <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.style')}</span>
            <textarea class="textarea textarea-sm w-full h-16 text-sm" bind:value={$continueAnalysis.writing_style} disabled={$taskRunning}></textarea>
          </div>
          <div>
            <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.pov')}</span>
            <textarea class="textarea textarea-sm w-full h-16 text-sm" bind:value={$continueAnalysis.writing_pov} disabled={$taskRunning}></textarea>
          </div>
          <div class="text-xs text-base-content/50">{$t('outline.analysis.detected', { n: $continueAnalysis.chapters?.length || 0 })}</div>
          <div class="max-h-48 overflow-y-auto space-y-1">
            {#each ($continueAnalysis.chapters || []) as ch}
              <div class="bg-base-300 rounded p-2 text-xs">
                <span class="font-medium">{$t('outline.analysis.chapter', { num: ch.num, title: ch.title })}</span>
                <span class="text-base-content/50">{ch.outline || ch.summary || ''}</span>
              </div>
            {/each}
          </div>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-xs" on:click={() => continueAnalysis.set(null)}>{$t('outline.analysis.abandon')}</button>
            <button class="btn btn-success btn-xs" on:click={confirmImport} disabled={$taskRunning}>{$t('outline.analysis.confirm')}</button>
          </div>
        </div>
      </div>
    {/if}
  {:else}
    <!-- 操作栏 -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <div class="flex items-center gap-2 flex-wrap">
          <h3 class="text-base font-semibold flex-1 min-w-0 truncate">📖 {p.title || $t('common.untitled')}</h3>
          {#if inOutlinePhase}
            <button class="btn btn-success btn-xs" on:click={confirmOutline} disabled={$taskRunning || chapters.length === 0}>{$t('outline.btn.confirm')}</button>
          {/if}
          <button class="btn btn-ghost btn-xs" on:click={() => showRevise = !showRevise} disabled={$taskRunning}>{$t('outline.btn.revise')}</button>
          {#if hasAccepted}
            <div class="join">
              <input type="number" min="1" max="50" class="input input-xs join-item w-14" bind:value={continuationCount} disabled={$taskRunning} />
              <button class="btn btn-primary btn-xs join-item" on:click={generateContinuation} disabled={$taskRunning}>{$t('outline.btn.continuation')}</button>
            </div>
          {:else if inOutlinePhase}
            <button class="btn btn-ghost btn-xs" on:click={generateOutline} disabled={$taskRunning}>{$t('outline.btn.regenerate')}</button>
          {/if}
          {#if !hasAccepted}
            <button class="btn btn-ghost btn-xs text-error" on:click={deleteOutline} disabled={$taskRunning}>{$t('outline.btn.deleteOutline')}</button>
          {/if}
        </div>

        {#if showRevise}
          <div class="bg-base-300 rounded-lg p-3 space-y-2">
            <textarea class="textarea textarea-sm w-full h-20 text-sm" bind:value={reviseFeedback} placeholder={$t('outline.revise.placeholder')} disabled={$taskRunning}></textarea>
            <div class="flex justify-between items-center">
              <span class="text-xs text-base-content/40">{$t('outline.revise.hint')}</span>
              <div class="flex gap-2">
                <button class="btn btn-ghost btn-xs" on:click={() => { showRevise = false; reviseFeedback = ''; }}>{$t('common.cancel')}</button>
                <button class="btn btn-primary btn-xs" on:click={reviseOutline} disabled={$taskRunning || !reviseFeedback.trim()}>{$t('outline.revise.submit')}</button>
              </div>
            </div>
          </div>
        {/if}

        {#if p.core_prompt}
          <div>
            <span class="text-xs text-base-content/50">{$t('outline.corePrompt')}</span>
            <div class="bg-base-300 rounded p-2 text-sm mt-0.5 max-h-24 overflow-y-auto">{p.core_prompt}</div>
          </div>
        {/if}
        {#if p.story_synopsis}
          <div>
            <span class="text-xs text-base-content/50">{$t('outline.synopsis')}</span>
            <div class="bg-base-300 rounded p-2 text-sm mt-0.5 max-h-24 overflow-y-auto">{p.story_synopsis}</div>
          </div>
        {/if}
      </div>
    </div>

    <!-- 章节大纲列表 -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <div class="flex items-center justify-between">
          <h4 class="text-sm font-semibold text-base-content/60">{$t('outline.chapterList')} <span class="font-normal text-base-content/35">{$t('outline.chapterList.summary', { total: chapters.length, suffix: pendingCount ? $t('outline.chapterList.pendingSuffix', { n: pendingCount }) : '' })}</span></h4>
          <span class="text-xs text-base-content/35">{$t('outline.chapterList.editHint')}</span>
        </div>
        <div class="space-y-1.5">
          {#each chapters as ch (ch.num)}
            {#if editingNum === ch.num}
              <div class="bg-base-300 rounded-lg p-3 space-y-2 ring-1 ring-primary/50">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-bold text-base-content/50 shrink-0">{$t('outline.chapter.chapterLabel', { num: ch.num })}</span>
                  <input type="text" class="input input-sm flex-1" bind:value={editTitle} placeholder={$t('outline.chapter.titlePlaceholder')} disabled={$taskRunning} />
                </div>
                <textarea class="textarea textarea-sm w-full h-24 text-sm" bind:value={editOutline} placeholder={$t('outline.chapter.outlinePlaceholder')} disabled={$taskRunning}></textarea>
                <div class="flex justify-end gap-2">
                  <button class="btn btn-ghost btn-xs" on:click={cancelEdit}>{$t('common.cancel')}</button>
                  <button class="btn btn-success btn-xs" on:click={saveEdit} disabled={$taskRunning}>{$t('common.save')}</button>
                </div>
              </div>
            {:else}
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <!-- svelte-ignore a11y-no-static-element-interactions -->
              <div
                class="bg-base-300 rounded-lg p-2.5 group {ch.status === 'pending' && !$taskRunning ? 'cursor-pointer hover:ring-1 hover:ring-primary/40' : ''} transition-shadow"
                on:click={() => ch.status === 'pending' && !$taskRunning && startEdit(ch)}
              >
                <div class="flex items-center gap-2">
                  <span class="text-sm font-bold text-base-content/40 w-12 shrink-0">{ch.num}</span>
                  <span class="text-sm font-medium flex-1 min-w-0 truncate">{ch.title}</span>
                  <span class="badge badge-xs {statusMeta[ch.status]?.cls || 'badge-ghost'}">{statusMeta[ch.status]?.label || ch.status}</span>
                  {#if ch.status === 'pending'}
                    <span class="text-xs text-primary opacity-0 group-hover:opacity-100 transition-opacity shrink-0">{$t('outline.chapter.editTag')}</span>
                  {/if}
                </div>
                <p class="text-xs text-base-content/50 mt-1 ml-14 line-clamp-2">{ch.outline}</p>
              </div>
            {/if}
          {/each}
        </div>

        {#if $streamingChapterIdx >= 0 && $streamingContent}
          <div class="bg-base-300 rounded p-3 mt-1 text-sm max-h-48 overflow-y-auto chapter-content">
            <div class="text-xs text-base-content/40 mb-1 flex items-center gap-1">
              <span class="loading loading-dots loading-xs"></span> {$t('outline.streamHint')}
            </div>
            {$streamingContent}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
