<?php
$ztToken = isset($var['csrf_token']) ? $var['csrf_token'] : '';
$ztBase = '/plugins/zfs.toolkit/assets';
?>
<link rel="stylesheet" href="<?php autov("$ztBase/app.css"); ?>">
<div id="zt-app" class="zt" data-csrf="<?=htmlspecialchars($ztToken, ENT_QUOTES)?>"
     data-api="/plugins/zfs.toolkit/include/api.php">
  <div class="zt-boot">Loading&hellip;</div>
</div>
<script src="<?php autov("$ztBase/app.js"); ?>"></script>
