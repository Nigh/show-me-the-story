<script>
  import { createEventDispatcher } from 'svelte';
  import { api } from '../lib/api.js';
  import { progress, settings, taskRunning, addToast } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';
  export let chapterNum = 0;
  export let facts = [];
  export let activeFact = null;
  const dispatch = createEventDispatcher();
  let pending = [];
  let changes = [];
  let request = 0;
  function changedFields(change) {
    return [...new Set([...Object.keys(change.before || {}), ...Object.keys(change.after || {})])].filter(k => k !== 'id' && JSON.stringify(change.before?.[k]) !== JSON.stringify(change.after?.[k]));
  }
  function displayValue(value) {
    const entities = [...($settings?.characters || []), ...($settings?.worldview || []), ...($settings?.organizations || [])];
    const name = id => entities.find(e => e.id === id)?.name || id;
    if (Array.isArray(value)) return value.map(name).join(', ');
    return value ? name(String(value)) : '—';
  }
  $: refresh(chapterNum, $progress, $settings, $taskRunning);
  async function refresh(num, p, s, running) {
    if (running) return;
    const token = ++request;
    try {
      const data = await api('GET', '/api/knowledge?chapter=' + num);
      if (token !== request) return;
      facts = data.facts || [];
      pending = data.pending_chapters || [];
      changes = (data.changes || []).filter(c => c.status === 'pending');
      if (activeFact) activeFact = facts.find(f => f.id === activeFact.id) || activeFact;
    } catch (e) { addToast(e.message, 'error'); }
  }
  async function retry() {
    try { await api('POST', '/api/knowledge/sync'); }
    catch (e) { addToast(e.message, 'error'); }
  }
  async function resolve(change, accept) {
    try {
      settings.set(await api('POST', '/api/settings/story-changes', { id: change.id, accept }));
    } catch (e) { addToast(e.message, 'error'); }
  }
</script>

<div class="card bg-base-200 shadow-sm">
  <div class="card-body p-4 gap-3 text-sm">
    <h3 class="card-title text-base">{$t('facts.title')}</h3>
    {#if pending.length}
      <div class="flex flex-wrap items-center gap-2 text-warning">{$t('facts.pending', {chapters: pending.join(', ')})}
        <button class="btn btn-xs" disabled={$taskRunning} on:click={retry}>{$t('facts.retry')}</button>
      </div>
    {/if}
    {#if facts.length === 0}<p class="text-base-content/60">{$t('facts.empty')}</p>{/if}
    <div class="flex flex-col items-start gap-2">
      {#each facts as f}
        <button class="btn btn-sm h-auto min-h-8 max-w-full py-2 text-left justify-start whitespace-normal break-words font-normal" aria-pressed={activeFact?.id === f.id} class:btn-primary={activeFact?.id === f.id} on:click={() => activeFact = activeFact?.id === f.id ? null : f}>#{f.id} {f.content}</button>
      {/each}
    </div>
    {#if activeFact}
      <p class="font-medium">{activeFact.content}</p>
      <p class="text-xs text-base-content/60">{$t('facts.links')}</p>
      <ul class="space-y-1 max-h-60 overflow-auto">
        {#each activeFact.references || [] as ref}
          <li>
            <button class="text-left w-full rounded-lg p-2 hover:bg-base-300 disabled:opacity-60 break-words" disabled={ref.stale} on:click={() => dispatch('jump', ref)}>
              {$t('facts.source', {chapter: ref.chapter, block: ref.block_id})}
              {#if ref.stale}<span class="text-warning"> — {$t('facts.stale')}</span>{/if}
              <span class="block text-xs text-base-content/70 line-clamp-2">{ref.quote}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
    {#if changes.length}
      <details>
        <summary class="cursor-pointer">{$t('facts.settingsPending', {count: changes.length})}</summary>
        {#each changes as change}
          <div class="border-t border-base-content/10 py-2 space-y-1">
            <p>{change.after?.name || change.before?.name || change.after?.label || displayValue(change.entity_id)} — {change.reason === 'source_changed' ? $t('facts.sourceChanged') : change.reason}</p>
            {#each changedFields(change) as field}
              <p class="text-xs font-medium">{$t('facts.field.' + field)}</p>
              <div class="grid grid-cols-1 @sm:grid-cols-2 gap-2 text-sm">
                <div class="bg-base-100 p-2 rounded"><span>{$t('facts.before')}</span><p class="whitespace-pre-wrap break-words">{displayValue(change.before?.[field])}</p></div>
                <div class="bg-base-100 p-2 rounded"><span>{$t('facts.after')}</span><p class="whitespace-pre-wrap break-words">{displayValue(change.after?.[field])}</p></div>
              </div>
            {/each}
            <button class="btn btn-xs" disabled={$taskRunning || change.reason === 'source_changed'} on:click={() => dispatch('jump', change.source)}>{$t('facts.source', {chapter: change.source.chapter, block: change.source.block_id})}</button>
            <button class="btn btn-xs btn-primary" disabled={$taskRunning} on:click={() => resolve(change, true)}>{$t('facts.accept')}</button>
            <button class="btn btn-xs" disabled={$taskRunning} on:click={() => resolve(change, false)}>{$t('facts.ignore')}</button>
          </div>
        {/each}
      </details>
    {/if}
  </div>
</div>
