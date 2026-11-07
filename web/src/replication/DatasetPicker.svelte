<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { bytes, plural } from '../lib/format';
  import type { SendDataset, SendJob } from './send.svelte';

  let {
    title,
    remote = null,
    current = '',
    hint = '',
    close
  }: { title: string; remote?: SendJob | null; current?: string; hint?: string; close: (r?: string) => void } = $props();

  let items = $state<SendDataset[]>([]);
  let error = $state('');
  let loading = $state(true);
  let query = $state('');
  let picked = $state('');

  let q = $derived(query.trim().toLowerCase());
  let shown = $derived(q ? items.filter((d) => d.name.toLowerCase().includes(q)) : items);

  onMount(async () => {
    picked = current;
    try {
      const r = remote ? await api<{ datasets: SendDataset[] }>('send_targets', { job: remote }) : await api<{ datasets: SendDataset[] }>('send_datasets');
      items = r.datasets;
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loading = false;
    }
  });

  function depth(name: string): number {
    return name.split('/').length - 1;
  }

  function leaf(name: string): string {
    const i = name.lastIndexOf('/');
    return i < 0 ? name : name.slice(i + 1);
  }
</script>

<Dialog {title} icon="database" wide onclose={() => close()}>
  {#if hint}<p class="zt-hint zr-dp-hint">{hint}</p>{/if}
  <div class="zt-toolbar">
    <input type="search" placeholder="Filter" aria-label="Filter datasets" bind:value={query} autocomplete="off" spellcheck="false" />
    <div class="zt-spacer"></div>
    {#if !loading && !error}<span class="zt-hint">{plural(items.length, 'dataset')}</span>{/if}
  </div>
  {#if loading}
    <div class="zt-hint">Loading&hellip;</div>
  {:else if error}
    <div class="zt-card-note err"><Icon name="alert" /><div>{error}</div></div>
  {:else}
    <div class="zr-dp-list" role="listbox" aria-label="Datasets">
      {#each shown as d (d.name)}
        <button
          type="button"
          role="option"
          class="zr-dp-row"
          class:active={picked === d.name}
          aria-selected={picked === d.name}
          style:padding-left="{10 + (q ? 0 : depth(d.name) * 18)}px"
          onclick={() => (picked = d.name)}
          ondblclick={() => close(d.name)}
        >
          <Icon name={depth(d.name) === 0 ? 'hdd' : 'database'} />
          <span class="zr-dp-name">{q ? d.name : leaf(d.name)}</span>
          {#if depth(d.name) === 0}<span class="zt-pill">pool</span>{/if}
          {#if d.replica}<span class="zt-pill accent" title="Already the destination of a replication job">copy</span>{/if}
          {#if d.readonly}<span class="zt-pill" title="readonly=on">read-only</span>{/if}
          {#if d.encryption && d.encryption !== 'off' && d.encryption !== '-'}<span class="zt-pill" title={d.encryption}><Icon name="lock" />encrypted</span>{/if}
          <span class="zr-dp-meta">
            {bytes(d.used)}{d.children ? ' · ' + plural(d.children, 'child', 'children') : ''}{d.snapshots ? ' · ' + plural(d.snapshots, 'snapshot') : ''}
          </span>
        </button>
      {:else}
        <div class="zt-hint zr-dp-empty">Nothing matches.</div>
      {/each}
    </div>
  {/if}
  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="primary" icon="check" disabled={!picked} onclick={() => close(picked)}>Choose</Button>
  {/snippet}
</Dialog>

<style>
  .zr-dp-hint {
    margin: 0 0 12px;
  }
  .zr-dp-list {
    max-height: min(56vh, 520px);
    overflow: auto;
    border: 1px solid var(--zt-border);
    border-radius: var(--zt-radius-sm);
  }
  :global(#zt-app) .zr-dp-row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 10px;
    border: 0;
    border-bottom: 1px solid var(--zt-border);
    background: transparent;
    color: var(--zt-text);
    font-size: 13px;
    text-align: left;
    cursor: pointer;
    margin: 0;
    border-radius: 0;
  }
  :global(#zt-app) .zr-dp-row:last-child {
    border-bottom: 0;
  }
  :global(#zt-app) .zr-dp-row:hover {
    background: var(--zt-surface-2);
  }
  :global(#zt-app) .zr-dp-row.active {
    background: var(--zt-select);
  }
  :global(#zt-app) .zr-dp-row :global(svg.zt-i) {
    flex: none;
    color: var(--zt-dim);
  }
  .zr-dp-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--zt-mono);
    font-size: 12.5px;
  }
  :global(#zt-app) .zr-dp-row .zt-pill {
    font-size: 10.5px;
    padding: 0 6px;
  }
  .zr-dp-meta {
    margin-left: auto;
    color: var(--zt-dim);
    font-size: 12px;
    white-space: nowrap;
    padding-left: 10px;
  }
  .zr-dp-empty {
    padding: 14px;
  }
  @media (max-width: 560px) {
    .zr-dp-meta {
      display: none;
    }
  }
</style>
