<?php

while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

$lines = [];
exec('zfs list -H -o name 2>/dev/null', $lines);

$datasets = array_values(array_filter($lines, function($l) {
    return trim($l) !== '' && strpos($l, '@') === false;
}));

echo json_encode(['datasets' => $datasets]);
