<script>
  import { api } from '../lib/api.js';
  import { progress, addToast } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  let categoryFilter = 'all';

  const categoryKeys = ['character', 'location', 'item', 'event', 'promise', 'other'];
  const categoryBadge = {
    character: 'badge-primary',
    location: 'badge-secondary',
    item: 'badge-accent',
    event: 'badge-info',
    promise: 'badge-warning',
    other: 'badge-ghost',
  };

  $: entries = $progress?.memory_entries || [];
  $: maxTokens = $progress?.memory_max_tokens || 0;
  $: categoryCounts = categoryKeys.reduce((acc, key) => {
    acc[key] = entries.filter(e => e.category === key).length;
    return acc;
  }, {});
  $: contentChars = entries.reduce((sum, e) => sum + [...(e.content || '')].length, 0);
  $: filtered = entries.filter(e => {
    if (categoryFilter !== 'all' && e.category !== categoryFilter) return false;
    return true;
  });

  // 原文片段由后端在 /api/progress 中直接解析（正文不随 progress 下发）
  function extractSnippet(entry) {
    return entry?.snippet || '';
  }

  function formatEntryLine(e) {
    const snippet = extractSnippet(e);
    if (snippet) {
      return `${e.content}（原文：「${snippet}」）`;
    }
    return e.content;
  }

  async function refreshProgress() {
    try {
      progress.set(await api('GET', '/api/progress'));
      addToast($t('memory.refreshed'), 'success');
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  async function copyAll() {
    if (entries.length === 0) return;
    const text = entries.map(formatEntryLine).join('\n');
    try {
      await navigator.clipboard.writeText(text);
      addToast($t('memory.copy.done'), 'success');
    } catch (e) {
      addToast($t('memory.copy.failed'), 'error');
    }
  }
</script>

<div class="space-y-4">
  <div class="card bg-base-200 shadow-sm">
    <div class="card-body py-4 gap-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h2 class="card-title text-base">{$t('memory.title')}</h2>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-ghost btn-sm" on:click={refreshProgress}>{$t('common.refresh')}</button>
          <button class="btn btn-outline btn-sm" disabled={entries.length === 0} on:click={copyAll}>
            {$t('common.copy')}
          </button>
        </div>
      </div>
      <div class="flex flex-wrap gap-2 text-sm">
        <span class="badge badge-ghost">{$t('memory.stats.total', { n: entries.length })}</span>
        {#if maxTokens > 0}
          <span class="badge badge-outline">{$t('memory.stats.budget', { n: maxTokens })}</span>
        {/if}
        {#if contentChars > 0}
          <span class="badge badge-outline">{$t('memory.stats.chars', { n: contentChars })}</span>
        {/if}
      </div>
      <p class="text-xs text-base-content/50">{$t('memory.hint')}</p>
      <p class="text-xs text-base-content/40">{$t('memory.readonly')}</p>
    </div>
  </div>

  {#if entries.length === 0}
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body py-10 text-center gap-2">
        <p class="font-medium text-base-content/70">{$t('memory.empty.title')}</p>
        <p class="text-sm text-base-content/50 max-w-lg mx-auto">{$t('memory.empty.hint')}</p>
      </div>
    </div>
  {:else}
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body py-4 gap-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex flex-wrap gap-2">
            <select class="select select-bordered select-xs" bind:value={categoryFilter}>
              <option value="all">{$t('memory.filter.allCategories')}</option>
              {#each categoryKeys as key}
                {#if categoryCounts[key] > 0}
                  <option value={key}>{$t('memory.category.' + key)} ({categoryCounts[key]})</option>
                {/if}
              {/each}
            </select>
          </div>
        </div>

        {#if filtered.length === 0}
          <p class="text-sm text-base-content/50 py-6 text-center">{$t('memory.filter.noMatch')}</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="table table-sm">
              <thead>
                <tr>
                  <th>{$t('memory.col.id')}</th>
                  <th>{$t('memory.col.category')}</th>
                  <th>{$t('memory.col.content')}</th>
                  <th>{$t('memory.col.snippet')}</th>
                </tr>
              </thead>
              <tbody>
                {#each filtered as e (e.id)}
                  {@const snippet = extractSnippet(e)}
                  <tr>
                    <td class="font-mono text-xs">{e.id}</td>
                    <td class="whitespace-nowrap">
                      <!-- DaisyUI 5 badge 固定高度且无 nowrap；窄列里中文会逐字换行成竖排 -->
                      <span class="badge badge-sm {categoryBadge[e.category] || 'badge-ghost'} whitespace-nowrap">
                        {$t('memory.category.' + (e.category || 'other'))}
                      </span>
                    </td>
                    <td class="max-w-md">{e.content}</td>
                    <td class="text-xs text-base-content/60 max-w-xs whitespace-normal">
                      {#if snippet}
                        「{snippet}」
                      {:else}
                        <span class="text-base-content/30">—</span>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
