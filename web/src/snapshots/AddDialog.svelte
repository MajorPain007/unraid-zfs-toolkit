<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { bytes } from '../lib/format';
  import { toast } from '../lib/state.svelte';
  import { addDataset, loadConfig, snap } from './store.svelte';

  let { close }: { close: (r?: unknown) => void } = $props();

  let query = $state('');
  let picked = $state('');
  let recursive = $state(false);
  let busy = $state(false);

  if (!snap.available.length) loadConfig();

  let taken = $derived(new Set(snap.config.map((d) => d.name)));
  let coveredBy = $derived((n: string) => snap.config.find((d) => d.recursive && n.startsWith(d.name + '/'))?.name ?? '');
  let shown = $derived(
    snap.available.filter((a) => (!query.trim() || a.name.toLowerCase().includes(query.trim().toLowerCase())))
  );
  let hasChildren = $derived(!!picked && snap.available.some((a) => a.name.startsWith(picked + '/')));

  function depth(n: string): number {
    return n.split('/').length - 1;
  }

  async function add() {
    if (!picked) return;
    busy = true;
    const ok = await addDataset(picked, recursive && hasChildren);
    busy = false;
    if (ok) {
      toast(picked + ' is now snapshotted with the global retention');
      close(picked);
    }
  }
</script>

<Dialog title="Add a dataset" icon="plus" onclose={() => close()}>
  <input type="search" placeholder="Filter datasets" bind:value={query} aria-label="Filter datasets" />
  <div class="ss-pick" role="listbox" aria-label="Datasets">
    {#each shown as a (a.name)}
      <button
        type="button"
        role="option"
        aria-selected={picked === a.name}
        aria-disabled={taken.has(a.name)}
        disabled={taken.has(a.name)}
        class:active={picked === a.name}
        class:taken={taken.has(a.name)}
        title={taken.has(a.name) ? a.name + ' is already in the schedule' : null}
        style:padding-left="{10 + Math.min(depth(a.name), 6) * 14}px"
        onclick={() => (picked = a.name)}
        ondblclick={() => {
          picked = a.name;
          add();
        }}
      >
        <Icon name={a.type === 'volume' ? 'hdd' : depth(a.name) ? 'folder' : 'database'} />
        <span class="ss-pick-name">{a.name}</span>
        {#if taken.has(a.name)}<span class="zt-pill">scheduled</span>{:else if coveredBy(a.name)}<span class="zt-pill" title="Already included by the recursive {coveredBy(a.name)}">covered</span>{/if}
        <span class="ss-pick-size">{bytes(a.used)}</span>
      </button>
    {:else}
      <div class="zt-hint ss-pick-empty">{snap.available.length ? 'No dataset matches.' : 'Loading datasets…'}</div>
    {/each}
  </div>
  <div class="ss-gap">
    <Toggle bind:checked={recursive} disabled={!hasChildren} label="Include child datasets (recursive)" />
  </div>
  <div class="zt-hint ss-gap">The dataset starts with the global retention. You can give it its own afterwards.</div>
  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="primary" icon="plus" {busy} disabled={!picked} onclick={add}>Add</Button>
  {/snippet}
</Dialog>

<style>
  :global(#zt-app) .ss-pick {
    margin-top: 10px;
    max-height: 46vh;
    overflow-y: auto;
    border: 1px solid var(--zt-border);
    border-radius: var(--zt-radius-sm);
    scrollbar-width: thin;
  }
  :global(#zt-app) .ss-pick button {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 10px;
    border: 0;
    border-bottom: 1px solid var(--zt-border);
    background: transparent;
    color: var(--zt-text);
    text-align: left;
    font-size: 13px;
    cursor: pointer;
  }
  :global(#zt-app) .ss-pick button:last-child {
    border-bottom: 0;
  }
  :global(#zt-app) .ss-pick button:hover {
    background: var(--zt-surface-2);
  }
  :global(#zt-app) .ss-pick button.taken {
    opacity: 0.45;
    cursor: default;
  }
  :global(#zt-app) .ss-pick button.taken:hover {
    background: transparent;
  }
  :global(#zt-app) .ss-pick button.active {
    background: var(--zt-select);
    box-shadow: inset 3px 0 0 var(--zt-accent);
  }
  :global(#zt-app) .ss-pick-name {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  :global(#zt-app) .ss-pick-size {
    color: var(--zt-dim);
    font-size: 12px;
    white-space: nowrap;
  }
  :global(#zt-app) .ss-pick-empty {
    padding: 14px;
  }
  :global(#zt-app) .ss-gap {
    margin-top: 12px;
  }
</style>
