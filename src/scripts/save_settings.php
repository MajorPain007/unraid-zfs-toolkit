<?php
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

set_error_handler(function(int $errno, string $errstr): bool {
    echo json_encode(['success' => false, 'error' => "PHP[$errno]: $errstr"]);
    exit(1);
});
register_shutdown_function(function(): void {
    $e = error_get_last();
    if ($e && ($e['type'] & (E_ERROR | E_PARSE | E_CORE_ERROR | E_COMPILE_ERROR))) {
        while (ob_get_level() > 0) ob_end_clean();
        header('Content-Type: application/json');
        echo json_encode(['success' => false, 'error' => 'Fatal: ' . $e['message']]);
    }
});

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    echo json_encode(['success' => false, 'error' => 'POST required']);
    exit;
}

$allowed = [
    'dry_run', 'cleanup', 'replace_spaces', 'send_notifications',
    'should_process_containers', 'appdata_pool', 'appdata_dataset',
    'should_process_vms', 'vm_pool', 'vm_dataset', 'vm_forceshutdown_wait',
    'buffer_zone', 'validation_tolerance', 'extra_datasets',
    // Converter cron schedule
    'cron_enabled', 'cron_preset', 'cron_hour', 'cron_minute', 'cron_weekday', 'cron_custom',
    // Snapshot scheduler
    'snapshots_enabled', 'snap_frequent', 'snap_hourly', 'snap_daily', 'snap_weekly',
    'snap_monthly', 'snap_yearly', 'snap_daily_hour',
    'snap_schedule_preset', 'snap_schedule_custom',
];

$configDir  = '/boot/config/plugins/zfs.dataset.converter';
$configFile = $configDir . '/settings.cfg';

if (!is_dir($configDir)) {
    if (!mkdir($configDir, 0755, true)) {
        echo json_encode(['success' => false, 'error' => 'Cannot create config dir: ' . $configDir]);
        exit;
    }
}

$lines = ['# ZFS Dataset Converter settings - saved ' . date('Y-m-d H:i:s'), ''];
foreach ($allowed as $key) {
    $val = isset($_POST[$key]) ? preg_replace('/[\r\n]/', '', $_POST[$key]) : '';
    $lines[] = $key . '=' . $val;
}
$lines[] = '';

if (file_put_contents($configFile, implode("\n", $lines)) === false) {
    echo json_encode(['success' => false, 'error' => 'Cannot write: ' . $configFile]);
    exit;
}

// Update converter cron
$setupCron = '/usr/local/emhttp/plugins/zfs.dataset.converter/scripts/setup_cron.sh';
if (file_exists($setupCron)) {
    shell_exec('/bin/bash ' . escapeshellarg($setupCron) . ' 2>/dev/null');
}
// Update snapshot cron
$setupSnap = '/usr/local/emhttp/plugins/zfs.dataset.converter/scripts/setup_snapshots.sh';
if (file_exists($setupSnap)) {
    shell_exec('/bin/bash ' . escapeshellarg($setupSnap) . ' 2>/dev/null');
}

echo json_encode(['success' => true]);
