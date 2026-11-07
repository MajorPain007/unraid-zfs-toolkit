<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import JobRow from '../replication/JobRow.svelte';
  import JobCard from '../replication/JobCard.svelte';
  import JobEditor from '../replication/JobEditor.svelte';
  import ScheduleCard from '../replication/ScheduleCard.svelte';
  import TestDialog from '../replication/TestDialog.svelte';
  import PreviewDialog from '../replication/PreviewDialog.svelte';
  import DetailsDialog from '../replication/DetailsDialog.svelte';
  import LogDialog from '../replication/LogDialog.svelte';
  import SshKeyDialog from '../replication/SshKeyDialog.svelte';
  import ForceDialog from '../replication/ForceDialog.svelte';
  import { provide } from '../lib/actions';
  import { confirm } from '../lib/dialogs';
  import { app, onRefresh, openDialog, listLayout } from '../lib/state.svelte';
  import { loadJobs, replication, runJobs, stopRun, type SendJob } from '../replication/send.svelte';

  let { opts = {} }: { opts?: Record<string, any> } = $props();

  let enabled = $derived(replication.jobs.filter((j) => j.enabled));
  let running = $derived(!!replication.state?.running);
  let arrayStarted = $derived(app.overview?.array !== false);
  let stopping = $state(false);

  function edit(id: string | null = null) {
    return openDialog(JobEditor, { id }).then(() => loadJobs());
  }

  provide({
    sendEdit: (id?: string | null) => edit(id ?? null),
    sendTest: (job: SendJob) => openDialog(TestDialog, { job }),
    sendPreview: (job: SendJob) => openDialog(PreviewDialog, { job }),
    sendDetails: (job: SendJob) => openDialog(DetailsDialog, { job }),
    sendLog: (run = '', job = '') => openDialog(LogDialog, { run, job }),
    sendKey: (job: SendJob) => openDialog(SshKeyDialog, { job }),
    sendForce: (job: SendJob) => openDialog(ForceDialog, { job }).then(() => loadJobs())
  });

  let wasRunning = false;
  onMount(() => {
    loadJobs().then(() => {
      if (opts.edit) edit(opts.edit === true ? null : String(opts.edit));
    });
    return onRefresh((o) => {
      const r = !!o.replication?.running;
      if (r || wasRunning || r !== !!replication.state?.running) loadJobs();
      wasRunning = r;
    });
  });

  async function runAll(dry: boolean) {
    if (!dry) {
      const yes = await confirm({
        title: 'Replicate now?',
        text: `Runs ${enabled.length === 1 ? 'the enabled job' : 'all ' + enabled.length + ' enabled jobs'} one after the other. This transfers data and can take a while.`,
        confirm: 'Replicate'
      });
      if (!yes) return;
    }
    await runJobs([], { dry });
  }

  async function stop() {
    stopping = true;
    await stopRun();
    stopping = false;
  }
</script>

<ScheduleCard />

<div class="zt-section-head">
  <h2>Jobs</h2>
  {#if replication.jobs.length}
    <div class="zr-head-actions">
      <Button icon="list" small kind="ghost" onclick={() => openDialog(LogDialog, { run: '', job: '' })}>Log</Button>
      {#if running}
        <Button icon="stop" small busy={stopping} onclick={stop}>Stop {replication.state?.dry_run ? 'dry run' : 'replication'}</Button>
      {:else}
        <Button icon="eye" small disabled={!enabled.length} title="Logs what every enabled job would do, without changing anything" onclick={() => runAll(true)}
          >Dry run</Button
        >
        <Button icon="play" small disabled={!enabled.length || !arrayStarted} onclick={() => runAll(false)}>Run all</Button>
      {/if}
      <Button icon="plus" small kind="primary" onclick={() => edit()}>New job</Button>
    </div>
  {/if}
</div>

{#if replication.error}
  <div class="zt-card-note err"><Icon name="alert" /><div>{replication.error}</div></div>
{:else if !replication.loaded}
  <div class="zt-hint">Loading&hellip;</div>
{:else if !replication.jobs.length}
  <div class="zt-empty">
    <Icon name="send" class="big" />
    <h3>Keep a copy somewhere else</h3>
    <p>
      Snapshots live on the same pool as the data - if the pool dies, they go with it. Replication copies datasets with their snapshots to a second pool or
      another server using zfs send and zfs recv, and later only what changed.
    </p>
    <div class="zt-steps">
      <span><b>1</b>Pick a dataset</span>
      <span><b>2</b>Pick a pool or a server</span>
      <span><b>3</b>Test, then run or schedule</span>
    </div>
    <Button icon="plus" kind="primary" onclick={() => edit()}>New replication job</Button>
  </div>
{:else if !listLayout()}
  <div class="zt-grid">
    {#each replication.jobs as job (job.id)}
      <JobCard {job} />
    {/each}
    <button type="button" class="zt-card add" onclick={() => edit()}>
      <Icon name="plus" />New replication job
    </button>
  </div>
{:else}
  <div class="zt-table-wrap zr-list">
    <div class="zr-list-head" aria-hidden="true">
      <span></span><span>Job</span><span>Source → copy</span><span>Last run</span><span class="r">Sent</span><span>Last runs</span>
    </div>
    {#each replication.jobs as job (job.id)}
      <JobRow {job} />
    {/each}
  </div>
{/if}

<style>
  :global(#zt-app) .zr-list {
    --zr-cols: 16px minmax(110px, 1fr) minmax(0, 1.5fr) 150px 72px 110px;
  }
  :global(#zt-app) .zr-list-head {
    display: grid;
    grid-template-columns: var(--zr-cols);
    gap: 12px;
    padding: 8px 142px 8px 12px;
    border-bottom: 1px solid var(--zt-border);
    background: var(--zt-surface-2);
    color: var(--zt-dim);
    font-size: 11.5px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  :global(#zt-app) .zr-list-head .r {
    text-align: right;
  }
  @media (max-width: 760px) {
    :global(#zt-app) .zr-list-head {
      display: none;
    }
  }
  .zr-head-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
</style>
