<?php

$NAME       = 'zfs.dataset.converter';
$CONFIG_DIR = '/boot/config/plugins/' . $NAME;
$TMP_DIR    = '/tmp/' . $NAME;
$PLUGIN_DIR = '/usr/local/emhttp/plugins/' . $NAME;

$download = (isset($_REQUEST['download']) && $_REQUEST['download'] === '1');

function run($cmd) {
    $out = array();
    exec($cmd . ' 2>&1', $out);
    return implode("\n", $out);
}

function section($title, $body) {
    $line = str_repeat('=', 72);
    return $line . "\n== " . $title . "\n" . $line . "\n" . rtrim((string)$body) . "\n\n";
}

function is_secret($path) {
    $base = basename($path);
    if (preg_match('/(^id_|\.key$|\.pem$|_rsa$|_ed25519$|_ecdsa$)/i', $base)) return true;
    if (substr($base, -4) === '.pub') return false;
    return false;
}

function tail_file($path, $lines) {
    if (!file_exists($path)) return '(not present)';
    $out = array();
    exec('tail -n ' . (int)$lines . ' ' . escapeshellarg($path) . ' 2>/dev/null', $out);
    return implode("\n", $out);
}

$version = '(unknown)';
$plg = '/boot/config/plugins/' . $NAME . '.plg';
if (file_exists($plg)) {
    if (preg_match('/<!ENTITY\s+version\s+"([^"]+)"/', file_get_contents($plg), $m)) $version = $m[1];
}

$report  = "ZFS Dataset Converter - diagnostics\n";
$report .= "Generated: " . date('Y-m-d H:i:s T') . "\n";
$report .= "Plugin version: " . $version . "\n\n";

$report .= section('System', implode("\n", array(
    'Unraid: ' . trim(@file_get_contents('/etc/unraid-version')),
    'Kernel: ' . run('uname -r'),
    'PHP:    ' . PHP_VERSION,
    'bash:   ' . run('bash --version | head -1'),
    'zfs:    ' . run('zfs version 2>/dev/null | head -2'),
    'tools:  pv=' . (trim(run('command -v pv')) ?: 'no')
             . ' mbuffer=' . (trim(run('command -v mbuffer')) ?: 'no')
             . ' flock=' . (trim(run('command -v flock')) ?: 'no'),
)));

$report .= section('zpool list',   run('zpool list'));
$report .= section('zpool status', run('zpool status'));
$report .= section('zfs list',     run('zfs list -o name,used,avail,refer,usedbysnapshots,mountpoint'));
$report .= section('Snapshot count per dataset',
    run("zfs list -H -t snapshot -o name | awk -F@ '{c[\$1]++} END {for (d in c) print c[d], d}' | sort -rn | head -50"));
$report .= section('Auto snapshots (newest 40)',
    run('zfs list -H -t snapshot -o name,used,creation -s creation | grep "@auto-" | tail -40'));
$report .= section('Holds', run('zfs holds $(zfs list -H -t snapshot -o name 2>/dev/null | head -200) 2>/dev/null | head -50'));

$report .= section('Live crontab (plugin entries)',
    run('crontab -l 2>/dev/null | grep -E "run_auto|snapshot_manager|zfs_send" || echo "(none)"'));
$report .= section('/etc/cron.d files',
    run('ls -la /etc/cron.d/ 2>/dev/null; echo; for f in /etc/cron.d/' . $NAME . '*; do [ -f "$f" ] && { echo "--- $f"; cat "$f"; }; done'));

foreach (array('settings.cfg', 'snap_datasets.json', 'send_jobs.json') as $f) {
    $path = $CONFIG_DIR . '/' . $f;
    $report .= section('config/' . $f,
        file_exists($path) ? file_get_contents($path) : '(not present)');
}

$report .= section('snapshots.log (last 200)', tail_file($TMP_DIR . '/snapshots.log', 200));
$report .= section('send.log (last 200)',      tail_file($TMP_DIR . '/send.log', 200));
$report .= section('snapshot_status.json',     tail_file($TMP_DIR . '/snapshot_status.json', 5));
$report .= section('send_status.json',         tail_file($TMP_DIR . '/send_status.json', 5));
$report .= section('setup_cron.log',           tail_file($TMP_DIR . '/setup_cron.log', 40));
$report .= section('setup_snapshots.log',      tail_file($TMP_DIR . '/setup_snapshots.log', 40));

$conv = glob($TMP_DIR . '/conversion_*.log') ?: array();
usort($conv, function($a, $b) { return filemtime($b) - filemtime($a); });
foreach (array_slice($conv, 0, 2) as $c) {
    $report .= section('conversion: ' . basename($c), tail_file($c, 300));
}

$report .= section('Plugin files', run('ls -la ' . escapeshellarg($PLUGIN_DIR . '/scripts') . ' 2>/dev/null'));

if (!$download) {
    while (ob_get_level() > 0) ob_end_clean();
    header('Content-Type: text/plain; charset=utf-8');
    header('Cache-Control: no-cache, no-store');
    echo $report;
    exit;
}

$stamp   = date('Ymd-His');
$workDir = $TMP_DIR . '/diag-' . $stamp;
@mkdir($workDir, 0755, true);
file_put_contents($workDir . '/report.txt', $report);

@mkdir($workDir . '/config', 0755, true);
foreach (glob($CONFIG_DIR . '/*') ?: array() as $f) {
    if (!is_file($f) || is_secret($f)) continue;
    if (substr($f, -4) === '.txz') continue;
    if (filesize($f) > 2 * 1024 * 1024) continue;
    @copy($f, $workDir . '/config/' . basename($f));
}

$tar = $TMP_DIR . '/zdc-diagnostics-' . $stamp . '.tar.gz';
exec('tar -czf ' . escapeshellarg($tar) . ' -C ' . escapeshellarg($TMP_DIR)
     . ' ' . escapeshellarg('diag-' . $stamp) . ' 2>&1');
exec('rm -rf ' . escapeshellarg($workDir));

$old = glob($TMP_DIR . '/zdc-diagnostics-*.tar.gz') ?: array();
usort($old, function($a, $b) { return filemtime($b) - filemtime($a); });
foreach (array_slice($old, 3) as $f) @unlink($f);

if (!file_exists($tar)) {
    while (ob_get_level() > 0) ob_end_clean();
    header('Content-Type: text/plain; charset=utf-8');
    echo "Failed to create the diagnostics archive.\n\n" . $report;
    exit;
}

while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/gzip');
header('Content-Disposition: attachment; filename="' . basename($tar) . '"');
header('Content-Length: ' . filesize($tar));
header('Cache-Control: no-cache, no-store');
readfile($tar);
exit;
