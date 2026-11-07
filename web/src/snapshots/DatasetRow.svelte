<script lang="ts">
  import { slide } from 'svelte/transition';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { actions } from '../lib/actions';
  import { openDialog, openMenu, show } from '../lib/state.svelte';
  import { confirm } from '../lib/dialogs';
  import { bytes, count, dateTime, plural, relative } from '../lib/format';
  import { openRows, saveConfig, settings, snap } from './store.svelte';
  import { keepsOf, retentionSummary, type SnapDS } from './types';
  import MiniStrip from './MiniStrip.svelte';
  import Manager from './Manager.svelte';
  import DatasetEditor from './DatasetEditor.svelte';
  import CreateDialog from './CreateDialog.svelte';
  import PreviewDialog from './PreviewDialog.svelte';

  let { ds, missing = false }: { ds: SnapDS; missing?: boolean } = $props();

  let s = $derived(settings());
  let keeps = $derived(keepsOf(ds, s));
  let off = $derived(Object.values(keeps).every((n) => n <= 0));
  let covers = $derived((name: string) => name === ds.name || (ds.recursive && name.startsWith(ds.name + '/')));
  let all = $derived(snap.list.filter((x) => covers(x.dataset)));
  let own = $derived(all.filter((x) => x.dataset === ds.name));
  let autoCount = $derived(all.filter((x) => x.managed).length);
  let held = $derived(all.filter((x) => x.held).length);
  let info = $derived(snap.datasets.filter((d) => covers(d.name)));
  let snapUsed = $derived(info.reduce((a, d) => a + d.usedbysnapshots, 0));
  let children = $derived(Math.max(0, info.length - 1));
  let newest = $derived(own.reduce<(typeof own)[number] | null>((a, x) => (!a || x.creation > a.creation ? x : a), null));
  let open = $derived(openRows.has(ds.name));

  function toggle() {
    if (open) openRows.delete(ds.name);
    else openRows.add(ds.name);
  }

  function edit() {
    openDialog(DatasetEditor, { name: ds.name });
  }

  function browseNewest() {
    if (!newest) return;
    const arg = { snapshot: newest.snapshot, dataset: newest.dataset };
    if (actions.browseSnapshot) actions.browseSnapshot(arg);
    else show('browse', arg);
  }

  async function remove() {
    const ok = await confirm({
      title: 'Stop snapshotting ' + ds.name + '?',
      text: 'No new automatic snapshots are taken and nothing is pruned there any more. The existing snapshots stay until you delete them.',
      confirm: 'Remove from schedule',
      danger: true
    });
    if (ok) {
      openRows.delete(ds.name);
      saveConfig(snap.config.filter((d) => d.name !== ds.name));
    }
  }

  function menu(e: MouseEvent) {
    openMenu(e.currentTarget as Element, [
      { label: 'Take a snapshot now', icon: 'camera', disabled: missing, onclick: () => openDialog(CreateDialog, { dataset: ds.name, recursive: ds.recursive }) },
      { label: 'Edit retention', icon: 'edit', onclick: edit },
      { label: 'Browse the newest snapshot', icon: 'folder', disabled: !newest, onclick: browseNewest },
      { label: 'What the next run does here', icon: 'eye', onclick: () => openDialog(PreviewDialog, { only: ds.name }) },
      '-',
      { label: 'Remove from schedule…', icon: 'trash', danger: true, onclick: remove }
    ]);
  }
</script>

<div class="ss-row" class:open class:off={off && !missing} class:missing id="ss-row-{ds.name}">
  <div class="ss-row-line">
    <button type="button" class="ss-row-main" aria-expanded={open} onclick={toggle}>
      <span class="ss-chev"><Icon name="right" /></span>
      <span class="ss-name">
        <Icon name={ds.recursive ? 'layers' : 'database'} />
        <b title={ds.name}>{ds.name}</b>
        {#if ds.recursive}<span class="zt-chip ss-kids" title="Recursive: every child dataset is included">+{children}</span>{/if}
      </span>
      <span class="ss-rule" title={retentionSummary(keeps)}>
        {#if missing}
          <span class="zt-pill err"><Icon name="alert" />not found</span>
        {:else if off}
          <span class="zt-pill warn">nothing kept</span>
        {:else}
          <span class="zt-pill" class:accent={!ds.use_template}>{ds.use_template ? 'global' : 'own'}</span>
        {/if}
      </span>
      <span class="ss-meta">
      <span class="ss-num" title="{autoCount} automatic{held ? `, ${held} held` : ''}{all.length > autoCount ? `, ${all.length - autoCount} other` : ''}">
        <span class="ss-cap"><Icon name="camera" /></span>{count(autoCount)}{#if held}<Icon name="lock" />{/if}
      </span>
      <span class="ss-num" title={ds.recursive ? 'Space held only by snapshots, including child datasets' : 'Space held only by snapshots'}>
        <span class="ss-cap"><Icon name="database" /></span>{snap.listLoaded ? bytes(snapUsed) : '-'}
      </span>
      <span class="ss-newest" title={newest ? dateTime(newest.creation) : ''}>
        <span class="ss-cap"><Icon name="clock" /></span>{newest ? relative(newest.creation) : 'none yet'}
      </span>
      </span>
      <span class="ss-strip"><MiniStrip snapshots={own} /></span>
    </button>
    <Button icon="more" small kind="ghost" title="More" onclick={menu} />
  </div>
  {#if open}
    <div class="ss-row-body" transition:slide={{ duration: 160 }}>
      {#if missing}
        <div class="zt-card-note err"><Icon name="alert" /><div>{ds.name} does not exist (any more). Every run logs an error until it is removed from the schedule.</div></div>
      {:else}
        <div class="ss-row-facts">
          <span>{retentionSummary(keeps)}</span>
          {#if ds.recursive}<span>· {plural(children, 'child dataset')} included</span>{/if}
          <button type="button" class="zt-link" onclick={edit}>Edit retention</button>
        </div>
        <Manager scope={ds.name} items={all} />
      {/if}
    </div>
  {/if}
</div>

<style>
  :global(#zt-app) .ss-row {
    border-bottom: 1px solid var(--zt-border);
  }
  :global(#zt-app) .ss-row:last-child {
    border-bottom: 0;
  }
  :global(#zt-app) .ss-row-line {
    display: flex;
    align-items: center;
    gap: 4px;
    padding-right: 8px;
  }
  :global(#zt-app) .ss-row-line:hover {
    background: var(--zt-surface-2);
  }
  :global(#zt-app) .ss-row.open > .ss-row-line {
    background: var(--zt-select-soft);
  }
  :global(#zt-app) button.ss-row-main {
    flex: 1 1 auto;
    min-width: 0;
    display: grid;
    grid-template-columns: var(--ss-cols);
    align-items: center;
    gap: 12px;
    padding: 10px 6px 10px 12px;
    border: 0;
    background: transparent;
    color: var(--zt-text);
    font: inherit;
    font-size: 13px;
    text-align: left;
    cursor: pointer;
  }
  :global(#zt-app) .ss-chev {
    display: grid;
    place-items: center;
    color: var(--zt-dim);
    transition: transform 0.15s;
  }
  :global(#zt-app) .ss-row.open .ss-chev {
    transform: rotate(90deg);
  }
  :global(#zt-app) .ss-name {
    display: flex;
    align-items: center;
    gap: 7px;
    min-width: 0;
  }
  :global(#zt-app) .ss-name > :global(svg.zt-i) {
    flex: none;
    color: var(--zt-dim);
  }
  :global(#zt-app) .ss-name b {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 600;
  }
  :global(#zt-app) .ss-kids {
    flex: none;
  }
  :global(#zt-app) .ss-row.off .ss-name b {
    color: var(--zt-dim);
  }
  :global(#zt-app) .ss-num {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 4px;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  :global(#zt-app) .ss-num :global(svg.zt-i) {
    width: 12px;
    height: 12px;
    color: var(--zt-accent-text);
  }
  :global(#zt-app) .ss-newest {
    color: var(--zt-dim);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  :global(#zt-app) .ss-meta {
    display: contents;
  }
  :global(#zt-app) .ss-cap {
    display: none;
  }
  :global(#zt-app) .ss-row-body {
    padding: 12px 14px 16px 40px;
    border-top: 1px solid var(--zt-border);
    background: var(--zt-surface);
  }
  :global(#zt-app) .ss-row-facts {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 10px;
    align-items: center;
    margin-bottom: 10px;
    font-size: 12.5px;
    color: var(--zt-dim);
  }
  @media (max-width: 760px) {
    :global(#zt-app) button.ss-row-main {
      grid-template-columns: 16px minmax(0, 1fr) auto;
      grid-template-areas:
        'chev name rule'
        '. meta meta';
      row-gap: 4px;
      column-gap: 8px;
    }
    :global(#zt-app) .ss-chev {
      grid-area: chev;
    }
    :global(#zt-app) .ss-name {
      grid-area: name;
    }
    :global(#zt-app) .ss-rule {
      grid-area: rule;
    }
    :global(#zt-app) .ss-meta {
      grid-area: meta;
      display: flex;
      flex-wrap: wrap;
      gap: 2px 14px;
      font-size: 12px;
      color: var(--zt-dim);
    }
    :global(#zt-app) .ss-cap {
      display: inline-flex;
      margin-right: 2px;
    }
    :global(#zt-app) .ss-cap :global(svg.zt-i) {
      width: 12px;
      height: 12px;
      color: var(--zt-faint);
    }
    :global(#zt-app) .ss-newest {
      display: flex;
      align-items: center;
    }
    :global(#zt-app) .ss-strip {
      display: none;
    }
    :global(#zt-app) .ss-row-body {
      padding: 10px 10px 14px;
    }
  }
</style>
