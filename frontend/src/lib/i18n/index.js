// Lightweight i18n module — no external deps.
// uiLocale is the user-selectable display language; defaults to project language.
import { writable, derived, get } from 'svelte/store';
import zh from './zh.js';
import en from './en.js';

const STORAGE_KEY = 'showmethestory.uiLocale';

const catalogs = { zh, en };

function readStored() {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'zh' || v === 'en') return v;
  } catch (e) {}
  return null;
}

export const uiLocale = writable(readStored() || 'zh');

uiLocale.subscribe(v => {
  try {
    if (v === 'zh' || v === 'en') {
      localStorage.setItem(STORAGE_KEY, v);
      document.documentElement.lang = v === 'en' ? 'en' : 'zh-CN';
    }
  } catch (e) {}
});

export function setLocale(lang) {
  if (lang !== 'zh' && lang !== 'en') return;
  uiLocale.set(lang);
}

export function getLocale() {
  return get(uiLocale);
}

function interpolatePositional(template, args) {
  if (!args || !args.length) return template;
  return template.replace(/\{(\d+)\}/g, (_, i) => {
    const idx = Number(i);
    return idx < args.length ? String(args[idx]) : `{${i}}`;
  });
}

function lookup(lang, key) {
  const dict = catalogs[lang] || catalogs.zh;
  if (Object.prototype.hasOwnProperty.call(dict, key)) return dict[key];
  // fallback to zh
  if (lang !== 'zh' && Object.prototype.hasOwnProperty.call(catalogs.zh, key)) return catalogs.zh[key];
  return key;
}

function interpolateNamed(template, params) {
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (_, name) =>
    Object.prototype.hasOwnProperty.call(params, name) ? String(params[name]) : `{${name}}`
  );
}

// Reactive translator. Use in components as `$t('some.key', {name: 'X'})`.
export const t = derived(uiLocale, ($lang) => (key, params) =>
  interpolateNamed(lookup($lang, key), params)
);

// Imperative translate (use outside Svelte components, e.g. in sse.js / api.js).
export function translate(key, params, lang) {
  return interpolateNamed(lookup(lang || get(uiLocale), key), params);
}

// Format server-keyed messages (log.* / agent.*) with positional args {0},{1},…
export function formatKeyedMessage(key, args, lang) {
  return interpolatePositional(lookup(lang || get(uiLocale), key), args || []);
}

export function formatLogEntry(entry, lang) {
  if (!entry) return '';
  const locale = lang || get(uiLocale);
  if (entry.msg_key) {
    return formatKeyedMessage(entry.msg_key, entry.msg_args, locale);
  }
  if (locale === 'en') {
    return entry.msg_en || translateServerMessage(entry.msg, 'en');
  }
  return entry.msg;
}

export function formatToolResult(msg, key, args, lang) {
  if (key) return formatKeyedMessage(key, args, lang);
  return msg || '';
}

// translateServerMessage: best-effort translation of server-emitted Chinese strings
// (toasts, log messages, error responses). Falls back to the original Chinese
// if no mapping is found. Keys are the exact Chinese strings.
const serverMessageEN = {
  // API errors (mirror of locale.go errorCatalog for the common cases)
  '缺少项目名称': 'Project name is required',
  '项目名称包含非法字符': 'Project name contains invalid characters',
  '项目已存在': 'Project already exists',
  '请先选择一个项目': 'Please select a project first',
  '有AI任务正在运行，暂不能修改，请等待任务完成或先停止任务': 'An AI task is running; please wait or stop it before editing',
  '有任务正在运行，请等待完成': 'A task is running; please wait until it finishes',
  '没有正在运行的任务': 'No task is currently running',
  '缺少 feedback 字段': 'feedback field is required',
  '缺少 content 字段': 'content field is required',
  '无效的章节编号': 'Invalid chapter number',
  '章节不存在': 'Chapter not found',
  '当前不在大纲阶段': 'Not in outline phase',
  '当前不在写作阶段': 'Not in writing phase',
  '大纲为空，请先生成大纲': 'Outline is empty; generate an outline first',
  '有正在写作/审核中的章节，请先处理后再重新生成大纲': 'There are chapters in writing/review; finish them before regenerating the outline',
  '存在已确认章节，无法整体重新生成大纲。如需追加章节请使用「生成后续大纲」': 'Confirmed chapters exist; cannot regenerate the full outline. Use "Generate Continuation Outline" to append.',
  '有正在写作/审核中的章节，请先处理后再删除大纲': 'There are chapters in writing/review; finish them before deleting the outline',
  '有任务正在运行，无法重置进度': 'A task is running; cannot reset progress',
  '有任务正在运行，无法删除章节': 'A task is running; cannot delete chapter',
  '有任务正在运行，无法删除大纲': 'A task is running; cannot delete outline',
  '有任务正在运行，无法删除项目': 'A task is running; cannot delete project',
  '不能删除当前正在使用的项目': 'Cannot delete the currently active project',
  '项目不存在': 'Project not found',
  '没有可删除的章节': 'No chapters to delete',
  '正在写作中的章节无法删除': 'Cannot delete a chapter that is being written',
  '删除范围内有正在写作中的章节，无法删除': 'Delete range contains a chapter being written; cannot delete',
  '请先生成大纲': 'Generate an outline first',
  '续写前请先重置进度': 'Reset progress before importing continuation',
  '请先分析内容': 'Analyse the content first',
  '分析结果中没有任何章节': 'Analysis result contains no chapters',
  '全书尚未完成（需所有章节已确认）': 'Book is not yet complete (all chapters must be confirmed)',
  '没有启用的润色技能，请先在技能管理页启用 polish 类技能': 'No polish skill enabled; enable a polish-type skill on the Skills page first',
  '章节内容为空，无法润色': 'Chapter content is empty; cannot polish',
  '章节正在写作中，无法润色': 'Chapter is being written; cannot polish',
  '请指定章节编号': 'Chapter number is required',
  '没有可优化的章节（需要至少两个相邻的已确认章节）': 'No transitions to optimise (need at least two adjacent confirmed chapters)',
  '缺少诊断或核查报告，请先运行全书诊断': 'Diagnosis or consistency report is missing; run book diagnosis first',
  '没有可执行的优化工单': 'No roadmap items to execute',
  '请至少勾选一条待执行的工单': 'Select at least one pending roadmap item',
  '会话不存在': 'Chat session not found',
  '技能不存在': 'Skill not found',
  '角色名不能为空': 'Character name is required',
  '角色不存在': 'Character not found',
  '名称和描述不能为空': 'Name and description are required',
  '世界观条目不存在': 'Worldview entry not found',
  '组织名不能为空': 'Organization name is required',
  '组织不存在': 'Organization not found',
  '源和目标不能为空': 'Source and target are required',
  '关系不存在': 'Relation not found',
  '缺少 name': 'name field is required',
  '缺少 description': 'description field is required',
  '伏笔不存在': 'Foreshadow not found',
  '无效的伏笔ID': 'Invalid foreshadow id',
  '连接成功': 'Connection succeeded',

  // Log lines (most common only)
  '正在生成小说大纲...': 'Generating novel outline...',
  '正在根据意见修订大纲...': 'Revising outline based on feedback...',
  '正在根据意见修改当前章节...': 'Revising current chapter based on feedback...',
  '正在分析已有内容...': 'Analysing existing content...',
  '正在分析大纲，设计伏笔方案...': 'Analysing outline and designing foreshadow plan...',
  '正在协调新设定与已有内容...': 'Reconciling new settings with existing content...',
  '正在生成续写大纲...': 'Generating continuation outline...',
  '大纲生成完成！': 'Outline generation complete.',
  '大纲已修订。': 'Outline revised.',
  '设定协调完成！': 'Settings reconciliation complete.',
  '续写大纲生成完成！': 'Continuation outline complete.',
  '助理回复完成': 'Assistant reply complete.',
  '大纲生成已取消': 'Outline generation cancelled',
  '大纲修订已取消': 'Outline revision cancelled',
  '章节创作已取消': 'Chapter writing cancelled',
  '章节修订已取消': 'Chapter revision cancelled',
  '伏笔建议已取消': 'Foreshadow suggestions cancelled',
  '内容分析已取消': 'Content analysis cancelled',
  '续写大纲生成已取消': 'Continuation outline cancelled',
  '设定协调已取消': 'Settings reconciliation cancelled',
  '章节润色已取消': 'Chapter polish cancelled',
  '助理对话已取消': 'Assistant chat cancelled',
  '全书优化分析已取消': 'Full-book analysis cancelled',
  '全书一致性核查已取消': 'Consistency check cancelled',
  '路线图生成已取消': 'Roadmap generation cancelled',
  '全书优化执行已取消（已完成项不会丢失）': 'Optimisation execution cancelled (completed items are preserved)',
  '章节衔接优化已取消（已完成部分不会丢失）': 'Transition smoothing cancelled (completed parts are preserved)',
  '已开启自动确认模式：每章生成完成后将自动确认并继续生成下一章': 'Auto-confirm enabled: each chapter is automatically confirmed once generation completes, then the next chapter begins.',
  '已关闭自动确认模式': 'Auto-confirm disabled',
  '大纲已确认，进入写作阶段。': 'Outline confirmed. Entering writing phase.',
  '大纲已删除。': 'Outline deleted.',
  '全部章节已创作完成！': 'All chapters generated.',
};

export function translateServerMessage(msg, lang) {
  if (msg == null) return msg;
  const target = lang || get(uiLocale);
  if (target !== 'en') return msg;
  if (Object.prototype.hasOwnProperty.call(serverMessageEN, msg)) return serverMessageEN[msg];
  // Try common dynamic prefixes
  const prefixMap = [
    ['保存失败: ', 'Save failed: '],
    ['保存进度失败: ', 'Failed to save progress: '],
    ['保存配置失败: ', 'Failed to save config: '],
    ['项目「', 'Project "'],
    ['第 ', 'Chapter '],
    ['正在创作第 ', 'Writing chapter '],
    ['第 ', 'Chapter '],
  ];
  for (const [zh, en] of prefixMap) {
    if (msg.startsWith(zh)) return en + msg.slice(zh.length);
  }
  return msg;
}
