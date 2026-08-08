<?php

header('Content-Type: application/json');

$file   = $_GET['file']   ?? '';
$offset = max(0, (int)($_GET['offset'] ?? 0));

$allowedDir = '/tmp/zfs.toolkit/';
$realFile   = realpath($file);

if (!$realFile || strpos($realFile, $allowedDir) !== 0 || !file_exists($realFile)) {
    echo json_encode(['lines' => [], 'next_offset' => $offset]);
    exit;
}

$size = filesize($realFile);

if ($size < $offset) {
    $offset = 0;
}

if ($size <= $offset) {
    echo json_encode(['lines' => [], 'next_offset' => $offset]);
    exit;
}

$fh    = fopen($realFile, 'rb');
$chunk = '';
if ($fh) {
    fseek($fh, $offset);

    $chunk = fread($fh, 65536);
    fclose($fh);
}

$newOffset = $offset + strlen($chunk);

$raw   = explode("\n", $chunk);
$lines = array_values($raw);

if (end($lines) === '') array_pop($lines);

echo json_encode(['lines' => $lines, 'next_offset' => $newOffset]);
