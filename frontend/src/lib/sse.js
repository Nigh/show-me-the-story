import { addLog, addToast, config, progress, taskRunning, streamingContent, streamingChapterIdx, streamCharCount, continueAnalysis, currentChatSession, settings, chatSessions, lastFailedTask, currentTaskName, logEntries, postprocess } from './stores.js';
import { api } from './api.js';

let eventSource = null;
let reconnectTimer = null;

// —— 流式输出节流缓冲 + 尾部窗口 ——
// 节流只能降低更新频率，但若 store 中保存完整流式全文，每次刷新仍要对全文
// 重新渲染/排版（成本随长度线性增长，总成本 O(n²)），长章节会把主线程占满
// 直至页面无响应。因此完整文本只存模块级变量，store 仅保留尾部窗口，
// 每次刷新渲染成本恒定 O(1)。生成结束后由 progress 重新拉取展示全文。
const FLUSH_INTERVAL = 150;
const TAIL_MAX = 3000; // store 中保留的尾部窗口字符数

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
  streamCharCount.update(n => n + Array.from(text).length);
}

function resetContentStream(idx) {
  contentBuf = '';
  contentFull = '';
  if (contentTimer) { clearTimeout(contentTimer); contentTimer = null; }
  contentIdx = idx;
  streamingChapterIdx.set(idx);
  streamingContent.set('');
  streamCharCount.set(0);
}

// —— progress 拉取去抖 ——
// progress_update 事件可能在短时间内连发，而 /api/progress 返回含全书正文的
// 大 JSON，每次都拉取会造成解析 + 整页重渲染的尖峰。这里 500ms 内合并为一次。
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

let chatBuf = '';
let chatSessionId = null;
let chatTimer = null;

function flushChatBuf() {
  if (chatTimer) { clearTimeout(chatTimer); chatTimer = null; }
  if (!chatBuf) return;
  const text = chatBuf;
  const sid = chatSessionId;
  chatBuf = '';
  streamCharCount.update(n => n + Array.from(text).length);
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
  eventSource = new EventSource('/api/events');

  eventSource.addEventListener('log', e => {
    const d = JSON.parse(e.data);
    addLog(d);
  });

  eventSource.addEventListener('progress_update', () => {
    refreshProgress();
  });

  eventSource.addEventListener('task_start', e => {
    const d = JSON.parse(e.data);
    taskRunning.set(true);
    resetContentStream(-1);
    clearChatBuf();
    streamCharCount.set(0);
    currentTaskName.set(taskNames[d.task] || d.task);
    logEntries.set([]);
    lastFailedTask.set(null);
  });

  const taskNames = {
    'outline_generation': '大纲生成',
    'outline_revision': '大纲修订',
    'chapter_generation': '章节创作',
    'chapter_revision': '章节修订',
    'foreshadow_suggest': '伏笔建议',
    'continue_analysis': '内容分析',
    'continuation_outline': '续写大纲',
    'settings_reconciliation': '设定协调',
    'chat_message': '助理对话',
    'postprocess_diagnose': '全书优化分析',
    'postprocess_consistency': '全书一致性核查',
    'postprocess_roadmap': '优化路线图生成',
    'postprocess_execute': '全书优化执行',
    'chapter_polish': '章节润色',
    'smooth_transitions': '章节衔接优化',
  };

  eventSource.addEventListener('task_end', e => {
    const d = JSON.parse(e.data);
    taskRunning.set(false);
    resetContentStream(-1);
    clearChatBuf();
    streamCharCount.set(0);
    currentTaskName.set(null);
    refreshProgress(true);

    if (d.success) {
      const name = taskNames[d.task] || d.task;
      addToast(`✓ ${name}已完成`, 'success');
    } else {
      // 任务失败时记录重试信息
      lastFailedTask.set({ task: d.task, taskName: taskNames[d.task] || d.task });
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

  // 一次新的流式输出开始（章节生成/修订/润色），清空旧缓冲，
  // 避免事实核查重试或自动连写时新旧内容叠加。
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

  eventSource.addEventListener('stream_progress', e => {
    const d = JSON.parse(e.data);
    addLog({ level: 'info', msg: `正在生成中... 已写 ${d.char_count} 字`, time: new Date().toLocaleTimeString('zh-CN', { hour12: false }) });
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
    addToast('设定协调完成：' + (d.explanation || ''), 'success');
  });

  eventSource.addEventListener('settings_updated', () => {
    api('GET', '/api/settings').then(s => settings.set(s)).catch(() => {});
    api('GET', '/api/config').then(c => config.set(c)).catch(() => {});
  });

  eventSource.addEventListener('foreshadow_suggestions', e => {
    const d = JSON.parse(e.data);
    addToast(`伏笔建议已生成，共 ${d.length} 条`, 'info');
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
      const toolCalls = (s.pending_tool_calls || []).map(tc =>
        tc.name === d.tool_name && tc.status === 'running'
          ? { ...tc, status: 'done', result: d.result }
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
