<?php
/**
 * scan_folders.php - Return folder/dataset listing for configured sources
 */
header('Content-Type: application/json');

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    echo json_encode(['error' => 'POST required']);
    exit;
}

$input = json_decode(file_get_contents('php://input'), true) ?? [];

// Build list of source paths from submitted settings
$sources = [];

if (($input['should_process_containers'] ?? 'no') === 'yes') {
    $pool    = preg_replace('/[^a-zA-Z0-9_\-.]/', '', $input['appdata_pool']    ?? 'cache');
    $dataset = preg_replace('/[^a-zA-Z0-9_\-.]/', '', $input['appdata_dataset'] ?? 'appdata');
    if ($pool && $dataset) {
        $sources[] = "$pool/$dataset";
    }
}

if (($input['should_process_vms'] ?? 'no') === 'yes') {
    $pool    = preg_replace('/[^a-zA-Z0-9_\-.]/', '', $input['vm_pool']    ?? 'cache');
    $dataset = preg_replace('/[^a-zA-Z0-9_\-.]/', '', $input['vm_dataset'] ?? 'domains');
    if ($pool && $dataset) {
        $sources[] = "$pool/$dataset";
    }
}

$extra = $input['extra_datasets'] ?? '';
foreach (explode(',', $extra) as $e) {
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

// Format bytes to human readable
function humanSize(string $path): string {
    $output = [];
    exec('du -sh ' . escapeshellarg($path) . ' 2>/dev/null', $output);
    if (isset($output[0])) {
        return explode("\t", $output[0])[0];
    }
    return '-';
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

    $entries = glob($fullPath . '/*', GLOB_ONLYDIR);
    if ($entries === false) $entries = [];

    foreach ($entries as $entry) {
        $name = basename($entry);

        // Skip temp dirs from previous runs
        if (str_ends_with($name, '_temp')) continue;

        $childDataset = $sourcePath . '/' . $name;
        $type = isset($zfsSet[$childDataset]) ? 'dataset' : 'folder';

        $srcEntry['entries'][] = [
            'name' => $name,
            'type' => $type,
            'size' => humanSize($entry),
        ];
    }

    // Sort: folders first (will be converted), then datasets
    usort($srcEntry['entries'], function($a, $b) {
        if ($a['type'] === $b['type']) return strcasecmp($a['name'], $b['name']);
        return $a['type'] === 'folder' ? -1 : 1;
    });

    $result['sources'][] = $srcEntry;
}

echo json_encode($result);
