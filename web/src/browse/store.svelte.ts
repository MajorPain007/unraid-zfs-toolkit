import { api } from '../lib/api';
import { fail, pref } from '../lib/state.svelte';

export interface Dataset {
  name: string;
  mountpoint: string;
  mounted: boolean;
  used: number;
  snapshots: number;
  latest: number;
}

export interface Snap {
  name: string;
  creation: number;
  used: number;
  refer: number;
  holds: number;
  defer: boolean;
  kind: string;
}

export interface Entry {
  name: string;
  path: string;
  type: string;
  size: number;
  mtime: number;
  mode: string;
  target?: string;
  live?: '' | 'gone' | 'changed';
  child?: string;
  child_snap?: boolean;
}

export interface Item {
  path: string;
  type: string;
  name: string;
}

export interface Restore {
  id: string;
  dataset: string;
  snapshot: string;
  target: string;
  target_dir: string;
  overwrite: string;
  delete: boolean;
  items: string[];
  total: number;
  state: 'queued' | 'running' | 'done' | 'error' | 'cancelled';
  created: number;
  started: number;
  finished: number;
  done: number;
  failed: number;
  current: string;
  bytes: number;
  bytes_done: number;
  percent: number;
  message: string;
  dismissed: boolean;
}

export interface Reveal {
  snap: Snap;
  path: string;
  focus: string;
  restore?: Item;
}

export type SortKey = '' | 'name' | 'size' | 'mtime';

export const KIND_LABEL: Record<string, string> = {
  frequent: 'Frequent',
  hourly: 'Hourly',
  daily: 'Daily',
  weekly: 'Weekly',
  monthly: 'Monthly',
  yearly: 'Yearly',
  manual: 'Manual',
  replication: 'Replication'
};

export const B = $state({
  datasets: null as Dataset[] | null,
  dsLoading: false,
  dsError: '',
  dataset: '',
  snaps: null as Snap[] | null,
  snapsLoading: false,
  snapsError: '',
  problem: '',
  mountpoint: '',
  kind: '',
  snap: null as Snap | null,
  path: '/',
  entries: null as Entry[] | null,
  listing: false,
  listError: '',
  live: false,
  liveAdded: 0,
  truncated: false,
  checked: {} as Record<string, Entry>,
  filter: '',
  changedOnly: false,
  cursor: -1,
  focus: '',
  sort: { key: '' as SortKey, desc: false },
  scroll: { list: 0, browser: 0 }
});

export function parentOf(path: string): string {
  return path.replace(/\/[^/]+$/, '') || '/';
}

export function snapPath(s: Snap, path = '/'): string {
  const mp = B.mountpoint.replace(/\/+$/, '');
  return mp + '/.zfs/snapshot/' + s.name + (path === '/' ? '' : path);
}

export async function loadDatasets(): Promise<void> {
  B.dsLoading = true;
  B.dsError = '';
  try {
    const r = await api<{ datasets: Dataset[] }>('browse_datasets');
    B.datasets = r.datasets;
  } catch (e) {
    B.dsError = (e as Error).message;
  } finally {
    B.dsLoading = false;
  }
}

export function pickDefaultDataset(): string {
  const list = (B.datasets || []).filter((d) => d.snapshots > 0);
  const saved = pref('browseDataset');
  if (saved && list.some((d) => d.name === saved)) return saved;
  let best: Dataset | null = null;
  for (const d of list) if (!best || d.latest > best.latest) best = d;
  return best ? best.name : '';
}

function resetBrowser(): void {
  B.snap = null;
  B.entries = null;
  B.checked = {};
  B.filter = '';
  B.listError = '';
  B.cursor = -1;
  B.scroll.browser = 0;
}

export async function selectDataset(name: string, then?: (snaps: Snap[]) => void): Promise<void> {
  if (name !== B.dataset) {
    B.dataset = name;
    B.snaps = null;
    B.kind = '';
    B.scroll.list = 0;
    resetBrowser();
  }
  if (name) pref('browseDataset', name);
  await loadSnaps();
  if (then && B.snaps) then(B.snaps);
}

export async function loadSnaps(): Promise<void> {
  const ds = B.dataset;
  if (!ds) return;
  B.snapsLoading = true;
  B.snapsError = '';
  try {
    const r = await api<{ snapshots: Snap[]; mountpoint: string; problem: string }>('browse_snapshots', { dataset: ds });
    if (ds !== B.dataset) return;
    B.snaps = r.snapshots;
    B.mountpoint = r.mountpoint;
    B.problem = r.problem;
    if (B.snap && !r.snapshots.some((s) => s.name === B.snap?.name)) resetBrowser();
    else if (B.snap) B.snap = r.snapshots.find((s) => s.name === B.snap?.name) ?? B.snap;
  } catch (e) {
    if (ds !== B.dataset) return;
    B.snapsError = (e as Error).message;
  } finally {
    if (ds === B.dataset) B.snapsLoading = false;
  }
}

export function openSnap(s: Snap, path = '/', focus = ''): void {
  if (B.snap?.name !== s.name) B.checked = {};
  B.snap = s;
  navigate(path, focus);
}

export function navigate(path: string, focus = '', keep = false): void {
  const snap = B.snap;
  const ds = B.dataset;
  if (!snap) return;
  B.path = path;
  B.listing = true;
  B.listError = '';
  if (!keep) {
    B.checked = {};
    B.filter = '';
    B.cursor = -1;
    B.scroll.browser = 0;
  }
  B.focus = focus;
  const stale = () => B.snap?.name !== snap.name || B.path !== path || B.dataset !== ds;
  api<{ entries: Entry[]; live: boolean; live_added: number; truncated: boolean; mountpoint: string }>('browse_list', {
    dataset: ds,
    snapshot: snap.name,
    path
  })
    .then((r) => {
      if (stale()) return;
      B.entries = r.entries || [];
      B.live = r.live;
      B.liveAdded = r.live_added;
      B.truncated = r.truncated;
      B.mountpoint = r.mountpoint || B.mountpoint;
      B.listing = false;
    })
    .catch((e) => {
      if (stale()) return;
      const msg = (e as Error).message;
      if (path !== '/' && /is not a folder/.test(msg)) {
        navigate(parentOf(path), path);
        return;
      }
      B.listing = false;
      B.entries = [];
      B.listError = msg;
    });
}

export function reload(): void {
  if (B.snap) navigate(B.path, '', true);
}

export function goUp(): void {
  if (B.path === '/') return;
  const from = B.path;
  navigate(parentOf(B.path), from);
}

export async function openAt(dataset: string, snapshot: string, path = '/'): Promise<void> {
  if (!B.datasets) await loadDatasets();
  await selectDataset(dataset, (snaps) => {
    const s = snaps.find((x) => x.name === snapshot);
    if (s) openSnap(s, path || '/');
    else fail(new Error('Snapshot ' + dataset + '@' + snapshot + ' no longer exists.'));
  });
}
