<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Segmented from '../lib/ui/Segmented.svelte';
  import ErrorBox from '../lib/ui/ErrorBox.svelte';
  import { api, ApiError } from '../lib/api';
  import { openDialog, toast } from '../lib/state.svelte';
  import { bytes, dateTime, hourLabel, plural } from '../lib/format';
  import { saveSettings, settings } from './store.svelte';
  import { TYPES, TYPE_LABEL, reach, reachText, validTarget, type Preview, type SnapType } from './types';
  import PreviewDialog from './PreviewDialog.svelte';

  let { close }: { close: (r?: unknown) => void } = $props();

  const KEYS = [
    'snap_schedule_preset',
    'snap_schedule_custom',
    'snap_daily_hour',
    'snap_free_target',
    'snap_min_free_pct',
    ...TYPES.map((t) => 'snap_' + t),
    ...TYPES.map((t) => 'snap_age_' + t)
  ];

  const initial = settings();
  const form = $state<Record<string, string>>(Object.fromEntries(KEYS.map((k) => [k, initial[k] ?? ''])));
  if (!form.snap_schedule_preset) form.snap_schedule_preset = '15min';
  if (!form.snap_schedule_custom) form.snap_schedule_custom = '*/15 * * * *';

  let saving = $state(false);
  let error = $state<unknown>(null);
  let cronProblem = $state('');
  let cronNext = $state<number[]>([]);
  let preview = $state<Preview | null>(null);
  let previewing = $state(false);

  const PRESETS: [string, string][] = [
    ['5min', '5 min'],
    ['15min', '15 min'],
    ['30min', '30 min'],
    ['hourly', 'Hourly'],
    ['custom', 'Custom']
  ];
  const PRESET_EXPR: Record<string, string> = { '5min': '*/5 * * * *', '15min': '*/15 * * * *', '30min': '*/30 * * * *', hourly: '0 * * * *' };
  let expr = $derived(form.snap_schedule_preset === 'custom' ? form.snap_schedule_custom : PRESET_EXPR[form.snap_schedule_preset] || '*/15 * * * *');

  $effect(() => {
    const e = expr;
    const t = setTimeout(async () => {
      try {
        const r = await api<{ problem: string; next: number[] }>('snap_schedule', { expr: e });
        cronProblem = r.problem;
        cronNext = r.next;
      } catch {
        cronProblem = '';
      }
    }, 300);
    return () => clearTimeout(t);
  });

  let numberProblems = $derived.by(() => {
    const out: string[] = [];
    for (const t of TYPES) {
      for (const k of ['snap_' + t, 'snap_age_' + t]) {
        const v = (form[k] ?? '').trim();
        if (v !== '' && !/^\d{1,5}$/.test(v)) out.push(`${TYPE_LABEL[t]}: "${v}" is not a whole number.`);
      }
    }
    if (!validTarget(form.snap_free_target)) out.push('Free-space target: use something like 100G or 10%.');
    const pct = form.snap_min_free_pct.trim();
    if (pct !== '' && (!/^\d+$/.test(pct) || Number(pct) > 90)) out.push('Warning threshold must be 0-90 %.');
    return out;
  });

  let snapshotKey = $derived(JSON.stringify(form));
  $effect(() => {
    void snapshotKey;
    if (numberProblems.length) return;
    const body = { ...form };
    const t = setTimeout(async () => {
      previewing = true;
      try {
        preview = await api<Preview>('snap_preview', { settings: body });
      } catch {
        preview = null;
      } finally {
        previewing = false;
      }
    }, 600);
    return () => clearTimeout(t);
  });

  let changed = $derived(KEYS.some((k) => (form[k] ?? '') !== (initial[k] ?? '')));
  let total = $derived(TYPES.reduce((a, t) => Math.max(a, reach(t, Number(form['snap_' + t]) || 0, expr)), 0));

  async function save() {
    if (numberProblems.length) {
      error = new ApiError('Please check the form.', numberProblems);
      return;
    }
    if (form.snap_schedule_preset === 'custom' && cronProblem) {
      error = new Error('The custom schedule is not valid: ' + cronProblem);
      return;
    }
    saving = true;
    error = null;
    const changes: Record<string, string> = {};
    for (const k of KEYS) changes[k] = (form[k] ?? '').trim();
    const w = await saveSettings(changes);
    saving = false;
    if (w === null) return;
    if (w.length) {
      w.forEach((m) => toast(m, 'err'));
      return;
    }
    toast('Snapshot schedule and retention saved');
    close(true);
  }
</script>

<Dialog title="Schedule & retention" icon="settings" drawer sticky={changed} onclose={() => close()}>
  <ErrorBox {error} />
  <div class="zt-form-section">
    <h3><Icon name="clock" />How often the scheduler checks</h3>
    <div class="zt-hint">
      Each check takes whatever is due and prunes what retention no longer keeps. Missed periods are caught up on the next check, and no period is
      ever taken twice - so this only limits how fine frequent snapshots can be.
    </div>
    <div class="zt-fields">
      <Field label="Interval" wide>
        <Segmented bind:value={form.snap_schedule_preset} options={PRESETS} />
      </Field>
      {#if form.snap_schedule_preset === 'custom'}
        <Field label="Cron expression" hint="minute hour day month weekday, e.g. */10 * * * *" wide>
          <input type="text" class="zt-mono" class:invalid={!!cronProblem} bind:value={form.snap_schedule_custom} spellcheck="false" />
        </Field>
      {/if}
      {#if cronProblem}
        <div class="zt-error ss-wide">{cronProblem}</div>
      {:else if cronNext.length}
        <div class="zt-next-runs ss-wide">
          {#each cronNext as t (t)}<span class="zt-chip"><Icon name="clock" />{dateTime(t)}</span>{/each}
        </div>
      {/if}
      <Field wide label="Daily, weekly, monthly, yearly from" hint="The first check at or after this hour takes them, and later checks catch up if the server was off.">
        <select bind:value={form.snap_daily_hour}>
          {#each { length: 24 } as _, h (h)}<option value={String(h)}>{hourLabel(h)}</option>{/each}
        </select>
      </Field>
    </div>
  </div>

  <div class="zt-form-section">
    <h3><Icon name="layers" />Retention</h3>
    <div class="zt-hint">
      How many of each kind to keep. Datasets set to "global retention" use these numbers. The newest snapshot of a kind that is kept is never pruned,
      neither are held snapshots or any snapshot not named <span class="zt-mono">auto-…</span>.
    </div>
    <div class="zt-table-wrap">
      <table class="zt-table ss-ret">
        <thead>
          <tr><th>Kind</th><th>Keep</th><th>Max age (days)</th><th class="zt-hide-narrow">Reaches back</th></tr>
        </thead>
        <tbody>
          {#each TYPES as t (t)}
            <tr class:ss-off={!(Number(form['snap_' + t]) > 0)}>
              <td><b>{TYPE_LABEL[t]}</b></td>
              <td><input type="text" inputmode="numeric" placeholder="0" bind:value={form['snap_' + t]} aria-label="{TYPE_LABEL[t]} snapshots to keep" /></td>
              <td><input type="text" inputmode="numeric" placeholder="none" bind:value={form['snap_age_' + t]} aria-label="{TYPE_LABEL[t]} maximum age in days" /></td>
              <td class="dim zt-hide-narrow">{reachText(reach(t as SnapType, Number(form['snap_' + t]) || 0, expr))}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <div class="zt-hint ss-gap">
      {#if total > 0}
        The oldest automatic snapshot reaches back about <b>{reachText(total)}</b>.
      {:else}
        All counts are 0 - nothing would be taken.
      {/if}
      Frequent snapshots are taken on every check (empty max age = no age limit).
    </div>
  </div>

  <div class="zt-form-section">
    <h3><Icon name="database" />Space</h3>
    <div class="zt-fields">
      <Field label="Free-space target" optional hint="Below this, each run prunes the oldest automatic snapshots of that pool until it is met. e.g. 100G or 10%">
        <input type="text" class:invalid={!validTarget(form.snap_free_target)} placeholder="off" bind:value={form.snap_free_target} />
      </Field>
      <Field label="Warn when free space drops below (%)" hint="Unraid notification at most once a day. 0 turns it off.">
        <input type="text" inputmode="numeric" bind:value={form.snap_min_free_pct} />
      </Field>
    </div>
  </div>

  <div class="zt-callout ss-preview" class:accent={!!preview && preview.pruned > 0}>
    <Icon name={previewing ? 'loader' : 'eye'} class={previewing ? 'zt-spin' : ''} />
    <div>
      {#if numberProblems.length}
        Fix the form to see what this would do.
      {:else if !preview}
        Working out what these settings would do&hellip;
      {:else}
        With these settings the next run would create <b>{plural(preview.created, 'snapshot')}</b> and prune
        <b>{preview.pruned}</b>{#if preview.freed}{' '}(about {bytes(preview.freed)}){/if}.
        {#if preview.plan.length}
          <button type="button" class="zt-link" onclick={() => openDialog(PreviewDialog, { preview })}>Show which</button>
        {/if}
      {/if}
    </div>
  </div>

  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="primary" icon="check" busy={saving} disabled={!changed} onclick={save}>Save</Button>
  {/snippet}
</Dialog>

<style>
  :global(#zt-app) .ss-wide {
    grid-column: 1 / -1;
  }
  :global(#zt-app) .ss-ret input[type='text'] {
    width: 96px;
  }
  :global(#zt-app) .ss-ret tr.ss-off td:first-child {
    color: var(--zt-dim);
  }
  :global(#zt-app) .ss-gap {
    margin-top: 10px;
  }
  :global(#zt-app) .ss-preview {
    margin-top: 18px;
  }
</style>
