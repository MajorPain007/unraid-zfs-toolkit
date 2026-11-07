<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Timeline from './Timeline.svelte';
  import { bytes, clock, dayLabel } from '../lib/format';
  import { B, KIND_LABEL, openSnap, type Snap } from './store.svelte';

  let listEl: HTMLElement | undefined = $state();

  let shown = $derived((B.snaps || []).filter((s) => !B.kind || s.kind === B.kind));

  let groups = $derived.by(() => {
    const out: { key: string; ts: number; snaps: Snap[] }[] = [];
    for (const s of shown) {
      const key = new Date(s.creation * 1000).toDateString();
      const last = out[out.length - 1];
      if (last && last.key === key) last.snaps.push(s);
      else out.push({ key, ts: s.creation, snaps: [s] });
    }
    return out;
  });

  let activeDay = $derived(B.snap ? new Date(B.snap.creation * 1000).toDateString() : '');

  onMount(() => {
    tick().then(() => {
      if (listEl) listEl.scrollTop = B.scroll.list;
    });
  });

  function jumpTo(day: string) {
    if (!listEl) return;
    const head = listEl.querySelector<HTMLElement>(`[data-day="${CSS.escape(day)}"]`);
    const first = head?.nextElementSibling as HTMLElement | null;
    if (head && first) listEl.scrollTo({ top: first.offsetTop - head.offsetHeight, behavior: 'smooth' });
  }

  function keydown(e: KeyboardEvent) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    const btns = [...(listEl?.querySelectorAll<HTMLButtonElement>('button.zt-snap') ?? [])];
    const i = btns.indexOf(document.activeElement as HTMLButtonElement);
    if (i < 0) return;
    e.preventDefault();
    btns[Math.max(0, Math.min(btns.length - 1, i + (e.key === 'ArrowDown' ? 1 : -1)))].focus();
  }
</script>

<div class="snapcol">
  {#if shown.length}
    <Timeline snaps={shown} active={activeDay} onpick={jumpTo} />
  {/if}
  <div class="zt-snaplist" bind:this={listEl} onscroll={() => listEl && (B.scroll.list = listEl.scrollTop)} onkeydown={keydown} role="presentation">
    {#if B.snapsLoading && !B.snaps}
      <div class="zt-hint pad"><Icon name="loader" class="zt-spin" /> Loading snapshots...</div>
    {:else if B.snapsError}
      <div class="zt-card-note err note"><Icon name="alert" /><div>{B.snapsError}</div></div>
    {:else if !shown.length}
      <div class="zt-hint pad">{B.kind ? 'No ' + (KIND_LABEL[B.kind] || B.kind).toLowerCase() + ' snapshots.' : 'This dataset has no snapshots yet.'}</div>
    {:else}
      {#each groups as g (g.key)}
        <div class="zt-snap-day" data-day={g.key}>{dayLabel(g.ts)}<span class="n">{g.snaps.length}</span></div>
        {#each g.snaps as s (s.name)}
          <button
            type="button"
            class="zt-snap"
            class:active={B.snap?.name === s.name}
            title={B.dataset + '@' + s.name}
            onclick={() => openSnap(s)}
          >
            <span class="zt-snap-time">{clock(s.creation)}</span>
            <span class="zt-snap-main">
              <span class="kind k-{s.kind}">{KIND_LABEL[s.kind] || s.kind}</span>
              <span class="sname">{s.name.replace(/^auto-[a-z]+-/, '')}</span>
            </span>
            {#if s.holds > 0}
              <span class="held" title={'Held (' + s.holds + ') - automatic pruning leaves it alone'}><Icon name="lock" /></span>
            {/if}
            <span class="zt-snap-size" title="Space only this snapshot holds">{bytes(s.used)}</span>
          </button>
        {/each}
      {/each}
    {/if}
  </div>
</div>

<style>
  .snapcol {
    min-width: 0;
  }
  :global(#zt-app) .zt-snap-day {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .n {
    font-weight: 500;
    color: var(--zt-faint);
    letter-spacing: 0;
  }
  :global(#zt-app) .pad {
    padding: 16px;
  }
  :global(#zt-app) .note {
    margin: 12px;
  }
  .pad :global(.zt-spin) {
    animation: zt-spin 0.9s linear infinite;
  }
  :global(#zt-app) .zt-snaplist :global(button.zt-snap:focus-visible) {
    outline: 2px solid var(--zt-accent);
    outline-offset: -2px;
  }
  .kind {
    display: inline-block;
    padding: 1px 7px;
    margin-right: 6px;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 600;
    background: var(--zt-surface-2);
    color: var(--zt-dim);
  }
  .kind.k-manual {
    background: var(--zt-accent-soft);
    color: var(--zt-accent-text);
  }
  .kind.k-replication {
    background: var(--zt-info-soft);
    color: var(--zt-info);
  }
  .kind.k-daily,
  .kind.k-weekly,
  .kind.k-monthly,
  .kind.k-yearly {
    background: var(--zt-ok-soft);
    color: var(--zt-ok);
  }
  .sname {
    font-family: var(--zt-mono);
    font-size: 11.5px;
  }
  .held {
    display: inline-flex;
    color: var(--zt-warn);
  }
  .held :global(svg.zt-i) {
    width: 13px;
    height: 13px;
  }
</style>
