<?php
header('Content-Type: application/json');

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    echo json_encode(['error' => 'POST required']);
    exit;
}

// Read parameters from $_POST (form-encoded)
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

// Fetch all ZFS dataset names once
$zfsList = [];
exec('zfs list -H -o name 2>/dev/null', $zfsList);
$zfsSet  = array_flip($zfsList);

function humanSize(string $path): string {
    $output = [];
    exec('du -sh ' . escapeshellarg($path) . ' 2>/dev/null', $output);
    return isset($output[0]) ? explode("\t", $output[0])[0] : '-';
}

$result = ['sources' => []];

foreach ($sources as $sourcePath) {
    $fullPath = '/mnt/' . $sourcePath;
    $srcEntry = ['path' => $sourcePath, 'entries' => [], 'error' => null];

    if (!is_dir($fullPath)) {
        $srcEntry['error'] = 'Path not found: ' . $fullPath;
        $result['sources'][] = $srcEntry;
        continue;
    }

    if (!isset($zfsSet[$sourcePath])) {
        $srcEntry['error'] = 'Not a ZFS dataset: ' . $sourcePath;
        $result['sources'][] = $srcEntry;
        continue;
    }

    $entries = glob($fullPath . '/*', GLOB_ONLYDIR) ?: [];

    foreach ($entries as $entry) {
        $name = basename($entry);
        if (substr($name, -5) === '_temp') continue;

        $type = isset($zfsSet[$sourcePath . '/' . $name]) ? 'dataset' : 'folder';
        $srcEntry['entries'][] = [
            'name' => $name,
            'type' => $type,
            'size' => humanSize($entry),
        ];
    }

    usort($srcEntry['entries'], function($a, $b) {
        if ($a['type'] !== $b['type']) return $a['type'] === 'folder' ? -1 : 1;
        return strcasecmp($a['name'], $b['name']);
    });

    $result['sources'][] = $srcEntry;
}

echo json_encode($result);
