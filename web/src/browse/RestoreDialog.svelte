<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import OptionCards from '../lib/ui/OptionCards.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import { api } from '../lib/api';
  import { confirm, pickPath } from '../lib/dialogs';
  import { fail, pref } from '../lib/state.svelte';
  import { bytes, count, dateTime, plural } from '../lib/format';
  import type { Item, Snap } from './store.svelte';

  type Target = 'original' | 'path';

  interface Stats {
    files: number;
    dirs: number;
    bytes: number;
    new: number;
    replace: number;
    same: number;
    kept: number;
    remove: number;
    copy_bytes: number;
    replaced: { path: string; type: string; size: number; mtime: number; new_size: number; new_mtime: number }[];
    removed: string[];
    partial: boolean;
  }

  interface Preview {
    items: { rel: string; into: string; type: string; exists: boolean }[];
    target_dir: string;
    stats: Stats;
  }

  let {
    dataset,
    mountpoint,
    snap,
    items,
    close
  }: { dataset: string; mountpoint: string; snap: Snap; items: Item[]; close: (started?: boolean) => void } = $props();

  const OVERWRITE: [string, string][] = [
    ['changed', 'Replace files that differ from the snapshot'],
    ['never', 'Keep files that are there; only add missing ones'],
    ['always', 'Replace every file, even identical ones']
  ];

  let target = $state<Target>('original');
  let targetPath = $state(pref('restoreTarget') || '/mnt/user/restore');
  let overwrite = $state('changed');
  let del = $state(false);
  let working = $state(false);
  let preview = $state<Preview | null>(null);
  let previewError = $state('');
  let previewing = $state(false);
  let showAll = $state(false);
  let seq = 0;

  let hasDir = $derived(items.some((i) => i.type === 'dir'));
  let deleting = $derived(target === 'original' && del && hasDir);
  let st = $derived(preview?.stats);

  $effect(() => {
    const params = {
      dataset,
      snapshot: snap.name,
      items: items.map((i) => i.path),
      target,
      target_path: target === 'path' ? targetPath : '',
      overwrite,
      delete: deleting ? 1 : 0
    };
    const my = ++seq;
    previewing = true;
    const t = setTimeout(() => {
      api<Preview>('restore_preview', params)
        .then((r) => {
          if (my !== seq) return;
          preview = r;
          previewError = '';
        })
        .catch((e) => {
          if (my !== seq) return;
          preview = null;
          previewError = (e as Error).message;
        })
        .finally(() => {
          if (my === seq) previewing = false;
        });
    }, 350);
    return () => clearTimeout(t);
  });

  async function browse() {
    const start = targetPath.startsWith('/') ? targetPath : mountpoint;
    const path = await pickPath({ title: 'Restore into', create: true, start });
    if (path) targetPath = path;
  }

  async function restore() {
    if (deleting) {
      const yes = await confirm({
        title: 'Delete files that are not in the snapshot?',
        text:
          (st && st.remove ? plural(st.remove, 'file or folder', 'files and folders') + ' created since the snapshot will be deleted. ' : '') +
          'The folders end up exactly as they were. Child datasets are left alone.',
        confirm: 'Restore and delete',
        danger: true
      });
      if (!yes) return;
    }
    working = true;
    try {
      await api('restore_start', {
        dataset,
        snapshot: snap.name,
        items: items.map((i) => i.path),
        target,
        target_path: target === 'path' ? targetPath : '',
        overwrite,
        delete: deleting ? 1 : 0
      });
      if (target === 'path' && targetPath.startsWith('/')) pref('restoreTarget', targetPath);
      close(true);
    } catch (e) {
      fail(e);
    } finally {
      working = false;
    }
  }
</script>

<Dialog title={'Restore from ' + dateTime(snap.creation)} icon="restore" wide onclose={() => close()}>
  <div class="zt-pathlist">
    {#each items.slice(0, 6) as it (it.path)}
      <div class="zt-pathlist-item">
        <Icon name={it.type === 'dir' ? 'folder' : it.type === 'symlink' ? 'link' : 'file'} />
        <span class="zt-mono" title={it.path}>{it.path === '/' ? 'Everything in ' + dataset : it.path}</span>
      </div>
    {/each}
    {#if items.length > 6}
      <div class="zt-hint">and {items.length - 6} more</div>
    {/if}
  </div>
  <div class="gap-lg">
    <OptionCards
      bind:value={target}
      two
      options={[
        { value: 'original', title: 'Where it came from', icon: 'restore', text: 'Puts the files back in place, in ' + (mountpoint || dataset) + '.' },
        { value: 'path', title: 'Into another folder', icon: 'folder', text: 'Leaves the current files untouched, to compare first.' }
      ]}
    />
  </div>
  <div class="gap">
    {#if target === 'path'}
      <Field label="Restore into" hint={'Each item lands in this folder, which is created if needed. A path without a leading / is taken inside ' + (mountpoint || 'the dataset') + '.'}>
        <div class="zt-input-group">
          <input type="text" class="zt-mono" autocomplete="off" spellcheck="false" bind:value={targetPath} />
          <Button icon="folder" onclick={browse}>Browse...</Button>
        </div>
      </Field>
    {/if}
    <div class="gap-sm">
      <Field label="Files that already exist">
        <select bind:value={overwrite}>
          {#each OVERWRITE as [v, label] (v)}
            <option value={v}>{label}</option>
          {/each}
        </select>
      </Field>
    </div>
    {#if target === 'original' && hasDir}
      <div class="gap">
        <Toggle bind:checked={del} label="Also delete files that are not in the snapshot" />
        <div class="zt-hint del-hint">Makes the folders exactly as they were. Files created since are lost; child datasets are not touched.</div>
      </div>
    {/if}
  </div>

  <div class="preview" aria-live="polite">
    <div class="phead">
      <b>What happens</b>
      {#if previewing}<Icon name="loader" class="zt-spin" />{/if}
    </div>
    {#if previewError}
      <div class="zt-card-note err"><Icon name="alert" /><div>{previewError}</div></div>
    {:else if st}
      <div class="facts">
        <span><b>{count(st.files)}</b> {st.files === 1 ? 'file' : 'files'} · {bytes(st.bytes)}</span>
        {#if st.new}<span class="zt-pill ok">{count(st.new)} new</span>{/if}
        {#if st.replace}<span class="zt-pill warn">{count(st.replace)} replaced</span>{/if}
        {#if st.same}<span class="zt-pill">{count(st.same)} {overwrite === 'always' ? 'rewritten' : 'unchanged'}</span>{/if}
        {#if st.kept}<span class="zt-pill">{count(st.kept)} kept as they are</span>{/if}
        {#if st.remove}<span class="zt-pill err">{count(st.remove)} deleted</span>{/if}
      </div>
      {#if preview && target === 'path'}
        <div class="zt-hint">Lands in <span class="zt-mono">{preview.items[0]?.into}</span>{preview.items.length > 1 ? ' and next to it' : ''}.</div>
      {/if}
      {#if st.replace && st.replaced.length}
        <div class="list">
          {#each showAll ? st.replaced : st.replaced.slice(0, 6) as f (f.path)}
            <div class="row">
              <span class="zt-mono p" title={f.path}>{f.path}</span>
              <span class="dim">{bytes(f.size)}, {dateTime(f.mtime)} → {bytes(f.new_size)}, {dateTime(f.new_mtime)}</span>
            </div>
          {/each}
        </div>
      {/if}
      {#if st.remove && st.removed.length}
        <div class="list">
          {#each showAll ? st.removed : st.removed.slice(0, 6) as p (p)}
            <div class="row del"><span class="zt-mono p" title={p}>{p}</span><span class="dim">deleted</span></div>
          {/each}
        </div>
      {/if}
      {#if !showAll && (st.replaced.length > 6 || st.removed.length > 6)}
        <button type="button" class="zt-link" onclick={() => (showAll = true)}>Show more</button>
      {/if}
      {#if st.replace > st.replaced.length || st.remove > st.removed.length}
        <div class="zt-hint">Only the first 200 are listed.</div>
      {/if}
      {#if st.partial}
        <div class="zt-hint">This is a lot of data - the preview stopped counting early, so the numbers are a lower bound.</div>
      {/if}
      {#if !st.files && !st.dirs}
        <div class="zt-hint">Nothing to copy.</div>
      {/if}
    {:else}
      <div class="zt-hint">Checking what would change...</div>
    {/if}
  </div>
  {#snippet actions()}
    <div class="zt-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind={deleting ? 'danger solid' : 'primary'} icon="restore" busy={working} disabled={!!previewError} onclick={restore}>Restore</Button>
  {/snippet}
</Dialog>

<style>
  .gap-lg {
    margin-top: 18px;
  }
  .gap {
    margin-top: 14px;
  }
  .gap-sm {
    margin-top: 12px;
  }
  .del-hint {
    margin: 4px 0 0 44px;
  }
  .preview {
    margin-top: 18px;
    padding: 12px 14px;
    border: 1px solid var(--zt-border);
    border-radius: var(--zt-radius);
    background: var(--zt-surface);
    font-size: 13px;
  }
  .phead {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }
  .phead :global(.zt-spin) {
    animation: zt-spin 0.9s linear infinite;
  }
  .facts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
    margin-bottom: 6px;
  }
  .list {
    margin: 8px 0 4px;
    border-top: 1px solid var(--zt-border);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 2px 12px;
    padding: 5px 0;
    border-bottom: 1px solid var(--zt-border);
    font-size: 12px;
  }
  .row .p {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .row.del .p {
    color: var(--zt-err);
  }
  .dim {
    color: var(--zt-dim);
  }
</style>
