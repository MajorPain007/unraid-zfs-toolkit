<?php

require_once __DIR__ . '/zdc_php_common.php';

zdc_require_post();

$dataset = trim(zdc_post('dataset'));
$from    = trim(zdc_post('from'));
$to      = trim(zdc_post('to'));
$limit   = (int)zdc_post('limit', '2000');
if ($limit < 1 || $limit > 20000) $limit = 2000;

if (!zdc_valid_dataset($dataset)) zdc_fail('Invalid dataset name');

function zdc_snap_creation($snap) {
    list($out, $rc) = zdc_run('zfs list -Hp -t snapshot -o creation ' . escapeshellarg($snap));
    if ($rc !== 0 || !isset($out[0]) || !is_numeric(trim($out[0]))) return null;
    return (int)trim($out[0]);
}

$snapRe = '#^[A-Za-z0-9][A-Za-z0-9_.:-]*$#';
if (!preg_match($snapRe, $from)) zdc_fail('Invalid "from" snapshot');
if ($to !== '' && !preg_match($snapRe, $to)) zdc_fail('Invalid "to" snapshot');

$fromFull = $dataset . '@' . $from;
$toArg    = ($to === '') ? $dataset : ($dataset . '@' . $to);

$reversed = false;
if ($to !== '') {
    $cFrom = zdc_snap_creation($fromFull);
    $cTo   = zdc_snap_creation($toArg);
    if ($cFrom !== null && $cTo !== null && $cFrom > $cTo) {
        $tmp = $fromFull; $fromFull = $toArg; $toArg = $tmp;
        $reversed = true;
    }
}

@set_time_limit(300);

list($out, $rc) = zdc_run('zfs diff -FH ' . escapeshellarg($fromFull) . ' ' . escapeshellarg($toArg));

if ($rc !== 0) {
    $msg = trim(implode(' ', $out));
    if (stripos($msg, 'permission') !== false || stripos($msg, 'not allowed') !== false) {
        $msg .= ' (zfs diff requires the dataset to be mounted)';
    } elseif (stripos($msg, 'earlier snapshot') !== false) {
        $msg .= ' (both snapshots must belong to this dataset; a snapshot taken'
              . ' on a different dataset or after a rollback cannot be compared)';
    }
    zdc_fail($msg !== '' ? $msg : 'zfs diff failed');
}

$mountpoint = '';
list($mp, ) = zdc_run('zfs list -H -o mountpoint ' . escapeshellarg($dataset));
if (isset($mp[0])) $mountpoint = rtrim(trim($mp[0]), '/');

$typeNames = array(
    'F' => 'file', '/' => 'dir', '@' => 'symlink', '|' => 'fifo',
    'B' => 'block', 'C' => 'char', '=' => 'socket',
);
$changeNames = array(
    '-' => 'removed', '+' => 'added', 'M' => 'modified', 'R' => 'renamed',
);

// zfs diff writes every byte outside printable ASCII, and the space, as a
// backslash and four octal digits: "My File" comes out as My\0040File and an
// umlaut as two escapes. Turn them back into the name.
function zdc_diff_unescape($s) {
    return preg_replace_callback('/\\\\([0-7]{4})/', function ($m) {
        return chr(octdec($m[1]) & 0xff);
    }, $s);
}

$entries   = array();
$truncated = false;
$counts    = array('added' => 0, 'removed' => 0, 'modified' => 0, 'renamed' => 0);

foreach ($out as $line) {
    if ($line === '') continue;
    if (count($entries) >= $limit) { $truncated = true; break; }

    $f = explode("\t", $line);
    if (count($f) < 3) continue;

    $change = $f[0];
    $ftype  = $f[1];
    $path   = zdc_diff_unescape($f[2]);
    $newpath = isset($f[3]) ? zdc_diff_unescape($f[3]) : '';

    $kind = isset($changeNames[$change]) ? $changeNames[$change] : $change;
    if (isset($counts[$kind])) $counts[$kind]++;

    $rel = $path;
    if ($mountpoint !== '' && strpos($path, $mountpoint . '/') === 0) {
        $rel = substr($path, strlen($mountpoint));
    }
    $relNew = $newpath;
    if ($newpath !== '' && $mountpoint !== '' && strpos($newpath, $mountpoint . '/') === 0) {
        $relNew = substr($newpath, strlen($mountpoint));
    }

    $entries[] = array(
        'change'   => $change,
        'kind'     => $kind,
        'type'     => isset($typeNames[$ftype]) ? $typeNames[$ftype] : $ftype,
        'path'     => $rel,
        'new_path' => $relNew,
    );
}

zdc_out(array(
    'ok'        => true,
    'from'      => $fromFull,
    'to'        => $toArg,
    'reversed'  => $reversed,
    'entries'   => $entries,
    'counts'    => $counts,
    'total'     => count($out),
    'truncated' => $truncated,
));
