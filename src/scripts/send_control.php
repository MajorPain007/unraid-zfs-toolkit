<?php

require_once __DIR__ . '/zdc_php_common.php';

zdc_require_post();

$action   = trim(zdc_post('action'));
$jobsFile = ZDC_CONFIG_DIR . '/send_jobs.json';
$worker   = ZDC_PLUGIN_DIR . '/scripts/zfs_send.sh';
$logFile  = ZDC_TMP_DIR . '/send.log';

if ($action === 'get_jobs') {
    $jobs = array();
    if (file_exists($jobsFile)) {
        $d = json_decode(file_get_contents($jobsFile), true);
        if (is_array($d) && isset($d['jobs']) && is_array($d['jobs'])) $jobs = $d['jobs'];
    }
    zdc_out(array('ok' => true, 'jobs' => $jobs, 'has_pv' => (trim((string)shell_exec('command -v pv')) !== '')));
}

if ($action === 'save_jobs') {
    $raw = zdc_post('jobs_json');
    if ($raw === '') zdc_fail('No job data');

    $data = json_decode($raw, true);
    if ($data === null) zdc_fail('Invalid JSON: ' . json_last_error_msg());
    if (!isset($data['jobs']) || !is_array($data['jobs'])) $data = array('jobs' => array());

    $seen = array();
    foreach ($data['jobs'] as $i => $j) {
        $label = isset($j['name']) && $j['name'] !== '' ? $j['name'] : ('#' . ($i + 1));

        if (empty($j['source']) || !zdc_valid_dataset($j['source'])) {
            zdc_fail('Job ' . $label . ': invalid source dataset');
        }
        if (empty($j['dest']) || !zdc_valid_dataset($j['dest'])) {
            zdc_fail('Job ' . $label . ': invalid destination dataset');
        }

        $transport = (isset($j['transport']) && $j['transport'] === 'ssh') ? 'ssh' : 'local';
        if ($transport === 'local') {
            if ($j['source'] === $j['dest']) {
                zdc_fail('Job ' . $label . ': source and destination are identical');
            }
            if (strpos($j['dest'] . '/', $j['source'] . '/') === 0) {
                zdc_fail('Job ' . $label . ': destination lies inside the source dataset');
            }
        } else {
            if (empty($j['ssh_host'])
                || !preg_match('/^[A-Za-z0-9._@-]+$/', $j['ssh_host'])) {
                zdc_fail('Job ' . $label . ': invalid SSH host (expected user@host)');
            }
            if (!empty($j['ssh_key']) && !preg_match('#^/[A-Za-z0-9._/-]+$#', $j['ssh_key'])) {
                zdc_fail('Job ' . $label . ': invalid SSH key path');
            }
        }

        $id = isset($j['id']) ? $j['id'] : '';
        if ($id === '' || !preg_match('/^[A-Za-z0-9_-]+$/', $id)) {
            $id = 'job' . ($i + 1);
            $data['jobs'][$i]['id'] = $id;
        }
        if (isset($seen[$id])) zdc_fail('Duplicate job id: ' . $id);
        $seen[$id] = true;
    }

    if (!is_dir(ZDC_CONFIG_DIR)) @mkdir(ZDC_CONFIG_DIR, 0755, true);
    $written = file_put_contents(
        $jobsFile,
        json_encode($data, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE));
    if ($written === false) zdc_fail('Cannot write ' . $jobsFile);

    $setup = ZDC_PLUGIN_DIR . '/scripts/setup_send.sh';
    $warn  = '';
    if (file_exists($setup)) {
        list($out, $rc) = zdc_run('/bin/bash ' . escapeshellarg($setup));
        if ($rc !== 0) $warn = trim(implode(' ', $out));
    }

    zdc_out(array('ok' => true, 'warning' => $warn));
}

if ($action === 'status') {
    $st = array();
    $statusFile = ZDC_TMP_DIR . '/send_status.json';
    if (file_exists($statusFile)) {
        $d = json_decode(file_get_contents($statusFile), true);
        if (is_array($d)) $st = $d;
    }

    $running = trim((string)shell_exec('pgrep -f "zfs_send\.sh" 2>/dev/null | head -1')) !== '';

    $cronFile = '/etc/cron.d/zfs.dataset.converter-send';
    $entry = '';
    if (is_readable($cronFile)) {
        foreach (file($cronFile, FILE_IGNORE_NEW_LINES) as $line) {
            $line = trim($line);
            if ($line === '' || $line[0] === '#') continue;
            if (strpos($line, 'zfs_send.sh') !== false) { $entry = $line; break; }
        }
    }
    $live = array();
    exec('crontab -l 2>/dev/null', $live);
    $inLive = false;
    foreach ($live as $l) { if (strpos($l, 'zfs_send.sh') !== false) { $inLive = true; break; } }

    zdc_out(array(
        'ok'           => true,
        'run'          => $st,
        'running'      => $running,
        'entry'        => $entry,
        'cron_healthy' => ($entry !== '' && $inLive),
        'log'          => zdc_tail($logFile, 40),
    ));
}

if ($action === 'run_now' || $action === 'dry_run' || $action === 'test') {
    if (!file_exists($worker)) zdc_fail('zfs_send.sh not found');

    if (trim((string)shell_exec('pgrep -f "zfs_send\.sh" 2>/dev/null | head -1')) !== '') {
        zdc_fail('A replication run is already in progress');
    }

    $job = trim(zdc_post('job'));
    if ($job !== '' && !preg_match('/^[A-Za-z0-9_-]+$/', $job)) zdc_fail('Invalid job id');

    $args = '';
    if ($action === 'test') {
        if ($job === '') zdc_fail('test requires a job id');
        $args = '--test ' . escapeshellarg($job);
    } else {
        if ($job !== '') $args = '--job ' . escapeshellarg($job);
        if ($action === 'dry_run') $args .= ' --dry-run';
    }

    $cmd = 'nohup /bin/bash ' . escapeshellarg($worker) . ' ' . $args
         . ' >> ' . escapeshellarg($logFile) . ' 2>&1 & echo $!';
    $pid = (int)trim((string)shell_exec('/bin/bash -c ' . escapeshellarg($cmd)));

    if ($pid <= 0) zdc_fail('Failed to start the replication worker');
    zdc_out(array('ok' => true, 'pid' => $pid));
}

zdc_fail('Unknown action: ' . htmlspecialchars($action));
