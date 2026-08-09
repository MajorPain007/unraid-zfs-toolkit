<?php

require_once __DIR__ . '/zdc_php_common.php';

while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

$tmpDir    = '/tmp/zfs.toolkit';
$configDir = '/boot/config/plugins/zfs.toolkit';
$cron = zdc_cron_state('zfs.toolkit-snapshots', 'snapshot_manager.sh');
$snap_entry   = $cron['entry'];
$cron_source  = $cron['source'];
$cron_healthy = $cron['healthy'];

$snap_lines = array();
exec('zfs list -H -t snapshot -o name 2>/dev/null | grep -c "@auto-"', $snap_lines);
$snapshot_count = (int)(isset($snap_lines[0]) ? $snap_lines[0] : 0);

$run = array();
$statusFile = $tmpDir . '/snapshot_status.json';
if (file_exists($statusFile)) {
    $decoded = json_decode(file_get_contents($statusFile), true);
    if (is_array($decoded)) $run = $decoded;
}

$last_file = $tmpDir . '/snapshot_last.txt';
$last_run  = isset($run['last_run'])
    ? $run['last_run']
    : (file_exists($last_file) ? trim(file_get_contents($last_file)) : '');

$datasets_file = $configDir . '/snap_datasets.json';
$dataset_count = 0;
if (file_exists($datasets_file)) {
    $json = json_decode(file_get_contents($datasets_file), true);
    if ($json && isset($json['datasets'])) {
        $dataset_count = count($json['datasets']);
    }
}

$pools = array();
$poolLines = array();
exec('zpool list -H -o name,capacity 2>/dev/null', $poolLines);
foreach ($poolLines as $line) {
    $parts = preg_split('/\s+/', trim($line));
    if (count($parts) >= 2) {
        $pools[] = array('name' => $parts[0], 'capacity' => (int)rtrim($parts[1], '%'));
    }
}

function tail_lines($path, $count) {
    if (!file_exists($path)) return array();
    $fh = @fopen($path, 'rb');
    if (!$fh) return array();
    $chunk = 8192;
    $size  = filesize($path);
    $pos   = $size;
    $data  = '';
    while ($pos > 0 && substr_count($data, "\n") <= $count) {
        $read = ($pos >= $chunk) ? $chunk : $pos;
        $pos -= $read;
        fseek($fh, $pos);
        $data = fread($fh, $read) . $data;
    }
    fclose($fh);
    $all = explode("\n", rtrim($data, "\n"));
    return array_slice($all, -$count);
}

$recent_log = tail_lines($tmpDir . '/snapshots.log', 30);

echo json_encode(array(
    'active'         => $snap_entry !== '',
    'entry'          => $snap_entry,
    'cron_source'    => $cron_source,
    'cron_healthy'   => $cron_healthy,
    'snapshot_count' => $snapshot_count,
    'last_run'       => $last_run,
    'server_time'    => date('Y-m-d H:i:s T'),
    'dataset_count'  => $dataset_count,
    'pools'          => $pools,
    'run'            => $run,
    'recent_log'     => $recent_log,
));
