<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../lib/api.js';
  import { settings } from '../lib/stores.js';
  import { t, uiLocale } from '../lib/i18n/index.js';
  import { layoutParams, fitTransform, kineticEnergy } from '../lib/forceGraphLayout.js';

  let canvas;
  let container;
  let graph = null;
  let resizeObserver = null;
  let memberEdgeLabel = '';

  function stripNameMarks(name) {
    return (name.startsWith('「') && name.endsWith('」')) ? name.slice(1, -1) : name;
  }

  class ForceGraph {
    constructor(canvas, data) {
      this.canvas = canvas;
      this.ctx = canvas.getContext('2d');
      this.nodes = [];
      this.edges = [];
      this.dragging = null;
      this.hovering = null;
      this.offsetX = 0;
      this.offsetY = 0;
      this.scale = 1;
      this.panX = 0;
      this.panY = 0;
      this.alpha = 1;
      this.needsFit = true;
      this.params = layoutParams(1);
      this.activePointers = new Map();
      this.dragPointerId = null;
      this.panPointerId = null;
      this.lastPanPoint = null;
      this.pinch = null;
      this.running = true;
      this.updateData(data);
      this.setupEvents();
      this.tick();
    }
    updateData(data) {
      const prev = new Map(this.nodes.map(n => [n.id, n]));
      const chars = data.characters || [];
      const wvs = data.worldview || [];
      const orgs = data.organizations || [];
      const total = chars.length + wvs.length + orgs.length;
      this.params = layoutParams(total);
      const cx = this.canvas.width / 2;
      const cy = this.canvas.height / 2;
      const next = [];
      const place = (list, type, orbit, r) => {
        const n = list.length;
        list.forEach((item, i) => {
          const old = prev.get(item.id);
          if (old) {
            next.push({ id: item.id, label: stripNameMarks(item.name), type, x: old.x, y: old.y, vx: old.vx, vy: old.vy, r });
            return;
          }
          const a = n ? (i / n) * Math.PI * 2 : 0;
          next.push({
            id: item.id,
            label: stripNameMarks(item.name),
            type,
            x: cx + Math.cos(a) * orbit,
            y: cy + Math.sin(a) * orbit,
            vx: 0,
            vy: 0,
            r,
          });
        });
      };
      place(chars, 'character', this.params.charOrbit, 28);
      place(wvs, 'worldview', this.params.worldviewOrbit, 24);
      place(orgs, 'organization', this.params.orgOrbit, 26);

      const prevIds = [...prev.keys()].sort().join(',');
      const nextIds = next.map(n => n.id).sort().join(',');
      const structureChanged = prevIds !== nextIds;

      this.nodes = next;
      this.edges = [];
      (data.relations || []).forEach(r => {
        this.edges.push({ source: r.source_id, target: r.target_id, label: r.label });
      });
      orgs.forEach(o => {
        (o.members || []).forEach(mid => {
          this.edges.push({ source: o.id, target: mid, label: memberEdgeLabel });
        });
      });

      if (structureChanged) {
        this.alpha = 1;
        this.needsFit = true;
      }
    }
    resize(w, h) {
      if (w <= 0 || h <= 0 || (this.canvas.width === w && this.canvas.height === h)) return;
      this.canvas.width = w;
      this.canvas.height = h;
      // Viewport changed after a settled layout — re-fit so the graph still fills the area.
      if (this.alpha < 0.005 && this.nodes.length) {
        this.needsFit = true;
      }
    }
    destroy() {
      this.running = false;
      const c = this.canvas;
      c.removeEventListener('pointerdown', this.onPointerDown);
      c.removeEventListener('pointermove', this.onPointerMove);
      c.removeEventListener('pointerup', this.onPointerUp);
      c.removeEventListener('pointercancel', this.onPointerCancel);
      c.removeEventListener('lostpointercapture', this.onLostPointerCapture);
      c.removeEventListener('pointerleave', this.onPointerLeave);
      c.removeEventListener('wheel', this.onWheel);
    }
    toWorld(mx, my) { return { x: (mx - this.panX) / this.scale, y: (my - this.panY) / this.scale }; }
    wake(minAlpha = 0.35) {
      this.alpha = Math.max(this.alpha, minAlpha);
    }
    fitView() {
      const t = fitTransform(this.nodes, this.canvas.width, this.canvas.height);
      this.scale = t.scale;
      this.panX = t.panX;
      this.panY = t.panY;
    }
    localPoint(e) {
      const r = this.canvas.getBoundingClientRect();
      return { x: e.clientX - r.left, y: e.clientY - r.top, pointerType: e.pointerType };
    }
    hitTest(point) {
      const p = this.toWorld(point.x, point.y);
      for (let i = this.nodes.length - 1; i >= 0; i--) {
        const n = this.nodes[i];
        if (Math.hypot(n.x - p.x, n.y - p.y) < n.r + 4) return n;
      }
      return null;
    }
    updateHover(point) {
      this.hovering = this.hitTest(point);
      this.canvas.style.cursor = this.hovering ? 'pointer' : 'default';
    }
    clampScale(value) { return Math.max(0.15, Math.min(3, value)); }
    zoomAt(factor, mx, my) {
      const next = this.clampScale(this.scale * factor);
      if (next === this.scale) return;
      this.panX = mx - (mx - this.panX) * (next / this.scale);
      this.panY = my - (my - this.panY) * (next / this.scale);
      this.scale = next;
      this.needsFit = false; // user took over the camera
    }
    zoomBy(factor) {
      this.zoomAt(factor, this.canvas.clientWidth / 2, this.canvas.clientHeight / 2);
    }
    resetView() {
      this.fitView();
      this.needsFit = false;
      this.hovering = null;
      this.canvas.style.cursor = 'default';
    }
    touchPointers() {
      return [...this.activePointers.entries()].filter(([, p]) => p.pointerType === 'touch');
    }
    beginPinch() {
      const pointers = this.touchPointers();
      if (pointers.length < 2) return;
      const [, a] = pointers[0];
      const [, b] = pointers[1];
      const midpoint = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
      const anchor = this.toWorld(midpoint.x, midpoint.y);
      this.pinch = {
        distance: Math.max(Math.hypot(b.x - a.x, b.y - a.y), 1),
        scale: this.scale,
        anchor,
      };
      this.dragging = null;
      this.dragPointerId = null;
      this.panPointerId = null;
      this.lastPanPoint = null;
      this.hovering = null;
      this.needsFit = false;
    }
    finishPointer(pointerId) {
      this.activePointers.delete(pointerId);
      if (this.dragPointerId === pointerId) {
        this.dragging = null;
        this.dragPointerId = null;
      }
      if (this.panPointerId === pointerId) {
        this.panPointerId = null;
        this.lastPanPoint = null;
      }
      if (this.pinch) {
        const remaining = this.touchPointers();
        this.pinch = null;
        if (remaining.length === 1) {
          const [id, point] = remaining[0];
          this.panPointerId = id;
          this.lastPanPoint = point;
        }
      }
    }
    setupEvents() {
      const c = this.canvas;
      this.onPointerDown = e => {
        if (e.pointerType === 'mouse' && e.button !== 0) return;
        const point = this.localPoint(e);
        this.activePointers.set(e.pointerId, point);
        try { c.setPointerCapture(e.pointerId); } catch {}

        if (e.pointerType === 'touch' && this.touchPointers().length >= 2) {
          this.beginPinch();
          e.preventDefault();
          return;
        }

        const node = this.hitTest(point);
        if (node) {
          const world = this.toWorld(point.x, point.y);
          this.dragging = node;
          this.dragPointerId = e.pointerId;
          this.offsetX = node.x - world.x;
          this.offsetY = node.y - world.y;
          this.hovering = node;
          this.wake(0.4);
        } else if (e.pointerType === 'touch') {
          this.panPointerId = e.pointerId;
          this.lastPanPoint = point;
          this.hovering = null;
          this.needsFit = false;
        }
        if (e.pointerType === 'touch') e.preventDefault();
      };
      this.onPointerMove = e => {
        const point = this.localPoint(e);
        if (this.activePointers.has(e.pointerId)) this.activePointers.set(e.pointerId, point);

        if (this.pinch) {
          const pointers = this.touchPointers();
          if (pointers.length >= 2) {
            const [, a] = pointers[0];
            const [, b] = pointers[1];
            const midpoint = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
            const distance = Math.max(Math.hypot(b.x - a.x, b.y - a.y), 1);
            const next = this.clampScale(this.pinch.scale * distance / this.pinch.distance);
            this.scale = next;
            this.panX = midpoint.x - this.pinch.anchor.x * next;
            this.panY = midpoint.y - this.pinch.anchor.y * next;
          }
          e.preventDefault();
          return;
        }

        if (this.dragging && this.dragPointerId === e.pointerId) {
          const world = this.toWorld(point.x, point.y);
          this.dragging.x = world.x + this.offsetX;
          this.dragging.y = world.y + this.offsetY;
          this.dragging.vx = 0;
          this.dragging.vy = 0;
          this.hovering = this.dragging;
          this.wake(0.25);
          if (e.pointerType === 'touch') e.preventDefault();
          return;
        }

        if (this.panPointerId === e.pointerId && this.lastPanPoint) {
          this.panX += point.x - this.lastPanPoint.x;
          this.panY += point.y - this.lastPanPoint.y;
          this.lastPanPoint = point;
          this.needsFit = false;
          e.preventDefault();
          return;
        }

        if (e.pointerType === 'mouse') this.updateHover(point);
      };
      this.onPointerUp = e => {
        this.finishPointer(e.pointerId);
        try { c.releasePointerCapture(e.pointerId); } catch {}
      };
      this.onPointerCancel = e => this.finishPointer(e.pointerId);
      this.onLostPointerCapture = e => this.finishPointer(e.pointerId);
      this.onPointerLeave = e => {
        if (e.pointerType === 'mouse' && !this.activePointers.has(e.pointerId)) {
          this.hovering = null;
          c.style.cursor = 'default';
        }
      };
      this.onWheel = e => {
        e.preventDefault();
        const r = c.getBoundingClientRect();
        const mx = e.clientX - r.left, my = e.clientY - r.top;
        this.zoomAt(e.deltaY < 0 ? 1.1 : 1 / 1.1, mx, my);
      };

      c.addEventListener('pointerdown', this.onPointerDown);
      c.addEventListener('pointermove', this.onPointerMove);
      c.addEventListener('pointerup', this.onPointerUp);
      c.addEventListener('pointercancel', this.onPointerCancel);
      c.addEventListener('lostpointercapture', this.onLostPointerCapture);
      c.addEventListener('pointerleave', this.onPointerLeave);
      c.addEventListener('wheel', this.onWheel, { passive: false });
    }
    tick() {
      if (!this.running) return;
      this.simulate();
      this.draw();
      requestAnimationFrame(() => this.tick());
    }
    simulate() {
      if (this.alpha < 0.005) {
        if (this.needsFit) {
          this.fitView();
          this.needsFit = false;
        }
        return;
      }
      const nodes = this.nodes;
      const { restLength, repulsion, centerPull } = this.params;
      const k = 0.01;
      const damp = 0.85;
      const a = this.alpha;
      const center = { x: this.canvas.width / 2, y: this.canvas.height / 2 };
      const byId = new Map(nodes.map(n => [n.id, n]));

      for (let i = 0; i < nodes.length; i++) {
        if (nodes[i] === this.dragging) continue;
        let fx = (center.x - nodes[i].x) * centerPull * a;
        let fy = (center.y - nodes[i].y) * centerPull * a;
        for (let j = 0; j < nodes.length; j++) {
          if (i === j) continue;
          const dx = nodes[i].x - nodes[j].x;
          const dy = nodes[i].y - nodes[j].y;
          const dist = Math.max(Math.hypot(dx, dy), 1);
          const force = (repulsion / (dist * dist)) * a;
          fx += (dx / dist) * force;
          fy += (dy / dist) * force;
        }
        nodes[i].vx = (nodes[i].vx + fx) * damp;
        nodes[i].vy = (nodes[i].vy + fy) * damp;
        nodes[i].x += nodes[i].vx;
        nodes[i].y += nodes[i].vy;
      }
      for (const e of this.edges) {
        const s = byId.get(e.source);
        const t = byId.get(e.target);
        if (!s || !t) continue;
        const dx = t.x - s.x, dy = t.y - s.y, dist = Math.max(Math.hypot(dx, dy), 1);
        const force = (dist - restLength) * k * a;
        const fx = (dx / dist) * force, fy = (dy / dist) * force;
        if (s !== this.dragging) { s.vx += fx; s.vy += fy; }
        if (t !== this.dragging) { t.vx -= fx; t.vy -= fy; }
      }

      this.alpha *= 0.985;
      // Settle early when motion is already tiny (avoids long micro-jitter before fit).
      if (kineticEnergy(nodes) < 0.05 * Math.max(nodes.length, 1) && this.alpha < 0.15) {
        this.alpha = 0;
      }
    }
    nodePath(ctx, n, pad = 0) {
      ctx.beginPath();
      if (n.type === 'organization') {
        const s = (n.r + pad) * 0.7;
        ctx.save(); ctx.translate(n.x, n.y); ctx.rotate(Math.PI / 4);
        ctx.rect(-s, -s, s * 2, s * 2); ctx.restore();
      } else if (n.type === 'worldview') {
        const s = (n.r + pad) * 0.9;
        ctx.moveTo(n.x, n.y - s); ctx.lineTo(n.x + s, n.y + s * 0.6); ctx.lineTo(n.x - s, n.y + s * 0.6);
        ctx.closePath();
      } else {
        ctx.arc(n.x, n.y, n.r + pad, 0, Math.PI * 2);
      }
    }
    draw() {
      const ctx = this.ctx, w = this.canvas.width, h = this.canvas.height;
      ctx.clearRect(0, 0, w, h);
      ctx.save();
      ctx.translate(this.panX, this.panY);
      ctx.scale(this.scale, this.scale);
      const hov = this.hovering;
      let neighborIds = null;
      if (hov) {
        neighborIds = new Set();
        for (const e of this.edges) {
          if (e.source === hov.id) neighborIds.add(e.target);
          else if (e.target === hov.id) neighborIds.add(e.source);
        }
      }
      const byId = new Map(this.nodes.map(n => [n.id, n]));
      for (const e of this.edges) {
        const s = byId.get(e.source), t = byId.get(e.target);
        if (!s || !t) continue;
        const connected = hov && (e.source === hov.id || e.target === hov.id);
        ctx.globalAlpha = hov && !connected ? 0.12 : 1;
        ctx.strokeStyle = connected ? '#7e9bff' : '#4d5577';
        ctx.lineWidth = connected ? 2.5 : 1.5;
        ctx.beginPath(); ctx.moveTo(s.x, s.y); ctx.lineTo(t.x, t.y); ctx.stroke();
        if (e.label) {
          const mx = (s.x + t.x) / 2, my = (s.y + t.y) / 2;
          ctx.fillStyle = connected ? '#cdd6ff' : '#9aa0b5';
          ctx.font = connected ? 'bold 13px sans-serif' : '12px sans-serif';
          ctx.textAlign = 'center';
          ctx.fillText(e.label, mx, my - 5);
        }
      }
      ctx.globalAlpha = 1;
      const colors = { character: '#5b8af5', worldview: '#4caf50', organization: '#ff9800' };
      for (const n of this.nodes) {
        const isHov = n === hov;
        const isNeighbor = neighborIds ? neighborIds.has(n.id) : false;
        ctx.globalAlpha = hov && !isHov && !isNeighbor ? 0.25 : 1;
        ctx.fillStyle = colors[n.type] || '#5b8af5';
        this.nodePath(ctx, n);
        ctx.fill();
        if (isHov || isNeighbor) {
          ctx.strokeStyle = isHov ? '#ffffff' : 'rgba(255,255,255,0.55)';
          ctx.lineWidth = isHov ? 3 : 2;
          this.nodePath(ctx, n, 3);
          ctx.stroke();
        }
        ctx.fillStyle = '#fff'; ctx.font = 'bold 13px sans-serif'; ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
        const lbl = n.label.length > 6 ? n.label.slice(0, 5) + '…' : n.label;
        ctx.fillText(lbl, n.x, n.y);
      }
      ctx.globalAlpha = 1;
      if (hov) {
        ctx.fillStyle = 'rgba(0,0,0,0.8)'; ctx.font = '13px sans-serif';
        const tw = ctx.measureText(hov.label).width;
        ctx.fillRect(hov.x - tw / 2 - 7, hov.y - hov.r - 28, tw + 14, 22);
        ctx.fillStyle = '#fff'; ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
        ctx.fillText(hov.label, hov.x, hov.y - hov.r - 17);
      }
      ctx.restore();
    }
  }

  onMount(async () => {
    memberEdgeLabel = $t('config.rel.memberEdge');
    try { settings.set(await api('GET', '/api/settings')); } catch (e) {}
    initGraph();
    window.addEventListener('resize', handleResize);
    if (typeof ResizeObserver !== 'undefined' && container) {
      resizeObserver = new ResizeObserver(() => handleResize());
      resizeObserver.observe(container);
    }
  });

  // Refresh graph when UI locale changes so the embedded "member of" edge label tracks language.
  $: if ($uiLocale) {
    const next = $t('config.rel.memberEdge');
    if (next !== memberEdgeLabel) {
      memberEdgeLabel = next;
      if (graph && $settings) graph.updateData($settings);
    }
  }

  onDestroy(() => {
    if (graph) graph.destroy();
    if (resizeObserver) resizeObserver.disconnect();
    window.removeEventListener('resize', handleResize);
  });

  function handleResize() {
    if (!graph || !container || !canvas) return;
    const width = container.clientWidth;
    const height = container.clientHeight;
    // A collapsed mobile workspace measures 0x0. Preserve the current
    // simulation and canvas until the disclosure is visible again.
    if (width <= 0 || height <= 0) return;
    graph.resize(width, height);
  }

  function initGraph() {
    if (!canvas || !container || !$settings) return;
    const width = container.clientWidth;
    const height = container.clientHeight;
    if (width <= 0 || height <= 0) return;
    if (graph) {
      graph.resize(width, height);
      graph.updateData($settings);
    } else {
      canvas.width = width;
      canvas.height = height;
      graph = new ForceGraph(canvas, $settings);
    }
  }

  $: if ($settings && graph) {
    graph.updateData($settings);
  }

  function zoomIn() { if (graph) graph.zoomBy(1.2); }
  function zoomOut() { if (graph) graph.zoomBy(1 / 1.2); }
  function resetView() { if (graph) graph.resetView(); }
</script>

<div bind:this={container} class="relative w-full h-[calc(100vh-180px)] max-lg:h-[min(62dvh,34rem)] max-lg:min-h-88 bg-base-200 border border-base-content/10 rounded-lg overflow-hidden">
  <canvas bind:this={canvas} class="block w-full h-full" style="touch-action:none"></canvas>
  <div class="hidden max-lg:flex absolute top-2 right-2 z-10 join bg-base-300/95 rounded-lg shadow-md">
    <button type="button" class="btn btn-sm btn-ghost join-item" on:click={zoomOut} title={$t('relations.zoomOut')} aria-label={$t('relations.zoomOut')}>−</button>
    <button type="button" class="btn btn-sm btn-ghost join-item" on:click={resetView} title={$t('relations.resetView')} aria-label={$t('relations.resetView')}>↺</button>
    <button type="button" class="btn btn-sm btn-ghost join-item" on:click={zoomIn} title={$t('relations.zoomIn')} aria-label={$t('relations.zoomIn')}>+</button>
  </div>
  <div class="hidden max-lg:block absolute top-3 left-3 right-36 z-10 text-[11px] leading-tight text-base-content/60 pointer-events-none">
    {$t('relations.gestureHint')}
  </div>
  <div class="absolute bottom-3 right-3 max-lg:left-2 max-lg:right-2 max-lg:bottom-2 bg-base-300 max-lg:bg-base-300/95 border border-base-content/10 rounded-lg p-2 text-xs max-lg:text-[11px] flex gap-4 max-lg:flex-wrap max-lg:justify-center max-lg:gap-x-3 max-lg:gap-y-1">
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-[#5b8af5] mr-1 align-middle"></span>{$t('relations.legend.character')}</span>
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-[#4caf50] mr-1 align-middle"></span>{$t('relations.legend.worldview')}</span>
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-[#ff9800] mr-1 align-middle"></span>{$t('relations.legend.organization')}</span>
  </div>
</div>
