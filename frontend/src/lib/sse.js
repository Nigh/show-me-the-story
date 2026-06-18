import { get } from 'svelte/store';
import { addLog, addToast, config, progress, taskRunning, streamingContent, streamingChapterIdx, taskTokenUsage, continueAnalysis, currentChatSession, settings, chatSessions, lastFailedTask, currentTaskName, logEntries, postprocess, foreshadowSuggestions, foreshadowShowSuggestions } from './stores.js';
import { api } from './api.js';
import { getLocale, translate, formatLogEntry, formatToolResult } from './i18n/index.js';

let eventSource = null;
let reconnectTimer = null;
let tokenPollTimer = null;

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
  tokenPollTimer = setInterval(refreshTokenUsage, 2000);
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

export function connectSSE() {
  if (eventSource) eventSource.close();
  const locale = getLocale();
  eventSource = new EventSource(`/api/events?locale=${encodeURIComponent(locale)}`);

  eventSource.addEventListener('log', e => {
    const d = JSON.parse(e.data);
    d.msg = formatLogEntry(d, locale);
    addLog(d);
  });

  eventSource.addEventListener('progress_update', () => {
    refreshProgress();
  });

  function taskLabel(task) {
    return translate(`task.${task}`) || task;
  }

  eventSource.addEventListener('task_start', e => {
    const d = JSON.parse(e.data);
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
    taskRunning.set(false);
    resetContentStream(-1);
    clearChatBuf();
    taskTokenUsage.set(null);
    currentTaskName.set(null);
    stopTokenPoll();
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

    if (d.task === 'postprocess_diagnose' || d.task === 'postprocess_consistency' || d.task === 'postprocess_roadmap' || d.task === 'postprocess_execute') {
      api('GET', '/api/postprocess').then(p => postprocess.set(p)).catch(() => {});
    }

    if (d.task === 'chat_message') {
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

  eventSource.addEventListener('continue_analysis', e => {
    const d = JSON.parse(e.data);
    continueAnalysis.set(d);
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

  eventSource.addEventListener('postprocess_update', e => {
    const d = JSON.parse(e.data);
    postprocess.set(d);
  });

  eventSource.addEventListener('postprocess_roadmap', e => {
    const d = JSON.parse(e.data);
    postprocess.update(pp => pp ? { ...pp, state: d } : { book_complete: true, state: d });
  });

  eventSource.addEventListener('postprocess_item_done', e => {
    const item = JSON.parse(e.data);
    postprocess.update(pp => {
      if (!pp?.state?.roadmap) return pp;
      const roadmap = pp.state.roadmap.map(r => r.id === item.id ? { ...r, ...item } : r);
      return { ...pp, state: { ...pp.state, roadmap } };
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
