<?php
// get_snap_datasets.php - Return saved snapshot dataset config
while (ob_get_level() > 0) ob_end_clean();
header('Content-Type: application/json');
header('Cache-Control: no-cache, no-store');

$file = '/boot/config/plugins/zfs.dataset.converter/snap_datasets.json';
if (file_exists($file)) {
    echo file_get_contents($file);
} else {
    echo json_encode(['datasets' => []]);
}
