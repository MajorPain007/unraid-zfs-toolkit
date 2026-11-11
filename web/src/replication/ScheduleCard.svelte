<script lang="ts">
  import { untrack } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import Segmented from '../lib/ui/Segmented.svelte';
  import { api } from '../lib/api';
  import { cronProblem } from '../lib/cron';
  import { hourLabel } from '../lib/format';
  import { app, fail, refresh, toast } from '../lib/state.svelte';

  type Preset = 'hourly' | '6hourly' | 'daily' | 'weekly' | 'custom';

  const s = untrack(() => app.overview?.settings ?? {});
  let enabled = $state(s.send_enabled === 'yes');
  let preset = $state<Preset>((['hourly', '6hourly', 'daily', 'weekly', 'custom'].includes(s.send_schedule_preset) ? s.send_schedule_preset : 'daily') as Preset);
  let hour = $state(Number(s.send_schedule_hour ?? 4) || 0);
  let custom = $state(s.send_schedule_custom || '0 4 * * *');
  let saving = $state(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  let cron = $derived(app.overview?.cron?.replication ?? null);
  let customProblem = $derived(preset === 'custom' ? cronProblem(custom) : '');
  let summary = $derived.by(() => {
    const hh = hourLabel(hour);
    switch (preset) {
      case 'hourly':
        return 'every hour at :15';
      case '6hourly':
        return 'every 6 hours at :15 (00:15, 06:15, 12:15, 18:15)';
      case 'daily':
        return 'every day at ' + hh;
      case 'weekly':
        return 'every Sunday at ' + hh;
      default:
        return custom;
    }
  });

  function changed() {
    if (timer) clearTimeout(timer);
    timer = setTimeout(save, 600);
  }

  async function save() {
    if (customProblem) return;
    saving = true;
    try {
      const r = await api<{ warnings: string[] }>('settings_save', {
        settings: {
          send_enabled: enabled ? 'yes' : 'no',
          send_schedule_preset: preset,
          send_schedule_hour: String(Math.max(0, Math.min(23, Math.round(hour || 0)))),
          send_schedule_custom: custom.trim()
        }
      });
      for (const w of r.warnings || []) toast(w, 'err');
      if (!r.warnings?.length) toast(enabled ? 'Schedule saved: ' + summary : 'Scheduled replication is off');
      refresh();
    } catch (e) {
      fail(e);
    } finally {
      saving = false;
    }
  }
</script>

<div class="zr-sched">
  <div class="zr-sched-row">
    <Toggle bind:checked={enabled} label="Replicate on a schedule" onchange={changed} />
    {#if enabled}
      <span class="zt-hint zr-sched-sum">
        {#if saving}<Icon name="loader" class="zt-spin" />{/if}
        Runs every enabled job {preset === 'custom' ? 'on "' + custom + '"' : summary}.
        {#if cron && !saving}
          {#if cron.live}<span class="zt-ok-text">Active.</span>{:else if cron.entry}<span class="zt-warn-text">Written, not live yet in the crontab.</span>{/if}
        {/if}
      </span>
    {:else}
      <span class="zt-hint">Jobs only run when started here.</span>
    {/if}
  </div>
  {#if enabled}
    <div class="zr-sched-row zr-sched-opts">
      <Segmented
        bind:value={preset}
        options={[
          ['hourly', 'Hourly'],
          ['6hourly', 'Every 6 h'],
          ['daily', 'Daily'],
          ['weekly', 'Weekly'],
          ['custom', 'Custom']
        ]}
        onchange={changed}
      />
      {#if preset === 'daily' || preset === 'weekly'}
        <label class="zt-inline zr-sched-hour">
          at
          <select bind:value={hour} onchange={changed} aria-label="Hour">
            {#each { length: 24 } as _, h (h)}<option value={h}>{hourLabel(h)}</option>{/each}
          </select>
        </label>
      {:else if preset === 'custom'}
        <input type="text" class="zt-mono zr-sched-cron" class:invalid={!!customProblem} placeholder="0 4 * * *" spellcheck="false" bind:value={custom} oninput={changed} aria-label="Cron expression" />
      {/if}
    </div>
    {#if customProblem}<div class="zt-error">{customProblem}</div>{/if}
  {/if}
</div>

<style>
  .zr-sched {
    border: 1px solid var(--zt-border);
    border-radius: var(--zt-radius);
    background: var(--zt-surface);
    padding: 12px 16px;
    margin-bottom: 18px;
    display: grid;
    gap: 10px;
  }
  .zr-sched-row {
    display: flex;
    align-items: center;
    gap: 8px 16px;
    flex-wrap: wrap;
  }
  .zr-sched-sum {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .zr-sched-hour select {
    width: 100px;
  }
  .zr-sched-cron {
    max-width: 180px;
  }
</style>
