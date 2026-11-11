export const KEYS = [
  'dry_run',
  'cleanup',
  'replace_spaces',
  'should_process_containers',
  'appdata_pool',
  'appdata_dataset',
  'should_process_vms',
  'vm_pool',
  'vm_dataset',
  'vm_forceshutdown_wait',
  'buffer_zone',
  'validation_tolerance',
  'extra_datasets',
  'cron_enabled',
  'cron_preset',
  'cron_hour',
  'cron_minute',
  'cron_weekday',
  'cron_custom'
] as const;

export type Key = (typeof KEYS)[number];
export type Form = Record<Key, string>;

export const BOOLS: Key[] = ['dry_run', 'cleanup', 'replace_spaces', 'should_process_containers', 'should_process_vms', 'cron_enabled'];

const DEFAULTS: Form = {
  dry_run: 'yes',
  cleanup: 'yes',
  replace_spaces: 'no',
  should_process_containers: 'yes',
  appdata_pool: 'cache',
  appdata_dataset: 'appdata',
  should_process_vms: 'no',
  vm_pool: 'cache',
  vm_dataset: 'domains',
  vm_forceshutdown_wait: '90',
  buffer_zone: '11',
  validation_tolerance: '5',
  extra_datasets: '',
  cron_enabled: 'no',
  cron_preset: 'daily',
  cron_hour: '2',
  cron_minute: '0',
  cron_weekday: '0',
  cron_custom: '0 2 * * *'
};

export function formFrom(s: Record<string, string> | undefined): Form {
  const f = { ...DEFAULTS };
  for (const k of KEYS) {
    const v = s?.[k];
    if (v !== undefined && v !== null && (v !== '' || k === 'extra_datasets')) f[k] = String(v);
  }
  if (f.dry_run !== 'no') f.dry_run = 'yes';
  for (const k of BOOLS) if (k !== 'dry_run') f[k] = f[k] === 'yes' ? 'yes' : 'no';
  return f;
}

export function fingerprint(f: Form): string {
  return JSON.stringify(KEYS.map((k) => String(f[k]).trim()));
}

const COMPONENT = /^[A-Za-z0-9_.: -]+$/;

export function datasetProblem(ds: string, label: string): string {
  const v = ds.trim().replace(/^\/+|\/+$/g, '');
  if (!v) return label + ' is empty.';
  const parts = v.split('/');
  if (parts.some((p) => !COMPONENT.test(p) || p === '.' || p === '..')) return label + ' "' + v + '" is not a valid dataset name.';
  if (!/^[A-Za-z0-9]/.test(parts[0]) || parts[0].includes(' ')) return label + ': the pool name "' + parts[0] + '" is not valid.';
  return '';
}

export function intProblem(v: string, lo: number, hi: number, label: string): string {
  if (!/^\d+$/.test(v.trim())) return label + ' must be a whole number.';
  const n = Number(v);
  if (n < lo || n > hi) return label + ' must be ' + lo + '-' + hi + '.';
  return '';
}

export function cronPreview(f: Form): string {
  const h = Number(f.cron_hour) || 0;
  const m = Number(f.cron_minute) || 0;
  switch (f.cron_preset) {
    case 'hourly':
      return '0 * * * *';
    case '6hourly':
      return m + ' */6 * * *';
    case 'weekly':
      return m + ' ' + h + ' * * ' + (Number(f.cron_weekday) || 0);
    case 'custom':
      return f.cron_custom.trim();
    default:
      return m + ' ' + h + ' * * *';
  }
}

export function splitExtra(v: string): string[] {
  return v
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}
