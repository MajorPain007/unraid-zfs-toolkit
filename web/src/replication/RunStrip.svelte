<script lang="ts">
  import { dateTime } from '../lib/format';
  import { resultText, type SendResult } from './send.svelte';

  let { runs, slots = 20 }: { runs: SendResult[]; slots?: number } = $props();

  let shown = $derived(runs.slice(-slots));
  let max = $derived(Math.max(1, ...shown.map((r) => r.bytes || 0)));
  let ok = $derived(shown.filter((r) => r.status === 'ok').length);

  function title(r: SendResult): string {
    return dateTime(r.started) + ': ' + resultText(r) + (r.error ? ' - ' + r.error : '');
  }
</script>

<span class="zr-mini" role="img" aria-label={shown.length ? `${ok} of the last ${shown.length} runs succeeded` : 'No runs yet'}>
  {#each { length: slots - shown.length } as _, i (i)}
    <span class="zr-mini-col"></span>
  {/each}
  {#each shown as r (r.run)}
    <span class="zr-mini-col" title={title(r)}>
      <span class="zr-mini-bar st-{r.status}" style:height="{(30 + 70 * Math.sqrt((r.bytes || 0) / max)).toFixed(0)}%"></span>
    </span>
  {/each}
</span>

<style>
  :global(#zt-app) .zr-mini {
    display: flex;
    align-items: flex-end;
    gap: 2px;
    width: 100%;
    height: 20px;
    padding: 2px 3px;
    border-radius: 4px;
    background: var(--zt-surface-2);
  }
  :global(#zt-app) .zr-mini-col {
    flex: 1 1 0;
    height: 100%;
    display: flex;
    align-items: flex-end;
  }
  :global(#zt-app) .zr-mini-bar {
    width: 100%;
    min-height: 3px;
    border-radius: 1px;
    background: var(--zt-ok);
  }
  :global(#zt-app) .zr-mini-bar.st-error {
    background: var(--zt-err);
  }
  :global(#zt-app) .zr-mini-bar.st-cancelled {
    background: var(--zt-faint);
  }
  :global(#zt-app) .zr-mini-bar.st-running {
    background: var(--zt-info);
  }
</style>
