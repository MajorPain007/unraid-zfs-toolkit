import { api } from '../lib/api';
import { confirm } from '../lib/dialogs';
import { fail, refresh, toast } from '../lib/state.svelte';
import { bytes } from '../lib/format';

export interface SendJob {
  id: string;
  enabled: boolean;
  name: string;
  source: string;
  dest: string;
  recursive: boolean;
  transport: 'local' | 'ssh';
  ssh_host: string;
  ssh_port: number;
  ssh_key: string;
  raw: boolean;
  compressed: boolean;
  allow_rollback: boolean;
  keep_dest: number;
  readonly: boolean;
  bwlimit: number;
}

export interface SendResult {
  run: string;
  status: 'ok' | 'error' | 'cancelled' | 'running';
  dry: boolean;
  started: number;
  ended: number;
  bytes: number;
  estimate: number;
  full: boolean;
  base: string;
  checkpoint: string;
  error: string;
}

export interface SendProgress {
  job: string;
  name: string;
  phase: string;
  bytes: number;
  total: number;
  rate: number;
  started: number;
}

export interface SendState {
  running: boolean;
  pid: number;
  progress: SendProgress | null;
  current_id: string;
  queue: string[] | null;
  dry_run: boolean;
  started: number;
  ended: number;
  jobs: Record<string, { last: SendResult | null; recent: SendResult[] | null }>;
  last_run: string;
  errors: number;
  last_error: string;
}

export interface SendKey {
  path: string;
  exists: boolean;
  public: string;
  fingerprint: string;
}

export interface SendCheck {
  level: 'ok' | 'info' | 'warn' | 'err';
  text: string;
}

export interface SendPlan {
  checks: SendCheck[];
  fatal: boolean;
  dest_exists: boolean;
  full: boolean;
  base: string;
  base_creation: number;
  parents: string[];
  rollback: string[];
  checkpoint: string;
  estimate: number;
  estimate_note: string;
  commands: string[];
}

export interface SendDataset {
  name: string;
  used: number;
  avail: number;
  mountpoint: string;
  encryption: string;
  children: number;
  snapshots: number;
  readonly: boolean;
  replica: boolean;
}

export const replication = $state({
  jobs: [] as SendJob[],
  state: null as SendState | null,
  key: null as SendKey | null,
  loaded: false,
  error: ''
});

export async function loadJobs(): Promise<void> {
  try {
    const r = await api<{ jobs: SendJob[]; state: SendState; key: SendKey }>('send_jobs');
    replication.jobs = r.jobs;
    replication.state = r.state;
    replication.key = r.key;
    replication.error = '';
  } catch (e) {
    replication.error = (e as Error).message;
  } finally {
    replication.loaded = true;
  }
}

export function blankJob(jobs: SendJob[]): SendJob {
  let n = 1;
  while (jobs.some((j) => j.id === 'job' + n)) n++;
  return {
    id: 'job' + n,
    enabled: true,
    name: '',
    source: '',
    dest: '',
    recursive: true,
    transport: 'local',
    ssh_host: '',
    ssh_port: 22,
    ssh_key: '',
    raw: false,
    compressed: true,
    allow_rollback: false,
    keep_dest: 0,
    readonly: true,
    bwlimit: 0
  };
}

export function target(j: SendJob): string {
  return j.transport === 'ssh' ? (j.ssh_host || '?') + ':' + (j.dest || '?') : j.dest || '?';
}

export function hostOnly(h: string): string {
  const i = h.indexOf('@');
  return i >= 0 ? h.slice(i + 1) : h;
}

export function lastOf(id: string): SendResult | null {
  return replication.state?.jobs?.[id]?.last ?? null;
}

export function recentOf(id: string): SendResult[] {
  return replication.state?.jobs?.[id]?.recent ?? [];
}

export function isRunning(id: string): boolean {
  const s = replication.state;
  return !!s?.running && s.current_id === id;
}

export function isQueued(id: string): boolean {
  const s = replication.state;
  return !!s?.running && s.current_id !== id && (s.queue ?? []).includes(id);
}

export function jobStatus(j: SendJob): [string, string] {
  if (isRunning(j.id)) return ['run', replication.state?.dry_run ? 'Dry run' : 'Replicating'];
  if (isQueued(j.id)) return ['run', 'Waiting'];
  if (!j.enabled) return ['idle', 'Disabled'];
  const last = lastOf(j.id);
  if (!last) return ['idle', 'Never run'];
  if (last.status === 'ok') return ['ok', 'Succeeded'];
  if (last.status === 'cancelled') return ['warn', 'Stopped'];
  return ['err', 'Failed'];
}

export function resultText(r: SendResult): string {
  if (r.status === 'ok') {
    if (r.dry) return 'dry run';
    return (r.full ? 'full, ' : '') + bytes(r.bytes);
  }
  if (r.status === 'cancelled') return 'stopped';
  if (r.status === 'running') return 'running';
  return 'failed';
}

export function needsForce(text: string): boolean {
  return /turn on Force|newer than the last common one|written to the destination/.test(text);
}

export async function runJobs(ids: string[], opts: { dry?: boolean; force?: boolean } = {}): Promise<boolean> {
  try {
    const r = await api<{ state: SendState }>('send_run', { ids: ids.length ? ids : undefined, dry_run: opts.dry ? 1 : 0, force: opts.force ? 1 : 0 });
    replication.state = r.state;
    toast(opts.dry ? 'Dry run started' : 'Replication started');
    refresh();
    return true;
  } catch (e) {
    fail(e);
    return false;
  }
}

export async function stopRun(): Promise<void> {
  try {
    const r = await api<{ state: SendState }>('send_stop');
    replication.state = r.state;
    toast('Stopped', 'info');
  } catch (e) {
    fail(e);
  }
  refresh();
  loadJobs();
}

export async function setEnabled(j: SendJob, on: boolean): Promise<void> {
  try {
    const r = await api<{ jobs: SendJob[] }>('send_toggle', { id: j.id, enabled: on ? 1 : 0 });
    replication.jobs = r.jobs;
    toast(on ? `"${j.name}" enabled` : `"${j.name}" disabled`);
    refresh();
  } catch (e) {
    fail(e);
  }
}

export async function deleteJob(j: SendJob): Promise<boolean> {
  const yes = await confirm({
    title: `Delete "${j.name}"?`,
    text:
      'The job is removed. The copy on the destination stays as it is. Its checkpoint snapshots on this server (' +
      j.source +
      '@zdc-send-' +
      j.id +
      '-...) are removed too - they are only useful to this job.',
    confirm: 'Delete',
    danger: true
  });
  if (!yes) return false;
  try {
    const r = await api<{ jobs: SendJob[]; removed: string[] }>('send_delete', { id: j.id, checkpoints: 1 });
    replication.jobs = r.jobs;
    toast(`"${j.name}" deleted` + (r.removed.length ? `, ${r.removed.length} checkpoint set(s) removed` : ''));
    refresh();
    return true;
  } catch (e) {
    fail(e);
    return false;
  }
}

export function lines(text: string): { t: string; c: string }[] {
  return text
    .split('\n')
    .filter((t) => t !== '')
    .map((t) => ({ t, c: classify(t) }));
}

export function classify(t: string): string {
  if (/ ERROR: /.test(t)) return 'bad';
  if (/ WARNING: /.test(t)) return 'warn';
  if (/ OK: /.test(t)) return 'good';
  if (/────/.test(t)) return 'head';
  return '';
}
