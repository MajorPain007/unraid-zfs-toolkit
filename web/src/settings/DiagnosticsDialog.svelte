<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { copyText } from '../lib/state.svelte';

  let { name, report, close }: { name: string; report: string; close: (r?: unknown) => void } = $props();

  function save() {
    const url = URL.createObjectURL(new Blob([report], { type: 'text/plain' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
</script>

<Dialog title="Diagnostics" icon="terminal" wide onclose={() => close()}>
  <p class="zt-hint" style="margin-top:0">
    Settings, cron entries, pools, snapshot counts and the last log lines. Replication hosts and ports are masked, passwords and SSH keys are not included. Pool, dataset, share and container names and disk IDs are, so read it through before you post it publicly.
  </p>
  <pre class="zt-log diag">{report}</pre>
  {#snippet actions()}
    <Button icon="copy" onclick={() => copyText(report)}>Copy</Button>
    <Button icon="download" kind="primary" onclick={save}>Download</Button>
  {/snippet}
</Dialog>

<style>
  .diag {
    max-height: 60vh;
    overflow: auto;
    white-space: pre;
    font-size: 11.5px;
  }
</style>
