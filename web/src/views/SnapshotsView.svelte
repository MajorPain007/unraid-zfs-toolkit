<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { onRefresh, openDialog } from '../lib/state.svelte';
  import { loadConfig, loadList, part, setEnabled, snap } from '../snapshots/store.svelte';
  import StatusCard from '../snapshots/StatusCard.svelte';
  import DatasetCard from '../snapshots/DatasetCard.svelte';
  import RetentionCard from '../snapshots/RetentionCard.svelte';
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

  $effect(() => {
    if (opts.dataset) setTimeout(() => document.getElementById('ss-manager')?.scrollIntoView({ block: 'start' }), 200);
  });

  async function turnOn() {
    turningOn = true;
    await setEnabled(true);
    turningOn = false;
  }
</script>

<StatusCard />

{#if enabled}
  <div class="zt-section-head">
    <h2>Datasets &amp; retention</h2>
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
    <div class="zt-grid">
      <RetentionCard />
      {#each snap.config as ds (ds.name)}
        <DatasetCard {ds} missing={missing.has(ds.name)} />
      {/each}
      {#if snap.configLoaded}
        <button type="button" class="zt-card add" onclick={() => openDialog(AddDialog, {})}>
          <Icon name="plus" />
          {snap.config.length ? 'Add a dataset' : 'Add the first dataset to snapshot'}
          {#if !snap.config.length}<span class="zt-hint ss-add-hint">appdata, domains or a share - recursive covers everything below it</span>{/if}
        </button>
      {/if}
    </div>
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

<div class="zt-section-head" id="ss-manager">
  <h2>Snapshot manager</h2>
</div>
<Manager dataset={opts.dataset || ''} />

<style>
  :global(#zt-app) .ss-add-hint {
    max-width: 260px;
    text-align: center;
    font-weight: 400;
  }
  :global(#zt-app) .ss-off {
    margin-top: 16px;
  }
</style>
