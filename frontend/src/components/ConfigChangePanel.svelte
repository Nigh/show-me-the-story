<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import {
    pendingConfigChanges,
    showConfigChangePanel,
    config,
    progress,
    taskRunning,
    addToast,
  } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  const sourceKeys = {
    outline_generation: 'configChange.source.outline_generation',
    outline_revision: 'configChange.source.outline_revision',
    reconcile: 'configChange.source.reconcile',
    agent: 'configChange.source.agent',
  };

  async function loadPending() {
    try {
      const res = await api('GET', '/api/config/pending-changes');
      const items = (res?.changes || []).map(c => ({ ...c, _selected: true }));
      pendingConfigChanges.set(items);
      showConfigChangePanel.set(items.length > 0);
    } catch (e) {
      // ignore when no project
    }
  }

  onMount(loadPending);

  function fieldLabel(field) {
    const key = 'configChange.field.' + field;
    const label = $t(key);
    return label === key ? field : label;
  }

  function sourceLabel(source) {
    const key = sourceKeys[source];
    return key ? $t(key) : source;
  }

  async function applySelected() {
    const selected = $pendingConfigChanges.filter(c => c._selected !== false);
    if (selected.length === 0) {
      addToast($t('configChange.pickRequired'), 'error');
      return;
    }
    try {
      await api('POST', '/api/config/apply-changes', { fields: selected.map(c => c.field) });
      pendingConfigChanges.set([]);
      showConfigChangePanel.set(false);
      config.set(await api('GET', '/api/config'));
      progress.set(await api('GET', '/api/progress'));
      addToast($t('configChange.applied', { n: selected.length }), 'success');
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  async function dismissAll() {
    try {
      await api('DELETE', '/api/config/pending-changes');
      pendingConfigChanges.set([]);
      showConfigChangePanel.set(false);
    } catch (e) {
      addToast(e.message, 'error');
    }
  }
</script>

{#if $showConfigChangePanel && $pendingConfigChanges.length > 0}
  <div class="config-change-panel card bg-base-200 border border-primary/30 shadow-sm">
    <div class="config-change-body card-body py-4 gap-3">
      <h3 class="font-semibold">{$t('configChange.title', { n: $pendingConfigChanges.length })}</h3>
      <p class="text-sm text-base-content/60">{$t('configChange.hint')}</p>
      <div class="config-change-list space-y-2 max-h-80 overflow-y-auto">
        {#each $pendingConfigChanges as c}
          <label class="flex gap-3 p-3 rounded-lg bg-base-300/50 cursor-pointer">
            <input type="checkbox" class="checkbox checkbox-sm mt-1" bind:checked={c._selected} disabled={$taskRunning} />
            <div class="min-w-0 flex-1 space-y-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-medium">{fieldLabel(c.field)}</span>
                <span class="badge badge-xs badge-ghost">{sourceLabel(c.source)}</span>
              </div>
              {#if c.reason}
                <p class="text-xs text-base-content/50">{c.reason}</p>
              {/if}
              <div class="text-xs text-base-content/60">
                <div><span class="text-base-content/40">{$t('configChange.current')}:</span> {c.current || $t('configChange.empty')}</div>
                <div class="mt-0.5"><span class="text-base-content/40">{$t('configChange.proposed')}:</span> {c.proposed}</div>
              </div>
            </div>
          </label>
        {/each}
      </div>
      <div class="config-change-actions flex gap-2">
        <button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={applySelected}>{$t('configChange.adopt')}</button>
        <button class="btn btn-ghost btn-sm" disabled={$taskRunning} on:click={dismissAll}>{$t('configChange.dismiss')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  @media (max-width: 63.999rem) {
    .config-change-body { min-width: 0; padding: 1rem; }
    .config-change-list { max-height: min(20rem, 50dvh); overscroll-behavior: contain; }
    .config-change-list label { min-width: 0; }
    .config-change-list p, .config-change-list .text-xs { overflow-wrap: anywhere; }
    .config-change-actions { width: 100%; }
    .config-change-actions .btn { flex: 1; min-height: 44px; }
  }
</style>
