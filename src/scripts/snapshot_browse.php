<?php
// snapshot_browse.php - Backend for the snapshot file browser
// Actions: list_snapshots, browse, restore
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

set_error_handler(function($errno, $errstr) {
    echo json_encode(['ok' => false, 'error' => "PHP error: $errstr"]);
    exit;
});

// -----------------------------------------------------------------------
// Get the real mountpoint for a ZFS dataset
// -----------------------------------------------------------------------
function get_mountpoint(string $dataset): string {
    $out = [];
    exec('zfs list -H -o mountpoint ' . escapeshellarg($dataset) . ' 2>/dev/null', $out);
    return trim($out[0] ?? '');
}

// -----------------------------------------------------------------------
// Validate that a resolved path stays within the allowed base directory
// -----------------------------------------------------------------------
function safe_path(string $base, string $rel): string|false {
    // Normalize: strip leading slash, prevent traversal
    $rel = ltrim(str_replace(['..', "\0"], '', $rel), '/');
    $full = realpath($base . '/' . $rel);
    if ($full === false) {
        // realpath fails if path doesn't exist — build it manually for non-existent paths
        $full = $base . '/' . $rel;
    }
    // Ensure it stays within base
    $base_real = realpath($base) ?: $base;
    if (strpos($full, $base_real) !== 0) {
        return false;
    }
    return $full;
}

// -----------------------------------------------------------------------
// Format bytes human-readable
// -----------------------------------------------------------------------
function fmt_size(int $bytes): string {
    if ($bytes >= 1073741824) return round($bytes/1073741824, 1) . ' GB';
    if ($bytes >= 1048576)    return round($bytes/1048576,    1) . ' MB';
    if ($bytes >= 1024)       return round($bytes/1024,       0) . ' KB';
    return $bytes . ' B';
}

$action = $_GET['action'] ?? ($_POST['action'] ?? '');

// -----------------------------------------------------------------------
// list_snapshots?dataset=pool/name
// Returns list of snapshot names for the dataset, newest first
// -----------------------------------------------------------------------
if ($action === 'list_snapshots') {
    $dataset = $_GET['dataset'] ?? '';
    if (empty($dataset) || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        echo json_encode(['ok' => false, 'error' => 'Invalid dataset']);
        exit;
    }

    $lines = [];
    exec('zfs list -H -t snapshot -o name -s creation ' . escapeshellarg($dataset) . ' 2>/dev/null', $lines);

    $snaps = [];
    foreach (array_reverse($lines) as $line) {  // reverse = newest first
        $line = trim($line);
        if (strpos($line, '@') !== false) {
            $snaps[] = substr($line, strpos($line, '@') + 1);
        }
    }

    echo json_encode(['ok' => true, 'snapshots' => $snaps]);
    exit;
}

// -----------------------------------------------------------------------
// browse?dataset=pool/name&snapshot=auto-daily-...&path=/sub/dir
// Returns directory listing within the snapshot
// -----------------------------------------------------------------------
if ($action === 'browse') {
    $dataset  = $_GET['dataset']  ?? '';
    $snapshot = $_GET['snapshot'] ?? '';
    $path     = $_GET['path']     ?? '/';

    if (empty($dataset) || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        echo json_encode(['ok' => false, 'error' => 'Invalid dataset']);
        exit;
    }
    if (empty($snapshot) || preg_match('/[^a-zA-Z0-9_.-]/', $snapshot)) {
        echo json_encode(['ok' => false, 'error' => 'Invalid snapshot name']);
        exit;
    }

    $mountpoint = get_mountpoint($dataset);
    if (empty($mountpoint)) {
        echo json_encode(['ok' => false, 'error' => 'Dataset not mounted']);
        exit;
    }

    $snap_base = $mountpoint . '/.zfs/snapshot/' . $snapshot;
    if (!is_dir($snap_base)) {
        echo json_encode(['ok' => false, 'error' => 'Snapshot directory not found: ' . $snap_base]);
        exit;
    }

    $full_path = safe_path($snap_base, $path);
    if ($full_path === false || !is_dir($full_path)) {
        echo json_encode(['ok' => false, 'error' => 'Invalid path']);
        exit;
    }

    $entries = [];
    $items = @scandir($full_path);
    if ($items === false) {
        echo json_encode(['ok' => false, 'error' => 'Cannot read directory']);
        exit;
    }

    foreach ($items as $item) {
        if ($item === '.' || $item === '..') continue;
        $item_path = $full_path . '/' . $item;
        $is_dir    = is_dir($item_path);
        $size      = $is_dir ? 0 : (@filesize($item_path) ?: 0);
        $mtime     = @filemtime($item_path) ?: 0;
        $entries[] = [
            'name'  => $item,
            'type'  => $is_dir ? 'dir' : 'file',
            'size'  => $is_dir ? '' : fmt_size($size),
            'mtime' => $mtime ? date('Y-m-d H:i', $mtime) : '',
        ];
    }

    // Sort: dirs first, then files, both alphabetically
    usort($entries, function($a, $b) {
        if ($a['type'] !== $b['type']) return $a['type'] === 'dir' ? -1 : 1;
        return strcasecmp($a['name'], $b['name']);
    });

    // Build breadcrumb
    $parts = array_filter(explode('/', ltrim($path, '/')));
    $crumbs = [['label' => '/', 'path' => '/']];
    $cumulative = '';
    foreach ($parts as $part) {
        $cumulative .= '/' . $part;
        $crumbs[] = ['label' => $part, 'path' => $cumulative];
    }

    echo json_encode([
        'ok'      => true,
        'path'    => $path,
        'crumbs'  => $crumbs,
        'entries' => $entries,
    ]);
    exit;
}

// -----------------------------------------------------------------------
// restore  (POST)
// Copies a file or directory from snapshot back to the live dataset
// -----------------------------------------------------------------------
if ($action === 'restore') {
    $dataset  = $_POST['dataset']  ?? '';
    $snapshot = $_POST['snapshot'] ?? '';
    $src_rel  = $_POST['src_path'] ?? '';   // path relative to snapshot root
    $dst_rel  = $_POST['dst_path'] ?? '';   // path relative to live dataset root (defaults to src_rel)

    if (empty($dataset) || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        echo json_encode(['ok' => false, 'error' => 'Invalid dataset']);
        exit;
    }
    if (empty($snapshot) || preg_match('/[^a-zA-Z0-9_.-]/', $snapshot)) {
        echo json_encode(['ok' => false, 'error' => 'Invalid snapshot']);
        exit;
    }

    $mountpoint = get_mountpoint($dataset);
    if (empty($mountpoint)) {
        echo json_encode(['ok' => false, 'error' => 'Dataset not mounted']);
        exit;
    }

    $snap_base = $mountpoint . '/.zfs/snapshot/' . $snapshot;
    $live_base = $mountpoint;

    $src = safe_path($snap_base, $src_rel);
    if ($src === false || !file_exists($src)) {
        echo json_encode(['ok' => false, 'error' => 'Source path not found in snapshot']);
        exit;
    }

    // Default destination: same relative path in live dataset
    if (empty($dst_rel)) $dst_rel = $src_rel;
    $dst = safe_path($live_base, $dst_rel);
    if ($dst === false) {
        echo json_encode(['ok' => false, 'error' => 'Invalid destination path']);
        exit;
    }

    // Ensure destination parent directory exists
    $dst_dir = dirname($dst);
    if (!is_dir($dst_dir)) {
        if (!mkdir($dst_dir, 0755, true)) {
            echo json_encode(['ok' => false, 'error' => 'Cannot create destination directory']);
            exit;
        }
    }

    if (is_dir($src)) {
        // Use rsync for directories (preserves permissions, handles overwrites cleanly)
        $cmd = 'rsync -a ' . escapeshellarg($src . '/') . ' ' . escapeshellarg($dst . '/') . ' 2>&1';
    } else {
        // Single file: cp -a
        $cmd = 'cp -a ' . escapeshellarg($src) . ' ' . escapeshellarg($dst) . ' 2>&1';
    }

    $output = shell_exec($cmd);
    $rc = 0;
    // Check if command succeeded by testing destination exists
    if (!file_exists($dst)) {
        echo json_encode(['ok' => false, 'error' => 'Restore failed: ' . trim($output)]);
        exit;
    }

    echo json_encode([
        'ok'      => true,
        'src'     => $src,
        'dst'     => $dst,
        'message' => 'Restored successfully.',
    ]);
    exit;
}

echo json_encode(['ok' => false, 'error' => 'Unknown action: ' . htmlspecialchars($action)]);
