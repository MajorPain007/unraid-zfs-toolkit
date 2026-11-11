<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import Segmented from '../lib/ui/Segmented.svelte';
  import ErrorBox from '../lib/ui/ErrorBox.svelte';
  import { api } from '../lib/api';
  import { app, busy, refresh, toast } from '../lib/state.svelte';
  import { cronProblem } from '../lib/cron';
  import { cronPreview, datasetProblem, fingerprint, formFrom, intProblem, splitExtra, type Form } from './settings';

  let { onsaved }: { onsaved?: () => void } = $props();

  const initial = formFrom(app.overview?.settings);
  let f = $state<Form>({ ...initial });
  let saved = $state<Form>({ ...initial });
  let error = $state<unknown>(null);
  let saving = $state({ busy: false });
  let datasets = $state<{ name: string; mountpoint: string }[]>([]);
  let addExtra = $state('');

  let dirty = $derived(fingerprint(f) !== fingerprint(saved));
  let pools = $derived((app.overview?.pools ?? []).map((p) => p.name));
  let extras = $derived(splitExtra(f.extra_datasets));
  let cron = $derived(app.overview?.cron?.conversion);

  let problems = $derived.by(() => {
    const out: string[] = [];
    if (f.should_process_containers === 'yes') {
      const p = datasetProblem(f.appdata_pool + '/' + f.appdata_dataset, 'Appdata dataset');
      if (p) out.push(p);
    }
    if (f.should_process_vms === 'yes') {
      const p = datasetProblem(f.vm_pool + '/' + f.vm_dataset, 'VM dataset');
      if (p) out.push(p);
      const w = intProblem(f.vm_forceshutdown_wait, 10, 600, 'Shutdown timeout');
      if (w) out.push(w);
    }
    for (const e of extras) {
      const p = datasetProblem(e, 'Extra dataset');
      if (p) out.push(p);
    }
    const b = intProblem(f.buffer_zone, 0, 100, 'Buffer zone');
    if (b) out.push(b);
    const t = intProblem(f.validation_tolerance, 0, 20, 'Validation tolerance');
    if (t) out.push(t);
    if (f.cron_enabled === 'yes' && f.cron_preset === 'custom') {
      const c = cronProblem(f.cron_custom);
      if (c) out.push('Schedule: ' + c);
    }
    return out;
  });

  $effect(() => {
    const incoming = app.overview?.settings;
    if (!incoming) return;
    const copy = formFrom(incoming);
    untrack(() => {
      if (dirty || fingerprint(copy) === fingerprint(saved)) return;
      f = { ...copy };
      saved = copy;
    });
  });

  onMount(async () => {
    try {
      const r = await api<{ datasets: { name: string; mountpoint: string }[] }>('convert_datasets');
      datasets = r.datasets;
    } catch {
      datasets = [];
    }
  });

  function bool(k: keyof Form): boolean {
    return f[k] === 'yes';
  }

  function setBool(k: keyof Form, v: boolean) {
    f[k] = v ? 'yes' : 'no';
  }

  function dsUnder(pool: string): string[] {
    return datasets.filter((d) => d.name.startsWith(pool + '/') && d.mountpoint.startsWith('/')).map((d) => d.name.slice(pool.length + 1));
  }

  function addExtraDataset() {
    const v = addExtra.trim().replace(/^\/+|\/+$/g, '');
    if (!v) return;
    if (!extras.includes(v)) f.extra_datasets = [...extras, v].join(',');
    addExtra = '';
  }

  function removeExtra(e: string) {
    f.extra_datasets = extras.filter((x) => x !== e).join(',');
  }

  async function save() {
    try {
      const payload: Record<string, string> = {};
      for (const [k, v] of Object.entries(f)) payload[k] = String(v).trim();
      payload.extra_datasets = extras.join(',');
      const r = await busy(saving, api<{ settings: Record<string, string>; warnings: string[] }>('settings_save', { settings: payload }));
      error = null;
      saved = formFrom(r.settings);
      f = { ...saved };
      if (r.warnings?.length) r.warnings.forEach((w) => toast(w, 'err'));
      else toast('Conversion settings saved.');
      await refresh();
      onsaved?.();
    } catch (e) {
      error = e;
    }
  }

  const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
</script>

<datalist id="cv-ds-appdata">{#each dsUnder(f.appdata_pool) as d (d)}<option value={d}></option>{/each}</datalist>
<datalist id="cv-ds-vm">{#each dsUnder(f.vm_pool) as d (d)}<option value={d}></option>{/each}</datalist>
<datalist id="cv-ds-all">{#each datasets.filter((d) => d.mountpoint.startsWith('/')) as d (d.name)}<option value={d.name}></option>{/each}</datalist>

<div class="zt-grid cv-settings">
  <div class="zt-card">
    <div class="cv-card-title"><Icon name="layers" /><h3>What to convert</h3></div>
    <p class="zt-hint">Every folder directly inside these datasets becomes a child dataset of its own, so it can be snapshotted and replicated on its own.</p>

    <div class="cv-block">
      <Toggle label="Docker appdata" checked={bool('should_process_containers')} onchange={(v) => setBool('should_process_containers', v)} />
      {#if bool('should_process_containers')}
        <div class="zt-fields cv-sub-fields">
          <Field label="Pool">
            <select bind:value={f.appdata_pool}>
              {#if !pools.includes(f.appdata_pool)}<option value={f.appdata_pool}>{f.appdata_pool}</option>{/if}
              {#each pools as p (p)}<option value={p}>{p}</option>{/each}
            </select>
          </Field>
          <Field label="Dataset">
            <input type="text" list="cv-ds-appdata" bind:value={f.appdata_dataset} spellcheck="false" autocomplete="off" />
          </Field>
        </div>
      {/if}
    </div>

    <div class="cv-block">
      <Toggle label="VM domains" checked={bool('should_process_vms')} onchange={(v) => setBool('should_process_vms', v)} />
      {#if bool('should_process_vms')}
        <div class="zt-fields cv-sub-fields">
          <Field label="Pool">
            <select bind:value={f.vm_pool}>
              {#if !pools.includes(f.vm_pool)}<option value={f.vm_pool}>{f.vm_pool}</option>{/if}
              {#each pools as p (p)}<option value={p}>{p}</option>{/each}
            </select>
          </Field>
          <Field label="Dataset">
            <input type="text" list="cv-ds-vm" bind:value={f.vm_dataset} spellcheck="false" autocomplete="off" />
          </Field>
          <Field label="Shutdown timeout" hint="Seconds to wait for a clean shutdown before the VM is forced off.">
            <div class="zt-inline"><input type="number" min="10" max="600" bind:value={f.vm_forceshutdown_wait} /> s</div>
          </Field>
        </div>
      {/if}
    </div>

    <div class="cv-block">
      <div class="zt-field-label">Extra datasets</div>
      {#if extras.length}
        <div class="cv-chips">
          {#each extras as e (e)}
            <span class="zt-chip"><Icon name="database" />{e}<button type="button" class="cv-chip-x" aria-label="Remove {e}" onclick={() => removeExtra(e)}><Icon name="x" /></button></span>
          {/each}
        </div>
      {/if}
      <div class="zt-input-group cv-add">
        <input type="text" list="cv-ds-all" placeholder="pool/dataset" bind:value={addExtra} spellcheck="false" autocomplete="off" onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addExtraDataset(); } }} />
        <Button icon="plus" small disabled={!addExtra.trim()} onclick={addExtraDataset}>Add</Button>
      </div>
      <div class="zt-hint">Any other dataset whose folders should become datasets. Containers and VMs that use them are stopped during the copy, too.</div>
    </div>
  </div>

  <div class="zt-card">
    <div class="cv-card-title"><Icon name="settings" /><h3>How</h3></div>
    <div class="cv-toggles">
      <div>
        <Toggle label="Dry run" checked={bool('dry_run')} onchange={(v) => setBool('dry_run', v)} />
        <div class="zt-hint">Only simulate: every run, scheduled ones too, logs what it would do and changes nothing.</div>
      </div>
      <div>
        <Toggle label="Delete the original after a verified copy" checked={bool('cleanup')} onchange={(v) => setBool('cleanup', v)} />
        <div class="zt-hint">Off keeps it as <span class="zt-mono">name_temp</span> next to the new dataset - it then uses the space twice.</div>
      </div>
      <div>
        <Toggle label="Replace spaces with underscores" checked={bool('replace_spaces')} onchange={(v) => setBool('replace_spaces', v)} />
        <div class="zt-hint">In dataset names only. The folder keeps its path either way.</div>
      </div>
    </div>
    <div class="zt-fields cv-numbers">
      <Field label="Buffer zone" hint="Free space needed on top of the folder size.">
        <div class="zt-inline"><input type="number" min="0" max="100" bind:value={f.buffer_zone} /> %</div>
      </Field>
      <Field label="Validation tolerance" hint="Allowed size difference between original and copy.">
        <div class="zt-inline"><input type="number" min="0" max="20" bind:value={f.validation_tolerance} /> %</div>
      </Field>
    </div>
  </div>

  <div class="zt-card">
    <div class="cv-card-title"><Icon name="calendar" /><h3>Schedule</h3></div>
    <Toggle label="Convert new folders automatically" checked={bool('cron_enabled')} onchange={(v) => setBool('cron_enabled', v)} />
    <div class="zt-hint cv-after">New containers get their appdata folder converted on the next run. Only folders whose containers and VMs can be stopped are converted.</div>
    {#if bool('cron_enabled')}
      <div class="cv-sched">
        <Segmented bind:value={f.cron_preset} options={[['hourly', 'Hourly'], ['6hourly', 'Every 6 h'], ['daily', 'Daily'], ['weekly', 'Weekly'], ['custom', 'Custom']]} />
        <div class="zt-fields cv-sub-fields">
          {#if f.cron_preset === 'weekly'}
            <Field label="Day">
              <select bind:value={f.cron_weekday}>
                {#each WEEKDAYS as d, i (i)}<option value={String(i)}>{d}</option>{/each}
              </select>
            </Field>
          {/if}
          {#if f.cron_preset === 'daily' || f.cron_preset === 'weekly'}
            <Field label="Time">
              <div class="zt-inline cv-time">
                <input type="number" min="0" max="23" bind:value={f.cron_hour} aria-label="Hour" /> :
                <input type="number" min="0" max="59" bind:value={f.cron_minute} aria-label="Minute" />
              </div>
            </Field>
          {:else if f.cron_preset === '6hourly'}
            <Field label="Minute past the hour">
              <input type="number" min="0" max="59" bind:value={f.cron_minute} />
            </Field>
          {:else if f.cron_preset === 'custom'}
            <Field label="Cron expression" wide hint="minute hour day month weekday">
              <input type="text" class="zt-mono" class:invalid={!!cronProblem(f.cron_custom)} bind:value={f.cron_custom} spellcheck="false" autocomplete="off" />
            </Field>
          {/if}
        </div>
        <div class="cv-cron-line">
          <span class="cv-dim">Runs as</span> <code>{cronPreview(f)}</code>
        </div>
      </div>
    {/if}
    {#if cron}
      <div class="cv-cron-state">
        <span class="zt-dot {cron.live ? 'ok' : cron.entry ? 'warn' : 'idle'}"></span>
        {#if cron.live}
          Active: <code>{cron.entry.split(' ').slice(0, 5).join(' ')}</code>
        {:else if cron.entry}
          Written, but the crontab does not have it yet
        {:else}
          No conversion is scheduled
        {/if}
      </div>
    {/if}
  </div>
</div>

<ErrorBox {error} />
{#if problems.length}
  <div class="zt-errors"><ul>{#each problems as p, i (i)}<li>{p}</li>{/each}</ul></div>
{/if}

<div class="cv-savebar" class:dirty>
  <span class="cv-dim cv-grow">{dirty ? 'Unsaved changes' : 'All changes saved'}</span>
  {#if dirty}<Button onclick={() => (f = { ...saved })}>Revert</Button>{/if}
  <Button kind="primary" icon="check" disabled={!dirty || problems.length > 0} busy={saving.busy} onclick={save}>Save</Button>
</div>

<style>
  :global(#zt-app) .zt-grid.cv-settings {
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr));
  }
  .cv-card-title {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }
  .cv-card-title :global(svg.zt-i) {
    color: var(--zt-dim);
  }
  .cv-card-title h3 {
    margin: 0;
    font-size: 14.5px;
    font-weight: 600;
  }
  .cv-block {
    margin-top: 16px;
  }
  :global(#zt-app) .zt-fields.cv-sub-fields {
    margin-top: 10px;
    grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  }
  .cv-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin: 8px 0;
  }
  .cv-chip-x {
    display: inline-flex;
    border: 0;
    background: none;
    padding: 0;
    margin: 0 0 0 2px;
    color: var(--zt-dim);
    cursor: pointer;
    min-width: 0;
  }
  .cv-chip-x:hover {
    color: var(--zt-err);
  }
  .cv-add {
    margin: 8px 0 6px;
  }
  .cv-toggles {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin-top: 12px;
  }
  .cv-toggles .zt-hint {
    margin: 3px 0 0 44px;
  }
  .cv-numbers {
    margin-top: 18px;
  }
  .cv-after {
    margin: 4px 0 0 44px;
  }
  .cv-sched {
    margin-top: 14px;
  }
  .cv-time input {
    width: 70px;
  }
  .cv-cron-line,
  .cv-cron-state {
    margin-top: 12px;
    font-size: 12.5px;
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .cv-cron-state .zt-dot {
    margin: 0 4px 0 0;
  }
  code {
    font-family: var(--zt-mono);
    font-size: 12px;
    background: var(--zt-surface-2);
    padding: 1px 6px;
    border-radius: 4px;
    color: var(--zt-text);
  }
  .cv-dim {
    color: var(--zt-dim);
  }
  .cv-grow {
    flex: 1 1 auto;
  }
  .cv-savebar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 14px;
    padding: 10px 14px;
    border: 1px solid var(--zt-border);
    border-radius: var(--zt-radius);
    background: var(--zt-panel);
    font-size: 13px;
  }
  .cv-savebar.dirty {
    position: sticky;
    bottom: 8px;
    z-index: 2;
    border-color: var(--zt-accent);
    box-shadow: var(--zt-shadow);
  }
</style>
