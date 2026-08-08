# ZFS Toolkit for Unraid

An Unraid plugin for working with ZFS: turn plain folders into datasets, take and
prune snapshots on a schedule, browse and restore individual files out of a
snapshot, and replicate datasets to a second pool or another machine.

Everything runs on native `zfs` commands — no sanoid, no znapzend, nothing to
install alongside it.

## What it does

| | |
|---|---|
| **Conversion** | Turns folders under a dataset (appdata, VM domains, custom paths) into ZFS child datasets, stopping only the containers and VMs that are affected |
| **Snapshots** | Scheduled snapshots with count-, age- and free-space-based retention |
| **Browse & Restore** | Walk any snapshot in the browser, compare two snapshots, restore single files or whole folders |
| **Replication** | `zfs send`/`recv` to another pool or over SSH, incremental |

## Installation

**Plugins → Install Plugin**, paste:

```
https://raw.githubusercontent.com/MajorPain007/zfs-dataset-converter/main/src/zfs.dataset.converter.plg
```

Requires Unraid 6.12 or newer with ZFS. The plugin appears under
**Settings → Utilities**, and as a **ZFS** entry in the top menu next to Docker
and VMs — that entry can be switched off in the plugin header.

## Snapshots

### The schedule is a check interval, not a snapshot interval

This is the part that trips people up. The cron entry decides how often the
plugin *looks for work*; each run creates only what the current period is still
missing. Checking every 15 minutes with hourly retention gives you one snapshot
per hour, not four:

| Cron every 15 min, 3 days | |
|---|---|
| cron ran | 288× |
| hourly snapshots | 72 |
| daily | 3 |
| weekly / monthly | 1 / 1 |

Only **Frequent** takes one on every run, which is why it defaults to 0.

Snapshots are named `dataset@auto-TYPE-PERIODKEY`, e.g.
`cache/appdata@auto-daily-2026-08-08`. The period key is also the deduplication
key, so a type can never be taken twice for the same period — and a period that
was missed because the server was off is caught up on the next run rather than
being lost.

**Daily snapshot time** is the *earliest* hour for daily, weekly, monthly and
yearly snapshots. Set it to 0 to take them right after midnight, or to a quiet
hour if the snapshot should capture a specific state (after a nightly backup,
before the mover).

### Retention runs in three passes, every cycle

1. **Count** — keep the newest N per type
2. **Age** — additionally drop anything older than `N` days per type (0 = off)
3. **Free space** — while the pool has less free than the target (`100G`, `10%`),
   delete the oldest auto snapshots

Never deleted: snapshots with a `zfs hold`, the newest snapshot of an active
type, replication checkpoints, and anything not created by this plugin.

### Snapshot Manager

Lists every snapshot with how much space it actually costs (`used`) and what it
references, biggest first, plus how much of each dataset is snapshots. Filter by
substring or `*`/`?` wildcard, select everything matching, then delete, hold or
release in one go. The selection survives changing the filter, so you can
collect across several searches.

## Browse & Restore

Pick a dataset and a snapshot, walk the tree, restore a file or a folder either
to its original location or anywhere else. Restores run in the background with a
progress indicator, so a large folder does not die on a web-request timeout.

**Compare** two snapshots — or a snapshot against the live filesystem — to see
what was added, removed, modified or renamed. The order does not matter; the
plugin sorts by creation time.

## Replication

Per-job `zfs send`/`recv` to a second pool or another machine:

```
[ this server ]  backup/appdata
[ over SSH    ]  root@10.0.0.5 : tank/backup/appdata
```

Incremental by default. Each job keeps its own checkpoint snapshots
(`@zdc-send-<job>-<stamp>`) so replication does not break when the regular
snapshots are pruned; the base for the increment is the newest snapshot present
on **both** sides.

- **Children** (`zfs send -R`) also replicates every dataset below the source
- **Force** (`zfs recv -F`) rolls the destination back to the last common
  snapshot first, discarding anything written there since — off by default
- Both ends are ZFS datasets. `zfs recv` cannot write into a plain folder, so
  `/mnt/user/Backup` is not a valid destination; the pool has to exist, the
  dataset is created on the first run
- SSH is key-based only — the plugin never handles passwords:

```bash
ssh-keygen -t ed25519 -f /boot/config/plugins/zfs.dataset.converter/id_send -N ""
ssh-copy-id -i /boot/config/plugins/zfs.dataset.converter/id_send.pub root@10.0.0.5
```

**Test** checks reachability and **Dry run** shows what would be sent — neither
moves any data.

## Conversion

For each folder under a configured source dataset:

1. The folder is renamed to `<name>_temp`
2. A dataset is created at `<pool>/<parent>/<name>` and verified to be mounted
   there — otherwise rsync would write into the parent dataset and the data
   would be shadowed the moment the child mounts
3. Data is copied with `rsync -aHAX --numeric-ids`, so hardlinks, ACLs and
   extended attributes survive (plain `-a` drops all three, which breaks a
   surprising number of containers)
4. Entry count and size are validated against a tolerance
5. The temp folder is removed if cleanup is enabled

Containers and VMs that were stopped are restarted from an exit trap, so
pressing Stop or an out-of-memory kill cannot leave them powered off. An
interrupted run resumes from the leftover `*_temp` directory.

## Settings

### Conversion

| Setting | Default | Description |
|---|---|---|
| Dry Run | yes | Simulate — no changes made |
| Cleanup after conversion | yes | Remove the temp folder after a successful copy |
| Replace spaces | no | Replace spaces with underscores (ZFS does not allow spaces) |
| Send notifications | yes | Unraid notification on completion |
| Buffer zone | 11 % | Extra free space required beyond the folder size |
| Validation tolerance | 5 % | Allowed size difference after rsync |

### Snapshots

| Setting | Default | Description |
|---|---|---|
| Retention per type | 24 / 30 / 4 / 3 / 0 / 0 | hourly / daily / weekly / monthly / yearly / frequent |
| Maximum age per type | 0 | Days; 0 = keep by count only |
| Daily snapshot time | 2 | Earliest hour for daily and longer |
| Warn below free space | 5 % | Notify when a snapshotted pool fills up |
| Free-space target | — | e.g. `100G` or `10%`; prune oldest until reached |
| Check every | 15 min | How often the plugin looks for work |

## Troubleshooting

**Diagnostics** on the Snapshots tab downloads a bundle with the configuration,
logs, cron state and the pool and snapshot inventory. Private keys are excluded.

Check that the schedule is actually live:

```bash
cat /etc/cron.d/zfs.dataset.converter-snapshots; crontab -l | grep snapshot_manager
```

`/etc/cron.d` is on a RAM disk, so the plugin re-registers all cron jobs after
every array start via a `disks_mounted` event hook.

## Development

```bash
tests/run.sh                       # 79 checks, no Unraid needed
./scripts/build.sh 2026.08.08.23   # build the package, sync the manifest
```

CI runs the same suite plus shellcheck and a PHP 7.4 lint, because Unraid builds
in the wild may still ship PHP 7.

## Credits

Conversion logic based on the original script by
[SpaceInvaderOne](https://github.com/SpaceInvaderOne), inspired by
[SplitAnAtom/zfs-dataset-converter](https://github.com/SplitAnAtom/zfs-dataset-converter).
Packaged and extended by [MajorPain007](https://github.com/MajorPain007).

## License

GPL v2.0
