<script lang="ts">
  import { dayBuckets, weekDate } from '../lib/format';
  import type { SnapInfo } from './types';

  let { snapshots, days = 30 }: { snapshots: SnapInfo[]; days?: number } = $props();

  const RANK = ['manual', 'frequent', 'hourly', 'daily', 'weekly', 'monthly', 'yearly'];

  let buckets = $derived(
    dayBuckets(snapshots, days).map((b) => {
      const types: Record<string, number> = {};
      let top = '';
      for (const s of b.items) {
        const k = s.type || 'manual';
        types[k] = (types[k] || 0) + 1;
        if (RANK.indexOf(k) > RANK.indexOf(top)) top = k;
      }
      return { day: b.day, n: b.items.length, top, types };
    })
  );
  let max = $derived(Math.max(1, ...buckets.map((b) => b.n)));
  let older = $derived(snapshots.filter((s) => s.creation < buckets[0].day).length);

  function title(b: { day: number; n: number; types: Record<string, number> }): string {
    const d = weekDate(b.day);
    if (!b.n) return d + ': no snapshot';
    const parts = Object.entries(b.types)
      .sort((a, c) => RANK.indexOf(c[0]) - RANK.indexOf(a[0]))
      .map(([k, v]) => v + ' ' + k);
    return d + ': ' + parts.join(', ');
  }
</script>

<div class="ss-heat" role="img" aria-label="Snapshots per day over the last {days} days{older ? `, ${older} older` : ''}">
  <div class="ss-heat-bars">
    {#each buckets as b (b.day)}
      <span class="t-{b.top || 'none'}" style:height={b.n ? (5 + 17 * Math.sqrt(b.n / max)).toFixed(1) + 'px' : '4px'} title={title(b)}></span>
    {/each}
  </div>
  <div class="ss-heat-axis">
    <span>{days} days ago{older ? ` · ${older} older` : ''}</span>
    <span>today</span>
  </div>
</div>

<style>
  :global(#zt-app) .ss-heat {
    margin-top: 14px;
  }
  :global(#zt-app) .ss-heat-bars {
    display: flex;
    align-items: flex-end;
    gap: 2px;
    height: 22px;
  }
  :global(#zt-app) .ss-heat-bars span {
    flex: 1 1 0;
    min-width: 2px;
    border-radius: 2px;
    background: var(--zt-accent);
    opacity: 0.75;
  }
  :global(#zt-app) .ss-heat-bars span.t-none {
    background: var(--zt-surface-3);
    opacity: 1;
  }
  :global(#zt-app) .ss-heat-bars span.t-frequent,
  :global(#zt-app) .ss-heat-bars span.t-hourly {
    opacity: 0.5;
  }
  :global(#zt-app) .ss-heat-bars span.t-weekly,
  :global(#zt-app) .ss-heat-bars span.t-monthly,
  :global(#zt-app) .ss-heat-bars span.t-yearly {
    opacity: 1;
  }
  :global(#zt-app) .ss-heat-bars span.t-manual {
    background: var(--zt-info);
  }
  :global(#zt-app) .ss-heat-axis {
    display: flex;
    justify-content: space-between;
    margin-top: 4px;
    font-size: 11px;
    color: var(--zt-faint);
  }
</style>
