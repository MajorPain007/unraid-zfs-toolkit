<?php
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

set_error_handler(function(int $errno, string $errstr): bool {
    echo json_encode(['success' => false, 'error' => "PHP[$errno]: $errstr"]);
    exit(1);
});
register_shutdown_function(function(): void {
    $e = error_get_last();
    if ($e && ($e['type'] & (E_ERROR | E_PARSE | E_CORE_ERROR | E_COMPILE_ERROR))) {
        while (ob_get_level() > 0) ob_end_clean();
        header('Content-Type: application/json');
        echo json_encode(['success' => false, 'error' => 'Fatal: ' . $e['message']]);
    }
});

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    echo json_encode(['success' => false, 'error' => 'POST required']);
    exit;
}

$statusFile = '/tmp/zfs.dataset.converter/status.json';
$tmpDir     = '/tmp/zfs.dataset.converter';

if (file_exists($statusFile)) {
    $st = json_decode(file_get_contents($statusFile), true);
    if (isset($st['pid']) && file_exists('/proc/' . (int)$st['pid'])) {
        echo json_encode(['success' => false, 'error' => 'Already running (PID ' . (int)$st['pid'] . ')']);
        exit;
    }
}

if (!is_dir($tmpDir) && !mkdir($tmpDir, 0755, true)) {
    echo json_encode(['success' => false, 'error' => 'Cannot create tmp dir: ' . $tmpDir]);
    exit;
}

function sanitizeBool(string $key, string $default = 'no'): string {
    return in_array(strtolower($_POST[$key] ?? $default), ['yes','true','1'], true) ? 'yes' : 'no';
}
function sanitizePath(string $key, string $default = ''): string {
    return preg_replace('/[^a-zA-Z0-9_\-. ]/', '', $_POST[$key] ?? $default);
}
function sanitizeInt(string $key, int $default, int $min, int $max): int {
    return max($min, min($max, (int)($_POST[$key] ?? $default)));
}

$extraParts = [];
foreach (explode(',', $_POST['extra_datasets'] ?? '') as $e) {
    $e = trim($e);
    if ($e !== '' && preg_match('/^[a-zA-Z0-9_\-.\/ ]+$/', $e)) {
        $extraParts[] = '"' . $e . '"';
    }
}

$vars = [
    '__DRY_RUN__'              => sanitizeBool('dry_run', 'yes'),
    '__CLEANUP__'              => sanitizeBool('cleanup', 'yes'),
    '__REPLACE_SPACES__'       => sanitizeBool('replace_spaces', 'no'),
    '__PROCESS_CONTAINERS__'   => sanitizeBool('should_process_containers', 'yes'),
    '__APPDATA_POOL__'         => sanitizePath('appdata_pool', 'cache'),
    '__APPDATA_DATASET__'      => sanitizePath('appdata_dataset', 'appdata'),
    '__PROCESS_VMS__'          => sanitizeBool('should_process_vms', 'no'),
    '__VM_POOL__'              => sanitizePath('vm_pool', 'cache'),
    '__VM_DATASET__'           => sanitizePath('vm_dataset', 'domains'),
    '__VM_SHUTDOWN_WAIT__'     => (string)sanitizeInt('vm_forceshutdown_wait', 90, 10, 600),
    '__BUFFER_ZONE__'          => (string)sanitizeInt('buffer_zone', 11, 0, 100),
    '__SEND_NOTIFICATIONS__'   => sanitizeBool('send_notifications', 'yes'),
    '__VALIDATION_TOLERANCE__' => (string)sanitizeInt('validation_tolerance', 5, 0, 20),
    '__EXTRA_DATASETS__'       => implode(' ', $extraParts),
];

$templateFile = __DIR__ . '/zfs_converter.sh';
if (!file_exists($templateFile)) {
    echo json_encode(['success' => false, 'error' => 'Script not found: ' . $templateFile]);
    exit;
}

$script = file_get_contents($templateFile);
foreach ($vars as $ph => $val) {
    $script = str_replace($ph, $val, $script);
}

$runScript = $tmpDir . '/zfs_converter_run.sh';
file_put_contents($runScript, $script);
chmod($runScript, 0755);

foreach (['conversion_*.log', 'auto_*.log'] as $pattern) {
    $old = glob($tmpDir . '/' . $pattern) ?: [];
    if (count($old) > 10) {
        usort($old, function($a, $b) { return filemtime($b) - filemtime($a); });
        foreach (array_slice($old, 10) as $f) { @unlink($f); }
    }
}

$logFile = $tmpDir . '/conversion_' . date('Ymd_His') . '.log';
$cmd     = 'nohup ' . escapeshellarg($runScript) . ' >' . escapeshellarg($logFile) . ' 2>&1 & echo $!';
$raw     = shell_exec('/bin/bash -c ' . escapeshellarg($cmd));
$pid     = (int)trim($raw ?? '0');

if ($pid <= 0) {
    echo json_encode(['success' => false, 'error' => 'Failed to start process. shell_exec returned: ' . var_export($raw, true)]);
    exit;
}

file_put_contents($statusFile, json_encode([
    'pid'      => $pid,
    'log_file' => $logFile,
    'started'  => date('Y-m-d H:i:s'),
    'status'   => 'running',
]));

echo json_encode(['success' => true, 'pid' => $pid, 'log_file' => $logFile]);
