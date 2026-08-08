# ZFS Toolkit for Unraid

An Unraid plugin for working with ZFS: convert folders into datasets, take and
prune snapshots on a schedule, browse and restore files out of a snapshot, and
replicate datasets to another pool or machine.

Everything runs on native `zfs` commands — no sanoid, no znapzend, nothing to
install alongside it.

## Features

| | |
|---|---|
| **Conversion** | Folders under a dataset (appdata, VM domains, custom paths) become ZFS child datasets. Only the affected containers and VMs are stopped, and started again afterwards. |
| **Snapshots** | Scheduled, with retention by count, age and free space. A period missed while the server was off is caught up on the next run. |
| **Browse & Restore** | Walk any snapshot, compare two of them, restore single files or whole folders. Restores run in the background. |
| **Replication** | Incremental `zfs send`/`recv` to a second pool or over SSH. Key-based only — the plugin never handles passwords. |
| **Snapshot Manager** | Every snapshot with the space it actually costs, filterable, with hold, rollback and bulk delete. |

## Installation

**Plugins → Install Plugin**, paste:

```
https://raw.githubusercontent.com/MajorPain007/unraid-zfs-toolkit/main/src/zfs.toolkit.plg
```

Requires Unraid 6.12 or newer with ZFS.

The plugin appears under **Settings → Utilities**, and as a **ZFS** entry in the
top menu next to Docker and VMs — that one can be switched off in the plugin
header. Every setting is explained where you set it.

## Troubleshooting

**Diagnostics** on the Snapshots tab downloads the configuration, logs, cron
state and the pool and snapshot inventory in one file. Files that look like
private keys are left out.

## Credits

Conversion logic based on the original script by
[SpaceInvaderOne](https://github.com/SpaceInvaderOne).
Packaged and extended by [MajorPain007](https://github.com/MajorPain007).

## License

GPL v2.0
