// Run: node frontend/src/lib/sse.check.js
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('./sse.js', import.meta.url), 'utf8');
const script = source.replace(/^import .*;\r?\n/gm, '').replace('export function connectSSE', 'function connectSSE');
const pending = [];
const intervals = new Map();
let timer = 0;
let events;
const context = vm.createContext({
  get: store => store.value,
  setTimeout: () => ++timer, clearTimeout: () => {},
  setInterval: fn => { intervals.set(++timer, fn); return timer; },
  clearInterval: id => intervals.delete(id),
  TOKEN_POLL_INTERVAL_MS: 2000,
  getLocale: () => 'en', translate: key => key, addToast: () => {},
  api: async (method, url) => url === '/api/status'
    ? new Promise(resolve => pending.push(resolve)) : {},
  EventSource: class {
    constructor() { events = {}; }
    addEventListener(name, fn) { events[name] = fn; }
    close() {}
  },
});
for (const name of ['taskRunning', 'currentTaskName', 'taskTokenUsage', 'streamingContent',
  'streamingChapterIdx', 'progress', 'settings', 'chatSessions', 'currentChatSession',
  'logEntries', 'lastFailedTask']) {
  context[name] = { value: null, set(v) { this.value = v; }, update(fn) { this.value = fn(this.value); } };
}
vm.runInContext(script + '\nconnectSSE();', context);
const settle = () => new Promise(setImmediate);
const emit = (name, data = {}) => events[name]({ data: JSON.stringify(data) });
const reply = async running => { pending.shift()({ is_task_running: running }); await settle(); };

// Lose task_end during a disconnect: the idle snapshot must unlock the UI.
emit('task_start', { task: 'chapter_generation' });
await reply(true);
emit('open');
await reply(false);
assert.equal(context.taskRunning.value, false);
assert.equal(intervals.size, 0);

// A stale idle snapshot must not clear a task that started after the request.
emit('open');
emit('task_start', { task: 'chapter_generation' });
await reply(false);
assert.equal(context.taskRunning.value, true);
await reply(true);

// A child task ending does not unlock a still-running parent / knowledge sync.
emit('task_end', { task: 'chapter_generation', success: true });
await reply(true);
assert.equal(context.taskRunning.value, true);
assert.equal(intervals.size, 1);

// Polling also repairs a dropped end event without a disconnect.
[...intervals.values()][0]();
await reply(false);
assert.equal(context.taskRunning.value, false);

// A slow older request must not overwrite a more recent snapshot.
emit('open');
emit('open');
const older = pending.shift();
await reply(false);
older({ is_task_running: true });
await settle();
assert.equal(context.taskRunning.value, false);
assert.equal(intervals.size, 0);
console.log('sse.check.js: ok');
