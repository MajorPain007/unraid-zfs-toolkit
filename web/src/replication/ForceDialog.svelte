<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Checks from './Checks.svelte';
  import { api } from '../lib/api';
  import { runJobs, target, type SendJob, type SendPlan } from './send.svelte';

  let { job, close }: { job: SendJob; close: (r?: unknown) => void } = $props();

  let plan = $state<SendPlan | null>(null);
  let error = $state('');
  let loading = $state(true);
  let starting = $state(false);

  onMount(async () => {
    try {
      const r = await api<{ plan: SendPlan }>('send_test', { job: { ...job, allow_rollback: true } });
      plan = r.plan;
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loading = false;
    }
  });

  async function go() {
    starting = true;
    if (await runJobs([job.id], { force: true })) close(true);
    starting = false;
  }
</script>

<Dialog title="Roll back the copy and retry" icon="restore" wide onclose={() => close()}>
  <p>
    One run with Force (zfs recv -F) for <b>{job.name}</b>: the copy at <span class="zt-mono">{target(job)}</span> is rolled back to the last snapshot both sides
    have, then the run continues as usual. The job setting stays as it is.
  </p>
  {#if loading}
    <div class="zt-hint"><Icon name="loader" class="zt-spin" /> Checking what would be rolled back&hellip;</div>
  {:else if error}
    <div class="zt-card-note err"><Icon name="alert" /><div>{error}</div></div>
  {:else if plan}
    <Checks checks={plan.checks} />
    {#if plan.rollback.length}
      <div class="zt-callout warn zr-f-list">
        <Icon name="alert" />
        <div>
          Destroyed on the copy: <span class="zt-mono">{plan.rollback.join(', ')}</span>, and anything written there since the last common snapshot.
        </div>
      </div>
    {/if}
  {/if}
  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="danger" icon="restore" busy={starting} disabled={loading || !plan || plan.fatal} onclick={go}>Roll back and run</Button>
  {/snippet}
</Dialog>

<style>
  .zr-f-list {
    margin-top: 14px;
  }
</style>
