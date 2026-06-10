<script>
  import { api } from '../lib/api.js';
  import { progress, streamingContent, streamingChapterIdx, taskRunning, addToast, showConfirm, continueAnalysis } from '../lib/stores.js';

  $: p = $progress;
  $: chapters = p?.chapters || [];
  $: hasOutline = chapters.length > 0;
  $: hasAccepted = chapters.some(c => c.status === 'accepted');
  $: inOutlinePhase = p?.phase === 'outline';
  $: pendingCount = chapters.filter(c => c.status === 'pending').length;

  const statusMeta = {
    pending:  { label: '待写作', cls: 'badge-ghost' },
    writing:  { label: '写作中', cls: 'badge-warning' },
    review:   { label: '审核中', cls: 'badge-info' },
    accepted: { label: '已确认', cls: 'badge-success' },
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
      addToast('大纲生成任务已启动', 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function confirmOutline() {
    showConfirm('确认大纲后将进入写作阶段，确定继续？', async () => {
      try {
        await api('POST', '/api/outline/confirm');
        progress.set(await api('GET', '/api/progress'));
        addToast('大纲已确认，进入写作阶段', 'success');
        window.location.hash = '#writing';
      } catch (e) { addToast(e.message, 'error'); }
    });
  }

  async function reviseOutline() {
    const fb = reviseFeedback.trim();
    if (!fb) { addToast('请填写修改意见', 'error'); return; }
    try {
      await api('POST', '/api/outline/revise', { feedback: fb });
      addToast('大纲修订任务已启动（已确认章节不会被改动）', 'info');
      reviseFeedback = '';
      showRevise = false;
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function deleteOutline() {
    showConfirm(`确认删除整个大纲（共 ${chapters.length} 章）？此操作不可恢复！`, async () => {
      try {
        await api('DELETE', '/api/outline');
        progress.set(await api('GET', '/api/progress'));
        addToast('大纲已删除', 'success');
      } catch (e) { addToast(e.message, 'error'); }
    });
  }

  async function generateContinuation() {
    try {
      await api('POST', '/api/outline/generate-continuation', { chapter_count: Number(continuationCount) || 5 });
      addToast('续写大纲生成任务已启动', 'info');
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
    if (!editTitle.trim() || !editOutline.trim()) { addToast('标题和大纲不能为空', 'error'); return; }
    try {
      await api('PUT', '/api/outline/' + editingNum, { title: editTitle.trim(), outline: editOutline.trim() });
      progress.set(await api('GET', '/api/progress'));
      addToast(`第 ${editingNum} 章大纲已更新`, 'success');
      editingNum = -1;
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function importExisting() {
    const content = importContent.trim();
    if (!content) { addToast('请粘贴已有内容', 'error'); return; }
    try {
      await api('POST', '/api/continue/import', { content });
      addToast('内容分析任务已启动，请稍候', 'info');
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
      addToast('续写导入完成', 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }
</script>

<div class="space-y-3">
  {#if !hasOutline}
    <!-- 空状态 -->
    <div class="text-center py-14 text-base-content/50">
      <div class="text-5xl mb-3">📝</div>
      <p class="text-base mb-1">尚未生成大纲</p>
      <p class="text-sm text-base-content/35 mb-6">请先在「配置」页完善故事设定，然后点击下方按钮</p>
      <div class="flex justify-center gap-2">
        <button class="btn btn-primary btn-sm" on:click={generateOutline} disabled={$taskRunning}>✨ 生成大纲</button>
        <button class="btn btn-ghost btn-sm" on:click={() => showImport = !showImport} disabled={$taskRunning}>📥 导入已有内容续写</button>
      </div>
    </div>

    {#if showImport}
      <div class="card bg-base-200 shadow-sm">
        <div class="card-body p-4 gap-2">
          <h3 class="card-title text-base">导入已有内容</h3>
          <p class="text-xs text-base-content/50">粘贴已写好的小说文本（支持「第X章」等常见章节格式），AI 将分析章节结构和故事设定，导入后可继续生成后续章节。</p>
          <textarea class="textarea w-full h-48 text-sm font-serif" bind:value={importContent} placeholder="在此粘贴小说全文..."></textarea>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-xs" on:click={() => { showImport = false; importContent = ''; }}>取消</button>
            <button class="btn btn-primary btn-xs" on:click={importExisting} disabled={$taskRunning || !importContent.trim()}>开始分析</button>
          </div>
        </div>
      </div>
    {/if}

    {#if $continueAnalysis}
      <div class="card bg-base-200 shadow-sm border border-primary/30">
        <div class="card-body p-4 gap-2">
          <h3 class="card-title text-base">分析结果（可在确认前修改）</h3>
          <div class="grid grid-cols-2 gap-2">
            <div>
              <span class="text-xs text-base-content/50 mb-0.5 block">标题</span>
              <input type="text" class="input input-sm w-full" bind:value={$continueAnalysis.title} />
            </div>
            <div>
              <span class="text-xs text-base-content/50 mb-0.5 block">故事类型</span>
              <input type="text" class="input input-sm w-full" bind:value={$continueAnalysis.story_type} />
            </div>
          </div>
          <div>
            <span class="text-xs text-base-content/50 mb-0.5 block">故事梗概</span>
            <textarea class="textarea textarea-sm w-full h-20 text-sm" bind:value={$continueAnalysis.story_synopsis}></textarea>
          </div>
          <div>
            <span class="text-xs text-base-content/50 mb-0.5 block">写作风格</span>
            <textarea class="textarea textarea-sm w-full h-16 text-sm" bind:value={$continueAnalysis.writing_style}></textarea>
          </div>
          <div class="text-xs text-base-content/50">识别到 {$continueAnalysis.chapters?.length || 0} 章</div>
          <div class="max-h-48 overflow-y-auto space-y-1">
            {#each ($continueAnalysis.chapters || []) as ch}
              <div class="bg-base-300 rounded p-2 text-xs">
                <span class="font-medium">第{ch.num}章 《{ch.title}》</span>
                <span class="text-base-content/50">{ch.outline || ch.summary || ''}</span>
              </div>
            {/each}
          </div>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-xs" on:click={() => continueAnalysis.set(null)}>放弃</button>
            <button class="btn btn-success btn-xs" on:click={confirmImport} disabled={$taskRunning}>确认导入</button>
          </div>
        </div>
      </div>
    {/if}
  {:else}
    <!-- 操作栏 -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <div class="flex items-center gap-2 flex-wrap">
          <h3 class="text-base font-semibold flex-1 min-w-0 truncate">📖 {p.title || '未命名'}</h3>
          {#if inOutlinePhase}
            <button class="btn btn-success btn-xs" on:click={confirmOutline} disabled={$taskRunning || chapters.length === 0}>✓ 确认大纲，开始写作</button>
          {/if}
          <button class="btn btn-ghost btn-xs" on:click={() => showRevise = !showRevise} disabled={$taskRunning}>✏️ 提修改意见</button>
          {#if hasAccepted}
            <div class="join">
              <input type="number" min="1" max="50" class="input input-xs join-item w-14" bind:value={continuationCount} />
              <button class="btn btn-primary btn-xs join-item" on:click={generateContinuation} disabled={$taskRunning}>＋生成后续大纲</button>
            </div>
          {:else if inOutlinePhase}
            <button class="btn btn-ghost btn-xs" on:click={generateOutline} disabled={$taskRunning}>🔄 重新生成</button>
          {/if}
          {#if !hasAccepted}
            <button class="btn btn-ghost btn-xs text-error" on:click={deleteOutline} disabled={$taskRunning}>删除大纲</button>
          {/if}
        </div>

        {#if showRevise}
          <div class="bg-base-300 rounded-lg p-3 space-y-2">
            <textarea class="textarea textarea-sm w-full h-20 text-sm" bind:value={reviseFeedback} placeholder="对大纲的修改意见，例如：第 10 章节奏太慢，把冲突提前；结局改为开放式..."></textarea>
            <div class="flex justify-between items-center">
              <span class="text-xs text-base-content/40">已确认章节不会被改动</span>
              <div class="flex gap-2">
                <button class="btn btn-ghost btn-xs" on:click={() => { showRevise = false; reviseFeedback = ''; }}>取消</button>
                <button class="btn btn-primary btn-xs" on:click={reviseOutline} disabled={$taskRunning || !reviseFeedback.trim()}>提交修订</button>
              </div>
            </div>
          </div>
        {/if}

        {#if p.core_prompt}
          <div>
            <span class="text-xs text-base-content/50">核心写作提示词</span>
            <div class="bg-base-300 rounded p-2 text-sm mt-0.5 max-h-24 overflow-y-auto">{p.core_prompt}</div>
          </div>
        {/if}
        {#if p.story_synopsis}
          <div>
            <span class="text-xs text-base-content/50">故事梗概</span>
            <div class="bg-base-300 rounded p-2 text-sm mt-0.5 max-h-24 overflow-y-auto">{p.story_synopsis}</div>
          </div>
        {/if}
      </div>
    </div>

    <!-- 章节大纲列表 -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <div class="flex items-center justify-between">
          <h4 class="text-sm font-semibold text-base-content/60">章节大纲 <span class="font-normal text-base-content/35">（共 {chapters.length} 章{pendingCount ? `，${pendingCount} 章待写作` : ''}）</span></h4>
          <span class="text-xs text-base-content/35">待写作章节可点击编辑</span>
        </div>
        <div class="space-y-1.5">
          {#each chapters as ch (ch.num)}
            {#if editingNum === ch.num}
              <div class="bg-base-300 rounded-lg p-3 space-y-2 ring-1 ring-primary/50">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-bold text-base-content/50 shrink-0">第 {ch.num} 章</span>
                  <input type="text" class="input input-sm flex-1" bind:value={editTitle} placeholder="章节标题" />
                </div>
                <textarea class="textarea textarea-sm w-full h-24 text-sm" bind:value={editOutline} placeholder="本章大纲"></textarea>
                <div class="flex justify-end gap-2">
                  <button class="btn btn-ghost btn-xs" on:click={cancelEdit}>取消</button>
                  <button class="btn btn-success btn-xs" on:click={saveEdit} disabled={$taskRunning}>保存</button>
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
                    <span class="text-xs text-primary opacity-0 group-hover:opacity-100 transition-opacity shrink-0">编辑</span>
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
              <span class="loading loading-dots loading-xs"></span> 正在生成中...
            </div>
            {$streamingContent}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
