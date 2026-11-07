<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Checks from './Checks.svelte';
  import { api } from '../lib/api';
  import { act } from '../lib/actions';
  import { fail, toast } from '../lib/state.svelte';
  import { target, type SendCheck, type SendJob } from './send.svelte';

  let { job, close }: { job: SendJob; close: (r?: unknown) => void } = $props();

  let checks = $state<SendCheck[]>([]);
  let ok = $state(false);
  let loading = $state(true);
  let error = $state('');

  let hostKey = $derived(checks.some((c) => c.text.includes('host key')));
  let keyProblem = $derived(checks.some((c) => c.text.includes('did not accept the key')));

  async function test() {
    loading = true;
    error = '';
    try {
      const r = await api<{ checks: SendCheck[]; ok: boolean }>('send_test', { job });
      checks = r.checks;
      ok = r.ok;
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loading = false;
    }
  }

  async function forget() {
    try {
      await api('send_forget_host', { host: job.ssh_host, port: job.ssh_port });
      toast('Forgot the old host key');
      test();
    } catch (e) {
      fail(e);
    }
  }

  onMount(test);
</script>

<Dialog title="Test {job.name || 'job'}" icon="link" wide onclose={() => close()}>
  <p class="zt-hint zr-t-head">{job.source || '?'} <Icon name="right" /> {target(job)} - nothing is changed by this test.</p>
  {#if loading}
    <div class="zt-hint"><Icon name="loader" class="zt-spin" /> Checking&hellip;</div>
  {:else if error}
    <div class="zt-card-note err"><Icon name="alert" /><div>{error}</div></div>
  {:else}
    <Checks {checks} />
    <div class="zt-callout zr-t-sum" class:warn={!ok}>
      <Icon name={ok ? 'check' : 'alert'} />
      <div>{ok ? 'Everything needed for a run is in place.' : 'A run would fail - see the red line above.'}</div>
    </div>
  {/if}
  {#snippet actions()}
    {#if hostKey}<Button icon="trash" onclick={forget}>Forget host key</Button>{/if}
    {#if keyProblem || job.transport === 'ssh'}<Button icon="key" onclick={() => act('sendKey', job)}>SSH key</Button>{/if}
    <div class="zt-spacer"></div>
    <Button icon="refresh" busy={loading} onclick={test}>Test again</Button>
    <Button kind="primary" onclick={() => close()}>Close</Button>
  {/snippet}
</Dialog>

<style>
  .zr-t-head {
    margin: 0 0 14px;
    font-family: var(--zt-mono);
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .zr-t-sum {
    margin-top: 16px;
  }
</style>
