<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Segmented from '../lib/ui/Segmented.svelte';
  import { api } from '../lib/api';
  import { actions } from '../lib/actions';
  import { copyText, fail, openDialog, openMenu, show, toast } from '../lib/state.svelte';
  import { bytes, count, dateTime, plural, relative } from '../lib/format';
  import { reloadAll, snap } from './store.svelte';
  import { globMatcher, kindOf, type SnapInfo } from './types';
  import DestroyDialog from './DestroyDialog.svelte';
  import RollbackDialog from './RollbackDialog.svelte';
  import CreateDialog from './CreateDialog.svelte';

  let { dataset = '' }: { dataset?: string } = $props();

  type Kind = 'all' | 'auto' | 'manual' | 'held' | 'replication';
  type SortKey = 'creation' | 'used' | 'name';

  let ds = $state('');
  let q = $state('');
  let kind = $state<Kind>('all');
  let sortKey = $state<SortKey>('creation');
  let sortDesc = $state(true);
  let limit = $state(200);
  let working = $state(false);
  const selected = new SvelteSet<string>();

  $effect.pre(() => {
    if (dataset) ds = dataset;
  });

  $effect(() => {
    if (snap.filterSeq) {
      ds = snap.filterDataset;
      q = '';
      kind = 'all';
    }
  });

  $effect(() => {
    const alive = new Set(snap.list.map((s) => s.name));
    for (const n of [...selected]) if (!alive.has(n)) selected.delete(n);
  });

  let datasetNames = $derived([...new Set(snap.list.map((s) => s.dataset))].sort());
  let match = $derived(globMatcher(q));

  let filtered = $derived.by(() => {
    const out = snap.list.filter((s) => {
      if (ds && s.dataset !== ds) return false;
      if (kind === 'auto' && !s.managed) return false;
      if (kind === 'manual' && (s.managed || s.checkpoint)) return false;
      if (kind === 'held' && !s.held) return false;
      if (kind === 'replication' && !s.checkpoint) return false;
      return !match || match(s.name);
    });
    const dir = sortDesc ? -1 : 1;
    out.sort((a, b) => {
      if (sortKey === 'name') return a.name < b.name ? -dir : a.name > b.name ? dir : 0;
      const d = sortKey === 'used' ? a.used - b.used : a.creation - b.creation;
      return d ? d * dir : (b.creation - a.creation) * 1;
    });
    return out;
  });
  let shown = $derived(filtered.slice(0, limit));
  let filteredBytes = $derived(filtered.reduce((a, s) => a + s.used, 0));
  let visible = $derived(new Set(filtered.map((s) => s.name)));
  let hiddenSelected = $derived([...selected].filter((n) => !visible.has(n)).length);
  let allShownSelected = $derived(filtered.length > 0 && filtered.every((s) => selected.has(s.name)));
  let someShownSelected = $derived(filtered.some((s) => selected.has(s.name)));
  let selectedItems = $derived(snap.list.filter((s) => selected.has(s.name)));
  let heavy = $derived(
    [...snap.datasets]
      .filter((d) => d.usedbysnapshots > 0 && (!ds || d.name === ds))
      .sort((a, b) => b.usedbysnapshots - a.usedbysnapshots)
      .slice(0, 4)
  );

  function sortBy(k: SortKey) {
    if (sortKey === k) sortDesc = !sortDesc;
    else {
      sortKey = k;
      sortDesc = k !== 'name';
    }
  }

  function toggleAll() {
    if (allShownSelected) filtered.forEach((s) => selected.delete(s.name));
    else filtered.forEach((s) => selected.add(s.name));
  }

  function toggle(name: string) {
    if (selected.has(name)) selected.delete(name);
    else selected.add(name);
  }

  async function setHold(items: SnapInfo[], hold: boolean) {
    if (!items.length) return;
    working = true;
    try {
      const r = await api<{ changed: number; failed: number; results: { name: string; ok: boolean; error?: string }[] }>(hold ? 'snap_hold' : 'snap_release', {
        snapshots: items.map((s) => s.name)
      });
      const bad = r.results.filter((x) => !x.ok);
      if (bad.length) toast(`${bad.length} failed: ${bad[0].error}`, 'err');
      else toast(hold ? plural(r.changed, 'snapshot') + ' held - never pruned automatically' : 'Hold released on ' + plural(r.changed, 'snapshot'));
      await reloadAll();
    } catch (e) {
      fail(e);
    } finally {
      working = false;
    }
  }

  async function destroy(items: SnapInfo[]) {
    if (!items.length) return;
    const r = await openDialog<{ results: { name: string; ok: boolean }[] } | undefined>(DestroyDialog, { snapshots: items });
    if (r) {
      r.results.filter((x) => x.ok).forEach((x) => selected.delete(x.name));
      reloadAll();
    }
  }

  async function rollback(s: SnapInfo) {
    if (await openDialog(RollbackDialog, { snapshot: s })) reloadAll();
  }

  function browse(s: SnapInfo) {
    if (actions.browseSnapshot) actions.browseSnapshot({ snapshot: s.snapshot, dataset: s.dataset });
    else show('browse', { snapshot: s.snapshot, dataset: s.dataset });
  }

  function rowMenu(e: MouseEvent, s: SnapInfo) {
    openMenu(e.currentTarget as Element, [
      { label: 'Browse files', icon: 'folder', onclick: () => browse(s) },
      { label: s.held ? 'Release the hold' : 'Hold - never prune', icon: s.held ? 'unlock' : 'lock', onclick: () => setHold([s], !s.held) },
      { label: 'Copy name', icon: 'copy', onclick: () => copyText(s.name) },
      { label: 'Show only ' + s.dataset, icon: 'database', disabled: ds === s.dataset, onclick: () => (ds = s.dataset) },
      '-',
      { label: 'Roll back to here…', icon: 'restore', danger: true, onclick: () => rollback(s) },
      { label: 'Delete', icon: 'trash', danger: true, onclick: () => destroy([s]) }
    ]);
  }

  const KINDS: [Kind, string][] = [
    ['all', 'All'],
    ['auto', 'Auto'],
    ['manual', 'Manual'],
    ['held', 'Held'],
    ['replication', 'Replication']
  ];

  function kindClass(k: string): string {
    if (k === 'replication') return 'warn';
    if (k === 'manual') return 'run';
    if (k === 'monthly' || k === 'yearly' || k === 'weekly') return 'accent';
    return '';
  }
</script>

<div class="zt-toolbar ss-mgr-tools">
  <select bind:value={ds} aria-label="Dataset">
    <option value="">All datasets</option>
    {#each datasetNames as n (n)}<option value={n}>{n}</option>{/each}
    {#if ds && !datasetNames.includes(ds)}<option value={ds}>{ds}</option>{/if}
  </select>
  <input type="search" placeholder="Filter, e.g. *daily* or plex" bind:value={q} aria-label="Filter snapshots" />
  <Segmented bind:value={kind} options={KINDS} />
  <div class="zt-spacer"></div>
  <Button icon="camera" small onclick={() => openDialog(CreateDialog, { dataset: ds })}>Snapshot</Button>
  <Button icon="refresh" small kind="ghost" title="Reload" busy={snap.loading} onclick={() => reloadAll()} />
</div>

<div class="zt-table-wrap">
  <div class="zt-snap-info">
    <span>
      {#if filtered.length !== snap.list.length}
        <b>{count(filtered.length)}</b> of {count(snap.list.length)} snapshots shown, {bytes(filteredBytes)}
      {:else}
        <b>{count(snap.list.length)}</b> snapshots, {bytes(filteredBytes)} unique
      {/if}
    </span>
    {#each heavy as d (d.name)}
      <span class="zt-hide-narrow" title="Space held only by snapshots of {d.name}"><Icon name="database" /> {d.name}: <b>{bytes(d.usedbysnapshots)}</b></span>
    {/each}
  </div>

  {#if snap.listError}
    <div class="zt-card-note err ss-mgr-pad"><Icon name="alert" /><div>{snap.listError}</div></div>
  {:else if !snap.listLoaded}
    <div class="zt-hint ss-mgr-pad"><Icon name="loader" class="zt-spin" /> Loading snapshots&hellip;</div>
  {:else if !filtered.length}
    <div class="zt-hint ss-mgr-pad">{snap.list.length ? 'No snapshot matches this filter.' : 'No snapshots yet.'}</div>
  {:else}
    <div class="ss-mgr-scroll">
      <table class="zt-table ss-mgr">
        <thead>
          <tr>
            <th class="shrink">
              <input
                type="checkbox"
                checked={allShownSelected}
                indeterminate={someShownSelected && !allShownSelected}
                onchange={toggleAll}
                aria-label="Select all shown"
              />
            </th>
            <th><button type="button" class="ss-sort" onclick={() => sortBy('name')}>Snapshot{sortKey === 'name' ? (sortDesc ? ' ↓' : ' ↑') : ''}</button></th>
            <th class="zt-hide-narrow">Kind</th>
            <th class="num zt-hide-narrow"><button type="button" class="ss-sort" onclick={() => sortBy('used')}>Used{sortKey === 'used' ? (sortDesc ? ' ↓' : ' ↑') : ''}</button></th>
            <th class="num zt-hide-narrow">Refers</th>
            <th class="zt-hide-narrow"><button type="button" class="ss-sort" onclick={() => sortBy('creation')}>Created{sortKey === 'creation' ? (sortDesc ? ' ↓' : ' ↑') : ''}</button></th>
            <th class="shrink"><span class="ss-vh">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {#each shown as s (s.name)}
            {@const k = kindOf(s)}
            <tr class:ss-sel={selected.has(s.name)}>
              <td class="shrink"><input type="checkbox" checked={selected.has(s.name)} onchange={() => toggle(s.name)} aria-label="Select {s.name}" /></td>
              <td class="ss-mgr-name">
                <span class="ss-mgr-ds">{s.dataset}@</span><span class="zt-mono">{s.snapshot}</span>
                <div class="ss-mgr-meta">
                  <span class="zt-pill {kindClass(k)}">{k}</span>
                  {bytes(s.used)} · {relative(s.creation)}{s.held ? ' · held' : ''}
                </div>
              </td>
              <td class="zt-hide-narrow"><span class="zt-pill {kindClass(k)}">{k}</span></td>
              <td class="num zt-hide-narrow" class:zt-warn-text={s.used > 1073741824}>{bytes(s.used)}</td>
              <td class="num dim zt-hide-narrow">{bytes(s.refer)}</td>
              <td class="dim zt-hide-narrow" title={dateTime(s.creation)}>{relative(s.creation)}</td>
              <td class="shrink ss-mgr-act">
                <span class="zt-hide-narrow"><Button icon={s.held ? 'lock' : 'unlock'} small kind="ghost" title={s.held ? 'Held - click to release' : 'Hold: protect from pruning'} onclick={() => setHold([s], !s.held)} /></span>
                <Button icon="more" small kind="ghost" title="More" onclick={(e) => rowMenu(e, s)} />
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if filtered.length > shown.length}
      <div class="ss-mgr-more">
        <Button small onclick={() => (limit += 300)}>Show {Math.min(300, filtered.length - shown.length)} more</Button>
        <span class="zt-hint">{count(filtered.length - shown.length)} not shown</span>
      </div>
    {/if}
  {/if}

  {#if selected.size}
    <div class="zt-selbar">
      <b>{plural(selected.size, 'snapshot')} selected</b>
      {#if hiddenSelected}<span class="zt-warn-text">{hiddenSelected} of them hidden by the filter</span>{/if}
      <div class="zt-spacer"></div>
      <Button icon="lock" small busy={working} onclick={() => setHold(selectedItems.filter((s) => !s.held), true)}>Hold</Button>
      <Button icon="unlock" small busy={working} onclick={() => setHold(selectedItems.filter((s) => s.held), false)}>Release</Button>
      <Button icon="trash" small kind="danger" onclick={() => destroy(selectedItems)}>Delete</Button>
      <Button icon="x" small kind="ghost" title="Clear the selection" onclick={() => selected.clear()} />
    </div>
  {/if}
</div>

<style>
  :global(#zt-app) .ss-mgr-scroll {
    max-height: 70vh;
    overflow-y: auto;
    scrollbar-width: thin;
  }
  :global(#zt-app) table.ss-mgr thead th {
    position: sticky;
    top: 0;
    z-index: 1;
    background: var(--zt-panel);
  }
  :global(#zt-app) .ss-mgr-name {
    overflow-wrap: anywhere;
  }
  :global(#zt-app) .ss-mgr-meta {
    display: none;
    margin-top: 4px;
    font-size: 12px;
    color: var(--zt-dim);
  }
  @media (max-width: 560px) {
    :global(#zt-app) .ss-mgr-meta {
      display: block;
    }
    :global(#zt-app) .ss-mgr-ds {
      display: block;
    }
  }
  :global(#zt-app) .ss-mgr-ds {
    color: var(--zt-dim);
    font-size: 12.5px;
  }
  :global(#zt-app) table.ss-mgr tr.ss-sel td {
    background: var(--zt-select-soft);
  }
  :global(#zt-app) .ss-mgr-act {
    text-align: right;
  }
  :global(#zt-app) .ss-sort {
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    color: inherit;
    text-transform: inherit;
    letter-spacing: inherit;
    cursor: pointer;
  }
  :global(#zt-app) .ss-sort:hover {
    color: var(--zt-text);
  }
  :global(#zt-app) .ss-vh {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }
  :global(#zt-app) .ss-mgr-pad {
    padding: 14px;
    margin: 0;
  }
  :global(#zt-app) .ss-mgr-more {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 14px;
    border-top: 1px solid var(--zt-border);
  }
  :global(#zt-app) .ss-mgr-tools :global(.zt-seg) {
    flex-wrap: wrap;
  }
</style>
