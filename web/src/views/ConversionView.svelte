<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Segmented from '../lib/ui/Segmented.svelte';
  import RunCard from '../conversion/RunCard.svelte';
  import FolderList from '../conversion/FolderList.svelte';
  import ConvSettings from '../conversion/ConvSettings.svelte';
  import History from '../conversion/History.svelte';
  import LogDialog from '../conversion/LogDialog.svelte';
  import PlanDialog from '../conversion/PlanDialog.svelte';
  import { api } from '../lib/api';
  import { confirm } from '../lib/dialogs';
  import { app, busy, fail, onRefresh, openDialog, pref, refresh, toast } from '../lib/state.svelte';
  import { plural } from '../lib/format';
  import type { ConvFolder, ConvStatus, HistoryRun, LogFile, ScanResult } from '../conversion/types';

  let { opts = {} }: { opts?: Record<string, any> } = $props();

  let scan = $state<ScanResult | null>(null);
  let scanning = $state(false);
  let scanError = $state('');
  let st = $state<ConvStatus | null>(null);
  let runs = $state<HistoryRun[]>([]);
  let logs = $state<LogFile[]>([]);
  let selected = $state<Record<string, boolean>>({});
  let known = new Set<string>();
  let filter = $state<'all' | 'todo' | 'problems'>((pref('conv.filter') as 'all' | 'todo' | 'problems') || 'all');
  let starting = $state({ busy: false });
  let dryOff = $state({ busy: false });
  let dismissed = $state(Number(pref('conv.dismissed') || 0));
  let timer: ReturnType<typeof setTimeout> | null = null;
  let alive = true;

  let settings = $derived(app.overview?.settings ?? {});
  let dryOn = $derived((settings.dry_run ?? 'yes') !== 'no');
  let running = $derived(!!st?.running);
  let todo = $derived<ConvFolder[]>(scan ? scan.sources.flatMap((s) => s.folders.filter((f) => f.state === 'convert')) : []);
  let chosen = $derived(todo.filter((f) => selected[f.path]));
  let problemCount = $derived(scan ? scan.counts.kept + scan.counts.blocked : 0);
  let showRun = $derived(!!st && (st.running || (st.status !== 'idle' && (st.started ?? 0) !== dismissed)));
  let noSources = $derived(!!scan && !scan.sources.length);

  async function loadStatus() {
    try {
      const prev = st?.running;
      st = await api<ConvStatus>('convert_status');
      if (prev && !st.running) {
        refresh();
        loadHistory();
        rescan();
        const r = st.result;
        if (r) toast(r.message, st.status === 'error' ? 'err' : 'ok');
      }
    } catch (e) {
      if (alive) fail(e);
    }
    schedule();
  }

  function schedule() {
    if (timer) clearTimeout(timer);
    timer = null;
    if (alive && st?.running) timer = setTimeout(loadStatus, 1500);
  }

  async function loadHistory() {
    try {
      const r = await api<{ runs: HistoryRun[]; logs: LogFile[] }>('convert_history');
      runs = r.runs;
      logs = r.logs;
    } catch {
      runs = [];
    }
  }

  async function rescan() {
    if (scanning) return;
    scanning = true;
    try {
      const r = await api<ScanResult>('scan_folders');
      const next: Record<string, boolean> = {};
      for (const s of r.sources) {
        for (const f of s.folders) {
          if (f.state !== 'convert') continue;
          next[f.path] = known.has(f.path) ? !!selected[f.path] : true;
          known.add(f.path);
        }
      }
      selected = next;
      scan = r;
      scanError = '';
      if (!st || !st.running) st = r.status;
    } catch (e) {
      scanError = (e as Error).message;
    } finally {
      scanning = false;
    }
  }

  onMount(() => {
    loadStatus();
    loadHistory();
    rescan();
    if (opts.log) showLog(opts.log);
  });

  const off = onRefresh((o) => {
    if (o.conversion?.running && !st?.running) loadStatus();
  });

  onDestroy(() => {
    alive = false;
    off();
    if (timer) clearTimeout(timer);
  });

  function showLog(file?: string) {
    openDialog(LogDialog, { file: file ?? '' });
  }

  async function start(dry: boolean) {
    const folders = chosen;
    if (!folders.length) return;
    const ok = await openDialog<boolean>(PlanDialog, { folders, dry, cleanup: (settings.cleanup ?? 'yes') === 'yes' });
    if (!ok) return;
    try {
      await busy(starting, api('convert_start', { dry_run: dry ? 'yes' : 'no', only: folders.map((f) => f.path) }));
      toast(dry ? 'Dry run started.' : 'Conversion started.', 'info');
      await loadStatus();
      refresh();
      loadHistory();
    } catch (e) {
      fail(e);
    }
  }

  async function turnOffDry() {
    const yes = await confirm({
      title: 'Turn off dry run?',
      text: 'Runs then really convert folders - the ones you start here, and the scheduled ones if a schedule is on. Each folder is copied and checked before its original is removed.',
      confirm: 'Turn off dry run'
    });
    if (!yes) return;
    try {
      const r = await busy(dryOff, api<{ warnings: string[] }>('settings_save', { settings: { dry_run: 'no' } }));
      r.warnings?.forEach((w) => toast(w, 'err'));
      toast('Dry run is off.');
      refresh();
    } catch (e) {
      fail(e);
    }
  }

  function toggle(path: string, on: boolean) {
    selected[path] = on;
  }

  function all(paths: string[], on: boolean) {
    for (const p of paths) selected[p] = on;
  }

  function dismiss() {
    dismissed = st?.started ?? 0;
    pref('conv.dismissed', String(dismissed));
  }
</script>

{#if dryOn}
  <div class="cv-banner">
    <Icon name="eye" />
    <div>
      <b>Dry run is on.</b> Every conversion only simulates and changes nothing{settings.cron_enabled === 'yes' ? ', the scheduled one too' : ''}. Use it to check what would happen, then turn it off.
    </div>
    <Button small busy={dryOff.busy} onclick={turnOffDry}>Turn off</Button>
  </div>
{/if}

{#if st?.journal && !st.running}
  {@const j = st.journal}
  <div class="zt-callout cv-alert">
    <Icon name="alert" />
    <div>
      <b>The conversion of "{j.entry}" was cut off.</b>
      {#if j.committed}
        The copy in <span class="zt-mono">{j.dataset}</span> was already checked; the next real run deletes the rest of the original.
      {:else}
        The next real run settles it by itself when it can: it puts the original back if nothing was copied yet, or finishes if the copy matches. Otherwise nothing is touched and you decide:
        keep the dataset by deleting <span class="zt-mono">{j.temp}</span>, or destroy <span class="zt-mono">{j.dataset}</span> and rename <span class="zt-mono">{j.temp}</span> back to <span class="zt-mono">{j.path}</span>.
      {/if}
    </div>
  </div>
{/if}

{#if st && showRun}
  <div class="cv-gap"><RunCard {st} onlog={showLog} ondismiss={st.running ? undefined : dismiss} /></div>
{/if}

<div class="zt-section-head">
  <h2>Folders</h2>
  <Button icon="refresh" small kind="ghost" busy={scanning} onclick={rescan}>Rescan</Button>
</div>

{#if scanError}
  <div class="zt-card-note err cv-gap"><Icon name="alert" /><div>{scanError}</div></div>
{/if}

{#if !scan}
  <div class="zt-empty"><Icon name="loader" class="big zt-spin" /><h3>Scanning folders</h3><p>Measuring the folders can take a moment on large appdata shares.</p></div>
{:else}
  {#each scan.problems as p, i (i)}
    <div class="zt-card-note warn cv-gap"><Icon name="alert" /><div>{p}</div></div>
  {/each}
  {#if scan.error}
    <div class="zt-card-note err cv-gap"><Icon name="alert" /><div>{scan.error}</div></div>
  {/if}

  {#if noSources}
    <div class="zt-empty">
      <Icon name="convert" class="big" />
      <h3>Nothing to convert from yet</h3>
      <p>Turn on Docker appdata or VM domains below, or add an extra dataset.</p>
    </div>
  {:else}
    <div class="zt-toolbar cv-toolbar">
      <Segmented bind:value={filter} options={[['all', 'All'], ['todo', `To convert (${scan.counts.convert})`], ['problems', `Left alone (${problemCount})`]]} onchange={(v) => pref('conv.filter', v)} />
      <div class="zt-spacer"></div>
      {#if !dryOn}
        <Button icon="eye" disabled={running || !chosen.length} onclick={() => start(true)}>Dry run</Button>
      {/if}
      <Button kind="primary" icon={dryOn ? 'eye' : 'play'} disabled={running || !chosen.length} busy={starting.busy} onclick={() => start(dryOn)}>
        {dryOn ? 'Dry run' : 'Convert'}{chosen.length ? ' ' + plural(chosen.length, 'folder') : ''}
      </Button>
    </div>
    {#if running}
      <div class="zt-hint cv-gap">A conversion is running. Starting another one waits until it is done.</div>
    {:else if !scan.counts.convert}
      <div class="cv-allgood"><Icon name="check" /> Every folder that can be a dataset is one.{problemCount ? ' ' + plural(problemCount, 'folder is', 'folders are') + ' left alone - see why below.' : ''}</div>
    {/if}
    {#each scan.service_notes ?? [] as n, i (i)}
      <div class="zt-hint cv-gap">{n}</div>
    {/each}
    <div class="cv-sources">
      {#each scan.sources as s (s.dataset)}
        <FolderList src={s} {filter} {selected} measuring={scanning} ontoggle={toggle} onall={all} />
      {/each}
    </div>
  {/if}
{/if}

<div class="zt-section-head"><h2>Settings</h2></div>
<ConvSettings onsaved={rescan} />

<div class="zt-section-head"><h2>History</h2></div>
<History {runs} {logs} onlog={showLog} />

<style>
  .cv-banner {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 14px;
    padding: 12px 16px;
    border-radius: var(--zt-radius);
    border: 1px solid var(--zt-accent);
    background: var(--zt-accent-soft);
    font-size: 13.5px;
  }
  .cv-banner > :global(svg.zt-i) {
    color: var(--zt-accent-text);
    flex: none;
    width: 20px;
    height: 20px;
  }
  .cv-banner > div {
    flex: 1 1 auto;
    min-width: 0;
  }
  .cv-alert {
    margin-bottom: 14px;
    background: var(--zt-err-soft);
    border-color: transparent;
  }
  .cv-alert > :global(svg.zt-i) {
    color: var(--zt-err);
  }
  .cv-gap {
    margin-bottom: 14px;
  }
  .cv-toolbar {
    margin-bottom: 12px;
  }
  .cv-allgood {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
    color: var(--zt-ok);
    font-size: 13px;
  }
  .cv-sources {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  @media (max-width: 560px) {
    .cv-banner {
      flex-wrap: wrap;
    }
  }
</style>
