# ZFS Dataset Converter for Unraid

An Unraid plugin that converts regular folders (appdata, VM domains, or custom paths) into ZFS child datasets — with a full web GUI, live log viewer, and Docker/VM awareness.

## Features

- **Dry-run mode** — preview what will happen before making changes
- **Docker awareness** — automatically stops only containers whose appdata is not yet a dataset
- **VM awareness** — gracefully shuts down VMs if their vdisk folder needs converting
- **Resume capability** — detects and resumes interrupted conversions (`*_temp` directories)
- **Validation with tolerance** — configurable size tolerance (default 5%) to avoid false failures with sparse files or extended attributes
- **Space check** — requires `folder_size + buffer_zone%` free space before starting
- **Name normalization** — converts German umlauts, replaces spaces, removes ZFS-invalid characters
- **Unraid notifications** — integrates with the native Unraid notification system
- **Live log viewer** — real-time streaming log output in the GUI

## Requirements

- Unraid 6.12 or newer (ZFS support required)
- ZFS pool with at least one dataset to host child datasets

## Installation

1. In Unraid, go to **Plugins → Install Plugin**
2. Paste the URL to the `.plg` file:
   ```
   https://raw.githubusercontent.com/MajorPain007/zfs-dataset-converter/main/src/zfs.dataset.converter.plg
   ```
3. Click **Install**

After installation, navigate to **Settings → ZFS Dataset Converter**.

## Usage

1. Configure your source paths (appdata pool/dataset, VM domains, or custom datasets)
2. Enable **Dry Run** first to preview which folders will be converted
3. Click **Refresh** in the Folder Scanner to see the current state
4. Disable Dry Run and click **Start Conversion**
5. Monitor progress in the live log viewer

## How the conversion works

For each folder under the configured source dataset:

1. The folder is renamed to `<name>_temp`
2. A new ZFS dataset is created at `<pool>/<parent>/<name>`
3. Data is copied using `rsync -a`
4. File count and size are validated (with configurable tolerance)
5. The `_temp` folder is removed if cleanup is enabled

If the process is interrupted, the next run detects leftover `*_temp` directories and resumes from where it stopped.

## Settings

| Setting | Default | Description |
|---|---|---|
| Dry Run | yes | Simulate — no changes made |
| Cleanup | yes | Remove temp folder after successful copy |
| Replace spaces | no | Replace spaces with underscores in dataset names |
| Send notifications | yes | Unraid notification on completion |
| Buffer zone | 11% | Extra free space required beyond folder size |
| Validation tolerance | 5% | Allowed size difference after rsync |

## Credits

Based on the original script by [SpaceInvaderOne](https://github.com/SpaceInvaderOne).  
Enhanced and packaged as a plugin by [MajorPain007](https://github.com/MajorPain007).  
Inspired by [SplitAnAtom/zfs-dataset-converter](https://github.com/SplitAnAtom/zfs-dataset-converter).

## License

GPL v2.0
