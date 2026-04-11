<?php
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

set_error_handler(function(int $errno, string $errstr): bool {
    echo json_encode(['error' => "PHP[$errno]: $errstr"]);
    exit(1);
});
register_shutdown_function(function(): void {
    $e = error_get_last();
    if ($e && ($e['type'] & (E_ERROR | E_PARSE | E_CORE_ERROR | E_COMPILE_ERROR))) {
        while (ob_get_level() > 0) ob_end_clean();
        header('Content-Type: application/json');
        echo json_encode(['error' => 'Fatal: ' . $e['message']]);
    }
});

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    echo json_encode(['error' => 'POST required']);
    exit;
}

$sources = [];

if (($_POST['should_process_containers'] ?? 'no') === 'yes') {
    $pool    = preg_replace('/[^a-zA-Z0-9_\-.]/', '', $_POST['appdata_pool']    ?? 'cache');
    $dataset = preg_replace('/[^a-zA-Z0-9_\-.]/', '', $_POST['appdata_dataset'] ?? 'appdata');
    if ($pool && $dataset) $sources[] = "$pool/$dataset";
}

if (($_POST['should_process_vms'] ?? 'no') === 'yes') {
    $pool    = preg_replace('/[^a-zA-Z0-9_\-.]/', '', $_POST['vm_pool']    ?? 'cache');
    $dataset = preg_replace('/[^a-zA-Z0-9_\-.]/', '', $_POST['vm_dataset'] ?? 'domains');
    if ($pool && $dataset) $sources[] = "$pool/$dataset";
}

foreach (explode(',', $_POST['extra_datasets'] ?? '') as $e) {
    $e = trim($e);
    if ($e !== '' && preg_match('/^[a-zA-Z0-9_\-.\/ ]+$/', $e)) {
        $sources[] = $e;
    }
}

$sources = array_unique(array_filter($sources));

$zfsList = [];
exec('zfs list -H -o name 2>/dev/null', $zfsList);
$zfsSet  = array_flip($zfsList);

function humanSize(string $path): string {
    $out = [];
    exec('du -sh ' . escapeshellarg($path) . ' 2>/dev/null', $out);
    return isset($out[0]) ? explode("\t", $out[0])[0] : '-';
}

$result = ['sources' => []];

foreach ($sources as $sourcePath) {
    $fullPath = '/mnt/' . $sourcePath;
    $entry    = ['path' => $sourcePath, 'entries' => [], 'error' => null];

    if (!is_dir($fullPath)) {
        $entry['error'] = 'Path not found: ' . $fullPath;
        $result['sources'][] = $entry;
        continue;
    }
    if (!isset($zfsSet[$sourcePath])) {
        $entry['error'] = 'Not a ZFS dataset: ' . $sourcePath;
        $result['sources'][] = $entry;
        continue;
    }

    foreach (glob($fullPath . '/*', GLOB_ONLYDIR) ?: [] as $dir) {
        $name = basename($dir);
        if (substr($name, -5) === '_temp') continue;
        $type = isset($zfsSet[$sourcePath . '/' . $name]) ? 'dataset' : 'folder';
        $entry['entries'][] = ['name' => $name, 'type' => $type, 'size' => humanSize($dir)];
    }

    usort($entry['entries'], function($a, $b) {
        if ($a['type'] !== $b['type']) return $a['type'] === 'folder' ? -1 : 1;
        return strcasecmp($a['name'], $b['name']);
    });

    $result['sources'][] = $entry;
}

echo json_encode($result);
