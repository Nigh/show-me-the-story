<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../lib/api.js';
  import { settings } from '../lib/stores.js';
  import { t, uiLocale } from '../lib/i18n/index.js';

  let canvas;
  let container;
  let graph = null;
  let memberEdgeLabel = '';

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
      this.running = true;
      this.updateData(data);
      this.setupEvents();
      this.tick();
    }
    updateData(data) {
      this.nodes = [];
      this.edges = [];
      const chars = data.characters || [];
      const wvs = data.worldview || [];
      const orgs = data.organizations || [];
      const cx = this.canvas.width / 2;
      const cy = this.canvas.height / 2;
      chars.forEach((c, i) => {
        this.nodes.push({ id: c.id, label: c.name, type: 'character', x: cx + Math.cos(i * 2) * 180, y: cy + Math.sin(i * 2) * 180, vx: 0, vy: 0, r: 28 });
      });
      wvs.forEach((w, i) => {
        this.nodes.push({ id: w.id, label: w.name, type: 'worldview', x: cx + Math.cos(i * 2 + 1) * 240, y: cy + Math.sin(i * 2 + 1) * 240, vx: 0, vy: 0, r: 24 });
      });
      orgs.forEach((o, i) => {
        this.nodes.push({ id: o.id, label: o.name, type: 'organization', x: cx + Math.cos(i * 2 + 2) * 300, y: cy + Math.sin(i * 2 + 2) * 300, vx: 0, vy: 0, r: 26 });
      });
      (data.relations || []).forEach(r => {
        this.edges.push({ source: r.source_id, target: r.target_id, label: r.label });
      });
      orgs.forEach(o => {
        (o.members || []).forEach(mid => {
          this.edges.push({ source: o.id, target: mid, label: memberEdgeLabel });
        });
      });
    }
    resize(w, h) { this.canvas.width = w; this.canvas.height = h; }
    destroy() { this.running = false; }
    toWorld(mx, my) { return { x: (mx - this.panX) / this.scale, y: (my - this.panY) / this.scale }; }
    setupEvents() {
      const c = this.canvas;
      c.addEventListener('mousedown', e => {
        const r = c.getBoundingClientRect();
        const p = this.toWorld(e.clientX - r.left, e.clientY - r.top);
        for (let i = this.nodes.length - 1; i >= 0; i--) {
          const n = this.nodes[i];
          if (Math.hypot(n.x - p.x, n.y - p.y) < n.r + 4) {
            this.dragging = n; this.offsetX = n.x - p.x; this.offsetY = n.y - p.y; break;
          }
        }
      });
      c.addEventListener('mousemove', e => {
        const r = c.getBoundingClientRect();
        const p = this.toWorld(e.clientX - r.left, e.clientY - r.top);
        if (this.dragging) { this.dragging.x = p.x + this.offsetX; this.dragging.y = p.y + this.offsetY; this.dragging.vx = 0; this.dragging.vy = 0; }
        this.hovering = null;
        for (let i = this.nodes.length - 1; i >= 0; i--) {
          if (Math.hypot(this.nodes[i].x - p.x, this.nodes[i].y - p.y) < this.nodes[i].r + 4) { this.hovering = this.nodes[i]; break; }
        }
        c.style.cursor = this.hovering ? 'pointer' : 'default';
      });
      c.addEventListener('mouseup', () => { this.dragging = null; });
      c.addEventListener('mouseleave', () => { this.dragging = null; this.hovering = null; });
      c.addEventListener('wheel', e => {
        e.preventDefault();
        const r = c.getBoundingClientRect();
        const mx = e.clientX - r.left, my = e.clientY - r.top;
        const factor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
        const next = Math.max(0.3, Math.min(3, this.scale * factor));
        // 以光标位置为中心缩放
        this.panX = mx - (mx - this.panX) * (next / this.scale);
        this.panY = my - (my - this.panY) * (next / this.scale);
        this.scale = next;
      }, { passive: false });
    }
    tick() {
      if (!this.running) return;
      this.simulate();
      this.draw();
      requestAnimationFrame(() => this.tick());
    }
    simulate() {
      const nodes = this.nodes, k = 0.01, damp = 0.85;
      const center = { x: this.canvas.width / 2, y: this.canvas.height / 2 };
      for (let i = 0; i < nodes.length; i++) {
        if (nodes[i] === this.dragging) continue;
        let fx = (center.x - nodes[i].x) * 0.001, fy = (center.y - nodes[i].y) * 0.001;
        for (let j = 0; j < nodes.length; j++) {
          if (i === j) continue;
          const dx = nodes[i].x - nodes[j].x, dy = nodes[i].y - nodes[j].y;
          const dist = Math.max(Math.hypot(dx, dy), 1);
          const force = 2200 / (dist * dist);
          fx += dx / dist * force; fy += dy / dist * force;
        }
        nodes[i].vx = (nodes[i].vx + fx) * damp; nodes[i].vy = (nodes[i].vy + fy) * damp;
        nodes[i].x += nodes[i].vx; nodes[i].y += nodes[i].vy;
        nodes[i].x = Math.max(nodes[i].r, Math.min(this.canvas.width - nodes[i].r, nodes[i].x));
        nodes[i].y = Math.max(nodes[i].r, Math.min(this.canvas.height - nodes[i].r, nodes[i].y));
      }
      for (const e of this.edges) {
        const s = nodes.find(n => n.id === e.source), t = nodes.find(n => n.id === e.target);
        if (!s || !t) continue;
        const dx = t.x - s.x, dy = t.y - s.y, dist = Math.max(Math.hypot(dx, dy), 1);
        const force = (dist - 170) * k, fx = dx / dist * force, fy = dy / dist * force;
        if (s !== this.dragging) { s.vx += fx; s.vy += fy; }
        if (t !== this.dragging) { t.vx -= fx; t.vy -= fy; }
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
      for (const e of this.edges) {
        const s = this.nodes.find(n => n.id === e.source), t = this.nodes.find(n => n.id === e.target);
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
    window.removeEventListener('resize', handleResize);
  });

  function handleResize() {
    if (graph && container && canvas) {
      canvas.width = container.clientWidth;
      canvas.height = container.clientHeight;
      graph.resize(canvas.width, canvas.height);
    }
  }

  function initGraph() {
    if (!canvas || !container || !$settings) return;
    canvas.width = container.clientWidth;
    canvas.height = container.clientHeight;
    if (graph) {
      graph.resize(canvas.width, canvas.height);
      graph.updateData($settings);
    } else {
      graph = new ForceGraph(canvas, $settings);
    }
  }

  $: if ($settings && graph) {
    graph.updateData($settings);
  }
</script>

<div bind:this={container} class="relative w-full bg-base-200 border border-base-content/10 rounded-lg overflow-hidden" style="height:calc(100vh - 180px)">
  <canvas bind:this={canvas}></canvas>
  <div class="absolute bottom-3 right-3 bg-base-300 border border-base-content/10 rounded-lg p-2 text-xs flex gap-4">
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-[#5b8af5] mr-1 align-middle"></span>{$t('relations.legend.character')}</span>
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-[#4caf50] mr-1 align-middle"></span>{$t('relations.legend.worldview')}</span>
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-[#ff9800] mr-1 align-middle"></span>{$t('relations.legend.organization')}</span>
  </div>
</div>
