<?php

require_once __DIR__ . '/zdc_php_common.php';

zdc_require_post();
$action = trim(zdc_post('action'));

define('ZDC_HOLD_TAG', 'zdc-keep');

if ($action === 'list') {
    $dataset  = trim(zdc_post('dataset'));
    $onlyAuto = zdc_post('only_auto', '0') === '1';

    if ($dataset !== '' && !zdc_valid_dataset($dataset)) zdc_fail('Invalid dataset name');

    $scope = $dataset !== '' ? escapeshellarg($dataset) : '';
    list($dsLines, ) = zdc_run('zfs list -Hp -o name,used,usedbysnapshots,available -r ' . $scope);
    $datasets = array();
    foreach ($dsLines as $line) {
        $f = explode("\t", $line);
        if (count($f) < 4) continue;
        $datasets[] = array(
            'name'            => $f[0],
            'used'            => (float)$f[1],
            'usedbysnapshots' => (float)$f[2],
            'available'       => (float)$f[3],
            'used_h'          => zdc_fmt_bytes($f[1]),
            'snapused_h'      => zdc_fmt_bytes($f[2]),
            'avail_h'         => zdc_fmt_bytes($f[3]),
        );
    }

    list($lines, $rc) = zdc_run(
        'zfs list -Hp -t snapshot -o name,used,referenced,creation,userrefs -s creation -r ' . $scope);
    if ($rc !== 0 && !$lines) zdc_fail('zfs list failed');

    $snaps = array();
    $totalUsed = 0;
    foreach ($lines as $line) {
        $f = explode("\t", $line);
        if (count($f) < 5) continue;
        $at   = strpos($f[0], '@');
        $ds   = substr($f[0], 0, $at);
        $sn   = substr($f[0], $at + 1);
        $auto = (strpos($sn, 'auto-') === 0);
        if ($onlyAuto && !$auto) continue;

        $type = '';
        if ($auto) {
            $rest = substr($sn, 5);
            $dash = strpos($rest, '-');
            $type = ($dash === false) ? '' : substr($rest, 0, $dash);
        }

        $used = (float)$f[1];
        $totalUsed += $used;
        $snaps[] = array(
            'name'      => $f[0],
            'dataset'   => $ds,
            'snapshot'  => $sn,
            'type'      => $type,
            'managed'   => $auto,
            'used'      => $used,
            'used_h'    => zdc_fmt_bytes($f[1]),
            'refer_h'   => zdc_fmt_bytes($f[2]),
            'creation'  => (int)$f[3],
            'created_h' => date('Y-m-d H:i', (int)$f[3]),
            'held'      => ($f[4] !== '-' && (int)$f[4] > 0),
        );
    }

    usort($snaps, function($a, $b) {
        if ($a['used'] === $b['used']) return $b['creation'] - $a['creation'];
        return ($b['used'] < $a['used']) ? -1 : 1;
    });

    zdc_out(array(
        'ok'         => true,
        'snapshots'  => $snaps,
        'datasets'   => $datasets,
        'count'      => count($snaps),
        'total_used' => $totalUsed,
        'total_h'    => zdc_fmt_bytes($totalUsed),
    ));
}

if ($action === 'destroy') {
    $names = isset($_POST['snapshots']) ? $_POST['snapshots'] : array();
    if (!is_array($names)) $names = array($names);
    if (!$names) zdc_fail('No snapshots selected');
    if (count($names) > 500) zdc_fail('Refusing to delete more than 500 snapshots in one request');

    $force   = zdc_post('force', '0') === '1';
    $results = array();
    $okCount = 0;

    foreach ($names as $name) {
        $name = trim($name);
        if (!zdc_valid_snapshot($name)) {
            $results[] = array('name' => $name, 'ok' => false, 'error' => 'Invalid snapshot name');
            continue;
        }

        if (!$force) {
            list($h, ) = zdc_run('zfs list -H -o userrefs ' . escapeshellarg($name));
            if (isset($h[0]) && trim($h[0]) !== '-' && (int)$h[0] > 0) {
                $results[] = array('name' => $name, 'ok' => false,
                                   'error' => 'Snapshot is held - release the hold first');
                continue;
            }
        }

        list($out, $rc) = zdc_run('zfs destroy ' . escapeshellarg($name));
        if ($rc === 0) {
            $results[] = array('name' => $name, 'ok' => true);
            $okCount++;
        } else {
            $results[] = array('name' => $name, 'ok' => false, 'error' => trim(implode(' ', $out)));
        }
    }

    zdc_out(array('ok' => true, 'deleted' => $okCount,
                  'failed' => count($results) - $okCount, 'results' => $results));
}

if ($action === 'hold' || $action === 'release') {
    $names = isset($_POST['snapshots']) ? $_POST['snapshots'] : array();
    if (!is_array($names)) $names = array($names);
    if (!$names) {
        $single = trim(zdc_post('snapshot'));
        if ($single !== '') $names = array($single);
    }
    if (!$names) zdc_fail('No snapshot given');
    if (count($names) > 500) zdc_fail('Refusing to change more than 500 holds in one request');

    $verb    = ($action === 'hold') ? 'hold' : 'release';
    $results = array();
    $okCount = 0;

    foreach ($names as $name) {
        $name = trim($name);
        if (!zdc_valid_snapshot($name)) {
            $results[] = array('name' => $name, 'ok' => false, 'error' => 'Invalid snapshot name');
            continue;
        }

        list($out, $rc) = zdc_run('zfs ' . $verb . ' ' . escapeshellarg(ZDC_HOLD_TAG) . ' ' . escapeshellarg($name));
        $msg = trim(implode(' ', $out));

        if ($rc === 0) {
            $results[] = array('name' => $name, 'ok' => true);
            $okCount++;
        } elseif ($verb === 'release' && stripos($msg, 'no such tag') !== false) {
            $results[] = array('name' => $name, 'ok' => true, 'note' => 'no plugin hold was set');
            $okCount++;
        } elseif ($verb === 'hold' && stripos($msg, 'tag already exists') !== false) {
            $results[] = array('name' => $name, 'ok' => true, 'note' => 'already held');
            $okCount++;
        } else {
            $results[] = array('name' => $name, 'ok' => false,
                               'error' => $msg !== '' ? $msg : ('zfs ' . $verb . ' failed'));
        }
    }

    zdc_out(array('ok' => true, 'held' => ($verb === 'hold'), 'changed' => $okCount,
                  'failed' => count($results) - $okCount, 'results' => $results));
}

if ($action === 'rollback') {
    $name    = trim(zdc_post('snapshot'));
    $confirm = trim(zdc_post('confirm'));

    if (!zdc_valid_snapshot($name)) zdc_fail('Invalid snapshot name');

    if ($confirm !== $name) {
        zdc_fail('Confirmation does not match the snapshot name');
    }

    $dataset = substr($name, 0, strpos($name, '@'));

    list($newer, ) = zdc_run(
        'zfs list -H -t snapshot -o name -s creation ' . escapeshellarg($dataset));
    $lost  = array();
    $found = false;
    foreach ($newer as $s) {
        $s = trim($s);
        if ($s === $name) { $found = true; continue; }
        if ($found) $lost[] = $s;
    }
    if (!$found) zdc_fail('Snapshot not found on dataset ' . $dataset);

    list($out, $rc) = zdc_run('zfs rollback -r ' . escapeshellarg($name));
    if ($rc !== 0) {
        zdc_fail('Rollback failed: ' . trim(implode(' ', $out)));
    }

    zdc_out(array(
        'ok'        => true,
        'dataset'   => $dataset,
        'destroyed' => $lost,
        'message'   => 'Rolled back ' . $dataset . ' to ' . $name
                     . (count($lost) ? ' (' . count($lost) . ' newer snapshot(s) destroyed)' : ''),
    ));
}

zdc_fail('Unknown action: ' . htmlspecialchars($action));
