import type { PartStatus } from '../lib/types';

export const TYPES = ['frequent', 'hourly', 'daily', 'weekly', 'monthly', 'yearly'] as const;
export type SnapType = (typeof TYPES)[number];

export const TYPE_LABEL: Record<SnapType, string> = {
  frequent: 'Frequent',
  hourly: 'Hourly',
  daily: 'Daily',
  weekly: 'Weekly',
  monthly: 'Monthly',
  yearly: 'Yearly'
};

export const TYPE_SECONDS: Record<SnapType, number> = {
  frequent: 0,
  hourly: 3600,
  daily: 86400,
  weekly: 7 * 86400,
  monthly: 30.44 * 86400,
  yearly: 365.25 * 86400
};

export interface SnapDS {
  name: string;
  recursive: boolean;
  use_template: boolean;
  hourly?: string;
  daily?: string;
  weekly?: string;
  monthly?: string;
  yearly?: string;
  frequent?: string;
}

export interface AvailDS {
  name: string;
  type: string;
  mountpoint: string;
  used: number;
  snapused: number;
}

export interface SnapInfo {
  name: string;
  dataset: string;
  snapshot: string;
  type: string;
  managed: boolean;
  checkpoint: boolean;
  used: number;
  refer: number;
  creation: number;
  held: boolean;
}

export interface DSInfo {
  name: string;
  used: number;
  usedbysnapshots: number;
  available: number;
  type: string;
}

export interface RunStatus {
  last_run: string;
  duration?: number;
  datasets?: number;
  created?: number;
  pruned?: number;
  freed?: number;
  errors?: number;
  last_error?: string;
  forced?: number;
  ended?: number;
  warnings?: string[];
}

export interface LowPool {
  pool: string;
  capacity: number;
  reason: 'full' | 'target';
  free?: number;
  target?: number;
}

export interface SnapOverview {
  status: PartStatus;
  count: number;
  running: boolean;
  enabled: boolean;
  configured: boolean;
  datasets: number;
  auto: number;
  total: number;
  held: number;
  used: number;
  newest: number;
  by_type: Record<string, number>;
  missing: string[];
  last?: RunStatus;
  run?: { pid: number; started: number; forced: boolean };
  next: Record<string, number>;
  tick: number;
  expr: string;
  low: LowPool[];
}

export interface PlanItem {
  op: 'create' | 'prune' | 'held' | 'fail';
  name: string;
  dataset: string;
  type: string;
  used: number;
  creation: number;
  reason: string;
}

export interface Preview {
  plan: PlanItem[];
  created: number;
  pruned: number;
  freed: number;
  errors: number;
  warnings: string[];
  lines: string[];
  now: number;
}

export function globalKeep(settings: Record<string, string>, t: SnapType): number {
  const v = (settings['snap_' + t] ?? '').trim();
  return /^\d+$/.test(v) ? Number(v) : 0;
}

export function keepOf(d: SnapDS, t: SnapType, settings: Record<string, string>): number {
  if (d.use_template) return globalKeep(settings, t);
  const v = d[t];
  if (v === undefined) return t === 'frequent' ? 0 : globalKeep(settings, t);
  return /^\d+$/.test(v.trim()) ? Number(v.trim()) : 0;
}

export function keepsOf(d: SnapDS, settings: Record<string, string>): Record<SnapType, number> {
  const out = {} as Record<SnapType, number>;
  for (const t of TYPES) out[t] = keepOf(d, t, settings);
  return out;
}

export function intervalOf(expr: string): number {
  const m = /^\*\/(\d+) \* \* \* \*$/.exec(expr.trim());
  if (m) return Number(m[1]) * 60;
  if (/^\d+ \* \* \* \*$/.test(expr.trim())) return 3600;
  return 900;
}

export function reach(t: SnapType, n: number, expr: string): number {
  if (n <= 0) return 0;
  const step = t === 'frequent' ? intervalOf(expr) : TYPE_SECONDS[t];
  return n * step;
}

export function reachText(sec: number): string {
  if (sec <= 0) return '-';
  const d = sec / 86400;
  if (sec < 3600) return Math.round(sec / 60) + ' min';
  if (sec < 86400) return Math.round(sec / 3600) + ' h';
  if (d < 14) return Math.round(d) + (Math.round(d) === 1 ? ' day' : ' days');
  if (d < 60) return Math.round(d / 7) + ' weeks';
  if (d < 365 * 1.5) return Math.round(d / 30.44) + ' months';
  return (d / 365.25).toFixed(d < 3650 ? 1 : 0).replace(/\.0$/, '') + ' years';
}

export function retentionSummary(k: Record<SnapType, number>): string {
  const parts = TYPES.filter((t) => k[t] > 0).map((t) => k[t] + ' ' + t);
  return parts.length ? parts.join(' · ') : 'No retention - nothing is taken';
}

export function scheduleExpr(settings: Record<string, string>): string {
  const map: Record<string, string> = { '5min': '*/5 * * * *', '15min': '*/15 * * * *', '30min': '*/30 * * * *', hourly: '0 * * * *' };
  const p = settings.snap_schedule_preset || '15min';
  return p === 'custom' ? settings.snap_schedule_custom || '*/15 * * * *' : map[p] || '*/15 * * * *';
}

export function scheduleText(settings: Record<string, string>): string {
  const p = settings.snap_schedule_preset || '15min';
  const map: Record<string, string> = {
    '5min': 'Checks every 5 minutes',
    '15min': 'Checks every 15 minutes',
    '30min': 'Checks every 30 minutes',
    hourly: 'Checks every hour'
  };
  return map[p] || 'Custom schedule ' + (settings.snap_schedule_custom || '');
}

export function globMatcher(pattern: string): ((s: string) => boolean) | null {
  const p = pattern.trim();
  if (!p) return null;
  if (!/[*?]/.test(p)) {
    const low = p.toLowerCase();
    return (s) => s.toLowerCase().includes(low);
  }
  const rx = new RegExp(p.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.'), 'i');
  return (s) => rx.test(s);
}

export function validTarget(v: string): boolean {
  const s = v.trim();
  return s === '' || /^[0-9]+\s*([KMGTP]i?B?|%)$/i.test(s);
}

export function kindOf(s: SnapInfo): string {
  if (s.checkpoint) return 'replication';
  if (s.type) return s.type;
  if (s.managed) return 'auto';
  return 'manual';
}
