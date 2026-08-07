<?php
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

$statusFile = '/tmp/zfs.dataset.converter/status.json';
$tmpDir     = '/tmp/zfs.dataset.converter';

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
            shell_exec('kill ' . $pid . ' 2>/dev/null');
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
    $status = 'completed';
    if (!empty($logFile) && file_exists($logFile)) {
        $tail = [];
        exec('tail -50 ' . escapeshellarg($logFile) . ' 2>/dev/null', $tail);
        $combined = implode("\n", $tail);
        if (preg_match('/VALIDATION FAILED|ERROR/m', $combined) ||
            strpos($combined, 'Script execution completed successfully') === false) {
            $status = 'error';
        }
    }
    $st['status'] = $status;
    file_put_contents($statusFile, json_encode($st));
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
