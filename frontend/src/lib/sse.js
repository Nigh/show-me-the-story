import { addLog, addToast, config, progress, taskRunning, streamingContent, streamingChapterIdx, continueAnalysis, currentChatSession, settings, chatSessions, lastFailedTask, currentTaskName, logEntries } from './stores.js';
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

  eventSource.addEventListener('task_start', e => {
    const d = JSON.parse(e.data);
    taskRunning.set(true);
    streamingContent.set('');
    streamingChapterIdx.set(-1);
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
  };

  eventSource.addEventListener('task_end', e => {
    const d = JSON.parse(e.data);
    taskRunning.set(false);
    streamingContent.set('');
    streamingChapterIdx.set(-1);
    currentTaskName.set(null);
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});

    if (d.success) {
      const name = taskNames[d.task] || d.task;
      addToast(`✓ ${name}已完成`, 'success');
    } else {
      // 任务失败时记录重试信息
      lastFailedTask.set({ task: d.task, taskName: taskNames[d.task] || d.task });
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
