<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import { api } from '../lib/api';
  import { count, dateTime } from '../lib/format';
  import { KIND_LABEL, parentOf, type Reveal, type Snap } from './store.svelte';

  type Kind = 'added' | 'removed' | 'modified' | 'renamed';

  interface Change {
    change: string;
    kind: Kind;
    type: string;
    path: string;
    new_path?: string;
  }

  interface Diff {
    from: string;
    to: string;
    live: boolean;
    reversed: boolean;
    entries: Change[];
    counts: Record<Kind, number>;
    total: number;
    truncated: boolean;
  }

  const LIVE = ':live';
  const SIGN: Record<string, string> = { added: '+', removed: '−', modified: 'M', renamed: 'R' };
  const CLS: Record<string, string> = { added: 'add', removed: 'del', modified: 'mod', renamed: 'ren' };

  let {
    dataset,
    snap,
    list,
    path,
    live,
    close
  }: { dataset: string; snap: Snap; list: Snap[]; path: string; live: boolean; close: (r?: Reveal) => void } = $props();

  let other = $state('');
  let here = $state(false);
  let filter = $state('');
  let data = $state<Diff | null>(null);
  let error = $state('');
  let loading = $state(false);
  let kinds = $state<Record<Kind, boolean>>({ added: true, removed: true, modified: true, renamed: true });
  let seq = 0;

  let pair = $derived.by(() => {
    if (other === LIVE) return { from: snap, to: null as Snap | null };
    const o = list.find((x) => x.name === other);
    if (!o) return null;
    return o.creation < snap.creation ? { from: o, to: snap as Snap | null } : { from: snap, to: o as Snap | null };
  });

  let rows = $derived.by(() => {
    if (!data) return [];
    const q = filter.toLowerCase();
    return data.entries.filter(
      (c) => kinds[c.kind] !== false && (!q || c.path.toLowerCase().includes(q) || (c.new_path || '').toLowerCase().includes(q))
    );
  });

  onMount(() => {
    here = path !== '/';
    if (live) other = LIVE;
    else {
      const older = list.filter((x) => x.creation < snap.creation);
      const same = older.filter((x) => x.kind === snap.kind);
      other = (same[0] || older[0] || list[0]).name;
    }
    load();
  });

  async function load() {
    if (!pair) return;
    const my = ++seq;
    loading = true;
    error = '';
    data = null;
    try {
      const r = await api<Diff>('diff_snapshots', {
        dataset,
        from: pair.from.name,
        to: pair.to ? pair.to.name : '',
        path: here ? path : '/'
      });
      if (my !== seq) return;
      data = r;
    } catch (e) {
      if (my !== seq) return;
      error = (e as Error).message;
    } finally {
      if (my === seq) loading = false;
    }
  }

  function toggle(k: Kind) {
    const on = Object.values(kinds).filter(Boolean).length;
    if (kinds[k] && on === 4) kinds = { added: false, removed: false, modified: false, renamed: false, [k]: true };
    else if (kinds[k] && on === 1) kinds = { added: true, removed: true, modified: true, renamed: true };
    else kinds[k] = !kinds[k];
  }

  function sideOf(c: Change): Snap | null {
    if (!pair) return null;
    if (c.kind === 'removed') return pair.from;
    if (pair.to) return pair.to;
    return c.kind === 'added' ? null : pair.from;
  }

  function where(c: Change): string {
    return c.kind === 'renamed' && pair?.to ? c.new_path || c.path : c.path;
  }

  function reveal(c: Change) {
    const s = sideOf(c);
    if (!s) return;
    const p = where(c);
    close(c.type === 'dir' ? { snap: s, path: p, focus: '' } : { snap: s, path: parentOf(p), focus: p });
  }

  function restorable(c: Change): boolean {
    return !!pair && !pair.to && (c.kind === 'removed' || c.kind === 'modified') && c.type !== 'dir';
  }

  function restore(c: Change) {
    if (!pair) return;
    close({ snap: pair.from, path: parentOf(c.path), focus: c.path, restore: { path: c.path, type: c.type, name: c.path } });
  }

  function label(s: Snap): string {
    return dateTime(s.creation) + ' · ' + (KIND_LABEL[s.kind] || s.kind) + (s.kind === 'manual' || s.kind === 'replication' ? ' · ' + s.name : '');
  }
</script>

<Dialog title="What changed" icon="compare" wide onclose={() => close()}>
  <div class="zt-fields">
    <Field label={'Compare ' + label(snap) + ' with'}>
      <select bind:value={other} onchange={load}>
        <option value={LIVE}>The current files (now)</option>
        {#each list as x (x.name)}
          <option value={x.name}>{label(x)}</option>
        {/each}
      </select>
    </Field>
    <Field label="Filter">
      <input type="search" placeholder="Filter paths" bind:value={filter} />
    </Field>
  </div>
  {#if path !== '/'}
    <div class="here"><Toggle bind:checked={here} label={'Only inside ' + path} onchange={load} /></div>
  {/if}
  {#if loading}
    <div class="zt-hint loading"><Icon name="loader" class="zt-spin" /> Comparing... zfs diff can take a while on big datasets.</div>
  {:else if error}
    <div class="zt-card-note err note"><Icon name="alert" /><div>{error}</div></div>
  {:else if data && pair}
    <div class="zt-inline stats">
      {#each ['added', 'removed', 'modified', 'renamed'] as Kind[] as k (k)}
        <button
          type="button"
          class="zt-pill {k === 'added' ? 'ok' : k === 'removed' ? 'err' : k === 'modified' ? 'warn' : 'run'}"
          class:off={!kinds[k]}
          aria-pressed={kinds[k]}
          title={'Show only ' + k + ', click again for all'}
          onclick={() => toggle(k)}>{SIGN[k]} {count(data.counts[k] || 0)} {k}</button
        >
      {/each}
      <span class="zt-hint">{dateTime(pair.from.creation)} → {pair.to ? dateTime(pair.to.creation) : 'now'}</span>
    </div>
    <div class="zt-table-wrap diff">
      {#each rows.slice(0, 3000) as c, i (i)}
        {@const s = sideOf(c)}
        <div class="zt-diff-line {CLS[c.kind]}">
          <b>{SIGN[c.kind]}</b>
          <button type="button" class="p" disabled={!s} title={s ? 'Show it in the snapshot' : 'Only in the current files'} onclick={() => reveal(c)}>
            {c.path}{c.type === 'dir' ? '/' : ''}{#if c.new_path}<i> → {c.new_path}</i>{/if}
          </button>
          {#if restorable(c)}
            <button type="button" class="zt-link act" title="Restore the snapshot's version" onclick={() => restore(c)}>Restore</button>
          {/if}
        </div>
      {:else}
        <div class="zt-hint empty">{data.total ? 'Nothing matches.' : 'No differences.'}</div>
      {/each}
    </div>
    {#if data.truncated || rows.length > 3000}
      <p class="zt-hint">Showing {count(Math.min(rows.length, 3000))} of {count(data.total)} changes.</p>
    {/if}
  {/if}
</Dialog>

<style>
  :global(#zt-app) .loading {
    margin-top: 14px;
  }
  .loading :global(.zt-spin) {
    animation: zt-spin 0.9s linear infinite;
  }
  .here {
    margin-top: 10px;
  }
  :global(#zt-app) .note {
    margin-top: 14px;
  }
  :global(#zt-app) .stats {
    margin: 14px 0;
    flex-wrap: wrap;
  }
  :global(#zt-app) .stats button {
    cursor: pointer;
  }
  :global(#zt-app) .stats button.off {
    opacity: 0.45;
  }
  :global(#zt-app) .diff {
    max-height: 50vh;
    overflow-y: auto;
  }
  :global(#zt-app) .diff .zt-diff-line {
    align-items: baseline;
  }
  :global(#zt-app) .diff .zt-diff-line.ren b {
    color: var(--zt-info);
  }
  :global(#zt-app) .diff button.p {
    flex: 1;
    min-width: 0;
    text-align: left;
    overflow-wrap: anywhere;
    font: inherit;
    cursor: pointer;
  }
  :global(#zt-app) .diff button.p:disabled {
    cursor: default;
  }
  :global(#zt-app) .diff button.p:hover:not(:disabled) {
    text-decoration: underline;
  }
  .diff i {
    font-style: normal;
    color: var(--zt-dim);
  }
  :global(#zt-app) .diff .act {
    flex: none;
    font-family: inherit;
  }
  :global(#zt-app) .empty {
    padding: 12px;
  }
</style>
