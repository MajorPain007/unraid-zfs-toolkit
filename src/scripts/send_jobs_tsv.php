<?php

// One line per job, fields separated by \x1f (the ASCII unit separator), read
// by zfs_send.sh. Not tabs: bash counts a tab as whitespace in IFS, and runs of
// whitespace count as one separator - so an empty field, like the SSH host of
// a local job or the key of an SSH job that uses the default one, vanished and
// every field after it moved one place left. \x1f is not whitespace, so empty
// fields stay where they are.
define('ZDC_SEP', "\x1f");

$dir  = getenv('ZDC_CONFIG_DIR') ?: '/boot/config/plugins/zfs.toolkit';
$file = $dir . '/send_jobs.json';
if (!file_exists($file)) exit(0);

$data = json_decode(file_get_contents($file), true);
if (!is_array($data) || !isset($data['jobs']) || !is_array($data['jobs'])) exit(0);

function b($v, $default = false) {
    if (!isset($v)) return $default ? '1' : '0';
    if (is_bool($v))   return $v ? '1' : '0';
    if (is_numeric($v)) return ((int)$v) ? '1' : '0';
    return in_array(strtolower((string)$v), array('1', 'true', 'yes', 'on'), true) ? '1' : '0';
}

function s($v) {
    return str_replace(array(ZDC_SEP, "\t", "\n", "\r"), ' ', trim((string)(isset($v) ? $v : '')));
}

foreach ($data['jobs'] as $i => $j) {
    if (!is_array($j)) continue;

    $id = s(isset($j['id']) ? $j['id'] : '');
    if ($id === '' || !preg_match('/^[A-Za-z0-9_-]+$/', $id)) $id = 'job' . ($i + 1);

    $port = isset($j['ssh_port']) ? (int)$j['ssh_port'] : 22;
    if ($port < 1 || $port > 65535) $port = 22;

    echo implode(ZDC_SEP, array(
        $id,
        b(isset($j['enabled']) ? $j['enabled'] : true, true),
        s(isset($j['name']) ? $j['name'] : $id),
        s(isset($j['source']) ? $j['source'] : ''),
        s(isset($j['dest']) ? $j['dest'] : ''),
        b(isset($j['recursive']) ? $j['recursive'] : false),
        (isset($j['transport']) && $j['transport'] === 'ssh') ? 'ssh' : 'local',
        s(isset($j['ssh_host']) ? $j['ssh_host'] : ''),
        $port,
        s(isset($j['ssh_key']) ? $j['ssh_key'] : ''),
        b(isset($j['raw']) ? $j['raw'] : false),
        b(isset($j['compressed']) ? $j['compressed'] : true, true),
        b(isset($j['allow_rollback']) ? $j['allow_rollback'] : false),
        (isset($j['keep_dest']) && (int)$j['keep_dest'] > 0) ? (int)$j['keep_dest'] : 0,
    )), "\n";
}
