<?php
// run_snapshot_manual.php - Manually trigger snapshot_manager.sh in background
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

$plugin_dir = '/usr/local/emhttp/plugins/zfs.dataset.converter';
$log_dir    = '/tmp/zfs.dataset.converter';
$runner     = "$plugin_dir/scripts/snapshot_manager.sh";

if (!file_exists($runner)) {
    echo json_encode(['ok' => false, 'error' => 'snapshot_manager.sh not found']);
    exit;
}

$log_file = "$log_dir/snapshots.log";
// --now bypasses time checks so snapshots are created immediately
$cmd      = "nohup /bin/bash " . escapeshellarg($runner) . " --now"
           . " >> " . escapeshellarg($log_file) . " 2>&1 & echo \$!";

$pid = trim(shell_exec($cmd));
if ($pid) {
    echo json_encode(['ok' => true, 'pid' => (int)$pid]);
} else {
    echo json_encode(['ok' => false, 'error' => 'Failed to start process']);
}
