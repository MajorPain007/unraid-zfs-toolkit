<?php
/**
 * start_conversion.php - Generate and launch the conversion script as a background job
 */
header('Content-Type: application/json');

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    echo json_encode(['success' => false, 'error' => 'POST required']);
    exit;
}

$input = json_decode(file_get_contents('php://input'), true);
if (!is_array($input)) {
    echo json_encode(['success' => false, 'error' => 'Invalid JSON body']);
    exit;
}

$statusFile = '/tmp/zfs.dataset.converter/status.json';
$tmpDir     = '/tmp/zfs.dataset.converter';

// Prevent concurrent runs
if (file_exists($statusFile)) {
    $st = json_decode(file_get_contents($statusFile), true);
    if (isset($st['pid']) && file_exists('/proc/' . (int)$st['pid'])) {
        echo json_encode(['success' => false, 'error' => 'A conversion is already running (PID ' . (int)$st['pid'] . ')']);
        exit;
    }
}

if (!is_dir($tmpDir)) mkdir($tmpDir, 0755, true);

// -----------------------------------------------------------------------
// Sanitize helpers
// -----------------------------------------------------------------------
function sanitizeBool(mixed $v, string $default = 'no'): string {
    $v = strtolower(trim((string)($v ?? $default)));
    return in_array($v, ['yes','true','1'], true) ? 'yes' : 'no';
}
function sanitizePath(mixed $v, string $default = ''): string {
    return preg_replace('/[^a-zA-Z0-9_\-. ]/', '', (string)($v ?? $default));
}
function sanitizeInt(mixed $v, int $default, int $min, int $max): int {
    $n = (int)($v ?? $default);
    return max($min, min($max, $n));
}

// -----------------------------------------------------------------------
// Build extra datasets array string for the shell script
// -----------------------------------------------------------------------
$extraRaw = $input['extra_datasets'] ?? '';
$extraParts = [];
foreach (explode(',', $extraRaw) as $e) {
    $e = trim($e);
    if ($e !== '' && preg_match('/^[a-zA-Z0-9_\-.\/ ]+$/', $e)) {
        $extraParts[] = '"' . $e . '"';
    }
}
$extraDatasetsStr = implode(' ', $extraParts);

// -----------------------------------------------------------------------
// Template variable substitutions
// -----------------------------------------------------------------------
$vars = [
    '__DRY_RUN__'           => sanitizeBool($input['dry_run'], 'yes'),
    '__CLEANUP__'           => sanitizeBool($input['cleanup'], 'yes'),
    '__REPLACE_SPACES__'    => sanitizeBool($input['replace_spaces'], 'no'),
    '__PROCESS_CONTAINERS__'=> sanitizeBool($input['should_process_containers'], 'yes'),
    '__APPDATA_POOL__'      => sanitizePath($input['appdata_pool'], 'cache'),
    '__APPDATA_DATASET__'   => sanitizePath($input['appdata_dataset'], 'appdata'),
    '__PROCESS_VMS__'       => sanitizeBool($input['should_process_vms'], 'no'),
    '__VM_POOL__'           => sanitizePath($input['vm_pool'], 'cache'),
    '__VM_DATASET__'        => sanitizePath($input['vm_dataset'], 'domains'),
    '__VM_SHUTDOWN_WAIT__'  => (string)sanitizeInt($input['vm_forceshutdown_wait'] ?? 90, 90, 10, 600),
    '__BUFFER_ZONE__'       => (string)sanitizeInt($input['buffer_zone'] ?? 11, 11, 0, 100),
    '__SEND_NOTIFICATIONS__'=> sanitizeBool($input['send_notifications'], 'yes'),
    '__VALIDATION_TOLERANCE__'=> (string)sanitizeInt($input['validation_tolerance'] ?? 5, 5, 0, 20),
    '__EXTRA_DATASETS__'    => $extraDatasetsStr,
];

$templateFile = __DIR__ . '/zfs_converter.sh';
if (!file_exists($templateFile)) {
    echo json_encode(['success' => false, 'error' => 'Template script not found: ' . $templateFile]);
    exit;
}

$script = file_get_contents($templateFile);
foreach ($vars as $placeholder => $value) {
    $script = str_replace($placeholder, $value, $script);
}

// Write generated script
$scriptFile = $tmpDir . '/zfs_converter_run.sh';
file_put_contents($scriptFile, $script);
chmod($scriptFile, 0755);

// Log file for this run
$logFile = $tmpDir . '/conversion_' . date('Ymd_His') . '.log';

// Launch as background process
$cmd = 'nohup ' . escapeshellarg($scriptFile) . ' >' . escapeshellarg($logFile) . ' 2>&1 & echo $!';
$pid = (int)trim(shell_exec($cmd));

if ($pid <= 0) {
    echo json_encode(['success' => false, 'error' => 'Failed to start background process']);
    exit;
}

// Write status
file_put_contents($statusFile, json_encode([
    'pid'      => $pid,
    'log_file' => $logFile,
    'started'  => date('Y-m-d H:i:s'),
    'status'   => 'running',
]));

echo json_encode(['success' => true, 'pid' => $pid, 'log_file' => $logFile]);
