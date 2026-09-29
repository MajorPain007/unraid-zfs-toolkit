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
    $pool    = preg_replace('#[^A-Za-z0-9_.: /-]#', '', $_POST['appdata_pool']    ?? 'cache');
    $dataset = preg_replace('#[^A-Za-z0-9_.: /-]#', '', $_POST['appdata_dataset'] ?? 'appdata');
    if ($pool && $dataset) $sources[] = "$pool/$dataset";
}

if (($_POST['should_process_vms'] ?? 'no') === 'yes') {
    $pool    = preg_replace('#[^A-Za-z0-9_.: /-]#', '', $_POST['vm_pool']    ?? 'cache');
    $dataset = preg_replace('#[^A-Za-z0-9_.: /-]#', '', $_POST['vm_dataset'] ?? 'domains');
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

// Where datasets are mounted. A folder counts as converted when a dataset is
// mounted at its path, whatever that dataset is called: one whose name ZFS
// cannot take is converted under another name and mounted where it was.
$mounted = [];
$mountLines = [];
exec('zfs list -H -o mounted,mountpoint 2>/dev/null', $mountLines);
foreach ($mountLines as $line) {
    $f = explode("\t", $line, 2);
    if (count($f) === 2 && $f[0] === 'yes') $mounted[$f[1]] = true;
}

$replaceSpaces = ($_POST['replace_spaces'] ?? 'no') === 'yes';

/** The dataset name zfs_converter.sh gives a folder - see dataset_name_for there. */
function zdcDatasetName(string $name, bool $replaceSpaces): string {
    if ($replaceSpaces) $name = str_replace(' ', '_', $name);
    if ($name !== '.' && $name !== '..' && strlen($name) <= 200
        && preg_match('/^[A-Za-z0-9_.: -]+$/', $name)) {
        return $name;
    }
    $name = strtr($name, ['ä' => 'ae', 'ö' => 'oe', 'ü' => 'ue', 'Ä' => 'Ae', 'Ö' => 'Oe', 'Ü' => 'Ue', 'ß' => 'ss']);
    return preg_replace('/[^A-Za-z0-9_.: -]/', '_', $name);
}

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

    // The same decisions the converter makes when it plans a run.
    foreach (glob($fullPath . '/*', GLOB_ONLYDIR) ?: [] as $dir) {
        if (is_link($dir)) continue;
        $name = basename($dir);
        $row  = ['name' => $name, 'size' => humanSize($dir), 'as' => '', 'why' => ''];

        if (isset($mounted[$dir])) {
            $row['type'] = 'dataset';
        } elseif (substr($name, -5) === '_temp' && isset($mounted[substr($dir, 0, -5)])) {
            $row['type'] = 'kept';
            $row['why']  = 'Next to the dataset of the same name without _temp: a copy kept with'
                         . ' Cleanup off, or an original an older version left behind. Left alone.';
        } else {
            $ds = zdcDatasetName($name, $replaceSpaces);
            if (isset($zfsSet[$sourcePath . '/' . $ds])) {
                $row['type'] = 'kept';
                $row['why']  = 'The dataset ' . $sourcePath . '/' . $ds . ' exists but is not mounted here.';
            } else {
                $row['type'] = 'folder';
                if ($ds !== $name) $row['as'] = $ds;
            }
        }
        $entry['entries'][] = $row;
    }

    $order = ['folder' => 0, 'kept' => 1, 'dataset' => 2];
    usort($entry['entries'], function($a, $b) use ($order) {
        if ($a['type'] !== $b['type']) return $order[$a['type']] - $order[$b['type']];
        return strcasecmp($a['name'], $b['name']);
    });

    $result['sources'][] = $entry;
}

echo json_encode($result);
