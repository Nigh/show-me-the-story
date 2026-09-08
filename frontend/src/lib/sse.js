import { get } from 'svelte/store';
import { addLog, addToast, config, progress, skills, taskRunning, streamingContent, streamingChapterIdx, taskTokenUsage, currentChatSession, settings, chatSessions, lastFailedTask, currentTaskName, logEntries, postprocess, foreshadowSuggestions, foreshadowShowSuggestions, outlineCharacterSuggestions, outlineCharacterShowSuggestions, pendingConfigChanges, showConfigChangePanel, storageError } from './stores.js';
import { api } from './api.js';
import { getLocale, translate, formatLogEntry, formatToolResult } from './i18n/index.js';
import { TOKEN_POLL_INTERVAL_MS } from './tokenPoll.js';

let eventSource = null;
let reconnectTimer = null;
let tokenPollTimer = null;
let taskCount = 0;

// —— 流式输出节流缓冲 + 尾部窗口 ——
const FLUSH_INTERVAL = 150;
const TAIL_MAX = 3000;

let contentBuf = '';
let contentFull = '';
let contentIdx = -1;
let contentTimer = null;

function flushContentBuf() {
  if (contentTimer) { clearTimeout(contentTimer); contentTimer = null; }
  if (!contentBuf) return;
  const text = contentBuf;
  contentBuf = '';
  contentFull += text;
  streamingChapterIdx.set(contentIdx);
  streamingContent.set(contentFull.length > TAIL_MAX ? contentFull.slice(-TAIL_MAX) : contentFull);
}

function resetContentStream(idx) {
  contentBuf = '';
  contentFull = '';
  if (contentTimer) { clearTimeout(contentTimer); contentTimer = null; }
  contentIdx = idx;
  streamingChapterIdx.set(idx);
  streamingContent.set('');
}

let progressFetchTimer = null;

function refreshProgress(immediate = false) {
  if (immediate) {
    if (progressFetchTimer) { clearTimeout(progressFetchTimer); progressFetchTimer = null; }
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
    return;
  }
  if (progressFetchTimer) return;
  progressFetchTimer = setTimeout(() => {
    progressFetchTimer = null;
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
  }, 500);
}

function refreshTokenUsage() {
  if (!get(taskRunning)) return;
  api('GET', '/api/status').then(s => {
    if (s?.token_usage) taskTokenUsage.set(s.token_usage);
  }).catch(() => {});
}

function startTokenPoll() {
  stopTokenPoll();
  refreshTokenUsage();
  tokenPollTimer = setInterval(refreshTokenUsage, TOKEN_POLL_INTERVAL_MS);
}

function stopTokenPoll() {
  if (tokenPollTimer) { clearInterval(tokenPollTimer); tokenPollTimer = null; }
}

let chatBuf = '';
let chatSessionId = null;
let chatTimer = null;

function flushChatBuf() {
  if (chatTimer) { clearTimeout(chatTimer); chatTimer = null; }
  if (!chatBuf) return;
  const text = chatBuf;
  const sid = chatSessionId;
  chatBuf = '';
  currentChatSession.update(s => {
    if (!s || s.id !== sid) return s;
    return { ...s, streaming_text: (s.streaming_text || '') + text };
  });
}

function clearChatBuf() {
  chatBuf = '';
  if (chatTimer) { clearTimeout(chatTimer); chatTimer = null; }
}

async function syncTaskFromStatus() {
  try {
    const s = await api('GET', '/api/status');
    if (s?.is_task_running) {
      // Refresh drops SSE task_start history; seed from backend active_work.
      const n = Math.max(1, Number(s.active_work) || 1);
      if (taskCount < n) taskCount = n;
      taskRunning.set(true);
      if (s.current_task) {
        currentTaskName.set(translate(`task.${s.current_task}`) || s.current_task);
      }
      startTokenPoll();
    } else if (taskCount <= 0) {
      taskRunning.set(false);
      currentTaskName.set(null);
      stopTokenPoll();
    }
  } catch (_) { /* ignore */ }
}

export function connectSSE() {
  if (eventSource) eventSource.close();
  const locale = getLocale();
  eventSource = new EventSource(`/api/events?locale=${encodeURIComponent(locale)}`);

  eventSource.addEventListener('open', () => {
    syncTaskFromStatus();
  });

  eventSource.addEventListener('log', e => {
    const d = JSON.parse(e.data);
    d.msg = formatLogEntry(d, locale);
    addLog(d);
  });

  eventSource.addEventListener('progress_update', () => {
    refreshProgress();
  });

  eventSource.addEventListener('storage_error', e => {
    storageError.set(JSON.parse(e.data));
  });

  function taskLabel(task) {
    return translate(`task.${task}`) || task;
  }

  eventSource.addEventListener('task_start', e => {
    const d = JSON.parse(e.data);
    taskCount++;
    taskRunning.set(true);
    resetContentStream(-1);
    clearChatBuf();
    taskTokenUsage.set(null);
    currentTaskName.set(taskLabel(d.task));
    logEntries.set([]);
    lastFailedTask.set(null);
    startTokenPoll();
  });

  eventSource.addEventListener('task_end', e => {
    const d = JSON.parse(e.data);
    taskCount--;
    if (taskCount <= 0) {
      taskCount = 0;
      taskRunning.set(false);
      resetContentStream(-1);
      clearChatBuf();
      taskTokenUsage.set(null);
      currentTaskName.set(null);
      stopTokenPoll();
    }
    refreshProgress(true);

    if (d.success) {
      const name = taskLabel(d.task);
      addToast(translate('toast.taskDone', { name }), 'success');
    } else if (d.task === 'chapter_generation') {
      api('GET', '/api/progress').then(p => {
        progress.set(p);
        if (!p?.pending_writing_conflict) {
          lastFailedTask.set({ task: d.task, taskName: taskLabel(d.task) });
        }
      }).catch(() => {
        lastFailedTask.set({ task: d.task, taskName: taskLabel(d.task) });
      });
    } else {
      lastFailedTask.set({ task: d.task, taskName: taskLabel(d.task) });
    }

      if (d.task === 'proofread_analyze' || d.task === 'proofread_apply') {
        api('GET', '/api/proofread').then(p => postprocess.set({ book_complete: true, state: p })).catch(() => {});
    }

    if (d.task === 'skill_validation' || d.task === 'skill_optimization') {
      api('GET', '/api/skill-library').then(s => skills.set(s)).catch(() => {});
    }

    if (d.task === 'chat_message') {
      // 异步工具（如 generate_outline）会启动子任务，taskCount 仍 > 0，
      // 上方 taskCount<=0 分支不会 clearChatBuf；须在此取消 chat_chunk 延迟 flush，
      // 否则 reload 后的 messages 与 streaming_text 会各显示一遍相同 reply。
      clearChatBuf();
      let sessionId = null;
      currentChatSession.update(s => {
        if (!s) return s;
        sessionId = s.id;
        return { ...s, streaming_text: '' };
      });
      if (sessionId) {
        api('GET', '/api/chat/sessions/' + sessionId).then(s => {
          currentChatSession.set(s);
        }).catch(() => {});
      }
      api('GET', '/api/chat/sessions').then(s => chatSessions.set(s)).catch(() => {});
      api('GET', '/api/config').then(c => config.set(c)).catch(() => {});
      api('GET', '/api/settings').then(s => settings.set(s)).catch(() => {});
    }
  });

  eventSource.addEventListener('token_usage', e => {
    const d = JSON.parse(e.data);
    taskTokenUsage.set(d);
  });

  eventSource.addEventListener('stream_start', e => {
    const d = JSON.parse(e.data);
    resetContentStream(d.chapter_idx);
  });

  eventSource.addEventListener('content_chunk', e => {
    const d = JSON.parse(e.data);
    if (d.chapter_idx !== contentIdx) {
      flushContentBuf();
      resetContentStream(d.chapter_idx);
    }
    contentBuf += d.text;
    if (!contentTimer) contentTimer = setTimeout(flushContentBuf, FLUSH_INTERVAL);
  });

  eventSource.addEventListener('settings_reconciled', e => {
    const d = JSON.parse(e.data);
    api('GET', '/api/config').then(c => {
      config.set(c);
    }).catch(() => {});
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
    addToast(translate('toast.settingsReconciled', { detail: d.explanation || '' }), 'success');
  });

  eventSource.addEventListener('settings_updated', () => {
    api('GET', '/api/settings').then(s => settings.set(s)).catch(() => {});
    api('GET', '/api/config').then(c => config.set(c)).catch(() => {});
  });

  eventSource.addEventListener('foreshadow_suggestions', e => {
    const d = JSON.parse(e.data);
    const items = (d || []).map(s => ({ ...s, _selected: true }));
    foreshadowSuggestions.set(items);
    foreshadowShowSuggestions.set(true);
    addToast(translate('toast.foreshadowReady', { n: items.length }), 'info');
  });

  eventSource.addEventListener('outline_character_suggestions', e => {
    const d = JSON.parse(e.data);
    const items = (d || []).map(s => ({ ...s, _selected: true }));
    outlineCharacterSuggestions.set(items);
    outlineCharacterShowSuggestions.set(true);
    addToast(translate('toast.outlineCharacterReady', { n: items.length }), 'info');
  });

  eventSource.addEventListener('config_change_proposal', e => {
    const d = JSON.parse(e.data);
    const items = (d || []).map(c => ({ ...c, _selected: true }));
    pendingConfigChanges.set(items);
    showConfigChangePanel.set(items.length > 0);
    if (items.length > 0) {
      addToast(translate('toast.configChangeProposal', { n: items.length }), 'info');
    }
  });

  eventSource.addEventListener('foreshadow_outline_conflicts', e => {
    const d = JSON.parse(e.data);
    refreshProgress(true);
    addToast(translate('toast.foreshadowOutlineConflict', { n: (d.conflicts || []).length }), 'warning');
  });

  eventSource.addEventListener('writing_conflict', e => {
    const d = JSON.parse(e.data);
    refreshProgress(true);
    addToast(translate('toast.writingConflict'), 'warning');
  });

  eventSource.addEventListener('chat_chunk', e => {
    const d = JSON.parse(e.data);
    if (d.session_id !== chatSessionId) {
      flushChatBuf();
      chatSessionId = d.session_id;
    }
    chatBuf += d.text;
    if (!chatTimer) chatTimer = setTimeout(flushChatBuf, FLUSH_INTERVAL);
  });

  eventSource.addEventListener('tool_call_start', e => {
    const d = JSON.parse(e.data);
    flushChatBuf();
    currentChatSession.update(s => {
      if (!s) return s;
      const toolCalls = [...(s.pending_tool_calls || []), { name: d.tool_name, status: 'running', args: d.args }];
      return { ...s, pending_tool_calls: toolCalls };
    });
  });

  eventSource.addEventListener('tool_call_end', e => {
    const d = JSON.parse(e.data);
    currentChatSession.update(s => {
      if (!s) return s;
      const display = formatToolResult(d.result, d.result_key, d.result_args, locale);
      const toolCalls = (s.pending_tool_calls || []).map(tc =>
        tc.name === d.tool_name && tc.status === 'running'
          ? { ...tc, status: 'done', result: display, result_key: d.result_key, result_args: d.result_args }
          : tc
      );
      return { ...s, pending_tool_calls: toolCalls };
    });
    api('GET', '/api/config').then(c => config.set(c)).catch(() => {});
    api('GET', '/api/settings').then(s => settings.set(s)).catch(() => {});
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
  });

  eventSource.onerror = () => {
    eventSource.close();
    clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(connectSSE, 3000);
  };
}
