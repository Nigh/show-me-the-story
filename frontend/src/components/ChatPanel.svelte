<script>
  import { onMount, afterUpdate } from 'svelte';
  import { api } from '../lib/api.js';
  import { renderMarkdown } from '../lib/markdown.js';
  import { chatSessions, currentChatSession, addToast, showConfirm, taskRunning, lastFailedTask, logEntries, currentTaskName } from '../lib/stores.js';
  import { t, uiLocale, formatToolResult } from '../lib/i18n/index.js';
  import TaskTokenBadge from './TaskTokenBadge.svelte';

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

  const dangerTools = new Set(['delete_chapter', 'delete_chapters_from', 'delete_outline', 'reset_progress']);

  function toolLabel(name) {
    if (!name) return '';
    const key = 'chat.toolNames.' + name;
    const label = $t(key);
    return label === key ? name : label;
  }
  function toolResultText(msg, key, args) {
    return formatToolResult(msg, key, args, $uiLocale);
  }
  function fmtArgs(args) {
    const s = typeof args === 'string' ? args : JSON.stringify(args);
    if (!s || s === '{}' || s === 'null') return '';
    return s.length > 300 ? s.slice(0, 300) + '...' : s;
  }
  function fmtTime(ts) {
    if (!ts) return '';
    try {
      const tag = $uiLocale === 'en' ? 'en-US' : 'zh-CN';
      return new Date(ts).toLocaleTimeString(tag, { hour12: false, hour: '2-digit', minute: '2-digit' });
    } catch { return ''; }
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
    // ponytail: heuristic on both zh and en wait-phrases; false positives are acceptable.
    const waitPattern = /请(耐心)?等待|请稍等|正在生成|等待完成|please wait|generating|in progress|hold on|one moment/i;
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
        segments.push({ type: 'tool_call', name: tc.name || tc.tool || $t('chat.tool.unknown'), args: tc.arguments || tc.args || {} });
      } catch {
        // 未闭合/不完整的 tool_call（流式中途），按工具调用占位显示
        segments.push({ type: 'tool_call', name: $t('chat.tool.preparing'), args: '' });
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

  // 滚动守卫：afterUpdate 在任何 store 变化（如 token 计数、
  // 日志追加）后都会触发，无条件写 scrollTop 会造成高频强制重排。
  // 仅在消息区内容实际变化时才滚动。
  let lastScrollKey = '';
  afterUpdate(() => {
    const key = msgs.length + ':' + streamingText.length + ':' + pendingTools.map(t => t.status).join(',');
    if (key === lastScrollKey) return;
    lastScrollKey = key;
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
    showConfirm($t('chat.session.deleteConfirm'), async () => {
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
    if ($taskRunning) { addToast($t('chat.toast.taskRunning'), 'error'); return; }
    if (!$currentChatSession) { addToast($t('chat.toast.needSession'), 'error'); return; }
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
      addToast($t('chat.toast.stopping'), 'info');
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
      addToast($t('chat.toast.retryNoMsg'), 'error');
      return;
    }

    const endpoint = retryEndpoints[failed.task];
    if (endpoint) {
      try {
        await api(endpoint.method, endpoint.url);
      } catch (e) { addToast($t('chat.toast.retryFailed', { msg: e.message }), 'error'); }
    } else {
      addToast($t('chat.toast.retryUnsupported'), 'error');
    }
  }

  $: welcomeHints = [
    $t('chat.welcome.q1'),
    $t('chat.welcome.q2'),
    $t('chat.welcome.q3'),
    $t('chat.welcome.q4'),
  ];
</script>

<div class="flex flex-col h-full">
  <!-- 会话栏 -->
  <div class="border-b border-base-content/10 px-3 py-2 flex items-center gap-2 shrink-0">
    <button
      class="btn btn-sm gap-1 border border-base-content/20 hover:border-primary/50 hover:bg-base-300 transition-colors"
      class:border-primary={showSessionList}
      class:bg-base-300={showSessionList}
      on:click={() => showSessionList = !showSessionList}
    >
      <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" /></svg>
      {$t('chat.session.menu')}
      <svg xmlns="http://www.w3.org/2000/svg" class="h-3 w-3 transition-transform" class:rotate-180={showSessionList} fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" /></svg>
    </button>
    <span class="text-sm text-base-content/50 truncate flex-1">
      {$currentChatSession?.title || $t('chat.session.placeholder')}
    </span>
    {#if $taskRunning}
      <button class="btn btn-error btn-xs gap-1" on:click={stopTask}>{$t('chat.session.stop')}</button>
    {/if}
    <button class="btn btn-primary btn-xs" on:click={createSession} disabled={$taskRunning}>{$t('chat.session.new')}</button>
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
            <div class="text-xs text-base-content/40">{new Date(s.updated_at).toLocaleString($uiLocale === 'en' ? 'en-US' : 'zh-CN')} · {$t('chat.session.msgs', { n: s.msg_count || 0 })}</div>
          </div>
          <button class="btn btn-ghost btn-xs text-error opacity-0 group-hover:opacity-100 transition-opacity" on:click={(e) => deleteSession(s.id, e)}>{$t('common.delete')}</button>
        </div>
      {/each}
      {#if sessions.length === 0}
        <div class="px-3 py-2 text-sm text-base-content/40">{$t('chat.session.empty')}</div>
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
        <span class="text-xs font-semibold text-base-content/70">{$currentTaskName || $t('chat.task.placeholder')}{$taskRunning ? $t('chat.task.running') : $t('chat.task.ended')}</span>
        {#if $taskRunning}
          <TaskTokenBadge />
        {/if}
        <span class="text-xs text-base-content/40 ml-auto">{taskStatusCollapsed ? $t('chat.task.expand') : $t('chat.task.collapse')}</span>
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
      <div class="text-center text-base-content/40 py-8 text-base">{$t('chat.notSelected')}</div>
    {:else}
      {#if msgs.length === 0 && !streamingText}
        <div class="text-center text-base-content/40 py-10 space-y-3">
          <div class="text-3xl">💬</div>
          <p class="text-sm">{$t('chat.welcome.hint')}</p>
          <div class="flex flex-wrap justify-center gap-1.5 px-4">
            {#each welcomeHints as hint}
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
                  <div class="text-warning font-semibold mb-1">{$t('chat.assistant.maybeNoop')}</div>
                  <div class="text-base-content/70 md-body">{@html renderMarkdown(m.content)}</div>
                </div>
              </div>
            {:else}
              {#each parseContentSegments(m.content) as seg}
                {#if seg.type === 'tool_call'}
                  {#if !m.tool_calls?.length}
                    <div class="chat chat-start">
                      <div class="chat-bubble bg-base-300 text-xs font-mono max-w-[85%]">
                        <div class="text-warning font-semibold mb-0.5">🔧 {toolLabel(seg.name)}</div>
                        {#if fmtArgs(seg.args)}
                          <div class="text-base-content/50 break-all">{fmtArgs(seg.args)}</div>
                        {/if}
                      </div>
                    </div>
                  {/if}
                {:else if seg.content.trim()}
                  <div class="chat chat-start">
                    <div class="chat-bubble bg-base-300 text-sm max-w-[85%] md-body">{@html renderMarkdown(seg.content.trim())}</div>
                  </div>
                {/if}
              {/each}
            {/if}
          {/if}
        {:else if m.role === 'tool'}
          <div class="chat chat-start">
            <div class="chat-bubble bg-base-300/60 text-xs font-mono max-w-[85%]">
              <details>
                <summary class="text-info font-semibold cursor-pointer select-none">{$t('chat.tool.result')}</summary>
                <div class="text-base-content/50 break-all mt-1 max-h-32 overflow-y-auto whitespace-pre-wrap">{toolResultText(m.tool_result, m.tool_result_key, m.tool_result_args)}</div>
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
              <div class="text-warning animate-pulse">{$t('chat.tool.running')}</div>
            {:else}
              <div class="text-success font-semibold mb-0.5">✅ {toolLabel(tc.name)}</div>
              {#if tc.result}
                <div class="text-base-content/50 break-all max-h-20 overflow-y-auto">{tc.result ? (tc.result.length > 200 ? tc.result.slice(0, 200) + '...' : tc.result) : ''}</div>
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
              <div class="chat-bubble bg-base-300 text-sm max-w-[85%]"><span class="md-body">{@html renderMarkdown(seg.content.trim())}</span><span class="inline-block w-1.5 h-3.5 bg-primary/70 animate-pulse ml-0.5 align-text-bottom"></span></div>
            </div>
          {/if}
        {/each}
      {/if}
    {/if}
  </div>

  <!-- 失败重试 -->
  {#if $lastFailedTask && !$taskRunning}
    <div class="border-t border-error/30 bg-error/10 px-3 py-2 flex items-center gap-2 shrink-0">
      <span class="text-sm text-error">❌ {$lastFailedTask.taskName}{$t('chat.failed.suffix')}</span>
      <div class="flex-1"></div>
      <button class="btn btn-error btn-xs" on:click={retryTask}>{$t('chat.failed.retry')}</button>
      <button class="btn btn-ghost btn-xs" on:click={() => lastFailedTask.set(null)}>{$t('chat.failed.ignore')}</button>
    </div>
  {/if}

  <!-- 输入区 -->
  {#if $currentChatSession}
    <div class="border-t border-base-content/10 p-2 flex gap-2 items-end shrink-0">
      <textarea
        bind:this={inputEl}
        class="textarea textarea-sm flex-1 min-h-[38px] max-h-[120px] resize-none text-base leading-relaxed"
        bind:value={chatInput}
        placeholder={$taskRunning ? $t('chat.input.placeholderBusy') : $t('chat.input.placeholder')}
        on:keydown={handleKeydown}
        on:input={autoGrow}
        disabled={$taskRunning}
      ></textarea>
      <button class="btn btn-primary btn-sm" on:click={sendMessage} disabled={$taskRunning || !chatInput.trim()}>{$t('chat.input.send')}</button>
    </div>
  {/if}
</div>
