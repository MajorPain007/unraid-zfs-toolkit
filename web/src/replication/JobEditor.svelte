<script lang="ts">
  import { untrack } from 'svelte';
  import { slide } from 'svelte/transition';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import OptionCards from '../lib/ui/OptionCards.svelte';
  import ErrorBox from '../lib/ui/ErrorBox.svelte';
  import DatasetPicker from './DatasetPicker.svelte';
  import { api, ApiError } from '../lib/api';
  import { act } from '../lib/actions';
  import { app, fail, openDialog, refresh, toast } from '../lib/state.svelte';
  import { blankJob, deleteJob, replication, type SendJob } from './send.svelte';

  let { id = null, close }: { id?: string | null; close: (r?: unknown) => void } = $props();

  const saved = untrack(() => (id ? (replication.jobs.find((j) => j.id === id) ?? null) : null));
  let isNew = $state(!saved);
  const savedName = saved?.name ?? '';
  let job = $state<SendJob>(saved ? ($state.snapshot(saved) as SendJob) : blankJob(untrack(() => replication.jobs)));
  let error = $state<unknown>(null);
  let saving = $state(false);
  let root: HTMLElement | undefined = $state();
  let errorEl: HTMLElement | undefined = $state();
  let keyBusy = $state(false);
  const saveKeys = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘S' : 'Ctrl+S';

  let pools = $derived((app.overview?.pools ?? []).map((p) => p.name));
  let srcLeaf = $derived(job.source ? job.source.split('/').pop() || job.source : 'name');
  let destParts = $derived(job.dest.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean));

  let destNotes = $derived.by<{ level: string; text: string; fix?: string }[]>(() => {
    const out: { level: string; text: string; fix?: string }[] = [];
    const d = job.dest.trim();
    if (!d) return out;
    if (d.startsWith('/mnt/') || d.startsWith('/')) {
      out.push({ level: 'err', text: 'This is a folder path. zfs recv writes into a dataset: pool/dataset, without /mnt.', fix: d.replace(/^\/(mnt\/)?/, '') });
      return out;
    }
    if (destParts.length === 1) {
      out.push({ level: 'err', text: 'A whole pool cannot be the destination - the copy needs a dataset of its own.', fix: `${d}/replica/${srcLeaf}` });
      return out;
    }
    if (job.transport === 'local') {
      if (job.source && (d === job.source || d.startsWith(job.source + '/'))) {
        out.push({ level: 'err', text: 'The destination lies inside the source.' });
      }
      if (destParts.length >= 1 && pools.length && !pools.includes(destParts[0])) {
        out.push({ level: 'err', text: `There is no pool "${destParts[0]}" on this server.` });
      } else if (job.source && sameDataset(job.source, destParts[0])) {
        out.push({ level: 'warn', text: 'Source and copy are on the same pool, so losing that pool loses both.' });
      }
      if (destParts.length === 2) {
        out.push({
          level: 'warn',
          text: `Unraid treats every top-level folder of every pool as a user share, so ${d} would show up as the share "${destParts[1]}" from the next boot on.`,
          fix: `${destParts[0]}/replica/${destParts[1]}`
        });
      }
    } else if (destParts.length === 2) {
      out.push({
        level: 'info',
        text: `If ${job.ssh_host || 'the other server'} runs Unraid, a copy directly below the pool becomes a share "${destParts[1]}" there. One level down avoids that.`,
        fix: `${destParts[0]}/replica/${destParts[1]}`
      });
    }
    return out;
  });

  function sameDataset(src: string, pool: string): boolean {
    return src.split('/')[0] === pool;
  }

  function suggestName() {
    if (!job.name.trim() && job.source) {
      const where = job.transport === 'ssh' ? job.ssh_host.replace(/^.*@/, '') || 'remote' : job.dest.split('/')[0] || 'backup';
      job.name = `${srcLeaf} to ${where}`;
    }
  }

  async function pickSource() {
    const r = await openDialog<string | undefined>(DatasetPicker, {
      title: 'Choose what to replicate',
      current: job.source,
      hint: 'Pick a dataset. With "Child datasets" on, everything below it comes along.'
    });
    if (!r) return;
    job.source = r;
    if (!job.dest && job.transport === 'local') {
      const other = pools.find((p) => p !== r.split('/')[0]);
      if (other) job.dest = `${other}/replica/${r.split('/').pop()}`;
    }
  }

  async function pickDest() {
    if (job.transport === 'ssh' && !job.ssh_host.trim()) {
      toast('Enter the SSH host first.', 'err');
      return;
    }
    const r = await openDialog<string | undefined>(DatasetPicker, {
      title: job.transport === 'ssh' ? `Datasets on ${job.ssh_host}` : 'Choose where the copy goes',
      remote: job.transport === 'ssh' ? $state.snapshot(job) : null,
      current: job.dest,
      hint: 'Pick the existing copy, or the dataset the copy should go below. The copy itself is created on the first run.'
    });
    if (!r) return;
    const leaf = r.split('/').pop();
    if (leaf === srcLeaf && r.includes('/')) job.dest = r;
    else if (!r.includes('/')) job.dest = `${r}/replica/${srcLeaf}`;
    else job.dest = `${r}/${srcLeaf}`;
  }

  async function useKey() {
    keyBusy = true;
    try {
      const r = await api<{ key: { path: string; public: string } }>('send_key', { op: 'create' });
      replication.key = r.key as any;
      job.ssh_key = r.key.path;
      act('sendKey', $state.snapshot(job));
    } catch (e) {
      fail(e);
    } finally {
      keyBusy = false;
    }
  }

  async function save() {
    if (saving) return;
    saving = true;
    error = null;
    suggestName();
    try {
      const res = await api<{ job: SendJob; jobs: SendJob[]; problems: string[]; warnings: string[] }>('send_save_job', { job: $state.snapshot(job) });
      replication.jobs = res.jobs;
      job = res.job;
      refresh();
      if (res.problems.length) {
        isNew = false;
        error = new ApiError('Saved, but switched off until this is fixed:', res.problems);
        setTimeout(() => errorEl?.scrollIntoView({ block: 'start', behavior: 'smooth' }), 0);
        return;
      }
      close('saved');
      toast(isNew ? `Job "${res.job.name}" created.` : 'Saved.');
      for (const w of res.warnings) toast(w, 'info');
    } catch (e) {
      error = e;
      setTimeout(() => errorEl?.scrollIntoView({ block: 'start', behavior: 'smooth' }), 0);
    } finally {
      saving = false;
    }
  }

  async function remove() {
    const current = replication.jobs.find((j) => j.id === job.id);
    if (current && (await deleteJob(current))) close('deleted');
  }

  function keydown(e: KeyboardEvent) {
    if (!root || !root.contains(e.target as Node)) return;
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault();
      save();
    }
  }
</script>

<svelte:window onkeydown={keydown} />

<Dialog drawer title={isNew ? 'New replication job' : `Edit "${savedName || job.name}"`} icon="send" onclose={() => close()}>
  <div bind:this={root} class="zr-ed">
    <div bind:this={errorEl} class="zr-ed-error">
      {#if error}<div transition:slide={{ duration: 150 }}><ErrorBox {error} /></div>{/if}
    </div>

    <div class="zt-form-section">
      <h3><Icon name="database" />What to copy</h3>
      <div class="zt-fields">
        <Field label="Source dataset" wide hint="A dataset on this server, never a folder path.">
          <div class="zt-input-group">
            <input type="text" class="zt-mono" placeholder="cache/appdata" autocomplete="off" spellcheck="false" bind:value={job.source} />
            <Button icon="search" onclick={pickSource}>Choose</Button>
          </div>
        </Field>
        <div class="zt-field wide">
          <Toggle bind:checked={job.recursive} label="Child datasets too" />
          <div class="zt-hint">
            zfs send -R. On: {job.source || 'the source'} and every dataset below it, with their snapshots and properties. Off: only that one dataset.
          </div>
        </div>
      </div>
    </div>

    <div class="zt-form-section">
      <h3><Icon name="hdd" />Where the copy goes</h3>
      <OptionCards
        two
        bind:value={job.transport}
        options={[
          { value: 'local', title: 'Another pool here', icon: 'hdd', text: 'A second pool in this server. Survives losing the source pool.' },
          { value: 'ssh', title: 'Another server', icon: 'server', text: 'Any machine with ZFS reachable over SSH, with a key. Survives losing this server.' }
        ]}
      />
      {#if job.transport === 'ssh'}
        <div class="zt-fields zr-gap" transition:slide={{ duration: 150 }}>
          <Field label="SSH login" hint="user@host - usually root on the other server.">
            <input type="text" placeholder="root@10.0.0.5" autocomplete="off" spellcheck="false" bind:value={job.ssh_host} />
          </Field>
          <Field label="Port">
            <input type="number" min="1" max="65535" bind:value={job.ssh_port} />
          </Field>
          <Field label="Private key" wide optional>
            <div class="zt-input-group">
              <input type="text" class="zt-mono" placeholder="empty = root's own keys (~/.ssh/id_*)" autocomplete="off" spellcheck="false" bind:value={job.ssh_key} />
              {#if replication.key?.exists && job.ssh_key === replication.key.path}
                <Button icon="key" onclick={() => act('sendKey', $state.snapshot(job))}>Show key</Button>
              {:else}
                <Button icon="key" busy={keyBusy} onclick={useKey}>{replication.key?.exists ? 'Use plugin key' : 'Make a key'}</Button>
              {/if}
            </div>
            {#snippet hintContent()}
              Key-based login only; the plugin never handles passwords. The plugin's own key lives on the flash drive and survives reboots.
            {/snippet}
          </Field>
        </div>
      {/if}
      <div class="zt-fields zr-gap">
        <Field label="Destination dataset" wide>
          <div class="zt-input-group">
            <input
              type="text"
              class="zt-mono"
              placeholder={job.transport === 'ssh' ? 'tank/replica/' + srcLeaf : 'backup/replica/' + srcLeaf}
              autocomplete="off"
              spellcheck="false"
              bind:value={job.dest}
            />
            <Button icon="search" onclick={pickDest}>Browse</Button>
          </div>
          {#snippet hintContent()}
            Created on the first run, with any missing parents. The pool has to exist{job.transport === 'ssh' ? ' on the other server' : ''}.
          {/snippet}
        </Field>
      </div>
      {#each destNotes as n (n.text)}
        <div class="zt-card-note {n.level}" transition:slide={{ duration: 120 }}>
          <Icon name={n.level === 'info' ? 'info' : 'alert'} />
          <div>
            {n.text}
            {#if n.fix}<button type="button" class="zt-link" onclick={() => (job.dest = n.fix || job.dest)}>Use {n.fix}</button>{/if}
          </div>
        </div>
      {/each}
    </div>

    <div class="zt-form-section">
      <h3><Icon name="shield" />Safety</h3>
      <div class="zr-toggles">
        <div>
          <Toggle bind:checked={job.readonly} label="Read-only copy" />
          <div class="zt-hint">Received with readonly=on, so nothing writes to it by accident and the next run never finds it changed.</div>
        </div>
        <div>
          <Toggle bind:checked={job.allow_rollback} label="Force: roll the destination back when needed" />
          <div class="zt-hint">
            zfs recv -F. Off, a run stops when the copy changed since the last one. On, the copy is rolled back to the last common snapshot first,
            <b>discarding what was written there and snapshots made there</b>{job.recursive ? ', and snapshots deleted here are deleted there too' : ''}. Only for a
            pure copy.
          </div>
        </div>
      </div>
      {#if job.allow_rollback}
        <div class="zt-card-note warn" transition:slide={{ duration: 120 }}>
          <Icon name="alert" />
          <div>Force is meant for a copy nothing else touches. For a one-off repair, leave it off and use "Roll back the copy and retry" when a run asks for it.</div>
        </div>
      {/if}
    </div>

    <div class="zt-form-section">
      <details class="zt-more">
        <summary>Advanced</summary>
        <div class="zr-toggles">
          <div>
            <Toggle bind:checked={job.compressed} label="Send compressed blocks as they are" />
            <div class="zt-hint">zfs send -L -e -c: blocks stay as compressed on disk. Less to transfer; the copy keeps the source's compression.</div>
          </div>
          <div>
            <Toggle bind:checked={job.raw} label="Raw (encrypted datasets)" />
            <div class="zt-hint">zfs send -w: encrypted data stays encrypted and the other side never needs the key.</div>
          </div>
        </div>
        <div class="zt-fields zr-gap">
          <Field label="Checkpoints kept on the destination" hint="0 = keep all. This server always keeps the two newest.">
            <input type="number" min="0" bind:value={job.keep_dest} />
          </Field>
          <Field label="Bandwidth limit (MB/s)" hint="0 = as fast as it goes.">
            <input type="number" min="0" bind:value={job.bwlimit} />
          </Field>
          <Field label="Name" hint="Shown on the card and in notifications.">
            <input type="text" placeholder="{srcLeaf} to {job.transport === 'ssh' ? 'remote' : 'backup'}" autocomplete="off" bind:value={job.name} />
          </Field>
          <div class="zt-field">
            <Toggle bind:checked={job.enabled} label="Part of the scheduled runs" />
          </div>
        </div>
      </details>
    </div>
  </div>

  {#snippet actions()}
    {#if !isNew}
      <Button icon="trash" kind="danger" onclick={remove}>Delete</Button>
    {/if}
    <Button icon="link" onclick={() => act('sendTest', $state.snapshot(job))} title="Check source, login and destination without changing anything">Test</Button>
    <Button icon="eye" onclick={() => act('sendPreview', $state.snapshot(job))} title="Show what a run would do">Preview</Button>
    <div class="zt-spacer"></div>
    <span class="zt-hint zr-keys zt-hide-narrow">{saveKeys} saves</span>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="primary" icon="check" busy={saving} onclick={save}>{isNew ? 'Create job' : 'Save'}</Button>
  {/snippet}
</Dialog>

<style>
  .zr-ed-error > div {
    padding-top: 14px;
  }
  .zr-gap {
    margin-top: 14px;
  }
  .zr-toggles {
    display: grid;
    gap: 14px;
  }
  .zr-toggles .zt-hint {
    margin-top: 4px;
  }
  .zr-ed :global(.zt-card-note .zt-link) {
    margin-left: 6px;
    font-family: var(--zt-mono);
    font-size: 12px;
  }
  .zr-keys {
    font-size: 12px;
    margin-right: 4px;
  }
</style>
