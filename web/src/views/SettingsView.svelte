<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import DiagnosticsDialog from '../settings/DiagnosticsDialog.svelte';
  import { api } from '../lib/api';
  import { app, refresh, toast, fail, openDialog } from '../lib/state.svelte';
  import type { CronState } from '../lib/types';

  let { opts = {} }: { opts?: Record<string, any> } = $props();
  let fromOpts = $derived(opts.focus ?? '');

  interface System {
    zfs_version: string;
    unraid: string;
    kernel: string;
    timezone: string;
    tools: Record<string, boolean>;
    cron_log: string;
  }

  let sys = $state<System | null>(null);
  let saving = $state<Record<string, boolean>>({});
  let diagBusy = $state(false);

  api<System>('system').then((s) => (sys = s), fail);

  let o = $derived(app.overview);
  let st = $derived(o?.settings ?? {});

  async function setFlag(key: string, on: boolean) {
    saving[key] = true;
    try {
      const r = await api<{ warnings: string[] }>('settings_save', { settings: { [key]: on ? 'yes' : 'no' } });
      for (const w of r.warnings) toast(w, 'err');
      await refresh();
      toast(key === 'show_in_menu' ? 'Saved. The menu changes when the page is loaded again.' : 'Saved.');
    } catch (e) {
      fail(e);
    } finally {
      saving[key] = false;
    }
  }

  async function diagnostics() {
    diagBusy = true;
    try {
      const r = await api<{ name: string; report: string }>('diagnostics');
      openDialog(DiagnosticsDialog, r);
    } catch (e) {
      fail(e);
    } finally {
      diagBusy = false;
    }
  }

  const CRONS: [keyof NonNullable<typeof o>['cron'], string, string, string][] = [
    ['snapshots', 'Snapshots', 'camera', 'snapshots_enabled'],
    ['replication', 'Replication', 'send', 'send_enabled'],
    ['conversion', 'Conversion', 'convert', 'cron_enabled']
  ];

  function cronLine(c: CronState): string {
    const m = c.entry.match(/^((?:\S+\s+){4}\S+)/);
    return m ? m[1] : c.entry;
  }
</script>

{#if o}
  <div class="zt-card sv" data-focus={fromOpts}>
    <section>
      <div class="sv-head">
        <h3><Icon name="settings" />General</h3>
        <p>Applies to the whole plugin.</p>
      </div>
      <div class="sv-body">
        <div class="sv-row">
          <Toggle checked={st.send_notifications === 'yes'} label="Unraid notifications" disabled={saving.send_notifications} onchange={(v) => setFlag('send_notifications', v)} />
          <div class="zt-hint">Failed snapshots, replication errors, low pool space and finished conversions.</div>
        </div>
        <div class="sv-row">
          <Toggle checked={st.show_in_menu === 'yes'} label="Show ZFS in the top menu" disabled={saving.show_in_menu} onchange={(v) => setFlag('show_in_menu', v)} />
          <div class="zt-hint">Next to Docker and VMs. The page stays under Settings &gt; Utilities either way.</div>
        </div>
      </div>
    </section>

    <section>
      <div class="sv-head">
        <h3><Icon name="clock" />Schedules</h3>
        <p>Entries live in /boot/config/plugins/zfs.toolkit/*.cron, which Unraid reads into its crontab, so they survive reboots.</p>
      </div>
      <div class="sv-body">
        <div class="sv-list">
          {#each CRONS as [key, label, icon, flag] (key)}
            {@const c = o.cron[key]}
            {@const on = st[flag] === 'yes'}
            <div class="sv-item">
              <span class="zt-dot {!on ? 'idle' : c.live ? 'ok' : 'err'}"></span>
              <Icon name={icon} />
              <b>{label}</b>
              <span class="sv-item-val">
                {#if !on}
                  <span class="zt-sub">Off</span>
                {:else if c.live}
                  <code>{cronLine(c)}</code>
                {:else}
                  <span class="bad">Not in the crontab. Save the schedule again, or reinstall the plugin.</span>
                {/if}
              </span>
            </div>
          {/each}
        </div>
      </div>
    </section>

    <section>
      <div class="sv-head">
        <h3><Icon name="info" />About</h3>
        <p>Versions and the tools the plugin can use.</p>
      </div>
      <div class="sv-body">
        <div class="zt-facts">
          <div class="zt-fact"><div class="zt-label">Plugin</div><div class="zt-value">{o.version || '-'}</div></div>
          <div class="zt-fact"><div class="zt-label">Server</div><div class="zt-value">{o.server}</div></div>
          <div class="zt-fact"><div class="zt-label">ZFS</div><div class="zt-value" title={sys?.zfs_version}>{sys?.zfs_version || '-'}</div></div>
          <div class="zt-fact"><div class="zt-label">Unraid</div><div class="zt-value">{sys?.unraid || '-'}</div></div>
          <div class="zt-fact"><div class="zt-label">Time zone</div><div class="zt-value">{sys?.timezone || '-'}</div></div>
          <div class="zt-fact"><div class="zt-label">Array</div><div class="zt-value">{o.array ? 'Started' : 'Stopped'}</div></div>
        </div>
        {#if sys}
          <div class="tools">
            {#each Object.entries(sys.tools) as [t, ok] (t)}
              <span class="zt-pill" class:ok class:off={!ok} title={ok ? t + ' is installed' : t + ' is not installed'}>{t}</span>
            {/each}
          </div>
        {/if}
      </div>
    </section>

    <section>
      <div class="sv-head">
        <h3><Icon name="terminal" />Diagnostics</h3>
        <p>Settings, cron entries, pools and recent logs in one report, to look at or attach to a bug report.</p>
      </div>
      <div class="sv-body">
        <Button icon="eye" busy={diagBusy} onclick={diagnostics}>Show diagnostics</Button>
      </div>
    </section>
  </div>
{/if}

<style>
  .sv {
    padding: 0;
  }
  section {
    display: grid;
    grid-template-columns: minmax(200px, 280px) minmax(0, 1fr);
    gap: 24px;
    padding: 20px 22px;
    border-top: 1px solid var(--zt-border);
  }
  section:first-child {
    border-top: 0;
  }
  .sv-head h3 {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 6px;
    font-size: 14px;
    font-weight: 600;
  }
  .sv-head p {
    margin: 0;
    color: var(--zt-dim);
    font-size: 12.5px;
    line-height: 1.5;
  }
  .sv-row + .sv-row {
    margin-top: 14px;
  }
  .sv-row :global(.zt-hint) {
    margin: 4px 0 0 46px;
  }
  .sv-list {
    border: 1px solid var(--zt-border);
    border-radius: var(--zt-radius-sm);
  }
  .sv-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    border-top: 1px solid var(--zt-border);
  }
  .sv-item:first-child {
    border-top: 0;
  }
  .sv-item b {
    width: 100px;
    flex: none;
  }
  .sv-item-val {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .sv-item code {
    font-family: var(--zt-mono);
    font-size: 12.5px;
  }
  .bad {
    color: var(--zt-err);
    font-size: 12.5px;
  }
  .tools {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    margin-top: 14px;
  }
  .tools .off {
    opacity: 0.5;
    text-decoration: line-through;
  }
  @media (max-width: 760px) {
    section {
      grid-template-columns: minmax(0, 1fr);
      gap: 12px;
      padding: 16px;
    }
    .sv-item {
      flex-wrap: wrap;
    }
    .sv-item-val {
      flex-basis: 100%;
      padding-left: 26px;
    }
  }
</style>
