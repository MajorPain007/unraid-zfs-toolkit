<?php

require_once __DIR__ . '/zdc_php_common.php';

while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

$cron = zdc_cron_state('zfs.toolkit', 'run_auto.sh');
$entry  = $cron['entry'];
$source = $cron['source'];

echo json_encode([
    'entry'        => $entry,
    'active'       => $entry !== '',
    'cron_source'  => $source,
    'cron_healthy' => $cron['healthy'],
    'server_time'  => date('Y-m-d H:i:s T'),
]);
