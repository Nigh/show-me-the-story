/**
 * Pure layout helpers for Relations ForceGraph.
 * Spacing / repulsion scale with √N so denser casts do not collapse into the viewport box.
 */

/** @param {number} n node count */
export function layoutParams(n) {
  const count = Math.max(n, 1);
  // ponytail: √N density vs baseline ~8 nodes; upgrade to Barnes–Hut if casts >> 200
  const density = Math.sqrt(count / 8);
  return {
    restLength: 170 * density,
    repulsion: 2200 * density * density,
    charOrbit: 180 * density,
    worldviewOrbit: 240 * density,
    orgOrbit: 300 * density,
    centerPull: 0.0006 / density,
  };
}

/**
 * Camera transform that fits all nodes into the viewport.
 * @param {{x:number,y:number,r?:number}[]} nodes
 * @param {number} viewW
 * @param {number} viewH
 * @param {{padding?:number,minScale?:number,maxScale?:number}} [opts]
 */
export function fitTransform(nodes, viewW, viewH, opts = {}) {
  const padding = opts.padding ?? 48;
  const minScale = opts.minScale ?? 0.15;
  const maxScale = opts.maxScale ?? 3;
  if (!nodes.length || viewW <= 0 || viewH <= 0) {
    return { scale: 1, panX: 0, panY: 0 };
  }
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const n of nodes) {
    const r = n.r ?? 0;
    minX = Math.min(minX, n.x - r);
    minY = Math.min(minY, n.y - r);
    maxX = Math.max(maxX, n.x + r);
    maxY = Math.max(maxY, n.y + r);
  }
  const bw = Math.max(maxX - minX, 1);
  const bh = Math.max(maxY - minY, 1);
  const scale = Math.max(
    minScale,
    Math.min(maxScale, Math.min((viewW - 2 * padding) / bw, (viewH - 2 * padding) / bh)),
  );
  return {
    scale,
    panX: (viewW - bw * scale) / 2 - minX * scale,
    panY: (viewH - bh * scale) / 2 - minY * scale,
  };
}

/** Sum of squared velocities — used as a settle signal. */
export function kineticEnergy(nodes) {
  let e = 0;
  for (const n of nodes) e += (n.vx || 0) ** 2 + (n.vy || 0) ** 2;
  return e;
}
