<?php
// get_snapshot_status.php - Return snapshot cron status and counts
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

// Crontab entry
$lines = [];
exec('crontab -l 2>/dev/null', $lines);
$snap_entry = '';
foreach ($lines as $line) {
    if (strpos($line, 'snapshot_manager.sh') !== false) {
        $snap_entry = trim($line);
        break;
    }
}

// Total snapshot count
$snap_lines = [];
exec('zfs list -H -t snapshot -o name 2>/dev/null | grep "@auto-" | wc -l', $snap_lines);
$snapshot_count = (int)($snap_lines[0] ?? 0);

// Last run timestamp
$last_file = '/tmp/zfs.dataset.converter/snapshot_last.txt';
$last_run  = file_exists($last_file) ? trim(file_get_contents($last_file)) : '';

echo json_encode([
    'active'         => $snap_entry !== '',
    'entry'          => $snap_entry,
    'snapshot_count' => $snapshot_count,
    'last_run'       => $last_run,
    'server_time'    => date('Y-m-d H:i:s T'),
]);
