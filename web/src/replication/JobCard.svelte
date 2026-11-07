<script lang="ts">
  import { slide } from 'svelte/transition';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { act } from '../lib/actions';
  import { app, openMenu } from '../lib/state.svelte';
  import { bytes, dateTime, duration, now, relative } from '../lib/format';
  import {
    deleteJob,
    hostOnly,
    isQueued,
    isRunning,
    jobStatus,
    lastOf,
    needsForce,
    recentOf,
    replication,
    resultText,
    runJobs,
    setEnabled,
    stopRun,
    type SendJob,
    type SendResult
  } from './send.svelte';

  let { job }: { job: SendJob } = $props();

  let st = $derived(jobStatus(job));
  let last = $derived(lastOf(job.id));
  let running = $derived(isRunning(job.id));
  let queued = $derived(isQueued(job.id));
  let busyRun = $derived(!!replication.state?.running);
  let progress = $derived(running ? (replication.state?.progress ?? null) : null);
  let runs = $derived(recentOf(job.id).filter((r) => !r.dry && r.status !== 'running').slice(-30));
  let lastOk = $derived([...recentOf(job.id)].reverse().find((r) => r.status === 'ok' && !r.dry) ?? null);
  let arrayStarted = $derived(app.overview?.array !== false);
  let working = $state(false);
  let pct = $derived(progress && progress.total > 0 ? Math.min(100, (progress.bytes / progress.total) * 100) : -1);
  let eta = $derived(progress && progress.total > 0 && progress.rate > 0 ? Math.max(0, (progress.total - progress.bytes) / progress.rate) : -1);
  let maxBytes = $derived(Math.max(0, ...runs.map((r) => r.bytes || 0)));

  async function run(force = false) {
    working = true;
    await runJobs([job.id], { force });
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
      { label: 'Preview (dry run)', icon: 'eye', onclick: () => act('sendPreview', job) },
      { label: 'Source and copy', icon: 'compare', onclick: () => act('sendDetails', job) },
      { label: 'History and logs', icon: 'list', onclick: () => act('sendLog', last?.run ?? '', job.id) },
      job.transport === 'ssh' ? { label: 'SSH key', icon: 'key', onclick: () => act('sendKey', job) } : null,
      '-',
      { label: job.enabled ? 'Disable' : 'Enable', icon: 'power', onclick: () => setEnabled(job, !job.enabled) },
      { label: 'Delete', icon: 'trash', danger: true, disabled: running || queued, onclick: () => deleteJob(job) }
    ]);
  }

  function barHeight(b: number): string {
    if (!maxBytes || !b) return '6px';
    return (6 + 16 * Math.sqrt(b / maxBytes)).toFixed(1) + 'px';
  }

  function runTitle(r: SendResult): string {
    return dateTime(r.started) + ' - ' + resultText(r) + (r.error ? ' - ' + r.error : '');
  }
</script>

<div class="zt-card" class:is-running={running} class:is-failed={st[0] === 'err'} class:is-disabled={!job.enabled && !running}>
  <div class="zt-card-head">
    <span class="zt-dot {st[0]}" title={st[1]}></span>
    <div class="zt-card-title">
      <h3 title={job.name}>
        <button type="button" class="zr-name" onclick={() => act('sendEdit', job.id)}>{job.name}</button>
      </h3>
    </div>
    <div class="zt-card-actions">
      {#if running}
        <Button icon="stop" small busy={working} onclick={stop}>Stop</Button>
      {:else if queued}
        <Button icon="clock" small disabled title="Waits for the job before it">Waiting</Button>
      {:else}
        <Button
          icon="play"
          small
          kind="primary"
          busy={working}
          disabled={busyRun || !arrayStarted}
          title={busyRun ? 'Another replication is running' : arrayStarted ? '' : 'The array is stopped'}
          onclick={() => run()}>Run now</Button
        >
      {/if}
      <Button icon="more" small kind="ghost" title="More" onclick={menu} />
    </div>
  </div>
  <div class="zt-meta zr-route">
        <span class="zt-chip" title={job.source}><Icon name="database" />{job.source}</span>
        <span class="zr-to"><Icon name="right" /><span class="zt-chip" title={job.transport === 'ssh' ? job.ssh_host + ':' + job.dest : job.dest}>
          <Icon name={job.transport === 'ssh' ? 'server' : 'hdd'} />{job.transport === 'ssh' ? hostOnly(job.ssh_host) + ': ' : ''}{job.dest}
        </span></span>
      </div>

  {#if running && progress}
    <div class="zt-progress" transition:slide={{ duration: 180 }}>
      <div class="zt-progress-track" class:indeterminate={pct < 0}>
        <div class="zt-progress-bar" style:width="{pct < 0 ? 35 : pct}%"></div>
      </div>
      <div class="zt-progress-text">
        <span>
          {#if progress.phase === 'sending'}
            <b>{bytes(progress.bytes)}</b>{progress.total > 0 ? ' of ~' + bytes(progress.total) : ''}{progress.rate > 0 ? ' · ' + bytes(progress.rate) + '/s' : ''}
          {:else if progress.phase === 'measuring'}
            Measuring what to send&hellip;
          {:else}
            Checking source and destination&hellip;
          {/if}
        </span>
        <span>{eta >= 0 ? duration(eta) + ' left' : duration(now() - progress.started)}</span>
      </div>
    </div>
  {:else}
    <div transition:slide={{ duration: 180 }}>
      <div class="zt-facts">
        <div class="zt-fact">
          <div class="zt-label">Last run</div>
          <div class="zt-value">
            {#if last}
              <span title={dateTime(last.ended || last.started)}>{relative(last.ended || last.started)} <small>· {st[1].toLowerCase()}</small></span>
            {:else}
              Never
            {/if}
          </div>
        </div>
        <div class="zt-fact">
          <div class="zt-label">Copy is from</div>
          <div class="zt-value">
            {#if lastOk}
              <span title={'Last good copy ' + dateTime(lastOk.ended) + (lastOk.checkpoint ? ' (@' + lastOk.checkpoint + ')' : '')}
                >{relative(lastOk.started)}</span
              >
            {:else}
              -
            {/if}
          </div>
        </div>
        <div class="zt-fact">
          <div class="zt-label">Sent</div>
          <div class="zt-value">
            {#if last && last.status === 'ok'}
              {bytes(last.bytes)} <small>· {last.full ? 'full copy' : 'incremental'}</small>
            {:else}
              -
            {/if}
          </div>
        </div>
        <div class="zt-fact">
          <div class="zt-label">Options</div>
          <div class="zt-value zr-flags">
            {#if job.recursive}<span class="zt-pill" title="zfs send -R: child datasets come along">children</span>{/if}
            {#if job.allow_rollback}<span class="zt-pill warn" title="zfs recv -F: the destination can be rolled back">force</span>{/if}
            {#if job.raw}<span class="zt-pill" title="zfs send -w">raw</span>{/if}
            {#if job.readonly}<span class="zt-pill" title="The copy is read-only">read-only</span>{/if}
            {#if job.bwlimit > 0}<span class="zt-pill" title="Bandwidth limit">{job.bwlimit} MB/s</span>{/if}
            {#if !job.recursive && !job.allow_rollback && !job.raw && !job.readonly && !(job.bwlimit > 0)}<small>defaults</small>{/if}
          </div>
        </div>
      </div>
      {#if last && last.status === 'error' && last.error && job.enabled}
        <div class="zt-card-note err">
          <Icon name="alert" />
          <div>
            {last.error}
            <span class="zr-note-actions">
              <button type="button" class="zt-link" onclick={() => act('sendLog', last.run, job.id)}>Show log</button>
              {#if needsForce(last.error) && !job.allow_rollback}
                <button type="button" class="zt-link" onclick={() => act('sendForce', job)}>Roll back the copy and retry</button>
              {/if}
            </span>
          </div>
        </div>
      {/if}
    </div>
  {/if}

  <div class="zt-strip zr-strip" role="group" aria-label="Recent runs">
    {#each { length: 30 - runs.length } as _, i (i)}
      <span class="st-empty"></span>
    {/each}
    {#each runs as r (r.run)}
      <button
        type="button"
        class="st-{r.status}"
        style:height={barHeight(r.bytes)}
        title={runTitle(r)}
        aria-label={runTitle(r)}
        onclick={() => act('sendLog', r.run, job.id)}
      ></button>
    {/each}
  </div>
</div>

<style>
  :global(#zt-app) .zr-name {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font: inherit;
    color: inherit;
    background: none;
    border: 0;
    padding: 0;
    margin: 0;
    cursor: pointer;
  }
  :global(#zt-app) .zr-name:hover {
    color: var(--zt-accent-text);
  }
  .zr-route {
    margin: 8px 0 0 22px;
  }
  .zr-route .zt-chip {
    min-width: 0;
    max-width: 100%;
  }
  .zr-to {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    max-width: 100%;
  }
  .zr-to > :global(svg.zt-i) {
    flex: none;
    width: 13px;
    height: 13px;
  }
  .zr-flags {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
    white-space: normal;
  }
  .zr-flags .zt-pill {
    font-size: 11px;
    padding: 1px 7px;
  }
  .zr-note-actions {
    display: inline-flex;
    gap: 12px;
    margin-left: 6px;
  }
  :global(#zt-app) .zr-strip button {
    flex: 1 1 0;
    max-width: 10px;
    min-height: 6px;
    border-radius: 2px;
    background: var(--zt-ok);
    opacity: 0.85;
    border: 0;
    padding: 0;
    margin: 0;
    cursor: pointer;
  }
  :global(#zt-app) .zr-strip button:hover,
  :global(#zt-app) .zr-strip button:focus-visible {
    opacity: 1;
    filter: brightness(1.15);
  }
  :global(#zt-app) .zr-strip button.st-error {
    background: var(--zt-err);
  }
  :global(#zt-app) .zr-strip button.st-cancelled {
    background: var(--zt-faint);
  }
</style>
