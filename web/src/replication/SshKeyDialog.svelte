<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Field from '../lib/ui/Field.svelte';
  import { api } from '../lib/api';
  import { act } from '../lib/actions';
  import { confirm } from '../lib/dialogs';
  import { copyText, fail, toast } from '../lib/state.svelte';
  import { replication, type SendJob, type SendKey } from './send.svelte';

  let { job, close }: { job: SendJob; close: (r?: unknown) => void } = $props();

  let key = $state<SendKey | null>(null);
  let busy = $state(false);
  let ownKey = $derived(!job.ssh_key || (key ? job.ssh_key === key.path : false));
  let host = $derived(job.ssh_host || 'root@other-server');
  let copyId = $derived(`ssh-copy-id -i ${(key?.path ?? '') + '.pub'}${job.ssh_port && job.ssh_port !== 22 ? ' -p ' + job.ssh_port : ''} ${host}`);
  let append = $derived(key?.public ? `echo '${key.public}' >> ~/.ssh/authorized_keys` : '');

  async function load(op: string) {
    busy = true;
    try {
      const r = await api<{ key: SendKey }>('send_key', { op });
      key = r.key;
      replication.key = r.key;
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }

  async function renew() {
    const yes = await confirm({
      title: 'Make a new key?',
      text: 'The old key stops working everywhere it was added; the new one has to be authorised on each server again.',
      confirm: 'New key',
      danger: true
    });
    if (yes) {
      await load('renew');
      toast('New key made - authorise it on the other server.');
    }
  }

  onMount(() => load('show'));
</script>

<Dialog title="SSH key for {host}" icon="key" wide onclose={() => close()}>
  {#if job.ssh_key && key && job.ssh_key !== key.path}
    <p>This job uses its own key file <code>{job.ssh_key}</code>. Its public half has to be in <code>~/.ssh/authorized_keys</code> of {host}.</p>
  {:else if !key?.exists}
    <p>The plugin can make its own key pair. It is kept on the flash drive, so it survives reboots. No password is ever stored or sent.</p>
    <Button kind="primary" icon="key" {busy} onclick={() => load('create')}>Make a key</Button>
  {:else}
    <p>The other server has to accept this public key for the user in the SSH login ({host}).</p>
    <Field label="Public key">
      <div class="zt-secret"><code>{key.public}</code><Button icon="copy" small kind="ghost" title="Copy" onclick={() => copyText(key?.public ?? '')} /></div>
    </Field>
    {#if key.fingerprint}<div class="zt-hint zr-k-fp">{key.fingerprint}</div>{/if}
    <ol class="zr-k-steps">
      <li>
        <b>Another Unraid server:</b> Users, root, paste the key into "SSH authorized keys", Apply. SSH has to be on (Settings, Management Access).
      </li>
      <li>
        <b>Any Linux box, from this server's terminal</b> - asks for that server's password once, in your terminal, not here:
        <div class="zt-secret zr-k-cmd"><code>{copyId}</code><Button icon="copy" small kind="ghost" title="Copy" onclick={() => copyText(copyId)} /></div>
      </li>
      <li>
        <b>Or on the other server itself:</b>
        <div class="zt-secret zr-k-cmd"><code>{append}</code><Button icon="copy" small kind="ghost" title="Copy" onclick={() => copyText(append)} /></div>
      </li>
    </ol>
    <p class="zt-hint">The first connection remembers the server's host key; if it changes later, runs stop until you confirm it with Test.</p>
  {/if}
  {#snippet actions()}
    {#if key?.exists && ownKey}<Button icon="refresh" kind="ghost" small {busy} onclick={renew}>New key</Button>{/if}
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Close</Button>
    {#if job.ssh_host}<Button kind="primary" icon="link" onclick={() => { close(); act('sendTest', job); }}>Test connection</Button>{/if}
  {/snippet}
</Dialog>

<style>
  .zr-k-fp {
    margin-top: 6px;
    font-family: var(--zt-mono);
    font-size: 11.5px;
  }
  .zr-k-steps {
    margin: 16px 0 10px;
    padding-left: 20px;
    display: grid;
    gap: 12px;
    font-size: 13px;
  }
  .zr-k-cmd {
    margin-top: 6px;
    font-size: 12px;
  }
</style>
