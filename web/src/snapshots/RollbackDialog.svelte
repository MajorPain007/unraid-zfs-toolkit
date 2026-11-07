<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { fail, toast } from '../lib/state.svelte';
  import { dateTime, plural } from '../lib/format';
  import type { SnapInfo } from './types';

  let { snapshot, close }: { snapshot: SnapInfo; close: (r?: unknown) => void } = $props();

  let check = $state<{ newer: SnapInfo[]; held: number; checkpoints: number } | null>(null);
  let error = $state('');
  let sure = $state(false);
  let busy = $state(false);
  let releasing = $state(false);

  async function load() {
    try {
      check = await api('snap_rollback_check', { snapshot: snapshot.name });
    } catch (e) {
      error = (e as Error).message;
    }
  }
  load();

  async function releaseHeld() {
    if (!check) return;
    releasing = true;
    try {
      await api('snap_release', { snapshots: check.newer.filter((s) => s.held).map((s) => s.name) });
      await load();
    } catch (e) {
      fail(e);
    } finally {
      releasing = false;
    }
  }

  async function go() {
    busy = true;
    try {
      const r = await api<{ message: string }>('snap_rollback', { snapshot: snapshot.name, confirm: snapshot.name });
      toast(r.message);
      close(r);
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="Roll back {snapshot.dataset}" icon="restore" onclose={() => close()}>
  <p>
    Back to <b class="zt-mono">{snapshot.snapshot}</b> from {dateTime(snapshot.creation)}. Every change made to this dataset since then is
    <b>discarded</b>.
  </p>
  {#if error}
    <div class="zt-errors">{error}</div>
  {:else if !check}
    <p class="zt-dim"><Icon name="loader" class="zt-spin" /> Checking what a rollback would destroy&hellip;</p>
  {:else}
    {#if check.newer.length}
      <p>{plural(check.newer.length, 'newer snapshot')} of this dataset will be destroyed:</p>
      <div class="ss-rb-list">
        {#each check.newer as s (s.name)}
          <div>
            <span class="zt-mono">{s.snapshot}</span>
            {#if s.held}<span class="zt-pill accent"><Icon name="lock" />held</span>{/if}
            {#if s.checkpoint}<span class="zt-pill warn"><Icon name="send" />replication</span>{/if}
          </div>
        {/each}
      </div>
    {:else}
      <p class="zt-dim">No newer snapshot exists, nothing else is destroyed.</p>
    {/if}
    {#if check.checkpoints}
      <div class="zt-card-note warn">
        <Icon name="send" />
        <div>{plural(check.checkpoints, 'replication checkpoint')} go with them - those jobs need a full send afterwards.</div>
      </div>
    {/if}
    {#if check.held}
      <div class="zt-card-note err">
        <Icon name="lock" />
        <div>
          {plural(check.held, 'newer snapshot is', 'newer snapshots are')} held. ZFS would destroy the others and then stop halfway, so the rollback is
          blocked until the hold is released.
          <button type="button" class="zt-link ss-rb-link" disabled={releasing} onclick={releaseHeld}>Release the holds</button>
        </div>
      </div>
    {/if}
    <label class="ss-rb-sure">
      <input type="checkbox" bind:checked={sure} />
      I have stopped the containers and VMs that use {snapshot.dataset}, and want to discard its newer data.
    </label>
  {/if}
  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="danger solid" icon="restore" {busy} disabled={!check || !sure || check.held > 0} onclick={go}>Roll back</Button>
  {/snippet}
</Dialog>

<style>
  :global(#zt-app) .ss-rb-list {
    max-height: 200px;
    overflow-y: auto;
    padding: 8px 10px;
    border-radius: var(--zt-radius-sm);
    background: var(--zt-surface-2);
    line-height: 1.8;
    overflow-wrap: anywhere;
  }
  :global(#zt-app) .ss-rb-list .zt-pill {
    margin-left: 6px;
  }
  :global(#zt-app) .ss-rb-sure {
    display: flex;
    gap: 9px;
    align-items: flex-start;
    margin-top: 14px;
    font-size: 13px;
    cursor: pointer;
  }
  :global(#zt-app) .ss-rb-sure input {
    margin-top: 3px;
  }
  :global(#zt-app) .ss-rb-link {
    margin-left: 6px;
    font-weight: 600;
  }
</style>
