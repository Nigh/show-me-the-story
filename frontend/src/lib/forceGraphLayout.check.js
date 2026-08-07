/**
 * Self-check: node frontend/src/lib/forceGraphLayout.check.js
 */
import assert from 'node:assert/strict';
import { layoutParams, fitTransform, kineticEnergy } from './forceGraphLayout.js';

const p8 = layoutParams(8);
const p32 = layoutParams(32);
assert.ok(p32.restLength > p8.restLength);
assert.ok(p32.repulsion > p8.repulsion);
assert.ok(p32.charOrbit > p8.charOrbit);
assert.ok(p32.centerPull < p8.centerPull);

const fit = fitTransform(
  [
    { x: 0, y: 0, r: 10 },
    { x: 400, y: 200, r: 10 },
  ],
  800,
  600,
);
assert.ok(fit.scale > 0 && fit.scale <= 3);
assert.ok(Number.isFinite(fit.panX) && Number.isFinite(fit.panY));

const empty = fitTransform([], 800, 600);
assert.equal(empty.scale, 1);

assert.ok(kineticEnergy([{ vx: 3, vy: 4 }]) === 25);
assert.equal(kineticEnergy([]), 0);

console.log('forceGraphLayout.check: ok');
