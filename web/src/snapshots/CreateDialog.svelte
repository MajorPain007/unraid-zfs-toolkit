<script lang="ts">
  import { untrack } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import ErrorBox from '../lib/ui/ErrorBox.svelte';
  import { api } from '../lib/api';
  import { toast } from '../lib/state.svelte';
  import { reloadAll, snap } from './store.svelte';

  let { dataset = '', recursive = false, close }: { dataset?: string; recursive?: boolean; close: (r?: unknown) => void } = $props();

  function stamp(): string {
    const d = new Date();
    const p = (n: number) => String(n).padStart(2, '0');
    return `manual-${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}_${p(d.getHours())}-${p(d.getMinutes())}`;
  }

  const form = $state(untrack(() => ({ dataset, name: stamp(), recursive })));
  let busy = $state(false);
  let error = $state<unknown>(null);
  let names = $derived(snap.available.length ? snap.available.map((a) => a.name) : snap.datasets.map((d) => d.name));
  let problem = $derived(
    !form.name.trim()
      ? ''
      : /^(auto-|zdc-send-)/.test(form.name.trim())
        ? 'auto- and zdc-send- are reserved for the scheduler and replication.'
        : !/^[A-Za-z0-9_][A-Za-z0-9_.: -]*$/.test(form.name.trim())
          ? 'Use letters, digits, space and _ . : -'
          : ''
  );

  async function create() {
    if (problem || !form.dataset) return;
    busy = true;
    error = null;
    try {
      const r = await api<{ name: string }>('snap_create', { dataset: form.dataset, name: form.name.trim(), recursive: form.recursive ? 1 : 0 });
      toast('Created ' + r.name);
      reloadAll();
      close(r.name);
    } catch (e) {
      error = e;
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="Take a snapshot" icon="camera" onclose={() => close()}>
  <ErrorBox {error} />
  <div class="zt-fields ss-one">
    {#if dataset}
      <Field label="Dataset"><div class="zt-mono ss-ro">{dataset}</div></Field>
    {:else}
      <Field label="Dataset">
        <select bind:value={form.dataset}>
          <option value="" disabled>Choose a dataset</option>
          {#each names as n (n)}<option value={n}>{n}</option>{/each}
        </select>
      </Field>
    {/if}
    <Field label="Name" hint="Snapshots that do not start with auto- are never pruned automatically.">
      <input type="text" class="zt-mono" class:invalid={!!problem} bind:value={form.name} spellcheck="false" />
    </Field>
    {#if problem}<div class="zt-error">{problem}</div>{/if}
    <Toggle bind:checked={form.recursive} label="Include child datasets" />
  </div>
  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="primary" icon="camera" {busy} disabled={!!problem || !form.dataset || !form.name.trim()} onclick={create}>Take snapshot</Button>
  {/snippet}
</Dialog>

<style>
  :global(#zt-app) .ss-one {
    grid-template-columns: minmax(0, 1fr);
  }
  :global(#zt-app) .ss-ro {
    padding: 7px 0;
    overflow-wrap: anywhere;
  }
</style>
