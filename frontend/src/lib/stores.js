import { writable, get } from 'svelte/store';

export const apiConfig = writable(null);
export const config = writable(null);
export const progress = writable(null);
export const settings = writable(null);
export const skills = writable([]);
export const taskRunning = writable(false);
export const currentTaskName = writable(null);

export const currentProject = writable(null);
export const projects = writable([]);

export const currentPage = writable('config');
export const contextPage = writable('config');
export const selectedChapter = writable(-1);

export const streamingContent = writable('');
export const streamingChapterIdx = writable(-1);
// 当前任务流式输出累计字数（章节正文 + 助理回复）
export const streamCharCount = writable(0);
// 自动确认模式（每章生成完成后自动确认并继续下一章）
export const autoConfirm = writable(false);

export const chatSessions = writable(null);
export const currentChatSession = writable(null);

export const continueAnalysis = writable(null);
export const editingChapterNum = writable(-1);
export const editingCharID = writable(null);
export const editingWvID = writable(null);
export const wvFilter = writable('all');

export const logEntries = writable([]);

// 重试信息：记录最后一次失败的任务
export const lastFailedTask = writable(null);

export function addLog(entry) {
  logEntries.update(entries => {
    const next = [...entries, entry];
    return next.length > 500 ? next.slice(-500) : next;
  });
}

export function addToast(msg, type = 'info') {
  const id = Date.now();
  const unsub = toastStore.subscribe(() => {});
  toastStore.update(t => [...t, { id, msg, type }]);
  unsub();
  setTimeout(() => {
    toastStore.update(t => t.filter(x => x.id !== id));
  }, 3000);
}

export const toastStore = writable([]);

export const taskNotification = writable(null);

export const confirmModal = writable(null);

export const postprocess = writable(null);

export function showConfirm(message, onConfirm) {
  confirmModal.set({ message, onConfirm });
}
