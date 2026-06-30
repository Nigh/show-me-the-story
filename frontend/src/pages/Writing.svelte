<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { progress, taskRunning, streamingContent, streamingChapterIdx, selectedChapter, autoConfirm, addToast, confirmModal, currentPage } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';
  import { countProseUnits } from '../lib/proseUnits.js';
  import PostProcessPanel from '../components/PostProcessPanel.svelte';
  import TaskTokenBadge from '../components/TaskTokenBadge.svelte';

  // 保留 prop 以兼容 App 传参
  export const sendToChat = async () => {};

  onMount(async () => {
    try {
      const res = await api('GET', '/api/autoconfirm');
      autoConfirm.set(!!res.enabled);
    } catch (e) {}
    try {
      const sk = await api('GET', '/api/skills');
      hasPolishSkills = (sk || []).some(s => s.enabled && s.skill?.category === 'polish');
    } catch (e) {}
  });

  async function toggleAutoConfirm(e) {
    const enabled = e.target.checked;
    try {
      const res = await api('PUT', '/api/autoconfirm', { enabled });
      autoConfirm.set(!!res.enabled);
      addToast(res.enabled ? $t('writing.toasts.autoConfirmOn') : $t('writing.toasts.autoConfirmOff'), 'info');
    } catch (err) {
      e.target.checked = $autoConfirm;
      addToast(err.message, 'error');
    }
  }

  $: p = $progress;
  $: inWriting = p?.phase === 'writing';
  $: chapters = p?.chapters || [];
  $: total = chapters.length;
  $: accepted = chapters.filter(c => c.status === 'accepted').length;
  $: pct = total > 0 ? Math.round(accepted / total * 100) : 0;
  $: currentIdx = p?.current_chapter_index ?? 0;

  // 默认选中当前章节
  $: if (inWriting && ($selectedChapter < 0 || $selectedChapter >= chapters.length)) {
    selectedChapter.set(Math.min(currentIdx, chapters.length - 1));
  }

  // 自动确认模式下，自动跟随正在生成的章节
  $: if ($autoConfirm && $streamingChapterIdx >= 0 && $streamingChapterIdx < chapters.length && $streamingChapterIdx !== $selectedChapter) {
    selectedChapter.set($streamingChapterIdx);
  }

  $: ch = $selectedChapter >= 0 && $selectedChapter < chapters.length ? chapters[$selectedChapter] : null;
  $: isCurrent = ch && currentIdx === $selectedChapter;
  $: isStreamingThis = $streamingChapterIdx === $selectedChapter && $streamingContent;
  // 流式期间 $streamingContent 只含尾部窗口（性能保护），全文在生成结束后由 progress 拉取
  $: displayContent = isStreamingThis ? $streamingContent : (ch?.content || '');
  $: chapterWordCount = ch?.content ? countProseUnits(ch.content) : 0;
  $: showTaskTokens = $taskRunning && isCurrent;
  $: totalWords = chapters.reduce((sum, c) => sum + (c.content ? countProseUnits(c.content) : 0), 0);

  $: foreshadows = p?.foreshadows || [];
  $: fsActive = foreshadows.filter(f => f.status === 'planted' || f.status === 'progressing');
  $: fsOverdue = fsActive.filter(f => f.target_chapter > 0 && (currentIdx + 1) > f.target_chapter);
  $: fsNearTarget = fsActive.filter(f =>
    f.target_chapter > 0 && (currentIdx + 1) >= f.target_chapter - 2 && (currentIdx + 1) <= f.target_chapter
  );
  $: writingConflict = p?.pending_writing_conflict || null;

  async function resolveWritingConflict(action) {
    if ($taskRunning) return;
    try {
      const res = await api('POST', '/api/chapter/conflict-resolve', { action });
      if (action === 'retry') {
        progress.set(await api('GET', '/api/progress'));
        await api('POST', '/api/chapter/generate');
        addToast($t('writing.toasts.generateStarted', { num: writingConflict?.chapter_num }), 'info');
        return;
      }
      progress.set(res);
      if (action === 'force_review') {
        addToast($t('writing.conflict.forceReview'), 'success');
      }
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  function gotoPage(page) {
    currentPage.set(page);
    window.location.hash = '#' + page;
  }

  $: statusMeta = {
    pending:  { label: $t('writing.status.pending'),  cls: 'badge-ghost',   dot: 'bg-base-content/20' },
    writing:  { label: $t('writing.status.writing'),  cls: 'badge-warning', dot: 'bg-warning animate-pulse' },
    review:   { label: $t('writing.status.review'),   cls: 'badge-info',    dot: 'bg-info' },
    accepted: { label: $t('writing.status.accepted'), cls: 'badge-success', dot: 'bg-success' },
  };

  let reviseFeedback = '';
  let showRevise = false;
  let contentEl;
  let reviseTextareaEl;
  let hasPolishSkills = false;

  // 框选原文后的浮动「引用」按钮：null 表示隐藏
  let quotePopover = null;

  function checkContentSelection() {
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed || sel.rangeCount === 0) { quotePopover = null; return; }
    const text = sel.toString().trim();
    if (!text || text.length < 2) { quotePopover = null; return; }
    const range = sel.getRangeAt(0);
    if (!contentEl || !contentEl.contains(range.commonAncestorContainer)) {
      quotePopover = null; return;
    }
    const rect = range.getBoundingClientRect();
    if (rect.width === 0 && rect.height === 0) { quotePopover = null; return; }
    quotePopover = {
      text,
      x: rect.left + rect.width / 2,
      y: rect.top,
    };
  }

  function hideQuotePopover() { quotePopover = null; }

  function insertQuoteToFeedback() {
    if (!quotePopover) return;
    const text = quotePopover.text;
    const quoteLine = `> ${text}`;
    // 若用户已在修改意见里写过内容，先确保引用行与原内容之间有空行分隔
    const current = reviseFeedback;
    let insertion;
    if (current === '') {
      insertion = quoteLine + '\n';
    } else if (current.endsWith('\n')) {
      insertion = quoteLine + '\n';
    } else {
      insertion = '\n' + quoteLine + '\n';
    }
    // 优先在 textarea 光标位置插入；否则追加到末尾
    const ta = reviseTextareaEl;
    if (ta && document.activeElement === ta && ta.selectionStart != null) {
      const start = ta.selectionStart;
      const end = ta.selectionEnd;
      reviseFeedback = current.slice(0, start) + insertion + current.slice(end);
      requestAnimationFrame(() => {
        ta.focus();
        const pos = start + insertion.length;
        ta.setSelectionRange(pos, pos);
      });
    } else {
      reviseFeedback = current + insertion;
      requestAnimationFrame(() => {
        if (ta) { ta.focus(); const pos = reviseFeedback.length; ta.setSelectionRange(pos, pos); }
      });
    }
    showRevise = true;
    addToast($t('writing.toasts.quoteInserted', { n: text.length }), 'success');
    // 清空选区并隐藏按钮
    const sel = window.getSelection();
    if (sel) sel.removeAllRanges();
    hideQuotePopover();
  }

  // 流式输出时自动滚动到底部：合并到 rAF，每帧最多一次，避免高频强制重排
  let scrollPending = false;
  function scheduleScroll() {
    if (scrollPending) return;
    scrollPending = true;
    requestAnimationFrame(() => {
      scrollPending = false;
      if (contentEl) contentEl.scrollTop = contentEl.scrollHeight;
    });
  }
  $: if (isStreamingThis && contentEl) scheduleScroll();

  function selectChapter(i) {
    selectedChapter.set(i);
    showRevise = false;
    reviseFeedback = '';
    hideQuotePopover();
  }

  async function doGenerate() {
    try {
      await api('POST', '/api/chapter/generate');
      addToast($t('writing.toasts.generateStarted', { num: ch?.num }), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doConfirm() {
    try {
      await api('POST', '/api/chapter/confirm');
      progress.set(await api('GET', '/api/progress'));
      addToast($t('writing.toasts.confirmed', { num: ch?.num }), 'success');
      // 跳到下一章
      const next = await api('GET', '/api/progress');
      if (next.current_chapter_index < (next.chapters || []).length) {
        selectedChapter.set(next.current_chapter_index);
      }
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doRevise() {
    const fb = reviseFeedback.trim();
    if (!fb) { addToast($t('writing.toasts.feedbackRequired'), 'error'); return; }
    if (!ch) return;
    try {
      if (isCurrent && ch.status === 'review') {
        // 当前审核中章节：完整修订流程
        await api('POST', '/api/chapter/revise', { feedback: fb });
      } else {
        // 其他章节（含已确认）：定向最小化修订，不影响其他章节
        await api('POST', '/api/chapter/revise/' + ch.num, { feedback: fb });
      }
      addToast($t('writing.toasts.reviseStarted', { num: ch.num }), 'info');
      reviseFeedback = '';
      showRevise = false;
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doPolish() {
    if (!ch) return;
    try {
      await api('POST', '/api/chapter/polish', { num: ch.num });
      addToast($t('writing.toasts.polishStarted', { num: ch.num }), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function copyContent() {
    if (!ch?.content) return;
    try {
      await navigator.clipboard.writeText(ch.content);
      addToast($t('writing.toasts.copied'), 'success');
    } catch (e) { addToast($t('common.copy.failed'), 'error'); }
  }

  function exportBook() {
    const written = chapters.filter(c => c.content);
    if (written.length === 0) { addToast($t('writing.toasts.exportEmpty'), 'error'); return; }
    const titleStr = p.title || $t('common.untitled');
    const parts = [$t('writing.export.bookTitle', { title: titleStr }) + '\n'];
    for (const c of written) {
      parts.push('\n\n' + $t('writing.export.chapterHeader', { num: c.num, title: c.title }) + '\n\n' + c.content);
    }
    const blob = new Blob([parts.join('')], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${p.title || $t('writing.export.defaultName')}.txt`;
    a.click();
    URL.revokeObjectURL(url);
    addToast($t('writing.toasts.exportDone', { n: written.length }), 'success');
  }

  function prevChapter() { if ($selectedChapter > 0) selectChapter($selectedChapter - 1); }
  function nextChapter() { if ($selectedChapter < chapters.length - 1) selectChapter($selectedChapter + 1); }

  function smoothTransitions() {
    confirmModal.set({
      message: $t('writing.toasts.smoothAsk'),
      onConfirm: async () => {
        try {
          await api('POST', '/api/chapters/smooth-transitions');
          addToast($t('writing.toasts.smoothStarted'), 'info');
        } catch (e) { addToast(e.message, 'error'); }
      },
    });
  }
</script>

{#if !inWriting}
  <div class="text-center py-16 text-base-content/50">
    <div class="text-5xl mb-4">✍️</div>
    <p class="text-base mb-1">{$t('writing.notReady.title')}</p>
    <p class="text-sm text-base-content/35 mb-6">{$t('writing.notReady.hint')}</p>
    <button class="btn btn-primary btn-sm" on:click={() => window.location.hash = '#outline'}>{$t('writing.notReady.goto')}</button>
  </div>
{:else}
  <div class="space-y-3">
    <!-- 进度 -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <div class="flex items-center gap-3">
          <h2 class="card-title text-base flex-1">{$t('writing.progress.title')}</h2>
          <label class="flex items-center gap-1.5 cursor-pointer" title={$t('writing.progress.autoConfirmTip')}>
            <input type="checkbox" class="toggle toggle-xs toggle-success" checked={$autoConfirm} on:change={toggleAutoConfirm} />
            <span class="text-xs text-base-content/60">{$t('writing.progress.autoConfirm')}</span>
          </label>
          <span class="text-xs text-base-content/40">{$t('writing.progress.totalWords', { n: totalWords.toLocaleString() })}</span>
          {#if accepted >= 2}
            <button class="btn btn-ghost btn-xs" on:click={smoothTransitions} disabled={$taskRunning} title={$t('writing.btn.smoothTransitions.tip')}>{$t('writing.btn.smoothTransitions')}</button>
          {/if}
          <button class="btn btn-ghost btn-xs" on:click={exportBook}>{$t('writing.btn.exportTxt')}</button>
        </div>
        <progress class="progress progress-primary w-full" value={pct} max="100"></progress>
        <div class="text-sm text-base-content/50">{$t('writing.progress.acceptedSummary', { pct, accepted, total })}</div>
      </div>
    </div>

    {#if writingConflict}
      <div class="card bg-error/10 border border-error/30 shadow-sm">
        <div class="card-body p-4 gap-3">
          <h3 class="font-semibold text-error">{$t('writing.conflict.title')}</h3>
          <p class="text-sm">{$t('writing.conflict.summary')}：{writingConflict.summary}</p>
          {#if writingConflict.issues?.length}
            <div class="text-xs text-base-content/70">
              <div class="font-medium mb-1">{$t('writing.conflict.issues')}</div>
              <ul class="list-disc list-inside space-y-0.5">
                {#each writingConflict.issues as issue}
                  <li>{issue}</li>
                {/each}
              </ul>
            </div>
          {/if}
          <div class="flex flex-wrap gap-2">
            {#each (writingConflict.suggested_actions || []) as action}
              {#if action.id === 'edit_outline'}
                <button class="btn btn-warning btn-xs" disabled={$taskRunning} on:click={() => gotoPage('outline')}>{$t('writing.conflict.gotoOutline')}</button>
              {:else if action.id === 'adjust_foreshadow'}
                <button class="btn btn-warning btn-xs" disabled={$taskRunning} on:click={() => gotoPage('foreshadows')}>{$t('writing.conflict.gotoForeshadows')}</button>
              {:else if action.id === 'retry'}
                <button class="btn btn-primary btn-xs" disabled={$taskRunning} on:click={() => resolveWritingConflict('retry')}>{$t('writing.conflict.retry')}</button>
              {:else if action.id === 'force_review'}
                <button class="btn btn-ghost btn-xs" disabled={$taskRunning} on:click={() => resolveWritingConflict('force_review')}>{$t('writing.conflict.forceReview')}</button>
              {/if}
            {/each}
            <button class="btn btn-ghost btn-xs" disabled={$taskRunning} on:click={() => resolveWritingConflict('dismiss')}>{$t('writing.conflict.dismiss')}</button>
          </div>
        </div>
      </div>
    {/if}

    {#if foreshadows.length > 0}
      <div class="card bg-base-200 shadow-sm">
        <div class="card-body p-4 gap-2">
          <div class="flex items-center justify-between gap-2">
            <h3 class="font-medium text-sm">{$t('writing.fs.title')}</h3>
            <button class="btn btn-ghost btn-xs" on:click={() => window.location.hash = '#foreshadows'}>{$t('writing.fs.goto')}</button>
          </div>
          <div class="flex flex-wrap gap-2 text-xs">
            <span class="badge badge-ghost">{$t('writing.fs.total', { n: foreshadows.length })}</span>
            <span class="badge badge-info badge-outline">{$t('writing.fs.active', { n: fsActive.length })}</span>
            {#if fsOverdue.length > 0}
              <span class="badge badge-error">{$t('writing.fs.overdue', { n: fsOverdue.length })}</span>
            {/if}
            {#if fsNearTarget.length > 0}
              <span class="badge badge-warning badge-outline">{$t('writing.fs.nearTarget', { n: fsNearTarget.length })}</span>
            {/if}
          </div>
          {#if fsOverdue.length > 0}
            <p class="text-xs text-warning">{$t('writing.fs.overdueDetail', { names: fsOverdue.map(f => `#${f.id} ${f.name}`).join(', ') })}</p>
          {:else if fsNearTarget.length > 0}
            <p class="text-xs text-base-content/50">{$t('writing.fs.nearDetail', { names: fsNearTarget.map(f => f.name).join(', ') })}</p>
          {/if}
        </div>
      </div>
    {:else}
      <div class="card bg-base-200 shadow-sm">
        <div class="card-body p-4 flex items-center justify-between gap-2">
          <p class="text-sm text-base-content/50">{$t('writing.fs.none')}</p>
          <button class="btn btn-ghost btn-xs" on:click={() => window.location.hash = '#foreshadows'}>{$t('writing.fs.setup')}</button>
        </div>
      </div>
    {/if}

    <PostProcessPanel />

    <!-- 章节区 -->
    <div class="grid grid-cols-[230px_1fr] gap-3" style="min-height:400px">
      <!-- 章节列表 -->
      <div class="card bg-base-200 shadow-sm overflow-y-auto max-h-[calc(100vh-280px)]">
        <ul class="menu menu-sm p-0 w-full">
          {#each chapters as c, i}
            <li>
              <button class="flex gap-2 items-center {$selectedChapter === i ? 'active' : ''}" on:click={() => selectChapter(i)}>
                <span class="w-2 h-2 rounded-full shrink-0 {statusMeta[c.status]?.dot || ''}"></span>
                <span class="text-base-content/50 w-6 shrink-0 text-right">{c.num}</span>
                <span class="flex-1 text-left truncate text-sm">{c.title}</span>
                {#if i === currentIdx && c.status !== 'accepted'}
                  <span class="badge badge-primary badge-xs shrink-0">{$t('writing.tag.current')}</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
      </div>

      <!-- 内容区 -->
      <div class="min-w-0">
        {#if ch}
          <div class="card bg-base-200 shadow-sm">
            <div class="card-body p-4 gap-2">
              <div class="flex items-center gap-2 flex-wrap">
                <h2 class="card-title text-base flex-1 min-w-0">{$t('writing.chapter.title', { num: ch.num, title: ch.title })}</h2>
                <span class="badge badge-sm {statusMeta[ch.status]?.cls || 'badge-ghost'}">{statusMeta[ch.status]?.label || ch.status}</span>
                {#if showTaskTokens}
                  <TaskTokenBadge className="text-xs text-base-content/40 font-mono" />
                {:else if chapterWordCount > 0}
                  <span class="text-xs text-base-content/40">{$t('writing.chapter.words', { n: chapterWordCount.toLocaleString() })}</span>
                {/if}
              </div>

              {#if ch.outline}
                <details class="bg-base-300 rounded">
                  <summary class="p-2 text-xs text-base-content/50 cursor-pointer select-none">{$t('writing.chapter.outline')}</summary>
                  <div class="px-2 pb-2 text-sm text-base-content/70">{ch.outline}</div>
                </details>
              {/if}

              {#if ch.summary}
                <details class="bg-base-300 rounded">
                  <summary class="p-2 text-xs text-base-content/50 cursor-pointer select-none">{$t('writing.chapter.summary')}</summary>
                  <div class="px-2 pb-2 text-sm text-base-content/70 whitespace-pre-wrap">{ch.summary}</div>
                </details>
              {/if}

              {#if displayContent}
                {#if isStreamingThis}
                  <div class="text-xs text-warning/80 flex items-center gap-1.5">
                    <span class="loading loading-dots loading-xs"></span>
                    {$t('writing.chapter.streamHint')}
                  </div>
                {/if}
                <!-- svelte-ignore a11y-no-static-element-interactions -->
                <div bind:this={contentEl} class="bg-base-300 rounded-lg p-4 text-[15px] chapter-content reading-area max-h-[calc(100vh-420px)] min-h-[200px] overflow-y-auto"
                     on:mouseup={checkContentSelection}
                     on:scroll={hideQuotePopover}>
                  {displayContent}
                  {#if isStreamingThis}
                    <span class="inline-block w-2 h-4 bg-primary/70 animate-pulse ml-0.5 align-text-bottom"></span>
                  {/if}
                </div>
                {#if quotePopover}
                  <button type="button"
                    class="fixed z-50 btn btn-primary btn-xs shadow-lg"
                    style="left: {quotePopover.x}px; top: {quotePopover.y}px; transform: translate(-50%, -100%); margin-top: -6px;"
                    on:click={insertQuoteToFeedback}
                    title={$t('writing.revise.quoteBtn.tip')}>
                    {$t('writing.revise.quoteBtn')}
                  </button>
                {/if}
              {:else if ch.status === 'pending'}
                <div class="bg-base-300 rounded-lg p-6 text-center text-sm text-base-content/40">
                  {#if isCurrent}
                    {$t('writing.chapter.pendingCurrent')}
                  {:else}
                    {$t('writing.chapter.pendingOther', { n: chapters[currentIdx]?.num ?? '-' })}
                  {/if}
                </div>
              {/if}

              <!-- 操作 -->
              <div class="flex gap-2 flex-wrap items-center mt-1">
                {#if ch.status === 'pending' && isCurrent}
                  <button class="btn btn-primary btn-sm" on:click={doGenerate} disabled={$taskRunning}>{$t('writing.btn.generate')}</button>
                {/if}
                {#if ch.status === 'review' && isCurrent}
                  <button class="btn btn-success btn-sm" on:click={doConfirm} disabled={$taskRunning}>{$t('writing.btn.confirm')}</button>
                {/if}
                {#if ch.content && ch.status !== 'writing'}
                  <button class="btn btn-ghost btn-sm" on:click={() => showRevise = !showRevise} disabled={$taskRunning}>{$t('writing.btn.revise')}</button>
                  {#if hasPolishSkills}
                    <button class="btn btn-ghost btn-sm" on:click={doPolish} disabled={$taskRunning} title={$t('writing.btn.polish.tip')}>{$t('writing.btn.polish')}</button>
                  {/if}
                  <button class="btn btn-ghost btn-sm" on:click={copyContent}>{$t('writing.btn.copy')}</button>
                {/if}
                <div class="flex-1"></div>
                <div class="join">
                  <button class="btn btn-ghost btn-xs join-item" on:click={prevChapter} disabled={$selectedChapter <= 0}>{$t('writing.btn.prev')}</button>
                  <button class="btn btn-ghost btn-xs join-item" on:click={nextChapter} disabled={$selectedChapter >= chapters.length - 1}>{$t('writing.btn.next')}</button>
                </div>
              </div>

              {#if showRevise}
                <div class="bg-base-300 rounded-lg p-3 space-y-2">
                  <textarea
                    class="textarea textarea-sm w-full h-20 text-sm"
                    bind:value={reviseFeedback}
                    bind:this={reviseTextareaEl}
                    placeholder={$t('writing.revise.placeholder')}
                    disabled={$taskRunning}
                  ></textarea>
                  <div class="flex justify-between items-center gap-2 flex-wrap">
                    <span class="text-xs text-base-content/40">
                      {#if !(isCurrent && ch.status === 'review')}
                        {$t('writing.revise.hintTargeted')}
                      {:else}
                        {$t('writing.revise.hintCurrent')}
                      {/if}
                      <span class="ml-1 text-base-content/30">· {$t('writing.revise.quoteHint')}</span>
                    </span>
                    <div class="flex gap-2">
                      <button class="btn btn-ghost btn-xs" on:click={() => { showRevise = false; reviseFeedback = ''; }}>{$t('common.cancel')}</button>
                      <button class="btn btn-primary btn-xs" on:click={doRevise} disabled={$taskRunning || !reviseFeedback.trim()}>{$t('writing.revise.submit')}</button>
                    </div>
                  </div>
                </div>
              {/if}
            </div>
          </div>
        {:else}
          <div class="text-center py-16 text-base-content/50 text-base">{$t('writing.emptySelection')}</div>
        {/if}
      </div>
    </div>
  </div>
{/if}
