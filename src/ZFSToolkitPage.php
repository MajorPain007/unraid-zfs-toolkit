<?php
$configFile = '/boot/config/plugins/zfs.toolkit/settings.cfg';
$settings = [];
if (file_exists($configFile)) {
    foreach (file($configFile) as $line) {
        $line = trim($line);
        if ($line === '' || $line[0] === '#') continue;
        [$key, $val] = array_pad(explode('=', $line, 2), 2, '');
        $settings[trim($key)] = trim($val);
    }
}
function cfg($key, $default = '') {
    global $settings;
    return htmlspecialchars($settings[$key] ?? $default, ENT_QUOTES);
}
function cfgBool($key, $default = 'no') {
    global $settings;
    return in_array(strtolower($settings[$key] ?? $default), ['yes','true','1'], true);
}
?>
<link type="text/css" rel="stylesheet" href="<?= function_exists('autov') ? autov('/webGui/styles/jquery.filetree.css') : '/webGui/styles/jquery.filetree.css' ?>">
<script src="<?= function_exists('autov') ? autov('/webGui/javascript/jquery.filetree.js') : '/webGui/javascript/jquery.filetree.js' ?>" charset="utf-8"></script>

<style>
.zdc-wrap {
  width: 100%;
  /* Follow Unraid's theme where it has an equivalent. The hex is the fallback
     for an Unraid old enough not to expose these variables. */
  --zdc-card:     var(--mild-background-color, #1e2329);
  --zdc-sunken:   var(--table-background-color, #0d1117);
  --zdc-border:   var(--border-color, #3a4049);
  --zdc-border-2: var(--table-border-color, #30363d);
  --zdc-text:     var(--text-color, #c9d1d9);
  --zdc-dim:      var(--alt-text-color, #8b949e);
  --zdc-muted:    var(--disabled-text-color, #6e7681);
  --zdc-accent:   var(--link-text-color, #79c0ff);

  /* Status colours stay ours - green, amber and red have to mean the same on
     every theme - but they need darker shades on a light background. */
  --zdc-ok:       #56d364;
  --zdc-warn:     #e3b341;
  --zdc-err:      #f85149;
  --zdc-blue:     #1f6feb;
}

/* Unraid's --alt-text-color drops to about 3:1 on the white theme. Fine for a
   hint, too weak for a form label, so labels take the normal text colour
   there. The dark theme keeps the softer look. */
.zdc-wrap.zdc-light .zdc-row label.row-label,
.zdc-wrap.zdc-light .zdc-sub { color: var(--zdc-text); }

.zdc-wrap.zdc-light {
  --zdc-ok:       #1a7f37;
  --zdc-warn:     #9a6700;
  --zdc-err:      #cf222e;
  --zdc-blue:     #0969da;
  --zdc-accent:   #0550ae;
  --zdc-sunken:   #f6f8fa;
}

.zdc-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  margin: 12px 0 16px;
  border-bottom: 1px solid var(--zdc-border);
}
.zdc-wrap .zdc-tab {
  appearance: none;
  background: transparent;
  border: 1px solid transparent;
  border-bottom: none;
  border-radius: 5px 5px 0 0;
  box-shadow: none;
  color: var(--zdc-dim);
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: .03em;
  margin-bottom: -1px;
  padding: 8px 16px;
  text-shadow: none;
  text-transform: uppercase;
}
.zdc-wrap .zdc-tab:hover { color: var(--zdc-text); background: rgba(255,255,255,.04); }
.zdc-wrap .zdc-tab.active {
  color: var(--zdc-text);
  background: var(--zdc-card);
  border-color: var(--zdc-border);
  border-bottom: 1px solid var(--zdc-card);
}
.zdc-panel[hidden] { display: none; }

/* One rule instead of a fixed 54/46 split: one column on a phone, two on a
   laptop, three on a wide monitor, without breakpoints to keep in sync. */
/* Grid, so cards sit predictably side by side, with row spans computed in JS
   so a short card does not leave a hole under it until the next row.
   CSS multi-column was tried instead and made the cards overlap and stack
   two-in-one-column when only two were present. */
.zdc-cols {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(330px, 1fr));
  gap: 0 16px;
  align-items: start;
  grid-auto-flow: row dense;
  /* Fine rows keep the span rounding error small. */
  grid-auto-rows: 4px;
}
/* Three at most. Five columns of two-toggle cards read as scattered, and six
   settings cards divide evenly by three. */
@media (min-width: 1120px) {
  .zdc-cols { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
.zdc-cols .zdc-card {
  margin: 0 0 16px;
  /* Until the script has measured, one card per row beats overlapping ones. */
  grid-row: span 40;
}
.zdc-cols.zdc-measured .zdc-card { grid-row: auto; }

.zdc-wide-stack > .zdc-card { margin-bottom: 16px; }
.zdc-col { min-width: 0; }

.zdc-sub {
  color: var(--zdc-dim);
  font-size: 12px;
  font-weight: 600;
  margin: 14px 0 6px;
}
.zdc-sub:first-child { margin-top: 0; }

.zdc-status { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 4px; }
.zdc-pill {
  align-items: center;
  background: var(--zdc-card);
  border: 1px solid var(--zdc-border);
  border-radius: 20px;
  color: var(--zdc-dim);
  display: flex;
  font-size: 12px;
  gap: 7px;
  padding: 5px 13px;
  white-space: nowrap;
}
.zdc-pill b { color: var(--zdc-text); font-weight: 600; }
.zdc-pill.zdc-ok   { border-color: rgba(86,211,100,.4); }
.zdc-pill.zdc-warn { border-color: rgba(227,179,65,.5); }
.zdc-pill.zdc-err  { border-color: rgba(248,81,73,.5); }
.zdc-pill .dot { background: var(--zdc-muted); border-radius: 50%; flex: none; height: 7px; width: 7px; }
.zdc-pill.zdc-ok   .dot { background: var(--zdc-ok); }
.zdc-pill.zdc-warn .dot { background: var(--zdc-warn); }
.zdc-pill.zdc-err  .dot { background: var(--zdc-err); }
.zdc-meter { background: rgba(255,255,255,.12); border-radius: 3px; height: 5px; overflow: hidden; width: 54px; }
.zdc-meter i { display: block; height: 100%; }
.zdc-card {
  background: var(--zdc-card);
  border: 1px solid var(--zdc-border);
  border-radius: 6px;
  padding: 16px 18px;
  margin-bottom: 16px;
}
.zdc-card h3 {
  margin: 0 0 12px 0;
  font-size: 13px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: .05em;
  border-bottom: 1px solid var(--zdc-border);
  padding-bottom: 7px;
  overflow: hidden;
}
.zdc-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 9px;
  min-height: 30px;
}
.zdc-row label.row-label {
  min-width: 180px;
  width: 180px;
  font-size: 13px;
  color: var(--zdc-dim);
  flex-shrink: 0;
}
.snap-table-scroll { overflow-x: auto; }
.snap-table { width:100%; border-collapse:collapse; font-size:12px; margin-top:6px; min-width:540px; }
/* Unraid's file tree, restyled to sit inside the cards */
.zdc-wrap .fileTree {
  position: static !important;
  left: auto !important;
  top: auto !important;
  flex: 0 0 100%;
  box-sizing: border-box;
  max-height: 240px;
  overflow: auto;
  margin-top: 4px;
  padding: 8px 10px;
  background: var(--zdc-sunken);
  color: var(--zdc-text);
  border: 1px solid var(--zdc-border-2);
  border-radius: 4px;
  font-family: 'Consolas','Monaco',monospace;
  font-size: 12px;
}
.zdc-wrap .fileTree UL.jqueryFileTree {
  font-family: inherit;
  font-size: 12px;
  line-height: 19px;
}
.zdc-wrap .fileTree UL.jqueryFileTree LI { padding-left: 22px; }
.zdc-wrap .fileTree UL.jqueryFileTree LI.directory,
.zdc-wrap .fileTree UL.jqueryFileTree LI.expanded,
.zdc-wrap .fileTree UL.jqueryFileTree LI.file,
.zdc-wrap .fileTree UL.jqueryFileTree LI.wait { background-position: left 2px; }
.zdc-wrap .fileTree UL.jqueryFileTree A {
  color: var(--zdc-text);
  padding: 1px 5px;
  border-radius: 3px;
}
.zdc-wrap .fileTree UL.jqueryFileTree A:hover {
  background: var(--zdc-blue);
  color: #fff;
}
.zdc-wrap .fileTree UL.jqueryFileTree LI.expanded > A { color: var(--zdc-accent); }
.zdc-wrap input.textPath { font-family: inherit; }

.zdc-note {
  font-size: 11px;
  color: var(--zdc-muted);
}

.zdc-toggle {
  position: relative;
  display: inline-block;
  width: 42px;
  height: 22px;
  flex-shrink: 0;
}
.zdc-toggle input { opacity: 0; width: 0; height: 0; position: absolute; }
.zdc-slider {
  position: absolute; inset: 0;
  background: #484e57;
  border-radius: 22px;
  cursor: pointer;
  transition: background .2s;
}
.zdc-slider::before {
  content: '';
  position: absolute;
  width: 16px; height: 16px;
  left: 3px; top: 3px;
  background: #fff;
  border-radius: 50%;
  transition: transform .2s;
}
.zdc-toggle input:checked + .zdc-slider { background: #3498db; }
.zdc-toggle input:checked + .zdc-slider::before { transform: translateX(20px); }

#log-viewer {
  background: var(--zdc-sunken);
  color: var(--zdc-text);
  font-family: 'Consolas','Monaco',monospace;
  font-size: 12px;
  line-height: 1.6;
  padding: 10px;
  height: 320px;
  overflow-y: auto;
  border-radius: 4px;
  border: 1px solid var(--zdc-border-2);
  white-space: pre-wrap;
  word-break: break-all;
}
#log-viewer .log-error { color: var(--zdc-err); }
#log-viewer .log-warn  { color: var(--zdc-warn); }
#log-viewer .log-ok    { color: var(--zdc-ok); }
#log-viewer .log-step  { color: var(--zdc-accent); font-weight: bold; }

#status-badge {
  display: inline-block;
  padding: 3px 11px;
  border-radius: 12px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: .04em;
}
.badge-idle    { background:var(--zdc-border-2); color:var(--zdc-dim); }
.badge-running { background:rgba(31,111,235,0.267); color:#58a6ff; }
.badge-done    { background:rgba(26,74,39,.27); color:var(--zdc-ok); }
.badge-error   { background:rgba(90,26,26,.27); color:var(--zdc-err); }

.snap-table th { text-align:center; padding:5px 8px; background:var(--zdc-sunken); color:var(--zdc-dim); font-weight:600; border-bottom:1px solid var(--zdc-border-2); }
.snap-table td { padding:4px 6px; border-bottom:1px solid var(--zdc-border-2); text-align:center; vertical-align:middle; }
.snap-table td:first-child { text-align:left; font-family:monospace; font-size:12px; }
.snap-table tr:last-child td { border-bottom:none; }
.snap-table input[type=number] { width:52px; padding:2px 5px; font-size:12px; }

#snap-browser-area { margin-top:10px; }
.snap-browser-path { display:flex; align-items:center; gap:4px; flex-wrap:wrap; margin-bottom:8px; font-size:12px; }
.snap-crumb { color:#58a6ff; cursor:pointer; text-decoration:underline; }
.snap-crumb-sep { color:#555; }
.snap-file-table { width:100%; border-collapse:collapse; font-size:12px; table-layout:fixed; }
.snap-file-table td:nth-child(2) { word-break:break-all; }
.snap-file-table th:nth-child(3), .snap-file-table td:nth-child(3) { width:110px; }
.snap-file-table th:nth-child(4), .snap-file-table td:nth-child(4) { width:150px; }
.snap-file-table th:nth-child(5), .snap-file-table td:nth-child(5) { width:90px; }
.snap-file-table th { text-align:left; padding:5px 8px; background:var(--zdc-sunken); color:var(--zdc-dim); font-weight:600; border-bottom:1px solid var(--zdc-border-2); }
.snap-file-table td { padding:4px 8px; border-bottom:1px solid var(--zdc-border-2); }
.snap-file-table tr:last-child td { border-bottom:none; }
.snap-file-table tr:hover td { background:var(--zdc-border-2); }
.snap-dir  { color:var(--zdc-warn); cursor:pointer; }
.snap-dir:hover { text-decoration:underline; }
.snap-file { color:var(--zdc-text); }
.snap-restore-btn { background:var(--zdc-blue); color:#fff; border:none; padding:2px 9px; border-radius:4px; font-size:11px; cursor:pointer; }
.snap-restore-btn:hover { background:#388bfd; }

select.snap-sel {
  background:var(--zdc-sunken); border:1px solid var(--zdc-border-2); color:var(--zdc-text);
  border-radius:4px; padding:4px 8px; font-size:13px;
}

.zdc-table { width:100%; border-collapse:collapse; font-size:13px; margin-top:4px; }
.zdc-table th { text-align:left; padding:6px 10px; background:var(--zdc-sunken); color:var(--zdc-dim); font-weight:600; border-bottom:1px solid var(--zdc-border-2); }
.zdc-table td { padding:5px 10px; border-bottom:1px solid var(--zdc-border-2); }
.zdc-table tr:last-child td { border-bottom:none; }
.tag-folder  { color:var(--zdc-warn); font-size:12px; }
.tag-dataset { color:var(--zdc-ok); font-size:12px; }

.btn-primary   { background:#238636; color:#fff; border:none; padding:7px 18px; border-radius:5px; font-size:13px; font-weight:600; cursor:pointer; }
.btn-primary:hover { background:#2ea043; }
.btn-primary:disabled { background:#3d4449; color:#888; cursor:default; }
.btn-secondary { background:var(--zdc-border-2); color:var(--zdc-text); border:1px solid #444d56; padding:6px 14px; border-radius:5px; font-size:13px; cursor:pointer; }
.btn-secondary:hover { background:var(--zdc-border-2); }
.btn-danger    { background:#b91c1c; color:#fff; border:none; padding:7px 14px; border-radius:5px; font-size:13px; font-weight:600; cursor:pointer; }
.btn-danger:hover { background:var(--zdc-err); }

input[type=text], input[type=number] {
  background:var(--zdc-sunken); border:1px solid var(--zdc-border-2); color:var(--zdc-text);
  border-radius:4px; padding:4px 8px; font-size:13px;
}
input[type=text]:focus, input[type=number]:focus { outline:none; border-color:#58a6ff; }
.w80  { width:80px; }
.w180 { width:180px; }

.dry-run-banner {
  background:rgba(227,179,65,0.094); border:1px solid rgba(227,179,65,0.533);
  border-radius:5px; padding:7px 13px; color:var(--zdc-warn);
  font-size:13px; margin-bottom:12px; display:none;
}
</style>

<div class="zdc-wrap">

<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:18px;">
  <div>
    <h2 style="margin:0;font-size:18px;">ZFS Toolkit</h2>
    <p style="margin:3px 0 0;color:var(--zdc-dim);font-size:12px;">Convert plain folders to ZFS child datasets — with Docker &amp; VM awareness.</p>
  </div>
  <div style="display:flex;align-items:center;gap:14px;flex-wrap:wrap;justify-content:flex-end;">
    <label style="display:flex;align-items:center;gap:7px;font-size:12px;color:var(--zdc-dim);cursor:pointer;"
           title="Adds a ZFS entry to the top menu bar, next to Docker and VMs. Reload the page after changing it.">
      <span class="zdc-toggle" style="pointer-events:none;">
        <input type="checkbox" id="show_in_menu" name="show_in_menu" <?= cfgBool('show_in_menu','yes')?'checked':'' ?>
               onchange="onMenuToggle()"><span class="zdc-slider"></span>
      </span>
      Show in top menu
    </label>
    <span id="menu-hint" style="font-size:11px;color:var(--zdc-warn);"></span>
    <span id="autosave-status" style="font-size:11px;color:var(--zdc-ok);opacity:0;transition:opacity .4s;"></span>
    <span id="status-badge" class="badge-idle">Idle</span>
  </div>
</div>


<div class="zdc-status" id="zdc-status"></div>

<div class="zdc-tabs" role="tablist">
  <button type="button" class="zdc-tab active" id="tabbtn-convert" role="tab" onclick="zdcTab('convert')">Conversion</button>
  <button type="button" class="zdc-tab" id="tabbtn-snapshots" role="tab" onclick="zdcTab('snapshots')">Snapshots</button>
  <button type="button" class="zdc-tab" id="tabbtn-browse" role="tab" onclick="zdcTab('browse')">Browse &amp; Restore</button>
  <button type="button" class="zdc-tab" id="tabbtn-replicate" role="tab" onclick="zdcTab('replicate')">Replication</button>
</div>

<div class="zdc-panel" id="tab-convert" role="tabpanel" hidden>
<form id="settings-form" onsubmit="return false;">
<div class="zdc-cols">
<div class="zdc-card">
  <h3>General Settings</h3>

  <div class="zdc-row">
    <label class="row-label">Dry Run</label>
    <label class="zdc-toggle"><input type="checkbox" name="dry_run" id="dry_run" <?= cfgBool('dry_run','yes')?'checked':'' ?>><span class="zdc-slider"></span></label>
    <span class="zdc-note">Simulate — no changes made</span>
  </div>
  <div class="zdc-row">
    <label class="row-label">Cleanup after conversion</label>
    <label class="zdc-toggle"><input type="checkbox" name="cleanup" id="cleanup" <?= cfgBool('cleanup','yes')?'checked':'' ?>><span class="zdc-slider"></span></label>
    <span class="zdc-note">Remove temp folder after successful copy</span>
  </div>
  <div class="zdc-row">
    <label class="row-label">Replace spaces with underscores</label>
    <label class="zdc-toggle"><input type="checkbox" name="replace_spaces" id="replace_spaces" <?= cfgBool('replace_spaces')?'checked':'' ?>><span class="zdc-slider"></span></label>
  </div>
  <div class="zdc-row">
    <label class="row-label">Send Unraid notifications</label>
    <label class="zdc-toggle"><input type="checkbox" name="send_notifications" id="send_notifications" <?= cfgBool('send_notifications','yes')?'checked':'' ?>><span class="zdc-slider"></span></label>
  </div>
  <div class="zdc-row">
    <label class="row-label">Buffer zone (%)</label>
    <input type="number" name="buffer_zone" value="<?= cfg('buffer_zone','11') ?>" min="0" max="100" class="w80">
    <span class="zdc-note">Extra free space required beyond folder size</span>
  </div>
  <div class="zdc-row">
    <label class="row-label">Validation tolerance (%)</label>
    <input type="number" name="validation_tolerance" value="<?= cfg('validation_tolerance','5') ?>" min="0" max="20" class="w80">
    <span class="zdc-note">Allowed size difference after copy</span>
  </div>
</div>
<div class="zdc-card">
  <h3>Automatic Schedule</h3>
  <div class="zdc-row">
    <label class="row-label">Enable auto-conversion</label>
    <label class="zdc-toggle"><input type="checkbox" name="cron_enabled" id="cron_enabled" <?= cfgBool('cron_enabled')?'checked':'' ?> onchange="toggleCronDetails()"><span class="zdc-slider"></span></label>
    <span class="zdc-note">Run automatically on a schedule</span>
  </div>
  <div id="cron-details" <?= cfgBool('cron_enabled')?'':'style="display:none"' ?>>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">Schedule</label>
      <select name="cron_preset" id="cron_preset" onchange="updateCronPreview()" style="width:160px;background:var(--zdc-sunken);border:1px solid var(--zdc-border-2);color:var(--zdc-text);border-radius:4px;padding:4px 8px;font-size:13px;">
        <option value="hourly"  <?= cfg('cron_preset','daily')==='hourly' ?'selected':'' ?>>Every hour</option>
        <option value="6hourly" <?= cfg('cron_preset','daily')==='6hourly'?'selected':'' ?>>Every 6 hours</option>
        <option value="daily"   <?= cfg('cron_preset','daily')==='daily'  ?'selected':'' ?>>Daily</option>
        <option value="weekly"  <?= cfg('cron_preset','daily')==='weekly' ?'selected':'' ?>>Weekly</option>
        <option value="custom"  <?= cfg('cron_preset','daily')==='custom' ?'selected':'' ?>>Custom cron</option>
      </select>
    </div>
    <div id="cron-time-row" class="zdc-row" style="margin-left:20px">
      <label class="row-label">Time</label>
      <input type="number" name="cron_hour"   id="cron_hour"   value="<?= cfg('cron_hour','2') ?>"  min="0" max="23" class="w80" style="width:60px" oninput="updateCronPreview()"> h &nbsp;
      <input type="number" name="cron_minute" id="cron_minute" value="<?= cfg('cron_minute','0') ?>" min="0" max="59" class="w80" style="width:60px" oninput="updateCronPreview()"> min
    </div>
    <div id="cron-weekday-row" class="zdc-row" style="margin-left:20px;display:none">
      <label class="row-label">Day of week</label>
      <select name="cron_weekday" id="cron_weekday" onchange="updateCronPreview()" style="width:130px;background:var(--zdc-sunken);border:1px solid var(--zdc-border-2);color:var(--zdc-text);border-radius:4px;padding:4px 8px;font-size:13px;">
        <option value="0" <?= cfg('cron_weekday','0')==='0'?'selected':'' ?>>Sunday</option>
        <option value="1" <?= cfg('cron_weekday','0')==='1'?'selected':'' ?>>Monday</option>
        <option value="2" <?= cfg('cron_weekday','0')==='2'?'selected':'' ?>>Tuesday</option>
        <option value="3" <?= cfg('cron_weekday','0')==='3'?'selected':'' ?>>Wednesday</option>
        <option value="4" <?= cfg('cron_weekday','0')==='4'?'selected':'' ?>>Thursday</option>
        <option value="5" <?= cfg('cron_weekday','0')==='5'?'selected':'' ?>>Friday</option>
        <option value="6" <?= cfg('cron_weekday','0')==='6'?'selected':'' ?>>Saturday</option>
      </select>
    </div>
    <div id="cron-custom-row" class="zdc-row" style="margin-left:20px;display:none">
      <label class="row-label">Cron expression</label>
      <input type="text" name="cron_custom" id="cron_custom" value="<?= cfg('cron_custom','0 2 * * *') ?>" style="width:180px" placeholder="0 2 * * *" oninput="updateCronPreview()">
    </div>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">Cron expression</label>
      <span id="cron-preview" style="font-size:13px;color:var(--zdc-accent);font-family:monospace;"></span>
      <div id="cron-hint" style="display:none;color:var(--zdc-err);font-size:12px;margin-top:4px;"></div>
    </div>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">Active cron entry</label>
      <span id="cron-active" style="font-size:12px;color:var(--zdc-dim);font-family:monospace;">—</span>
    </div>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">Server time</label>
        <?php
          $tz = trim(shell_exec('cat /etc/timezone 2>/dev/null || timedatectl 2>/dev/null | grep "Time zone" | awk \'{print $3}\'') ?? '');
          $tzLabel = ' ' . date('T') . ($tz ? " ($tz)" : '');
        ?>
        <span id="server-time" style="font-size:12px;color:var(--zdc-dim);font-family:monospace;"
              data-epoch="<?= time() + (int)date('Z') ?>"
              data-suffix="<?= htmlspecialchars($tzLabel, ENT_QUOTES) ?>"><?=
          htmlspecialchars(date('H:i:s') . $tzLabel) ?></span>
    </div>
  </div>
</div>
<div class="zdc-card">
  <h3>Docker Containers</h3>
  <div class="zdc-row">
    <label class="row-label">Process appdata</label>
    <label class="zdc-toggle"><input type="checkbox" name="should_process_containers" id="should_process_containers" <?= cfgBool('should_process_containers','yes')?'checked':'' ?> onchange="toggle('container-details',this.checked)"><span class="zdc-slider"></span></label>
  </div>
  <div id="container-details" <?= cfgBool('should_process_containers','yes')?'':'style="display:none"' ?>>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">Appdata pool</label>
      <input type="text" name="appdata_pool" value="<?= cfg('appdata_pool','cache') ?>" class="w180">
    </div>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">Appdata dataset</label>
      <input type="text" name="appdata_dataset" value="<?= cfg('appdata_dataset','appdata') ?>" class="w180">
    </div>
  </div>
</div>
<div class="zdc-card">
  <h3>Virtual Machines</h3>
  <div class="zdc-row">
    <label class="row-label">Process VM domains</label>
    <label class="zdc-toggle"><input type="checkbox" name="should_process_vms" id="should_process_vms" <?= cfgBool('should_process_vms')?'checked':'' ?> onchange="toggle('vm-details',this.checked)"><span class="zdc-slider"></span></label>
  </div>
  <div id="vm-details" <?= cfgBool('should_process_vms')?'':'style="display:none"' ?>>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">VM pool</label>
      <input type="text" name="vm_pool" value="<?= cfg('vm_pool','cache') ?>" class="w180">
    </div>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">VM dataset</label>
      <input type="text" name="vm_dataset" value="<?= cfg('vm_dataset','domains') ?>" class="w180">
    </div>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">Graceful shutdown timeout (s)</label>
      <input type="number" name="vm_forceshutdown_wait" value="<?= cfg('vm_forceshutdown_wait','90') ?>" min="10" max="600" class="w80">
    </div>
  </div>
</div>
<div class="zdc-card">
  <h3>Additional Source Datasets</h3>
  <div class="zdc-row">
    <label class="row-label">Extra datasets</label>
    <input type="text" name="extra_datasets" value="<?= cfg('extra_datasets') ?>" style="width:300px" placeholder="pool/dataset,pool/other">
    <span class="zdc-note">Comma-separated</span>
  </div>
</div>
<div class="zdc-card">
  <h3>Scan &amp; Convert
    <span style="float:right;display:flex;gap:8px;align-items:center;">
      <span id="scan-spinner" style="display:none;font-size:12px;color:var(--zdc-dim);">Scanning…</span>
      <button type="button" class="btn-secondary" style="padding:3px 11px;font-size:12px;" onclick="scanFolders()">Refresh</button>
    </span>
  </h3>
  <div id="folder-list"><p style="color:var(--zdc-dim);font-size:13px;margin:0;">Click Refresh to scan for folders.</p></div>
  <div id="dry-run-banner" class="dry-run-banner">⚠ Dry run mode active — no actual changes will be made.</div>
  <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;">
    <button type="button" class="btn-primary" id="btn-start" onclick="startConversion()">Start Conversion</button>
    <button type="button" class="btn-danger" id="btn-stop" onclick="stopConversion()" style="display:none;">Stop</button>
    <span id="progress-text" style="font-size:13px;color:var(--zdc-dim);"></span>
  </div>
</div>
</div>
<div class="zdc-wide-stack">
<div class="zdc-card wide">
  <h3>Live Log
    <button type="button" class="btn-secondary" style="float:right;padding:3px 11px;font-size:12px;" onclick="clearLog()">Clear</button>
  </h3>
  <div id="log-viewer"></div>
</div>
</div>
</form>
</div>

<div class="zdc-panel" id="tab-snapshots" role="tabpanel">
<div class="zdc-cols">
<div class="zdc-card">
  <h3>Snapshot Schedule</h3>
<div class="zdc-row">
    <label class="row-label">Enable automatic snapshots</label>
    <label class="zdc-toggle"><input type="checkbox" id="snapshots_enabled" name="snapshots_enabled" <?= cfgBool('snapshots_enabled')?'checked':'' ?> onchange="toggleSnapDetails()"><span class="zdc-slider"></span></label>
    <span class="zdc-note">Uses native <code>zfs snapshot</code> — no extra tools needed</span>
  </div>
  <div class="snap-gated"<?= cfgBool('snapshots_enabled')?'':' style="display:none"' ?>>
    <div class="zdc-row">
      <label class="row-label">Check every</label>
      <select name="snap_schedule_preset" id="snap_schedule_preset" onchange="updateSnapPreview()" class="snap-sel">
        <option value="5min"   <?= cfg('snap_schedule_preset','15min')==='5min'  ?'selected':'' ?>>Every 5 minutes</option>
        <option value="15min"  <?= cfg('snap_schedule_preset','15min')==='15min' ?'selected':'' ?>>Every 15 minutes</option>
        <option value="30min"  <?= cfg('snap_schedule_preset','15min')==='30min' ?'selected':'' ?>>Every 30 minutes</option>
        <option value="hourly" <?= cfg('snap_schedule_preset','15min')==='hourly'?'selected':'' ?>>Every hour</option>
        <option value="custom" <?= cfg('snap_schedule_preset','15min')==='custom'?'selected':'' ?>>Custom cron</option>
      </select>
    </div>
    <div id="snap-custom-row" class="zdc-row" style="<?= cfg('snap_schedule_preset','15min')==='custom'?'':'display:none' ?>">
      <label class="row-label">Custom expression</label>
      <input type="text" name="snap_schedule_custom" id="snap_schedule_custom" value="<?= cfg('snap_schedule_custom','*/15 * * * *') ?>" style="width:180px" oninput="updateSnapPreview()">
    </div>
    <div class="zdc-row">
      <label class="row-label">Cron expression</label>
      <span id="snap-cron-preview" style="font-size:13px;color:var(--zdc-accent);font-family:monospace;"></span>
      <div id="snap-cron-hint" style="display:none;color:var(--zdc-err);font-size:12px;margin-top:4px;"></div>
    </div>
    <p class="zdc-note" style="margin:2px 0 0;">
      How often the plugin looks for work, not how often it snapshots. Each run creates only
      what the current period is still missing, so checking every 15 minutes with hourly
      retention still yields one snapshot per hour. Only <b>Frequent</b> takes one on every run.
    </p>

    
<div style="margin-top:12px;display:flex;flex-wrap:wrap;gap:16px;font-size:12px;color:var(--zdc-dim);">
      <span>Cron: <span id="snap-cron-badge" style="font-family:monospace;color:var(--zdc-warn);">—</span></span>
      <span>Datasets: <span id="snap-dataset-count">—</span></span>
      <span>Snapshots: <span id="snap-total-count">—</span></span>
      <span>Last run: <span id="snap-last-run">—</span></span>
      <span>Result: <span id="snap-last-result">—</span></span>
    </div>
    <div id="snap-health" style="font-size:12px;margin:6px 0 0;display:none;"></div>
    <div id="snap-no-datasets-warn" style="color:var(--zdc-warn);font-size:12px;margin:6px 0 0;display:none;">
      ⚠ No datasets configured — add at least one dataset above (settings save automatically).
    </div>
  
  </div>
<div style="margin-top:12px;display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
    <button type="button" class="btn-primary" id="snap-run-btn" onclick="runSnapshotNow(false)" <?= cfgBool('snapshots_enabled')?'':'disabled' ?>>Run Now</button>
    <button type="button" class="btn-secondary" id="snap-dry-btn" onclick="runSnapshotNow(true)" title="Show what would be created and pruned, change nothing">Dry Run</button>
    <button type="button" class="btn-secondary" id="snap-log-toggle-btn" onclick="toggleSnapLog()">View Log</button>
    <a class="btn-secondary" href="/plugins/zfs.toolkit/scripts/diagnostics.php?download=1"
       style="text-decoration:none;padding:6px 14px;" title="Download a support bundle (config, logs, zfs state). Private keys are excluded.">Diagnostics</a>
    <span id="snap-save-result" style="font-size:13px;"></span>
  </div>
<div id="snap-log-section" style="margin-top:10px;display:none;">
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:5px;">
      <span style="font-size:12px;font-weight:600;color:var(--zdc-dim);">Snapshot Log <span style="font-weight:400;color:#555;">(last 30 lines of /tmp/zfs.toolkit/snapshots.log)</span></span>
      <button type="button" class="btn-secondary" style="padding:2px 10px;font-size:11px;" onclick="clearSnapLog()">Clear view</button>
    </div>
    <div id="snap-log-viewer" style="background:var(--zdc-sunken);color:var(--zdc-text);font-family:'Consolas','Monaco',monospace;font-size:12px;line-height:1.6;padding:10px;height:220px;overflow-y:auto;border-radius:4px;border:1px solid var(--zdc-border-2);white-space:pre-wrap;word-break:break-all;"></div>
  </div>
</div>
<div class="zdc-card snap-gated"<?= cfgBool('snapshots_enabled')?'':' style="display:none"' ?>>
  <h3>Retention</h3>
<p class="zdc-sub">Global Retention (per dataset unless overridden)</p>
    <div style="display:flex;flex-wrap:wrap;gap:8px 20px;margin-bottom:10px;">
      <?php
        $snap_fields = [
          ['snap_hourly',  'Hourly',   '24'],
          ['snap_daily',   'Daily',    '30'],
          ['snap_weekly',  'Weekly',   '4'],
          ['snap_monthly', 'Monthly',  '3'],
          ['snap_yearly',  'Yearly',   '0'],
          ['snap_frequent','Frequent', '0'],
        ];
        foreach ($snap_fields as [$k,$lbl,$def]) {
          echo '<label style="font-size:12px;color:var(--zdc-dim);display:flex;flex-direction:column;align-items:center;gap:3px;">'
            . $lbl
            . '<input type="number" name="'.$k.'" value="'.cfg($k,$def).'" min="0" max="999" style="width:60px;text-align:center;">'
            . '</label>';
        }
      ?>
    </div>
    <div class="zdc-row">
      <label class="row-label">Daily snapshot time (hour)</label>
      <input type="number" name="snap_daily_hour" value="<?= cfg('snap_daily_hour','2') ?>" min="0" max="23" class="w80" style="width:60px">
      <span class="zdc-note">Earliest hour for daily / weekly / monthly / yearly &mdash; still only one per period. 0 = right after midnight. A period missed because the server was off is caught up on the next run.</span>
    </div>
    <div class="zdc-row">
      <label class="row-label">Warn below free space</label>
      <input type="number" name="snap_min_free_pct" value="<?= cfg('snap_min_free_pct','5') ?>" min="0" max="90" class="w80" style="width:60px">
      <span class="zdc-note">% — notify when a snapshotted pool fills up (0 = off)</span>
    </div>

    <p class="zdc-sub">
      Maximum age (days, 0 = keep by count only)
    </p>
    <div style="display:flex;flex-wrap:wrap;gap:8px 20px;margin-bottom:6px;">
      <?php
        foreach ([['snap_age_hourly','Hourly'],['snap_age_daily','Daily'],['snap_age_weekly','Weekly'],
                  ['snap_age_monthly','Monthly'],['snap_age_yearly','Yearly'],['snap_age_frequent','Frequent']] as [$k,$lbl]) {
          echo '<label style="font-size:12px;color:var(--zdc-dim);display:flex;flex-direction:column;align-items:center;gap:3px;">'
            . $lbl
            . '<input type="number" name="'.$k.'" value="'.cfg($k,'0').'" min="0" max="3650" style="width:60px;text-align:center;">'
            . '</label>';
        }
      ?>
    </div>
    <div class="zdc-row">
      <label class="row-label">Free-space target</label>
      <input type="text" name="snap_free_target" value="<?= cfg('snap_free_target','') ?>" placeholder="e.g. 100G or 10%" style="width:120px">
      <span class="zdc-note">Prune the oldest snapshots until the pool has this much free (empty = off)</span>
    </div>

    
</div>
</div>
<div class="zdc-wide-stack">
<div class="zdc-card wide snap-gated"<?= cfgBool('snapshots_enabled')?'':' style="display:none"' ?>>
  <h3>Datasets to snapshot</h3>
    <div>
      <div class="snap-table-scroll">
      <table class="snap-table">
        <thead><tr>
          <th style="text-align:left">Dataset</th>
          <th>Recursive</th>
          <th title="Use global retention values">Template</th>
          <th>Hourly</th><th>Daily</th><th>Weekly</th><th>Monthly</th><th>Yearly</th>
          <th></th>
        </tr></thead>
        <tbody id="snap-ds-tbody"></tbody>
      </table>
      </div><!-- /snap-table-scroll -->
      <div style="margin-top:8px;display:flex;gap:8px;align-items:center;">
        <select id="snap-ds-picker" class="snap-sel" style="width:260px">
          <option value="">— select dataset —</option>
        </select>
        <button type="button" class="btn-secondary" style="padding:4px 12px;font-size:12px;" onclick="addSnapDataset()">Add</button>
      </div>
    </div>

    
</div>
<div class="zdc-card wide">
  <h3>Snapshot Manager
    <span style="float:right;font-weight:400;text-transform:none;letter-spacing:0;font-size:11px;color:var(--zdc-dim);">
      Which snapshot is eating my space?
    </span>
  </h3>

  <div style="display:flex;flex-wrap:wrap;gap:10px;align-items:flex-end;margin-bottom:10px;">
    <div>
      <label style="font-size:12px;color:var(--zdc-dim);display:block;margin-bottom:3px;">Dataset</label>
      <select id="mgr-dataset" class="snap-sel" style="width:min(260px,100%)" onchange="loadSnapManager()">
        <option value="">— all datasets —</option>
      </select>
    </div>
    <label style="font-size:12px;color:var(--zdc-dim);display:flex;align-items:center;gap:5px;height:30px;">
      <input type="checkbox" id="mgr-only-auto" checked onchange="loadSnapManager()"> only plugin snapshots
    </label>
    <div>
      <label style="font-size:12px;color:var(--zdc-dim);display:block;margin-bottom:3px;">Filter</label>
      <input type="text" id="mgr-filter" style="width:min(230px,100%);font-size:12px;"
             placeholder="e.g. hourly  or  auto-daily-2026-07*"
             oninput="renderSnapManager()" title="Substring match, or use * and ? as wildcards. Select-all only takes the rows shown.">
    </div>
    <button type="button" class="btn-secondary" style="padding:5px 14px;font-size:12px;" onclick="loadSnapManager()">Refresh</button>
    <span style="flex:1"></span>
    <button type="button" class="btn-secondary" style="padding:5px 12px;font-size:12px;" onclick="mgrHoldSelected(true)" title="Protect the selected snapshots from automatic pruning">Hold</button>
    <button type="button" class="btn-secondary" style="padding:5px 12px;font-size:12px;" onclick="mgrHoldSelected(false)" title="Remove the plugin hold again">Release</button>
    <button type="button" class="btn-danger" style="padding:5px 14px;font-size:12px;" onclick="mgrDeleteSelected()">Delete selected (<span id="mgr-sel-count">0</span>)</button>
  </div>

  <div id="mgr-summary" style="font-size:12px;color:var(--zdc-dim);margin-bottom:8px;"></div>

  <div style="overflow-x:auto;max-height:420px;overflow-y:auto;">
    <table class="snap-table" style="min-width:820px;">
      <thead><tr>
        <th style="width:26px;"><input type="checkbox" id="mgr-all" onchange="mgrSelectAll(this)"></th>
        <th style="text-align:left">Snapshot</th>
        <th>Type</th>
        <th style="text-align:right">Used</th>
        <th style="text-align:right">Refer</th>
        <th>Created</th>
        <th>Hold</th>
        <th></th>
      </tr></thead>
      <tbody id="mgr-tbody">
        <tr><td colspan="8" style="color:var(--zdc-dim);padding:10px;">Click Refresh to load snapshots.</td></tr>
      </tbody>
    </table>
  </div>
  <div style="display:flex;gap:14px;align-items:center;flex-wrap:wrap;margin-top:8px;">
    <span id="mgr-result" style="font-size:12px;"></span>
    <span id="mgr-sel-note" style="font-size:12px;"></span>
  </div>
</div>
</div>
</div>

<div class="zdc-panel" id="tab-browse" role="tabpanel">
<div class="zdc-wide-stack">
<div class="zdc-card wide">
  <h3>Snapshot Browser</h3>
  <p style="font-size:12px;color:var(--zdc-dim);margin:0 0 10px;">Browse any ZFS snapshot and restore individual files or folders to the live dataset.</p>

  <div style="display:flex;flex-wrap:wrap;gap:10px;align-items:flex-end;margin-bottom:10px;">
    <div style="flex:1 1 220px;min-width:0;max-width:360px;">
      <label style="font-size:12px;color:var(--zdc-dim);display:block;margin-bottom:3px;">Dataset</label>
      <select id="browser-dataset" class="snap-sel" style="width:100%" onchange="loadBrowserSnapshots()">
        <option value="">— select dataset —</option>
      </select>
    </div>
    <div style="flex:1 1 220px;min-width:0;max-width:360px;">
      <label style="font-size:12px;color:var(--zdc-dim);display:block;margin-bottom:3px;">Snapshot</label>
      <select id="browser-snapshot" class="snap-sel" style="width:100%" onchange="browserBrowse('/')">
        <option value="">— select snapshot —</option>
      </select>
    </div>
    <div style="align-self:flex-end;display:flex;gap:6px;">
      <button type="button" class="btn-primary" style="padding:5px 16px;font-size:12px;" onclick="browserBrowse('/')">Browse</button>
      <button type="button" class="btn-secondary" style="padding:5px 14px;font-size:12px;" onclick="browserBrowse(browserPath || '/')">Refresh</button>
    </div>
  </div>

  <div style="display:flex;flex-wrap:wrap;gap:8px;align-items:flex-end;margin-bottom:10px;padding-top:8px;border-top:1px solid var(--zdc-border);">
    <div style="flex:1 1 220px;min-width:0;max-width:360px;">
      <label style="font-size:12px;color:var(--zdc-dim);display:block;margin-bottom:3px;">Compare with</label>
      <select id="diff-target" class="snap-sel" style="width:100%">
        <option value="">— live filesystem —</option>
      </select>
    </div>
    <button type="button" class="btn-secondary" style="padding:5px 14px;font-size:12px;" onclick="runDiff()"
            title="zfs diff between the selected snapshot and this target">Show changes</button>
    <span id="diff-summary" style="font-size:12px;color:var(--zdc-dim);"></span>
  </div>
  <div id="diff-result" style="display:none;max-height:240px;overflow:auto;background:var(--zdc-sunken);border:1px solid var(--zdc-border-2);border-radius:4px;padding:8px;font-family:'Consolas','Monaco',monospace;font-size:12px;line-height:1.5;margin-bottom:10px;"></div>

  <div id="snap-browser-area">
    <div class="snap-browser-path" id="snap-breadcrumb"></div>
    <div id="snap-file-list">
      <p style="color:var(--zdc-dim);font-size:13px;">Select a dataset and snapshot above, then click Browse.</p>
    </div>
    <div style="position:relative;margin-top:10px;display:flex;flex-wrap:wrap;align-items:center;gap:8px;font-size:12px;">
      <span style="color:var(--zdc-dim);white-space:nowrap;">Destination folder:</span>
      <input type="text" id="restore-dst" class="textPath"
             data-pickroot="/mnt/" data-picktop="/mnt/" data-pickfolders="true"
             data-pickfilter="HIDE_FILES_FILTER"
             style="flex:1 1 260px;min-width:0;max-width:560px;font-size:12px;"
             placeholder="click to pick, or type a path + Enter — empty = original location">
      <button type="button" id="dest-browse-btn" class="btn-secondary" style="padding:4px 12px;font-size:12px;display:none;" onclick="toggleDestPicker()">Browse…</button>
      <span id="dest-hint" style="font-size:11px;color:var(--zdc-dim);"></span>
      <button type="button" class="btn-primary" style="padding:4px 12px;font-size:12px;" onclick="restoreSelected()">Restore Selected (<span id="sel-count">0</span>)</button>
      <span id="restore-result" style="font-size:12px;"></span>
    </div>

    <div id="dest-picker" style="display:none;margin-top:8px;border:1px solid var(--zdc-border);border-radius:4px;padding:8px;background:var(--zdc-sunken);">
      <div class="snap-browser-path" id="dest-crumbs" style="margin-bottom:6px;"></div>
      <div id="dest-list" style="max-height:180px;overflow-y:auto;font-size:12px;"></div>
      <div style="display:flex;flex-wrap:wrap;gap:8px;align-items:center;margin-top:8px;">
        <span style="font-size:12px;color:var(--zdc-dim);">Selected:</span>
        <code id="dest-current" style="font-size:12px;color:var(--zdc-accent);">/mnt</code>
        <input type="text" id="dest-newfolder" placeholder="optional: new subfolder" style="font-size:12px;width:170px;">
        <button type="button" class="btn-primary" style="padding:3px 10px;font-size:12px;" onclick="destUse()">Use this folder</button>
        <button type="button" class="btn-secondary" style="padding:3px 10px;font-size:12px;" onclick="toggleDestPicker()">Cancel</button>
        <span id="dest-note" style="font-size:11px;color:var(--zdc-dim);"></span>
      </div>
    </div>
  </div>
</div>
</div>
</div>

<div class="zdc-panel" id="tab-replicate" role="tabpanel">
<div class="zdc-wide-stack">
<div class="zdc-card wide">
  <h3>Replication (ZFS Send)</h3>
  <p style="font-size:12px;color:var(--zdc-dim);margin:0 0 10px;">
    Snapshots on the same pool do not survive losing that pool. Replication copies them to a
    second pool or another machine using native <code>zfs send</code> / <code>zfs recv</code>.
  </p>

  <div class="zdc-row">
    <label class="row-label">Enable scheduled replication</label>
    <label class="zdc-toggle"><input type="checkbox" id="send_enabled" name="send_enabled" <?= cfgBool('send_enabled')?'checked':'' ?> onchange="toggleSendDetails()"><span class="zdc-slider"></span></label>
  </div>

  <div id="send-details" <?= cfgBool('send_enabled')?'':'style="display:none"' ?>>
    <div class="zdc-row" style="margin-left:20px">
      <label class="row-label">Run</label>
      <select name="send_schedule_preset" id="send_schedule_preset" onchange="updateSendPreview()" class="snap-sel">
        <option value="hourly"  <?= cfg('send_schedule_preset','daily')==='hourly' ?'selected':'' ?>>Every hour</option>
        <option value="6hourly" <?= cfg('send_schedule_preset','daily')==='6hourly'?'selected':'' ?>>Every 6 hours</option>
        <option value="daily"   <?= cfg('send_schedule_preset','daily')==='daily'  ?'selected':'' ?>>Daily</option>
        <option value="weekly"  <?= cfg('send_schedule_preset','daily')==='weekly' ?'selected':'' ?>>Weekly (Sunday)</option>
        <option value="custom"  <?= cfg('send_schedule_preset','daily')==='custom' ?'selected':'' ?>>Custom cron</option>
      </select>
      <label style="font-size:12px;color:var(--zdc-dim);" id="send-hour-wrap">at hour
        <input type="number" name="send_schedule_hour" id="send_schedule_hour" value="<?= cfg('send_schedule_hour','4') ?>" min="0" max="23" style="width:56px" oninput="updateSendPreview()">
      </label>
      <input type="text" name="send_schedule_custom" id="send_schedule_custom" value="<?= cfg('send_schedule_custom','0 4 * * *') ?>" style="width:150px;display:none" oninput="updateSendPreview()">
      <span id="send-cron-preview" style="font-size:13px;color:var(--zdc-accent);font-family:monospace;"></span>
      <div id="send-cron-hint" style="display:none;color:var(--zdc-err);font-size:12px;margin-top:4px;"></div>
    </div>

    <p class="zdc-sub">Jobs</p>
    <div style="margin-left:20px;overflow-x:auto;">
      <datalist id="zdc-dataset-list"></datalist>
      <table class="snap-table" style="min-width:900px;">
        <thead><tr>
          <th style="width:26px;">On</th>
          <th style="text-align:left">Name</th>
          <th style="text-align:left">Source dataset</th>
          <th style="text-align:left">Destination dataset &mdash; where the copy goes</th>
          <th title="zfs send -R: also replicate every child dataset of the source">Children</th>
          <th title="zfs recv -F: let the destination be rolled back if it diverged">Force</th>
          <th></th>
        </tr></thead>
        <tbody id="send-tbody"></tbody>
      </table>
      <div style="margin-top:8px;display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
        <button type="button" class="btn-secondary" style="padding:4px 12px;font-size:12px;" onclick="addSendJob()">Add job</button>
        <button type="button" class="btn-primary"   style="padding:4px 12px;font-size:12px;" onclick="sendRun('dry_run')">Dry run</button>
        <button type="button" class="btn-primary"   style="padding:4px 12px;font-size:12px;" onclick="sendRun('run_now')">Replicate now</button>
        <span id="send-result" style="font-size:12px;"></span>
      </div>
      <div class="zdc-note" style="margin-top:10px;line-height:1.6;">
        <b style="color:var(--zdc-text);">Children</b> &mdash; <code>zfs send -R</code>. Off: only the
        source dataset itself is copied. On: every dataset below it comes along too, so
        <code>cache/appdata</code> also carries <code>cache/appdata/plex</code> and the rest.
        Turn it on when the source is a parent that holds other datasets.<br>

        <b style="color:var(--zdc-text);">Force</b> &mdash; <code>zfs recv -F</code>. Off (recommended):
        the transfer aborts if the destination has changed since the last run. On: the destination is
        rolled back to the last common snapshot first, <b style="color:var(--zdc-warn);">discarding
        anything written there in the meantime</b>. Only switch it on if the destination is a pure
        copy that nothing else writes to.<br>

        <b style="color:var(--zdc-text);">Both ends are ZFS datasets, never folders.</b>
        <code>zfs recv</code> can only write into a dataset, so a path like
        <code>/mnt/user/Backup</code> is not a valid destination &mdash; use <code>pool/dataset</code>.
        The pool has to exist; the dataset is created on the first run.<br>

        <b style="color:var(--zdc-text);">Source</b> &mdash; picked from the datasets on this server.
        <b style="color:var(--zdc-text);">Destination</b> &mdash; the dataset the copy is written to.
        Typed rather than picked, because it usually does not exist yet; for a local target the
        existing datasets are offered as suggestions.
        With <i>over SSH</i> it lives on the remote host, written the way ZFS does it:
        <code>root@10.0.0.5 : tank/backup/appdata</code>. It is created on first run; the pool has to
        exist already.<br>

        <b style="color:var(--zdc-text);">SSH</b> &mdash; key-based authentication only, this plugin
        never handles passwords. Create a key, copy it to the destination, then enter its path above:<br>
        <code style="font-size:11px;">ssh-keygen -t ed25519 -f /boot/config/plugins/zfs.toolkit/id_send -N ""</code><br>
        <code style="font-size:11px;">ssh-copy-id -i /boot/config/plugins/zfs.toolkit/id_send.pub root@10.0.0.5</code><br>

        Use <b style="color:var(--zdc-text);">Test</b> to check the destination is reachable and
        <b style="color:var(--zdc-text);">Dry run</b> to see what would be sent &mdash; neither
        transfers any data.
      </div>
    </div>

    <div style="margin-top:10px;margin-left:20px;display:flex;flex-wrap:wrap;gap:16px;font-size:12px;color:var(--zdc-dim);">
      <span>Cron: <span id="send-cron-badge" style="font-family:monospace;color:var(--zdc-warn);">—</span></span>
      <span>Last run: <span id="send-last-run">—</span></span>
      <span>Result: <span id="send-last-result">—</span></span>
    </div>
    <div id="send-log-viewer" style="margin-top:8px;background:var(--zdc-sunken);color:var(--zdc-text);font-family:'Consolas','Monaco',monospace;font-size:12px;line-height:1.5;padding:10px;height:180px;overflow-y:auto;border-radius:4px;border:1px solid var(--zdc-border-2);white-space:pre-wrap;word-break:break-all;"></div>
  </div>
</div>
</div>
</div>

</div>
<script>
var _poll = null, _logFile = null, _logOffset = 0;
var _base = '/plugins/zfs.toolkit/scripts';

function toggle(id, show) { document.getElementById(id).style.display = show ? '' : 'none'; }

function formData() {
  var f = document.getElementById('settings-form');
  var bools = ['dry_run','cleanup','replace_spaces','send_notifications',
               'should_process_containers','should_process_vms',
               'cron_enabled'];
  var d = {};
  bools.forEach(function(n) {
    d[n] = (f.elements[n] && f.elements[n].checked) ? 'yes' : 'no';
  });
  ['buffer_zone','validation_tolerance','appdata_pool','appdata_dataset',
   'vm_pool','vm_dataset','vm_forceshutdown_wait','extra_datasets',
   'cron_preset','cron_hour','cron_minute','cron_weekday','cron_custom'].forEach(function(n) {
    if (f.elements[n]) d[n] = f.elements[n].value;
  });
  return d;
}

function postForm(url, data) {
  var params = new URLSearchParams(data);
  if (typeof csrf_token !== 'undefined') params.append('csrf_token', csrf_token);
  return fetch(url, {
    method: 'POST',
    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
    body: params
  }).then(function(r) {
    return r.text().then(function(txt) {
      if (!txt || txt.trim() === '') {
        throw new Error('Empty response (HTTP ' + r.status + '). Check Unraid logs.');
      }
      try { return JSON.parse(txt); }
      catch(e) {
        throw new Error('Non-JSON (HTTP ' + r.status + '): ' + txt.slice(0,300));
      }
    });
  });
}


function updateDryBanner() {
  var cb = document.getElementById('dry_run');
  document.getElementById('dry-run-banner').style.display = (cb && cb.checked) ? 'block' : 'none';
}

function scanFolders() {
  document.getElementById('scan-spinner').style.display = 'inline';
  postForm(_base + '/scan_folders.php', formData())
  .then(function(res) {
    document.getElementById('scan-spinner').style.display = 'none';
    renderFolderTable(res);
  }).catch(function() {
    document.getElementById('scan-spinner').style.display = 'none';
    document.getElementById('folder-list').innerHTML = '<p style="color:var(--zdc-err);">Error scanning folders.</p>';
  });
}

function renderFolderTable(res) {
  var el = document.getElementById('folder-list');
  if (!res || !res.sources || !res.sources.length) {
    el.innerHTML = '<p style="color:var(--zdc-dim);font-size:13px;">No sources configured.</p>';
    return;
  }
  var html = '';
  res.sources.forEach(function(src) {
    html += '<p style="font-weight:600;color:var(--zdc-accent);margin:10px 0 5px">' + esc(src.path) + '</p>';
    if (src.error) { html += '<p style="color:var(--zdc-err);font-size:12px;margin:0 0 8px 8px;">' + esc(src.error) + '</p>'; return; }
    if (!src.entries || !src.entries.length) { html += '<p style="color:var(--zdc-dim);font-size:13px;margin:0 0 8px 8px;">No entries.</p>'; return; }
    html += '<table class="zdc-table"><thead><tr><th>Name</th><th>Type</th><th>Size</th></tr></thead><tbody>';
    src.entries.forEach(function(e) {
      var cls = e.type === 'dataset' ? 'tag-dataset' : 'tag-folder';
      var lbl = e.type === 'dataset' ? '✓ Dataset' : '→ Will convert';
      html += '<tr><td>' + esc(e.name) + '</td><td class="'+cls+'">' + lbl + '</td><td>' + esc(e.size) + '</td></tr>';
    });
    html += '</tbody></table>';
  });
  el.innerHTML = html;
}

function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function startConversion() {
  clearLog(); _logOffset = 0;
  document.getElementById('btn-start').disabled = true;
  document.getElementById('btn-stop').style.display = 'inline-block';
  setStatus('running','Running');
  appendLog('Starting conversion…\n','log-step');

  postForm(_base + '/start_conversion.php', formData())
  .then(function(res) {
    if (res.success) {
      _logFile = res.log_file;
      appendLog('Process started (PID ' + res.pid + ')\n','log-ok');
      startPoll();
    } else {
      appendLog('ERROR: ' + (res.error||'Could not start') + '\n','log-error');
      resetButtons(); setStatus('error','Error');
    }
  }).catch(function(e) {
    appendLog('ERROR: ' + e + '\n','log-error');
    resetButtons(); setStatus('error','Error');
  });
}

function stopConversion() {
  postForm(_base + '/get_status.php', {action:'stop'})
  .then(function(){ appendLog('\nStop requested.\n','log-warn'); });
}

function startPoll() { if (_poll) clearInterval(_poll); _poll = setInterval(pollStatus, 1500); }
function stopPoll()  { if (_poll) { clearInterval(_poll); _poll = null; } }

function pollStatus() {
  fetch(_base + '/get_status.php').then(function(r){ return r.json(); })
  .then(function(res) {
    if (_logFile) fetchLogs();
    if (res.current_folder) document.getElementById('progress-text').textContent = 'Processing: ' + res.current_folder;
    if (res.status === 'completed') { stopPoll(); setStatus('done','Done'); resetButtons(); scanFolders(); }
    else if (res.status === 'error') {
      stopPoll(); resetButtons();
      if (_logFile) {
        fetch(_base + '/get_logs.php?file=' + encodeURIComponent(_logFile) + '&offset=0')
        .then(function(r){ return r.json(); })
        .then(function(lg) {
          var text = (lg.lines || []).join('\n');
          if (text.indexOf('All folders are already datasets') !== -1 ||
              text.indexOf('Nothing to convert') !== -1) {
            setStatus('done','Done — nothing to convert');
          } else {
            setStatus('error','Error');
          }
          scanFolders();
        });
      } else {
        setStatus('error','Error'); scanFolders();
      }
    }
  });
}

function fetchLogs() {
  fetch(_base + '/get_logs.php?file=' + encodeURIComponent(_logFile) + '&offset=' + _logOffset)
  .then(function(r){ return r.json(); })
  .then(function(res) {
    if (res.lines && res.lines.length) {
      res.lines.forEach(function(line) {
        var cls = '';
        if (/ERROR|FAILED/.test(line))     cls = 'log-error';
        else if (/WARNING/.test(line))     cls = 'log-warn';
        else if (/^=== Step/.test(line))   cls = 'log-step';
        else if (/\bOK:/.test(line))       cls = 'log-ok';
        appendLog(line + '\n', cls);
      });
      _logOffset = res.next_offset;
    }
  });
}

function appendLog(text, cls) {
  var el = document.getElementById('log-viewer');
  var sp = document.createElement('span');
  if (cls) sp.className = cls;
  sp.textContent = text;
  el.appendChild(sp);
  el.scrollTop = el.scrollHeight;
}
function clearLog() { document.getElementById('log-viewer').innerHTML = ''; }
function setStatus(type, label) {
  var el = document.getElementById('status-badge');
  el.className = 'badge-' + type;
  el.textContent = label;
}
function resetButtons() {
  document.getElementById('btn-start').disabled = false;
  document.getElementById('btn-stop').style.display = 'none';
  document.getElementById('progress-text').textContent = '';
}

function toggleCronDetails() {
  var en = document.getElementById('cron_enabled').checked;
  document.getElementById('cron-details').style.display = en ? '' : 'none';
  if (en) updateCronPreview();
}

function updateCronPreview() {
  var preset  = document.getElementById('cron_preset').value;
  var hour    = parseInt(document.getElementById('cron_hour').value)   || 0;
  var minute  = parseInt(document.getElementById('cron_minute').value) || 0;
  var weekday = parseInt(document.getElementById('cron_weekday').value);
  var custom  = document.getElementById('cron_custom').value.trim();

  var timeRow    = document.getElementById('cron-time-row');
  var weekRow    = document.getElementById('cron-weekday-row');
  var customRow  = document.getElementById('cron-custom-row');
  timeRow.style.display   = (preset === 'custom' || preset === 'hourly') ? 'none' : '';
  weekRow.style.display   = preset === 'weekly'  ? '' : 'none';
  customRow.style.display = preset === 'custom'  ? '' : 'none';

  var days = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];
  var pad  = function(n){ return n < 10 ? '0'+n : n; };
  var expr, label;

  switch (preset) {
    case 'hourly':
      expr = '0 * * * *';
      label = 'Every hour at :00';
      break;
    case '6hourly':
      expr = pad(minute) + ' */6 * * *';
      label = 'Every 6 hours at :' + pad(minute);
      break;
    case 'daily':
      expr = pad(minute) + ' ' + hour + ' * * *';
      label = 'Daily at ' + pad(hour) + ':' + pad(minute);
      break;
    case 'weekly':
      expr = pad(minute) + ' ' + hour + ' * * ' + weekday;
      label = 'Every ' + days[weekday] + ' at ' + pad(hour) + ':' + pad(minute);
      break;
    case 'custom':
      expr  = custom;
      label = expr;
      break;
  }

  zdcCronPreview('cron-preview', 'cron-hint', expr, preset === 'custom');
  if (label !== expr && validCronExpr(expr)) {
    document.getElementById('cron-preview').textContent = expr + '  (' + label + ')';
  }
}

function loadCronStatus() {
  fetch(_base + '/get_cron_status.php')
  .then(function(r){ return r.json(); })
  .then(function(res) {
    var el = document.getElementById('cron-active');
    if (res.active) {
      el.style.color = 'var(--zdc-ok)';
      el.textContent = res.entry;
    } else {
      el.style.color = 'var(--zdc-warn)';
      el.textContent = 'Not installed in crontab';
    }
  }).catch(function() {
    document.getElementById('cron-active').textContent = 'Could not read crontab';
  });
}

document.getElementById('dry_run').addEventListener('change', updateDryBanner);
updateDryBanner();
updateCronPreview();
loadCronStatus();

document.getElementById('settings-form').addEventListener('change', _triggerAutoSave);
document.getElementById('settings-form').addEventListener('input',  _triggerAutoSave);

document.getElementById('snapshots_enabled').addEventListener('change', _triggerAutoSave);
var _snapPanelEl = document.getElementById('tab-snapshots');
if (_snapPanelEl) {
  _snapPanelEl.addEventListener('change', _triggerAutoSave);
  _snapPanelEl.addEventListener('input',  _triggerAutoSave);
}
fetch(_base + '/get_status.php').then(function(r){ return r.json(); })
.then(function(res) {
  // Show the last run's log on load. Without this the viewer only ever had
  // content while a conversion happened to be running.
  if (res.log_file && res.status !== 'running') {
    _logFile = res.log_file;
    _logOffset = 0;
    appendLog('--- log of the last run'
      + (res.log_time ? ', ' + res.log_time : '') + ' ---\n', 'log-step');
    fetchLogs();
    if (res.status === 'completed')     setStatus('done', 'Done');
    else if (res.status === 'error')    setStatus('error', 'Error');
    else if (res.status === 'stopped')  setStatus('error', 'Stopped');
  } else if (!res.log_file && res.status !== 'running') {
    appendLog('No conversion has run yet. Output appears here while one is running.\n', 'log-warn');
  }

  if (res.status === 'running') {
    _logFile = res.log_file;
    setStatus('running','Running');
    document.getElementById('btn-start').disabled = true;
    document.getElementById('btn-stop').style.display = 'inline-block';
    startPoll();
  }
});

var _snapDatasets = [];  // in-memory dataset list

var _autoSaveTimer = null;

var _saveWarnings = null;

function _showAutoStatus(msg, color, fadeMs) {
  var el = document.getElementById('autosave-status');
  if (!el) return;
  el.textContent = msg;
  el.style.color = color || 'var(--zdc-dim)';
  el.style.opacity = '1';
  if (fadeMs) setTimeout(function() { el.style.opacity = '0'; }, fadeMs);
}

function _triggerAutoSave() {
  clearTimeout(_autoSaveTimer);
  _autoSaveTimer = setTimeout(_doAutoSave, 800);
}

function _doAutoSave() {
  _showAutoStatus('Saving\u2026', 'var(--zdc-dim)', 0);

  var d = formData();
  d['snapshots_enabled'] = document.getElementById('snapshots_enabled').checked ? 'yes' : 'no';
  d['send_enabled']   = document.getElementById('send_enabled').checked ? 'yes' : 'no';
  d['show_in_menu']   = document.getElementById('show_in_menu').checked ? 'yes' : 'no';
  ['snap_hourly','snap_daily','snap_weekly','snap_monthly','snap_yearly',
   'snap_frequent','snap_daily_hour','snap_min_free_pct','snap_free_target',
   'snap_age_hourly','snap_age_daily','snap_age_weekly','snap_age_monthly',
   'snap_age_yearly','snap_age_frequent',
   'send_schedule_preset','send_schedule_hour','send_schedule_custom'].forEach(function(n) {
    var el = document.querySelector('[name="'+n+'"]');
    if (el) d[n] = el.value;
  });
  var presetEl = document.getElementById('snap_schedule_preset');
  if (presetEl) d['snap_schedule_preset'] = presetEl.value;
  var customEl = document.getElementById('snap_schedule_custom');
  if (customEl) d['snap_schedule_custom'] = customEl.value;

  postForm(_base + '/save_settings.php', d)
  .then(function(res) {
    if (!res.success) throw new Error(res.error || 'save_settings failed');
    _saveWarnings = (res.warnings && res.warnings.length) ? res.warnings : null;
    updateDryBanner();
    setTimeout(loadCronStatus, 500);
    setTimeout(zdcRefreshStatus, 700);
    if (document.getElementById('snapshots_enabled').checked) setTimeout(loadSnapshotStatus, 500);
    var params = new URLSearchParams({datasets_json: JSON.stringify({datasets: _snapDatasets})});
    if (typeof csrf_token !== 'undefined') params.append('csrf_token', csrf_token);
    return fetch(_base + '/save_snapshot_config.php', {
      method: 'POST',
      headers: {'Content-Type': 'application/x-www-form-urlencoded'},
      body: params
    }).then(function(r) { return r.json(); });
  })
  .then(function(res2) {
    if (res2.ok) {
      if (_saveWarnings) {
        _showAutoStatus('\u26a0 ' + _saveWarnings[0], 'var(--zdc-warn)', 0);
      } else {
        _showAutoStatus('\u2713 Saved', 'var(--zdc-ok)', 2000);
      }
    } else {
      _showAutoStatus('\u2717 ' + (res2.error || 'Error'), 'var(--zdc-err)', 0);
    }
  })
  .catch(function() {
    _showAutoStatus('\u2717 Save error', 'var(--zdc-err)', 0);
  });
}

var ZDC_TABS = ['convert','snapshots','browse','replicate'];

function zdcTab(name) {
  if (ZDC_TABS.indexOf(name) === -1) name = ZDC_TABS[0];
  ZDC_TABS.forEach(function(t) {
    var panel = document.getElementById('tab-' + t);
    var btn   = document.getElementById('tabbtn-' + t);
    if (panel) panel.hidden = (t !== name);
    if (btn)   btn.className = 'zdc-tab' + (t === name ? ' active' : '');
  });
  try { localStorage.setItem('zdc-tab', name); } catch (e) {}

  // Load what a tab needs when it becomes visible, instead of leaving the
  // user to find a Refresh button.
  if (name === 'snapshots' && !_mgrLast) loadSnapManager();
  if (name === 'replicate') loadSendStatus();
  zdcLayout();
}

/* Unraid's theme is a server setting, not an OS preference, so
   prefers-color-scheme is the wrong signal. Read the background Unraid
   actually renders and decide from its luminance - that also covers the
   azure and gray themes. */
function zdcApplyTheme() {
  var wrap = document.querySelector('.zdc-wrap');
  if (!wrap) return;
  var el = document.body, bg = '';
  while (el && (!bg || bg === 'rgba(0, 0, 0, 0)' || bg === 'transparent')) {
    bg = getComputedStyle(el).backgroundColor;
    el = el.parentElement;
  }
  var m = /(\d+)\D+(\d+)\D+(\d+)/.exec(bg || '');
  var lum = m ? (0.2126 * +m[1] + 0.7152 * +m[2] + 0.0722 * +m[3]) / 255 : 0;
  wrap.classList.toggle('zdc-light', lum > 0.5);
}

function zdcRestoreTab() {
  var saved = null;
  try { saved = localStorage.getItem('zdc-tab'); } catch (e) {}
  zdcTab(saved || 'convert');
}

var _zdcSnap = null, _zdcConv = null;

function zdcRenderStatus() {
  var el = document.getElementById('zdc-status');
  if (!el) return;
  var snap = _zdcSnap, conv = _zdcConv, pills = [];

  function pill(state, label, value, extra) {
    pills.push('<span class="zdc-pill' + (state ? ' zdc-' + state : '') + '"><span class="dot"></span>'
      + esc(label) + ' <b>' + esc(value) + '</b>' + (extra || '') + '</span>');
  }

  var dry = document.getElementById('dry_run');
  if (dry && dry.checked) pill('warn', 'Mode', 'Dry run');

  if (snap) {
    if (!snap.active)            pill('warn', 'Snapshots', 'not scheduled');
    else if (!snap.cron_healthy) pill('warn', 'Snapshots', 'cron not live');
    else                         pill('ok', 'Snapshots', String(snap.snapshot_count));

    if (snap.last_run) {
      var r = snap.run || {};
      pill(r.errors > 0 ? 'err' : '', 'Last run', snap.last_run
        + (typeof r.created !== 'undefined'
            ? '  (+' + r.created + ' / -' + r.pruned + (r.errors > 0 ? ' / ' + r.errors + ' err' : '') + ')'
            : ''));
    }
    (snap.pools || []).forEach(function(pl) {
      var cls = pl.capacity >= 90 ? 'err' : (pl.capacity >= 80 ? 'warn' : 'ok');
      pill(cls, 'Pool ' + pl.name, pl.capacity + '%',
        '<span class="zdc-meter"><i style="width:' + Math.min(100, pl.capacity)
        + '%;background:var(--zdc-' + cls + ')"></i></span>');
    });
  }

  if (conv && conv.active) pill(conv.cron_healthy ? 'ok' : 'warn', 'Auto-convert', 'scheduled');
  el.innerHTML = pills.join('');
}

function zdcRefreshStatus() {
  fetch(_base + '/get_snapshot_status.php').then(function(r) { return r.json(); })
    .then(function(res) { _zdcSnap = res; zdcRenderStatus(); }).catch(function() {});
  fetch(_base + '/get_cron_status.php').then(function(r) { return r.json(); })
    .then(function(res) { _zdcConv = res; zdcRenderStatus(); }).catch(function() {});
}

/* The menu entry is rendered server-side from the page header, so the change
   only becomes visible after a reload. Say so instead of leaving the user
   wondering why nothing happened. */
function onMenuToggle() {
  var on = document.getElementById('show_in_menu').checked;
  var el = document.getElementById('menu-hint');
  _triggerAutoSave();
  if (!el) return;
  el.textContent = on ? 'reload the page to see the entry'
                      : 'reload the page, the entry will be gone';
  clearTimeout(window._menuHintTimer);
  window._menuHintTimer = setTimeout(function() { el.textContent = ''; }, 8000);
}

/* Masonry: each card spans as many 8px rows as its own height needs, so the
   next card in that column starts right below it. Re-run whenever a card
   changes size (a toggle opening a section) or the window resizes. */
var _zdcRO = null;

function zdcLayout() {
  document.querySelectorAll('.zdc-cols').forEach(function(grid) {
    if (!grid.offsetParent && grid.closest('.zdc-panel[hidden]')) return;
    var cs   = getComputedStyle(grid);
    var row  = parseFloat(cs.gridAutoRows) || 4;
    var gapY = parseFloat(cs.rowGap) || 0;
    grid.classList.add('zdc-measured');
    grid.querySelectorAll(':scope > .zdc-card').forEach(function(card) {
      var mb = parseFloat(getComputedStyle(card).marginBottom) || 0;
      var h  = card.getBoundingClientRect().height + mb;
      card.style.gridRowEnd = 'span ' + Math.max(1, Math.ceil((h + gapY) / (row + gapY)));
    });
  });
}

function zdcWatchLayout() {
  zdcLayout();
  if (window.ResizeObserver && !_zdcRO) {
    _zdcRO = new ResizeObserver(function() { zdcLayout(); });
    document.querySelectorAll('.zdc-cols > .zdc-card').forEach(function(c) { _zdcRO.observe(c); });
  }
  window.addEventListener('resize', zdcLayout);
}

function toggleSnapDetails() {
  var en = document.getElementById('snapshots_enabled').checked;
  document.querySelectorAll('.snap-gated').forEach(function(el) {
    el.style.display = en ? '' : 'none';
  });
  document.getElementById('snap-run-btn').disabled = !en;
  if (en) { updateSnapPreview(); loadSnapshotStatus(); }
  zdcLayout();
}

function updateSnapPreview() {
  var preset = document.getElementById('snap_schedule_preset').value;
  document.getElementById('snap-custom-row').style.display = preset === 'custom' ? '' : 'none';
  var expr, custom = false;
  switch (preset) {
    case '5min':   expr = '*/5 * * * *';  break;
    case '15min':  expr = '*/15 * * * *'; break;
    case '30min':  expr = '*/30 * * * *'; break;
    case 'hourly': expr = '0 * * * *';    break;
    case 'custom': expr = document.getElementById('snap_schedule_custom').value.trim(); custom = true; break;
    default:       expr = '*/15 * * * *';
  }
  zdcCronPreview('snap-cron-preview', 'snap-cron-hint', expr, custom);
}

// Says what is wrong with a cron expression, or '' when nothing is. Digits and
// the four operators only - no month or weekday names. Numbers are unambiguous,
// and refusing names is what makes it possible to check each field against its
// own range instead of only its shape: "70 * * * *" has five fields and legal
// characters, and never runs.
//
// Kept in step with zdc_valid_cron in zdc_common.sh and cronProblem in
// save_settings.php; tests/run.sh compares all three against one table.
function cronProblem(expr) {
  // Inside the function on purpose. As a top-level var it was hoisted but not
  // yet assigned when updateCronPreview() runs during page setup further up the
  // script, so the first call read CRON_FIELDS[0] off undefined and took the
  // whole init with it - tabs, layout and the dataset list included.
  var CRON_FIELDS = [
    {name: 'Minute',       lo: 0, hi: 59},
    {name: 'Hour',         lo: 0, hi: 23},
    {name: 'Day of month', lo: 1, hi: 31},
    {name: 'Month',        lo: 1, hi: 12},
    {name: 'Weekday',      lo: 0, hi: 7}
  ];
  expr = (expr || '').trim();
  if (!expr) return 'No cron expression entered.';
  if (/[;&|$`()<>\r\n]/.test(expr)) {
    return 'Contains characters that could be read as a shell command.';
  }
  if (!/^[0-9*\/,\s-]+$/.test(expr)) {
    return 'Only digits and * / , - are allowed. Use numbers for weekdays and months.';
  }
  var fields = expr.split(/\s+/);
  if (fields.length !== 5) {
    return 'A cron expression has 5 fields (minute hour day month weekday), this one has '
         + fields.length + '.';
  }
  for (var i = 0; i < 5; i++) {
    var f = CRON_FIELDS[i];
    var parts = fields[i].split(',');
    for (var j = 0; j < parts.length; j++) {
      var part = parts[j];
      if (part === '') return f.name + ': empty value in the list.';
      var slash = part.indexOf('/');
      if (slash !== -1) {
        var step = part.slice(slash + 1);
        part = part.slice(0, slash);
        if (!/^[0-9]+$/.test(step) || +step < 1 || +step > f.hi) {
          return f.name + ': step must be 1-' + f.hi + '.';
        }
      }
      if (part === '*') continue;
      var dash = part.indexOf('-');
      if (dash !== -1) {
        var a = part.slice(0, dash), b = part.slice(dash + 1);
        if (!/^[0-9]+$/.test(a) || !/^[0-9]+$/.test(b)) {
          return f.name + ': range must be two numbers.';
        }
        if (+a < f.lo || +b > f.hi || +a > +b) {
          return f.name + ': range must run upwards inside ' + f.lo + '-' + f.hi + '.';
        }
        continue;
      }
      if (!/^[0-9]+$/.test(part) || +part < f.lo || +part > f.hi) {
        return f.name + ' must be ' + f.lo + '-' + f.hi + ', got "' + part + '".';
      }
    }
  }
  return '';
}

function validCronExpr(expr) { return cronProblem(expr) === ''; }

function zdcCronPreview(previewId, hintId, expr, custom) {
  var why = custom ? cronProblem(expr) : '';
  var el  = document.getElementById(previewId);
  if (el) {
    el.textContent = expr || '(empty)';
    el.style.color = why ? 'var(--zdc-err)' : 'var(--zdc-accent)';
    el.title = '';
  }
  var hint = document.getElementById(hintId);
  if (hint) {
    hint.textContent = why
      ? why + ' It will not be installed - the schedule currently running stays as it is.'
      : '';
    hint.style.display = why ? 'block' : 'none';
  }
}

var _zfsDatasets = [];

function loadDatasetPickers() {
  fetch(_base + '/list_zfs_datasets.php')
  .then(function(r){ return r.json(); })
  .then(function(res) {
    var ds = res.datasets || [];
    _zfsDatasets = ds;

    // shared suggestion list for the replication destination
    var dl = document.getElementById('zdc-dataset-list');
    if (dl) dl.innerHTML = ds.map(function(d) { return '<option value="' + esc(d) + '">'; }).join('');
    if (_sendJobs.length) renderSendTable();
    var pickers = ['snap-ds-picker', 'browser-dataset', 'mgr-dataset']
      .map(function(id) { return document.getElementById(id); })
      .filter(function(el) { return el; });
    pickers.forEach(function(sel) {
      var cur = sel.value;
      while (sel.options.length > 1) sel.remove(1);
      ds.forEach(function(d) {
        var o = new Option(d, d);
        sel.add(o);
      });
      if (cur) sel.value = cur;
    });
  });
}

function loadSnapDatasetConfig() {
  fetch(_base + '/get_snap_datasets.php')
  .then(function(r){ return r.json(); })
  .then(function(data) {
    _snapDatasets = (data && data.datasets) ? data.datasets : [];
    renderSnapTable();
  }).catch(function() {
    _snapDatasets = [];
    renderSnapTable();
  });
}

function addSnapDataset() {
  var picker = document.getElementById('snap-ds-picker');
  var name = picker.value;
  if (!name) return;
  if (_snapDatasets.find(function(d){ return d.name === name; })) {
    picker.value = ''; return;
  }
  _snapDatasets.push({name:name, recursive:false, use_template:true,
                       hourly:'', daily:'', weekly:'', monthly:'', yearly:''});
  picker.value = '';
  renderSnapTable();
  _triggerAutoSave();
}

function removeSnapDataset(idx) {
  _snapDatasets.splice(idx, 1);
  renderSnapTable();
  _triggerAutoSave();
}

function getGlobalRetention(field) {
  var el = document.querySelector('[name="snap_' + field + '"]');
  return el ? (el.value || '0') : '0';
}

function renderSnapTable() {
  var tbody = document.getElementById('snap-ds-tbody');
  if (!_snapDatasets.length) {
    tbody.innerHTML = '<tr><td colspan="9" style="color:var(--zdc-dim);text-align:center;padding:10px;">No datasets configured. Add one above.</td></tr>';
    return;
  }
  var html = '';
  _snapDatasets.forEach(function(ds, i) {
    var tpl = ds.use_template !== false;
    // One marker for the row rather than a placeholder in every field: with
    // only Hourly filled in, "off" in the other four is true but reads as if
    // the whole row were off.
    var rowOff = !tpl && ['hourly','daily','weekly','monthly','yearly']
      .every(function(f) { return !(parseInt(ds[f], 10) > 0); });
    html += '<tr>';
    html += '<td>' + esc(ds.name)
          + (rowOff ? ' <span style="color:var(--zdc-warn);font-size:11px;" '
                    + 'title="No retention set, so this dataset is not snapshotted">off</span>' : '')
          + '</td>';
    html += '<td><label class="zdc-toggle" style="margin:auto;"><input type="checkbox" onchange="snapDsField('+i+',\'recursive\',this.checked)" '+(ds.recursive?'checked':'')+'><span class="zdc-slider"></span></label></td>';
    html += '<td><label class="zdc-toggle" style="margin:auto;"><input type="checkbox" id="snap-tpl-'+i+'" onchange="snapToggleTpl('+i+',this.checked)" '+(tpl?'checked':'')+'><span class="zdc-slider"></span></label></td>';
    ['hourly','daily','weekly','monthly','yearly'].forEach(function(f) {
      var val = tpl ? getGlobalRetention(f) : (ds[f] !== undefined && ds[f] !== '' ? ds[f] : '');
      var attrs = tpl
        ? 'disabled title="Using global template value" style="opacity:.55;width:52px;text-align:center;padding:2px 4px;font-size:12px;background:var(--zdc-sunken);"'
        : 'title="Empty = no ' + f + ' snapshots for this dataset"'
          + ' oninput="snapDsField('+i+',\''+f+'\',this.value)" style="width:52px;text-align:center;padding:2px 4px;font-size:12px;"';
      html += '<td><input type="number" min="0" max="999" ' + attrs + ' value="' + esc(String(val)) + '"></td>';
    });
    html += '<td><button type="button" class="btn-danger" style="padding:2px 8px;font-size:11px;" onclick="removeSnapDataset('+i+')">✕</button></td>';
    html += '</tr>';
  });
  tbody.innerHTML = html;
}

function snapDsField(idx, field, val) {
  _snapDatasets[idx][field] = val;
  _triggerAutoSave();
}

function snapToggleTpl(idx, checked) {
  _snapDatasets[idx].use_template = checked;
  renderSnapTable();
  _triggerAutoSave();
}

function loadSnapshotStatus() {
  fetch(_base + '/get_snapshot_status.php')
  .then(function(r){ return r.json(); })
  .then(function(res) {
    var badge = document.getElementById('snap-cron-badge');
    if (res.active && res.cron_healthy) {
      badge.style.color = 'var(--zdc-ok)';
      badge.textContent = res.entry.split(' ').slice(0,5).join(' ') + ' ✓';
    } else if (res.active) {
      badge.style.color = 'var(--zdc-warn)';
      badge.textContent = res.entry.split(' ').slice(0,5).join(' ') + ' (!)';
    } else {
      badge.style.color = 'var(--zdc-warn)';
      badge.textContent = 'Not installed';
    }

    var rEl = document.getElementById('snap-last-result');
    if (rEl) {
      var run = res.run || {};
      if (typeof run.created === 'undefined') {
        rEl.textContent = '—';
      } else {
        rEl.textContent = run.created + ' created, ' + run.pruned + ' pruned'
                        + (run.errors > 0 ? ', ' + run.errors + ' error(s)' : '');
        rEl.style.color = run.errors > 0 ? 'var(--zdc-err)' : 'var(--zdc-dim)';
      }
    }

    var msgs = [];
    if (res.active && !res.cron_healthy) {
      msgs.push('⚠ The schedule is written to ' + (res.cron_source || 'cron')
              + ' but is not in the live crontab. Run "update_cron" or restart the array.');
    }
    if (res.run && res.run.errors > 0 && res.run.last_error) {
      msgs.push('⚠ Last error: ' + res.run.last_error);
    }
    (res.pools || []).forEach(function(p) {
      if (p.capacity >= 90) msgs.push('⚠ Pool "' + p.name + '" is ' + p.capacity + '% full.');
    });
    var hEl = document.getElementById('snap-health');
    if (hEl) {
      if (msgs.length) {
        hEl.style.display = '';
        hEl.style.color = 'var(--zdc-warn)';
        hEl.textContent = msgs.join('  ');
      } else {
        hEl.style.display = 'none';
      }
    }
    document.getElementById('snap-total-count').textContent = res.snapshot_count;
    document.getElementById('snap-last-run').textContent = res.last_run || '—';

    var dc = (typeof res.dataset_count !== 'undefined') ? res.dataset_count : '?';
    var dcEl = document.getElementById('snap-dataset-count');
    if (dcEl) dcEl.textContent = dc;
    var warnEl = document.getElementById('snap-no-datasets-warn');
    if (warnEl) warnEl.style.display = (dc === 0) ? '' : 'none';

    var logSec = document.getElementById('snap-log-section');
    if (logSec && logSec.style.display !== 'none' && res.recent_log && res.recent_log.length) {
      renderSnapLog(res.recent_log);
    }
  }).catch(function(){});
}

function runSnapshotNow(dry) {
  var resultEl = document.getElementById('snap-save-result');

  if (_snapDatasets.length === 0) {
    var warnEl = document.getElementById('snap-no-datasets-warn');
    if (warnEl) warnEl.style.display = '';
    resultEl.style.color = 'var(--zdc-warn)';
    resultEl.textContent = 'No datasets configured yet.';
    setTimeout(function(){ resultEl.textContent = ''; }, 5000);
    return;
  }

  var btn = document.getElementById(dry ? 'snap-dry-btn' : 'snap-run-btn');
  var label = dry ? 'Dry Run' : 'Run Now';
  btn.disabled = true; btn.textContent = 'Running…';

  var logSec = document.getElementById('snap-log-section');
  logSec.style.display = '';
  document.getElementById('snap-log-viewer').innerHTML =
    '<span style="color:var(--zdc-dim);">Starting snapshot_manager.sh ' + (dry ? '--now --dry-run' : '--now') + ' …\n</span>';

  postForm(_base + '/run_snapshot_manual.php', dry ? {dry: '1'} : {})
  .then(function(res) {
    if (res.ok) {
      resultEl.style.color = 'var(--zdc-ok)';
      resultEl.textContent = 'Started (PID ' + res.pid + ')';
      setTimeout(loadSnapshotStatus, 2000);
      setTimeout(loadSnapshotStatus, 5000);
    } else {
      resultEl.style.color = 'var(--zdc-err)';
      resultEl.textContent = res.error || 'Failed';
    }
    setTimeout(function(){
      btn.disabled = false; btn.textContent = label;
      resultEl.textContent = '';
    }, 6000);
  }).catch(function(e){
    btn.disabled = false; btn.textContent = label;
    resultEl.style.color = 'var(--zdc-err)';
    resultEl.textContent = 'Error: ' + e;
  });
}

// The label says "Server time", so it has to move. It ticks from the server's
// clock rather than the browser's: the epoch is rendered shifted by the
// server's UTC offset, so formatting it as UTC yields the server's local time
// and any difference between the two machines' clocks stays out of it.
function startServerClock() {
  var el = document.getElementById('server-time');
  if (!el) return;
  var epoch = parseInt(el.getAttribute('data-epoch'), 10);
  if (!epoch) return;
  var suffix   = el.getAttribute('data-suffix') || '';
  var loadedAt = Date.now();
  var pad = function(n) { return n < 10 ? '0' + n : String(n); };
  setInterval(function() {
    var t = new Date(epoch * 1000 + (Date.now() - loadedAt));
    el.textContent = pad(t.getUTCHours()) + ':' + pad(t.getUTCMinutes()) + ':'
                   + pad(t.getUTCSeconds()) + suffix;
  }, 1000);
}

function toggleSnapLog() {
  var sec = document.getElementById('snap-log-section');
  var open = sec.style.display !== 'none';
  sec.style.display = open ? 'none' : '';
  var btn = document.getElementById('snap-log-toggle-btn');
  if (btn) btn.textContent = open ? 'View Log' : 'Hide Log';
  if (!open) loadSnapshotStatus();  // refresh log content when opening
}

function clearSnapLog() {
  document.getElementById('snap-log-viewer').innerHTML = '';
}

function renderSnapLog(lines) {
  var el = document.getElementById('snap-log-viewer');
  el.innerHTML = '';
  lines.forEach(function(line) {
    var sp = document.createElement('span');
    if (/ERROR/.test(line))   sp.style.color = 'var(--zdc-err)';
    else if (/WARNING/.test(line)) sp.style.color = 'var(--zdc-warn)';
    else if (/\bOK:/.test(line))   sp.style.color = 'var(--zdc-ok)';
    sp.textContent = line + '\n';
    el.appendChild(sp);
  });
  el.scrollTop = el.scrollHeight;
}

var browserPath = '/';

function loadBrowserSnapshots() {
  var dataset = document.getElementById('browser-dataset').value;
  var sel = document.getElementById('browser-snapshot');
  while (sel.options.length > 1) sel.remove(1);
  document.getElementById('snap-file-list').innerHTML =
    '<p style="color:var(--zdc-dim);font-size:13px;">Select a dataset and snapshot above, then click Browse.</p>';
  document.getElementById('snap-breadcrumb').innerHTML = '';
  if (!dataset) return;

  fetch(_base + '/snapshot_browse.php?action=list_snapshots&dataset=' + encodeURIComponent(dataset))
  .then(function(r) {
    if (!r.ok) throw new Error('HTTP ' + r.status);
    return r.text().then(function(txt) {
      try { return JSON.parse(txt); }
      catch(e) { throw new Error('Non-JSON response: ' + txt.slice(0, 200)); }
    });
  })
  .then(function(res) {
    if (!res.ok) {
      document.getElementById('snap-file-list').innerHTML =
        '<p style="color:var(--zdc-err);">Error loading snapshots: ' + esc(res.error) + '</p>';
      return;
    }
    if (!res.snapshots.length) {
      sel.add(new Option('No snapshots found', ''));
      return;
    }
    res.snapshots.forEach(function(s) { sel.add(new Option(s, s)); });

    var dsel = document.getElementById('diff-target');
    if (dsel) {
      while (dsel.options.length > 1) dsel.remove(1);
      res.snapshots.forEach(function(s) { dsel.add(new Option(s, s)); });
    }
  }).catch(function(e) {
    document.getElementById('snap-file-list').innerHTML =
      '<p style="color:var(--zdc-err);">Could not load snapshots: ' + esc(String(e)) + '</p>';
  });
}

function browserBrowse(path) {
  var dataset  = document.getElementById('browser-dataset').value;
  var snapshot = document.getElementById('browser-snapshot').value;
  var fileList = document.getElementById('snap-file-list');

  if (!dataset || !snapshot) {
    fileList.innerHTML = '<p style="color:var(--zdc-warn);font-size:13px;">Please select a dataset and snapshot first.</p>';
    return;
  }

  browserPath = path || '/';
  fileList.innerHTML = '<p style="color:var(--zdc-dim);font-size:13px;">Loading…</p>';

  var params = new URLSearchParams({
    action:   'browse',
    dataset:  dataset,
    snapshot: snapshot,
    path:     browserPath
  });
  if (typeof csrf_token !== 'undefined') params.append('csrf_token', csrf_token);

  fetch(_base + '/snapshot_browse.php', {
    method:  'POST',
    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
    body:    params
  })
  .then(function(r) {
    if (!r.ok) throw new Error('HTTP ' + r.status);
    return r.text().then(function(txt) {
      try { return JSON.parse(txt); }
      catch(e) { throw new Error('Non-JSON: ' + txt.slice(0, 300)); }
    });
  })
  .then(function(res) {
    if (!res.ok) {
      fileList.innerHTML = '<p style="color:var(--zdc-err);">Error: ' + esc(res.error) + '</p>';
      return;
    }
    renderBreadcrumb(res.crumbs);
    renderFileList(res.entries, dataset, snapshot, res.path);
  }).catch(function(e){
    fileList.innerHTML = '<p style="color:var(--zdc-err);">Request failed: ' + esc(String(e)) + '</p>';
  });
}

function renderBreadcrumb(crumbs) {
  var el = document.getElementById('snap-breadcrumb');
  var html = '';
  crumbs.forEach(function(c, i) {
    if (i > 0) html += '<span class="snap-crumb-sep">›</span>';
    html += '<span class="snap-crumb" onclick="browserBrowse(\''+c.path.replace(/'/g,"\\'")+'\')">' + esc(c.label) + '</span>';
  });
  el.innerHTML = html;
}

function renderFileList(entries, dataset, snapshot, currentPath) {
  var el = document.getElementById('snap-file-list');

  var html = '<table class="snap-file-table"><thead><tr>'
    + '<th style="width:24px;padding:5px 6px;text-align:center;"><input type="checkbox" id="snap-sel-all" onchange="snapSelectAll(this)" title="Select all"></th>'
    + '<th>Name</th><th>Size</th><th>Modified</th><th></th>'
    + '</tr></thead><tbody>';

  // One level up, so you do not have to aim at the breadcrumb.
  if (currentPath && currentPath !== '/') {
    var parent = currentPath.replace(/\/[^\/]*$/, '') || '/';
    html += '<tr><td></td><td colspan="4" style="padding:4px 8px;">'
          + '<span class="snap-dir" onclick="browserBrowse(\'' + parent.replace(/'/g, "\\'") + '\')"'
          + ' title="Up to ' + esc(parent) + '">\u21b0 ..</span></td></tr>';
  }

  // Render the empty case inside the table, otherwise the ".." row above is
  // skipped along with it and there is no way back out of an empty folder.
  if (!entries.length) {
    html += '<tr><td></td><td colspan="4" style="padding:6px 8px;color:var(--zdc-dim);">'
          + 'Empty directory.</td></tr>';
  }

  entries.forEach(function(e) {
    var isDir = e.type === 'dir';
    var nameCell;
    if (isDir) {
      var subPath = (currentPath === '/' ? '' : currentPath) + '/' + e.name;
      nameCell = '<span class="snap-dir" onclick="browserBrowse(\''+subPath.replace(/'/g,"\\'")+'\')">'
               + '📁 ' + esc(e.name) + '</span>';
    } else {
      nameCell = '<span class="snap-file">📄 ' + esc(e.name) + '</span>';
    }
    var srcPath = (currentPath === '/' ? '' : currentPath) + '/' + e.name;
    html += '<tr>'
      + '<td style="padding:4px 6px;text-align:center;"><input type="checkbox" class="snap-sel-cb" onchange="updateSelCount()" data-src="' + esc(srcPath) + '"></td>'
      + '<td>' + nameCell + '</td>'
      + '<td style="color:var(--zdc-dim)">' + esc(e.size) + '</td>'
      + '<td style="color:var(--zdc-dim);font-size:11px">' + esc(e.mtime) + '</td>'
      + '<td><button class="snap-restore-btn" onclick="restoreEntry(\''
        + dataset.replace(/'/g,"\\'") + '\',\''
        + snapshot.replace(/'/g,"\\'") + '\',\''
        + srcPath.replace(/'/g,"\\'") + '\')">Restore</button></td>'
      + '</tr>';
  });

  html += '</tbody></table>';
  el.innerHTML = html;
  updateSelCount();
}

function snapSelectAll(cb) {
  document.querySelectorAll('.snap-sel-cb').forEach(function(c) { c.checked = cb.checked; });
  updateSelCount();
}

function updateSelCount() {
  var boxes = document.querySelectorAll('.snap-sel-cb');
  var n = document.querySelectorAll('.snap-sel-cb:checked').length;
  // Keep the header box honest: after moving to another folder the rows are
  // rebuilt unchecked, and a box left ticked claims a selection that is gone.
  var all = document.getElementById('snap-sel-all');
  if (all) all.checked = (boxes.length > 0 && n === boxes.length);
  var el = document.getElementById('sel-count');
  if (el) el.textContent = n;
}

var _destPath = '/mnt';

function destHint(msg, color) {
  var el = document.getElementById('dest-hint');
  if (!el) return;
  el.style.color = color || 'var(--zdc-dim)';
  el.textContent = msg || '';
  if (msg) setTimeout(function() { if (el.textContent === msg) el.textContent = ''; }, 6000);
}

function destNativeAvailable() {
  return !!(window.jQuery && jQuery.fn && typeof jQuery.fn.fileTree === 'function');
}

function destShowNativeAt(path) {
  var input = document.getElementById('restore-dst');
  var $i = jQuery(input);
  var $t = $i.next('.fileTree');
  if (!$t.length) {
    $t = jQuery('<div class="textarea fileTree"></div>');
    $i.after($t);
  }
  $t.html('');
  $t.fileTree(
    {root: path.replace(/\/*$/, '/'), top: '/mnt/', filter: ['HIDE_FILES_FILTER'],
     match: '.*', allowBrowsing: true},
    function(file) {},
    function(folder) { input.value = String(folder).replace(/\/\/+/g, '/'); }
  );
  $t.show();
}

/* Enter in the destination field: point the tree at what was typed, so you
   can see where you are instead of guessing. A path that does not exist yet
   opens its parent, which is what you want when creating a new folder. */
function destGoTo() {
  var input = document.getElementById('restore-dst');
  var raw = input.value.trim();

  if (!raw) { destHint('empty = restore to the original location'); return; }
  if (raw.charAt(0) !== '/') { destHint('Path must start with /', 'var(--zdc-warn)'); return; }

  var path = raw.replace(/\/+$/, '') || '/';

  function open(p, note) {
    if (destNativeAvailable()) destShowNativeAt(p);
    else {
      document.getElementById('dest-picker').style.display = '';
      destBrowse(p);
    }
    destHint(note || '');
  }

  postForm(_base + '/snapshot_browse.php', {action: 'list_dirs', path: path})
  .then(function(res) {
    if (res.ok && res.path === path) { open(path); return; }

    var parent = path.replace(/\/[^/]+$/, '') || '/mnt';
    return postForm(_base + '/snapshot_browse.php', {action: 'list_dirs', path: parent})
      .then(function(res2) {
        if (res2.ok && res2.path === parent) {
          open(parent, 'does not exist yet \u2014 showing ' + parent + ', it will be created on restore');
        } else {
          destHint('Not a folder below /mnt: ' + path, 'var(--zdc-warn)');
        }
      });
  }).catch(function(e) { destHint('Error: ' + e, 'var(--zdc-err)'); });
}

function initDestPicker() {
  var input = document.getElementById('restore-dst');
  if (!input) return;
  input.addEventListener('keydown', function(e) {
    if (e.key === 'Enter' || e.keyCode === 13) { e.preventDefault(); destGoTo(); }
  });

  if (window.jQuery && jQuery.fn && typeof jQuery.fn.fileTreeAttach === 'function') {
    jQuery(input).fileTreeAttach();
    input.title = 'Click to browse, or type a path and press Enter to jump there';
  } else {
    document.getElementById('dest-browse-btn').style.display = '';
    input.placeholder = 'empty = original location';
  }
}

function toggleDestPicker() {
  var el = document.getElementById('dest-picker');
  var open = el.style.display !== 'none';
  el.style.display = open ? 'none' : '';
  if (!open) {
    var cur = document.getElementById('restore-dst').value.trim();
    destBrowse(cur.charAt(0) === '/' ? cur : '/mnt');
  }
}

function destBrowse(path) {
  var listEl = document.getElementById('dest-list');
  listEl.innerHTML = '<span style="color:var(--zdc-dim);">Loading\u2026</span>';

  postForm(_base + '/snapshot_browse.php', {action: 'list_dirs', path: path})
  .then(function(res) {
    if (!res.ok) {
      listEl.innerHTML = '<span style="color:var(--zdc-err);">' + esc(res.error || 'Error') + '</span>';
      return;
    }
    _destPath = res.path;
    document.getElementById('dest-current').textContent = res.path;
    document.getElementById('dest-note').textContent = res.writable ? '' : '\u26a0 not writable';

    var cr = '';
    res.crumbs.forEach(function(c, i) {
      if (i > 0) cr += '<span style="color:#555;"> \u203a </span>';
      cr += '<span class="snap-crumb" onclick="destBrowse(\'' + c.path.replace(/'/g, "\\'") + '\')">'
          + esc(c.label) + '</span>';
    });
    document.getElementById('dest-crumbs').innerHTML = cr;

    var html = '';
    if (res.path && res.path !== '/mnt') {
      var up = res.path.replace(/\/[^\/]*$/, '') || '/mnt';
      html += '<div class="snap-dir" style="padding:2px 0;" onclick="destBrowse(\''
            + up.replace(/'/g, "\\'") + '\')">\u21b0 ..</div>';
    }
    if (!res.dirs.length) {
      listEl.innerHTML = html + '<span style="color:var(--zdc-dim);">No subfolders here.</span>';
      return;
    }
    res.dirs.forEach(function(d) {
      html += '<div class="snap-dir" style="padding:2px 0;" onclick="destBrowse(\''
            + d.path.replace(/'/g, "\\'") + '\')">\ud83d\udcc1 ' + esc(d.name) + '</div>';
    });
    listEl.innerHTML = html;
  }).catch(function(e) {
    listEl.innerHTML = '<span style="color:var(--zdc-err);">' + esc(String(e)) + '</span>';
  });
}

function destUse() {
  var extra = document.getElementById('dest-newfolder').value.trim().replace(/^\/+|\/+$/g, '');
  var target = _destPath + (extra ? '/' + extra : '');
  document.getElementById('restore-dst').value = target;
  document.getElementById('dest-newfolder').value = '';
  toggleDestPicker();
}

function restoreEntry(dataset, snapshot, srcPath) {
  startRestore(dataset, snapshot, [srcPath]);
}

function restoreSelected() {
  var dataset  = document.getElementById('browser-dataset').value;
  var snapshot = document.getElementById('browser-snapshot').value;
  var resultEl = document.getElementById('restore-result');

  if (!dataset || !snapshot) {
    resultEl.style.color = 'var(--zdc-warn)';
    resultEl.textContent = 'Select a dataset and snapshot first.';
    setTimeout(function(){ resultEl.textContent=''; }, 3000);
    return;
  }
  var cbs = Array.prototype.slice.call(document.querySelectorAll('.snap-sel-cb:checked'));
  if (!cbs.length) {
    resultEl.style.color = 'var(--zdc-warn)';
    resultEl.textContent = 'Nothing selected.';
    setTimeout(function(){ resultEl.textContent=''; }, 3000);
    return;
  }
  startRestore(dataset, snapshot, cbs.map(function(c) { return c.dataset.src; }));
}

var _restorePoll = null;

function startRestore(dataset, snapshot, items) {
  var dst      = document.getElementById('restore-dst').value.trim();
  var resultEl = document.getElementById('restore-result');

  resultEl.style.color = 'var(--zdc-dim)';
  resultEl.textContent = 'Starting restore\u2026';

  var params = new URLSearchParams();
  params.append('action', 'restore_start');
  params.append('dataset', dataset);
  params.append('snapshot', snapshot);
  params.append('dst_path', dst);
  items.forEach(function(i) { params.append('items[]', i); });
  if (typeof csrf_token !== 'undefined') params.append('csrf_token', csrf_token);

  fetch(_base + '/snapshot_browse.php', {
    method: 'POST',
    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
    body: params
  }).then(function(r) { return r.json(); })
  .then(function(res) {
    if (!res.ok) {
      resultEl.style.color = 'var(--zdc-err)';
      resultEl.textContent = '\u2717 ' + (res.error || 'Failed');
      return;
    }
    pollRestore(res.job, res.total);
  }).catch(function(e) {
    resultEl.style.color = 'var(--zdc-err)';
    resultEl.textContent = '\u2717 Error: ' + e;
  });
}

function pollRestore(job, total) {
  var resultEl = document.getElementById('restore-result');
  if (_restorePoll) clearInterval(_restorePoll);

  function tick() {
    postForm(_base + '/snapshot_browse.php', {action: 'restore_status', job: job})
    .then(function(st) {
      if (!st.ok) return;
      if (st.state === 'starting') { resultEl.textContent = 'Starting\u2026'; return; }

      if (st.state === 'running') {
        resultEl.style.color = 'var(--zdc-dim)';
        resultEl.textContent = 'Restoring ' + (st.done + st.failed) + '/' + st.total
                             + (st.current ? ' \u2014 ' + st.current : '') + '\u2026';
        return;
      }

      clearInterval(_restorePoll); _restorePoll = null;
      if (st.state === 'done') {
        resultEl.style.color = 'var(--zdc-ok)';
        resultEl.textContent = '\u2713 ' + st.message;
      } else {
        resultEl.style.color = 'var(--zdc-err)';
        resultEl.textContent = '\u2717 ' + (st.message || st.state)
                             + ((st.log && st.log.length) ? ' \u2014 ' + st.log[st.log.length - 1] : '');
      }
      setTimeout(function() { resultEl.textContent = ''; }, 15000);
    }).catch(function(){});
  }

  _restorePoll = setInterval(tick, 1500);
  tick();
}

var _mgrSnaps = [];
var _mgrLast = null;
var _mgrVisible = [];
var _mgrSelected = {};

function mgrSelectedNames() {
  return Object.keys(_mgrSelected);
}

function mgrToggle(cb) {
  if (cb.checked) _mgrSelected[cb.getAttribute('data-name')] = true;
  else delete _mgrSelected[cb.getAttribute('data-name')];
  mgrSyncSelectAll();
  mgrUpdateSelCount();
}

function mgrSyncSelectAll() {
  var all = document.getElementById('mgr-all');
  if (!all) return;
  all.checked = _mgrVisible.length > 0 && _mgrVisible.every(function(s) {
    return _mgrSelected[s.name];
  });
}

function fmtBytes(b) {
  var u = ['B', 'KiB', 'MiB', 'GiB', 'TiB'], i = 0;
  b = Number(b) || 0;
  while (b >= 1024 && i < u.length - 1) { b /= 1024; i++; }
  return (i === 0 ? b : b.toFixed(b < 10 ? 2 : 1)) + ' ' + u[i];
}

function mgrHoldSelected(hold) {
  var names = mgrSelectedNames();
  if (!names.length) { mgrResult('Nothing selected.', 'var(--zdc-warn)'); return; }

  mgrResult((hold ? 'Holding ' : 'Releasing ') + names.length + ' snapshot(s)\u2026');

  var params = new URLSearchParams();
  params.append('action', hold ? 'hold' : 'release');
  names.forEach(function(n) { params.append('snapshots[]', n); });
  if (typeof csrf_token !== 'undefined') params.append('csrf_token', csrf_token);

  fetch(_base + '/snapshot_admin.php', {
    method: 'POST',
    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
    body: params
  }).then(function(r) { return r.json(); })
  .then(function(res) {
    if (!res.ok) { mgrResult(res.error || 'Failed', 'var(--zdc-err)'); return; }
    var msg = res.changed + (hold ? ' held' : ' released');
    if (res.failed) {
      var first = (res.results || []).filter(function(r) { return !r.ok; })[0];
      msg += ', ' + res.failed + ' failed' + (first ? ' (' + first.error + ')' : '');
    }
    mgrResult(msg, res.failed ? 'var(--zdc-warn)' : 'var(--zdc-ok)');
    loadSnapManager();
  }).catch(function(e) { mgrResult('Error: ' + e, 'var(--zdc-err)'); });
}

function loadSnapManager() {
  var tbody = document.getElementById('mgr-tbody');
  tbody.innerHTML = '<tr><td colspan="8" style="color:var(--zdc-dim);padding:10px;">Loading…</td></tr>';

  postForm(_base + '/snapshot_admin.php', {
    action: 'list',
    dataset: document.getElementById('mgr-dataset').value,
    only_auto: document.getElementById('mgr-only-auto').checked ? '1' : '0'
  }).then(function(res) {
    if (!res.ok) {
      tbody.innerHTML = '<tr><td colspan="8" style="color:var(--zdc-err);padding:10px;">' + esc(res.error || 'Error') + '</td></tr>';
      return;
    }
    _mgrSnaps = res.snapshots || [];
    _mgrLast = res;
    var alive = {};
    _mgrSnaps.forEach(function(s) { alive[s.name] = true; });
    Object.keys(_mgrSelected).forEach(function(n) { if (!alive[n]) delete _mgrSelected[n]; });
    renderSnapManager();
  }).catch(function(e) {
    tbody.innerHTML = '<tr><td colspan="8" style="color:var(--zdc-err);padding:10px;">' + esc(String(e)) + '</td></tr>';
  });
}

function mgrFilterMatcher() {
  var p = document.getElementById('mgr-filter').value.trim();
  if (!p) return null;
  var rx = p.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.');
  try { return new RegExp(rx, 'i'); } catch (e) { return null; }
}

function renderSnapManager() {
  var res = _mgrLast || {count: 0, total_h: '0 B', datasets: []};
  var tbody = document.getElementById('mgr-tbody');
  var rx = mgrFilterMatcher();
  var shown = rx ? _mgrSnaps.filter(function(s) { return rx.test(s.name); }) : _mgrSnaps;
  _mgrVisible = shown;

  var worst = (res.datasets || []).slice().sort(function(a, b) {
    return b.usedbysnapshots - a.usedbysnapshots;
  }).filter(function(d) { return d.usedbysnapshots > 0; }).slice(0, 4);

  var sum = document.getElementById('mgr-summary');
  var shownBytes = shown.reduce(function(a, s) { return a + (s.used || 0); }, 0);
  var parts = [rx
    ? shown.length + ' of ' + _mgrSnaps.length + ' snapshot(s) shown, ' + fmtBytes(shownBytes)
    : res.count + ' snapshot(s), ' + res.total_h + ' total'];
  worst.forEach(function(d) { parts.push(d.name + ': ' + d.snapused_h + ' in snapshots'); });
  sum.textContent = parts.join('   •   ');

  if (!shown.length) {
    tbody.innerHTML = '<tr><td colspan="8" style="color:var(--zdc-dim);padding:10px;">'
      + (_mgrSnaps.length ? 'No snapshot matches this filter.' : 'No snapshots found.') + '</td></tr>';
    mgrSyncSelectAll();
    mgrUpdateSelCount();
    return;
  }

  var html = '';
  shown.forEach(function(s) {
    var i = _mgrSnaps.indexOf(s);
    html += '<tr>'
      + '<td><input type="checkbox" class="mgr-cb" data-name="' + esc(s.name) + '"'
        + (_mgrSelected[s.name] ? ' checked' : '') + ' onchange="mgrToggle(this)"></td>'
      + '<td style="text-align:left;font-family:monospace;font-size:11px;word-break:break-all;">' + esc(s.name) + '</td>'
      + '<td>' + (s.checkpoint
            ? '<span style="color:var(--zdc-accent);" title="Base of a replication chain">replication</span>'
            : (s.type ? esc(s.type) : (s.managed ? '—' : '<span style="color:var(--zdc-dim);">external</span>'))) + '</td>'
      + '<td style="text-align:right;' + (s.used > 1073741824 ? 'color:var(--zdc-warn);font-weight:600;' : '') + '">' + esc(s.used_h) + '</td>'
      + '<td style="text-align:right;color:var(--zdc-dim);">' + esc(s.refer_h) + '</td>'
      + '<td style="white-space:nowrap;">' + esc(s.created_h) + '</td>'
      + '<td><button type="button" class="btn-secondary" style="padding:1px 7px;font-size:11px;" '
        + 'onclick="mgrHold(' + i + ')" title="' + (s.held ? 'Release the hold' : 'Hold: protect from automatic pruning') + '">'
        + (s.held ? '🔒' : '○') + '</button></td>'
      + '<td style="white-space:nowrap;">'
        + '<button type="button" class="btn-secondary" style="padding:1px 7px;font-size:11px;" onclick="mgrRollback(' + i + ')" '
        + 'title="Roll the dataset back to this snapshot (destroys newer snapshots)">Rollback</button>'
      + '</td>'
      + '</tr>';
  });
  tbody.innerHTML = html;
  mgrSyncSelectAll();
  mgrUpdateSelCount();
}

function mgrSelectAll(cb) {
  _mgrVisible.forEach(function(s) {
    if (cb.checked) _mgrSelected[s.name] = true;
    else delete _mgrSelected[s.name];
  });
  document.querySelectorAll('.mgr-cb').forEach(function(x) { x.checked = cb.checked; });
  mgrUpdateSelCount();
}

function mgrUpdateSelCount() {
  var names = mgrSelectedNames();
  document.getElementById('mgr-sel-count').textContent = names.length;

  var visible = {};
  _mgrVisible.forEach(function(s) { visible[s.name] = true; });
  var hidden = names.filter(function(n) { return !visible[n]; }).length;

  var el = document.getElementById('mgr-sel-note');
  if (!el) return;
  if (hidden > 0) {
    el.style.color = 'var(--zdc-warn)';
    el.textContent = hidden + ' of the ' + names.length + ' selected are hidden by the filter';
  } else {
    el.textContent = '';
  }
}

function mgrResult(msg, color) {
  var el = document.getElementById('mgr-result');
  el.style.color = color || 'var(--zdc-dim)';
  el.textContent = msg;
}

function mgrDeleteSelected() {
  var names = mgrSelectedNames();
  if (!names.length) { mgrResult('Nothing selected.', 'var(--zdc-warn)'); return; }
  // The backend refuses replication checkpoints unless told otherwise; ask
  // separately rather than letting the delete half-fail.
  var chk = names.filter(function(n) { return n.indexOf('@zdc-send-') !== -1; });
  var force = false;
  if (chk.length) {
    force = confirm(chk.length + ' of the selected snapshots are replication checkpoints:\n\n'
      + chk.slice(0, 5).join('\n') + (chk.length > 5 ? '\n…' : '')
      + '\n\nThey are the base of the incremental chain. Deleting them means the next run '
      + 'of those jobs has to send everything again.\n\nDelete them too?');
    if (!force) {
      names = names.filter(function(n) { return n.indexOf('@zdc-send-') === -1; });
      if (!names.length) { mgrResult('Nothing left to delete.', 'var(--zdc-warn)'); return; }
    }
  }

  if (!confirm('Destroy ' + names.length + ' snapshot(s)?\n\nThis cannot be undone.\n\n'
               + names.slice(0, 10).join('\n') + (names.length > 10 ? '\n…' : ''))) return;

  mgrResult('Deleting ' + names.length + ' snapshot(s)…');

  var params = new URLSearchParams();
  params.append('action', 'destroy');
  if (force) params.append('force_checkpoint', '1');
  names.forEach(function(n) { params.append('snapshots[]', n); });
  if (typeof csrf_token !== 'undefined') params.append('csrf_token', csrf_token);

  fetch(_base + '/snapshot_admin.php', {
    method: 'POST',
    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
    body: params
  }).then(function(r) { return r.json(); })
  .then(function(res) {
    if (!res.ok) { mgrResult(res.error || 'Failed', 'var(--zdc-err)'); return; }
    var msg = res.deleted + ' deleted';
    if (res.failed) {
      var first = (res.results || []).filter(function(r) { return !r.ok; })[0];
      msg += ', ' + res.failed + ' failed' + (first ? ' (' + first.error + ')' : '');
    }
    mgrResult(msg, res.failed ? 'var(--zdc-warn)' : 'var(--zdc-ok)');
    loadSnapManager();
    loadSnapshotStatus();
  }).catch(function(e) { mgrResult('Error: ' + e, 'var(--zdc-err)'); });
}

function mgrHold(i) {
  var s = _mgrSnaps[i];
  if (!s) return;
  postForm(_base + '/snapshot_admin.php',
           {action: s.held ? 'release' : 'hold', snapshot: s.name})
  .then(function(res) {
    if (!res.ok) { mgrResult(res.error || 'Failed', 'var(--zdc-err)'); return; }
    mgrResult((s.held ? 'Hold released on ' : 'Hold set on ') + s.name, 'var(--zdc-ok)');
    loadSnapManager();
  }).catch(function(e) { mgrResult('Error: ' + e, 'var(--zdc-err)'); });
}

function mgrRollback(i) {
  var s = _mgrSnaps[i];
  if (!s) return;

  var newer = _mgrSnaps.filter(function(x) {
    return x.dataset === s.dataset && x.creation > s.creation;
  });

  var warn = 'Roll back ' + s.dataset + ' to\n  ' + s.snapshot + '\n\n'
           + 'Every change made since that snapshot is DISCARDED.\n';
  if (newer.length) warn += newer.length + ' newer snapshot(s) of this dataset will be destroyed.\n';
  warn += '\nStop any container or VM using this dataset first.\n\nContinue?';
  if (!confirm(warn)) return;

  var typed = prompt('Type the snapshot name to confirm:\n' + s.name);
  if (typed !== s.name) { mgrResult('Rollback cancelled.', 'var(--zdc-dim)'); return; }

  mgrResult('Rolling back…');
  postForm(_base + '/snapshot_admin.php',
           {action: 'rollback', snapshot: s.name, confirm: s.name})
  .then(function(res) {
    if (!res.ok) { mgrResult(res.error || 'Rollback failed', 'var(--zdc-err)'); return; }
    mgrResult(res.message, 'var(--zdc-ok)');
    loadSnapManager();
    loadSnapshotStatus();
  }).catch(function(e) { mgrResult('Error: ' + e, 'var(--zdc-err)'); });
}

function runDiff() {
  var dataset  = document.getElementById('browser-dataset').value;
  var snapshot = document.getElementById('browser-snapshot').value;
  var target   = document.getElementById('diff-target').value;
  var sumEl    = document.getElementById('diff-summary');
  var outEl    = document.getElementById('diff-result');

  if (!dataset || !snapshot) { sumEl.style.color = 'var(--zdc-warn)'; sumEl.textContent = 'Select a dataset and snapshot first.'; return; }
  if (target === snapshot)   { sumEl.style.color = 'var(--zdc-warn)'; sumEl.textContent = 'Pick a different comparison target.'; return; }

  sumEl.style.color = 'var(--zdc-dim)';
  sumEl.textContent = 'Comparing…';
  outEl.style.display = 'block';
  outEl.textContent = '';

  postForm(_base + '/snapshot_diff.php', {dataset: dataset, from: snapshot, to: target})
  .then(function(res) {
    if (!res.ok) {
      sumEl.style.color = 'var(--zdc-err)';
      sumEl.textContent = res.error || 'diff failed';
      outEl.style.display = 'none';
      return;
    }
    var c = res.counts || {};
    sumEl.style.color = 'var(--zdc-dim)';
    var shortFrom = String(res.from).split('@').pop();
    var shortTo   = String(res.to).indexOf('@') === -1 ? 'live' : String(res.to).split('@').pop();
    sumEl.textContent = shortFrom + ' \u2192 ' + shortTo + '   '
                      + '+' + (c.added || 0) + '  \u2212' + (c.removed || 0)
                      + '  M' + (c.modified || 0) + '  R' + (c.renamed || 0)
                      + (res.truncated ? '  (showing first ' + res.entries.length + ' of ' + res.total + ')' : '');

    if (!res.entries.length) { outEl.textContent = 'No differences.'; return; }

    var colors = {added: 'var(--zdc-ok)', removed: 'var(--zdc-err)', modified: 'var(--zdc-accent)', renamed: 'var(--zdc-warn)'};
    var signs  = {added: '+', removed: '−', modified: 'M', renamed: 'R'};
    var html = '';
    res.entries.forEach(function(e) {
      var col = colors[e.kind] || 'var(--zdc-text)';
      html += '<div style="color:' + col + ';white-space:pre-wrap;word-break:break-all;">'
            + esc(signs[e.kind] || e.change) + ' ' + esc(e.path)
            + (e.new_path ? '  →  ' + esc(e.new_path) : '')
            + (e.type === 'dir' ? '/' : '')
            + '</div>';
    });
    outEl.innerHTML = html;
  }).catch(function(e) {
    sumEl.style.color = 'var(--zdc-err)';
    sumEl.textContent = 'Error: ' + e;
  });
}

var _sendJobs = [];
var _sendPoll = null;

function toggleSendDetails() {
  var en = document.getElementById('send_enabled').checked;
  document.getElementById('send-details').style.display = en ? '' : 'none';
  if (en) { updateSendPreview(); loadSendStatus(); }
  _triggerAutoSave();
}

function updateSendPreview() {
  var preset = document.getElementById('send_schedule_preset').value;
  var hour   = parseInt(document.getElementById('send_schedule_hour').value, 10);
  if (isNaN(hour) || hour < 0 || hour > 23) hour = 4;

  document.getElementById('send-hour-wrap').style.display =
    (preset === 'daily' || preset === 'weekly') ? '' : 'none';
  document.getElementById('send_schedule_custom').style.display =
    (preset === 'custom') ? '' : 'none';

  var expr, custom = false;
  switch (preset) {
    case 'hourly':  expr = '15 * * * *';            break;
    case '6hourly': expr = '15 */6 * * *';          break;
    case 'daily':   expr = '0 ' + hour + ' * * *';  break;
    case 'weekly':  expr = '0 ' + hour + ' * * 0';  break;
    case 'custom':  expr = document.getElementById('send_schedule_custom').value.trim(); custom = true; break;
    default:        expr = '0 4 * * *';
  }
  zdcCronPreview('send-cron-preview', 'send-cron-hint', expr, custom);
}

function loadSendJobs() {
  postForm(_base + '/send_control.php', {action: 'get_jobs'})
  .then(function(res) {
    if (res.ok) { _sendJobs = res.jobs || []; renderSendTable(); }
  }).catch(function(){});
}

function sendField(i, field, val, width, ph) {
  return '<input type="text" value="' + esc(val || '') + '" style="width:' + width + ';font-size:11px;"'
       + (ph ? ' placeholder="' + esc(ph) + '"' : '')
       + ' oninput="_sendJobs[' + i + '][\'' + field + '\']=this.value; saveSendJobs();">';
}

function sendDatasetSelect(i, field, val) {
  var list = _zfsDatasets.slice();
  if (val && list.indexOf(val) === -1) list.unshift(val);   // keep an unknown saved value
  var opts = '<option value="">' + (_zfsDatasets.length ? '— select —' : '— loading —') + '</option>';
  list.forEach(function(d) {
    opts += '<option value="' + esc(d) + '"' + (d === val ? ' selected' : '') + '>' + esc(d) + '</option>';
  });
  return '<select style="font-size:11px;width:172px;"'
       + ' onchange="_sendJobs[' + i + '][\'' + field + '\']=this.value; saveSendJobs();">'
       + opts + '</select>';
}

function sendCheck(i, field, checked) {
  return '<input type="checkbox"' + (checked ? ' checked' : '')
       + ' onchange="_sendJobs[' + i + '][\'' + field + '\']=this.checked; saveSendJobs();">';
}

function renderSendTable() {
  var tbody = document.getElementById('send-tbody');
  if (!_sendJobs.length) {
    tbody.innerHTML = '<tr><td colspan="7" style="color:var(--zdc-dim);padding:8px;">No replication jobs yet — click "Add job".</td></tr>';
    return;
  }
  var html = '';
  _sendJobs.forEach(function(j, i) {
    var isSsh = (j.transport === 'ssh');
    html += '<tr>'
      + '<td>' + sendCheck(i, 'enabled', j.enabled !== false) + '</td>'
      + '<td style="text-align:left;">' + sendField(i, 'name', j.name, '120px', 'name') + '</td>'
      + '<td style="text-align:left;">' + sendDatasetSelect(i, 'source', j.source) + '</td>'
      + '<td style="text-align:left;">'
        + '<div style="display:flex;align-items:center;gap:4px;flex-wrap:wrap;">'
        + '<select style="font-size:11px;width:74px;" title="Where the destination pool lives"'
          + ' onchange="_sendJobs[' + i + '].transport=this.value; saveSendJobs(); renderSendTable();">'
          + '<option value="local"' + (isSsh ? '' : ' selected') + '>local</option>'
          + '<option value="ssh"'   + (isSsh ? ' selected' : '') + '>SSH</option></select>'
        + (isSsh
            ? sendField(i, 'ssh_host', j.ssh_host, '125px', 'root@10.0.0.5')
              + '<span style="color:var(--zdc-dim);">:</span>'
            : '')
        + (isSsh
            ? sendField(i, 'dest', j.dest, '145px', 'tank/backup/appdata')
            : sendField(i, 'dest', j.dest, '145px', 'backup/appdata').replace(
                '<input ', '<input list="zdc-dataset-list" '))
        + '</div>'
        + (isSsh
            ? '<div style="margin-top:3px;">'
              + sendField(i, 'ssh_key', j.ssh_key, '278px', 'SSH key path, e.g. /boot/config/plugins/…/id_send')
              + '</div>'
            : '')
      + '</td>'
      + '<td>' + sendCheck(i, 'recursive', !!j.recursive) + '</td>'
      + '<td>' + sendCheck(i, 'allow_rollback', !!j.allow_rollback) + '</td>'
      + '<td style="white-space:nowrap;">'
        + '<button type="button" class="btn-secondary" style="padding:1px 7px;font-size:11px;" onclick="sendTest(' + i + ')" title="Check the destination is reachable">Test</button> '
        + '<button type="button" class="btn-danger" style="padding:1px 7px;font-size:11px;" onclick="removeSendJob(' + i + ')">✕</button>'
      + '</td></tr>';
  });
  tbody.innerHTML = html;
}

function addSendJob() {
  var n = 1;
  var ids = _sendJobs.map(function(j) { return j.id; });
  while (ids.indexOf('job' + n) !== -1) n++;
  _sendJobs.push({
    id: 'job' + n, enabled: true, name: 'Job ' + n,
    source: '', dest: '', recursive: false, transport: 'local',
    ssh_host: '', ssh_port: 22, ssh_key: '',
    raw: false, compressed: true, allow_rollback: false, keep_dest: 0
  });
  renderSendTable();
}

function removeSendJob(i) {
  if (!confirm('Remove job "' + (_sendJobs[i].name || _sendJobs[i].id) + '"?\n\n'
             + 'Existing snapshots on the destination are left untouched.')) return;
  _sendJobs.splice(i, 1);
  renderSendTable();
  clearTimeout(_sendSaveTimer);
  _sendSaveTimer = null;
  saveSendJobs();
}

var _sendSaveTimer = null;
function saveSendJobs() {
  clearTimeout(_sendSaveTimer);
  _sendSaveTimer = setTimeout(function() {
    var el = document.getElementById('send-result');

    // Send the list as it stands, including an empty one. Filtering here used
    // to mean that deleting the last job saved nothing at all, and that
    // clearing a field to retype it dropped that job from the file.
    postForm(_base + '/send_control.php',
             {action: 'save_jobs', jobs_json: JSON.stringify({jobs: _sendJobs})})
    .then(function(res) {
      if (!res.ok) { el.style.color = 'var(--zdc-err)'; el.textContent = res.error; return; }

      var draft = _sendJobs.filter(function(j) { return !(j.source && j.dest); }).length;
      if (res.warning) {
        el.style.color = 'var(--zdc-warn)'; el.textContent = res.warning;
      } else if (draft) {
        el.style.color = 'var(--zdc-warn)';
        el.textContent = draft + ' job(s) need a source and a destination \u2014 saved but disabled';
      } else {
        el.style.color = 'var(--zdc-ok)';
        el.textContent = '\u2713 Saved';
        setTimeout(function() { if (el.textContent === '\u2713 Saved') el.textContent = ''; }, 2000);
      }
    }).catch(function(){});
  }, 900);
}

function sendTest(i) {
  var j = _sendJobs[i];
  var el = document.getElementById('send-result');
  el.style.color = 'var(--zdc-dim)';
  el.textContent = 'Testing "' + (j.name || j.id) + '"…';
  postForm(_base + '/send_control.php', {action: 'test', job: j.id})
  .then(function(res) {
    if (!res.ok) { el.style.color = 'var(--zdc-err)'; el.textContent = res.error; return; }
    el.style.color = 'var(--zdc-dim)';
    el.textContent = 'Test started — see the log below.';
    setTimeout(loadSendStatus, 1500);
    setTimeout(loadSendStatus, 4000);
  }).catch(function(e) { el.style.color = 'var(--zdc-err)'; el.textContent = 'Error: ' + e; });
}

function sendRun(action) {
  var el = document.getElementById('send-result');
  if (action === 'run_now' && !confirm('Start replication now?\n\nThis transfers data and can take a while.')) return;
  el.style.color = 'var(--zdc-dim)';
  el.textContent = 'Starting…';

  postForm(_base + '/send_control.php', {action: action})
  .then(function(res) {
    if (!res.ok) { el.style.color = 'var(--zdc-err)'; el.textContent = res.error; return; }
    el.textContent = (action === 'dry_run' ? 'Dry run started' : 'Replication started') + ' (PID ' + res.pid + ')';
    startSendPoll();
  }).catch(function(e) { el.style.color = 'var(--zdc-err)'; el.textContent = 'Error: ' + e; });
}

function startSendPoll() {
  if (_sendPoll) clearInterval(_sendPoll);
  _sendPoll = setInterval(loadSendStatus, 3000);
  loadSendStatus();
}

function loadSendStatus() {
  postForm(_base + '/send_control.php', {action: 'status'})
  .then(function(res) {
    if (!res.ok) return;

    var badge = document.getElementById('send-cron-badge');
    if (res.entry && res.cron_healthy) {
      badge.style.color = 'var(--zdc-ok)';
      badge.textContent = res.entry.split(' ').slice(0, 5).join(' ') + ' ✓';
    } else if (res.entry) {
      badge.style.color = 'var(--zdc-warn)';
      badge.textContent = res.entry.split(' ').slice(0, 5).join(' ') + ' (!)';
    } else {
      badge.style.color = 'var(--zdc-warn)';
      badge.textContent = res.running ? 'running…' : 'Not installed';
    }

    var run = res.run || {};
    document.getElementById('send-last-run').textContent = run.last_run || '—';
    var rEl = document.getElementById('send-last-result');
    if (typeof run.jobs_run === 'undefined') {
      rEl.textContent = '—';
    } else {
      rEl.textContent = run.jobs_ok + '/' + run.jobs_run + ' job(s) ok'
                      + (run.errors > 0 ? ', ' + run.errors + ' error(s)' : '')
                      + (run.dry_run ? ' [dry run]' : '');
      rEl.style.color = run.errors > 0 ? 'var(--zdc-err)' : 'var(--zdc-dim)';
    }

    var v = document.getElementById('send-log-viewer');
    var atBottom = (v.scrollTop + v.clientHeight >= v.scrollHeight - 30);
    v.textContent = (res.log || []).join('\n');
    if (atBottom) v.scrollTop = v.scrollHeight;

    if (!res.running && _sendPoll) { clearInterval(_sendPoll); _sendPoll = null; }
  }).catch(function(){});
}

updateSnapPreview();
loadDatasetPickers();
loadSnapDatasetConfig();
if (document.getElementById('snapshots_enabled').checked) loadSnapshotStatus();

var _sendPanelEl = document.getElementById('tab-replicate');
if (_sendPanelEl) {
  _sendPanelEl.addEventListener('change', _triggerAutoSave);
  _sendPanelEl.addEventListener('input',  _triggerAutoSave);
}
initDestPicker();
updateSendPreview();
loadSendJobs();
zdcApplyTheme();
startServerClock();
zdcRestoreTab();
zdcWatchLayout();
zdcRefreshStatus();
setInterval(zdcRefreshStatus, 30000);
if (document.getElementById('send_enabled').checked) loadSendStatus();
</script>
