<?php
/**
 * get_status.php - Return current conversion status
 * GET  → return status JSON
 * POST ?action=stop → kill running process
 */
header('Content-Type: application/json');

$statusFile = '/tmp/zfs.dataset.converter/status.json';

// --- Stop action ---
if ($_SERVER['REQUEST_METHOD'] === 'POST' && ($_GET['action'] ?? '') === 'stop') {
    if (file_exists($statusFile)) {
        $st = json_decode(file_get_contents($statusFile), true);
        $pid = (int)($st['pid'] ?? 0);
        if ($pid > 0 && file_exists('/proc/' . $pid)) {
            // Kill the process group to also kill child processes
            posix_kill(-$pid, SIGTERM);
            posix_kill($pid, SIGTERM);
        }
        $st['status'] = 'stopped';
        file_put_contents($statusFile, json_encode($st));
    }
    echo json_encode(['success' => true]);
    exit;
}

// --- Status query ---
if (!file_exists($statusFile)) {
    echo json_encode(['status' => 'idle']);
    exit;
}

$st = json_decode(file_get_contents($statusFile), true);
if (!$st) {
    echo json_encode(['status' => 'idle']);
    exit;
}

$pid     = (int)($st['pid'] ?? 0);
$logFile = $st['log_file'] ?? '';
$status  = $st['status'] ?? 'idle';

// If marked running, verify process is actually still alive
if ($status === 'running') {
    if ($pid > 0 && !file_exists('/proc/' . $pid)) {
        // Process ended — determine outcome from log
        $status = 'completed';
        if (!empty($logFile) && file_exists($logFile)) {
            $tail = [];
            exec('tail -50 ' . escapeshellarg($logFile) . ' 2>/dev/null', $tail);
            $combined = implode("\n", $tail);
            if (preg_match('/VALIDATION FAILED|^.*ERROR.*$/m', $combined)) {
                $status = 'error';
            } elseif (!str_contains($combined, 'Script execution completed successfully')) {
                $status = 'error';
            }
        }
        $st['status'] = $status;
        file_put_contents($statusFile, json_encode($st));
    }
}

// Try to extract current folder being processed from log
$currentFolder = '';
if (!empty($logFile) && file_exists($logFile)) {
    $tail = [];
    exec('tail -20 ' . escapeshellarg($logFile) . ' 2>/dev/null', $tail);
    foreach (array_reverse($tail) as $line) {
        if (preg_match("/Processing '(.+)' \(/", $line, $m)) {
            $currentFolder = $m[1];
            break;
        }
    }
}

echo json_encode([
    'status'         => $status,
    'pid'            => $pid,
    'log_file'       => $logFile,
    'started'        => $st['started'] ?? '',
    'current_folder' => $currentFolder,
]);
