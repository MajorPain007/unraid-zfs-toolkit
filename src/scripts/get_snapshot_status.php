<?php
// get_snapshot_status.php - Return snapshot cron status, counts and recent log
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

// Crontab entry
$lines = array();
exec('crontab -l 2>/dev/null', $lines);
$snap_entry = '';
foreach ($lines as $line) {
    if (strpos($line, 'snapshot_manager.sh') !== false) {
        $snap_entry = trim($line);
        break;
    }
}

// Total snapshot count (@auto- only)
$snap_lines = array();
exec('zfs list -H -t snapshot -o name 2>/dev/null | grep "@auto-" | wc -l', $snap_lines);
$snapshot_count = (int)(isset($snap_lines[0]) ? $snap_lines[0] : 0);

// Last run timestamp
$last_file = '/tmp/zfs.dataset.converter/snapshot_last.txt';
$last_run  = file_exists($last_file) ? trim(file_get_contents($last_file)) : '';

// Dataset count from snap_datasets.json
$datasets_file = '/boot/config/plugins/zfs.dataset.converter/snap_datasets.json';
$dataset_count = 0;
if (file_exists($datasets_file)) {
    $json = json_decode(file_get_contents($datasets_file), true);
    if ($json && isset($json['datasets'])) {
        $dataset_count = count($json['datasets']);
    }
}

// Last 30 lines of snapshots.log (most recent run output)
$log_file   = '/tmp/zfs.dataset.converter/snapshots.log';
$recent_log = array();
if (file_exists($log_file)) {
    $all = file($log_file, FILE_IGNORE_NEW_LINES);
    if ($all) {
        $recent_log = array_slice($all, -30);
    }
}

echo json_encode(array(
    'active'         => $snap_entry !== '',
    'entry'          => $snap_entry,
    'snapshot_count' => $snapshot_count,
    'last_run'       => $last_run,
    'server_time'    => date('Y-m-d H:i:s T'),
    'dataset_count'  => $dataset_count,
    'recent_log'     => $recent_log,
));
