<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { bytes, dateTime, plural } from '../lib/format';
  import type { PlanItem, Preview, SnapDS } from './types';

  let {
    now = false,
    settings = undefined,
    datasets = undefined,
    preview = undefined,
    only = '',
    close
  }: {
    now?: boolean;
    settings?: Record<string, string>;
    datasets?: SnapDS[];
    preview?: Preview;
    only?: string;
    close: (r?: unknown) => void;
  } = $props();

  let data = $state<Preview | null>(null);
  let error = $state('');

  $effect(() => {
    if (preview) {
      data = preview;
      return;
    }
    api<Preview>('snap_preview', { now: now ? 1 : 0, settings, datasets }).then(
      (r) => (data = r),
      (e) => (error = (e as Error).message)
    );
  });

  let items = $derived(
    (data?.plan ?? []).filter((p) => !only || p.dataset === only || p.dataset.startsWith(only + '/'))
  );
  let groups = $derived.by(() => {
    const m = new Map<string, PlanItem[]>();
    for (const it of items) {
      if (!m.has(it.dataset)) m.set(it.dataset, []);
      m.get(it.dataset)!.push(it);
    }
    return [...m.entries()];
  });
  let created = $derived(items.filter((i) => i.op === 'create').length);
  let pruned = $derived(items.filter((i) => i.op === 'prune'));
  let freed = $derived(pruned.reduce((a, i) => a + i.used, 0));

  const REASON: Record<string, string> = { count: 'beyond the count', age: 'too old', space: 'free space', held: 'held - kept' };
</script>

<Dialog title={now ? 'What a run now would do' : 'What the next run would do'} icon="eye" wide onclose={() => close()}>
  {#if error}
    <div class="zt-errors">{error}</div>
  {:else if !data}
    <p class="zt-dim"><Icon name="loader" class="zt-spin" /> Working it out&hellip;</p>
  {:else}
    <p>
      <b>{plural(created, 'snapshot')} created</b>, <b class:zt-err-text={pruned.length > 0}>{pruned.length} pruned</b>{#if freed}{' '}(about {bytes(freed)}
        freed){/if}. Nothing has changed yet{now ? '' : ' - the scheduler applies this on its next run'}.
    </p>
    {#each (data.warnings ?? []).filter((w) => !/DRY RUN/.test(w)) as w, i (i)}
      <div class="zt-card-note warn ss-pv-note"><Icon name="alert" /><div>{w}</div></div>
    {/each}
    {#if !groups.length}
      <div class="zt-empty ss-pv-empty">Nothing is due and nothing would be pruned.</div>
    {/if}
    {#each groups as [ds, list] (ds)}
      <h4 class="ss-pv-head"><Icon name="database" />{ds}</h4>
      <div class="zt-table-wrap">
        <table class="zt-table">
          <tbody>
            {#each list as it (it.op + it.name)}
              <tr class:ss-pv-gone={it.op === 'prune'}>
                <td class="shrink">
                  {#if it.op === 'create'}
                    <span class="zt-pill ok"><Icon name="plus" />new</span>
                  {:else if it.op === 'prune'}
                    <span class="zt-pill err"><Icon name="trash" />prune</span>
                  {:else if it.op === 'held'}
                    <span class="zt-pill accent"><Icon name="lock" />held</span>
                  {:else}
                    <span class="zt-pill warn"><Icon name="alert" />fails</span>
                  {/if}
                </td>
                <td class="zt-mono ss-pv-name">{it.name.slice(it.name.indexOf('@') + 1)}</td>
                <td class="dim zt-hide-narrow">{it.op === 'create' ? '' : dateTime(it.creation)}</td>
                <td class="dim">{REASON[it.reason] ?? it.reason}</td>
                <td class="num">{it.op === 'prune' ? bytes(it.used) : ''}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/each}
  {/if}
</Dialog>

<style>
  :global(#zt-app) .ss-pv-head {
    display: flex;
    align-items: center;
    gap: 7px;
    margin: 18px 0 8px;
    font-size: 13.5px;
  }
  :global(#zt-app) .ss-pv-name {
    overflow-wrap: anywhere;
  }
  :global(#zt-app) .ss-pv-gone .ss-pv-name {
    text-decoration: line-through;
    text-decoration-color: var(--zt-err);
  }
  :global(#zt-app) .ss-pv-note {
    margin: 0 0 8px;
  }
  :global(#zt-app) .ss-pv-empty {
    padding: 24px;
  }
</style>
