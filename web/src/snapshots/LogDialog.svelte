<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { api } from '../lib/api';
  import { copyText, fail } from '../lib/state.svelte';
  import { part } from './store.svelte';

  let { close }: { close: (r?: unknown) => void } = $props();

  let text = $state('');
  let loading = $state(true);
  let pre: HTMLElement | undefined = $state();

  async function load() {
    loading = true;
    try {
      const r = await api<{ log: string }>('snap_log', { lines: 600 });
      const stick = !pre || pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 20;
      text = r.log;
      if (stick) setTimeout(() => pre && (pre.scrollTop = pre.scrollHeight), 0);
    } catch (e) {
      fail(e);
    } finally {
      loading = false;
    }
  }

  load();

  $effect(() => {
    const t = setInterval(() => {
      if (part()?.running) load();
    }, 3000);
    return () => clearInterval(t);
  });

  let lines = $derived(text ? text.split('\n') : []);

  function cls(l: string): string {
    if (/ERROR:/.test(l)) return 'bad';
    if (/WARNING:/.test(l)) return 'warn';
    if (/starting|finished/.test(l)) return 'phase';
    return '';
  }
</script>

<Dialog title="Snapshot log" icon="list" wide onclose={() => close()}>
  {#if !text && !loading}
    <p class="zt-dim">The log is empty. It lives in RAM and starts over after a reboot.</p>
  {:else}
    <pre class="zt-log ss-log" bind:this={pre}>{#each lines as l, i (i)}<span class={cls(l)}>{l}</span>{'\n'}{/each}</pre>
  {/if}
  {#snippet actions()}
    <span class="zt-hint">Last 600 lines of /tmp/zfs.toolkit/snapshots.log</span>
    <div class="zt-spacer"></div>
    <Button icon="copy" onclick={() => copyText(text)}>Copy</Button>
    <Button icon="refresh" busy={loading} onclick={load}>Refresh</Button>
  {/snippet}
</Dialog>

<style>
  :global(#zt-app) pre.ss-log {
    min-height: 200px;
  }
  :global(#zt-app) pre.ss-log .warn {
    color: var(--zt-warn);
  }
</style>
