<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import type { SendCheck } from './send.svelte';

  let { checks }: { checks: SendCheck[] } = $props();

  const ICON: Record<string, string> = { ok: 'check', info: 'info', warn: 'alert', err: 'x' };
</script>

<ul class="zr-checks">
  {#each checks as c, i (i)}
    <li class={c.level}><Icon name={ICON[c.level] ?? 'info'} /><span>{c.text}</span></li>
  {/each}
</ul>

<style>
  .zr-checks {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 8px;
  }
  .zr-checks li {
    display: flex;
    gap: 9px;
    align-items: flex-start;
    font-size: 13px;
    line-height: 1.45;
    overflow-wrap: anywhere;
  }
  .zr-checks li :global(svg.zt-i) {
    flex: none;
    margin-top: 2px;
    width: 15px;
    height: 15px;
  }
  .zr-checks li.ok :global(svg.zt-i) {
    color: var(--zt-ok);
  }
  .zr-checks li.info :global(svg.zt-i) {
    color: var(--zt-info);
  }
  .zr-checks li.warn {
    color: var(--zt-warn);
  }
  .zr-checks li.err {
    color: var(--zt-err);
  }
</style>
