<?php

if (!defined('ZDC_COMMON_LOADED')) {
    define('ZDC_COMMON_LOADED', true);

    define('ZDC_NAME',       'zfs.toolkit');
    define('ZDC_PLUGIN_DIR', '/usr/local/emhttp/plugins/' . ZDC_NAME);
    define('ZDC_CONFIG_DIR', '/boot/config/plugins/' . ZDC_NAME);
    define('ZDC_TMP_DIR',    '/tmp/' . ZDC_NAME);
    define('ZDC_RSYNC',      '-a -H -A -X --numeric-ids');

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

    function zdc_out($data) {
        echo json_encode($data);
        exit;
    }

    function zdc_fail($msg) {
        zdc_out(array('ok' => false, 'error' => $msg));
    }

    function zdc_require_post() {
        if ($_SERVER['REQUEST_METHOD'] !== 'POST') zdc_fail('POST required');
    }

    function zdc_post($key, $default = '') {
        return isset($_POST[$key]) ? $_POST[$key] : $default;
    }

    function zdc_valid_dataset($name) {
        return (bool)preg_match('#^[A-Za-z0-9][A-Za-z0-9_.:-]*(/[A-Za-z0-9_.:-]+)*$#', $name);
    }

    function zdc_valid_snapshot($snap) {
        $at = strpos($snap, '@');
        if ($at === false) return false;
        $ds = substr($snap, 0, $at);
        $sn = substr($snap, $at + 1);
        return zdc_valid_dataset($ds) && preg_match('#^[A-Za-z0-9][A-Za-z0-9_.:-]*$#', $sn);
    }

    function zdc_run($cmd) {
        $out = array();
        $rc  = 0;
        exec($cmd . ' 2>&1', $out, $rc);
        return array($out, $rc);
    }

    function zdc_fmt_bytes($bytes) {
        $b = (float)$bytes;
        if ($b < 0) return '-';
        $units = array('B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB');
        $i = 0;
        while ($b >= 1024 && $i < count($units) - 1) { $b /= 1024; $i++; }
        return (($i === 0) ? (int)$b : round($b, ($b < 10 ? 2 : 1))) . ' ' . $units[$i];
    }

    function zdc_load_settings() {
        $file = ZDC_CONFIG_DIR . '/settings.cfg';
        $cfg  = array();
        if (!file_exists($file)) return $cfg;
        foreach (file($file) as $line) {
            $line = trim($line);
            if ($line === '' || $line[0] === '#') continue;
            $parts = explode('=', $line, 2);
            if (count($parts) === 2) $cfg[trim($parts[0])] = trim($parts[1]);
        }
        return $cfg;
    }

    function zdc_tail($path, $count) {
        if (!file_exists($path)) return array();
        $fh = @fopen($path, 'rb');
        if (!$fh) return array();
        $size = filesize($path);
        $pos  = $size;
        $data = '';
        while ($pos > 0 && substr_count($data, "\n") <= $count) {
            $read = ($pos >= 8192) ? 8192 : $pos;
            $pos -= $read;
            fseek($fh, $pos);
            $data = fread($fh, $read) . $data;
        }
        fclose($fh);
        if ($data === '') return array();
        return array_slice(explode("\n", rtrim($data, "\n")), -$count);
    }
}
