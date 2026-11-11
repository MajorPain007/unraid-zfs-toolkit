export function cronProblem(expr: string): string {
  const e = expr.trim();
  if (!e) return 'Enter a cron expression.';
  if (!/^[0-9*/, \t-]+$/.test(e)) return 'Only digits, spaces and * / , - are allowed.';
  const n = e.split(/[ \t]+/).length;
  if (n !== 5) return 'A cron expression has 5 fields (minute hour day month weekday), this one has ' + n + '.';
  return '';
}
