<script>
  import { api } from '../lib/api.js';
  import { progress, addToast } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  let viewMode = 'list'; // list | timeline
  let categoryFilter = 'all';
  let chapterFilter = 'all';

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
  $: chapters = $progress?.chapters || [];
  $: chapterNums = [...new Set(entries.map(e => e.chapter).filter(Boolean))].sort((a, b) => a - b);
  $: categoryCounts = categoryKeys.reduce((acc, key) => {
    acc[key] = entries.filter(e => e.category === key).length;
    return acc;
  }, {});
  $: contentChars = entries.reduce((sum, e) => sum + [...(e.content || '')].length, 0);
  $: filtered = entries.filter(e => {
    if (categoryFilter !== 'all' && e.category !== categoryFilter) return false;
    if (chapterFilter !== 'all' && e.chapter !== Number(chapterFilter)) return false;
    return true;
  });
  $: timelineRows = buildTimeline(filtered);

  function extractSnippet(chapterNum, position, maxRunes = 100) {
    if (!position || !chapterNum) return '';
    const ch = chapters.find(c => c.num === chapterNum);
    if (!ch?.content) return '';
    const paragraphs = ch.content.split('\n\n');
    const idx = position - 1;
    if (idx < 0 || idx >= paragraphs.length) return '';
    const para = paragraphs[idx].trim();
    const runes = [...para];
    if (runes.length > maxRunes) return runes.slice(0, maxRunes).join('') + '…';
    return para;
  }

  function buildTimeline(items) {
    const byChapter = new Map();
    for (const e of items) {
      const n = e.chapter || 0;
      if (!byChapter.has(n)) byChapter.set(n, []);
      byChapter.get(n).push(e);
    }
    return [...byChapter.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([num, list]) => ({
        num,
        title: chapters.find(c => c.num === num)?.title || '',
        entries: [...list].sort((a, b) => a.id - b.id),
      }));
  }

  function formatEntryLine(e) {
    const snippet = extractSnippet(e.chapter, e.position);
    if (snippet) {
      return `[第${e.chapter}章] ${e.content}（原文：「${snippet}」）`;
    }
    return `[第${e.chapter}章] ${e.content}`;
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
        <span class="badge badge-outline">{$t('memory.stats.chapters', { n: chapterNums.length })}</span>
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
          <div class="tabs tabs-boxed tabs-sm">
            <button class="tab {viewMode === 'list' ? 'tab-active' : ''}" on:click={() => viewMode = 'list'}>
              {$t('memory.tabs.list')}
            </button>
            <button class="tab {viewMode === 'timeline' ? 'tab-active' : ''}" on:click={() => viewMode = 'timeline'}>
              {$t('memory.tabs.timeline')}
            </button>
          </div>
          <div class="flex flex-wrap gap-2">
            <select class="select select-bordered select-xs" bind:value={categoryFilter}>
              <option value="all">{$t('memory.filter.allCategories')}</option>
              {#each categoryKeys as key}
                {#if categoryCounts[key] > 0}
                  <option value={key}>{$t('memory.category.' + key)} ({categoryCounts[key]})</option>
                {/if}
              {/each}
            </select>
            <select class="select select-bordered select-xs" bind:value={chapterFilter}>
              <option value="all">{$t('memory.filter.allChapters')}</option>
              {#each chapterNums as num}
                <option value={num}>{$t('memory.chapterLabel', { n: num })}</option>
              {/each}
            </select>
          </div>
        </div>

        {#if filtered.length === 0}
          <p class="text-sm text-base-content/50 py-6 text-center">{$t('memory.filter.noMatch')}</p>
        {:else if viewMode === 'list'}
          <div class="overflow-x-auto">
            <table class="table table-sm">
              <thead>
                <tr>
                  <th>{$t('memory.col.id')}</th>
                  <th>{$t('memory.col.category')}</th>
                  <th>{$t('memory.col.chapter')}</th>
                  <th>{$t('memory.col.position')}</th>
                  <th>{$t('memory.col.content')}</th>
                  <th>{$t('memory.col.snippet')}</th>
                </tr>
              </thead>
              <tbody>
                {#each filtered as e (e.id)}
                  {@const snippet = extractSnippet(e.chapter, e.position)}
                  <tr>
                    <td class="font-mono text-xs">{e.id}</td>
                    <td>
                      <span class="badge badge-xs {categoryBadge[e.category] || 'badge-ghost'}">
                        {$t('memory.category.' + (e.category || 'other'))}
                      </span>
                    </td>
                    <td>{$t('memory.chapterLabel', { n: e.chapter })}</td>
                    <td class="text-base-content/60">{e.position > 0 ? e.position : '—'}</td>
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
        {:else}
          <div class="space-y-3">
            {#each timelineRows as row}
              <div class="rounded-lg bg-base-300/40 p-3">
                <div class="font-medium text-sm mb-2">
                  {$t('memory.timeline.chapter', { n: row.num })}
                  {#if row.title}
                    <span class="text-base-content/50 font-normal">· {row.title}</span>
                  {/if}
                  <span class="badge badge-xs badge-ghost ml-1">{row.entries.length}</span>
                </div>
                <div class="space-y-2">
                  {#each row.entries as e (e.id)}
                    {@const snippet = extractSnippet(e.chapter, e.position)}
                    <div class="rounded-md bg-base-200/80 px-3 py-2 text-sm">
                      <div class="flex flex-wrap items-center gap-2 mb-1">
                        <span class="font-mono text-xs text-base-content/50">#{e.id}</span>
                        <span class="badge badge-xs {categoryBadge[e.category] || 'badge-ghost'}">
                          {$t('memory.category.' + (e.category || 'other'))}
                        </span>
                        {#if e.position > 0}
                          <span class="text-xs text-base-content/40">{$t('memory.positionLabel', { n: e.position })}</span>
                        {/if}
                      </div>
                      <div>{e.content}</div>
                      {#if snippet}
                        <div class="text-xs text-base-content/50 mt-1">{$t('memory.snippetLabel')}：「{snippet}」</div>
                      {/if}
                    </div>
                  {/each}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
