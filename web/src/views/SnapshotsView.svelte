<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { listLayout, onRefresh, openDialog } from '../lib/state.svelte';
  import { focusManager, loadConfig, loadList, part, saveConfig, setEnabled, snap } from '../snapshots/store.svelte';
  import StatusCard from '../snapshots/StatusCard.svelte';
  import DatasetRow from '../snapshots/DatasetRow.svelte';
  import DatasetCard from '../snapshots/DatasetCard.svelte';
  import Manager from '../snapshots/Manager.svelte';
  import AddDialog from '../snapshots/AddDialog.svelte';

  let { opts = {} }: { opts?: Record<string, any> } = $props();

  let p = $derived(part());
  let enabled = $derived(!!p?.enabled);
  let missing = $derived(new Set(p?.missing ?? []));
  let turningOn = $state(false);

  loadConfig();
  loadList();

  let stamp = '';
  $effect(() =>
    onRefresh((o) => {
      const sp = o.snapshots;
      if (!sp || typeof sp !== 'object') return;
      const next = [sp.count, sp.total, sp.held, sp.last?.ended, sp.running, sp.datasets].join('|');
      if (stamp && next !== stamp) {
        loadList();
        if (String(sp.datasets) !== String(snap.config.length)) loadConfig();
      }
      stamp = next;
    })
  );

  let focused = false;
  $effect(() => {
    if (opts.dataset && snap.configLoaded && snap.listLoaded && !focused) {
      focused = true;
      focusManager(String(opts.dataset));
    }
  });

  let outsideCount = $derived(
    snap.list.filter((x) => !snap.config.some((d) => x.dataset === d.name || (d.recursive && x.dataset.startsWith(d.name + '/')))).length
  );
  let list = $derived(listLayout());
  let allOpen = $derived(snap.allOpen || !enabled);

  let armed = $state('');
  let dragging = $state('');
  let over = $state('');

  function reorder(from: string, to: string) {
    const list = [...snap.config];
    const i = list.findIndex((d) => d.name === from);
    const j = list.findIndex((d) => d.name === to);
    if (i < 0 || j < 0 || i === j) return;
    const [d] = list.splice(i, 1);
    list.splice(j, 0, d);
    snap.config = list;
    saveConfig(list);
  }

  function move(name: string, dir: -1 | 1) {
    const i = snap.config.findIndex((d) => d.name === name);
    const j = i + dir;
    if (i < 0 || j < 0 || j >= snap.config.length) return;
    reorder(name, snap.config[j].name);
  }

  function dragEnd() {
    armed = '';
    dragging = '';
    over = '';
  }

  async function turnOn() {
    turningOn = true;
    await setEnabled(true);
    turningOn = false;
  }
</script>

<svelte:window
  onpointerup={() => {
    if (!dragging) armed = '';
  }}
/>

<StatusCard />

{#if enabled}
  <div class="zt-section-head">
    <h2>{list ? 'Scheduled datasets' : 'Datasets'}</h2>
    <Button icon="plus" small disabled={!snap.configLoaded} onclick={() => openDialog(AddDialog, {})}>Add dataset</Button>
  </div>
  {#if snap.configError}
    <div class="zt-callout warn">
      <Icon name="alert" />
      <div>
        <b>The dataset list could not be read.</b>
        {snap.configError}
        <button type="button" class="zt-link" onclick={loadConfig}>Try again</button>
      </div>
    </div>
  {:else}
    {#if !list}
      <div class="zt-grid ss-cards">
        {#each snap.config as ds (ds.name)}
          <div
            class="ss-drag"
            class:dragging={dragging === ds.name}
            class:over={over === ds.name && dragging && dragging !== ds.name}
            role="listitem"
            draggable={armed === ds.name}
            ondragstart={(e) => {
              dragging = ds.name;
              e.dataTransfer?.setData('text/plain', ds.name);
              if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
            }}
            ondragover={(e) => {
              if (!dragging) return;
              e.preventDefault();
              over = ds.name;
            }}
            ondragleave={() => {
              if (over === ds.name) over = '';
            }}
            ondrop={(e) => {
              e.preventDefault();
              if (dragging) reorder(dragging, ds.name);
              dragEnd();
            }}
            ondragend={dragEnd}
          >
            <DatasetCard {ds} missing={missing.has(ds.name)} onarm={() => (armed = ds.name)} onmove={(d) => move(ds.name, d)} />
          </div>
        {/each}
        {#if snap.configLoaded}
          <button type="button" class="zt-card add" onclick={() => openDialog(AddDialog, {})}>
            <Icon name="plus" />
            {snap.config.length ? 'Add a dataset' : 'Add the first dataset to snapshot'}
            {#if !snap.config.length}<span class="zt-hint ss-add-hint">appdata, domains or a share - recursive covers everything below it</span>{/if}
          </button>
        {/if}
      </div>
    {:else if snap.configLoaded && !snap.config.length}
      <div class="zt-empty">
        <Icon name="database" class="big" />
        <h3>No dataset is snapshotted yet</h3>
        <p>Add appdata, domains or a share. Recursive covers every dataset below it.</p>
        <Button icon="plus" kind="primary" onclick={() => openDialog(AddDialog, {})}>Add the first dataset</Button>
      </div>
    {:else}
      <div class="zt-table-wrap ss-list">
        <div class="ss-list-head zt-hide-narrow" aria-hidden="true">
          <span></span><span>Dataset</span><span>Rule</span><span class="r">Snapshots</span><span class="r">Space</span><span>Newest</span><span>Last 30 days</span>
        </div>
        {#each snap.config as ds (ds.name)}
          <DatasetRow {ds} missing={missing.has(ds.name)} />
        {/each}
      </div>
    {/if}
  {/if}
{:else if p}
  <div class="zt-empty ss-off">
    <Icon name="camera" class="big" />
    <h3>Automatic snapshots are off</h3>
    <p>
      Turn them on to take hourly, daily, weekly and monthly snapshots of the datasets you choose. They are pruned by retention, and you can browse
      and restore single files from them. Snapshots you already have stay untouched either way.
    </p>
    <Button icon="power" kind="primary" busy={turningOn} onclick={turnOn}>Turn on automatic snapshots</Button>
  </div>
{/if}

{#if !list}
  <div class="zt-section-head" id="ss-manager">
    <h2>Snapshot manager</h2>
  </div>
  <Manager dataset={opts.dataset || ''} />
{:else}
<details class="ss-all" id="ss-manager" open={allOpen} ontoggle={(e) => (snap.allOpen = (e.currentTarget as HTMLDetailsElement).open)}>
  <summary class="zt-section-head">
    <h2>{enabled ? 'All snapshots on this system' : 'Snapshots on this system'}</h2>
    {#if enabled && snap.listLoaded}<span class="zt-hint">{outsideCount} outside the scheduled datasets · {snap.list.length} in total</span>{/if}
  </summary>
  {#if allOpen}
    <Manager outside={enabled} />
  {/if}
</details>
{/if}

<style>
  :global(#zt-app) .ss-list {
    --ss-cols: 16px minmax(0, 1fr) 76px 84px 84px 104px 140px;
  }
  :global(#zt-app) .ss-list-head {
    display: grid;
    grid-template-columns: var(--ss-cols);
    gap: 12px;
    padding: 8px 48px 8px 12px;
    border-bottom: 1px solid var(--zt-border);
    background: var(--zt-surface-2);
    color: var(--zt-dim);
    font-size: 11.5px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  @media (max-width: 760px) {
    :global(#zt-app) .ss-list-head {
      display: none;
    }
  }
  :global(#zt-app) .ss-list-head .r {
    text-align: right;
  }
  :global(#zt-app) details.ss-all > summary {
    flex-wrap: wrap;
    row-gap: 4px;
    cursor: pointer;
    list-style: none;
    margin-top: 28px;
  }
  :global(#zt-app) details.ss-all > summary::-webkit-details-marker {
    display: none;
  }
  :global(#zt-app) details.ss-all > summary h2 {
    flex: none;
  }
  :global(#zt-app) details.ss-all > summary::before {
    content: "";
    width: 6px;
    height: 6px;
    border-right: 2px solid var(--zt-dim);
    border-bottom: 2px solid var(--zt-dim);
    transform: rotate(-45deg);
    transition: transform 0.15s;
  }
  :global(#zt-app) details.ss-all[open] > summary::before {
    transform: rotate(45deg);
  }
  :global(#zt-app) .zt-grid.ss-cards {
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 480px), 1fr));
  }
  :global(#zt-app) .ss-drag {
    display: flex;
    min-width: 0;
    border-radius: var(--zt-radius);
    transition: opacity 0.15s, box-shadow 0.15s;
  }
  :global(#zt-app) .ss-drag > :global(.zt-card) {
    flex: 1;
  }
  :global(#zt-app) .ss-drag.dragging {
    opacity: 0.45;
  }
  :global(#zt-app) .ss-drag.over {
    box-shadow: 0 0 0 2px var(--zt-accent);
  }
  :global(#zt-app) .ss-add-hint {
    max-width: 260px;
    text-align: center;
    font-weight: 400;
  }
  :global(#zt-app) .ss-off {
    margin-top: 16px;
  }
</style>
