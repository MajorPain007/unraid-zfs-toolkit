<?php
// snapshot_browse.php - Backend for the snapshot file browser
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

// Catch fatal errors as JSON
register_shutdown_function(function() {
    $e = error_get_last();
    if ($e && ($e['type'] & (E_ERROR | E_PARSE | E_CORE_ERROR | E_COMPILE_ERROR))) {
        while (ob_get_level() > 0) ob_end_clean();
        header('Content-Type: application/json');
        echo json_encode(array('ok' => false, 'error' => 'Fatal: ' . $e['message']));
    }
});

function zdc_out($data) {
    echo json_encode($data);
    exit;
}

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
        if ($p === '' || $p === '.') continue;
        if ($p === '..') { array_pop($resolved); continue; }
        $resolved[] = $p;
    }
    $full = rtrim($base, '/') . (count($resolved) ? '/' . implode('/', $resolved) : '');
    $base_norm = rtrim($base, '/');
    if ($full !== $base_norm && strpos($full, $base_norm . '/') !== 0) {
        return false;
    }
    return $full;
}

// Sanitise an absolute destination path (no base restriction, but prevents .. escapes and null bytes)
function zdc_safe_abs_path($path) {
    $path = str_replace(array("\0", "\r", "\n"), '', $path);
    $parts = explode('/', ltrim($path, '/'));
    $resolved = array();
    foreach ($parts as $p) {
        if ($p === '' || $p === '.') continue;
        if ($p === '..') { if ($resolved) array_pop($resolved); continue; }
        $resolved[] = $p;
    }
    if (empty($resolved)) return false;
    return '/' . implode('/', $resolved);
}

function zdc_fmt_size($bytes) {
    $bytes = (int)$bytes;
    if ($bytes >= 1073741824) return round($bytes / 1073741824, 1) . ' GB';
    if ($bytes >= 1048576)    return round($bytes / 1048576,    1) . ' MB';
    if ($bytes >= 1024)       return round($bytes / 1024,       0) . ' KB';
    return $bytes . ' B';
}

// Read action from POST or GET
$action = '';
if (isset($_POST['action'])) $action = trim($_POST['action']);
elseif (isset($_GET['action'])) $action = trim($_GET['action']);

// -----------------------------------------------------------------------
// test
// -----------------------------------------------------------------------
if ($action === 'test') {
    zdc_out(array('ok' => true, 'php' => PHP_VERSION, 'time' => date('Y-m-d H:i:s')));
}

// -----------------------------------------------------------------------
// list_snapshots
// -----------------------------------------------------------------------
if ($action === 'list_snapshots') {
    $dataset = isset($_POST['dataset']) ? $_POST['dataset'] : (isset($_GET['dataset']) ? $_GET['dataset'] : '');
    if ($dataset === '' || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid dataset name'));
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
    zdc_out(array('ok' => true, 'snapshots' => $snaps));
}

// -----------------------------------------------------------------------
// browse (POST preferred to avoid ad-blocker URL matching)
// -----------------------------------------------------------------------
if ($action === 'browse') {
    $dataset  = isset($_POST['dataset'])  ? $_POST['dataset']  : (isset($_GET['dataset'])  ? $_GET['dataset']  : '');
    $snapshot = isset($_POST['snapshot']) ? $_POST['snapshot'] : (isset($_GET['snapshot']) ? $_GET['snapshot'] : '');
    $path     = isset($_POST['path'])     ? $_POST['path']     : (isset($_GET['path'])     ? $_GET['path']     : '/');

    if ($dataset === '' || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid dataset name'));
    }
    // Allow letters, digits, hyphens, underscores, dots, colons (for autosnap_2026-04-11_22:50:05)
    if ($snapshot === '' || preg_match('/[^a-zA-Z0-9_.:\-]/', $snapshot)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid snapshot name: ' . htmlspecialchars($snapshot)));
    }

    $mountpoint = zdc_get_mountpoint($dataset);
    if ($mountpoint === '') {
        zdc_out(array('ok' => false, 'error' => 'Dataset not found or not mounted: ' . $dataset));
    }

    // Make .zfs/snapshot visible if needed
    $snap_dir = $mountpoint . '/.zfs/snapshot';
    if (!is_dir($snap_dir)) {
        exec('zfs set snapdir=visible ' . escapeshellarg($dataset) . ' 2>/dev/null');
    }

    $snap_base = $mountpoint . '/.zfs/snapshot/' . $snapshot;
    if (!is_dir($snap_base)) {
        zdc_out(array(
            'ok'    => false,
            'error' => 'Snapshot not accessible at ' . $snap_base
                     . '. Run: zfs set snapdir=visible ' . $dataset,
        ));
    }

    $full_path = zdc_safe_path($snap_base, $path);
    if ($full_path === false) {
        zdc_out(array('ok' => false, 'error' => 'Path traversal detected'));
    }
    if (!is_dir($full_path)) {
        zdc_out(array('ok' => false, 'error' => 'Path not found: ' . $full_path));
    }

    $items = @scandir($full_path);
    if ($items === false) {
        zdc_out(array('ok' => false, 'error' => 'Cannot read directory (permission denied)'));
    }

    $entries = array();
    foreach ($items as $item) {
        if ($item === '.' || $item === '..') continue;
        $item_path = $full_path . '/' . $item;
        $is_dir    = is_dir($item_path);
        $entries[] = array(
            'name'  => $item,
            'type'  => $is_dir ? 'dir' : 'file',
            'size'  => $is_dir ? '' : zdc_fmt_size((int)@filesize($item_path)),
            'mtime' => ($m = (int)@filemtime($item_path)) > 0 ? date('Y-m-d H:i', $m) : '',
        );
    }

    usort($entries, function($a, $b) {
        if ($a['type'] !== $b['type']) return $a['type'] === 'dir' ? -1 : 1;
        return strcasecmp($a['name'], $b['name']);
    });

    $crumbs = array(array('label' => '/', 'path' => '/'));
    $cumulative = '';
    foreach (array_filter(explode('/', ltrim($path, '/'))) as $part) {
        $cumulative .= '/' . $part;
        $crumbs[] = array('label' => $part, 'path' => $cumulative);
    }

    zdc_out(array('ok' => true, 'path' => $path, 'crumbs' => $crumbs, 'entries' => $entries));
}

// -----------------------------------------------------------------------
// restore
// -----------------------------------------------------------------------
if ($action === 'restore') {
    $dataset  = isset($_POST['dataset'])  ? $_POST['dataset']  : '';
    $snapshot = isset($_POST['snapshot']) ? $_POST['snapshot'] : '';
    $src_rel  = isset($_POST['src_path']) ? $_POST['src_path'] : '';
    $dst_rel  = isset($_POST['dst_path']) ? $_POST['dst_path'] : '';

    if ($dataset === '' || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid dataset'));
    }
    if ($snapshot === '' || preg_match('/[^a-zA-Z0-9_.:\-]/', $snapshot)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid snapshot'));
    }

    $mountpoint = zdc_get_mountpoint($dataset);
    if ($mountpoint === '') {
        zdc_out(array('ok' => false, 'error' => 'Dataset not mounted'));
    }

    $snap_base = $mountpoint . '/.zfs/snapshot/' . $snapshot;
    $live_base = $mountpoint;

    $src = zdc_safe_path($snap_base, $src_rel);
    if ($src === false || !file_exists($src)) {
        zdc_out(array('ok' => false, 'error' => 'Source not found in snapshot: ' . $src_rel));
    }

    if ($dst_rel === '') {
        // Restore to original location within dataset
        $dst = zdc_safe_path($live_base, $src_rel);
        if ($dst === false) {
            zdc_out(array('ok' => false, 'error' => 'Invalid source path'));
        }
    } elseif (isset($dst_rel[0]) && $dst_rel[0] === '/') {
        // Absolute destination path — use as-is, sanitised
        $dst = zdc_safe_abs_path($dst_rel);
        if ($dst === false) {
            zdc_out(array('ok' => false, 'error' => 'Invalid destination path'));
        }
    } else {
        // Relative destination — relative to dataset mountpoint
        $dst = zdc_safe_path($live_base, $dst_rel);
        if ($dst === false) {
            zdc_out(array('ok' => false, 'error' => 'Invalid destination path'));
        }
    }

    // Ensure destination directory exists
    if (!is_dir($dst)) {
        if (!mkdir($dst, 0755, true)) {
            zdc_out(array('ok' => false, 'error' => 'Cannot create directory: ' . $dst));
        }
    }

    if (is_dir($src)) {
        // Directory: rsync contents into dst
        $cmd = 'rsync -a ' . escapeshellarg($src . '/') . ' ' . escapeshellarg($dst . '/') . ' 2>&1';
        $check = $dst;
    } else {
        // File: copy into dst directory, keeping original filename
        $cmd = 'cp -a ' . escapeshellarg($src) . ' ' . escapeshellarg($dst . '/') . ' 2>&1';
        $check = $dst . '/' . basename($src);
    }
    $output = shell_exec($cmd);

    if (!file_exists($check)) {
        zdc_out(array('ok' => false, 'error' => 'Restore failed: ' . trim($output)));
    }
    zdc_out(array('ok' => true, 'dst' => $check, 'message' => 'Restored successfully.'));
}

zdc_out(array('ok' => false, 'error' => 'Unknown action: ' . htmlspecialchars($action)));
