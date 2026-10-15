import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { parse } from 'svelte/compiler';

const source = readFileSync(new URL('../pages/Writing.svelte', import.meta.url), 'utf8');
const fn = parse(source).instance.content.body.find(n => n.type === 'FunctionDeclaration' && n.id.name === 'restartChapter');
let confirm, fail = false;
const calls = [], toasts = [];
const context = vm.createContext({
  ch: { num: 3 }, loadedRev: 'original-revision', chapterContent: 'original', chapterBlocks: [1],
  facts: [1], activeFact: {}, returnRef: {}, showRevise: true,
  $t: key => key, confirmAction: (message, action) => { confirm = action; },
  cancelBlockOps() {}, addToast: (message, type) => toasts.push(type),
  progress: { set: value => calls.push(value) },
  api: async (method, url, body, headers) => {
    calls.push({ method, url, headers });
    if (fail) throw new Error('stale revision');
    return 'updated-progress';
  },
});
vm.runInContext(source.slice(fn.start, fn.end), context);
vm.runInContext('restartChapter()', context);
assert.equal(calls.length, 0, 'reset must wait for confirmation');
context.ch = { num: 4 };
context.loadedRev = 'new-revision';
await confirm();
assert.equal(calls[0].url, '/api/chapter?num=3', 'confirmation must retain the original chapter');
assert.equal(calls[0].headers['X-Content-Rev'], 'original-revision');
assert.equal(context.chapterContent, 'original', 'reset must not clear another selected chapter');
fail = true;
vm.runInContext('restartChapter()', context);
await confirm();
assert.equal(context.chapterContent, 'original', 'failure must retain prose');
assert.equal(toasts.at(-1), 'error');
fail = false;
context.loadedRev = '';
vm.runInContext('restartChapter()', context);
await confirm();
assert.equal(context.chapterContent, '', 'empty review chapters must also be restartable');
assert.equal(context.facts.length, 0);
assert.equal(toasts.at(-1), 'success');
console.log('chapterRestart.check.js: ok');
