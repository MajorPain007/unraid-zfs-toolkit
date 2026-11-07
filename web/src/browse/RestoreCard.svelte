<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { api } from '../lib/api';
  import { fail } from '../lib/state.svelte';
  import { bytes, duration, now, plural, relative } from '../lib/format';
  import { dismissRestore, stopRestore } from './ops';
  import type { Restore } from './store.svelte';

  let { r }: { r: Restore } = $props();

  let stopping = $state({ busy: false });
  let log = $state<string | null>(null);
  let loadingLog = $state(false);

  let active = $derived(r.state === 'queued' || r.state === 'running');
  let level = $derived(active ? 'run' : r.state === 'done' ? 'ok' : r.state === 'cancelled' ? 'warn' : 'err');
  let where = $derived(r.target === 'path' ? r.target_dir : 'where it came from');
  let what = $derived(r.items.length === 1 ? (r.items[0] === '/' ? 'everything' : r.items[0]) : plural(r.total, 'item'));
  let elapsed = $derived(r.started ? (r.finished || now()) - r.started : 0);

  $effect(() => {
    if (log !== null && active) {
      r.bytes_done;
      loadLog();
    }
  });

  async function loadLog() {
    loadingLog = true;
    try {
      const res = await api<{ log: string }>('restore_status', { id: r.id, lines: 300 });
      log = res.log || '(empty)';
    } catch (e) {
      fail(e);
    } finally {
      loadingLog = false;
    }
  }

  function toggleLog() {
    if (log !== null) log = null;
    else loadLog();
  }
</script>

<div class="zt-card card {level}">
  <div class="head">
    <Icon name={active ? 'loader' : r.state === 'done' ? 'check' : 'alert'} class={active ? 'zt-spin' : ''} />
    <div class="rc-title">
      <b>
        {#if active}Restoring {what}{:else if r.state === 'done'}Restored {what}{:else if r.state === 'cancelled'}Restore stopped{:else}Restore failed{/if}
      </b>
      <span class="sub">from <span class="zt-mono">{r.dataset}@{r.snapshot}</span> into {where}</span>
    </div>
    <div class="acts">
      <Button small kind="ghost" icon="list" onclick={toggleLog}>{log === null ? 'Log' : 'Hide log'}</Button>
      {#if active}
        <Button small kind="danger" icon="stop" busy={stopping.busy} onclick={() => stopRestore(r, stopping)}>Stop</Button>
      {:else}
        <Button small kind="ghost" icon="x" title="Dismiss" onclick={() => dismissRestore(r)} />
      {/if}
    </div>
  </div>
  {#if active}
    <div class="zt-progress">
      <div class="zt-progress-track" class:indeterminate={!r.bytes}>
        <div class="zt-progress-bar" style:width={Math.max(2, r.percent) + '%'}></div>
      </div>
      <div class="zt-progress-text">
        <span>
          {#if r.state === 'queued'}Starting...{:else}<b>{Math.floor(r.percent)}%</b> · {bytes(r.bytes_done)} of {bytes(r.bytes)}{/if}
          {#if r.total > 1} · item {Math.min(r.done + r.failed + 1, r.total)} of {r.total}{/if}
        </span>
        <span class="cur zt-mono" title={r.current}>{r.current}</span>
      </div>
    </div>
  {:else}
    <div class="msg">{r.message} <span class="dim">{relative(r.finished)}{elapsed > 5 ? ' · took ' + duration(elapsed) : ''}</span></div>
  {/if}
  {#if log !== null}
    <pre class="zt-log">{loadingLog && log === '' ? 'Loading...' : log}</pre>
  {/if}
</div>

<style>
  .card {
    margin-bottom: 14px;
    padding: 12px 14px;
    border-left: 3px solid var(--zt-info);
  }
  .card.ok {
    border-left-color: var(--zt-ok);
  }
  .card.warn {
    border-left-color: var(--zt-warn);
  }
  .card.err {
    border-left-color: var(--zt-err);
  }
  .head {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .head > :global(svg.zt-i) {
    flex: none;
    color: var(--zt-info);
  }
  .card.ok .head > :global(svg.zt-i) {
    color: var(--zt-ok);
  }
  .card.warn .head > :global(svg.zt-i) {
    color: var(--zt-warn);
  }
  .card.err .head > :global(svg.zt-i) {
    color: var(--zt-err);
  }
  .head :global(.zt-spin) {
    animation: zt-spin 0.9s linear infinite;
  }
  .rc-title {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .rc-title b {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sub {
    font-size: 12.5px;
    color: var(--zt-dim);
    overflow-wrap: anywhere;
  }
  .acts {
    display: flex;
    gap: 4px;
    flex: none;
  }
  .msg {
    margin-top: 8px;
    font-size: 13px;
  }
  .dim {
    color: var(--zt-dim);
  }
  .cur {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .card pre {
    margin-top: 10px;
    max-height: 260px;
  }
</style>
