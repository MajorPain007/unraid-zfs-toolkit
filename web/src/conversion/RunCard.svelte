<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { api } from '../lib/api';
  import { confirm } from '../lib/dialogs';
  import { busy, fail, refresh, toast } from '../lib/state.svelte';
  import { bytes, duration, now, plural, relative } from '../lib/format';
  import { PHASE_LABEL, STATUS_LABEL, type ConvItem, type ConvStatus } from './types';

  let { st, onlog, ondismiss }: { st: ConvStatus; onlog: (file?: string) => void; ondismiss?: () => void } = $props();

  let stopping = $state({ busy: false });
  let tick = $state(now());

  $effect(() => {
    if (!st.running) return;
    const t = setInterval(() => (tick = now()), 1000);
    return () => clearInterval(t);
  });

  let items = $derived(st.items ?? []);
  let doneCount = $derived(items.filter((i) => i.state === 'done' || i.state === 'failed' || i.state === 'skipped').length);
  let cur = $derived(items.find((i) => i.state === 'running') ?? null);
  let pill = $derived(STATUS_LABEL[st.status] ?? STATUS_LABEL.idle);
  let res = $derived(st.result ?? null);
  let elapsed = $derived(st.started ? (st.running ? tick : st.finished || tick) - st.started : 0);

  function pct(i: ConvItem): number | null {
    if (i.phase === 'validating' || i.phase === 'cleanup') return 1;
    if (i.phase !== 'copying' || !i.bytes || i.bytes <= 0) return null;
    return Math.max(0, Math.min(1, i.done / i.bytes));
  }

  function eta(i: ConvItem): string {
    if (!i.rate || i.rate <= 0 || i.bytes <= 0) return '';
    const left = (i.bytes - i.done) / i.rate;
    return left > 1 ? duration(left) + ' left' : '';
  }

  function icon(i: ConvItem): string {
    if (i.state === 'done') return 'check';
    if (i.state === 'failed') return 'x';
    if (i.state === 'running') return 'loader';
    if (i.state === 'skipped' || i.state === 'cancelled') return 'right';
    return 'clock';
  }

  async function stop() {
    const yes = await confirm({
      title: 'Stop the conversion?',
      text: 'The folder being converted is put back the way it was, and the containers and VMs it stopped are started again. Folders that are already done stay datasets.',
      confirm: 'Stop',
      danger: true
    });
    if (!yes) return;
    try {
      await busy(stopping, api('convert_stop'));
      toast('Stopping - putting the current folder back...', 'info');
      setTimeout(refresh, 1200);
    } catch (e) {
      fail(e);
    }
  }
</script>

<div class="zt-card cv-run" class:is-running={st.running} class:is-failed={st.status === 'error'}>
  <div class="zt-card-head">
    <span class="zt-dot {st.running ? 'run' : pill[1] || 'idle'}"></span>
    <div class="zt-card-title">
      <h3>
        {#if st.running}{st.dry_run ? 'Dry run in progress' : 'Converting folders'}{:else}{st.dry_run ? 'Last dry run' : 'Last conversion'}{/if}
      </h3>
      <div class="zt-meta">
        {#if st.started}{st.running ? 'Started' : 'Ran'} {relative(st.started)}{/if}
        {#if st.source === 'auto'} · by the schedule{/if}
        {#if elapsed > 0} · {duration(elapsed)}{/if}
        {#if !st.running}<span class="zt-pill {pill[1]} cv-pill">{st.dry_run && st.status === 'completed' ? 'Simulated' : pill[0]}</span>{/if}
      </div>
    </div>
    <div class="zt-card-actions">
      <Button icon="list" small onclick={() => onlog(st.log_file)}>Log</Button>
      {#if st.running}
        <Button icon="stop" small kind="danger" busy={stopping.busy} onclick={stop}>Stop</Button>
      {:else if ondismiss}
        <Button icon="x" small kind="ghost" title="Hide" onclick={ondismiss} />
      {/if}
    </div>
  </div>

  {#if st.running}
    <div class="zt-progress">
      <div class="zt-progress-track" class:indeterminate={!items.length} role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={items.length ? Math.round((doneCount / items.length) * 100) : undefined} aria-label="Folders done">
        <div class="zt-progress-bar" style:width={items.length ? ((doneCount + (cur ? pct(cur) ?? 0 : 0)) / items.length) * 100 + '%' : null}></div>
      </div>
      <div class="zt-progress-text">
        <span><b>{PHASE_LABEL[st.phase ?? ''] ?? 'Working'}</b>{#if st.current_folder} · {st.current_folder}{/if}</span>
        {#if items.length}<span>{doneCount} of {plural(items.length, 'folder')}</span>{/if}
      </div>
    </div>
  {/if}

  {#if res && !st.running}
    <div class="cv-result">{res.message}</div>
  {/if}

  {#if st.stopped?.length}
    <div class="cv-stopped">
      <span class="cv-label">{st.running ? (st.dry_run ? 'Would stop' : 'Stopped for the copy') : st.dry_run ? 'Would have stopped' : 'Stopped and restarted'}</span>
      {#each st.stopped as s (s.kind + s.name)}<span class="zt-chip"><Icon name={s.kind === 'vm' ? 'server' : 'box'} />{s.name}</span>{/each}
    </div>
  {/if}

  {#if items.length}
    <ul class="cv-items">
      {#each items as i (i.path)}
        {@const p = pct(i)}
        <li class="cv-item {i.state}">
          <Icon name={icon(i)} class={i.state === 'running' ? 'zt-spin' : ''} />
          <div class="cv-item-main">
            <div class="cv-item-line">
              <b>{i.name}</b>
              {#if i.as}<span class="cv-as">as {i.as}</span>{/if}
              <span class="cv-item-size">{i.size > 0 ? bytes(i.size) : i.bytes > 0 ? bytes(i.bytes) : ''}</span>
            </div>
            {#if i.state === 'running'}
              <div class="cv-item-phase">
                {PHASE_LABEL[i.phase] ?? i.phase}
                {#if i.phase === 'copying' && i.done > 0} · {bytes(i.done)}{i.bytes > 0 ? ' of ' + bytes(i.bytes) : ''}{#if i.rate > 0} · {bytes(i.rate)}/s{/if}{#if eta(i)} · {eta(i)}{/if}{/if}
              </div>
              {#if p !== null}
                <div class="cv-mini"><div style:width="{p * 100}%"></div></div>
              {/if}
            {:else if i.message}
              <div class="cv-item-msg">{i.message}</div>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .cv-run h3 {
    margin: 0;
  }
  .cv-pill {
    margin-left: 6px;
    vertical-align: 1px;
  }
  .cv-result {
    margin-top: 12px;
    font-size: 13.5px;
  }
  .cv-stopped {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin-top: 12px;
  }
  .cv-label {
    font-size: 12px;
    color: var(--zt-dim);
    margin-right: 2px;
  }
  .cv-items {
    list-style: none;
    margin: 14px 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 340px;
    overflow: auto;
  }
  .cv-item {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    padding: 7px 8px;
    border-radius: var(--zt-radius-sm);
    color: var(--zt-dim);
  }
  .cv-item.running {
    background: var(--zt-info-soft);
    color: var(--zt-text);
  }
  .cv-item.done :global(svg.zt-i) {
    color: var(--zt-ok);
  }
  .cv-item.failed :global(svg.zt-i) {
    color: var(--zt-err);
  }
  .cv-item.running :global(svg.zt-i) {
    color: var(--zt-info);
  }
  .cv-item :global(svg.zt-i) {
    margin-top: 2px;
    flex: none;
  }
  .cv-item-main {
    flex: 1 1 auto;
    min-width: 0;
  }
  .cv-item-line {
    display: flex;
    gap: 8px;
    align-items: baseline;
    min-width: 0;
  }
  .cv-item-line b {
    color: var(--zt-text);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cv-as {
    font-size: 12px;
    font-family: var(--zt-mono);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cv-item-size {
    margin-left: auto;
    font-size: 12px;
    white-space: nowrap;
  }
  .cv-item-phase,
  .cv-item-msg {
    font-size: 12px;
    margin-top: 2px;
    overflow-wrap: anywhere;
  }
  .cv-item.failed .cv-item-msg {
    color: var(--zt-err);
  }
  .cv-mini {
    height: 4px;
    margin-top: 6px;
    border-radius: 2px;
    background: var(--zt-surface-3);
    overflow: hidden;
  }
  .cv-mini div {
    height: 100%;
    background: var(--zt-info);
    transition: width 0.8s ease;
  }
</style>
