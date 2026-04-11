<?php
// snapshot_browse.php - Backend for the snapshot file browser
// Actions: test, list_snapshots, browse, restore
ob_start();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

// Catch fatal errors and return as JSON instead of crashing
register_shutdown_function(function() {
    $e = error_get_last();
    if ($e && ($e['type'] & (E_ERROR | E_PARSE | E_CORE_ERROR | E_COMPILE_ERROR))) {
        ob_end_clean();
        header('Content-Type: application/json');
        echo json_encode(array('ok' => false, 'error' => 'Fatal PHP error: ' . $e['message']));
    } else {
        ob_end_flush();
    }
});

set_error_handler(function($errno, $errstr) {
    ob_end_clean();
    header('Content-Type: application/json');
    echo json_encode(array('ok' => false, 'error' => 'PHP error [' . $errno . ']: ' . $errstr));
    exit;
});

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------
function zdc_get_mountpoint($dataset) {
    $out = array();
    exec('zfs list -H -o mountpoint ' . escapeshellarg($dataset) . ' 2>/dev/null', $out);
    return isset($out[0]) ? trim($out[0]) : '';
}

function zdc_safe_path($base, $rel) {
    $rel = str_replace(array("\0", "\r", "\n"), '', $rel);
    $parts = explode('/', ltrim($rel, '/'));
    $resolved = array();
    foreach ($parts as $p) {
        if ($p === '' || $p === '.') { continue; }
        if ($p === '..') { array_pop($resolved); continue; }
        $resolved[] = $p;
    }
    $full = rtrim($base, '/') . '/' . implode('/', $resolved);
    $base_norm = rtrim($base, '/');
    if (strpos($full, $base_norm . '/') !== 0 && $full !== $base_norm) {
        return false;
    }
    return $full;
}

function zdc_fmt_size($bytes) {
    $bytes = (int)$bytes;
    if ($bytes >= 1073741824) return round($bytes / 1073741824, 1) . ' GB';
    if ($bytes >= 1048576)    return round($bytes / 1048576,    1) . ' MB';
    if ($bytes >= 1024)       return round($bytes / 1024,       0) . ' KB';
    return $bytes . ' B';
}

// -----------------------------------------------------------------------
// Dispatch
// -----------------------------------------------------------------------
$action = '';
if (isset($_GET['action']))  $action = $_GET['action'];
if (isset($_POST['action'])) $action = $_POST['action'];

// -----------------------------------------------------------------------
// test - verify PHP execution and basic ZFS access
// -----------------------------------------------------------------------
if ($action === 'test') {
    $zfs_ok = is_executable('/sbin/zfs') || is_executable('/usr/sbin/zfs');
    echo json_encode(array(
        'ok'       => true,
        'php'      => PHP_VERSION,
        'zfs_bin'  => $zfs_ok,
        'time'     => date('Y-m-d H:i:s'),
    ));
    exit;
}

// -----------------------------------------------------------------------
// list_snapshots?dataset=pool/name
// -----------------------------------------------------------------------
if ($action === 'list_snapshots') {
    $dataset = isset($_GET['dataset']) ? $_GET['dataset'] : '';
    if ($dataset === '' || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        echo json_encode(array('ok' => false, 'error' => 'Invalid dataset name'));
        exit;
    }
    $lines = array();
    exec('zfs list -H -t snapshot -o name -s creation ' . escapeshellarg($dataset) . ' 2>/dev/null', $lines);
    $snaps = array();
    foreach (array_reverse($lines) as $line) {
        $line = trim($line);
        $at = strpos($line, '@');
        if ($at !== false) {
            $snaps[] = substr($line, $at + 1);
        }
    }
    echo json_encode(array('ok' => true, 'snapshots' => $snaps));
    exit;
}

// -----------------------------------------------------------------------
// browse?dataset=pool/name&snapshot=name&path=/sub/dir
// -----------------------------------------------------------------------
if ($action === 'browse') {
    // Accept both GET and POST (POST avoids ad-blocker URL matching on snapshot names with timestamps)
    $dataset  = isset($_POST['dataset'])  ? $_POST['dataset']  : (isset($_GET['dataset'])  ? $_GET['dataset']  : '');
    $snapshot = isset($_POST['snapshot']) ? $_POST['snapshot'] : (isset($_GET['snapshot']) ? $_GET['snapshot'] : '');
    $path     = isset($_POST['path'])     ? $_POST['path']     : (isset($_GET['path'])     ? $_GET['path']     : '/');

    if ($dataset === '' || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        echo json_encode(array('ok' => false, 'error' => 'Invalid dataset name'));
        exit;
    }
    // Allow alphanumeric, hyphen, underscore, dot (snapshot names like auto-daily-2026-04-11)
    if ($snapshot === '' || preg_match('/[^a-zA-Z0-9_.:-]/', $snapshot)) {
        echo json_encode(array('ok' => false, 'error' => 'Invalid snapshot name: ' . $snapshot));
        exit;
    }

    $mountpoint = zdc_get_mountpoint($dataset);
    if ($mountpoint === '') {
        echo json_encode(array('ok' => false, 'error' => 'Dataset not mounted or not found: ' . $dataset));
        exit;
    }

    // Check .zfs visibility — try to make snapdir visible if not already
    $snap_dir = $mountpoint . '/.zfs';
    if (!is_dir($snap_dir)) {
        // Try enabling snapdir visibility
        exec('zfs set snapdir=visible ' . escapeshellarg($dataset) . ' 2>/dev/null');
    }

    $snap_base = $mountpoint . '/.zfs/snapshot/' . $snapshot;
    if (!is_dir($snap_base)) {
        echo json_encode(array(
            'ok'    => false,
            'error' => 'Snapshot directory not accessible: ' . $snap_base
                     . '. Try running: zfs set snapdir=visible ' . $dataset,
        ));
        exit;
    }

    $full_path = zdc_safe_path($snap_base, $path);
    if ($full_path === false) {
        echo json_encode(array('ok' => false, 'error' => 'Path traversal detected'));
        exit;
    }
    if (!is_dir($full_path)) {
        echo json_encode(array('ok' => false, 'error' => 'Path not found in snapshot: ' . $full_path));
        exit;
    }

    $items = @scandir($full_path);
    if ($items === false) {
        echo json_encode(array('ok' => false, 'error' => 'Cannot read directory (permission denied?)'));
        exit;
    }

    $entries = array();
    foreach ($items as $item) {
        if ($item === '.' || $item === '..') { continue; }
        $item_path = $full_path . '/' . $item;
        $is_dir    = is_dir($item_path);
        $size_raw  = $is_dir ? 0 : (int)@filesize($item_path);
        $mtime     = (int)@filemtime($item_path);
        $entries[] = array(
            'name'  => $item,
            'type'  => $is_dir ? 'dir' : 'file',
            'size'  => $is_dir ? '' : zdc_fmt_size($size_raw),
            'mtime' => $mtime > 0 ? date('Y-m-d H:i', $mtime) : '',
        );
    }

    usort($entries, function($a, $b) {
        if ($a['type'] !== $b['type']) { return $a['type'] === 'dir' ? -1 : 1; }
        return strcasecmp($a['name'], $b['name']);
    });

    // Breadcrumb
    $crumbs = array(array('label' => '/', 'path' => '/'));
    $path_parts = array_filter(explode('/', ltrim($path, '/')));
    $cumulative = '';
    foreach ($path_parts as $part) {
        $cumulative .= '/' . $part;
        $crumbs[] = array('label' => $part, 'path' => $cumulative);
    }

    echo json_encode(array(
        'ok'      => true,
        'path'    => $path,
        'crumbs'  => $crumbs,
        'entries' => $entries,
    ));
    exit;
}

// -----------------------------------------------------------------------
// restore (POST)
// -----------------------------------------------------------------------
if ($action === 'restore') {
    $dataset  = isset($_POST['dataset'])  ? $_POST['dataset']  : '';
    $snapshot = isset($_POST['snapshot']) ? $_POST['snapshot'] : '';
    $src_rel  = isset($_POST['src_path']) ? $_POST['src_path'] : '';
    $dst_rel  = isset($_POST['dst_path']) ? $_POST['dst_path'] : '';

    if ($dataset === '' || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        echo json_encode(array('ok' => false, 'error' => 'Invalid dataset'));
        exit;
    }
    if ($snapshot === '' || preg_match('/[^a-zA-Z0-9_.:-]/', $snapshot)) {
        echo json_encode(array('ok' => false, 'error' => 'Invalid snapshot'));
        exit;
    }

    $mountpoint = zdc_get_mountpoint($dataset);
    if ($mountpoint === '') {
        echo json_encode(array('ok' => false, 'error' => 'Dataset not mounted'));
        exit;
    }

    $snap_base = $mountpoint . '/.zfs/snapshot/' . $snapshot;
    $live_base = $mountpoint;

    $src = zdc_safe_path($snap_base, $src_rel);
    if ($src === false || !file_exists($src)) {
        echo json_encode(array('ok' => false, 'error' => 'Source not found in snapshot: ' . $src_rel));
        exit;
    }

    if ($dst_rel === '') { $dst_rel = $src_rel; }
    $dst = zdc_safe_path($live_base, $dst_rel);
    if ($dst === false) {
        echo json_encode(array('ok' => false, 'error' => 'Invalid destination path'));
        exit;
    }

    $dst_dir = dirname($dst);
    if (!is_dir($dst_dir)) {
        if (!mkdir($dst_dir, 0755, true)) {
            echo json_encode(array('ok' => false, 'error' => 'Cannot create destination directory: ' . $dst_dir));
            exit;
        }
    }

    if (is_dir($src)) {
        $cmd = 'rsync -a ' . escapeshellarg($src . '/') . ' ' . escapeshellarg($dst . '/') . ' 2>&1';
    } else {
        $cmd = 'cp -a ' . escapeshellarg($src) . ' ' . escapeshellarg($dst) . ' 2>&1';
    }
    $output = shell_exec($cmd);

    if (!file_exists($dst)) {
        echo json_encode(array('ok' => false, 'error' => 'Restore failed: ' . trim($output)));
        exit;
    }

    echo json_encode(array(
        'ok'      => true,
        'src'     => $src,
        'dst'     => $dst,
        'message' => 'Restored successfully.',
    ));
    exit;
}

echo json_encode(array('ok' => false, 'error' => 'Unknown action: ' . $action));
