<script lang="ts">
  import { dayBuckets, weekDate } from '../lib/format';
  import type { SnapInfo } from './types';

  let { snapshots, days = 30 }: { snapshots: SnapInfo[]; days?: number } = $props();

  let buckets = $derived(dayBuckets(snapshots, days).map((b) => ({ day: b.day, n: b.items.length, manual: b.items.filter((s) => !s.managed).length })));
  let max = $derived(Math.max(1, ...buckets.map((b) => b.n)));
  let total = $derived(buckets.reduce((a, b) => a + b.n, 0));

  function title(b: { day: number; n: number; manual: number }): string {
    const d = weekDate(b.day);
    if (!b.n) return d + ': none';
    return d + ': ' + b.n + (b.n === 1 ? ' snapshot' : ' snapshots') + (b.manual ? ` (${b.manual} manual)` : '');
  }
</script>

<span class="ss-mini" role="img" aria-label="{total} snapshots taken in the last {days} days">
  {#each buckets as b (b.day)}
    <span class="ss-mini-col" title={title(b)}>
      {#if b.n}<span class="ss-mini-bar" class:manual={b.manual === b.n} style:height="{(25 + 75 * Math.sqrt(b.n / max)).toFixed(0)}%"></span>{/if}
    </span>
  {/each}
</span>

<style>
  :global(#zt-app) .ss-mini {
    display: flex;
    align-items: flex-end;
    gap: 1px;
    width: 100%;
    height: 20px;
    padding: 2px 3px;
    border-radius: 4px;
    background: var(--zt-surface-2);
  }
  :global(#zt-app) .ss-mini-col {
    flex: 1 1 0;
    height: 100%;
    display: flex;
    align-items: flex-end;
  }
  :global(#zt-app) .ss-mini-bar {
    width: 100%;
    min-height: 3px;
    border-radius: 1px;
    background: var(--zt-accent);
  }
  :global(#zt-app) .ss-mini-bar.manual {
    background: var(--zt-info);
  }
</style>
