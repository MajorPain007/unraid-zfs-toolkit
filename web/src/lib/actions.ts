export type Action = (...args: any[]) => any;

export const actions: Record<string, Action> = {};

export function provide(part: Record<string, Action>): void {
  Object.assign(actions, part);
}

export function act(name: string, ...args: any[]): any {
  const fn = actions[name];
  if (!fn) throw new Error(name + ' is not available yet');
  return fn(...args);
}
