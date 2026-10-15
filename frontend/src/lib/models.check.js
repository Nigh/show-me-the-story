import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { parse } from 'svelte/compiler';

const source = readFileSync(new URL('../pages/Config.svelte', import.meta.url), 'utf8');
const body = parse(source).instance.content.body;
const fn = body.find(n => n.type === 'FunctionDeclaration' && n.id.name === 'loadModels');
const sig = body.find(n => n.type === 'VariableDeclaration' && n.declarations[0].id.name === 'modelEndpointSig');
let resolve, reject;
const context = vm.createContext({
  loadingModels: false, models: [], modelsError: '', modelsEndpoint: '',
  localApiCfg: {base_url: 'http://first', api_key: 'key', model: 'custom-model'},
  api: () => new Promise((ok, fail) => { resolve = ok; reject = fail; }),
});
vm.runInContext(source.slice(sig.start, sig.end) + '\n' + source.slice(fn.start, fn.end), context);
let pending = vm.runInContext('loadModels()', context);
context.localApiCfg.base_url = 'http://second';
resolve([{id:'old-model'}]);
await pending;
assert.equal(context.models.length, 0, 'old endpoint response must be ignored');
pending = vm.runInContext('loadModels()', context);
resolve([{id:'listed-model', context_window:8192}]);
await pending;
assert.equal(context.models[0].id, 'listed-model');
assert.equal(context.localApiCfg.model, 'custom-model', 'discovery must preserve custom names');
pending = vm.runInContext('loadModels()', context);
reject(new Error('unsupported'));
await pending;
assert.equal(context.models.length, 0);
assert.equal(context.modelsError, 'unsupported');
assert.equal(context.localApiCfg.model, 'custom-model');
assert.equal(context.loadingModels, false);
assert.match(source, /<input\b[^>]*\blist="api-model-options"/);
console.log('models.check.js: ok');
