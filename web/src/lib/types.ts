export interface CronState {
  entry: string;
  live: boolean;
  source: string;
  healthy: boolean;
}

export interface Pool {
  name: string;
  size: number;
  alloc: number;
  free: number;
  capacity: number;
  health: string;
}

export interface PartStatus {
  level: 'ok' | 'warn' | 'err' | 'run' | 'idle';
  text: string;
  sub?: string;
}

export interface Overview {
  version: string;
  server: string;
  zfs: boolean;
  pools: Pool[];
  settings: Record<string, string>;
  cron: { conversion: CronState; snapshots: CronState; replication: CronState };
  array: boolean;
  now: number;
  [part: string]: any;
}
