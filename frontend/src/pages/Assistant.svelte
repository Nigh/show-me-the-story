<script>
  import { onMount, afterUpdate } from 'svelte';
  import { api } from '../lib/api.js';
  import { chatSessions, currentChatSession, addToast } from '../lib/stores.js';

  let chatInput = '';
  let messagesContainer;

  $: sessions = ($chatSessions?.sessions || []);
  $: msgs = ($currentChatSession?.messages || []);
  $: streamingText = $currentChatSession?.streaming_text || '';
  $: pendingTools = $currentChatSession?.pending_tool_calls || [];

  onMount(async () => {
    try { chatSessions.set(await api('GET', '/api/chat/sessions')); } catch (e) {}
  });

  afterUpdate(() => {
    if (messagesContainer) messagesContainer.scrollTop = messagesContainer.scrollHeight;
  });

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
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function sendMessage() {
    if (!$currentChatSession) { addToast('请先选择会话', 'error'); return; }
    const msg = chatInput.trim();
    if (!msg) return;
    chatInput = '';

    currentChatSession.update(s => {
      if (!s) return s;
      const messages = [...(s.messages || []), { role: 'user', content: msg, timestamp: new Date().toISOString() }];
      return { ...s, messages, streaming_text: '', pending_tool_calls: [] };
    });

    try {
      await api('POST', '/api/chat/sessions/' + $currentChatSession.id + '/messages', { content: msg });
      const session = await api('GET', '/api/chat/sessions/' + $currentChatSession.id);
      currentChatSession.set(session);
      chatSessions.set(await api('GET', '/api/chat/sessions'));
    } catch (e) { addToast(e.message, 'error'); }
  }

  function handleKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
  }
</script>

<div class="grid grid-cols-[240px_1fr] gap-3" style="height:calc(100vh - 180px)">
  <!-- Session list -->
  <div class="bg-base-200 shadow-sm border border-base-content/10 rounded-lg overflow-y-auto">
    <div class="p-3 border-b border-base-content/10">
      <button class="btn btn-primary btn-sm w-full" on:click={createSession}>新建会话</button>
    </div>
    {#each sessions as s}
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div
        class="px-4 py-3 border-b border-base-content/10 cursor-pointer hover:bg-base-300 transition-colors"
        class:bg-base-300={$currentChatSession?.id === s.id}
        class:border-l-2={$currentChatSession?.id === s.id}
        class:border-primary={$currentChatSession?.id === s.id}
        on:click={() => selectSession(s.id)}
      >
        <div class="text-sm font-medium truncate">{s.title}</div>
        <div class="text-xs text-base-content/50">{new Date(s.updated_at).toLocaleString('zh-CN')}</div>
      </div>
    {/each}
  </div>

  <!-- Chat area -->
  <div class="flex flex-col bg-base-200 shadow-sm border border-base-content/10 rounded-lg">
    <div bind:this={messagesContainer} class="flex-1 overflow-y-auto p-4 space-y-3">
      {#if !$currentChatSession}
        <div class="text-center text-base-content/50 py-16">选择或创建一个会话开始对话</div>
      {:else}
        {#each msgs as m}
          {#if m.role === 'user'}
            <div class="chat chat-end">
              <div class="chat-bubble chat-bubble-primary text-sm whitespace-pre-wrap">{m.content}</div>
            </div>
          {:else if m.role === 'assistant'}
            {#if m.tool_calls?.length > 0}
              {#each m.tool_calls as tc}
                <div class="chat chat-start">
                  <div class="chat-bubble bg-base-300 text-xs font-mono">
                    <div class="text-warning font-semibold mb-1">🔧 {tc.name}</div>
                    <div class="text-base-content/60">{typeof tc.arguments === 'string' ? tc.arguments : JSON.stringify(tc.arguments)}</div>
                  </div>
                </div>
              {/each}
            {/if}
            {#if m.content}
              <div class="chat chat-start">
                <div class="chat-bubble bg-base-300 text-sm whitespace-pre-wrap">{m.content}</div>
              </div>
            {/if}
          {:else if m.role === 'tool'}
            <div class="chat chat-start">
              <div class="chat-bubble bg-base-300 text-xs font-mono">
                <div class="text-info font-semibold mb-1">📋 工具结果</div>
                <div class="text-base-content/60">{m.tool_result || ''}</div>
              </div>
            </div>
          {/if}
        {/each}

        {#each pendingTools as tc}
          <div class="chat chat-start">
            <div class="chat-bubble bg-base-300 text-xs font-mono">
              <div class="text-warning font-semibold mb-1">🔧 调用 {tc.name}...</div>
              <div class="text-warning animate-pulse">执行中...</div>
            </div>
          </div>
        {/each}

        {#if streamingText}
          <div class="chat chat-start">
            <div class="chat-bubble bg-base-300 text-sm whitespace-pre-wrap">{streamingText}</div>
          </div>
        {/if}
      {/if}
    </div>

    {#if $currentChatSession}
      <div class="border-t border-base-content/10 p-3 flex gap-2">
        <textarea
          class="textarea textarea-sm flex-1 min-h-[44px] max-h-[120px] resize-none"
          bind:value={chatInput}
          placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
          on:keydown={handleKeydown}
        ></textarea>
        <button class="btn btn-primary btn-sm" on:click={sendMessage}>发送</button>
      </div>
    {/if}
  </div>
</div>
