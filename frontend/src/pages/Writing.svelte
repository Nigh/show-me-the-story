<script>
  import { api } from '../lib/api.js';
  import { progress, taskRunning, streamingContent, streamingChapterIdx, selectedChapter, addToast } from '../lib/stores.js';

  let showChFeedback = false;
  let chFeedbackText = '';

  $: p = $progress;
  $: inWriting = p?.phase === 'writing';
  $: chapters = p?.chapters || [];
  $: total = chapters.length;
  $: accepted = chapters.filter(c => c.status === 'accepted').length;
  $: pct = total > 0 ? Math.round(accepted / total * 100) : 0;
  $: ch = $selectedChapter >= 0 && $selectedChapter < chapters.length ? chapters[$selectedChapter] : null;
  $: isCurrent = ch && p?.current_chapter_index === $selectedChapter;
  $: isLast = ch && $selectedChapter === chapters.length - 1;
  $: noWritingAfter = ch && chapters.slice($selectedChapter).every(c => c.status !== 'writing');

  $: displayContent = ($streamingChapterIdx === $selectedChapter && $streamingContent) ? $streamingContent : (ch?.content || '');

  const statusIcons = { pending: '', writing: '⏳', review: '👀', accepted: '✅' };

  function selectChapter(i) {
    selectedChapter.set(i);
    showChFeedback = false;
  }

  async function doGenerate() {
    try { await api('POST', '/api/chapter/generate'); addToast('章节生成中...', 'info'); } catch (e) { addToast(e.message, 'error'); }
  }

  async function doConfirm() {
    try { await api('POST', '/api/chapter/confirm'); addToast('章节已确认', 'success'); progress.set(await api('GET', '/api/progress')); } catch (e) { addToast(e.message, 'error'); }
  }

  async function doRevise() {
    if (!chFeedbackText.trim()) return;
    try {
      await api('POST', '/api/chapter/revise', { feedback: chFeedbackText });
      chFeedbackText = ''; showChFeedback = false;
      addToast('章节修订中...', 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doPolish() {
    try { await api('POST', '/api/chapter/polish'); addToast('去AI味处理中...', 'info'); } catch (e) { addToast(e.message, 'error'); }
  }

  async function doDelete() {
    try {
      await api('DELETE', '/api/chapter');
      addToast('章节已删除', 'success');
      const newP = await api('GET', '/api/progress');
      progress.set(newP);
      if (!newP.chapters?.length) selectedChapter.set(-1);
      else if ($selectedChapter >= newP.chapters.length) selectedChapter.set(newP.chapters.length - 1);
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doDeleteFrom() {
    if (!ch) return;
    if (!confirm(`确认从第 ${ch.num} 章开始删除到末尾？此操作不可撤销。`)) return;
    try {
      await api('DELETE', '/api/chapters/from/' + ch.num);
      addToast('章节已删除', 'success');
      selectedChapter.set(-1);
      progress.set(await api('GET', '/api/progress'));
    } catch (e) { addToast(e.message, 'error'); }
  }
</script>

{#if !inWriting}
  <div class="text-center py-16 text-base-content/50">
    <div class="text-5xl mb-4">✍️</div>
    <p>请先完成大纲确认，再进入写作阶段。</p>
  </div>
{:else}
  <div class="space-y-3">
    <!-- Progress -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <h2 class="card-title text-sm">写作进度</h2>
        <progress class="progress progress-primary w-full" value={pct} max="100"></progress>
        <div class="text-xs text-base-content/50">{pct}% ({accepted}/{total})</div>
      </div>
    </div>

    <!-- Chapter viewer -->
    <div class="grid grid-cols-[280px_1fr] gap-4" style="min-height:500px">
      <!-- Chapter list -->
      <div class="card bg-base-200 shadow-sm overflow-y-auto max-h-[600px]">
        <ul class="menu menu-sm p-0">
          {#each chapters as c, i}
            <!-- svelte-ignore a11y-click-events-have-key-events -->
            <!-- svelte-ignore a11y-no-static-element-interactions -->
            <li>
              <button class="flex gap-2 {$selectedChapter === i ? 'active' : ''}" on:click={() => selectChapter(i)}>
                <span class="w-6 text-center">{statusIcons[c.status] || ''}</span>
                <span class="text-base-content/50 w-8">{c.num}</span>
                <span class="flex-1 text-left truncate">{c.title}</span>
              </button>
            </li>
          {/each}
        </ul>
      </div>

      <!-- Content area -->
      <div class="space-y-3">
        {#if ch}
          <div class="card bg-base-200 shadow-sm">
            <div class="card-body p-4">
              <h2 class="card-title text-sm">第 {ch.num} 章: {ch.title}</h2>

              {#if ch.summary}
                <div class="bg-base-300 rounded p-2 text-xs text-base-content/70">{ch.summary}</div>
              {/if}

              {#if displayContent}
                <div class="bg-base-300 rounded p-3 text-sm chapter-content max-h-[600px] overflow-y-auto">{displayContent}</div>
              {/if}

              <div class="flex gap-2 flex-wrap mt-2">
                {#if ch.status === 'pending' && isCurrent}
                  <button class="btn btn-primary btn-sm" on:click={doGenerate} disabled={$taskRunning}>生成本章</button>
                {/if}
                {#if ch.status === 'review'}
                  <button class="btn btn-success btn-sm" on:click={doConfirm} disabled={$taskRunning}>确认本章</button>
                {/if}
                {#if ch.status === 'review' || ch.status === 'writing'}
                  <button class="btn btn-outline btn-sm" on:click={() => showChFeedback = !showChFeedback} disabled={$taskRunning}>修改本章</button>
                {/if}
                {#if ch.status === 'review' && ch.content}
                  <button class="btn btn-outline btn-sm" on:click={doPolish} disabled={$taskRunning}>去AI味</button>
                {/if}
                {#if isLast && ch.status !== 'writing' && ch.status !== 'pending'}
                  <button class="btn btn-error btn-sm" on:click={doDelete} disabled={$taskRunning}>删除本章</button>
                {/if}
                {#if !isLast && ch.status !== 'writing' && noWritingAfter && $selectedChapter < chapters.length - 1}
                  <button class="btn btn-error btn-sm" on:click={doDeleteFrom} disabled={$taskRunning}>从此章删除到末尾</button>
                {/if}
              </div>

              {#if showChFeedback}
                <div class="flex gap-2 mt-2">
                  <input type="text" class="input input-sm flex-1" bind:value={chFeedbackText} placeholder="输入修改意见..." on:keydown={e => e.key === 'Enter' && doRevise()} />
                  <button class="btn btn-primary btn-sm" on:click={doRevise}>提交修改</button>
                </div>
              {/if}
            </div>
          </div>
        {:else}
          <div class="text-center py-16 text-base-content/50">选择一个章节查看</div>
        {/if}
      </div>
    </div>
  </div>
{/if}
