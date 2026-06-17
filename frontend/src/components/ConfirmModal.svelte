<script>
  import { confirmModal } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  function confirm() {
    if ($confirmModal?.onConfirm) $confirmModal.onConfirm();
    confirmModal.set(null);
  }

  function cancel() {
    confirmModal.set(null);
  }
</script>

{#if $confirmModal}
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div class="fixed inset-0 z-[110] bg-black/50 flex items-center justify-center" on:click={cancel}>
    <div class="bg-base-200 rounded-xl shadow-2xl p-6 max-w-sm mx-4 border border-base-content/10" on:click|stopPropagation>
      <p class="text-base mb-6">{$confirmModal.message}</p>
      <div class="flex justify-end gap-2">
        <button class="btn btn-ghost btn-sm" on:click={cancel}>{$t('common.cancel')}</button>
        <button class="btn btn-error btn-sm" on:click={confirm}>{$t('common.confirm')}</button>
      </div>
    </div>
  </div>
{/if}
