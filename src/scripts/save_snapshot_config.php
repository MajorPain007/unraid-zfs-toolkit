<?php
// save_snapshot_config.php - Save snapshot dataset list and trigger setup_snapshots.sh
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

set_error_handler(function($errno, $errstr) {
    echo json_encode(['ok' => false, 'error' => "PHP error: $errstr"]);
    exit;
});

$config_dir = '/boot/config/plugins/zfs.dataset.converter';
$datasets_raw = $_POST['datasets_json'] ?? '';

if (!empty($datasets_raw)) {
    $datasets = json_decode($datasets_raw, true);
    if ($datasets === null) {
        echo json_encode(['ok' => false, 'error' => 'Invalid JSON: ' . json_last_error_msg()]);
        exit;
    }
    if (file_put_contents($config_dir . '/snap_datasets.json', json_encode($datasets, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE)) === false) {
        echo json_encode(['ok' => false, 'error' => 'Failed to write snap_datasets.json']);
        exit;
    }
}

// Re-run cron setup (picks up new settings.cfg values already saved)
$setup = '/usr/local/emhttp/plugins/zfs.dataset.converter/scripts/setup_snapshots.sh';
shell_exec("/bin/bash " . escapeshellarg($setup) . " >/tmp/zfs.dataset.converter/setup_snapshots.log 2>&1");

echo json_encode(['ok' => true]);
