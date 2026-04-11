<?php
/**
 * get_logs.php - Stream new log lines since a given byte offset
 * GET ?file=<path>&offset=<bytes>
 * Returns: { lines: [...], next_offset: <int> }
 */
header('Content-Type: application/json');

$file   = $_GET['file']   ?? '';
$offset = max(0, (int)($_GET['offset'] ?? 0));

// Security: only allow files inside /tmp/zfs.dataset.converter/
$allowedDir = '/tmp/zfs.dataset.converter/';
$realFile   = realpath($file);

if (!$realFile || !str_starts_with($realFile, $allowedDir) || !file_exists($realFile)) {
    echo json_encode(['lines' => [], 'next_offset' => $offset]);
    exit;
}

$size = filesize($realFile);

// Nothing new since last read
if ($size <= $offset) {
    echo json_encode(['lines' => [], 'next_offset' => $offset]);
    exit;
}

$fh    = fopen($realFile, 'rb');
$chunk = '';
if ($fh) {
    fseek($fh, $offset);
    // Read up to 64 KB per poll to stay responsive
    $chunk = fread($fh, 65536);
    fclose($fh);
}

$newOffset = $offset + strlen($chunk);

// Split into lines, keep empty lines for spacing
$raw   = explode("\n", $chunk);
$lines = [];
foreach ($raw as $line) {
    $lines[] = htmlspecialchars($line, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
}

// Drop trailing empty line from split
if (end($lines) === '') array_pop($lines);

echo json_encode(['lines' => $lines, 'next_offset' => $newOffset]);
