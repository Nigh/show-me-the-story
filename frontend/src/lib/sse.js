import { addLog, addToast, config, progress, taskRunning, streamingContent, streamingChapterIdx, continueAnalysis, currentChatSession } from './stores.js';
import { api } from './api.js';

let eventSource = null;
let reconnectTimer = null;

export function connectSSE() {
  if (eventSource) eventSource.close();
  eventSource = new EventSource('/api/events');

  eventSource.addEventListener('log', e => {
    const d = JSON.parse(e.data);
    addLog(d);
  });

  eventSource.addEventListener('progress_update', () => {
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
  });

  eventSource.addEventListener('task_start', () => {
    taskRunning.set(true);
    streamingContent.set('');
    streamingChapterIdx.set(-1);
  });

  eventSource.addEventListener('task_end', () => {
    taskRunning.set(false);
    streamingContent.set('');
    streamingChapterIdx.set(-1);
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
  });

  eventSource.addEventListener('content_chunk', e => {
    const d = JSON.parse(e.data);
    streamingChapterIdx.set(d.chapter_idx);
    streamingContent.update(v => v + d.text);
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

  eventSource.addEventListener('foreshadow_suggestions', e => {
    const d = JSON.parse(e.data);
    addToast(`伏笔建议已生成，共 ${d.length} 条`, 'info');
  });

  eventSource.addEventListener('chat_chunk', e => {
    const d = JSON.parse(e.data);
    currentChatSession.update(s => {
      if (!s || s.id !== d.session_id) return s;
      return { ...s, streaming_text: (s.streaming_text || '') + d.text };
    });
  });

  eventSource.addEventListener('tool_call_start', e => {
    const d = JSON.parse(e.data);
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
      const toolCalls = (s.pending_tool_calls || []).map(tc =>
        tc.name === d.tool_name && tc.status === 'running'
          ? { ...tc, status: 'done', result: d.result }
          : tc
      );
      return { ...s, pending_tool_calls: toolCalls };
    });
  });

  eventSource.addEventListener('polish_result', e => {
    addToast('去AI味完成', 'success');
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
  });

  eventSource.addEventListener('settings_polish_result', e => {
    const d = JSON.parse(e.data);
    window.dispatchEvent(new CustomEvent('settings_polish_result', { detail: d }));
  });

  eventSource.onerror = () => {
    eventSource.close();
    clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(connectSSE, 3000);
  };
}
