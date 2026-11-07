<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { bytes, plural } from '../lib/format';
  import type { ConvFolder, ConvUser } from './types';

  let {
    folders,
    dry,
    cleanup,
    close
  }: { folders: ConvFolder[]; dry: boolean; cleanup: boolean; close: (r: boolean) => void } = $props();

  let users = $derived.by(() => {
    const seen = new Map<string, ConvUser & { folders: string[] }>();
    for (const f of folders) {
      for (const u of f.users) {
        if (!u.running) continue;
        const k = u.kind + ':' + u.name;
        const e = seen.get(k) ?? { ...u, folders: [] };
        e.folders.push(f.name);
        seen.set(k, e);
      }
    }
    return [...seen.values()].sort((a, b) => a.kind.localeCompare(b.kind) || a.name.localeCompare(b.name));
  });
  let total = $derived(folders.reduce((n, f) => n + Math.max(0, f.size), 0));
  let unknown = $derived(folders.some((f) => f.size < 0));
  let renamed = $derived(folders.filter((f) => f.as).length);
</script>

<Dialog title={dry ? 'Dry run' : 'Convert ' + plural(folders.length, 'folder')} icon="convert" wide onclose={() => close(false)}>
  {#if dry}
    <div class="zt-callout accent cv-gap"><Icon name="eye" /><div><b>Nothing is changed.</b> The run goes through the same checks and writes in the log what it would do, including which containers and VMs it would stop.</div></div>
  {:else}
    <div class="zt-callout warn cv-gap"><Icon name="alert" /><div>Each folder is renamed to <span class="zt-mono">name_temp</span>, copied into a new dataset at the same path, and checked. {cleanup ? 'Only after a matching copy is the original deleted.' : 'The original is kept as name_temp (Cleanup is off).'} If anything fails, or you press Stop, the folder is put back the way it was.</div></div>
  {/if}

  <h4 class="cv-h">{plural(folders.length, 'folder')}{total ? ' · ' + bytes(total) + (unknown ? '+' : '') : ''}</h4>
  <ul class="cv-plan">
    {#each folders as f (f.path)}
      <li>
        <Icon name="folder" />
        <span class="cv-plan-name" title={f.path}>{f.name}</span>
        <span class="cv-arrow">→</span>
        <span class="zt-mono cv-plan-ds" title={f.dataset}>{f.dataset}</span>
        <span class="cv-plan-size">{f.size >= 0 ? bytes(f.size) : ''}</span>
      </li>
    {/each}
  </ul>
  {#if renamed}
    <p class="zt-hint">{plural(renamed, 'folder')} cannot be used as a dataset name as it is and {renamed === 1 ? 'gets' : 'get'} a different one. The folder path stays the same, so nothing that uses it needs to change.</p>
  {/if}

  <h4 class="cv-h">{dry ? 'Would be stopped' : 'Stopped during the copy'}</h4>
  {#if users.length}
    <ul class="cv-plan">
      {#each users as u (u.kind + u.name)}
        <li>
          <Icon name={u.kind === 'vm' ? 'server' : 'box'} />
          <span class="cv-plan-name">{u.name}</span>
          <span class="cv-plan-size">{u.kind === 'vm' ? 'VM' : 'container'} · uses {u.folders.join(', ')}</span>
        </li>
      {/each}
    </ul>
    <p class="zt-hint">Each is started again as soon as all folders are done. One whose folder could not be put back after a failure stays stopped, so it cannot run on a half-copied folder.</p>
  {:else}
    <p class="zt-hint">No running container or VM uses these folders. The list is checked again right before the copy.</p>
  {/if}

  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button onclick={() => close(false)}>Cancel</Button>
    <Button kind="primary" icon={dry ? 'eye' : 'play'} onclick={() => close(true)}>{dry ? 'Start dry run' : 'Convert now'}</Button>
  {/snippet}
</Dialog>

<style>
  .cv-gap {
    margin-bottom: 6px;
  }
  .cv-h {
    margin: 18px 0 8px;
    font-size: 13px;
    font-weight: 600;
  }
  .cv-plan {
    list-style: none;
    margin: 0;
    padding: 0;
    border: 1px solid var(--zt-border);
    border-radius: var(--zt-radius-sm);
    max-height: 260px;
    overflow: auto;
  }
  .cv-plan li {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 10px;
    border-bottom: 1px solid var(--zt-border);
    font-size: 13px;
    min-width: 0;
  }
  .cv-plan li:last-child {
    border-bottom: 0;
  }
  .cv-plan :global(svg.zt-i) {
    flex: none;
    color: var(--zt-dim);
  }
  .cv-plan-name {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .cv-arrow {
    color: var(--zt-faint);
  }
  .cv-plan-ds {
    font-size: 12px;
    color: var(--zt-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
    flex: 1 1 auto;
  }
  .cv-plan-size {
    margin-left: auto;
    flex: none;
    font-size: 12px;
    color: var(--zt-dim);
    white-space: nowrap;
  }
  p.zt-hint {
    margin: 8px 0 0;
  }
  @media (max-width: 560px) {
    .cv-plan li {
      flex-wrap: wrap;
      row-gap: 2px;
    }
    .cv-arrow {
      display: none;
    }
    .cv-plan-name {
      flex: 1 1 auto;
    }
    .cv-plan-ds {
      order: 5;
      flex-basis: 100%;
      padding-left: 24px;
    }
  }
</style>
