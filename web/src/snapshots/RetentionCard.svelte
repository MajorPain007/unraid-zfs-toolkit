<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { openDialog } from '../lib/state.svelte';
  import { part, settings } from './store.svelte';
  import { TYPES, TYPE_LABEL, globalKeep, reach, reachText, scheduleExpr } from './types';
  import SettingsDrawer from './SettingsDrawer.svelte';

  let s = $derived(settings());
  let p = $derived(part());
  let expr = $derived(scheduleExpr(s));
  let rows = $derived(
    TYPES.map((t) => ({ t, n: globalKeep(s, t), age: Number(s['snap_age_' + t]) || 0, back: reach(t, globalKeep(s, t), expr), have: p?.by_type?.[t] ?? 0 }))
  );
  let maxBack = $derived(Math.max(1, ...rows.map((r) => r.back)));
  let target = $derived((s.snap_free_target || '').trim());
</script>

<div class="zt-card ss-ret-card">
  <div class="zt-card-head">
    <div class="zt-tile"><Icon name="clock" /></div>
    <div class="zt-card-title">
      <h3>Global retention</h3>
      <div class="zt-meta">Used by every dataset set to "global"</div>
    </div>
    <div class="zt-card-actions">
      <Button icon="edit" small onclick={() => openDialog(SettingsDrawer, {})}>Edit</Button>
    </div>
  </div>
  <div class="ss-ladder">
    {#each rows as r (r.t)}
      <div class="ss-rung" class:off={r.n <= 0}>
        <span class="ss-rung-label">{TYPE_LABEL[r.t]}</span>
        <span class="ss-rung-n">{r.n > 0 ? r.n : 'off'}</span>
        <span class="ss-rung-track" title={r.n > 0 ? `${r.n} ${r.t} snapshots reach back about ${reachText(r.back)}` : 'Not taken'}>
          <span class="ss-rung-bar" style:width="{r.n > 0 ? Math.max(3, (100 * Math.sqrt(r.back / maxBack))).toFixed(1) : 0}%"></span>
        </span>
        <span class="ss-rung-back">{r.n > 0 ? reachText(r.back) : ''}{r.age ? ` · max ${r.age} d` : ''}</span>
      </div>
    {/each}
  </div>
  <div class="zt-card-foot ss-ret-foot">
    <span class="zt-chip"><Icon name="database" />{target ? 'Keeps ' + target + ' free' : 'No free-space target'}</span>
    <span class="zt-chip"><Icon name="bell" />{Number(s.snap_min_free_pct || '5') > 0 ? 'Warns below ' + (s.snap_min_free_pct || '5') + '% free' : 'No space warning'}</span>
  </div>
</div>

<style>
  :global(#zt-app) .ss-ladder {
    display: flex;
    flex-direction: column;
    gap: 7px;
    margin-top: 16px;
  }
  :global(#zt-app) .ss-rung {
    display: grid;
    grid-template-columns: 74px 34px minmax(0, 1fr) 120px;
    align-items: center;
    gap: 10px;
    font-size: 13px;
  }
  :global(#zt-app) .ss-rung.off {
    color: var(--zt-faint);
  }
  :global(#zt-app) .ss-rung-n {
    text-align: right;
    font-variant-numeric: tabular-nums;
    font-weight: 600;
  }
  :global(#zt-app) .ss-rung.off .ss-rung-n {
    font-weight: 400;
  }
  :global(#zt-app) .ss-rung-track {
    height: 6px;
    border-radius: 3px;
    background: var(--zt-surface-3);
    overflow: hidden;
  }
  :global(#zt-app) .ss-rung-bar {
    display: block;
    height: 100%;
    border-radius: 3px;
    background: var(--zt-accent);
  }
  :global(#zt-app) .ss-rung-back {
    color: var(--zt-dim);
    font-size: 12px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  :global(#zt-app) .ss-ret-foot {
    flex-wrap: wrap;
    gap: 6px;
  }
  @media (max-width: 560px) {
    :global(#zt-app) .ss-rung {
      grid-template-columns: 62px 28px minmax(0, 1fr) 72px;
      gap: 8px;
    }
  }
</style>
