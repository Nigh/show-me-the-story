// Run: node frontend/src/lib/projectRestore.check.js
// Exercise the actual App script with mocked API/stores, without a browser.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { parse } from 'svelte/compiler';

const source = readFileSync(new URL('../App.svelte', import.meta.url), 'utf8');
const nodes = parse(source).instance.content.body;
const script = nodes.filter(n => n.type !== 'ImportDeclaration').map(n => source.slice(n.start, n.end)).join('\n');
const redirect = nodes.find(n => n.type === 'LabeledStatement' && n.body.type === 'IfStatement');
const calls = [];
let serverRunning = false;
let mount;
const context = vm.createContext({
  setTimeout, clearTimeout,
  onMount: fn => { mount = fn; }, onDestroy: () => {},
  connectSSE: () => {}, setLocale: () => {},
  $t: key => key, $uiLocale: 'en',
  api: async (method, url) => {
    calls.push([method, url]);
    if (url === '/api/version') return { version: 'v1' };
    if (url === '/api/projects/current') return { name: 'active-story', language: 'en' };
    if (url === '/api/status') return { is_task_running: serverRunning };
    if (url === '/api/config') throw new Error('config temporarily unavailable');
    return {};
  },
  // Simulate an update check that never returns.
  fetch: () => new Promise(() => {}),
});
for (const [key, value] of Object.entries({
  taskRunning: false, currentProject: null, currentPage: 'writing', progress: null,
  config: null, settings: null, chatSessions: null, currentChatSession: null,
  projectLanguage: 'zh',
})) {
  context[`$${key}`] = value;
  context[key] = { set: value => { context[`$${key}`] = value; } };
}
vm.runInContext(script, context);
mount();
await new Promise(setImmediate);
assert.equal(context.$currentProject, 'active-story', 'version/config requests must not block project recovery');
assert.equal(vm.runInContext('initializing', context), false);

// Server knows about a task before SSE has reached this tab.
serverRunning = true;
await vm.runInContext('backToProjects()', context);
assert.equal(context.$currentProject, 'active-story');
assert.equal(context.$taskRunning, true);

// An idle user may return to the picker.
serverRunning = false;
context.$taskRunning = false;
await vm.runInContext('backToProjects()', context);
assert.equal(context.$currentProject, null);

// Replay the App's reactive task-start statement while on the picker.
context.$taskRunning = true;
vm.runInContext(source.slice(redirect.start, redirect.end), context);
await new Promise(setImmediate);
assert.equal(context.$currentProject, 'active-story');
assert.ok(calls.every(([method]) => method === 'GET'), 'restoration must not select or switch a backend project');
console.log('projectRestore.check.js: ok');
