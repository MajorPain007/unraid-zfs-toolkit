<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { openDialog, openMenu } from '../lib/state.svelte';
  import { confirm } from '../lib/dialogs';
  import { bytes, count, dateTime, plural, relative } from '../lib/format';
  import { focusManager, saveConfig, settings, snap } from './store.svelte';
  import { keepsOf, retentionSummary, type SnapDS } from './types';
  import HeatStrip from './HeatStrip.svelte';
  import DatasetEditor from './DatasetEditor.svelte';
  import CreateDialog from './CreateDialog.svelte';
  import PreviewDialog from './PreviewDialog.svelte';

  let {
    ds,
    missing = false,
    onarm,
    onmove
  }: { ds: SnapDS; missing?: boolean; onarm?: () => void; onmove?: (dir: -1 | 1) => void } = $props();

  let idx = $derived(snap.config.findIndex((d) => d.name === ds.name));

  let s = $derived(settings());
  let keeps = $derived(keepsOf(ds, s));
  let off = $derived(Object.values(keeps).every((n) => n <= 0));
  let covered = $derived((name: string) => name === ds.name || (ds.recursive && name.startsWith(ds.name + '/')));
  let own = $derived(snap.list.filter((x) => x.dataset === ds.name));
  let all = $derived(ds.recursive ? snap.list.filter((x) => covered(x.dataset)) : own);
  let autoOwn = $derived(own.filter((x) => x.managed));
  let info = $derived(snap.datasets.filter((d) => covered(d.name)));
  let snapUsed = $derived(info.reduce((a, d) => a + d.usedbysnapshots, 0));
  let children = $derived(Math.max(0, info.length - 1));
  let newest = $derived(own.length ? Math.max(...own.map((x) => x.creation)) : 0);
  let oldest = $derived(own.length ? Math.min(...own.map((x) => x.creation)) : 0);
  let held = $derived(all.filter((x) => x.held).length);

  function edit() {
    openDialog(DatasetEditor, { name: ds.name });
  }

  async function remove() {
    const ok = await confirm({
      title: 'Stop snapshotting ' + ds.name + '?',
      text: 'No new automatic snapshots are taken and nothing is pruned there any more. The existing snapshots stay until you delete them in the snapshot manager.',
      confirm: 'Remove from schedule',
      danger: true
    });
    if (ok) saveConfig(snap.config.filter((d) => d.name !== ds.name));
  }

  function menu(e: MouseEvent) {
    openMenu(e.currentTarget as Element, [
      { label: 'Edit retention', icon: 'edit', onclick: edit },
      { label: 'Take a snapshot now', icon: 'camera', disabled: missing, onclick: () => openDialog(CreateDialog, { dataset: ds.name, recursive: ds.recursive }) },
      { label: 'Show snapshots', icon: 'list', onclick: () => focusManager(ds.name) },
      { label: 'What the next run does here', icon: 'eye', onclick: () => openDialog(PreviewDialog, { only: ds.name }) },
      '-',
      idx > 0 && onmove && { label: 'Move forward', icon: 'up', onclick: () => onmove(-1) },
      idx >= 0 && idx < snap.config.length - 1 && onmove && { label: 'Move back', icon: 'right', onclick: () => onmove(1) },
      '-',
      { label: 'Remove from schedule', icon: 'trash', danger: true, onclick: remove }
    ]);
  }
</script>

<div class="zt-card ss-ds" class:is-failed={missing} class:is-disabled={off && !missing}>
  <div class="zt-card-head">
    {#if onarm}
      <span class="ss-grip" title="Drag to reorder" role="presentation" onpointerdown={onarm}><Icon name="grip" /></span>
    {/if}
    <div class="zt-tile"><Icon name={ds.recursive ? 'layers' : 'database'} /></div>
    <div class="zt-card-title">
      <h3 title={ds.name}><button type="button" class="ss-ds-name" onclick={edit}>{ds.name}</button></h3>
      <div class="zt-meta">
        {#if ds.recursive}<span class="zt-chip" title="Snapshots include every child dataset">+ {plural(children, 'child', 'children')}</span>{/if}
        <span class="zt-chip" title={ds.use_template ? 'Uses the global retention' : 'Has its own retention'}>
          <Icon name={ds.use_template ? 'link' : 'edit'} />{ds.use_template ? 'global' : 'own'}
        </span>
        <span class="ss-ds-ret">{retentionSummary(keeps)}</span>
      </div>
    </div>
    <div class="zt-card-actions">
      <Button icon="list" small title="Show the snapshots of {ds.name}" onclick={() => focusManager(ds.name)}>Snapshots</Button>
      <Button icon="camera" small kind="ghost" title="Take a snapshot now" disabled={missing} onclick={() => openDialog(CreateDialog, { dataset: ds.name, recursive: ds.recursive })} />
      <Button icon="more" small kind="ghost" title="More" onclick={menu} />
    </div>
  </div>

  {#if missing}
    <div class="zt-card-note err">
      <Icon name="alert" />
      <div>This dataset does not exist (any more). Every run logs an error for it until it is removed from the schedule.</div>
    </div>
  {:else}
    <div class="zt-facts">
      <div class="zt-fact">
        <div class="zt-label">Snapshots</div>
        <div class="zt-value">
          {count(autoOwn.length)} auto
          {#if own.length > autoOwn.length}<small> · {own.length - autoOwn.length} other</small>{/if}
          {#if held}<small> · {held} held</small>{/if}
        </div>
      </div>
      <div class="zt-fact">
        <div class="zt-label">Space in snapshots</div>
        <div class="zt-value" title={ds.recursive ? 'Including child datasets' : ''}>{snap.listLoaded ? bytes(snapUsed) : '-'}</div>
      </div>
      <div class="zt-fact">
        <div class="zt-label">Newest</div>
        <div class="zt-value" title={newest ? dateTime(newest) : ''}>{newest ? relative(newest) : 'None yet'}</div>
      </div>
      <div class="zt-fact">
        <div class="zt-label">Oldest</div>
        <div class="zt-value" title={oldest ? dateTime(oldest) : ''}>{oldest ? relative(oldest) : '-'}</div>
      </div>
    </div>
    {#if off}
      <div class="zt-card-note warn"><Icon name="info" /><div>Every retention count is 0, so nothing is taken here.</div></div>
    {/if}
    <HeatStrip snapshots={own} />
  {/if}
</div>

<style>
  :global(#zt-app) .ss-ds h3 {
    white-space: normal;
    overflow: visible;
  }
  :global(#zt-app) .ss-ds-name {
    max-width: 100%;
    text-align: left;
    overflow-wrap: anywhere;
    white-space: normal;
    font: inherit;
    color: inherit;
    padding: 0;
    background: none;
    border: 0;
    cursor: pointer;
  }
  :global(#zt-app) .ss-ds-name:hover {
    color: var(--zt-accent-text);
  }
  :global(#zt-app) .ss-grip {
    display: flex;
    align-items: center;
    margin: 0 -4px 0 -8px;
    padding: 4px 0;
    color: var(--zt-faint);
    cursor: grab;
    touch-action: none;
  }
  :global(#zt-app) .ss-grip:hover {
    color: var(--zt-dim);
  }
  :global(#zt-app) .ss-ds-ret {
    overflow-wrap: anywhere;
  }
</style>
