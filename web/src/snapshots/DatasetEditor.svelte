<script lang="ts">
  import { untrack } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { openDialog, toast } from '../lib/state.svelte';
  import { confirm } from '../lib/dialogs';
  import { bytes, plural } from '../lib/format';
  import { saveConfig, settings, snap } from './store.svelte';
  import { TYPES, TYPE_LABEL, globalKeep, keepOf, reach, reachText, scheduleExpr, type Preview, type SnapDS, type SnapType } from './types';
  import PreviewDialog from './PreviewDialog.svelte';

  let { name, close }: { name: string; close: (r?: unknown) => void } = $props();

  const s = settings();
  const orig: SnapDS = untrack(() => snap.config.find((d) => d.name === name) ?? { name, recursive: false, use_template: true });
  const vals: Record<string, string> = {};
  for (const t of TYPES) {
    const v = orig[t];
    vals[t] = orig.use_template ? String(globalKeep(s, t)) : v === undefined ? String(keepOf(orig, t, s)) : v;
  }
  const form = $state({ recursive: orig.recursive, use_template: orig.use_template, vals });
  let saving = $state(false);
  let preview = $state<Preview | null>(null);

  function build(): SnapDS {
    const d: SnapDS = { name, recursive: form.recursive, use_template: form.use_template };
    for (const t of TYPES) {
      if (form.use_template) {
        if (t !== 'frequent') d[t] = '';
      } else {
        const v = (form.vals[t] ?? '').trim();
        d[t] = v === '0' ? '' : v;
      }
    }
    return d;
  }

  let problems = $derived(TYPES.filter((t) => !form.use_template && (form.vals[t] ?? '').trim() !== '' && !/^\d{1,5}$/.test(form.vals[t].trim())));
  let key = $derived(JSON.stringify(form));

  $effect(() => {
    void key;
    if (problems.length) return;
    const list = snap.config.map((d) => (d.name === name ? build() : d));
    const t = setTimeout(async () => {
      try {
        preview = await api<Preview>('snap_preview', { datasets: list });
      } catch {
        preview = null;
      }
    }, 500);
    return () => clearTimeout(t);
  });

  let mine = $derived((preview?.plan ?? []).filter((p) => p.dataset === name || (form.recursive && p.dataset.startsWith(name + '/'))));
  let pruning = $derived(mine.filter((p) => p.op === 'prune'));
  let creating = $derived(mine.filter((p) => p.op === 'create').length);
  let changed = $derived.by(() => {
    void key;
    const b = build();
    if (b.recursive !== orig.recursive || b.use_template !== orig.use_template) return true;
    return !b.use_template && TYPES.some((t) => keepOf(b, t, s) !== keepOf(orig, t, s));
  });

  async function save() {
    if (problems.length) {
      toast('Retention must be whole numbers.', 'err');
      return;
    }
    saving = true;
    const ok = await saveConfig(snap.config.map((d) => (d.name === name ? build() : d)));
    saving = false;
    if (ok) {
      toast('Saved ' + name);
      close(true);
    }
  }

  async function remove() {
    if (
      !(await confirm({
        title: 'Stop snapshotting ' + name + '?',
        text: 'Existing snapshots stay until you delete them in the snapshot manager.',
        confirm: 'Remove from schedule',
        danger: true
      }))
    )
      return;
    if (await saveConfig(snap.config.filter((d) => d.name !== name))) close(true);
  }
</script>

<Dialog title={name} icon="database" drawer sticky={changed} onclose={() => close()}>
  <div class="zt-form-section">
    <h3><Icon name="layers" />What is snapshotted</h3>
    <Toggle bind:checked={form.recursive} label="Include child datasets (recursive)" />
    <div class="zt-hint ss-gap">
      Recursive snapshots are taken atomically for {name} and every dataset below it, and pruned together. Child datasets that you also list
      separately are covered by this one first.
    </div>
  </div>

  <div class="zt-form-section">
    <h3><Icon name="clock" />Retention</h3>
    <Toggle bind:checked={form.use_template} label="Use the global retention" />
    <div class="zt-table-wrap ss-gap">
      <table class="zt-table ss-ret">
        <thead><tr><th>Kind</th><th>Keep</th><th class="zt-hide-narrow">Reaches back</th></tr></thead>
        <tbody>
          {#each TYPES as t (t)}
            {@const n = form.use_template ? globalKeep(s, t) : Number(form.vals[t]) || 0}
            <tr class:ss-off={n <= 0}>
              <td><b>{TYPE_LABEL[t]}</b></td>
              <td>
                {#if form.use_template}
                  <span class="zt-dim">{n}</span>
                {:else}
                  <input type="text" inputmode="numeric" class:invalid={problems.includes(t)} placeholder="0" bind:value={form.vals[t]} aria-label="{TYPE_LABEL[t]} snapshots to keep" />
                {/if}
              </td>
              <td class="dim zt-hide-narrow">{reachText(reach(t as SnapType, n, scheduleExpr(s)))}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <div class="zt-hint ss-gap">Maximum ages, the free-space target and the schedule are global and apply here too.</div>
  </div>

  <div class="zt-callout" class:accent={pruning.length > 0}>
    <Icon name="eye" />
    <div>
      {#if !preview}
        Working out what this would do&hellip;
      {:else if !mine.length}
        With this the next run would not change anything here.
      {:else}
        The next run would create <b>{plural(creating, 'snapshot')}</b> and prune <b>{pruning.length}</b>{#if pruning.length}{' '}(about
          {bytes(pruning.reduce((a, p) => a + p.used, 0))}){/if} here.
        <button type="button" class="zt-link" onclick={() => openDialog(PreviewDialog, { preview, only: name })}>Show which</button>
      {/if}
    </div>
  </div>

  {#snippet actions()}
    <Button icon="trash" kind="danger" onclick={remove}>Remove</Button>
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="primary" icon="check" busy={saving} disabled={!changed} onclick={save}>Save</Button>
  {/snippet}
</Dialog>

<style>
  :global(#zt-app) .ss-gap {
    margin-top: 10px;
  }
  :global(#zt-app) .ss-ret input[type='text'] {
    width: 96px;
  }
  :global(#zt-app) .ss-ret tr.ss-off td:first-child {
    color: var(--zt-dim);
  }
</style>
