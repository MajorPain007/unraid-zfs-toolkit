<?php

while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

register_shutdown_function(function() {
    $e = error_get_last();
    if ($e && ($e['type'] & (E_ERROR | E_PARSE | E_CORE_ERROR | E_COMPILE_ERROR))) {
        while (ob_get_level() > 0) ob_end_clean();
        header('Content-Type: application/json');
        echo json_encode(array('ok' => false, 'error' => 'Fatal: ' . $e['message']));
    }
});

define('ZDC_RSYNC',  '-a -H -A -X --numeric-ids');
define('ZDC_TMP',    '/tmp/zfs.dataset.converter');
define('ZDC_PLUGIN', '/usr/local/emhttp/plugins/zfs.dataset.converter');

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

$action = '';
if (isset($_POST['action'])) $action = trim($_POST['action']);
elseif (isset($_GET['action'])) $action = trim($_GET['action']);

if ($action === 'test') {
    zdc_out(array('ok' => true, 'php' => PHP_VERSION, 'time' => date('Y-m-d H:i:s')));
}

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

if ($action === 'browse') {
    $dataset  = isset($_POST['dataset'])  ? $_POST['dataset']  : (isset($_GET['dataset'])  ? $_GET['dataset']  : '');
    $snapshot = isset($_POST['snapshot']) ? $_POST['snapshot'] : (isset($_GET['snapshot']) ? $_GET['snapshot'] : '');
    $path     = isset($_POST['path'])     ? $_POST['path']     : (isset($_GET['path'])     ? $_GET['path']     : '/');

    if ($dataset === '' || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid dataset name'));
    }

    if ($snapshot === '' || preg_match('/[^a-zA-Z0-9_.:\-]/', $snapshot)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid snapshot name: ' . htmlspecialchars($snapshot)));
    }

    $mountpoint = zdc_get_mountpoint($dataset);
    if ($mountpoint === '') {
        zdc_out(array('ok' => false, 'error' => 'Dataset not found or not mounted: ' . $dataset));
    }

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

if ($action === 'restore_start') {
    $dataset  = isset($_POST['dataset'])  ? $_POST['dataset']  : '';
    $snapshot = isset($_POST['snapshot']) ? $_POST['snapshot'] : '';
    $dst_rel  = isset($_POST['dst_path']) ? $_POST['dst_path'] : '';
    $items    = isset($_POST['items'])    ? $_POST['items']    : array();

    if (!is_array($items)) $items = array($items);
    if (!$items) zdc_out(array('ok' => false, 'error' => 'Nothing selected'));
    if (count($items) > 1000) zdc_out(array('ok' => false, 'error' => 'Too many items (max 1000)'));

    if ($dataset === '' || preg_match('/[^a-zA-Z0-9\/_.-]/', $dataset)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid dataset'));
    }
    if ($snapshot === '' || preg_match('/[^a-zA-Z0-9_.:\-]/', $snapshot)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid snapshot'));
    }

    $mountpoint = zdc_get_mountpoint($dataset);
    if ($mountpoint === '') zdc_out(array('ok' => false, 'error' => 'Dataset not mounted'));

    $snap_base = $mountpoint . '/.zfs/snapshot/' . $snapshot;

    $dst_dir_fixed = null;
    if ($dst_rel !== '') {
        $dst_dir_fixed = (isset($dst_rel[0]) && $dst_rel[0] === '/')
            ? zdc_safe_abs_path($dst_rel)
            : zdc_safe_path($mountpoint, $dst_rel);
        if ($dst_dir_fixed === false) {
            zdc_out(array('ok' => false, 'error' => 'Invalid destination path'));
        }
        if (strpos($dst_dir_fixed, '/.zfs/') !== false) {
            zdc_out(array('ok' => false, 'error' => 'Destination is inside a snapshot (read-only)'));
        }
    }

    $lines = array();
    foreach ($items as $rel) {
        $rel = (string)$rel;

        if (strpbrk($rel, "\t\n\r\0") !== false) {
            zdc_out(array('ok' => false, 'error' => 'Unsupported character in path: ' . htmlspecialchars($rel)));
        }
        $src = zdc_safe_path($snap_base, $rel);
        if ($src === false || !file_exists($src)) {
            zdc_out(array('ok' => false, 'error' => 'Source not found in snapshot: ' . htmlspecialchars($rel)));
        }

        $mode = is_dir($src) ? 'dir' : 'file';

        if ($dst_dir_fixed !== null) {
            $dst_dir = $dst_dir_fixed;
        } else {

            $live = zdc_safe_path($mountpoint, $rel);
            if ($live === false) {
                zdc_out(array('ok' => false, 'error' => 'Invalid source path: ' . htmlspecialchars($rel)));
            }
            $dst_dir = dirname($live);
        }

        if ($mode === 'dir' && strpos(rtrim($dst_dir, '/') . '/', rtrim($src, '/') . '/') === 0) {
            zdc_out(array('ok' => false, 'error' => 'Destination is inside the source folder'));
        }

        $lines[] = $src . "\t" . $dst_dir . "\t" . $mode;
    }

    if (!is_dir(ZDC_TMP)) @mkdir(ZDC_TMP, 0755, true);

    $job     = date('Ymd_His') . '_' . mt_rand(1000, 9999);
    $jobFile = ZDC_TMP . '/restore_' . $job . '.job';
    if (file_put_contents($jobFile, implode("\n", $lines) . "\n") === false) {
        zdc_out(array('ok' => false, 'error' => 'Cannot write job file'));
    }

    $worker = ZDC_PLUGIN . '/scripts/restore_worker.sh';
    if (!file_exists($worker)) {
        zdc_out(array('ok' => false, 'error' => 'restore_worker.sh not found'));
    }

    $cmd = 'nohup /bin/bash ' . escapeshellarg($worker) . ' ' . escapeshellarg($jobFile)
         . ' >/dev/null 2>&1 & echo $!';
    $pid = (int)trim((string)shell_exec('/bin/bash -c ' . escapeshellarg($cmd)));
    if ($pid <= 0) {
        zdc_out(array('ok' => false, 'error' => 'Failed to start restore worker'));
    }

    zdc_out(array('ok' => true, 'job' => $job, 'pid' => $pid, 'total' => count($lines)));
}

if ($action === 'restore_status') {
    $job = isset($_POST['job']) ? $_POST['job'] : '';
    if (!preg_match('/^[0-9]{8}_[0-9]{6}_[0-9]{4}$/', $job)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid job id'));
    }

    $statusFile = ZDC_TMP . '/restore_' . $job . '.status';
    $logFile    = ZDC_TMP . '/restore_' . $job . '.log';

    if (!file_exists($statusFile)) {
        zdc_out(array('ok' => true, 'state' => 'starting'));
    }

    $st = json_decode(file_get_contents($statusFile), true);
    if (!is_array($st)) zdc_out(array('ok' => true, 'state' => 'starting'));

    $log = array();
    if (file_exists($logFile)) {
        $fh = @fopen($logFile, 'rb');
        if ($fh) {
            $size = filesize($logFile);
            fseek($fh, max(0, $size - 8192));
            $chunk = fread($fh, 8192);
            fclose($fh);
            $log = array_slice(explode("\n", rtrim((string)$chunk, "\n")), -15);
        }
    }

    $st['ok']  = true;
    $st['log'] = $log;
    zdc_out($st);
}

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

    @set_time_limit(0);
    @ignore_user_abort(true);

    $snap_base = $mountpoint . '/.zfs/snapshot/' . $snapshot;
    $live_base = $mountpoint;

    $src = zdc_safe_path($snap_base, $src_rel);
    if ($src === false || !file_exists($src)) {
        zdc_out(array('ok' => false, 'error' => 'Source not found in snapshot: ' . $src_rel));
    }

    if ($dst_rel === '') {

        $dst = zdc_safe_path($live_base, $src_rel);
        if ($dst === false) {
            zdc_out(array('ok' => false, 'error' => 'Invalid source path'));
        }
    } elseif (isset($dst_rel[0]) && $dst_rel[0] === '/') {

        $dst = zdc_safe_abs_path($dst_rel);
        if ($dst === false) {
            zdc_out(array('ok' => false, 'error' => 'Invalid destination path'));
        }
    } else {

        $dst = zdc_safe_path($live_base, $dst_rel);
        if ($dst === false) {
            zdc_out(array('ok' => false, 'error' => 'Invalid destination path'));
        }
    }

    if (strpos($dst, '/.zfs/') !== false) {
        zdc_out(array('ok' => false, 'error' => 'Destination is inside a snapshot (read-only): ' . $dst));
    }

    if (is_dir($src) && (rtrim($dst, '/') === rtrim($src, '/')
        || strpos(rtrim($dst, '/') . '/', rtrim($src, '/') . '/') === 0)) {
        zdc_out(array('ok' => false, 'error' => 'Destination is inside the source folder'));
    }

    if ($dst_rel === '') {

        if (is_dir($src)) {
            if (!is_dir($dst) && !mkdir($dst, 0755, true)) {
                zdc_out(array('ok' => false, 'error' => 'Cannot create directory: ' . $dst));
            }
            $cmd   = 'rsync ' . ZDC_RSYNC . ' ' . escapeshellarg($src . '/') . ' ' . escapeshellarg($dst . '/') . ' 2>&1';
            $check = $dst;
        } else {
            $dst_dir = dirname($dst);
            if (!is_dir($dst_dir) && !mkdir($dst_dir, 0755, true)) {
                zdc_out(array('ok' => false, 'error' => 'Cannot create directory: ' . $dst_dir));
            }
            $cmd   = 'cp -a ' . escapeshellarg($src) . ' ' . escapeshellarg($dst) . ' 2>&1';
            $check = $dst;
        }
    } else {

        if (!is_dir($dst) && !mkdir($dst, 0755, true)) {
            zdc_out(array('ok' => false, 'error' => 'Cannot create directory: ' . $dst));
        }
        if (is_dir($src)) {

            $dst_named = $dst . '/' . basename($src);
            if (!is_dir($dst_named) && !mkdir($dst_named, 0755, true)) {
                zdc_out(array('ok' => false, 'error' => 'Cannot create directory: ' . $dst_named));
            }
            $cmd   = 'rsync ' . ZDC_RSYNC . ' ' . escapeshellarg($src . '/') . ' ' . escapeshellarg($dst_named . '/') . ' 2>&1';
            $check = $dst_named;
        } else {
            $cmd   = 'cp -a ' . escapeshellarg($src) . ' ' . escapeshellarg($dst . '/') . ' 2>&1';
            $check = $dst . '/' . basename($src);
        }
    }
    $output = shell_exec($cmd);

    if (!file_exists($check)) {
        zdc_out(array('ok' => false, 'error' => 'Restore failed: ' . trim($output)));
    }
    zdc_out(array('ok' => true, 'dst' => $check, 'message' => 'Restored successfully.'));
}

zdc_out(array('ok' => false, 'error' => 'Unknown action: ' . htmlspecialchars($action)));
