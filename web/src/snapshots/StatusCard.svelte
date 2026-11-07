<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import { app, openDialog, openMenu } from '../lib/state.svelte';
  import { bytes, count, dateTime, plural, relative } from '../lib/format';
  import { part, runNow, setEnabled, settings } from './store.svelte';
  import { TYPES, TYPE_LABEL, scheduleText } from './types';
  import LogDialog from './LogDialog.svelte';
  import PreviewDialog from './PreviewDialog.svelte';
  import SettingsDrawer from './SettingsDrawer.svelte';

  let p = $derived(part());
  let s = $derived(settings());
  let cron = $derived(app.overview?.cron.snapshots);
  let enabled = $derived(p?.enabled ?? s.snapshots_enabled === 'yes');
  let toggling = $state(false);
  let starting = $state(false);
  let last = $derived(p?.last);
  let lastTs = $derived(last?.ended || (last?.last_run ? Date.parse(last.last_run.replace(' ', 'T')) / 1000 : 0));
  let nextList = $derived(
    TYPES.filter((t) => p?.next?.[t]).map((t) => [t, p!.next[t]] as [string, number]).sort((a, b) => a[1] - b[1])
  );
  let soonest = $derived(nextList[0]);

  async function toggle(on: boolean) {
    toggling = true;
    await setEnabled(on);
    toggling = false;
  }

  async function run() {
    starting = true;
    await runNow();
    starting = false;
  }

  function menu(e: MouseEvent) {
    openMenu(e.currentTarget as Element, [
      { label: 'Preview the next run', icon: 'eye', onclick: () => openDialog(PreviewDialog, { now: true }) },
      { label: 'Take all types now', icon: 'camera', disabled: !!p?.running, onclick: () => runNow(true) },
      { label: 'Show log', icon: 'list', onclick: () => openDialog(LogDialog, {}) },
      '-',
      { label: 'Schedule & retention', icon: 'settings', onclick: () => openDialog(SettingsDrawer, {}) }
    ]);
  }

  function nextText(ts: number): string {
    return ts <= Date.now() / 1000 + 30 ? 'at the next run' : relative(ts);
  }
</script>

<div class="zt-card ss-hero" class:is-running={p?.running} class:is-failed={p?.status.level === 'err' && enabled}>
  <div class="zt-card-head">
    <div class="zt-tile ss-tile" class:on={enabled}><Icon name="camera" /></div>
    <div class="zt-card-title">
      <h3>Automatic snapshots</h3>
      <div class="zt-meta">
        {#if enabled}
          <span>{scheduleText(s)}</span>
          <span class="zt-sep">·</span>
          <span>daily and longer from {String(s.snap_daily_hour || '2').padStart(2, '0')}:00</span>
          {#if cron && !cron.live}
            <span class="zt-sep">·</span><span class="zt-err-text">cron entry missing</span>
          {/if}
        {:else}
          <span>Off - existing snapshots stay, nothing new is taken or pruned</span>
        {/if}
      </div>
    </div>
    <div class="zt-card-actions ss-actions">
      <Toggle checked={enabled} label={enabled ? 'On' : 'Off'} disabled={toggling} onchange={toggle} />
      {#if enabled}
        <Button icon="play" small kind="primary" busy={starting || !!p?.running} disabled={!app.overview?.zfs} onclick={run}>
          {p?.running ? 'Running' : 'Run now'}
        </Button>
      {/if}
      <Button icon="more" small kind="ghost" title="More" onclick={menu} />
    </div>
  </div>

  {#if enabled}
    <div class="zt-facts ss-facts">
      <div class="zt-fact">
        <div class="zt-label">Last run</div>
        <div class="zt-value">
          {#if p?.running}
            <span class="zt-pill run"><Icon name="loader" class="zt-spin" />running</span>
          {:else if lastTs}
            <span title={dateTime(lastTs)}>{relative(lastTs)}</span>
            {#if last && last.created !== undefined}
              <small> · {last.created} new, {last.pruned} pruned{last.errors ? ', ' + plural(last.errors, 'error') : ''}</small>
            {/if}
          {:else}
            Not yet
          {/if}
        </div>
      </div>
      <div class="zt-fact">
        <div class="zt-label">Next snapshot</div>
        <div class="zt-value">
          {#if soonest}
            <span title={dateTime(soonest[1])}>{TYPE_LABEL[soonest[0] as keyof typeof TYPE_LABEL]} {nextText(soonest[1])}</span>
          {:else}
            -
          {/if}
        </div>
      </div>
      <div class="zt-fact">
        <div class="zt-label">Snapshots</div>
        <div class="zt-value">
          {count(p?.auto ?? 0)} automatic
          {#if p && p.total > p.auto}<small> · {count(p.total - p.auto)} other</small>{/if}
          {#if p?.held}<small> · {p.held} held</small>{/if}
        </div>
      </div>
      <div class="zt-fact">
        <div class="zt-label">Space in snapshots</div>
        <div class="zt-value">{bytes(p?.used ?? 0)} <small>· {plural(p?.datasets ?? 0, 'dataset')}</small></div>
      </div>
    </div>

    {#if nextList.length > 1}
      <div class="zt-next-runs" aria-label="Upcoming snapshots">
        {#each nextList as [t, ts] (t)}
          <span class="zt-chip" title={dateTime(ts)}><Icon name="clock" />{t} {nextText(ts)}</span>
        {/each}
      </div>
    {/if}

    {#if p?.missing?.length}
      <div class="zt-card-note err">
        <Icon name="alert" />
        <div>Not found: <b>{p.missing.join(', ')}</b>. These datasets were renamed or destroyed - remove them from the list below or create them again.</div>
      </div>
    {/if}
    {#if last?.errors && last.last_error}
      <div class="zt-card-note err">
        <Icon name="alert" />
        <div>
          {last.last_error}
          <button type="button" class="zt-link ss-link" onclick={() => openDialog(LogDialog, {})}>Show log</button>
        </div>
      </div>
    {/if}
    {#if cron && !cron.live}
      <div class="zt-card-note err">
        <Icon name="alert" />
        <div>The schedule is not in the crontab, so nothing runs automatically. Open <b>Schedule & retention</b> and save once to install it again.</div>
      </div>
    {/if}
    {#each p?.low ?? [] as l (l.pool)}
      <div class="zt-card-note warn">
        <Icon name="database" />
        <div>
          {#if l.reason === 'full'}
            Pool <b>{l.pool}</b> is {l.capacity}% full - above the warning threshold of {100 - Number(s.snap_min_free_pct || 5)}%.
          {:else}
            Pool <b>{l.pool}</b> has {bytes(l.free)} free, below the target of {bytes(l.target)}. Each run prunes the oldest automatic snapshots
            there until the target is met.
          {/if}
        </div>
      </div>
    {/each}
  {/if}
</div>

<style>
  :global(#zt-app) .ss-hero {
    margin-bottom: 8px;
  }
  :global(#zt-app) .ss-tile.on {
    color: var(--zt-accent-text);
    background: var(--zt-accent-soft);
    border-color: transparent;
  }
  :global(#zt-app) .ss-actions {
    align-items: center;
    gap: 10px;
  }
  :global(#zt-app) .ss-facts {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
  :global(#zt-app) .ss-link {
    margin-left: 6px;
    font-weight: 600;
  }
  @media (max-width: 760px) {
    :global(#zt-app) .ss-facts {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  @media (max-width: 560px) {
    :global(#zt-app) .ss-hero .zt-card-head {
      flex-wrap: wrap;
    }
    :global(#zt-app) .ss-actions {
      width: 100%;
      justify-content: flex-end;
    }
  }
</style>
