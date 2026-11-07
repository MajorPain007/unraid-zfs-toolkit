<script lang="ts">
  import { untrack } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { fail, toast } from '../lib/state.svelte';
  import { bytes, plural } from '../lib/format';
  import type { SnapInfo } from './types';

  let { snapshots, close }: { snapshots: SnapInfo[]; close: (r?: unknown) => void } = $props();

  const items = untrack(() => snapshots);
  const checkpoints = items.filter((s) => s.checkpoint);
  const held = items.filter((s) => s.held);
  let withCheckpoints = $state(false);
  let busy = $state(false);

  let targets = $derived(items.filter((s) => !s.held && (!s.checkpoint || withCheckpoints)));
  let size = $derived(targets.reduce((a, s) => a + s.used, 0));

  async function go() {
    busy = true;
    try {
      const r = await api<{ deleted: number; failed: number; results: { name: string; ok: boolean; error?: string }[] }>('snap_destroy', {
        snapshots: targets.map((s) => s.name),
        force_checkpoint: withCheckpoints ? 1 : 0
      });
      const bad = r.results.filter((x) => !x.ok);
      if (bad.length) toast(`${r.deleted} deleted, ${bad.length} failed: ${bad[0].name} - ${bad[0].error}`, 'err');
      else toast(plural(r.deleted, 'snapshot') + ' deleted');
      close(r);
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="Delete {plural(items.length, 'snapshot')}?" icon="alert" onclose={() => close()}>
  <p>
    {#if targets.length}
      <b>{plural(targets.length, 'snapshot')}</b> will be destroyed{size ? `, freeing up to ${bytes(size)}` : ''}. This cannot be undone.
    {:else}
      Nothing left to delete with these choices.
    {/if}
  </p>
  <div class="ss-dl">
    {#each targets.slice(0, 12) as s (s.name)}<div class="zt-mono">{s.name}</div>{/each}
    {#if targets.length > 12}<div class="zt-dim">and {targets.length - 12} more</div>{/if}
  </div>
  {#if held.length}
    <div class="zt-card-note info"><Icon name="lock" /><div>{plural(held.length, 'held snapshot')} skipped - release the hold first.</div></div>
  {/if}
  {#if checkpoints.length}
    <div class="zt-card-note warn">
      <Icon name="send" />
      <div>
        {plural(checkpoints.length, 'replication checkpoint')} selected. They are the base of the incremental chain - deleting them means the next run
        of those jobs sends everything again.
        <label class="ss-dl-check"><input type="checkbox" bind:checked={withCheckpoints} /> Delete the checkpoints too</label>
      </div>
    </div>
  {/if}
  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="danger solid" icon="trash" {busy} disabled={!targets.length} onclick={go}>Delete</Button>
  {/snippet}
</Dialog>

<style>
  :global(#zt-app) .ss-dl {
    max-height: 220px;
    overflow-y: auto;
    padding: 8px 10px;
    border-radius: var(--zt-radius-sm);
    background: var(--zt-surface-2);
    overflow-wrap: anywhere;
    line-height: 1.6;
  }
  :global(#zt-app) .ss-dl-check {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-top: 8px;
    color: var(--zt-text);
    font-weight: 600;
    cursor: pointer;
  }
</style>
