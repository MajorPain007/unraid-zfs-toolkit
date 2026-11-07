<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import { bytes, plural } from '../lib/format';
  import { sourceIcon, sourceLabel, type ConvFolder, type ConvSourceScan } from './types';

  let {
    src,
    filter,
    selected,
    measuring = false,
    ontoggle,
    onall
  }: {
    src: ConvSourceScan;
    filter: 'all' | 'todo' | 'problems';
    selected: Record<string, boolean>;
    measuring?: boolean;
    ontoggle: (path: string, on: boolean) => void;
    onall: (paths: string[], on: boolean) => void;
  } = $props();

  const REASON: Record<string, string> = {
    space: 'Not enough space',
    collision: 'Name clash',
    temp_exists: '_temp exists',
    mounts: 'Mount inside',
    bad_name: 'Unusable name',
    kept_copy: 'Kept copy',
    unmounted_dataset: 'Dataset not mounted'
  };

  let shown = $derived(
    src.folders.filter((f) => (filter === 'all' ? true : filter === 'todo' ? f.state === 'convert' : f.state === 'kept' || f.state === 'blocked'))
  );
  let todo = $derived(src.folders.filter((f) => f.state === 'convert'));
  let picked = $derived(todo.filter((f) => selected[f.path]).length);
  let datasets = $derived(src.folders.filter((f) => f.state === 'dataset').length);

  function pill(f: ConvFolder): [string, string] {
    if (f.state === 'dataset') return ['ok', 'Dataset'];
    if (f.state === 'convert') return ['accent', 'To convert'];
    if (f.state === 'kept') return ['warn', REASON[f.reason] ?? 'Left alone'];
    return ['err', REASON[f.reason] ?? 'Blocked'];
  }

  function size(f: ConvFolder): string {
    if (f.size >= 0) return bytes(f.size);
    if (f.state === 'dataset') return '';
    return measuring ? '...' : '?';
  }
</script>

<div class="zt-card cv-src">
  <div class="zt-card-head">
    <div class="zt-tile"><Icon name={sourceIcon(src.kind)} /></div>
    <div class="zt-card-title">
      <h3>{src.dataset}</h3>
      <div class="zt-meta">
        {sourceLabel(src.kind)}
        {#if src.mountpoint} · <span class="zt-mono">{src.mountpoint}</span>{/if}
        {#if !src.error} · {bytes(src.avail)} free{/if}
      </div>
    </div>
    {#if !src.error && src.folders.length}
      <div class="cv-src-count">
        {#if todo.length}<span class="zt-pill accent">{todo.length} to convert</span>{/if}
        <span class="zt-pill ok">{plural(datasets, 'dataset')}</span>
      </div>
    {/if}
  </div>

  {#if src.error}
    <div class="zt-card-note err"><Icon name="alert" /><div>{src.error}</div></div>
  {:else if !src.folders.length}
    <div class="cv-empty">No folders in {src.mountpoint}.</div>
  {:else if !shown.length}
    <div class="cv-empty">{filter === 'todo' ? 'Nothing to convert here.' : 'No problems here.'}</div>
  {:else}
    <div class="cv-rows" role="list">
      {#if todo.length > 1 && filter !== 'problems'}
        <label class="cv-row cv-head">
          <input type="checkbox" checked={picked === todo.length} indeterminate={picked > 0 && picked < todo.length} onchange={(e) => onall(todo.map((f) => f.path), e.currentTarget.checked)} />
          <span class="cv-dim">Select all {todo.length} folders to convert</span>
        </label>
      {/if}
      {#each shown as f (f.path)}
        {@const pl = pill(f)}
        <div class="cv-row {f.state}" role="listitem">
          <span class="cv-check">
            {#if f.state === 'convert'}
              <input type="checkbox" aria-label="Convert {f.name}" checked={!!selected[f.path]} onchange={(e) => ontoggle(f.path, e.currentTarget.checked)} />
            {:else}
              <Icon name={f.state === 'dataset' ? 'database' : f.state === 'kept' ? 'pause' : 'alert'} />
            {/if}
          </span>
          <div class="cv-name">
            <div class="cv-name-line">
              <b title={f.path}>{f.name}</b>
              <span class="zt-pill {pl[0]}">{pl[1]}</span>
            </div>
            {#if f.state === 'convert' && f.as}
              <div class="cv-sub">becomes dataset <span class="zt-mono">{f.as}</span> - the folder keeps its path</div>
            {:else if f.state === 'dataset' && f.dataset && f.dataset !== f.source + '/' + f.name}
              <div class="cv-sub">dataset <span class="zt-mono">{f.dataset}</span></div>
            {/if}
            {#if f.why}<div class="cv-why {f.state}">{f.why}</div>{/if}
            {#if f.users.length}
              <div class="cv-users">
                {#each f.users as u (u.kind + u.name)}
                  <span class="zt-chip" class:cv-running={u.running} title={u.running ? (f.state === 'convert' ? 'Running - it is stopped during the copy and started again afterwards' : 'Running') : 'Not running'}>
                    <Icon name={u.kind === 'vm' ? 'server' : 'box'} />{u.name}
                    {#if u.running && f.state === 'convert'}<span class="cv-stop">stops</span>{/if}
                  </span>
                {/each}
              </div>
            {/if}
          </div>
          <span class="cv-size">{size(f)}</span>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .cv-src h3 {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .cv-src-count {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
  .cv-empty {
    margin-top: 12px;
    color: var(--zt-dim);
    font-size: 13px;
  }
  .cv-rows {
    margin-top: 12px;
    border-top: 1px solid var(--zt-border);
  }
  .cv-row {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 9px 4px;
    border-bottom: 1px solid var(--zt-border);
  }
  .cv-row:last-child {
    border-bottom: 0;
  }
  .cv-head {
    cursor: pointer;
    font-size: 12.5px;
    align-items: center;
  }
  .cv-check {
    flex: none;
    width: 18px;
    display: flex;
    justify-content: center;
    padding-top: 2px;
    color: var(--zt-faint);
  }
  .cv-row.dataset .cv-check {
    color: var(--zt-ok);
  }
  .cv-row.kept .cv-check {
    color: var(--zt-warn);
  }
  .cv-row.blocked .cv-check {
    color: var(--zt-err);
  }
  input[type='checkbox'] {
    margin: 0;
    accent-color: var(--zt-accent);
    width: 15px;
    height: 15px;
  }
  .cv-name {
    flex: 1 1 auto;
    min-width: 0;
  }
  .cv-name-line {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .cv-name-line b {
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .cv-row.dataset .cv-name-line b {
    font-weight: 500;
    color: var(--zt-dim);
  }
  .cv-sub,
  .cv-why {
    font-size: 12px;
    color: var(--zt-dim);
    margin-top: 3px;
    overflow-wrap: anywhere;
  }
  .cv-why.blocked {
    color: var(--zt-err);
  }
  .cv-why.kept {
    color: var(--zt-warn);
  }
  .cv-users {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
    margin-top: 6px;
  }
  .cv-running :global(svg.zt-i) {
    color: var(--zt-ok);
  }
  .cv-stop {
    font-size: 11px;
    color: var(--zt-warn);
    font-weight: 600;
  }
  .cv-size {
    flex: none;
    font-size: 12.5px;
    color: var(--zt-dim);
    white-space: nowrap;
    padding-top: 1px;
  }
  .cv-dim {
    color: var(--zt-dim);
  }
</style>
