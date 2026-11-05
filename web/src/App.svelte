<script lang="ts">
  import Icon from './lib/ui/Icon.svelte';
  import Button from './lib/ui/Button.svelte';
  import Hosts from './lib/ui/Hosts.svelte';
  import SnapshotsView from './views/SnapshotsView.svelte';
  import BrowseView from './views/BrowseView.svelte';
  import ReplicationView from './views/ReplicationView.svelte';
  import ConversionView from './views/ConversionView.svelte';
  import SettingsView from './views/SettingsView.svelte';
  import { app, refresh, show, type Tab } from './lib/state.svelte';
  import { bytes } from './lib/format';
  import type { Overview, PartStatus } from './lib/types';

  const TABS: [Tab, string, string][] = [
    ['snapshots', 'Snapshots', 'camera'],
    ['browse', 'Browse & Restore', 'restore'],
    ['replication', 'Replication', 'send'],
    ['conversion', 'Conversion', 'convert'],
    ['settings', 'Settings', 'settings']
  ];

  const PARTS: Record<string, Tab> = { snapshots: 'snapshots', browse: 'browse', replication: 'replication', conversion: 'conversion' };
  const RANK: Record<string, number> = { err: 4, run: 3, warn: 2, ok: 1, idle: 0 };

  refresh();

  $effect(() => {
    const onVisible = () => {
      if (!document.hidden) refresh();
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => document.removeEventListener('visibilitychange', onVisible);
  });

  function partStatus(o: Overview, part: string): PartStatus | null {
    const v = o[part];
    return v && typeof v === 'object' && v.status ? (v.status as PartStatus) : null;
  }

  function overall(o: Overview): PartStatus {
    if (!o.zfs) return { level: 'err', text: 'ZFS is not available', sub: 'No pool is imported, or the zfs tools are missing.' };
    let best: PartStatus = { level: 'idle', text: 'Nothing set up yet', sub: 'Start with automatic snapshots.' };
    let any = false;
    for (const p of Object.keys(PARTS)) {
      const st = partStatus(o, p);
      if (!st) continue;
      if (!any || RANK[st.level] > RANK[best.level]) best = st;
      any = true;
    }
    return best;
  }

  let o = $derived(app.overview);
  let status = $derived(o ? overall(o) : ({ level: 'idle', text: '', sub: '' } as PartStatus));
  let counts = $derived.by<Record<string, [number, string]>>(() => {
    const out: Record<string, [number, string]> = {};
    if (!o) return out;
    for (const [part, tab] of Object.entries(PARTS)) {
      const v = o[part];
      if (v && typeof v === 'object' && typeof v.count === 'number' && v.count > 0) {
        out[tab] = [v.count, partStatus(o, part)?.level === 'err' ? 'err' : ''];
      }
    }
    return out;
  });
</script>

{#if !o}
  {#if app.loadError}
    <div class="zt-empty">
      <Icon name="alert" class="big" />
      <h3>The plugin could not load its data</h3>
      <p>{app.loadError}</p>
      <Button icon="refresh" onclick={() => refresh()}>Try again</Button>
    </div>
  {:else}
    <div class="zt-boot">Loading&hellip;</div>
  {/if}
{:else}
  <div class="zt-header">
    <div class="zt-header-main">
      <div class="zt-status-line">
        <span class="zt-dot {status.level}" style="margin-top:0"></span>
        <span>{status.text}</span>
        {#if status.sub}<span class="zt-sub">{status.sub}</span>{/if}
      </div>
      <div class="zt-sub" style="margin-top:3px">
        ZFS Toolkit{o.version ? ' ' + o.version : ''} &nbsp;·&nbsp; {o.server}
      </div>
    </div>
    {#if o.pools.length}
      <div class="zt-pools">
        {#each o.pools as p (p.name)}
          <div class="zt-pool" class:warn={p.capacity >= 80} class:err={p.capacity >= 90 || p.health !== 'ONLINE'} title="{p.name}: {bytes(p.alloc)} used of {bytes(p.size)}, {bytes(p.free)} free · {p.health}">
            <div class="zt-pool-head"><Icon name="database" /><b>{p.name}</b><span>{p.capacity}%</span></div>
            <div class="zt-pool-track"><div class="zt-pool-bar" style:width="{Math.min(100, p.capacity)}%"></div></div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  {#if !o.zfs}
    <div class="zt-card-note err zt-banner"><Icon name="alert" /><div>No ZFS pool is available. Snapshots, browsing and replication need at least one imported pool.</div></div>
  {/if}
  {#if !o.array}
    <div class="zt-card-note warn zt-banner"><Icon name="power" /><div>The array is stopped. Pools on it are not mounted, so scheduled snapshots and replication skip them until it is started.</div></div>
  {/if}

  <div class="zt-tabs" role="tablist">
    {#each TABS as [id, label, icon] (id)}
      <button type="button" role="tab" class="zt-tab" class:active={app.tab === id} aria-selected={app.tab === id} onclick={() => show(id)}>
        <Icon name={icon} /><span class="zt-tab-label">{label}</span>
        {#if counts[id]}<span class="zt-count" class:err={counts[id][1] === 'err'}>{counts[id][0]}</span>{/if}
      </button>
    {/each}
  </div>

  <div class="zt-view">
    {#key app.viewKey}
      {#if app.tab === 'snapshots'}
        <SnapshotsView opts={app.viewOpts} />
      {:else if app.tab === 'browse'}
        <BrowseView opts={app.viewOpts} />
      {:else if app.tab === 'replication'}
        <ReplicationView opts={app.viewOpts} />
      {:else if app.tab === 'conversion'}
        <ConversionView opts={app.viewOpts} />
      {:else}
        <SettingsView opts={app.viewOpts} />
      {/if}
    {/key}
  </div>
{/if}

<Hosts />

<style>
  .zt-banner {
    margin: 0 0 14px;
  }
  .zt-pools {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
  .zt-pool {
    min-width: 120px;
    padding: 7px 10px;
    border: 1px solid var(--zt-border);
    border-radius: var(--zt-radius-sm);
    background: var(--zt-surface);
    font-size: 12px;
  }
  .zt-pool-head {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 5px;
  }
  .zt-pool-head span {
    margin-left: auto;
    color: var(--zt-dim);
  }
  .zt-pool-track {
    height: 4px;
    border-radius: 2px;
    background: var(--zt-surface-3);
    overflow: hidden;
  }
  .zt-pool-bar {
    height: 100%;
    background: var(--zt-ok);
  }
  .zt-pool.warn .zt-pool-bar {
    background: var(--zt-warn);
  }
  .zt-pool.err .zt-pool-bar {
    background: var(--zt-err);
  }
</style>
