<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { copyText, fail } from '../lib/state.svelte';
  import { dateTime, plural } from '../lib/format';
  import { classify, pieces, saveText, type Line } from './lines';
  import { STATUS_LABEL } from './types';

  interface LogAnswer {
    file: string;
    name: string;
    text: string;
    offset: number;
    start: number;
    size: number;
    status: string;
  }

  let { file = '', close }: { file?: string; close: (r?: unknown) => void } = $props();

  let lines = $state<Line[]>([]);
  let name = $state('');
  let status = $state('');
  let error = $state('');
  let loaded = $state(false);
  let stick = $state(true);
  let query = $state('');
  let problemsOnly = $state(false);
  let current = $state(-1);
  let pre: HTMLElement | undefined = $state();

  let path = '';
  let offset = -1;
  let rest = '';
  let timer: ReturnType<typeof setTimeout> | null = null;
  let alive = true;

  let running = $derived(status === 'running');
  let q = $derived(query.trim().toLowerCase());
  let counts = $derived({
    bad: lines.filter((l) => l.c === 'bad').length,
    warn: lines.filter((l) => l.c === 'warn').length
  });
  let visible = $derived(problemsOnly ? lines.filter((l) => l.c === 'bad' || l.c === 'warn' || l.c === 'note' || l.c === 'phase') : lines);
  let hits = $derived(q ? visible.filter((l) => l.t.toLowerCase().includes(q)).map((l) => l.i) : []);
  let pill = $derived(STATUS_LABEL[status] ?? null);

  function add(text: string, final: boolean) {
    const parts = (rest + text).split('\n');
    rest = final ? '' : (parts.pop() ?? '');
    const next: Line[] = [];
    let i = lines.length;
    for (const t of parts) next.push({ i: i++, t, c: classify(t) });
    if (next.length) lines.push(...next);
  }

  async function follow() {
    await tick();
    if (stick && pre) pre.scrollTop = pre.scrollHeight;
  }

  async function poll() {
    try {
      const r = await api<LogAnswer>('convert_log', { file: path || file, offset });
      if (!alive) return;
      if (!path) {
        path = r.file;
        name = r.name;
        if (r.start > 0) lines.push({ i: 0, t: '(earlier lines left out)', c: 'note' });
      }
      status = r.status;
      add(r.text, r.status !== 'running' && r.offset >= r.size);
      offset = r.offset;
      loaded = true;
      follow();
      if (r.status === 'running' || r.offset < r.size) timer = setTimeout(poll, r.offset < r.size ? 50 : 1500);
    } catch (e) {
      if (alive) error = (e as Error).message;
    }
  }

  onMount(poll);
  onDestroy(() => {
    alive = false;
    if (timer) clearTimeout(timer);
  });

  function onScroll() {
    if (pre) stick = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 20;
  }

  async function jump(step: number) {
    if (!hits.length) return;
    current = (current + step + hits.length) % hits.length;
    stick = false;
    await tick();
    const el = pre?.querySelector<HTMLElement>(`[data-i="${hits[current]}"]`);
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

  function text(): string {
    return lines.filter((l) => l.c !== 'note').map((l) => l.t + '\n').join('') + rest;
  }

  function download() {
    try {
      saveText(name || 'conversion.log', text());
    } catch (e) {
      fail(e);
    }
  }

  function when(n: string): string {
    const m = /(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})/.exec(n);
    if (!m) return n;
    return dateTime(new Date(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +m[6]).getTime() / 1000);
  }
</script>

<Dialog title="Conversion log" icon="list" wide onclose={() => close()}>
  <div class="cv-log-head">
    <div class="cv-log-title">
      {#if name}<span><b>{name.startsWith('auto_') ? 'Scheduled run' : 'Run'}</b> · {when(name)}</span>{/if}
      {#if pill}<span class="zt-pill {pill[1]}">{#if running}<Icon name="loader" class="zt-spin" />{/if}{pill[0]}</span>{/if}
    </div>
    {#if loaded && (counts.bad || counts.warn)}
      <div class="cv-log-counts">
        {#if counts.bad}<span class="zt-err-text">{plural(counts.bad, 'error line')}</span>{/if}
        {#if counts.warn}<span class="zt-warn-text">{plural(counts.warn, 'warning')}</span>{/if}
      </div>
    {/if}
  </div>

  {#if loaded}
    <div class="cv-log-tools">
      <div class="cv-log-search">
        <Icon name="search" />
        <input type="search" placeholder="Search the log" aria-label="Search the log" autocomplete="off" spellcheck="false" bind:value={query} oninput={() => (current = -1)} onkeydown={searchKey} />
        {#if q}
          <span class="cv-log-hits">{hits.length ? (current < 0 ? plural(hits.length, 'match', 'matches') : current + 1 + ' of ' + hits.length) : 'No match'}</span>
          <Button icon="up" small kind="ghost" title="Previous match" disabled={!hits.length} onclick={() => jump(-1)} />
          <span class="cv-log-down"><Button icon="up" small kind="ghost" title="Next match" disabled={!hits.length} onclick={() => jump(1)} /></span>
        {/if}
      </div>
      <label class="zt-switch">
        <input type="checkbox" bind:checked={problemsOnly} onchange={() => (current = -1)} />
        <span></span>
        Problems only
      </label>
      {#if running}
        <label class="zt-switch">
          <input type="checkbox" checked={stick} onchange={(e) => { stick = e.currentTarget.checked; follow(); }} />
          <span></span>
          Follow
        </label>
      {/if}
    </div>
  {/if}

  {#if error}
    <pre class="zt-log">{error}</pre>
  {:else}
    <pre class="zt-log cv-log" aria-label="Log text" bind:this={pre} onscroll={onScroll}>{#if !loaded}<span class="cv-dim">Loading...</span>{/if}{#each visible as l (l.i)}<span class={l.c === 'note' ? 'cv-dim' : l.c || null} class:hit={q && hits[current] === l.i} data-i={l.i}>{#if q}{#each pieces(l.t, q) as [t, m], k (k)}{#if m}<mark>{t}</mark>{:else}{t}{/if}{/each}{:else}{l.t}{/if}{'\n'}</span>{/each}{#if loaded && !lines.length}<span class="cv-dim">The log is empty.</span>{/if}{#if loaded && problemsOnly && !counts.bad && !counts.warn}<span class="cv-dim">No errors or warnings in this log.</span>{/if}</pre>
  {/if}

  {#snippet actions()}
    <Button icon="copy" small disabled={!loaded} onclick={() => copyText(text())}>Copy</Button>
    <Button icon="download" small disabled={!loaded} onclick={download}>Download</Button>
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Close</Button>
  {/snippet}
</Dialog>

<style>
  .cv-log-head {
    display: flex;
    align-items: center;
    gap: 8px 14px;
    flex-wrap: wrap;
    margin-bottom: 10px;
  }
  .cv-log-title {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    flex: 1 1 auto;
    min-width: 0;
  }
  .cv-log-counts {
    display: flex;
    gap: 12px;
    font-size: 12.5px;
  }
  .cv-log-tools {
    display: flex;
    align-items: center;
    gap: 8px 18px;
    flex-wrap: wrap;
    margin-bottom: 10px;
  }
  .cv-log-search {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1 1 240px;
    min-width: 0;
    color: var(--zt-dim);
  }
  .cv-log-search input {
    flex: 1 1 auto;
    min-width: 0;
    margin: 0;
  }
  .cv-log-hits {
    font-size: 12px;
    white-space: nowrap;
  }
  .cv-log-down :global(svg.zt-i) {
    transform: rotate(180deg);
  }
  pre.cv-log {
    position: relative;
    min-height: 200px;
  }
  pre.cv-log .warn {
    color: var(--zt-warn);
  }
  pre.cv-log .good {
    color: var(--zt-ok);
  }
  pre.cv-log .cv-dim {
    color: var(--zt-faint);
  }
  pre.cv-log mark {
    background: var(--zt-accent-soft);
    color: inherit;
    border-radius: 2px;
    outline: 1px solid var(--zt-accent-soft);
  }
  pre.cv-log .hit mark {
    background: var(--zt-accent);
    color: var(--zt-panel);
  }
</style>
