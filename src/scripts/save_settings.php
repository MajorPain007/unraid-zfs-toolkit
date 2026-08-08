<?php
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

set_error_handler(function($errno, $errstr) {
    echo json_encode(['success' => false, 'error' => "PHP[$errno]: $errstr"]);
    exit(1);
});
register_shutdown_function(function() {
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
    'dry_run', 'cleanup', 'replace_spaces', 'send_notifications', 'show_in_menu',
    'should_process_containers', 'appdata_pool', 'appdata_dataset',
    'should_process_vms', 'vm_pool', 'vm_dataset', 'vm_forceshutdown_wait',
    'buffer_zone', 'validation_tolerance', 'extra_datasets',

    'cron_enabled', 'cron_preset', 'cron_hour', 'cron_minute', 'cron_weekday', 'cron_custom',

    'snapshots_enabled', 'snap_frequent', 'snap_hourly', 'snap_daily', 'snap_weekly',
    'snap_monthly', 'snap_yearly', 'snap_daily_hour', 'snap_min_free_pct',
    'snap_schedule_preset', 'snap_schedule_custom',

    'snap_age_frequent', 'snap_age_hourly', 'snap_age_daily', 'snap_age_weekly',
    'snap_age_monthly', 'snap_age_yearly', 'snap_free_target',

    'send_enabled', 'send_schedule_preset', 'send_schedule_hour', 'send_schedule_custom',
];

function validCron($expr) {
    $expr = trim($expr);
    if ($expr === '') return false;
    if (!preg_match('/^[0-9A-Za-z*\/,\s-]+$/', $expr)) return false;
    return count(preg_split('/\s+/', $expr)) === 5;
}

$configDir  = '/boot/config/plugins/zfs.dataset.converter';
$configFile = $configDir . '/settings.cfg';

if (!is_dir($configDir)) {
    if (!mkdir($configDir, 0755, true)) {
        echo json_encode(['success' => false, 'error' => 'Cannot create config dir: ' . $configDir]);
        exit;
    }
}

$existing = [];
if (file_exists($configFile)) {
    foreach (file($configFile) as $line) {
        $line = trim($line);
        if ($line === '' || $line[0] === '#') continue;
        $parts = explode('=', $line, 2);
        if (count($parts) === 2) $existing[trim($parts[0])] = trim($parts[1]);
    }
}

$lines = ['# ZFS Dataset Converter settings - saved ' . date('Y-m-d H:i:s'), ''];
foreach ($allowed as $key) {
    if (isset($_POST[$key])) {
        $val = preg_replace('/[\r\n]/', '', $_POST[$key]);
    } else {
        $val = isset($existing[$key]) ? $existing[$key] : '';
    }
    $lines[] = $key . '=' . $val;
}
$lines[] = '';

if (file_put_contents($configFile, implode("\n", $lines)) === false) {
    echo json_encode(['success' => false, 'error' => 'Cannot write: ' . $configFile]);
    exit;
}

$warnings = [];

if (($_POST['cron_preset'] ?? '') === 'custom' && isset($_POST['cron_custom'])
    && !validCron($_POST['cron_custom'])) {
    $warnings[] = 'Conversion schedule: "' . $_POST['cron_custom']
        . '" is not a valid 5-field cron expression - the previous schedule is still active.';
}
if (($_POST['snap_schedule_preset'] ?? '') === 'custom' && isset($_POST['snap_schedule_custom'])
    && !validCron($_POST['snap_schedule_custom'])) {
    $warnings[] = 'Snapshot schedule: "' . $_POST['snap_schedule_custom']
        . '" is not a valid 5-field cron expression - the previous schedule is still active.';
}
if (($_POST['send_schedule_preset'] ?? '') === 'custom' && isset($_POST['send_schedule_custom'])
    && !validCron($_POST['send_schedule_custom'])) {
    $warnings[] = 'Replication schedule: "' . $_POST['send_schedule_custom']
        . '" is not a valid 5-field cron expression - the previous schedule is still active.';
}

if (isset($_POST['snap_free_target']) && trim($_POST['snap_free_target']) !== ''
    && !preg_match('/^[0-9]+\s*([KMGTP]i?B?|%)$/i', trim($_POST['snap_free_target']))) {
    $warnings[] = 'Free-space target: "' . $_POST['snap_free_target']
        . '" is not understood - use something like 100G or 10%.';
}

$base = '/usr/local/emhttp/plugins/zfs.dataset.converter/scripts/';
foreach (['setup_cron.sh'      => 'Conversion',
          'setup_snapshots.sh' => 'Snapshot',
          'setup_send.sh'      => 'Replication'] as $script => $label) {
    if (!file_exists($base . $script)) continue;
    $out = [];
    $rc  = 0;
    exec('/bin/bash ' . escapeshellarg($base . $script) . ' 2>&1', $out, $rc);
    if ($rc !== 0) {
        $warnings[] = $label . ' cron: ' . trim(implode(' ', $out));
    }
}

echo json_encode(['success' => true, 'warnings' => $warnings]);
