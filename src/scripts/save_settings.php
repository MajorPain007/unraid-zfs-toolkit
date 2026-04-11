<?php
header('Content-Type: application/json');

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    echo json_encode(['success' => false, 'error' => 'POST required']);
    exit;
}

$allowed = [
    'dry_run', 'cleanup', 'replace_spaces', 'send_notifications',
    'should_process_containers', 'appdata_pool', 'appdata_dataset',
    'should_process_vms', 'vm_pool', 'vm_dataset', 'vm_forceshutdown_wait',
    'buffer_zone', 'validation_tolerance', 'extra_datasets',
];

$configDir  = '/boot/config/plugins/zfs.dataset.converter';
$configFile = $configDir . '/settings.cfg';

if (!is_dir($configDir)) {
    mkdir($configDir, 0755, true);
}

$lines = ['# ZFS Dataset Converter settings - saved ' . date('Y-m-d H:i:s'), ''];
foreach ($allowed as $key) {
    $val = isset($_POST[$key]) ? preg_replace('/[\r\n]/', '', $_POST[$key]) : '';
    $lines[] = $key . '=' . $val;
}
$lines[] = '';

if (file_put_contents($configFile, implode("\n", $lines)) === false) {
    echo json_encode(['success' => false, 'error' => 'Could not write config file']);
    exit;
}

echo json_encode(['success' => true]);
