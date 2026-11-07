<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import { dateTime, plural, relative } from '../lib/format';
  import { STATUS_LABEL, type HistoryRun, type LogFile } from './types';

  let { runs, logs, onlog }: { runs: HistoryRun[]; logs: LogFile[]; onlog: (file: string) => void } = $props();

  let known = $derived(new Set(logs.map((l) => l.file)));

  function when(n: string): number {
    const m = /(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})/.exec(n);
    if (!m) return 0;
    return new Date(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +m[6]).getTime() / 1000;
  }
</script>

<div class="zt-grid cv-hist">
  <div class="zt-card">
    <div class="cv-card-title"><Icon name="archive" /><h3>Conversions</h3></div>
    {#if !runs.length}
      <p class="zt-hint">No folder has been converted yet. Runs that convert or fail something are kept here, also across reboots.</p>
    {:else}
      <ul class="cv-list">
        {#each runs as r (r.started + r.log_file)}
          {@const pl = STATUS_LABEL[r.status] ?? STATUS_LABEL.idle}
          <li>
            <div class="cv-list-main">
              <div class="cv-list-line">
                <b>{dateTime(r.started)}</b>
                <span class="zt-pill {pl[1]}">{pl[0]}</span>
                {#if r.source === 'auto'}<span class="cv-dim">scheduled</span>{/if}
              </div>
              {#if r.converted.length}<div class="cv-names ok">+ {r.converted.join(', ')}</div>{/if}
              {#if r.failed.length}<div class="cv-names err">− {r.failed.join(', ')}</div>{/if}
              {#if r.interrupted}<div class="cv-names cv-dim">stopped before the end</div>{/if}
            </div>
            {#if known.has(r.log_file)}<Button icon="list" small kind="ghost" title="Log" onclick={() => onlog(r.log_file)} />{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <div class="zt-card">
    <div class="cv-card-title"><Icon name="list" /><h3>Recent logs</h3></div>
    {#if !logs.length}
      <p class="zt-hint">No logs yet. Logs of the last ten runs are kept in RAM until the next reboot.</p>
    {:else}
      <ul class="cv-list">
        {#each logs as l (l.file)}
          {@const pl = STATUS_LABEL[l.status] ?? STATUS_LABEL.idle}
          <li class="clickable">
            <button type="button" class="cv-log-btn" onclick={() => onlog(l.file)}>
              <span class="zt-dot {l.status === 'running' ? 'run' : pl[1] || 'idle'}"></span>
              <span class="cv-list-main">
                <b>{relative(when(l.name) || l.mtime)}</b>
                <span class="cv-dim">{l.source === 'auto' ? 'scheduled' : 'started here'} · {pl[0].toLowerCase()}</span>
              </span>
              <Icon name="right" />
            </button>
          </li>
        {/each}
      </ul>
      <p class="zt-hint cv-foot">{plural(logs.length, 'log')} kept in RAM, cleared at reboot.</p>
    {/if}
  </div>
</div>

<style>
  .cv-hist {
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  }
  .cv-card-title {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }
  .cv-card-title :global(svg.zt-i) {
    color: var(--zt-dim);
  }
  .cv-card-title h3 {
    margin: 0;
    font-size: 14.5px;
    font-weight: 600;
  }
  .cv-list {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 320px;
    overflow: auto;
  }
  .cv-list li {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 8px 0;
    border-bottom: 1px solid var(--zt-border);
  }
  .cv-list li:last-child {
    border-bottom: 0;
  }
  .cv-list-main {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .cv-list-line {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .cv-names {
    font-size: 12.5px;
    overflow-wrap: anywhere;
  }
  .cv-names.ok {
    color: var(--zt-ok);
  }
  .cv-names.err {
    color: var(--zt-err);
  }
  .cv-dim {
    color: var(--zt-dim);
    font-size: 12.5px;
  }
  .cv-log-btn {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    margin: 0;
    padding: 2px 4px;
    border: 0;
    background: none;
    color: inherit;
    text-align: left;
    cursor: pointer;
    font: inherit;
    min-width: 0;
    text-transform: none;
    letter-spacing: normal;
  }
  .cv-log-btn:hover b {
    color: var(--zt-accent-text);
  }
  .cv-log-btn .zt-dot {
    margin: 0;
    flex: none;
  }
  .cv-log-btn :global(svg.zt-i) {
    color: var(--zt-faint);
    flex: none;
  }
  .cv-foot {
    margin: 8px 0 0;
  }
</style>
