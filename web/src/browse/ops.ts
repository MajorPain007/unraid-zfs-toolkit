import { api, download } from '../lib/api';
import { confirm } from '../lib/dialogs';
import { copyText, fail, openDialog, openMenu, refresh, toast } from '../lib/state.svelte';
import CompareDialog from './CompareDialog.svelte';
import RestoreDialog from './RestoreDialog.svelte';
import { B, openSnap, snapPath, type Item, type Restore, type Reveal, type Snap } from './store.svelte';

export async function restoreItems(items: Item[], snap: Snap | null = B.snap): Promise<void> {
  if (!snap || !items.length) return;
  const started = await openDialog<boolean>(RestoreDialog, { dataset: B.dataset, mountpoint: B.mountpoint, snap, items });
  if (!started) return;
  B.checked = {};
  toast('Restore started - follow it at the top of this page.');
  setTimeout(refresh, 600);
}

export function downloadPath(path: string, snap: Snap | null = B.snap): void {
  if (!snap) return;
  download({ dataset: B.dataset, snapshot: snap.name, path });
  toast(path === '/' || !/\.[^/]*$/.test(path) ? 'Preparing the download - folders arrive as a zip file.' : 'Download started.', 'info');
}

export async function compare(snap: Snap, live = false): Promise<void> {
  const list = (B.snaps || []).filter((x) => x.name !== snap.name);
  const r = await openDialog<Reveal | undefined>(CompareDialog, {
    dataset: B.dataset,
    snap,
    list,
    path: B.path,
    live: live || !list.length
  });
  if (!r) return;
  if (r.restore) {
    restoreItems([r.restore], r.snap);
    return;
  }
  openSnap(r.snap, r.path, r.focus);
}

export function snapMenu(s: Snap, anchor: Element): void {
  openMenu(anchor, [
    { label: 'Compare with current files', icon: 'compare', onclick: () => compare(s, true) },
    { label: 'Restore everything...', icon: 'restore', onclick: () => restoreItems([{ path: '/', type: 'dir', name: B.dataset }], s) },
    { label: 'Download everything as zip', icon: 'download', onclick: () => downloadPath('/', s) },
    '-',
    { label: 'Copy snapshot name', icon: 'copy', onclick: () => copyText(B.dataset + '@' + s.name) },
    { label: 'Copy path in .zfs', icon: 'copy', disabled: !B.mountpoint, onclick: () => copyText(snapPath(s, B.path)) }
  ]);
}

export async function stopRestore(r: Restore, btn: { busy: boolean }): Promise<void> {
  const yes = await confirm({
    title: 'Stop this restore?',
    text: 'Files already restored stay in place. The file being copied right now is discarded, so nothing is left half-written.',
    confirm: 'Stop restore',
    danger: true
  });
  if (!yes) return;
  btn.busy = true;
  try {
    await api('restore_stop', { id: r.id });
    toast('Stopping...', 'info');
    setTimeout(refresh, 1200);
  } catch (e) {
    fail(e);
  } finally {
    btn.busy = false;
  }
}

export async function dismissRestore(r: Restore): Promise<void> {
  try {
    await api('restore_dismiss', { id: r.id });
    refresh();
  } catch (e) {
    fail(e);
  }
}
