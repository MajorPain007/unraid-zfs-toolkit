import { SvelteSet } from 'svelte/reactivity';
import { api } from '../lib/api';
import { provide } from '../lib/actions';
import { app, fail, refresh, show, toast, listLayout } from '../lib/state.svelte';
import type { AvailDS, DSInfo, SnapDS, SnapInfo, SnapOverview } from './types';

export const snap = $state({
  config: [] as SnapDS[],
  configLoaded: false,
  configError: '',
  available: [] as AvailDS[],
  list: [] as SnapInfo[],
  datasets: [] as DSInfo[],
  listLoaded: false,
  listError: '',
  loading: false,
  filterDataset: '',
  filterSeq: 0,
  allOpen: false
});

export const openRows = new SvelteSet<string>();

export function rootOf(dataset: string): string {
  const own = snap.config.find((d) => d.name === dataset);
  if (own) return own.name;
  return snap.config.find((d) => d.recursive && dataset.startsWith(d.name + '/'))?.name ?? '';
}

export function focusManager(dataset: string): void {
  const root = listLayout() ? rootOf(dataset) : '';
  if (root) {
    openRows.add(root);
    setTimeout(() => document.getElementById('ss-row-' + root)?.scrollIntoView({ behavior: 'smooth', block: 'start' }), 60);
    return;
  }
  snap.allOpen = true;
  snap.filterDataset = dataset;
  snap.filterSeq++;
  setTimeout(() => document.getElementById('ss-manager')?.scrollIntoView({ behavior: 'smooth', block: 'start' }), 60);
}

export function part(): SnapOverview | null {
  const v = app.overview?.snapshots;
  return v && typeof v === 'object' && v.status ? (v as SnapOverview) : null;
}

export function settings(): Record<string, string> {
  return app.overview?.settings ?? {};
}

export async function loadConfig(): Promise<void> {
  try {
    const r = await api<{ datasets: SnapDS[]; available: AvailDS[] }>('snap_datasets');
    snap.config = r.datasets;
    snap.available = r.available;
    snap.configLoaded = true;
    snap.configError = '';
  } catch (e) {
    snap.configLoaded = false;
    snap.configError = (e as Error).message;
  }
}

let listSeq = 0;

export async function loadList(): Promise<void> {
  const seq = ++listSeq;
  snap.loading = true;
  try {
    const r = await api<{ snapshots: SnapInfo[]; datasets: DSInfo[] }>('snap_list');
    if (seq !== listSeq) return;
    snap.list = r.snapshots;
    snap.datasets = r.datasets;
    snap.listLoaded = true;
    snap.listError = '';
  } catch (e) {
    if (seq === listSeq) snap.listError = (e as Error).message;
  } finally {
    if (seq === listSeq) snap.loading = false;
  }
}

export function reloadAll(): Promise<unknown> {
  return Promise.all([loadList(), refresh()]);
}

export async function saveConfig(next: SnapDS[]): Promise<boolean> {
  if (!snap.configLoaded) {
    toast('The dataset list has not been read yet - reload the page before changing it.', 'err');
    return false;
  }
  try {
    const r = await api<{ datasets: SnapDS[] }>('snap_datasets_save', { datasets: next });
    snap.config = r.datasets;
    refresh();
    return true;
  } catch (e) {
    fail(e);
    return false;
  }
}

export async function saveSettings(changes: Record<string, string>): Promise<string[] | null> {
  try {
    const r = await api<{ warnings: string[] }>('settings_save', { settings: changes });
    await refresh();
    return r.warnings || [];
  } catch (e) {
    fail(e);
    return null;
  }
}

export async function setEnabled(on: boolean): Promise<void> {
  const w = await saveSettings({ snapshots_enabled: on ? 'yes' : 'no' });
  if (w === null) return;
  if (w.length) w.forEach((m) => toast(m, 'err'));
  else toast(on ? 'Automatic snapshots are on' : 'Automatic snapshots are off');
}

export async function runNow(all = false): Promise<void> {
  try {
    await api('snap_run', all ? { all: 1 } : {});
    toast('Snapshot run started');
    setTimeout(() => reloadAll(), 1500);
    refresh();
  } catch (e) {
    fail(e);
  }
}

export async function addDataset(name: string, recursive = false): Promise<boolean> {
  if (!snap.configLoaded) await loadConfig();
  if (snap.config.some((d) => d.name === name)) return true;
  return saveConfig([...snap.config, { name, recursive, use_template: true, hourly: '', daily: '', weekly: '', monthly: '', yearly: '' }]);
}

provide({
  snapshotsOfDataset: (dataset: string) => show('snapshots', { dataset }),
  addSnapshotDataset: (name: string, recursive = false) => addDataset(name, recursive),
  snapshotDatasets: async () => {
    if (!snap.configLoaded) await loadConfig();
    return snap.config.map((d) => d.name);
  }
});
