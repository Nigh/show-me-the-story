<script>
  import { api } from '../lib/api.js';
  import { progress, config, streamingContent, streamingChapterIdx, taskRunning, addToast, showConfirm, outlineCharacterSuggestions, outlineCharacterShowSuggestions, settings } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';
  import { onMount, tick } from 'svelte';
  import ConfigChangePanel from '../components/ConfigChangePanel.svelte';

  const OUTLINE_FOCUS_KEY = 'showmethestory.outlineFocusChapter';

  function isOutlineEditable(status) {
    return status === 'pending' || status === 'writing' || status === 'review';
  }

  $: p = $progress;
  $: displayTitle = $config?.story?.title || p?.title || '';
  $: displaySynopsis = $config?.story?.story_synopsis || p?.story_synopsis || '';
  $: chapters = p?.chapters || [];
  $: arcs = p?.arcs || [];
  $: hasOutline = chapters.length > 0 || arcs.length > 0;
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

  // 导入续写（v3 流水线）
  let showImport = false;
  let importContent = '';
  let importPreview = null; // [{num,title,word_count,preview}]
  let importStatus = null;  // {active,total,cursor} 断点状态
  let continuationCount = 5;

  onMount(refreshImportStatus);
  $: if (!$taskRunning) refreshImportStatus();

  let outlineFocusTried = false;
  $: if (!outlineFocusTried && chapters.length > 0) {
    outlineFocusTried = true;
    focusChapterFromSession();
  }

  async function focusChapterFromSession() {
    let raw;
    try { raw = sessionStorage.getItem(OUTLINE_FOCUS_KEY); } catch { return; }
    if (!raw) return;
    try { sessionStorage.removeItem(OUTLINE_FOCUS_KEY); } catch {}
    const num = parseInt(raw, 10);
    if (!num) return;
    const ch = chapters.find(c => c.num === num);
    if (!ch || !isOutlineEditable(ch.status) || $taskRunning) return;
    startEdit(ch);
    await tick();
    const el = document.querySelector(`[data-outline-chapter="${num}"]`);
    if (el) el.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }

  async function refreshImportStatus() {
    try {
      const st = await api('GET', '/api/import/status');
      importStatus = st?.active ? st : null;
    } catch { importStatus = null; }
  }

  async function generateOutline() {
    try {
      await api('POST', '/api/outline/generate');
      addToast($t('outline.toasts.outlineStarted'), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  // 卷（arc）操作
  let arcReqOpenId = -1;
  let arcRequirements = '';
  let showAppendArc = false;
  let appendArcTitle = '';
  let appendArcGoal = '';
  let appendArcCount = 20;

  function arcChapterCounts(arc) {
    const inRange = chapters.filter(c => c.num >= arc.start_ch && c.num <= arc.end_ch);
    return { outlined: inRange.length, total: arc.end_ch - arc.start_ch + 1 };
  }

  async function generateSkeleton() {
    try {
      await api('POST', '/api/arcs/skeleton');
      addToast($t('outline.toasts.skeletonStarted'), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function generateArcOutline(arc) {
    try {
      await api('POST', `/api/arcs/${arc.id}/outline`, { requirements: arcReqOpenId === arc.id ? arcRequirements.trim() : '' });
      addToast($t('outline.toasts.arcOutlineStarted'), 'info');
      arcReqOpenId = -1;
      arcRequirements = '';
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function appendArc() {
    try {
      await api('POST', '/api/arcs/append', {
        title: appendArcTitle.trim(),
        goal: appendArcGoal.trim(),
        chapter_count: Number(appendArcCount) || 20,
      });
      addToast($t('outline.toasts.arcAppendStarted'), 'info');
      showAppendArc = false;
      appendArcTitle = '';
      appendArcGoal = '';
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

  async function previewImportSplit() {
    const content = importContent.trim();
    if (!content) { addToast($t('outline.toasts.importContentRequired'), 'error'); return; }
    try {
      const res = await api('POST', '/api/import/split', { content });
      importPreview = res.chapters || [];
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function startImport() {
    try {
      await api('POST', '/api/import/start', { content: importContent.trim() });
      addToast($t('outline.toasts.importStarted'), 'info');
      showImport = false;
      importContent = '';
      importPreview = null;
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function resumeImport() {
    try {
      await api('POST', '/api/import/resume');
      addToast($t('outline.toasts.importResumed'), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function confirmCharacterSuggestions() {
    const selected = $outlineCharacterSuggestions.filter(s => s._selected !== false);
    if (selected.length === 0) {
      addToast($t('outline.charSuggestions.noneSelected'), 'error');
      return;
    }
    try {
      await api('POST', '/api/outline/characters/confirm', { characters: selected });
      settings.set(await api('GET', '/api/settings'));
      outlineCharacterSuggestions.set([]);
      outlineCharacterShowSuggestions.set(false);
      addToast($t('outline.charSuggestions.adopted', { n: selected.length }), 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }

  function dismissCharacterSuggestions() {
    outlineCharacterSuggestions.set([]);
    outlineCharacterShowSuggestions.set(false);
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
        <button class="btn btn-secondary btn-sm" on:click={generateSkeleton} disabled={$taskRunning}>{$t('outline.btn.skeleton')}</button>
        <button class="btn btn-ghost btn-sm" on:click={() => showImport = !showImport} disabled={$taskRunning}>{$t('outline.btn.import')}</button>
      </div>
      <p class="text-xs text-base-content/35 mt-2">{$t('outline.empty.arcHint')}</p>
    </div>

    {#if showImport}
      <div class="card bg-base-200 shadow-sm">
        <div class="card-body p-4 gap-2">
          <h3 class="card-title text-base">{$t('outline.import.title')}</h3>
          <p class="text-xs text-base-content/50">{$t('outline.import.hint')}</p>
          <textarea class="textarea w-full h-48 text-sm font-serif" bind:value={importContent} on:input={() => importPreview = null} placeholder={$t('outline.import.placeholder')} disabled={$taskRunning}></textarea>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-xs" on:click={() => { showImport = false; importContent = ''; importPreview = null; }}>{$t('common.cancel')}</button>
            <button class="btn btn-primary btn-xs" on:click={previewImportSplit} disabled={$taskRunning || !importContent.trim()}>{$t('outline.import.preview')}</button>
          </div>

          {#if importPreview}
            <div class="bg-base-300 rounded-lg p-3 space-y-2">
              <div class="text-sm font-medium">{$t('outline.import.previewTitle', { n: importPreview.length })}</div>
              <div class="max-h-64 overflow-y-auto space-y-1">
                {#each importPreview as ch (ch.num)}
                  <div class="bg-base-100/50 rounded p-2 text-xs flex items-baseline gap-2">
                    <span class="font-bold text-base-content/40 w-8 shrink-0">{ch.num}</span>
                    <span class="font-medium shrink-0">{ch.title}</span>
                    <span class="text-base-content/40 shrink-0">{$t('outline.import.words', { n: ch.word_count })}</span>
                    <span class="text-base-content/50 truncate">{ch.preview}</span>
                  </div>
                {/each}
              </div>
              <p class="text-xs text-base-content/50">{$t('outline.import.startHint')}</p>
              <div class="flex justify-end">
                <button class="btn btn-success btn-xs" on:click={startImport} disabled={$taskRunning || importPreview.length === 0}>{$t('outline.import.start')}</button>
              </div>
            </div>
          {/if}
        </div>
      </div>
    {/if}
  {:else}
    {#if importStatus}
      <div class="alert alert-warning py-2 text-sm flex items-center justify-between">
        <span>{$t('outline.import.resumeBanner', { done: importStatus.cursor, total: importStatus.total })}</span>
        <button class="btn btn-primary btn-xs" on:click={resumeImport} disabled={$taskRunning}>{$t('outline.import.resume')}</button>
      </div>
    {/if}
    <ConfigChangePanel />

    {#if $outlineCharacterShowSuggestions && $outlineCharacterSuggestions.length > 0}
      <div class="card bg-base-200 border border-primary/30 shadow-sm">
        <div class="card-body py-4 gap-3">
          <h3 class="font-semibold">{$t('outline.charSuggestions.title', { n: $outlineCharacterSuggestions.length })}</h3>
          <p class="text-sm text-base-content/60">{$t('outline.charSuggestions.hint')}</p>
          <div class="space-y-2 max-h-72 overflow-y-auto">
            {#each $outlineCharacterSuggestions as s}
              <label class="flex gap-3 p-3 rounded-lg bg-base-300/50 cursor-pointer">
                <input type="checkbox" class="checkbox checkbox-sm mt-1" bind:checked={s._selected} />
                <div class="min-w-0 flex-1">
                  <div class="font-medium">{s.name}</div>
                  {#if s.description}
                    <div class="text-sm text-base-content/70 mt-1">{s.description}</div>
                  {/if}
                  <div class="text-xs text-base-content/50 mt-1">
                    {$t('outline.charSuggestions.line', { chapter: s.chapter_num, role: s.role || $t('outline.charSuggestions.noRole') })}
                  </div>
                </div>
              </label>
            {/each}
          </div>
          <div class="flex gap-2">
            <button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={confirmCharacterSuggestions}>{$t('outline.charSuggestions.adopt')}</button>
            <button class="btn btn-ghost btn-sm" on:click={dismissCharacterSuggestions}>{$t('outline.charSuggestions.dismiss')}</button>
          </div>
        </div>
      </div>
    {/if}

    <!-- 操作栏 -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <div class="flex items-center gap-2 flex-wrap">
          <h3 class="text-base font-semibold flex-1 min-w-0 truncate">📖 {displayTitle || $t('common.untitled')}</h3>
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
        {#if displaySynopsis}
          <div>
            <span class="text-xs text-base-content/50">{$t('outline.synopsis')}</span>
            <div class="bg-base-300 rounded p-2 text-sm mt-0.5 max-h-24 overflow-y-auto">{displaySynopsis}</div>
          </div>
        {/if}
      </div>
    </div>

    <!-- 卷结构（层级大纲） -->
    {#if arcs.length > 0}
      <div class="card bg-base-200 shadow-sm">
        <div class="card-body p-4 gap-2">
          <div class="flex items-center justify-between">
            <h4 class="text-sm font-semibold text-base-content/60">{$t('outline.arcs.title')} <span class="font-normal text-base-content/35">({arcs.length})</span></h4>
            <button class="btn btn-ghost btn-xs" on:click={() => showAppendArc = !showAppendArc} disabled={$taskRunning}>{$t('outline.arcs.append')}</button>
          </div>

          {#if showAppendArc}
            <div class="bg-base-300 rounded-lg p-3 space-y-2">
              <div class="flex gap-2">
                <input type="text" class="input input-sm flex-1" bind:value={appendArcTitle} placeholder={$t('outline.arcs.appendTitle')} disabled={$taskRunning} />
                <input type="number" min="1" max="100" class="input input-sm w-20" bind:value={appendArcCount} disabled={$taskRunning} title={$t('outline.arcs.appendCount')} />
              </div>
              <textarea class="textarea textarea-sm w-full h-16 text-sm" bind:value={appendArcGoal} placeholder={$t('outline.arcs.appendGoal')} disabled={$taskRunning}></textarea>
              <div class="flex justify-end gap-2">
                <button class="btn btn-ghost btn-xs" on:click={() => showAppendArc = false}>{$t('common.cancel')}</button>
                <button class="btn btn-primary btn-xs" on:click={appendArc} disabled={$taskRunning}>{$t('outline.arcs.appendSubmit')}</button>
              </div>
            </div>
          {/if}

          <div class="space-y-1.5">
            {#each arcs as arc, i (arc.id)}
              {@const counts = arcChapterCounts(arc)}
              <div class="bg-base-300 rounded-lg p-2.5">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-bold text-base-content/40 shrink-0">{$t('outline.arcs.volLabel', { n: i + 1 })}</span>
                  <span class="text-sm font-medium flex-1 min-w-0 truncate">{arc.title}</span>
                  <span class="text-xs text-base-content/40 shrink-0">{$t('outline.arcs.range', { start: arc.start_ch, end: arc.end_ch })}</span>
                  <span class="badge badge-xs {counts.outlined >= counts.total ? 'badge-success' : 'badge-ghost'}">{$t('outline.arcs.outlined', { n: counts.outlined, total: counts.total })}</span>
                  {#if arc.summary}
                    <span class="badge badge-xs badge-info">{$t('outline.arcs.summaryDone')}</span>
                  {/if}
                  <button class="btn btn-primary btn-xs shrink-0" on:click={() => generateArcOutline(arc)} disabled={$taskRunning}>
                    {counts.outlined > 0 ? $t('outline.arcs.regenOutline') : $t('outline.arcs.genOutline')}
                  </button>
                  <button class="btn btn-ghost btn-xs shrink-0" on:click={() => { arcReqOpenId = arcReqOpenId === arc.id ? -1 : arc.id; arcRequirements = ''; }} disabled={$taskRunning}>+</button>
                </div>
                {#if arc.goal}
                  <p class="text-xs text-base-content/50 mt-1 line-clamp-2">{arc.goal}</p>
                {/if}
                {#if arcReqOpenId === arc.id}
                  <textarea class="textarea textarea-sm w-full h-14 text-sm mt-2" bind:value={arcRequirements} placeholder={$t('outline.arcs.reqPlaceholder')} disabled={$taskRunning}></textarea>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      </div>
    {/if}

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
              <div data-outline-chapter={ch.num} class="bg-base-300 rounded-lg p-3 space-y-2 ring-1 ring-primary/50">
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
                data-outline-chapter={ch.num}
                class="bg-base-300 rounded-lg p-2.5 group {isOutlineEditable(ch.status) && !$taskRunning ? 'cursor-pointer hover:ring-1 hover:ring-primary/40' : ''} transition-shadow"
                on:click={() => isOutlineEditable(ch.status) && !$taskRunning && startEdit(ch)}
              >
                <div class="flex items-center gap-2">
                  <span class="text-sm font-bold text-base-content/40 w-12 shrink-0">{ch.num}</span>
                  <span class="text-sm font-medium flex-1 min-w-0 truncate">{ch.title}</span>
                  <span class="badge badge-xs {statusMeta[ch.status]?.cls || 'badge-ghost'}">{statusMeta[ch.status]?.label || ch.status}</span>
                  {#if isOutlineEditable(ch.status)}
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
