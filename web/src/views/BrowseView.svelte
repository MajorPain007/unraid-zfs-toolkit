<script lang="ts" module>
  import { provide } from '../lib/actions';
  import { show } from '../lib/state.svelte';

  provide({
    browseSnapshot: (o: { dataset: string; snapshot: string; path?: string }) => show('browse', { ...o, open: Date.now() })
  });
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import SnapshotList from '../browse/SnapshotList.svelte';
  import Browser from '../browse/Browser.svelte';
  import RestoreCard from '../browse/RestoreCard.svelte';
  import { app, busy, onRefresh } from '../lib/state.svelte';
  import { plural } from '../lib/format';
  import { B, KIND_LABEL, loadDatasets, loadSnaps, openAt, pickDefaultDataset, reload, selectDataset, type Restore } from '../browse/store.svelte';

  let { opts = {} }: { opts?: Record<string, any> } = $props();

  let reloading = $state({ busy: false });

  let part = $derived(app.overview?.browse as { active?: Restore[]; last?: Restore | null } | undefined);
  let active = $derived(part?.active ?? []);
  let last = $derived(part?.last && !part.last.dismissed ? part.last : null);
  let withSnaps = $derived((B.datasets || []).filter((d) => d.snapshots > 0));
  let kinds = $derived([...new Set((B.snaps || []).map((s) => s.kind))]);

  untrack(() => {
    if (opts.dataset && opts.snapshot) {
      openAt(opts.dataset, opts.snapshot, opts.path || '/');
      return;
    }
    if (!B.datasets && !B.dsLoading) {
      loadDatasets().then(() => {
        if (!B.dataset) {
          const d = pickDefaultDataset();
          if (d) selectDataset(d);
        }
      });
    }
  });

  let seenFinished = untrack(() => part?.last?.id ?? '');
  $effect(() =>
    onRefresh((o) => {
      const l = (o.browse as { last?: Restore | null } | undefined)?.last;
      if (l && l.id !== seenFinished) {
        seenFinished = l.id;
        if (l.dataset === B.dataset && B.snap) reload();
      }
    })
  );

  async function reloadAll() {
    await loadDatasets();
    if (B.dataset) await loadSnaps();
    reload();
  }
</script>

{#each active as r (r.id)}
  <RestoreCard {r} />
{/each}
{#if last && !active.length}
  <RestoreCard r={last} />
{/if}

{#if B.dsError && !B.datasets}
  <div class="zt-empty">
    <Icon name="alert" class="big" />
    <h3>Could not list the datasets</h3>
    <p>{B.dsError}</p>
    <Button icon="refresh" onclick={() => loadDatasets()}>Try again</Button>
  </div>
{:else if !B.datasets}
  <div class="zt-hint pad"><Icon name="loader" class="zt-spin" /> Loading datasets...</div>
{:else if !withSnaps.length}
  <div class="zt-empty">
    <Icon name="layers" class="big" />
    <h3>No snapshots yet</h3>
    <p>Once a dataset has snapshots, their files can be browsed, compared and restored here.</p>
    <Button icon="camera" kind="primary" onclick={() => show('snapshots')}>Set up snapshots</Button>
  </div>
{:else}
  <div class="zt-toolbar">
    <select aria-label="Dataset" value={B.dataset} onchange={(e) => selectDataset(e.currentTarget.value)}>
      {#if !B.dataset}<option value="">Choose a dataset</option>{/if}
      {#each withSnaps as d (d.name)}
        <option value={d.name}>{d.name} ({plural(d.snapshots, 'snapshot')})</option>
      {/each}
    </select>
    {#if kinds.length > 1}
      <select aria-label="Kind of snapshot" bind:value={B.kind}>
        <option value="">All kinds</option>
        {#each kinds as k (k)}
          <option value={k}>{KIND_LABEL[k] || k}</option>
        {/each}
      </select>
    {/if}
    <Button icon="refresh" title="Reload" busy={reloading.busy} onclick={() => busy(reloading, reloadAll())} />
  </div>
  {#if B.problem}
    <div class="zt-card-note warn problem"><Icon name="alert" /><div>{B.problem}</div></div>
  {/if}
  <div class="zt-split">
    <SnapshotList />
    <Browser />
  </div>
{/if}

<style>
  :global(#zt-app) .pad {
    padding: 16px;
  }
  .pad :global(.zt-spin) {
    animation: zt-spin 0.9s linear infinite;
  }
  :global(#zt-app) .problem {
    margin-bottom: 14px;
  }
  @media (max-width: 560px) {
    :global(#zt-app) .zt-toolbar select {
      flex: 1 1 100%;
      min-width: 0;
    }
  }
</style>
