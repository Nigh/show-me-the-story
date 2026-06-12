<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { progress, taskRunning, streamingContent, streamingChapterIdx, streamCharCount, selectedChapter, autoConfirm, addToast, confirmModal } from '../lib/stores.js';
  import PostProcessPanel from '../components/PostProcessPanel.svelte';

  // 保留 prop 以兼容 App 传参
  export let sendToChat = async () => {};

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
      addToast(res.enabled ? '已开启自动确认模式：每章生成完成后自动确认并继续下一章' : '已关闭自动确认模式：当前章节完成后停止', 'info');
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
  // 流式期间不对全文做正则统计，直接用 SSE 累计的字数
  $: wordCount = isStreamingThis ? $streamCharCount : (ch?.content ? ch.content.replace(/\s/g, '').length : 0);
  $: totalWords = chapters.reduce((sum, c) => sum + (c.content ? c.content.replace(/\s/g, '').length : 0), 0);

  const statusMeta = {
    pending:  { label: '待写作', cls: 'badge-ghost', dot: 'bg-base-content/20' },
    writing:  { label: '写作中', cls: 'badge-warning', dot: 'bg-warning animate-pulse' },
    review:   { label: '审核中', cls: 'badge-info', dot: 'bg-info' },
    accepted: { label: '已确认', cls: 'badge-success', dot: 'bg-success' },
  };

  let reviseFeedback = '';
  let showRevise = false;
  let contentEl;
  let hasPolishSkills = false;

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
  }

  async function doGenerate() {
    try {
      await api('POST', '/api/chapter/generate');
      addToast(`第 ${ch?.num} 章生成任务已启动`, 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doConfirm() {
    try {
      await api('POST', '/api/chapter/confirm');
      progress.set(await api('GET', '/api/progress'));
      addToast(`第 ${ch?.num} 章已确认`, 'success');
      // 跳到下一章
      const next = await api('GET', '/api/progress');
      if (next.current_chapter_index < (next.chapters || []).length) {
        selectedChapter.set(next.current_chapter_index);
      }
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doRevise() {
    const fb = reviseFeedback.trim();
    if (!fb) { addToast('请填写修改意见', 'error'); return; }
    if (!ch) return;
    try {
      if (isCurrent && ch.status === 'review') {
        // 当前审核中章节：完整修订流程
        await api('POST', '/api/chapter/revise', { feedback: fb });
      } else {
        // 其他章节（含已确认）：定向最小化修订，不影响其他章节
        await api('POST', '/api/chapter/revise/' + ch.num, { feedback: fb });
      }
      addToast(`第 ${ch.num} 章修订任务已启动（仅修改该章）`, 'info');
      reviseFeedback = '';
      showRevise = false;
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doPolish() {
    if (!ch) return;
    try {
      await api('POST', '/api/chapter/polish', { num: ch.num });
      addToast(`第 ${ch.num} 章润色任务已启动`, 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function copyContent() {
    if (!ch?.content) return;
    try {
      await navigator.clipboard.writeText(ch.content);
      addToast('本章正文已复制', 'success');
    } catch (e) { addToast('复制失败', 'error'); }
  }

  function exportBook() {
    const written = chapters.filter(c => c.content);
    if (written.length === 0) { addToast('暂无已写章节可导出', 'error'); return; }
    const parts = [`《${p.title || '未命名'}》\n`];
    for (const c of written) {
      parts.push(`\n\n第 ${c.num} 章　${c.title}\n\n${c.content}`);
    }
    const blob = new Blob([parts.join('')], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${p.title || '小说'}.txt`;
    a.click();
    URL.revokeObjectURL(url);
    addToast(`已导出 ${written.length} 章`, 'success');
  }

  function prevChapter() { if ($selectedChapter > 0) selectChapter($selectedChapter - 1); }
  function nextChapter() { if ($selectedChapter < chapters.length - 1) selectChapter($selectedChapter + 1); }

  function smoothTransitions() {
    confirmModal.set({
      message: '将逐章检查已确认章节之间的衔接，仅在生硬时由 AI 最小化重写本章开头片段（不改动正文主体），每章处理完立即保存，可随时停止。是否开始？',
      onConfirm: async () => {
        try {
          await api('POST', '/api/chapters/smooth-transitions');
          addToast('章节衔接优化任务已启动', 'info');
        } catch (e) { addToast(e.message, 'error'); }
      },
    });
  }
</script>

{#if !inWriting}
  <div class="text-center py-16 text-base-content/50">
    <div class="text-5xl mb-4">✍️</div>
    <p class="text-base mb-1">尚未进入写作阶段</p>
    <p class="text-sm text-base-content/35 mb-6">请先在「大纲」页生成并确认大纲</p>
    <button class="btn btn-primary btn-sm" on:click={() => window.location.hash = '#outline'}>前往大纲页</button>
  </div>
{:else}
  <div class="space-y-3">
    <!-- 进度 -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <div class="flex items-center gap-3">
          <h2 class="card-title text-base flex-1">写作进度</h2>
          <label class="flex items-center gap-1.5 cursor-pointer" title="开启后：每章生成完成自动确认，并继续生成下一章，直到全部完成或关闭开关">
            <input type="checkbox" class="toggle toggle-xs toggle-success" checked={$autoConfirm} on:change={toggleAutoConfirm} />
            <span class="text-xs text-base-content/60">自动确认模式</span>
          </label>
          <span class="text-xs text-base-content/40">全书约 {totalWords.toLocaleString()} 字</span>
          {#if accepted >= 2}
            <button class="btn btn-ghost btn-xs" on:click={smoothTransitions} disabled={$taskRunning} title="逐章检查并修补已确认章节之间的衔接，适合修补旧项目">🪡 优化章节衔接</button>
          {/if}
          <button class="btn btn-ghost btn-xs" on:click={exportBook}>📤 导出 TXT</button>
        </div>
        <progress class="progress progress-primary w-full" value={pct} max="100"></progress>
        <div class="text-sm text-base-content/50">{pct}%（已确认 {accepted} / {total} 章）</div>
      </div>
    </div>

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
                  <span class="badge badge-primary badge-xs shrink-0">当前</span>
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
                <h2 class="card-title text-base flex-1 min-w-0">第 {ch.num} 章 · {ch.title}</h2>
                <span class="badge badge-sm {statusMeta[ch.status]?.cls || 'badge-ghost'}">{statusMeta[ch.status]?.label || ch.status}</span>
                {#if wordCount > 0}
                  <span class="text-xs text-base-content/40">{wordCount.toLocaleString()} 字</span>
                {/if}
              </div>

              {#if ch.outline}
                <details class="bg-base-300 rounded">
                  <summary class="p-2 text-xs text-base-content/50 cursor-pointer select-none">本章大纲</summary>
                  <div class="px-2 pb-2 text-sm text-base-content/70">{ch.outline}</div>
                </details>
              {/if}

              {#if ch.summary}
                <details class="bg-base-300 rounded">
                  <summary class="p-2 text-xs text-base-content/50 cursor-pointer select-none">本章摘要</summary>
                  <div class="px-2 pb-2 text-sm text-base-content/70 whitespace-pre-wrap">{ch.summary}</div>
                </details>
              {/if}

              {#if displayContent}
                {#if isStreamingThis}
                  <div class="text-xs text-warning/80 flex items-center gap-1.5">
                    <span class="loading loading-dots loading-xs"></span>
                    生成中 · 为保证页面流畅仅显示最新内容，完成后展示全文
                  </div>
                {/if}
                <div bind:this={contentEl} class="bg-base-300 rounded-lg p-4 text-[15px] chapter-content reading-area max-h-[calc(100vh-420px)] min-h-[200px] overflow-y-auto">
                  {displayContent}
                  {#if isStreamingThis}
                    <span class="inline-block w-2 h-4 bg-primary/70 animate-pulse ml-0.5 align-text-bottom"></span>
                  {/if}
                </div>
              {:else if ch.status === 'pending'}
                <div class="bg-base-300 rounded-lg p-6 text-center text-sm text-base-content/40">
                  {#if isCurrent}
                    本章尚未生成，点击下方「生成本章」开始创作
                  {:else}
                    本章尚未生成（按顺序写作，当前进行到第 {chapters[currentIdx]?.num ?? '-'} 章）
                  {/if}
                </div>
              {/if}

              <!-- 操作 -->
              <div class="flex gap-2 flex-wrap items-center mt-1">
                {#if ch.status === 'pending' && isCurrent}
                  <button class="btn btn-primary btn-sm" on:click={doGenerate} disabled={$taskRunning}>✨ 生成本章</button>
                {/if}
                {#if ch.status === 'review' && isCurrent}
                  <button class="btn btn-success btn-sm" on:click={doConfirm} disabled={$taskRunning}>✓ 确认本章</button>
                {/if}
                {#if ch.content && ch.status !== 'writing'}
                  <button class="btn btn-ghost btn-sm" on:click={() => showRevise = !showRevise} disabled={$taskRunning}>✏️ 修改本章</button>
                  {#if hasPolishSkills}
                    <button class="btn btn-ghost btn-sm" on:click={doPolish} disabled={$taskRunning} title="需启用 polish 类技能">✨ 去AI味</button>
                  {/if}
                  <button class="btn btn-ghost btn-sm" on:click={copyContent}>📋 复制</button>
                {/if}
                <div class="flex-1"></div>
                <div class="join">
                  <button class="btn btn-ghost btn-xs join-item" on:click={prevChapter} disabled={$selectedChapter <= 0}>← 上一章</button>
                  <button class="btn btn-ghost btn-xs join-item" on:click={nextChapter} disabled={$selectedChapter >= chapters.length - 1}>下一章 →</button>
                </div>
              </div>

              {#if showRevise}
                <div class="bg-base-300 rounded-lg p-3 space-y-2">
                  <textarea
                    class="textarea textarea-sm w-full h-20 text-sm"
                    bind:value={reviseFeedback}
                    placeholder="修改意见，例如：第三段对话太生硬，改得口语化一些；把主角的剑改成长枪..."
                    disabled={$taskRunning}
                  ></textarea>
                  <div class="flex justify-between items-center">
                    <span class="text-xs text-base-content/40">
                      {#if !(isCurrent && ch.status === 'review')}
                        定向修订：仅修改本章，不影响其他章节和大纲
                      {:else}
                        修订当前章节，必要时会同步调整后续未写章节的大纲
                      {/if}
                    </span>
                    <div class="flex gap-2">
                      <button class="btn btn-ghost btn-xs" on:click={() => { showRevise = false; reviseFeedback = ''; }}>取消</button>
                      <button class="btn btn-primary btn-xs" on:click={doRevise} disabled={$taskRunning || !reviseFeedback.trim()}>提交修订</button>
                    </div>
                  </div>
                </div>
              {/if}
            </div>
          </div>
        {:else}
          <div class="text-center py-16 text-base-content/50 text-base">选择一个章节查看</div>
        {/if}
      </div>
    </div>
  </div>
{/if}
