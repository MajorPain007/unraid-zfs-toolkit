<script lang="ts">
  import { slide } from 'svelte/transition';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import RunStrip from './RunStrip.svelte';
  import { api } from '../lib/api';
  import { act } from '../lib/actions';
  import { app, openMenu } from '../lib/state.svelte';
  import { bytes, dateTime, duration, now, plural, relative } from '../lib/format';
  import {
    deleteJob,
    hostOnly,
    isQueued,
    isRunning,
    jobStatus,
    lastOf,
    needsForce,
    openJobs,
    recentOf,
    replication,
    runJobs,
    setEnabled,
    stopRun,
    type SendJob
  } from './send.svelte';

  interface Snap {
    name: string;
    guid: string;
    creation: number;
  }

  interface Side {
    exists?: boolean;
    snapshots?: number;
    readonly?: boolean;
    checkpoints?: Snap[];
    error?: string;
  }

  interface Inspect {
    source: Side;
    dest: Side;
    base?: Snap;
    pending: number;
    pending_names?: string[];
    dest_newer?: string[];
  }

  let { job }: { job: SendJob } = $props();

  let st = $derived(jobStatus(job));
  let last = $derived(lastOf(job.id));
  let running = $derived(isRunning(job.id));
  let queued = $derived(isQueued(job.id));
  let busyRun = $derived(!!replication.state?.running);
  let progress = $derived(running ? (replication.state?.progress ?? null) : null);
  let runs = $derived(recentOf(job.id).filter((r) => !r.dry && r.status !== 'running'));
  let lastOk = $derived([...runs].reverse().find((r) => r.status === 'ok') ?? null);
  let arrayStarted = $derived(app.overview?.array !== false);
  let open = $derived(openJobs.has(job.id));
  let pct = $derived(progress && progress.total > 0 ? Math.min(100, (progress.bytes / progress.total) * 100) : -1);
  let eta = $derived(progress && progress.total > 0 && progress.rate > 0 ? Math.max(0, (progress.total - progress.bytes) / progress.rate) : -1);
  let working = $state(false);
  let info = $state<Inspect | null>(null);
  let infoError = $state('');
  let infoLoading = $state(false);
  let infoFor = '';

  $effect(() => {
    const key = open ? job.id + '|' + (lastOk?.run ?? '') : '';
    if (key && key !== infoFor) {
      infoFor = key;
      inspect();
    }
  });

  async function inspect() {
    infoLoading = true;
    infoError = '';
    try {
      info = await api<Inspect>('send_inspect', { id: job.id });
    } catch (e) {
      infoError = (e as Error).message;
    } finally {
      infoLoading = false;
    }
  }

  function toggle() {
    if (open) openJobs.delete(job.id);
    else openJobs.add(job.id);
  }

  async function run() {
    working = true;
    await runJobs([job.id]);
    working = false;
  }

  async function stop() {
    working = true;
    await stopRun();
    working = false;
  }

  function menu(e: MouseEvent) {
    openMenu(e.currentTarget as Element, [
      { label: 'Edit', icon: 'edit', onclick: () => act('sendEdit', job.id) },
      { label: 'Test connection', icon: 'link', onclick: () => act('sendTest', job) },
      { label: 'Dry run', icon: 'eye', onclick: () => act('sendPreview', job) },
      { label: 'Source and copy', icon: 'compare', onclick: () => act('sendDetails', job) },
      { label: 'History and logs', icon: 'list', onclick: () => act('sendLog', last?.run ?? '', job.id) },
      job.transport === 'ssh' ? { label: 'SSH key', icon: 'key', onclick: () => act('sendKey', job) } : null,
      '-',
      { label: job.enabled ? 'Disable' : 'Enable', icon: 'power', onclick: () => setEnabled(job, !job.enabled) },
      { label: 'Delete…', icon: 'trash', danger: true, disabled: running || queued, onclick: () => deleteJob(job) }
    ]);
  }
</script>

<div class="zr-row" class:open class:zr-off={!job.enabled && !running}>
  <div class="zr-row-line">
    <button type="button" class="zr-row-main" aria-expanded={open} onclick={toggle}>
      <span class="zr-chev"><Icon name="right" /></span>
      <span class="zr-name">
        <span class="zt-dot {st[0]}" title={st[1]}></span>
        <b title={job.name}>{job.name}</b>
      </span>
      <span class="zr-route" title="{job.source} → {job.transport === 'ssh' ? job.ssh_host + ':' : ''}{job.dest}">
        <span class="zr-ds">{job.source}</span>
        <Icon name="right" />
        <Icon name={job.transport === 'ssh' ? 'server' : 'hdd'} />
        <span class="zr-ds">{job.transport === 'ssh' ? hostOnly(job.ssh_host) + ': ' : ''}{job.dest}</span>
      </span>
      {#if running && progress}
        <span class="zr-prog">
          <span class="zt-progress-track" class:indeterminate={pct < 0}><span class="zt-progress-bar" style:width="{pct < 0 ? 35 : pct}%"></span></span>
          <small>
            {#if progress.phase === 'sending'}
              {bytes(progress.bytes)}{progress.total > 0 ? ' of ~' + bytes(progress.total) : ''}{progress.rate > 0 ? ' · ' + bytes(progress.rate) + '/s' : ''}{eta >= 0
                ? ' · ' + duration(eta) + ' left'
                : ''}
            {:else if progress.phase === 'measuring'}
              Measuring&hellip;
            {:else}
              Checking&hellip; {duration(now() - progress.started)}
            {/if}
          </small>
        </span>
      {:else}
        <span class="zr-meta">
          <span class="zr-last" title={last ? dateTime(last.ended || last.started) : ''}>
            <span class="zr-cap"><Icon name="clock" /></span>
            {#if queued}
              waiting
            {:else if last}
              {relative(last.ended || last.started)} <span class="zr-res zr-res-{st[0]}">· {st[1].toLowerCase()}</span>
            {:else}
              {job.enabled ? 'never run' : 'disabled'}
            {/if}
          </span>
          <span class="zr-num" title={last && last.status === 'ok' ? (last.full ? 'Full copy' : 'Incremental') : ''}>
            <span class="zr-cap"><Icon name="send" /></span>{last && last.status === 'ok' ? bytes(last.bytes) : '-'}
          </span>
        </span>
      {/if}
      <span class="zr-strip"><RunStrip {runs} /></span>
    </button>
    <span class="zr-act">
      {#if running}
        <Button icon="stop" small busy={working} onclick={stop}>Stop</Button>
      {:else}
        <Button
          icon="play"
          small
          busy={working}
          disabled={busyRun || !arrayStarted}
          title={busyRun ? 'Another replication is running' : arrayStarted ? 'Run this job now' : 'The array is stopped'}
          onclick={run}><span class="zr-runlabel">Run now</span></Button
        >
      {/if}
    </span>
    <Button icon="more" small kind="ghost" title="More" onclick={menu} />
  </div>

  {#if open}
    <div class="zr-row-body" transition:slide={{ duration: 160 }}>
      {#if last && last.status === 'error' && last.error}
        <div class="zt-card-note err zr-gapb">
          <Icon name="alert" />
          <div>
            {last.error}
            <span class="zr-note-links">
              <button type="button" class="zt-link" onclick={() => act('sendLog', last.run, job.id)}>Show log</button>
              {#if needsForce(last.error) && !job.allow_rollback}
                <button type="button" class="zt-link" onclick={() => act('sendForce', job)}>Roll back the copy and retry</button>
              {:else if job.transport === 'ssh' && /key|log in|host key/i.test(last.error)}
                <button type="button" class="zt-link" onclick={() => act('sendTest', job)}>Test the connection</button>
              {/if}
            </span>
          </div>
        </div>
      {/if}

      <div class="zr-facts">
        <span class="zr-k">Copy is from</span>
        <div class="zr-v">
          {#if info?.base}
            <span title="@{info.base.name}">{dateTime(info.base.creation)} <small>({relative(info.base.creation)})</small></span>
          {:else if lastOk}
            {dateTime(lastOk.started)} <small>({relative(lastOk.started)})</small>
          {:else}
            no copy yet
          {/if}
        </div>
        <span class="zr-k">Waiting</span>
        <div class="zr-v">
          {#if infoLoading && !info}
            <span class="zt-dim">checking&hellip;</span>
          {:else if infoError}
            <span class="zt-warn-text">{infoError}</span>
          {:else if info}
            {#if info.dest.error}
              <span class="zt-warn-text">{info.dest.error}</span>
            {:else if info.dest.exists === false}
              everything ({plural(info.pending, 'snapshot')}) - the next run sends a full copy
            {:else if !info.base}
              <span class="zt-err-text">no snapshot in common - the copy cannot be continued</span>
            {:else if info.pending}
              {plural(info.pending, 'newer snapshot')}{info.pending_names?.length ? ': ' + info.pending_names.slice(0, 4).join(', ') + (info.pending > 4 ? ', …' : '') : ''}
            {:else}
              nothing, the copy is up to date
            {/if}
          {/if}
        </div>
        {#if info?.dest_newer?.length}
          <span class="zr-k">On the copy only</span>
          <div class="zr-v zt-warn-text">{info.dest_newer.join(', ')} - a run needs Force to roll these back</div>
        {/if}
        <span class="zr-k">Checkpoints</span>
        <div class="zr-v">
          {#if info}
            {plural(info.source.checkpoints?.length ?? 0, 'here', 'here')}{#if info.dest.exists}, {info.dest.checkpoints?.length ?? 0} on the copy{/if}
            {#if info.source.checkpoints?.length}<small> · newest {relative(info.source.checkpoints[info.source.checkpoints.length - 1].creation)}</small>{/if}
          {:else}
            -
          {/if}
        </div>
        <span class="zr-k">Last copy</span>
        <div class="zr-v">
          {#if lastOk}
            {bytes(lastOk.bytes)} <small>· {lastOk.full ? 'full' : 'incremental'} · {duration(lastOk.ended - lastOk.started)}</small>
          {:else}
            -
          {/if}
        </div>
        <span class="zr-k">Options</span>
        <div class="zr-v zr-pills">
          <span class="zt-pill">{job.transport === 'ssh' ? 'SSH port ' + job.ssh_port : 'local pool'}</span>
          {#if job.recursive}<span class="zt-pill" title="zfs send -R">children</span>{/if}
          {#if job.readonly}<span class="zt-pill" title="readonly=on on the copy">read-only copy</span>{/if}
          {#if job.allow_rollback}<span class="zt-pill warn" title="zfs recv -F">force</span>{/if}
          {#if job.raw}<span class="zt-pill" title="zfs send -w">raw</span>{/if}
          {#if job.compressed}<span class="zt-pill" title="zfs send -L -e -c">compressed</span>{/if}
          {#if job.keep_dest > 0}<span class="zt-pill">keeps {job.keep_dest} on the copy</span>{/if}
          {#if job.bwlimit > 0}<span class="zt-pill">{job.bwlimit} MB/s</span>{/if}
          {#if !job.enabled}<span class="zt-pill warn">not scheduled</span>{/if}
        </div>
      </div>

      <div class="zr-links">
        <button type="button" class="zt-link" onclick={() => act('sendEdit', job.id)}>Edit</button>
        <button type="button" class="zt-link" onclick={() => act('sendTest', job)}>Test connection</button>
        <button type="button" class="zt-link" onclick={() => act('sendPreview', job)}>Dry run</button>
        <button type="button" class="zt-link" onclick={() => act('sendLog', last?.run ?? '', job.id)}>Log</button>
        <button type="button" class="zt-link" onclick={() => act('sendDetails', job)}>Source and copy</button>
        <button type="button" class="zt-link" disabled={infoLoading} onclick={inspect}>Refresh</button>
      </div>
    </div>
  {/if}
</div>

<style>
  :global(#zt-app) .zr-row {
    border-bottom: 1px solid var(--zt-border);
  }
  :global(#zt-app) .zr-row:last-child {
    border-bottom: 0;
  }
  :global(#zt-app) .zr-row-line {
    display: flex;
    align-items: center;
    gap: 4px;
    padding-right: 8px;
  }
  :global(#zt-app) .zr-row-line:hover {
    background: var(--zt-surface-2);
  }
  :global(#zt-app) .zr-row.open > .zr-row-line {
    background: var(--zt-select-soft);
  }
  :global(#zt-app) button.zr-row-main {
    flex: 1 1 auto;
    min-width: 0;
    display: grid;
    grid-template-columns: var(--zr-cols);
    align-items: center;
    gap: 12px;
    padding: 10px 6px 10px 12px;
    border: 0;
    background: transparent;
    color: var(--zt-text);
    font: inherit;
    font-size: 13px;
    text-align: left;
    cursor: pointer;
  }
  :global(#zt-app) .zr-chev {
    display: grid;
    place-items: center;
    color: var(--zt-dim);
    transition: transform 0.15s;
  }
  :global(#zt-app) .zr-row.open .zr-chev {
    transform: rotate(90deg);
  }
  :global(#zt-app) .zr-name {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  :global(#zt-app) .zr-name .zt-dot {
    margin-top: 0;
    width: 8px;
    height: 8px;
  }
  :global(#zt-app) .zr-name b {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 600;
  }
  :global(#zt-app) .zr-row.zr-off .zr-name b {
    color: var(--zt-dim);
  }
  :global(#zt-app) .zr-route {
    display: flex;
    align-items: center;
    gap: 5px;
    min-width: 0;
    font-family: var(--zt-mono);
    font-size: 12px;
    color: var(--zt-dim);
  }
  :global(#zt-app) .zr-route :global(svg.zt-i) {
    flex: none;
    width: 12px;
    height: 12px;
  }
  :global(#zt-app) .zr-ds {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  :global(#zt-app) .zr-meta {
    display: contents;
  }
  :global(#zt-app) .zr-last {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  :global(#zt-app) .zr-res {
    color: var(--zt-dim);
  }
  :global(#zt-app) .zr-res-err {
    color: var(--zt-err);
  }
  :global(#zt-app) .zr-res-warn {
    color: var(--zt-warn);
  }
  :global(#zt-app) .zr-num {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 4px;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  :global(#zt-app) .zr-cap {
    display: none;
  }
  :global(#zt-app) .zr-prog {
    grid-column: span 2;
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }
  :global(#zt-app) .zr-prog small {
    color: var(--zt-dim);
    font-size: 11.5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  :global(#zt-app) .zr-prog .zt-progress-track {
    height: 6px;
  }
  :global(#zt-app) .zr-act {
    flex: none;
    width: 92px;
    display: flex;
    justify-content: flex-end;
  }
  :global(#zt-app) .zr-row-body {
    padding: 12px 14px 16px 40px;
    border-top: 1px solid var(--zt-border);
    background: var(--zt-surface);
  }
  :global(#zt-app) .zr-gapb {
    margin: 0 0 12px;
  }
  :global(#zt-app) .zr-note-links {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 4px 12px;
    margin-left: 6px;
  }
  :global(#zt-app) .zr-facts {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 6px 16px;
    margin: 0 0 12px;
    font-size: 13px;
  }
  :global(#zt-app) .zr-facts .zr-k {
    color: var(--zt-dim);
  }
  :global(#zt-app) .zr-facts .zr-v {
    margin: 0;
    overflow-wrap: anywhere;
  }
  :global(#zt-app) .zr-facts small {
    color: var(--zt-dim);
  }
  :global(#zt-app) .zr-pills {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  :global(#zt-app) .zr-pills .zt-pill {
    font-size: 11px;
    padding: 1px 8px;
  }
  :global(#zt-app) .zr-links {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 16px;
    font-size: 12.5px;
  }
  @media (max-width: 760px) {
    :global(#zt-app) button.zr-row-main {
      grid-template-columns: 16px minmax(0, 1fr);
      grid-template-areas:
        'chev name'
        '. route'
        '. meta';
      row-gap: 3px;
      column-gap: 8px;
    }
    :global(#zt-app) .zr-chev {
      grid-area: chev;
    }
    :global(#zt-app) .zr-name {
      grid-area: name;
    }
    :global(#zt-app) .zr-route {
      grid-area: route;
      flex-wrap: wrap;
      row-gap: 0;
    }
    :global(#zt-app) .zr-ds {
      white-space: normal;
      overflow-wrap: anywhere;
    }
    :global(#zt-app) .zr-meta {
      grid-area: meta;
      display: flex;
      flex-wrap: wrap;
      gap: 2px 14px;
      font-size: 12px;
      color: var(--zt-dim);
    }
    :global(#zt-app) .zr-prog {
      grid-area: meta;
    }
    :global(#zt-app) .zr-cap {
      display: inline-flex;
      margin-right: 3px;
    }
    :global(#zt-app) .zr-cap :global(svg.zt-i) {
      width: 12px;
      height: 12px;
      color: var(--zt-faint);
    }
    :global(#zt-app) .zr-last {
      display: flex;
      align-items: center;
    }
    :global(#zt-app) .zr-strip {
      display: none;
    }
    :global(#zt-app) .zr-act {
      width: auto;
    }
    :global(#zt-app) .zr-runlabel {
      display: none;
    }
    :global(#zt-app) .zr-row-body {
      padding: 10px 10px 14px;
    }
    :global(#zt-app) .zr-facts {
      grid-template-columns: 1fr;
      gap: 1px;
    }
    :global(#zt-app) .zr-facts .zr-v {
      margin-bottom: 6px;
    }
  }
</style>
