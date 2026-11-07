export interface ConvUser {
  kind: 'container' | 'vm';
  name: string;
  running: boolean;
}

export type FolderState = 'dataset' | 'convert' | 'kept' | 'blocked';

export interface ConvFolder {
  source: string;
  name: string;
  path: string;
  dataset: string;
  as: string;
  state: FolderState;
  reason: string;
  why: string;
  size: number;
  bytes: number;
  users: ConvUser[];
}

export interface ConvSourceScan {
  dataset: string;
  kind: 'appdata' | 'domains' | 'extra';
  mountpoint: string;
  avail: number;
  error: string;
  folders: ConvFolder[];
}

export interface ConvItem {
  path: string;
  name: string;
  source: string;
  dataset: string;
  as: string;
  state: 'pending' | 'running' | 'done' | 'failed' | 'skipped' | 'cancelled';
  message: string;
  size: number;
  bytes: number;
  done: number;
  phase: string;
  rate: number;
}

export interface ConvResult {
  converted: string[];
  failed: string[];
  skipped: string[];
  interrupted: boolean;
  message: string;
  errors: number;
  warnings: number;
}

export interface ConvJournal {
  entry: string;
  source: string;
  dataset: string;
  temp: string;
  path: string;
  committed: boolean;
}

export interface ConvStatus {
  status: 'idle' | 'running' | 'completed' | 'error' | 'stopped';
  running: boolean;
  pid?: number;
  log_file?: string;
  started?: number;
  finished?: number;
  source?: string;
  engine?: string;
  dry_run?: boolean;
  phase?: string;
  current_folder?: string;
  items?: ConvItem[];
  stopped?: ConvUser[];
  result?: ConvResult | null;
  journal: ConvJournal | null;
}

export interface ScanResult {
  sources: ConvSourceScan[];
  counts: { convert: number; dataset: number; kept: number; blocked: number };
  problems: string[];
  service_notes?: string[];
  dry_run: boolean;
  error?: string;
  status: ConvStatus;
}

export interface HistoryRun {
  started: number;
  finished: number;
  source: string;
  status: string;
  dry_run: boolean;
  converted: string[];
  failed: string[];
  interrupted: boolean;
  log_file: string;
}

export interface LogFile {
  file: string;
  name: string;
  source: string;
  mtime: number;
  size: number;
  status: string;
}

export const STATUS_LABEL: Record<string, [string, string]> = {
  running: ['Running', 'run'],
  completed: ['Completed', 'ok'],
  error: ['Problems', 'err'],
  stopped: ['Stopped', 'warn'],
  idle: ['Idle', '']
};

export const PHASE_LABEL: Record<string, string> = {
  starting: 'Starting',
  planning: 'Planning',
  stopping: 'Stopping containers and VMs',
  measuring: 'Measuring',
  creating: 'Creating the dataset',
  copying: 'Copying',
  validating: 'Checking the copy',
  cleanup: 'Removing the original',
  restarting: 'Restarting containers and VMs'
};

export function sourceLabel(kind: string): string {
  if (kind === 'appdata') return 'Docker appdata';
  if (kind === 'domains') return 'VM domains';
  return 'Extra dataset';
}

export function sourceIcon(kind: string): string {
  if (kind === 'appdata') return 'box';
  if (kind === 'domains') return 'server';
  return 'database';
}
