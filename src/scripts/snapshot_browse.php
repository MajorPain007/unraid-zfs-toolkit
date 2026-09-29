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

define('ZDC_TMP',    '/tmp/zfs.toolkit');
define('ZDC_PLUGIN', '/usr/local/emhttp/plugins/zfs.toolkit');

// A file name that is not valid UTF-8 - common for names written by old
// Windows clients over SMB - made json_encode return false, and the whole
// folder listing arrived empty. Such a name now shows with a replacement mark.
function zdc_out($data) {
    echo json_encode($data, JSON_INVALID_UTF8_SUBSTITUTE);
    exit;
}

// The dataset rule of zdc_php_common.php, repeated here because this endpoint
// stands on its own. Letters, digits, space and _ . : - as ZFS takes them.
function zdc_browse_dataset_ok($name) {
    return (bool)preg_match('#^[A-Za-z0-9][A-Za-z0-9_.:-]*(/[A-Za-z0-9_.: -]+)*$#', $name);
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

if ($action === 'list_snapshots') {
    $dataset = isset($_POST['dataset']) ? $_POST['dataset'] : (isset($_GET['dataset']) ? $_GET['dataset'] : '');
    if (!zdc_browse_dataset_ok($dataset)) {
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

    if (!zdc_browse_dataset_ok($dataset)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid dataset name'));
    }

    if ($snapshot === '' || preg_match('/[^a-zA-Z0-9_.:\-]/', $snapshot)) {
        zdc_out(array('ok' => false, 'error' => 'Invalid snapshot name: ' . htmlspecialchars($snapshot)));
    }

    $mountpoint = zdc_get_mountpoint($dataset);
    if ($mountpoint === '') {
        zdc_out(array('ok' => false, 'error' => 'Dataset not found or not mounted: ' . $dataset));
    }

    // A snapshot that was just destroyed can still leave a stale directory
    // entry behind, which then fails to mount. Check with zfs, not the path.
    $exists = array();
    exec('zfs list -Hp -o name,defer_destroy,userrefs -t snapshot '
         . escapeshellarg($dataset . '@' . $snapshot) . ' 2>&1', $exists, $rc_exists);
    if ($rc_exists !== 0) {
        zdc_out(array(
            'ok'    => false,
            'error' => 'Snapshot ' . $dataset . '@' . $snapshot . ' no longer exists. '
                     . 'It was probably destroyed since the list was loaded - reload the snapshot list.',
        ));
    }
    // defer_destroy=on means "destroy once the last hold or clone goes away".
    // Such a snapshot is still readable, so it is worth stating rather than
    // leaving it as a suspicion.
    $snap_note = '';
    if (isset($exists[0])) {
        $f = explode("\t", $exists[0]);
        if (isset($f[1]) && $f[1] === 'on') {
            $snap_note = ' This snapshot is marked for deferred destruction'
                       . (isset($f[2]) && $f[2] !== '0' ? ' and held (' . $f[2] . ' hold(s))' : '')
                       . ', which does not by itself prevent reading it.';
        }
    }

    $snap_base = $mountpoint . '/.zfs/snapshot/' . $snapshot;
    if (!is_dir($snap_base)) {
        // Accessing the path is what makes ZFS mount the snapshot; a plain
        // stat from PHP does not always trigger it.
        exec('ls -A -- ' . escapeshellarg($snap_base) . ' >/dev/null 2>&1');
        clearstatcache(true, $snap_base);
    }
    // .zfs is there whether snapdir is hidden or visible - hidden only keeps it
    // out of directory listings - so a snapshot that cannot be reached here is
    // not a snapdir question. The dataset is usually not mounted.
    if (!is_dir($snap_base)) {
        zdc_out(array(
            'ok'    => false,
            'error' => 'Snapshot not accessible at ' . $snap_base
                     . '. Is ' . $dataset . ' mounted at ' . $mountpoint . '?',
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
        // scandir returns false for any failure. Saying "permission denied"
        // was a guess; ask the system what actually went wrong. Running ls in
        // a fresh process also forces the snapshot automount, which is the
        // usual reason a directory inside .zfs cannot be read on first touch.
        $probe = array();
        exec('ls -A -- ' . escapeshellarg($full_path) . ' 2>&1', $probe, $rc_probe);
        clearstatcache(true, $full_path);
        $items = @scandir($full_path);

        if ($items === false) {
            $why = trim(implode(' ', $probe));
            if ($why === '') $why = 'no error reported by ls';
            $user = function_exists('posix_geteuid')
                ? ('uid ' . posix_geteuid() . (posix_geteuid() === 0 ? ' (root)' : ' (not root)'))
                : 'uid unknown';
            zdc_out(array(
                'ok'    => false,
                'error' => 'Cannot read ' . $full_path . ' - ' . $why
                         . '. Running as ' . $user
                         . (is_readable($full_path) ? '' : '; the path is not readable')
                         . '. Snapshots are read-only, so this is usually the snapshot failing to'
                         . ' mount rather than a rights problem.' . $snap_note,
            ));
        }
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

if ($action === 'list_dirs') {
    $root = '/mnt';
    $path = isset($_POST['path']) ? $_POST['path'] : $root;

    $p = zdc_safe_abs_path($path);
    if ($p === false) $p = $root;
    if ($p !== $root && strpos($p . '/', $root . '/') !== 0) $p = $root;

    if (!is_dir($p)) {
        zdc_out(array('ok' => false, 'error' => 'Not a directory: ' . $p));
    }

    $items = @scandir($p);
    if ($items === false) {
        zdc_out(array('ok' => false, 'error' => 'Cannot read ' . $p));
    }

    $dirs = array();
    foreach ($items as $item) {
        if ($item === '.' || $item === '..') continue;
        if ($item === '.zfs') continue;
        if ($item[0] === '.') continue;
        $full = $p . '/' . $item;
        if (!is_dir($full)) continue;
        $dirs[] = array('name' => $item, 'path' => $full);
    }
    usort($dirs, function($a, $b) { return strcasecmp($a['name'], $b['name']); });

    $crumbs = array(array('label' => '/mnt', 'path' => $root));
    $rest = trim(substr($p, strlen($root)), '/');
    if ($rest !== '') {
        $cum = $root;
        foreach (explode('/', $rest) as $part) {
            $cum .= '/' . $part;
            $crumbs[] = array('label' => $part, 'path' => $cum);
        }
    }

    zdc_out(array(
        'ok'       => true,
        'path'     => $p,
        'crumbs'   => $crumbs,
        'dirs'     => $dirs,
        'writable' => is_writable($p),
    ));
}

if ($action === 'restore_start') {
    $dataset  = isset($_POST['dataset'])  ? $_POST['dataset']  : '';
    $snapshot = isset($_POST['snapshot']) ? $_POST['snapshot'] : '';
    $dst_rel  = isset($_POST['dst_path']) ? $_POST['dst_path'] : '';
    $items    = isset($_POST['items'])    ? $_POST['items']    : array();

    if (!is_array($items)) $items = array($items);
    if (!$items) zdc_out(array('ok' => false, 'error' => 'Nothing selected'));
    if (count($items) > 1000) zdc_out(array('ok' => false, 'error' => 'Too many items (max 1000)'));

    if (!zdc_browse_dataset_ok($dataset)) {
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
        // The job file separates its fields with tabs.
        if (strpbrk($dst_rel, "\t\n\r\0") !== false) {
            zdc_out(array('ok' => false, 'error' => 'Unsupported character in the destination path'));
        }
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

zdc_out(array('ok' => false, 'error' => 'Unknown action: ' . htmlspecialchars($action)));
