<script>
  import { tick } from 'svelte';
  import { confirmModal } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  let modalBox;
  let cancelButton;
  let previouslyFocused = null;
  let wasOpen = false;

  $: if ($confirmModal && !wasOpen) {
    wasOpen = true;
    previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    tick().then(() => {
      if ($confirmModal) cancelButton?.focus();
    });
  }

  $: if (!$confirmModal && wasOpen) {
    wasOpen = false;
    const target = previouslyFocused;
    previouslyFocused = null;
    tick().then(() => {
      if (target?.isConnected) target.focus();
    });
  }

  function confirm() {
    if ($confirmModal?.onConfirm) $confirmModal.onConfirm();
    confirmModal.set(null);
  }

  function cancel() {
    confirmModal.set(null);
  }

  function handleKeydown(event) {
    if (event.key === 'Escape') {
      event.preventDefault();
      cancel();
      return;
    }
    if (event.key !== 'Tab' || !modalBox) return;
    const focusable = [...modalBox.querySelectorAll(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
    )].filter(element => element.getClientRects().length > 0);
    if (focusable.length === 0) {
      event.preventDefault();
      modalBox.focus();
      return;
    }
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }
</script>

{#if $confirmModal}
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div class="confirm-modal-overlay fixed inset-0 z-[110] bg-black/50 flex items-center justify-center" on:click={cancel} on:keydown={handleKeydown}>
    <!-- svelte-ignore a11y-click-events-have-key-events -->
    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
    <div bind:this={modalBox} class="confirm-modal-box bg-base-200 rounded-xl shadow-2xl p-6 max-w-sm mx-4 border border-base-content/10" role="alertdialog" aria-modal="true" aria-label={$t('common.confirm')} aria-describedby="confirm-modal-message" tabindex="-1" on:click|stopPropagation>
      <p id="confirm-modal-message" class="text-base mb-6 break-words">{$confirmModal.message}</p>
      <div class="confirm-modal-actions flex justify-end gap-2">
        <button bind:this={cancelButton} class="btn btn-ghost btn-sm" on:click={cancel}>{$t('common.cancel')}</button>
        <button class="btn btn-error btn-sm" on:click={confirm}>{$t('common.confirm')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  @media (max-width: 63.999rem) {
    .confirm-modal-overlay { padding: max(1rem, env(safe-area-inset-top)) max(1rem, env(safe-area-inset-right)) max(1rem, env(safe-area-inset-bottom)) max(1rem, env(safe-area-inset-left)); }
    .confirm-modal-box { width: 100%; max-height: calc(100vh - 2rem); max-height: calc(100dvh - 2rem); margin: 0; overflow-y: auto; overscroll-behavior: contain; }
    .confirm-modal-actions { width: 100%; }
    .confirm-modal-actions .btn { flex: 1; min-height: 44px; }
  }
</style>
