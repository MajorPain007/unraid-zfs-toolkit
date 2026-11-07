<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { bytes, dateTime, plural, relative } from '../lib/format';
  import { hostOnly, type SendJob } from './send.svelte';

  interface Snap {
    name: string;
    guid: string;
    creation: number;
  }

  interface Side {
    name: string;
    exists?: boolean;
    used?: number;
    avail?: number;
    readonly?: boolean;
    snapshots?: number;
    newest?: Snap;
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

  let { job, close }: { job: SendJob; close: (r?: unknown) => void } = $props();

  let info = $state<Inspect | null>(null);
  let error = $state('');
  let loading = $state(true);

  async function load() {
    loading = true;
    error = '';
    try {
      info = await api<Inspect>('send_inspect', { id: job.id });
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

{#snippet side(title: string, icon: string, s: Side, where: string)}
  <div class="zr-d-side">
    <div class="zr-d-title"><Icon name={icon} /><b>{title}</b><span class="zt-hint">{where}</span></div>
    <div class="zt-mono zr-d-name" title={s.name}>{s.name}</div>
    {#if s.exists === false}
      <div class="zt-hint">{s.error ? s.error : 'Does not exist yet - the next run sends a full copy.'}</div>
    {:else}
      <dl>
        <dt>Used</dt><dd>{bytes(s.used)}{s.avail !== undefined ? ' · ' + bytes(s.avail) + ' free' : ''}</dd>
        <dt>Snapshots</dt><dd>{s.snapshots ?? 0}</dd>
        <dt>Newest</dt><dd title={s.newest?.name}>{s.newest ? relative(s.newest.creation) : '-'}</dd>
        {#if s.readonly !== undefined}<dt>Read-only</dt><dd>{s.readonly ? 'yes' : 'no'}</dd>{/if}
      </dl>
      <div class="zr-d-cps">
        <span class="zt-hint">Checkpoints of this job</span>
        {#each s.checkpoints ?? [] as c (c.name)}
          <span class="zt-chip" title={c.name}><Icon name="clock" />{dateTime(c.creation)}</span>
        {:else}
          <span class="zt-hint">none</span>
        {/each}
      </div>
    {/if}
  </div>
{/snippet}

<Dialog title="Source and copy - {job.name}" icon="compare" wide onclose={() => close()}>
  {#if loading && !info}
    <div class="zt-hint"><Icon name="loader" class="zt-spin" /> Looking at both sides&hellip;</div>
  {:else if error}
    <div class="zt-card-note err"><Icon name="alert" /><div>{error}</div></div>
  {:else if info}
    <div class="zr-d-grid">
      {@render side('Source', 'database', info.source, 'this server')}
      <div class="zr-d-arrow"><Icon name="right" /></div>
      {@render side('Copy', job.transport === 'ssh' ? 'server' : 'hdd', info.dest, job.transport === 'ssh' ? hostOnly(job.ssh_host) : 'this server')}
    </div>
    <div class="zt-callout zr-d-sum" class:warn={info.dest.exists && !info.base}>
      <Icon name={info.base ? 'check' : 'info'} />
      <div>
        {#if info.base}
          The copy is up to date with <b>@{info.base.name}</b> from {dateTime(info.base.creation)} ({relative(info.base.creation)}).
          {#if info.pending}{plural(info.pending, 'newer snapshot')} here {info.pending === 1 ? 'is' : 'are'} not copied yet{info.pending_names?.length ? ': ' + info.pending_names.join(', ') : ''}.{:else}Nothing newer is waiting.{/if}
          {#if info.dest_newer?.length}
            <div class="zt-warn-text">The copy has {plural(info.dest_newer.length, 'snapshot')} the source does not: {info.dest_newer.join(', ')}. A run needs Force to roll those back.</div>
          {/if}
        {:else if info.dest.exists}
          The two sides share no snapshot. The next run cannot continue this copy.
        {:else}
          {plural(info.pending, 'snapshot')} will go over with the first run.
        {/if}
      </div>
    </div>
  {/if}
  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button icon="refresh" busy={loading} onclick={load}>Refresh</Button>
    <Button kind="primary" onclick={() => close()}>Close</Button>
  {/snippet}
</Dialog>

<style>
  .zr-d-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    gap: 12px;
    align-items: stretch;
  }
  .zr-d-arrow {
    display: grid;
    place-items: center;
    color: var(--zt-dim);
  }
  .zr-d-side {
    border: 1px solid var(--zt-border);
    border-radius: var(--zt-radius);
    background: var(--zt-surface);
    padding: 12px 14px;
    min-width: 0;
  }
  .zr-d-title {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-bottom: 6px;
  }
  .zr-d-title .zt-hint {
    margin-left: auto;
  }
  .zr-d-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    margin-bottom: 8px;
  }
  dl {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 3px 12px;
    margin: 0 0 10px;
    font-size: 13px;
  }
  dt {
    color: var(--zt-dim);
  }
  dd {
    margin: 0;
  }
  .zr-d-cps {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
    align-items: center;
  }
  .zr-d-cps .zt-hint {
    width: 100%;
  }
  .zr-d-sum {
    margin-top: 14px;
  }
  @media (max-width: 640px) {
    .zr-d-grid {
      grid-template-columns: 1fr;
    }
    .zr-d-arrow {
      transform: rotate(90deg);
    }
  }
</style>
