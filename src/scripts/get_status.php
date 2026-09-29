<?php
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

$statusFile = '/tmp/zfs.toolkit/status.json';
$tmpDir     = '/tmp/zfs.toolkit';

function zdc_classify_log($logFile) {
    if ($logFile === '' || !file_exists($logFile)) return 'idle';
    $tail = array();
    exec('tail -80 ' . escapeshellarg($logFile) . ' 2>/dev/null', $tail);
    $c = implode("\n", $tail);
    // Anchor on the log's own "ERROR:" prefix - a bare "ERROR" also matches
    // folder names and rsync chatter.
    if (preg_match('/VALIDATION FAILED|\] ERROR:/m', $c)) return 'error';
    if (strpos($c, 'Script execution completed successfully') !== false) return 'completed';
    if (strpos($c, 'Nothing to convert') !== false) return 'completed';
    return 'stopped';
}

function zdc_newest_log($dir) {
    $files = array_merge(glob($dir . '/conversion_*.log') ?: array(),
                         glob($dir . '/auto_*.log') ?: array());
    if (!$files) return '';
    usort($files, function($a, $b) { return filemtime($b) - filemtime($a); });
    return $files[0];
}

if ($_SERVER['REQUEST_METHOD'] === 'POST' && ($_POST['action'] ?? '') === 'stop') {
    if (file_exists($statusFile)) {
        $st  = json_decode(file_get_contents($statusFile), true);
        $pid = (int)($st['pid'] ?? 0);
        if ($pid > 0 && file_exists('/proc/' . $pid)) {
            // bash acts on the signal only once the command it waits for has
            // returned, and a folder's rsync can run for hours. So the script
            // gets the signal first and its running command second; it then
            // puts the half-converted folder back and restarts what it stopped.
            shell_exec('kill ' . $pid . ' 2>/dev/null');
            shell_exec('pkill -TERM -P ' . $pid . ' 2>/dev/null');
        }
        $st['status'] = 'stopped';
        file_put_contents($statusFile, json_encode($st));
    }
    echo json_encode(['success' => true]);
    exit;
}

// Even with nothing running, hand back the most recent run's log so the GUI
// can show what happened instead of an empty box.
function zdc_idle_response($tmpDir) {
    $last = zdc_newest_log($tmpDir);
    return array(
        'status'      => 'idle',
        'log_file'    => $last,
        'log_is_past' => $last !== '',
        'log_time'    => $last !== '' ? date('Y-m-d H:i:s', filemtime($last)) : '',
    );
}

if (!file_exists($statusFile)) {
    echo json_encode(zdc_idle_response($tmpDir));
    exit;
}

$st = json_decode(file_get_contents($statusFile), true);
if (!$st) { echo json_encode(zdc_idle_response($tmpDir)); exit; }

$pid     = (int)($st['pid'] ?? 0);
$logFile = $st['log_file'] ?? '';
$status  = $st['status'] ?? 'idle';

if ($status === 'running' && $pid > 0 && !file_exists('/proc/' . $pid)) {
    $status = zdc_classify_log($logFile);
    $st['status'] = $status;
    file_put_contents($statusFile, json_encode($st));
} elseif ($status !== 'running' && !empty($logFile) && file_exists($logFile)) {
    // Re-derive rather than trusting a verdict written by an older version.
    $status = zdc_classify_log($logFile);
}

$currentFolder = '';
if (!empty($logFile) && file_exists($logFile)) {
    $tail = [];
    exec('tail -20 ' . escapeshellarg($logFile) . ' 2>/dev/null', $tail);
    foreach (array_reverse($tail) as $line) {
        if (preg_match("/Processing '(.+)' \(/", $line, $m)) { $currentFolder = $m[1]; break; }
    }
}

if ($logFile === '' || !file_exists($logFile)) {
    $logFile = zdc_newest_log($tmpDir);
}

echo json_encode([
    'status'         => $status,
    'pid'            => $pid,
    'log_file'       => $logFile,
    'log_is_past'    => ($status !== 'running' && $logFile !== ''),
    'log_time'       => ($logFile !== '' && file_exists($logFile)) ? date('Y-m-d H:i:s', filemtime($logFile)) : '',
    'started'        => $st['started'] ?? '',
    'current_folder' => $currentFolder,
]);
