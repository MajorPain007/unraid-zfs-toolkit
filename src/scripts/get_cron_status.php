<?php

while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

$cronFile = '/etc/cron.d/zfs.toolkit';

$entry  = '';
$source = '';

if (is_readable($cronFile)) {
    foreach (file($cronFile, FILE_IGNORE_NEW_LINES) as $line) {
        $line = trim($line);
        if ($line === '' || $line[0] === '#') continue;
        if (strpos($line, 'run_auto.sh') !== false) {
            $entry  = $line;
            $source = 'cron.d';
            break;
        }
    }
}

$lines = array();
exec('crontab -l 2>/dev/null', $lines);
$in_live_crontab = false;
foreach ($lines as $line) {
    if (strpos($line, 'run_auto.sh') !== false) {
        $in_live_crontab = true;
        if ($entry === '') {
            $entry  = trim($line);
            $source = 'crontab';
        }
        break;
    }
}

echo json_encode([
    'entry'        => $entry,
    'active'       => $entry !== '',
    'cron_source'  => $source,
    'cron_healthy' => ($entry !== '' && $in_live_crontab),
    'server_time'  => date('Y-m-d H:i:s T'),
]);
