<script>
  import { api } from '../lib/api.js';
  import { progress, taskRunning, streamingContent, streamingChapterIdx, continueAnalysis, editingChapterNum, addToast } from '../lib/stores.js';

  let showImport = false;
  let showFeedback = false;
  let feedbackText = '';
  let importContent = '';
  let caTitle = '', caWritingStyle = '', caCorePrompt = '', caStorySynopsis = '';
  let editTitle = '', editOutline = '';

  $: p = $progress;
  $: hasOutline = p?.chapters?.length > 0;
  $: hasContent = p?.chapters?.some(c => c.content) || false;
  $: hasAccepted = p?.chapters?.some(c => c.status === 'accepted') || false;
  $: hasPending = p?.chapters?.some(c => c.status === 'pending') || false;
  $: inOutline = p?.phase === 'outline';
  $: showContBtn = inOutline && hasAccepted && !hasPending && !$taskRunning;

  $: if ($continueAnalysis) {
    const d = $continueAnalysis;
    caTitle = d.title || '';
    caWritingStyle = d.writing_style || '';
    caCorePrompt = d.core_prompt || '';
    caStorySynopsis = d.story_synopsis || '';
  }

  const statusIcons = { pending: '', writing: '⏳', review: '👀', accepted: '✅' };

  async function doGenerate() {
    try { await api('POST', '/api/outline/generate'); addToast('大纲生成中...', 'info'); } catch (e) { addToast(e.message, 'error'); }
  }

  async function doImport() {
    if (!importContent.trim()) { addToast('请粘贴已有内容', 'error'); return; }
    try { await api('POST', '/api/continue/import', { content: importContent }); addToast('内容分析中...', 'info'); } catch (e) { addToast(e.message, 'error'); }
  }

  async function doConfirmContinue() {
    const d = { ...$continueAnalysis, title: caTitle, writing_style: caWritingStyle, core_prompt: caCorePrompt, story_synopsis: caStorySynopsis };
    try {
      await api('POST', '/api/continue/confirm', d);
      addToast('导入成功', 'success');
      continueAnalysis.set(null);
      showImport = false;
      importContent = '';
      progress.set(await api('GET', '/api/progress'));
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doConfirm() {
    try { await api('POST', '/api/outline/confirm'); addToast('大纲已确认', 'success'); progress.set(await api('GET', '/api/progress')); } catch (e) { addToast(e.message, 'error'); }
  }

  async function doRevise() {
    if (!feedbackText.trim()) return;
    try {
      await api('POST', '/api/outline/revise', { feedback: feedbackText });
      feedbackText = ''; showFeedback = false;
      addToast('大纲修订中...', 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function doContinuation() {
    try { await api('POST', '/api/outline/generate-continuation', { chapter_count: 5 }); addToast('续写大纲生成中...', 'info'); } catch (e) { addToast(e.message, 'error'); }
  }

  async function doDelete() {
    try { await api('DELETE', '/api/outline'); addToast('大纲已删除', 'success'); progress.set(await api('GET', '/api/progress')); } catch (e) { addToast(e.message, 'error'); }
  }

  function startEditChapter(ch) {
    editTitle = ch.title || '';
    editOutline = ch.outline || '';
    editingChapterNum.set(ch.num);
  }

  async function saveChapter(num) {
    const title = editTitle.trim();
    const outline = editOutline.trim();
    if (!title) { addToast('标题不能为空', 'error'); return; }
    try {
      await api('PUT', '/api/outline/' + num, { title, outline });
      $editingChapterNum = -1;
      addToast('大纲已更新', 'success');
      progress.set(await api('GET', '/api/progress'));
    } catch (e) { addToast(e.message, 'error'); }
  }
</script>

<div class="space-y-3">
  {#if !hasOutline && !showImport && !$continueAnalysis}
    <div class="text-center py-20 text-base-content/40">
      <div class="text-5xl mb-3">📝</div>
      <p class="text-sm mb-4">尚未生成大纲。点击下方按钮开始生成，或导入已有内容。</p>
      <div class="flex gap-2 justify-center">
        <button class="btn btn-primary btn-sm" on:click={doGenerate} disabled={hasOutline || $taskRunning}>生成大纲</button>
        <button class="btn btn-outline btn-sm" on:click={() => { showImport = true; }}>导入已有内容</button>
      </div>
    </div>
  {/if}

  {#if showImport && !$continueAnalysis}
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <h3 class="card-title text-sm">导入已有内容</h3>
        <textarea class="textarea w-full h-48 text-sm" bind:value={importContent} placeholder="将已有小说文本粘贴到这里..."></textarea>
        <div class="flex gap-1.5 justify-end">
          <button class="btn btn-primary btn-xs" on:click={doImport}>分析内容</button>
          <button class="btn btn-ghost btn-xs" on:click={() => { showImport = false; }}>取消</button>
        </div>
      </div>
    </div>
  {/if}

  {#if $continueAnalysis}
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <h3 class="card-title text-sm">分析结果</h3>
        <div class="grid grid-cols-2 gap-x-3 gap-y-1.5">
          <div>
            <label class="text-[11px] text-base-content/50 mb-0.5 block">小说标题</label>
            <input type="text" class="input input-sm w-full" bind:value={caTitle} />
          </div>
          <div>
            <label class="text-[11px] text-base-content/50 mb-0.5 block">写作风格</label>
            <input type="text" class="input input-sm w-full" bind:value={caWritingStyle} />
          </div>
          <div>
            <label class="text-[11px] text-base-content/50 mb-0.5 block">核心写作提示词</label>
            <textarea class="textarea textarea-sm w-full h-14" bind:value={caCorePrompt}></textarea>
          </div>
          <div>
            <label class="text-[11px] text-base-content/50 mb-0.5 block">故事梗概</label>
            <textarea class="textarea textarea-sm w-full h-14" bind:value={caStorySynopsis}></textarea>
          </div>
        </div>
        {#if $continueAnalysis.chapters?.length > 0}
          <h4 class="text-xs font-semibold mt-1 text-base-content/60">已有章节大纲</h4>
          <div class="overflow-x-auto">
            <table class="table table-xs">
              <thead><tr><th>#</th><th>标题</th><th>大纲</th><th>摘要</th></tr></thead>
              <tbody>
                {#each $continueAnalysis.chapters as ch}
                  <tr><td>{ch.num}</td><td>{ch.title}</td><td class="max-w-xs truncate">{ch.outline || ''}</td><td class="max-w-xs truncate">{ch.summary || ''}</td></tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
        <div class="flex gap-1.5 justify-end">
          <button class="btn btn-success btn-xs" on:click={doConfirmContinue}>确认导入</button>
          <button class="btn btn-ghost btn-xs" on:click={() => { continueAnalysis.set(null); showImport = true; }}>重新分析</button>
        </div>
      </div>
    </div>
  {/if}

  {#if hasOutline && !$continueAnalysis}
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <h3 class="text-sm font-semibold">📖 {p.title || ''}</h3>
        {#if p.core_prompt}
          <div>
            <span class="text-[11px] text-base-content/50">核心写作提示词</span>
            <div class="bg-base-300 rounded p-2 text-xs mt-0.5">{p.core_prompt}</div>
          </div>
        {/if}
        {#if p.story_synopsis}
          <div>
            <span class="text-[11px] text-base-content/50">故事梗概</span>
            <div class="bg-base-300 rounded p-2 text-xs mt-0.5">{p.story_synopsis}</div>
          </div>
        {/if}
        <h4 class="text-xs font-semibold mt-1 text-base-content/60">章节大纲</h4>
        <div class="overflow-x-auto">
          <table class="table table-xs">
            <thead><tr><th>#</th><th>标题</th><th>大纲</th><th>状态</th><th>操作</th></tr></thead>
            <tbody>
              {#each p.chapters as ch}
                {#if $editingChapterNum === ch.num}
                  <tr class="bg-base-300">
                    <td>{ch.num}</td>
                    <td colspan="4">
                      <div class="flex flex-col gap-1">
                        <input type="text" class="input input-xs w-full" bind:value={editTitle} placeholder="章节标题" />
                        <textarea class="textarea textarea-xs w-full h-12" bind:value={editOutline} placeholder="章节大纲"></textarea>
                        <div class="flex gap-1">
                          <button class="btn btn-success btn-xs" on:click={() => saveChapter(ch.num)}>保存</button>
                          <button class="btn btn-ghost btn-xs" on:click={() => editingChapterNum.set(-1)}>取消</button>
                        </div>
                      </div>
                    </td>
                  </tr>
                {:else}
                  <tr>
                    <td>{ch.num}</td>
                    <td>{ch.title}</td>
                    <td class="max-w-md">{ch.outline}</td>
                    <td>{statusIcons[ch.status] || ''}</td>
                    <td>
                      {#if ch.status === 'pending'}
                        <button class="btn btn-ghost btn-xs px-1" on:click={() => startEditChapter(ch)}>✏️</button>
                      {/if}
                    </td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>

        {#if $streamingChapterIdx >= 0 && $streamingContent}
          <div class="bg-base-300 rounded p-3 mt-1 text-xs max-h-48 overflow-y-auto chapter-content">
            <div class="text-[11px] text-base-content/40 mb-1">正在生成中...</div>
            {$streamingContent}
          </div>
        {/if}

        <div class="flex gap-1.5 flex-wrap">
          <button class="btn btn-success btn-xs" on:click={doConfirm} disabled={!inOutline || $taskRunning}>确认大纲</button>
          {#if showContBtn}
            <button class="btn btn-primary btn-xs" on:click={doContinuation} disabled={$taskRunning}>生成后续大纲</button>
          {/if}
          <button class="btn btn-outline btn-xs" on:click={() => showFeedback = !showFeedback} disabled={!inOutline || $taskRunning}>修订大纲</button>
          <button class="btn btn-error btn-xs" on:click={doDelete} disabled={hasContent || $taskRunning}>删除大纲</button>
        </div>
        {#if showFeedback}
          <div class="flex gap-1.5">
            <input type="text" class="input input-sm flex-1" bind:value={feedbackText} placeholder="输入修改意见..." on:keydown={e => e.key === 'Enter' && doRevise()} />
            <button class="btn btn-primary btn-xs" on:click={doRevise}>提交修订</button>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
