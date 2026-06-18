<script>
  import { taskTokenUsage } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  export let className = 'badge badge-xs badge-info gap-1 font-mono';
</script>

{#if $taskTokenUsage}
  <span class={className}>
    {$t('task.tokens.usage', {
      prompt: $taskTokenUsage.prompt_tokens.toLocaleString(),
      completion: $taskTokenUsage.completion_tokens.toLocaleString(),
    })}
  </span>
{/if}
