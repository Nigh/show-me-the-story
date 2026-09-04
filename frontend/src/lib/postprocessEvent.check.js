/**
 * Self-check: node frontend/src/lib/postprocessEvent.check.js
 */
import assert from 'node:assert/strict';
import { normalizePostProcessPayload } from './postprocessEvent.js';

const bare = {
  roadmap: [{ id: '1' }],
  execute_options: { run_smooth_transitions_first: true, include_polish: false },
  author_requirements: 'keep names',
};
const wrapped = {
  book_complete: true,
  state: bare,
};

assert.deepEqual(normalizePostProcessPayload(wrapped, null), wrapped);

const fromBare = normalizePostProcessPayload(bare, null);
assert.equal(fromBare.book_complete, true);
assert.deepEqual(fromBare.state, bare);

const prev = { book_complete: false, state: { roadmap: [] } };
const merged = normalizePostProcessPayload(bare, prev);
assert.equal(merged.book_complete, false);
assert.deepEqual(merged.state, bare);

assert.equal(normalizePostProcessPayload(null, prev), prev);
assert.equal(normalizePostProcessPayload(null, null), null);

console.log('postprocessEvent.check.js: ok');
