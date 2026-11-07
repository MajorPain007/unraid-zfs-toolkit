<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Checks from './Checks.svelte';
  import { api } from '../lib/api';
  import { copyText } from '../lib/state.svelte';
  import { bytes, dateTime } from '../lib/format';
  import type { SendJob, SendPlan } from './send.svelte';

  let { job, close }: { job: SendJob; close: (r?: unknown) => void } = $props();

  let plan = $state<SendPlan | null>(null);
  let loading = $state(true);
  let error = $state('');

  onMount(async () => {
    try {
      const r = await api<{ plan: SendPlan }>('send_preview', { job });
      plan = r.plan;
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loading = false;
    }
  });
</script>

<Dialog title="Preview {job.name || 'job'}" icon="eye" wide onclose={() => close()}>
  {#if loading}
    <div class="zt-hint"><Icon name="loader" class="zt-spin" /> Working out what a run would do&hellip;</div>
  {:else if error}
    <div class="zt-card-note err"><Icon name="alert" /><div>{error}</div></div>
  {:else if plan}
    {#if !plan.fatal}
      <div class="zt-facts zr-p-facts">
        <div class="zt-fact">
          <div class="zt-label">Kind</div>
          <div class="zt-value">{plan.full ? 'Full copy' : 'Incremental'}</div>
        </div>
        <div class="zt-fact">
          <div class="zt-label">From</div>
          <div class="zt-value" title={plan.base}>{plan.base ? dateTime(plan.base_creation) : 'the beginning'}</div>
        </div>
        <div class="zt-fact">
          <div class="zt-label">About</div>
          <div class="zt-value" title={plan.estimate_note}>{plan.estimate > 0 ? bytes(plan.estimate) : plan.estimate === 0 ? 'Only new changes' : '-'} <small>{plan.estimate_note}</small></div>
        </div>
      </div>
    {/if}
    <Checks checks={plan.checks} />
    {#if plan.commands.length}
      <div class="zr-p-cmds-head">
        <b>Commands a run would execute</b>
        <Button icon="copy" small kind="ghost" onclick={() => copyText(plan?.commands.join('\n') ?? '')}>Copy</Button>
      </div>
      <pre class="zt-log zr-p-cmds">{plan.commands.join('\n\n')}</pre>
      <p class="zt-hint">The checkpoint name gets the time the run starts. After a good run older checkpoints of this job are pruned.</p>
    {/if}
  {/if}
  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button kind="primary" onclick={() => close()}>Close</Button>
  {/snippet}
</Dialog>

<style>
  :global(#zt-app) .zr-p-facts {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    margin: 0 0 16px;
  }
  .zr-p-cmds-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin: 18px 0 6px;
    font-size: 13px;
  }
  pre.zr-p-cmds {
    white-space: pre-wrap;
    word-break: break-all;
    max-height: 260px;
  }
  @media (max-width: 560px) {
    :global(#zt-app) .zr-p-facts {
      grid-template-columns: 1fr 1fr;
    }
  }
</style>
