// Run: node frontend/src/lib/i18n.check.js
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { get } from 'svelte/store';
import { parse, walk } from 'svelte/compiler';
import { t, setLocale, formatLogEntry } from './i18n/index.js';
import zh from './i18n/zh.js';
import en from './i18n/en.js';

const app = readFileSync(new URL('../App.svelte', import.meta.url), 'utf8');
const head = parse(app).html.children.find(n => n.type === 'Head');
const title = head.children.find(n => n.type === 'Title').children.find(n => n.type === 'MustacheTag').expression;
const titleExpression = app.slice(title.start, title.end);
const chat = readFileSync(new URL('../components/ChatPanel.svelte', import.meta.url), 'utf8');
let sessionExpression;
walk(parse(chat).html, {
  enter(node) {
    if (node.type === 'MustacheTag') {
      const expression = chat.slice(node.expression.start, node.expression.end);
      if (expression.includes('chat.session.defaultTitle')) sessionExpression = expression;
    }
  },
});
assert.ok(sessionExpression, 'empty chat title must use the UI locale');
assert.deepEqual(Object.keys(zh).sort(), Object.keys(en).sort(), 'catalog keys must match');

for (const [lang, windowTitle, chatTitle, placeholder] of [
  ['en', 'AI Novel Generator', 'New chat', 'No session selected'],
  ['zh', 'AI 小说写手', '新会话', '未选择会话'],
  ['en', 'AI Novel Generator', 'New chat', 'No session selected'],
]) {
  setLocale(lang);
  const context = vm.createContext({ $t: get(t), $currentChatSession: { title: '新会话' }, msgs: [] });
  assert.equal(vm.runInContext(titleExpression, context), windowTitle);
  assert.equal(vm.runInContext(sessionExpression, context), chatTitle, 'legacy empty sessions must translate too');
  context.$currentChatSession = { title: '用户写的标题' };
  context.msgs = [{ role: 'user', content: '用户写的标题' }];
  assert.equal(vm.runInContext(sessionExpression, context), '用户写的标题', 'user text must stay unchanged');
  context.$currentChatSession = null;
  context.msgs = [];
  assert.equal(vm.runInContext(sessionExpression, context), placeholder);
}
assert.equal(formatLogEntry({ msg_key: 'log.agent_step_messages', msg_args: ['1', '10', '3', '[system user]'] }, 'en'),
  '[Agent] Step 1/10: 3 messages: [system user]');
assert.equal(formatLogEntry({ msg_key: 'log.agent_step_tool_done', msg_args: ['2', 'search_project', '原文'] }, 'en'),
  '[Agent] Step 2: Tool search_project completed. Result: 原文');
console.log('i18n.check.js: ok');
