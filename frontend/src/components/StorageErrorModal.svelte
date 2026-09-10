<script>
  import { storageError, addToast } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  function close() {
    storageError.set(null);
  }

  function diagnostics(d) {
    return [
      `file: ${d.file || ''}`,
      `path: ${d.path || ''}`,
      `stage: ${d.stage || ''}`,
      `original_preserved: ${Boolean(d.original_preserved)}`,
      `backup_path: ${d.backup_path || ''}`,
      `detail: ${d.detail || ''}`,
      `user_agent: ${navigator.userAgent}`,
    ].join('\n');
  }

  async function copyDiagnostics() {
    try {
      await navigator.clipboard.writeText(diagnostics($storageError));
      addToast($t('storageError.copied'), 'success');
    } catch (_) {
      addToast($t('storageError.copyFailed'), 'error');
    }
  }
</script>

{#if $storageError}
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div class="fixed inset-0 z-[120] bg-black/60 flex items-center justify-center" on:click={close}>
    <div class="bg-base-200 rounded-xl p-6 w-full max-w-2xl mx-4 border border-error/40" on:click|stopPropagation>
      <h2 class="text-lg font-semibold text-error mb-2">{$t('storageError.title')}</h2>
      <div class="alert {$storageError.original_preserved ? 'alert-warning' : 'alert-error'} mb-4">
        <span>
          {$storageError.original_preserved
            ? $t('storageError.preserved')
            : $t('storageError.atRisk')}
        </span>
      </div>

      <p class="text-sm mb-3">{$t('storageError.intro', { file: $storageError.file || '?' })}</p>
      <ol class="list-decimal pl-5 text-sm space-y-1 mb-4">
        <li>{$t('storageError.check.close')}</li>
        <li>{$t('storageError.check.sync')}</li>
        <li>{$t('storageError.check.permission')}</li>
        <li>{$t('storageError.check.local')}</li>
      </ol>

      {#if $storageError.backup_path}
        <div class="text-xs mb-3">
          <span class="font-semibold">{$t('storageError.backup')}</span>
          <code class="block mt-1 p-2 rounded bg-base-300 break-all select-all">{$storageError.backup_path}</code>
        </div>
      {/if}

      <details class="mb-5">
        <summary class="cursor-pointer text-sm">{$t('storageError.details')}</summary>
        <pre class="mt-2 p-3 rounded bg-base-300 text-xs whitespace-pre-wrap break-all max-h-40 overflow-auto">{diagnostics($storageError)}</pre>
      </details>

      <div class="flex justify-end gap-2">
        <button class="btn btn-outline btn-sm" on:click={copyDiagnostics}>{$t('storageError.copy')}</button>
        <button class="btn btn-primary btn-sm" on:click={close}>{$t('common.close')}</button>
      </div>
    </div>
  </div>
{/if}
