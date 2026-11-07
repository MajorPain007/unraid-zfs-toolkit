<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import FileIcon from './FileIcon.svelte';
  import { bytes, dateTime, plural } from '../lib/format';
  import { toast } from '../lib/state.svelte';
  import { B, KIND_LABEL, goUp, navigate, openAt, reload, type Entry, type SortKey } from './store.svelte';
  import { compare, downloadPath, restoreItems, snapMenu } from './ops';

  let grid: HTMLElement | undefined = $state();
  let filterEl: HTMLInputElement | undefined = $state();

  let parts = $derived(B.path.split('/').filter(Boolean));
  let liveChanges = $derived((B.entries || []).filter((e) => e.live).length);

  let visible = $derived.by(() => {
    const q = B.filter.trim().toLowerCase();
    let list = (B.entries || []).filter((e) => (!q || e.name.toLowerCase().includes(q)) && (!B.changedOnly || e.live));
    const { key, desc } = B.sort;
    if (key) {
      const dir = desc ? -1 : 1;
      list = [...list].sort((a, b) => {
        if (key === 'name') {
          const d = Number(b.type === 'dir') - Number(a.type === 'dir');
          return d || a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }) * dir;
        }
        const av = key === 'size' ? (a.type === 'dir' ? -1 : a.size) : a.mtime;
        const bv = key === 'size' ? (b.type === 'dir' ? -1 : b.size) : b.mtime;
        return (av - bv) * dir || a.name.localeCompare(b.name);
      });
    }
    return list;
  });

  let items = $derived(Object.values(B.checked));
  let allChecked = $derived(visible.length > 0 && visible.every((e) => B.checked[e.path]));
  let someChecked = $derived(!allChecked && visible.some((e) => B.checked[e.path]));
  let hiddenChecked = $derived.by(() => {
    const seen = new Set(visible.map((e) => e.path));
    return items.filter((e) => !seen.has(e.path)).length;
  });
  let folderSize = $derived((B.entries || []).reduce((n, e) => n + (e.type === 'file' ? e.size : 0), 0));

  $effect(() => {
    if (B.listing || !B.focus || !B.entries) return;
    const i = visible.findIndex((e) => e.path === B.focus);
    B.focus = '';
    if (i >= 0) {
      B.cursor = i;
      grid?.focus({ preventScroll: true });
      reveal();
    }
  });

  onMount(() => {
    tick().then(() => {
      if (grid) grid.scrollTop = B.scroll.browser;
    });
  });

  function reveal() {
    tick().then(() => grid?.querySelector('tr.cursor')?.scrollIntoView({ block: 'nearest' }));
  }

  function toggle(e: Entry, on = !B.checked[e.path]) {
    if (on) B.checked[e.path] = e;
    else delete B.checked[e.path];
  }

  function selectAll(on: boolean) {
    B.checked = {};
    if (on) for (const e of visible) B.checked[e.path] = e;
  }

  function openChild(e: Entry) {
    if (!e.child || !B.snap) return;
    if (e.child_snap) openAt(e.child, B.snap.name, '/');
    else toast(e.child + ' is a dataset of its own and has no snapshot called ' + B.snap.name + '. Choose it in the list above.', 'info');
  }

  function activate(e: Entry) {
    if (e.child) openChild(e);
    else if (e.type === 'dir') navigate(e.path);
    else toggle(e);
  }

  function rowClick(e: Entry, i: number) {
    B.cursor = i;
    activate(e);
  }

  function sortBy(key: SortKey) {
    if (B.sort.key === key) {
      if (B.sort.desc) B.sort = { key: '', desc: false };
      else B.sort.desc = true;
    } else B.sort = { key, desc: false };
  }

  function sortMark(key: SortKey): string {
    return B.sort.key === key ? (B.sort.desc ? ' ↓' : ' ↑') : '';
  }

  function ariaSort(key: SortKey): 'ascending' | 'descending' | 'none' {
    return B.sort.key === key ? (B.sort.desc ? 'descending' : 'ascending') : 'none';
  }

  function move(to: number) {
    if (!visible.length) return;
    B.cursor = Math.max(0, Math.min(visible.length - 1, to));
    reveal();
  }

  function keydown(ev: KeyboardEvent) {
    if (ev.target !== grid) return;
    const cur = visible[B.cursor];
    const mod = ev.ctrlKey || ev.metaKey;
    switch (ev.key) {
      case 'ArrowDown':
        move(B.cursor < 0 ? 0 : B.cursor + 1);
        break;
      case 'ArrowUp':
        move(B.cursor < 0 ? 0 : B.cursor - 1);
        break;
      case 'PageDown':
        move(B.cursor + 10);
        break;
      case 'PageUp':
        move(B.cursor - 10);
        break;
      case 'Home':
        move(0);
        break;
      case 'End':
        move(visible.length - 1);
        break;
      case 'Enter':
        if (!cur) return;
        activate(cur);
        break;
      case 'ArrowRight':
        if (!cur || cur.type !== 'dir') return;
        activate(cur);
        break;
      case ' ':
        if (!cur) return;
        toggle(cur);
        break;
      case 'Backspace':
      case 'ArrowLeft':
        if (B.path === '/') return;
        goUp();
        break;
      case 'Escape':
        if (!items.length) return;
        B.checked = {};
        break;
      case '/':
        filterEl?.focus();
        break;
      case 'a':
      case 'A':
        if (!mod) return;
        selectAll(!allChecked);
        break;
      default:
        return;
    }
    ev.preventDefault();
    ev.stopPropagation();
  }

  function filterKey(ev: KeyboardEvent) {
    if (ev.key === 'ArrowDown' || ev.key === 'Enter') {
      ev.preventDefault();
      grid?.focus();
      if (visible.length) move(Math.max(0, B.cursor));
    } else if (ev.key === 'Escape' && B.filter) {
      ev.stopPropagation();
      B.filter = '';
    }
  }

  function asItem(e: Entry) {
    return { path: e.path, type: e.type, name: e.name };
  }
</script>

{#if !B.snap}
  <div class="zt-browser">
    <div class="zt-empty none">
      <Icon name="layers" class="big" />
      <h3>Choose a snapshot</h3>
      <p>Its files appear here, to look through, compare, restore or download.</p>
    </div>
  </div>
{:else}
  {@const s = B.snap}
  <div class="zt-browser">
    <div class="zt-snap-info">
      <span><b>{dateTime(s.creation)}</b></span>
      <span class="zt-mono sname" title={B.dataset + '@' + s.name}>{s.name}</span>
      <span>{KIND_LABEL[s.kind] || s.kind}</span>
      <span title="Data the dataset held when the snapshot was taken">Size <b>{bytes(s.refer)}</b></span>
      <span title="Space freed if only this snapshot were destroyed">Unique <b>{bytes(s.used)}</b></span>
      {#if s.holds > 0}<span class="held"><Icon name="lock" /> held</span>{/if}
      <span class="grow"></span>
      <span class="info-actions">
        <Button icon="compare" small kind="ghost" onclick={() => compare(s)}>Compare</Button>
        <Button icon="more" small kind="ghost" title="More" onclick={(e) => snapMenu(s, e.currentTarget as Element)} />
      </span>
    </div>

    <div class="zt-browser-head">
      <div class="zt-crumbs">
        <button type="button" title={B.mountpoint || '/'} aria-label="Top of the snapshot" onclick={() => navigate('/')}><Icon name="home" /></button>
        {#each parts as p, i (i)}
          <span class="zt-sep">/</span>
          <button type="button" onclick={() => navigate('/' + parts.slice(0, i + 1).join('/'))}>{p}</button>
        {/each}
      </div>
      {#if B.path !== '/'}
        <Button icon="up" small kind="ghost" title="Up" onclick={goUp} />
      {/if}
      {#if B.live && (liveChanges || B.changedOnly) && !B.listing}
        <button
          type="button"
          class="zt-pill warn changed"
          class:on={B.changedOnly}
          aria-pressed={B.changedOnly}
          title="Show only what differs from the current files"
          onclick={() => (B.changedOnly = !B.changedOnly)}>{liveChanges} {liveChanges === 1 ? 'differs' : 'differ'} from now</button
        >
      {/if}
      {#if ((B.entries && B.entries.length > 8) || B.filter) && !B.listing}
        <input
          bind:this={filterEl}
          class="quick"
          type="search"
          placeholder="Filter this folder"
          aria-label="Filter this folder"
          bind:value={B.filter}
          oninput={() => (B.cursor = -1)}
          onkeydown={filterKey}
        />
      {/if}
    </div>

    <div
      class="zt-browser-scroll grid"
      bind:this={grid}
      role="grid"
      tabindex="0"
      aria-label="Folder contents"
      aria-busy={B.listing}
      aria-multiselectable="true"
      aria-activedescendant={B.cursor >= 0 && visible[B.cursor] ? 'zt-brrow-' + B.cursor : undefined}
      onkeydown={keydown}
      onscroll={() => grid && (B.scroll.browser = grid.scrollTop)}
    >
      {#if B.listing}
        <div class="zt-hint pad"><Icon name="loader" class="zt-spin" /> Reading the folder...</div>
      {:else if B.listError}
        <div class="zt-card-note err note">
          <Icon name="alert" />
          <div>
            {B.listError}
            <div class="retry"><Button small icon="refresh" onclick={reload}>Try again</Button></div>
          </div>
        </div>
      {:else}
        <table class="zt-table">
          <thead>
            <tr>
              <th class="shrink">
                <input
                  type="checkbox"
                  aria-label="Select all"
                  tabindex="-1"
                  checked={allChecked}
                  {@attach (n: HTMLInputElement) => {
                    n.indeterminate = someChecked;
                  }}
                  onchange={(ev) => selectAll(ev.currentTarget.checked)}
                />
              </th>
              <th aria-sort={ariaSort('name')}><button type="button" tabindex="-1" class="sort" onclick={() => sortBy('name')}>Name{sortMark('name')}</button></th>
              <th class="num zt-hide-narrow" aria-sort={ariaSort('size')}><button type="button" tabindex="-1" class="sort" onclick={() => sortBy('size')}>Size{sortMark('size')}</button></th>
              <th class="zt-hide-narrow" aria-sort={ariaSort('mtime')}><button type="button" tabindex="-1" class="sort" onclick={() => sortBy('mtime')}>Modified{sortMark('mtime')}</button></th>
            </tr>
          </thead>
          <tbody>
            {#each visible as e, i (e.path)}
              <tr
                id={'zt-brrow-' + i}
                class="clickable"
                class:cursor={i === B.cursor}
                class:checked={!!B.checked[e.path]}
                aria-selected={!!B.checked[e.path]}
                onclick={() => rowClick(e, i)}
              >
                <td class="shrink">
                  <input
                    type="checkbox"
                    tabindex="-1"
                    aria-label={'Select ' + e.name}
                    checked={!!B.checked[e.path]}
                    onclick={(ev) => ev.stopPropagation()}
                    onchange={(ev) => toggle(e, ev.currentTarget.checked)}
                  />
                </td>
                <td class="name">
                  <FileIcon type={e.type} name={e.name} />
                  <span title={e.path + (e.target ? ' → ' + e.target : '')}>{e.name}{#if e.target}<i class="target"> → {e.target}</i>{/if}</span>
                  {#if e.child}
                    <em class="badge child" title={'Its files are in the snapshots of ' + e.child + (e.child_snap ? ' - open its snapshot of the same name' : '')}
                      >dataset{e.child_snap ? ' →' : ''}</em
                    >
                  {:else if e.live === 'gone'}
                    <em class="badge gone" title="Not in the current files any more">deleted since</em>
                  {:else if e.live === 'changed'}
                    <em class="badge changed" title="The current file differs in size, time or type">changed since</em>
                  {/if}
                </td>
                <td class="num zt-hide-narrow">{e.type === 'file' ? bytes(e.size) : ''}</td>
                <td class="dim zt-hide-narrow">{e.mtime ? dateTime(e.mtime) : ''}</td>
              </tr>
            {:else}
              <tr>
                <td colspan="4" class="dim">
                  {#if (B.filter || B.changedOnly) && B.entries?.length}
                    Nothing here matches.
                    <button
                      type="button"
                      class="zt-link"
                      onclick={() => {
                        B.filter = '';
                        B.changedOnly = false;
                      }}>Show all</button
                    >
                  {:else}
                    Empty folder.
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if B.truncated}
          <div class="zt-hint pad">This folder is very large - only the first 20,000 entries are shown.</div>
        {/if}
      {/if}
    </div>

    <div class="zt-selbar">
      {#if !items.length}
        <span>Select files and folders, or:</span>
        {#if B.entries?.length && !B.listing}
          <span class="count zt-hide-narrow"
            >{plural(B.entries.length, 'item')}{folderSize ? ' · ' + bytes(folderSize) + ' in files' : ''}{B.live && B.liveAdded
              ? ' · ' + plural(B.liveAdded, 'new item') + ' since'
              : ''}</span
          >
        {/if}
        <div class="zt-spacer"></div>
        <Button icon="download" small title="Downloads the folder as a zip file" onclick={() => downloadPath(B.path)}>Download folder</Button>
        <Button
          icon="restore"
          small
          kind="primary"
          onclick={() => restoreItems([{ path: B.path, type: 'dir', name: B.path === '/' ? B.dataset : B.path }])}
          >{B.path === '/' ? 'Restore everything...' : 'Restore this folder...'}</Button
        >
      {:else}
        <b>{plural(items.length, 'item') + ' selected'}</b>
        {#if hiddenChecked}
          <span class="count">{hiddenChecked} hidden by the filter</span>
        {/if}
        <button type="button" class="zt-link" onclick={() => (B.checked = {})}>Clear</button>
        <div class="zt-spacer"></div>
        <Button
          icon="download"
          small
          disabled={items.length !== 1 || items[0].type === 'symlink' || items[0].type === 'other'}
          title={items.length !== 1 ? 'One item at a time' : items[0].type === 'dir' ? 'Downloads the folder as a zip file' : ''}
          onclick={() => downloadPath(items[0].path)}>Download</Button
        >
        <Button icon="restore" small kind="primary" onclick={() => restoreItems(items.map(asItem))}>Restore...</Button>
      {/if}
    </div>
  </div>
{/if}

<style>
  :global(#zt-app) .none {
    border: 0;
  }
  .grow {
    flex: 1;
  }
  .sname {
    max-width: 260px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .held {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--zt-warn);
  }
  .held :global(svg.zt-i) {
    width: 13px;
    height: 13px;
  }
  .info-actions {
    display: inline-flex;
    gap: 4px;
    margin: -4px 0;
  }
  :global(#zt-app) .zt-snap-info {
    align-items: center;
  }
  :global(#zt-app) input.quick {
    width: 200px;
    height: 30px;
    flex: 0 1 200px;
  }
  :global(#zt-app) .pad {
    padding: 16px;
  }
  :global(#zt-app) .note {
    margin: 14px;
  }
  .retry {
    margin-top: 8px;
  }
  .pad :global(.zt-spin) {
    animation: zt-spin 0.9s linear infinite;
  }
  .grid:focus {
    outline: none;
  }
  .grid:focus-visible {
    box-shadow: inset 0 0 0 1px var(--zt-accent);
  }
  :global(#zt-app) .grid table tbody tr.checked td {
    background: var(--zt-select-soft);
  }
  :global(#zt-app) .grid:focus table tbody tr.cursor td {
    background: var(--zt-select);
  }
  :global(#zt-app) .grid:focus table tbody tr.cursor td:first-child {
    box-shadow: inset 3px 0 0 var(--zt-accent);
  }
  :global(#zt-app) button.sort {
    font: inherit;
    color: inherit;
    letter-spacing: inherit;
    text-transform: inherit;
    cursor: pointer;
  }
  :global(#zt-app) button.sort:hover {
    color: var(--zt-text);
  }
  :global(#zt-app) button.zt-link {
    font-size: inherit;
  }
  .count {
    color: var(--zt-dim);
  }
  .target {
    color: var(--zt-dim);
    font-style: normal;
  }
  .badge {
    flex: none;
    padding: 1px 7px;
    border-radius: 999px;
    font-size: 11px;
    font-style: normal;
    font-weight: 600;
    white-space: nowrap;
  }
  .badge.gone {
    background: var(--zt-err-soft);
    color: var(--zt-err);
  }
  .badge.child {
    background: var(--zt-info-soft);
    color: var(--zt-info);
  }
  .badge.changed {
    background: var(--zt-warn-soft);
    color: var(--zt-warn);
  }
  :global(#zt-app) button.changed {
    cursor: pointer;
    border: 1px solid transparent;
  }
  :global(#zt-app) button.changed.on {
    border-color: var(--zt-warn);
  }
  @media (max-width: 560px) {
    :global(#zt-app) input.quick {
      flex: 1 1 100%;
      width: 100%;
    }
  }
</style>
