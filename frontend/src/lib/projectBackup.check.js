// Run: node frontend/src/lib/projectBackup.check.js
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { parse } from 'svelte/compiler';

const source = readFileSync(new URL('../pages/Projects.svelte', import.meta.url), 'utf8');
const script = parse(source).instance.content.body.filter(n => n.type !== 'ImportDeclaration')
  .map(n => source.slice(n.start, n.end)).join('\n');
const calls = [], toasts = [], downloads = [];
let fail = true;
const context = vm.createContext({
  onMount: () => {}, $t: key => key,
  addToast: (message, type) => toasts.push(type),
  apiFetch: async (url, opts) => {
    calls.push({ url, opts });
    if (fail) throw new Error('request failed');
    return { blob: async () => 'archive' };
  },
  api: async () => [], projects: { set() {} },
  document: { createElement: () => ({ click() { downloads.push(this.download); } }) },
  URL: { createObjectURL: () => 'blob:backup', revokeObjectURL() {} },
  setTimeout: fn => fn(),
});
vm.runInContext(script, context);
await vm.runInContext('backupProject("my story")', context);
assert.equal(downloads.length, 0, 'failed download must not create a download');
assert.equal(vm.runInContext('backupBusy', context), false);
fail = false;
await vm.runInContext('backupProject("my story")', context);
assert.match(downloads[0], /^my story-.*\.zip$/);
assert.equal(calls.at(-1).url, '/api/projects/my%20story/backup');

vm.runInContext('restoreName = "new story"; restoreFiles = ["zip bytes"]; restoreInput = { value: "selected.zip" };', context);
fail = true;
await vm.runInContext('restoreProject()', context);
assert.equal(vm.runInContext('restoreName', context), 'new story');
assert.equal(toasts.at(-1), 'error');
assert.equal(vm.runInContext('backupBusy', context), false);
fail = false;
await vm.runInContext('restoreProject()', context);
assert.equal(calls.at(-1).url, '/api/projects/restore?name=new%20story');
assert.equal(calls.at(-1).opts.body, 'zip bytes');
assert.equal(toasts.at(-1), 'success');
assert.equal(vm.runInContext('restoreName', context), '');
assert.equal(vm.runInContext('restoreInput.value', context), '');
console.log('projectBackup.check.js: ok');
