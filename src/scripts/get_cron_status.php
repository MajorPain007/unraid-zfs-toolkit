<?php
// Returns current active cron entry and system time
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

$lines = [];
exec('crontab -l 2>/dev/null', $lines);

$entry = '';
foreach ($lines as $line) {
    if (strpos($line, 'run_auto.sh') !== false) {
        $entry = trim($line);
        break;
    }
}

echo json_encode([
    'entry'       => $entry,
    'active'      => $entry !== '',
    'server_time' => date('Y-m-d H:i:s T'),
]);
