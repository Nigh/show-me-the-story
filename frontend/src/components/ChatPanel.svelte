<script>
  import { onMount, afterUpdate } from 'svelte';
  import { api } from '../lib/api.js';
  import { chatSessions, currentChatSession, addToast, showConfirm, taskRunning, lastFailedTask, logEntries, currentTaskName } from '../lib/stores.js';

  export let contextPage = 'config';

  let chatInput = '';
  let messagesContainer;
  let inputEl;
  let showSessionList = false;
  let autoScroll = true;

  $: sessions = ($chatSessions?.sessions || []);
  $: msgs = ($currentChatSession?.messages || []);
  $: streamingText = $currentChatSession?.streaming_text || '';
  $: pendingTools = $currentChatSession?.pending_tool_calls || [];
  $: taskLogs = ($logEntries || []).slice(-20);
  let taskStatusCollapsed = false;

  // 工具名中文映射
  const toolNames = {
    read_characters: '读取角色列表', read_character: '读取角色详情', read_worldview: '读取世界观',
    read_organizations: '读取组织', read_chapter: '读取章节', read_outline: '读取大纲',
    read_foreshadows: '读取伏笔', search_project: '搜索项目', read_project_config: '读取故事配置',
    read_skills: '读取技能',
    create_character: '创建角色', update_character: '更新角色',
    create_worldview: '创建世界观条目', update_worldview: '更新世界观条目',
    create_organization: '创建组织', update_organization: '更新组织',
    create_relation: '创建关系', update_relation: '更新关系',
    create_foreshadow: '创建伏笔', update_foreshadow: '更新伏笔',
    update_project_config: '更新故事配置', toggle_skill: '切换技能',
    generate_outline: '生成大纲', confirm_outline: '确认大纲', revise_outline: '修订大纲',
    edit_chapter_outline: '编辑章节大纲', generate_chapter: '生成章节', confirm_chapter: '确认章节',
    revise_chapter: '修订章节', suggest_foreshadows: 'AI 伏笔建议',
    delete_character: '删除角色', delete_worldview: '删除世界观条目', delete_organization: '删除组织',
    delete_relation: '删除关系', delete_foreshadow: '删除伏笔',
    delete_chapter: '删除章节', delete_chapters_from: '批量删除章节',
    delete_outline: '删除大纲', reset_progress: '重置进度',
  };
  const dangerTools = new Set(['delete_chapter', 'delete_chapters_from', 'delete_outline', 'reset_progress']);

  function toolLabel(name) { return toolNames[name] || name; }
  function fmtArgs(args) {
    const s = typeof args === 'string' ? args : JSON.stringify(args);
    if (!s || s === '{}' || s === 'null') return '';
    return s.length > 300 ? s.slice(0, 300) + '...' : s;
  }
  function fmtTime(ts) {
    if (!ts) return '';
    try { return new Date(ts).toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit' }); } catch { return ''; }
  }

  // 重试 API 端点映射
  const retryEndpoints = {
    'outline_generation': { method: 'POST', url: '/api/outline/generate' },
    'outline_revision': { method: 'POST', url: '/api/outline/revise' },
    'chapter_generation': { method: 'POST', url: '/api/chapter/generate' },
    'chapter_revision': { method: 'POST', url: '/api/chapter/revise' },
    'foreshadow_suggest': { method: 'POST', url: '/api/foreshadows/suggest' },
    'continuation_outline': { method: 'POST', url: '/api/outline/generate-continuation' },
    'settings_reconciliation': { method: 'POST', url: '/api/settings/reconcile' },
  };

  function isHallucinatedWait(msg, allMsgs, idx) {
    if (msg.role !== 'assistant' || !msg.content) return false;
    if (msg.tool_calls?.length > 0) return false;
    const waitPattern = /请(耐心)?等待|请稍等|正在生成|等待完成/;
    if (!waitPattern.test(msg.content)) return false;
    for (let i = idx - 1; i >= 0; i--) {
      if (allMsgs[i].role === 'user') break;
      if (allMsgs[i].role === 'assistant' && allMsgs[i].tool_calls?.length > 0) return false;
    }
    return true;
  }

  function parseContentSegments(text) {
    if (!text) return [{ type: 'text', content: '' }];
    const segments = [];
    const regex = /<tool_call>([\s\S]*?)<\/tool_call>|<tool_call>([\s\S]*)/g;
    let lastIdx = 0;
    let match;
    while ((match = regex.exec(text)) !== null) {
      if (match.index > lastIdx) {
        segments.push({ type: 'text', content: text.slice(lastIdx, match.index) });
      }
      const jsonStr = (match[1] || match[2] || '').trim();
      try {
        const tc = JSON.parse(jsonStr);
        segments.push({ type: 'tool_call', name: tc.name || tc.tool || '未知工具', args: tc.arguments || tc.args || {} });
      } catch {
        // 未闭合/不完整的 tool_call（流式中途），按工具调用占位显示
        segments.push({ type: 'tool_call', name: '准备调用工具', args: '' });
      }
      lastIdx = match.index + match[0].length;
    }
    if (lastIdx < text.length) {
      segments.push({ type: 'text', content: text.slice(lastIdx) });
    }
    return segments;
  }

  onMount(async () => {
    try {
      chatSessions.set(await api('GET', '/api/chat/sessions'));
      if (!$currentChatSession) {
        if (sessions.length > 0) {
          await selectSession(sessions[0].id);
        } else {
          await createSession();
        }
      }
    } catch (e) {}
  });

  function handleScroll() {
    if (!messagesContainer) return;
    const nearBottom = messagesContainer.scrollHeight - messagesContainer.scrollTop - messagesContainer.clientHeight < 80;
    autoScroll = nearBottom;
  }

  afterUpdate(() => {
    if (messagesContainer && autoScroll) messagesContainer.scrollTop = messagesContainer.scrollHeight;
  });

  export async function sendMessageToChat(text) {
    if (!$currentChatSession) {
      await createSession();
    }
    chatInput = text;
    await sendMessage();
  }

  async function createSession() {
    try {
      const session = await api('POST', '/api/chat/sessions');
      chatSessions.set(await api('GET', '/api/chat/sessions'));
      await selectSession(session.id);
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function selectSession(id) {
    try {
      const session = await api('GET', '/api/chat/sessions/' + id);
      currentChatSession.set(session);
      showSessionList = false;
      autoScroll = true;
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function deleteSession(id, e) {
    e.stopPropagation();
    showConfirm('确认删除此会话？', async () => {
      try {
        await api('DELETE', '/api/chat/sessions/' + id);
        chatSessions.set(await api('GET', '/api/chat/sessions'));
        if ($currentChatSession?.id === id) {
          currentChatSession.set(null);
          const updated = (await api('GET', '/api/chat/sessions')).sessions || [];
          if (updated.length > 0) await selectSession(updated[0].id);
        }
      } catch (e) { addToast(e.message, 'error'); }
    });
  }

  async function sendMessage() {
    if ($taskRunning) { addToast('有任务正在运行，请等待完成', 'error'); return; }
    if (!$currentChatSession) { addToast('请先选择会话', 'error'); return; }
    const msg = chatInput.trim();
    if (!msg) return;
    chatInput = '';
    if (inputEl) inputEl.style.height = 'auto';
    autoScroll = true;

    currentChatSession.update(s => {
      if (!s) return s;
      const messages = [...(s.messages || []), { role: 'user', content: msg, timestamp: new Date().toISOString() }];
      return { ...s, messages, streaming_text: '', pending_tool_calls: [] };
    });

    try {
      await api('POST', '/api/chat/sessions/' + $currentChatSession.id + '/messages', { content: msg, context_page: contextPage });
    } catch (e) { addToast(e.message, 'error'); }
  }

  function handleKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
  }

  function autoGrow() {
    if (!inputEl) return;
    inputEl.style.height = 'auto';
    inputEl.style.height = Math.min(inputEl.scrollHeight, 120) + 'px';
  }

  async function stopTask() {
    try {
      await api('POST', '/api/task/stop');
      addToast('正在停止任务...', 'info');
    } catch (e) {}
  }

  async function retryTask() {
    const failed = $lastFailedTask;
    if (!failed) return;
    lastFailedTask.set(null);

    if (failed.task === 'chat_message') {
      if ($currentChatSession?.messages?.length > 0) {
        const lastUserMsg = [...$currentChatSession.messages].reverse().find(m => m.role === 'user');
        if (lastUserMsg) {
          chatInput = lastUserMsg.content;
          await sendMessage();
          return;
        }
      }
      addToast('无法重试：找不到上次发送的消息', 'error');
      return;
    }

    const endpoint = retryEndpoints[failed.task];
    if (endpoint) {
      try {
        await api(endpoint.method, endpoint.url);
      } catch (e) { addToast('重试失败: ' + e.message, 'error'); }
    } else {
      addToast('此任务类型不支持自动重试', 'error');
    }
  }
</script>

<div class="flex flex-col h-full">
  <!-- 会话栏 -->
  <div class="border-b border-base-content/10 px-3 py-2 flex items-center gap-2 shrink-0">
    <button class="btn btn-ghost btn-xs" on:click={() => showSessionList = !showSessionList}>
      {showSessionList ? '收起' : '☰ 会话'}
    </button>
    <span class="text-sm text-base-content/50 truncate flex-1">
      {$currentChatSession?.title || '未选择会话'}
    </span>
    {#if $taskRunning}
      <button class="btn btn-error btn-xs gap-1" on:click={stopTask}>⏹ 停止</button>
    {/if}
    <button class="btn btn-primary btn-xs" on:click={createSession} disabled={$taskRunning}>＋ 新建</button>
  </div>

  {#if showSessionList}
    <div class="border-b border-base-content/10 max-h-[200px] overflow-y-auto bg-base-200 shrink-0">
      {#each sessions as s}
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div
          class="px-3 py-2 border-b border-base-content/5 cursor-pointer hover:bg-base-300 transition-colors flex items-center gap-2 group"
          class:bg-base-300={$currentChatSession?.id === s.id}
          on:click={() => selectSession(s.id)}
        >
          <div class="flex-1 min-w-0">
            <div class="text-sm font-medium truncate">{s.title}</div>
            <div class="text-xs text-base-content/40">{new Date(s.updated_at).toLocaleString('zh-CN')} · {s.msg_count || 0} 条</div>
          </div>
          <button class="btn btn-ghost btn-xs text-error opacity-0 group-hover:opacity-100 transition-opacity" on:click={(e) => deleteSession(s.id, e)}>删除</button>
        </div>
      {/each}
      {#if sessions.length === 0}
        <div class="px-3 py-2 text-sm text-base-content/40">暂无会话</div>
      {/if}
    </div>
  {/if}

  <!-- 任务状态 -->
  {#if $taskRunning || taskLogs.length > 0}
    <div class="border-b border-base-content/10 shrink-0">
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div class="flex items-center gap-2 px-3 py-1.5 cursor-pointer hover:bg-base-300/50" on:click={() => taskStatusCollapsed = !taskStatusCollapsed}>
        {#if $taskRunning}
          <span class="loading loading-spinner loading-xs text-warning"></span>
        {:else}
          <span class="text-success text-xs">●</span>
        {/if}
        <span class="text-xs font-semibold text-base-content/70">{$currentTaskName || '任务'}{$taskRunning ? ' 进行中' : ' 已结束'}</span>
        <span class="text-xs text-base-content/40 ml-auto">{taskStatusCollapsed ? '展开 ▾' : '收起 ▴'}</span>
      </div>
      {#if !taskStatusCollapsed && taskLogs.length > 0}
        <div class="max-h-[150px] overflow-y-auto px-3 py-1 font-mono text-xs leading-relaxed space-y-0.5">
          {#each taskLogs as entry}
            <div class="flex gap-2">
              <span class="text-base-content/30 shrink-0">{entry.time}</span>
              <span class={entry.level === 'error' ? 'text-error' : entry.level === 'warn' ? 'text-warning' : entry.level === 'success' ? 'text-success' : 'text-base-content/60'}>{entry.msg}</span>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- 消息区 -->
  <div bind:this={messagesContainer} on:scroll={handleScroll} class="flex-1 overflow-y-auto p-3 space-y-2">
    {#if !$currentChatSession}
      <div class="text-center text-base-content/40 py-8 text-base">选择或创建一个会话开始对话</div>
    {:else}
      {#if msgs.length === 0 && !streamingText}
        <div class="text-center text-base-content/40 py-10 space-y-3">
          <div class="text-3xl">💬</div>
          <p class="text-sm">我是创作助理，可以帮你管理设定、生成大纲和章节。</p>
          <div class="flex flex-wrap justify-center gap-1.5 px-4">
            {#each ['帮我完善故事设定', '请生成大纲', '当前写作进度如何？', '给主角设计一个伏笔'] as hint}
              <button class="btn btn-ghost btn-xs border border-base-content/10" on:click={() => { chatInput = hint; sendMessage(); }}>{hint}</button>
            {/each}
          </div>
        </div>
      {/if}
      {#each msgs as m, msgIdx}
        {#if m.role === 'user'}
          <div class="chat chat-end">
            <div class="chat-bubble chat-bubble-primary text-sm whitespace-pre-wrap max-w-[85%]">{m.content}</div>
            <div class="chat-footer text-xs text-base-content/30 mt-0.5">{fmtTime(m.timestamp)}</div>
          </div>
        {:else if m.role === 'assistant'}
          {#if m.tool_calls?.length > 0}
            {#each m.tool_calls as tc}
              <div class="chat chat-start">
                <div class="chat-bubble text-xs font-mono max-w-[85%] {dangerTools.has(tc.name) ? 'bg-error/15 border border-error/30' : 'bg-base-300'}">
                  <div class="{dangerTools.has(tc.name) ? 'text-error' : 'text-warning'} font-semibold mb-0.5">🔧 {toolLabel(tc.name)}</div>
                  {#if fmtArgs(tc.arguments)}
                    <div class="text-base-content/50 break-all">{fmtArgs(tc.arguments)}</div>
                  {/if}
                </div>
              </div>
            {/each}
          {/if}
          {#if m.content}
            {#if isHallucinatedWait(m, msgs, msgIdx)}
              <div class="chat chat-start">
                <div class="chat-bubble bg-warning/20 border border-warning/40 text-sm max-w-[85%]">
                  <div class="text-warning font-semibold mb-1">⚠️ 该回复可能未实际执行操作</div>
                  <div class="text-base-content/70 whitespace-pre-wrap">{m.content}</div>
                </div>
              </div>
            {:else}
              {#each parseContentSegments(m.content) as seg}
                {#if seg.type === 'tool_call'}
                  <div class="chat chat-start">
                    <div class="chat-bubble bg-base-300 text-xs font-mono max-w-[85%]">
                      <div class="text-warning font-semibold mb-0.5">🔧 {toolLabel(seg.name)}</div>
                      {#if fmtArgs(seg.args)}
                        <div class="text-base-content/50 break-all">{fmtArgs(seg.args)}</div>
                      {/if}
                    </div>
                  </div>
                {:else if seg.content.trim()}
                  <div class="chat chat-start">
                    <div class="chat-bubble bg-base-300 text-sm whitespace-pre-wrap max-w-[85%]">{seg.content.trim()}</div>
                  </div>
                {/if}
              {/each}
            {/if}
          {/if}
        {:else if m.role === 'tool'}
          <div class="chat chat-start">
            <div class="chat-bubble bg-base-300/60 text-xs font-mono max-w-[85%]">
              <details>
                <summary class="text-info font-semibold cursor-pointer select-none">📋 工具结果</summary>
                <div class="text-base-content/50 break-all mt-1 max-h-32 overflow-y-auto whitespace-pre-wrap">{m.tool_result || ''}</div>
              </details>
            </div>
          </div>
        {/if}
      {/each}

      {#each pendingTools as tc}
        <div class="chat chat-start">
          <div class="chat-bubble text-xs font-mono max-w-[85%] {dangerTools.has(tc.name) ? 'bg-error/15 border border-error/30' : 'bg-base-300'}">
            {#if tc.status === 'running'}
              <div class="text-warning font-semibold mb-0.5">🔧 {toolLabel(tc.name)}</div>
              <div class="text-warning animate-pulse">执行中...</div>
            {:else}
              <div class="text-success font-semibold mb-0.5">✅ {toolLabel(tc.name)}</div>
              {#if tc.result}
                <div class="text-base-content/50 break-all max-h-20 overflow-y-auto">{tc.result.length > 200 ? tc.result.slice(0, 200) + '...' : tc.result}</div>
              {/if}
            {/if}
          </div>
        </div>
      {/each}

      {#if streamingText}
        {#each parseContentSegments(streamingText) as seg}
          {#if seg.type === 'tool_call'}
            <div class="chat chat-start">
              <div class="chat-bubble bg-base-300 text-xs font-mono max-w-[85%]">
                <div class="text-warning font-semibold mb-0.5">🔧 {toolLabel(seg.name)}</div>
                {#if fmtArgs(seg.args)}
                  <div class="text-base-content/50 break-all">{fmtArgs(seg.args)}</div>
                {/if}
              </div>
            </div>
          {:else if seg.content.trim()}
            <div class="chat chat-start">
              <div class="chat-bubble bg-base-300 text-sm whitespace-pre-wrap max-w-[85%]">{seg.content.trim()}<span class="inline-block w-1.5 h-3.5 bg-primary/70 animate-pulse ml-0.5 align-text-bottom"></span></div>
            </div>
          {/if}
        {/each}
      {/if}
    {/if}
  </div>

  <!-- 失败重试 -->
  {#if $lastFailedTask && !$taskRunning}
    <div class="border-t border-error/30 bg-error/10 px-3 py-2 flex items-center gap-2 shrink-0">
      <span class="text-sm text-error">❌ {$lastFailedTask.taskName}失败</span>
      <div class="flex-1"></div>
      <button class="btn btn-error btn-xs" on:click={retryTask}>重试</button>
      <button class="btn btn-ghost btn-xs" on:click={() => lastFailedTask.set(null)}>忽略</button>
    </div>
  {/if}

  <!-- 输入区 -->
  {#if $currentChatSession}
    <div class="border-t border-base-content/10 p-2 flex gap-2 items-end shrink-0">
      <textarea
        bind:this={inputEl}
        class="textarea textarea-sm flex-1 min-h-[38px] max-h-[120px] resize-none text-base leading-relaxed"
        bind:value={chatInput}
        placeholder={$taskRunning ? 'AI 任务进行中，请稍候...' : '输入消息... (Enter 发送, Shift+Enter 换行)'}
        on:keydown={handleKeydown}
        on:input={autoGrow}
        disabled={$taskRunning}
      ></textarea>
      <button class="btn btn-primary btn-sm" on:click={sendMessage} disabled={$taskRunning || !chatInput.trim()}>发送</button>
    </div>
  {/if}
</div>
