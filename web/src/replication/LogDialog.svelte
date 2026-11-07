<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { copyText } from '../lib/state.svelte';
  import { dateTime, duration, plural } from '../lib/format';
  import { classify, recentOf, replication, resultText, type SendResult } from './send.svelte';

  let { run = '', job = '', close }: { run?: string; job?: string; close: (r?: unknown) => void } = $props();

  interface Line {
    i: number;
    t: string;
    c: string;
  }

  let current = $state('');
  let lines = $state<Line[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let running = $state(false);
  let query = $state('');
  let problemsOnly = $state(false);
  let stick = $state(true);
  let hit = $state(-1);
  let pre: HTMLElement | undefined = $state();

  let offset = -1;
  let rest = '';
  let timer: ReturnType<typeof setTimeout> | null = null;
  let alive = true;
  let gen = 0;

  let jobName = $derived(replication.jobs.find((j) => j.id === job)?.name ?? '');
  let history = $derived(job ? [...recentOf(job)].reverse() : []);
  let entry = $derived(history.find((h) => h.run === current) ?? null);
  let q = $derived(query.trim().toLowerCase());
  let visible = $derived(problemsOnly ? lines.filter((l) => l.c === 'bad' || l.c === 'warn' || l.c === 'head') : lines);
  let hits = $derived(q ? visible.filter((l) => l.t.toLowerCase().includes(q)).map((l) => l.i) : []);
  let counts = $derived({ bad: lines.filter((l) => l.c === 'bad').length, warn: lines.filter((l) => l.c === 'warn').length });

  function add(text: string, final: boolean) {
    const parts = (rest + text).split('\n');
    rest = final ? '' : (parts.pop() ?? '');
    let i = lines.length;
    const next: Line[] = [];
    for (const t of parts) if (t !== '') next.push({ i: i++, t, c: classify(t) });
    if (next.length) lines.push(...next);
  }

  async function poll(g: number) {
    try {
      const r = await api<{ text: string; offset: number; start: number; running: boolean }>('send_log', { run: current || undefined, offset });
      if (!alive || g !== gen) return;
      if (offset < 0 && r.start > 0) lines.push({ i: lines.length, t: '(earlier lines left out)', c: 'note' });
      running = r.running;
      add(r.text, !r.running);
      offset = r.offset;
      loaded = true;
      await tick();
      if (stick && pre) pre.scrollTop = pre.scrollHeight;
      if (r.running) timer = setTimeout(() => poll(g), 1500);
    } catch (e) {
      if (alive && g === gen) {
        error = (e as Error).message;
        loaded = true;
      }
    }
  }

  function open(id: string) {
    if (timer) clearTimeout(timer);
    gen++;
    current = id;
    lines = [];
    rest = '';
    offset = -1;
    error = '';
    loaded = false;
    stick = true;
    hit = -1;
    poll(gen);
  }

  onMount(() => open(run));
  onDestroy(() => {
    alive = false;
    if (timer) clearTimeout(timer);
  });

  function onScroll() {
    if (pre) stick = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 20;
  }

  async function jump(step: number) {
    if (!hits.length) return;
    hit = (hit + step + hits.length) % hits.length;
    stick = false;
    await tick();
    const el = pre?.querySelector<HTMLElement>(`[data-i="${hits[hit]}"]`);
    if (pre && el) pre.scrollTop = el.offsetTop - pre.clientHeight / 2;
  }

  function searchKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      jump(e.shiftKey ? -1 : 1);
    } else if (e.key === 'Escape' && query) {
      e.stopPropagation();
      query = '';
    }
  }

  function pieces(t: string): [string, boolean][] {
    const out: [string, boolean][] = [];
    const low = t.toLowerCase();
    let at = 0;
    for (let k = low.indexOf(q); q && k >= 0; k = low.indexOf(q, at)) {
      if (k > at) out.push([t.slice(at, k), false]);
      out.push([t.slice(k, k + q.length), true]);
      at = k + q.length;
    }
    if (at < t.length) out.push([t.slice(at), false]);
    return out;
  }

  function label(h: SendResult): string {
    return dateTime(h.started);
  }
</script>

<Dialog title={jobName ? `Log - ${jobName}` : 'Replication log'} icon="list" wide onclose={() => close()}>
  <div class="zr-log-wrap" class:with-side={history.length > 0}>
    {#if history.length}
      <nav class="zr-log-side" aria-label="Runs">
        {#each history as h (h.run)}
          <button type="button" class:active={current === h.run} onclick={() => open(h.run)}>
            <span class="zt-dot {h.status === 'ok' ? 'ok' : h.status === 'running' ? 'run' : h.status === 'cancelled' ? 'warn' : 'err'}"></span>
            <span class="zr-log-side-text">
              <b>{label(h)}</b>
              <small>{resultText(h)}{h.ended ? ' · ' + duration(h.ended - h.started) : ''}</small>
            </span>
          </button>
        {/each}
        <button type="button" class:active={current === ''} onclick={() => open('')}>
          <Icon name="list" />
          <span class="zr-log-side-text"><b>Whole log</b><small>all jobs</small></span>
        </button>
      </nav>
    {/if}
    <div class="zr-log-main">
      {#if entry && entry.error}
        <div class="zt-card-note err zr-log-err"><Icon name="alert" /><div>{entry.error}</div></div>
      {/if}
      <div class="zt-toolbar zr-log-tools">
        <div class="zr-log-search">
          <Icon name="search" />
          <input type="search" placeholder="Search the log" aria-label="Search the log" autocomplete="off" spellcheck="false" bind:value={query} oninput={() => (hit = -1)} onkeydown={searchKey} />
          {#if q}
            <span class="zt-hint">{hits.length ? (hit < 0 ? plural(hits.length, 'match', 'matches') : hit + 1 + ' of ' + hits.length) : 'No match'}</span>
            <Button icon="up" small kind="ghost" title="Previous match" disabled={!hits.length} onclick={() => jump(-1)} />
            <span class="zr-log-down"><Button icon="up" small kind="ghost" title="Next match" disabled={!hits.length} onclick={() => jump(1)} /></span>
          {/if}
        </div>
        <label class="zt-switch"><input type="checkbox" bind:checked={problemsOnly} /><span></span>Problems only</label>
        {#if counts.bad || counts.warn}
          <span class="zt-hint">
            {#if counts.bad}<span class="zt-err-text">{plural(counts.bad, 'error')}</span>{/if}
            {#if counts.warn}<span class="zt-warn-text">{plural(counts.warn, 'warning')}</span>{/if}
          </span>
        {/if}
      </div>
      <pre class="zt-log zr-log" bind:this={pre} onscroll={onScroll}>{#if !loaded}<span class="zt-dim">Loading...</span>{:else if error}<span class="bad">{error}</span>{:else if !lines.length}<span class="zt-dim">Nothing logged yet.</span>{/if}{#each visible as l (l.i)}<span class={l.c === 'note' ? 'zt-dim' : l.c || null} class:hit={q && hits[hit] === l.i} data-i={l.i}>{#if q}{#each pieces(l.t) as [t, m], k (k)}{#if m}<mark>{t}</mark>{:else}{t}{/if}{/each}{:else}{l.t}{/if}{'\n'}</span>{/each}</pre>
      {#if running}<div class="zt-hint zr-log-live"><Icon name="loader" class="zt-spin" /> Running - new lines appear as they come.</div>{/if}
    </div>
  </div>
  {#snippet actions()}
    <Button icon="copy" small disabled={!loaded} onclick={() => copyText(lines.map((l) => l.t).join('\n'))}>Copy</Button>
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Close</Button>
  {/snippet}
</Dialog>

<style>
  .zr-log-wrap {
    display: grid;
    gap: 14px;
    min-width: 0;
  }
  .zr-log-wrap.with-side {
    grid-template-columns: 200px minmax(0, 1fr);
  }
  .zr-log-side {
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 62vh;
    overflow: auto;
  }
  :global(#zt-app) .zr-log-side button {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 6px 8px;
    border: 0;
    border-radius: var(--zt-radius-sm);
    background: transparent;
    color: var(--zt-text);
    text-align: left;
    cursor: pointer;
    margin: 0;
  }
  :global(#zt-app) .zr-log-side button:hover {
    background: var(--zt-surface-2);
  }
  :global(#zt-app) .zr-log-side button.active {
    background: var(--zt-select);
  }
  .zr-log-side .zt-dot {
    margin-top: 5px;
    width: 8px;
    height: 8px;
  }
  .zr-log-side :global(svg.zt-i) {
    margin-top: 2px;
    color: var(--zt-dim);
  }
  .zr-log-side-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
    font-size: 12.5px;
  }
  .zr-log-side-text b {
    font-weight: 500;
  }
  .zr-log-side-text small {
    color: var(--zt-dim);
    font-size: 11.5px;
  }
  .zr-log-main {
    min-width: 0;
  }
  .zr-log-err {
    margin: 0 0 10px;
  }
  .zr-log-tools {
    margin-bottom: 10px;
  }
  .zr-log-search {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1 1 240px;
    min-width: 0;
    color: var(--zt-dim);
  }
  :global(#zt-app) .zr-log-search input {
    flex: 1 1 auto;
    min-width: 0;
    width: auto;
    margin: 0;
  }
  .zr-log-down :global(svg.zt-i) {
    transform: rotate(180deg);
  }
  pre.zr-log {
    height: min(56vh, 520px);
    white-space: pre-wrap;
    word-break: break-word;
    position: relative;
  }
  pre.zr-log .bad {
    color: var(--zt-err);
  }
  pre.zr-log .warn {
    color: var(--zt-warn);
  }
  pre.zr-log .good {
    color: var(--zt-ok);
  }
  pre.zr-log .head {
    color: var(--zt-accent-text);
  }
  pre.zr-log mark {
    background: var(--zt-accent-soft);
    color: inherit;
    border-radius: 2px;
  }
  pre.zr-log .hit mark {
    background: var(--zt-accent);
    color: var(--zt-panel);
  }
  .zr-log-live {
    margin-top: 6px;
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .zt-hint .zt-err-text + .zt-warn-text {
    margin-left: 8px;
  }
  @media (max-width: 700px) {
    .zr-log-wrap.with-side {
      grid-template-columns: 1fr;
    }
    .zr-log-side {
      flex-direction: row;
      overflow-x: auto;
      max-height: none;
    }
    :global(#zt-app) .zr-log-side button {
      flex: none;
    }
  }
</style>
