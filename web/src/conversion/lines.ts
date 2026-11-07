export interface Line {
  i: number;
  t: string;
  c: string;
}

export function classify(t: string): string {
  if (/\] ERROR:|VALIDATION FAILED/.test(t)) return 'bad';
  if (/\] WARNING:/.test(t)) return 'warn';
  if (t.startsWith('=== Step')) return 'phase';
  if (/\] OK:/.test(t)) return 'good';
  if (/^\s+[+]/.test(t)) return 'good';
  if (/^\s+-\s/.test(t)) return 'bad';
  return '';
}

export function pieces(text: string, q: string): [string, boolean][] {
  const out: [string, boolean][] = [];
  if (!q) return [[text, false]];
  const low = text.toLowerCase();
  let at = 0;
  for (;;) {
    const i = low.indexOf(q, at);
    if (i < 0) break;
    if (i > at) out.push([text.slice(at, i), false]);
    out.push([text.slice(i, i + q.length), true]);
    at = i + q.length;
  }
  if (at < text.length) out.push([text.slice(at), false]);
  return out;
}

export function saveText(name: string, text: string): void {
  const blob = new Blob([text], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 2000);
}
