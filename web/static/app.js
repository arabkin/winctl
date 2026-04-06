function showToast(msg, type) {
    var el = document.getElementById("toast");
    el.textContent = msg;
    el.className = "toast toast-" + type;
    setTimeout(function() { el.className = "toast hidden"; }, 3000);
}

function api(method, path) {
    fetch(path, { method: method })
        .then(r => {
            if (r.status === 401) {
                window.location.reload();
                return null;
            }
            if (!r.ok) throw new Error("HTTP " + r.status);
            return r.json();
        })
        .then(data => { if (data !== null) poll(); })
        .catch(err => {
            showToast("Action failed: " + err.message, "error");
        });
}

// Track current state for toggle buttons
var _state = {};

function cancelActivities(obj) {
    fetch('/api/cancel', { method: 'POST', body: JSON.stringify(obj) })
        .then(r => {
            if (r.status === 401) { window.location.reload(); return null; }
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.json();
        })
        .then(data => { if (data !== null) poll(); })
        .catch(err => { showToast('Cancel failed: ' + err.message, 'error'); });
}

function toggleRestartOnce() {
    if (_state.restart_pending_once) {
        cancelActivities({restart_once: true});
    } else {
        api('POST', '/api/restart/once');
    }
}

function toggleRestartSchedule() {
    if (_state.restart_schedule_active) {
        cancelActivities({restart_schedule: true});
    } else {
        api('POST', '/api/restart/schedule');
    }
}

function toggleLockOnce() {
    if (_state.lock_pending_once) {
        cancelActivities({lock_once: true});
    } else {
        api('POST', '/api/lock/once');
    }
}

function toggleLockSchedule() {
    if (_state.lock_schedule_active) {
        cancelActivities({lock_schedule: true});
    } else {
        api('POST', '/api/lock/schedule');
    }
}

function toggleConfig() {
    var body = document.getElementById("config-body");
    var arrow = document.getElementById("config-arrow");
    if (body.style.display === "none") {
        body.style.display = "";
        arrow.innerHTML = "&#9660;";
    } else {
        body.style.display = "none";
        arrow.innerHTML = "&#9654;";
    }
}

var _cfgData = {};

function fetchConfig() {
    fetch("/api/config")
        .then(r => {
            if (!r.ok) throw new Error("HTTP " + r.status);
            return r.json();
        })
        .then(data => {
            _cfgData = data;
            document.getElementById("cfg-port").textContent = data.port;
            document.getElementById("cfg-username").textContent = data.username;
            document.getElementById("cfg-session-timeout").textContent = data.session_timeout_minutes;
            document.getElementById("cfg-restart-min").textContent = data.restart_min_minutes;
            document.getElementById("cfg-restart-max").textContent = data.restart_max_minutes;
            document.getElementById("cfg-lock-min").textContent = data.lock_min_minutes;
            document.getElementById("cfg-lock-max").textContent = data.lock_max_minutes;
            document.getElementById("cfg-update-check").textContent = data.update_check_minutes;
            document.getElementById("cfg-log-level").textContent = data.log_level;
        })
        .catch(function() {
            showToast("Failed to load config", "error");
        });
}

var _cfgNumericInputs = ["session-timeout", "restart-min", "restart-max", "lock-min", "lock-max", "update-check"];
var _cfgAllFields = ["session-timeout", "restart-min", "restart-max", "lock-min", "lock-max", "update-check", "log-level"];

function editConfig() {
    _cfgAllFields.forEach(function(f) {
        document.getElementById("cfg-" + f).style.display = "none";
        document.getElementById("cfg-" + f + "-input").style.display = "";
    });
    document.getElementById("cfg-session-timeout-input").value = _cfgData.session_timeout_minutes || 30;
    document.getElementById("cfg-restart-min-input").value = _cfgData.restart_min_minutes || 5;
    document.getElementById("cfg-restart-max-input").value = _cfgData.restart_max_minutes || 15;
    document.getElementById("cfg-lock-min-input").value = _cfgData.lock_min_minutes || 5;
    document.getElementById("cfg-lock-max-input").value = _cfgData.lock_max_minutes || 15;
    document.getElementById("cfg-update-check-input").value = _cfgData.update_check_minutes || 360;
    document.getElementById("cfg-log-level-input").value = _cfgData.log_level || "info";
    document.getElementById("cfg-edit-btn").style.display = "none";
    document.getElementById("cfg-save-btn").style.display = "";
    document.getElementById("cfg-cancel-btn").style.display = "";
    document.getElementById("cfg-reset-btn").style.display = "";
    document.getElementById("cfg-hint").style.display = "";
    validateConfigInputs();
}

function cancelEditConfig() {
    _cfgAllFields.forEach(function(f) {
        document.getElementById("cfg-" + f).style.display = "";
        var input = document.getElementById("cfg-" + f + "-input");
        input.style.display = "none";
        input.classList.remove("cfg-input-error");
    });
    document.getElementById("cfg-edit-btn").style.display = "";
    document.getElementById("cfg-save-btn").style.display = "none";
    document.getElementById("cfg-cancel-btn").style.display = "none";
    document.getElementById("cfg-reset-btn").style.display = "none";
    document.getElementById("cfg-hint").style.display = "none";
}

function validateConfigInputs() {
    var valid = true;
    _cfgNumericInputs.forEach(function(f) {
        var input = document.getElementById("cfg-" + f + "-input");
        var v = input.value;
        var n = parseInt(v);
        var ok = v !== "" && /^\d+$/.test(v) && n >= 1;
        if (f === "restart-min" || f === "restart-max" || f === "lock-min" || f === "lock-max") {
            ok = ok && n <= 1440;
        }
        if (ok) {
            input.classList.remove("cfg-input-error");
        } else {
            input.classList.add("cfg-input-error");
            valid = false;
        }
    });
    var rMin = parseInt(document.getElementById("cfg-restart-min-input").value) || 0;
    var rMax = parseInt(document.getElementById("cfg-restart-max-input").value) || 0;
    if (rMax < rMin) {
        document.getElementById("cfg-restart-max-input").classList.add("cfg-input-error");
        valid = false;
    }
    var lMin = parseInt(document.getElementById("cfg-lock-min-input").value) || 0;
    var lMax = parseInt(document.getElementById("cfg-lock-max-input").value) || 0;
    if (lMax < lMin) {
        document.getElementById("cfg-lock-max-input").classList.add("cfg-input-error");
        valid = false;
    }
    document.getElementById("cfg-save-btn").disabled = !valid;
    return valid;
}

function resetConfigDefaults() {
    document.getElementById("cfg-session-timeout-input").value = 30;
    document.getElementById("cfg-restart-min-input").value = 5;
    document.getElementById("cfg-restart-max-input").value = 15;
    document.getElementById("cfg-lock-min-input").value = 5;
    document.getElementById("cfg-lock-max-input").value = 15;
    document.getElementById("cfg-update-check-input").value = 360;
    document.getElementById("cfg-log-level-input").value = "info";
    validateConfigInputs();
}

function saveConfig() {
    if (!validateConfigInputs()) return;
    var payload = {
        session_timeout_minutes: parseInt(document.getElementById("cfg-session-timeout-input").value),
        restart_min_minutes: parseInt(document.getElementById("cfg-restart-min-input").value),
        restart_max_minutes: parseInt(document.getElementById("cfg-restart-max-input").value),
        lock_min_minutes: parseInt(document.getElementById("cfg-lock-min-input").value),
        lock_max_minutes: parseInt(document.getElementById("cfg-lock-max-input").value),
        update_check_minutes: parseInt(document.getElementById("cfg-update-check-input").value),
        log_level: document.getElementById("cfg-log-level-input").value
    };

    fetch("/api/config/update", { method: "POST", body: JSON.stringify(payload) })
        .then(r => {
            if (r.status === 401) { window.location.reload(); return null; }
            return r.json().then(data => ({ ok: r.ok, data: data }));
        })
        .then(result => {
            if (result === null) return;
            if (!result.ok) {
                var msg = result.data.error || "Save failed";
                if (result.data.details) msg += ": " + result.data.details.join(", ");
                showToast(msg, "error");
                return;
            }
            showToast("Configuration saved", "ok");
            cancelEditConfig();
            fetchConfig();
        })
        .catch(function(err) {
            showToast("Save failed: " + err.message, "error");
        });
}

function logout() {
    fetch("/api/logout", { method: "POST" })
        .then(() => { window.location.reload(); })
        .catch(err => console.error(err));
}

function formatCountdown(isoStr) {
    if (!isoStr) return "";
    const diff = new Date(isoStr) - Date.now();
    if (diff <= 0) return "imminent";
    const sec = Math.floor(diff / 1000);
    const min = Math.floor(sec / 60);
    const remSec = sec % 60;
    if (min > 0) return min + "m " + remSec + "s";
    return sec + "s";
}

function updateUI(data) {
    _state = data;

    if (data.version) {
        document.getElementById("version-badge").textContent = "v" + data.version;
    }

    const modeBadge = document.getElementById("mode-badge");
    if (data.dry_run) {
        modeBadge.textContent = "Dry Run";
        modeBadge.className = "badge badge-mode-dry";
    } else {
        modeBadge.textContent = "Real";
        modeBadge.className = "badge badge-mode-real";
    }

    // Restart schedule + toggle button
    var rSched = document.getElementById("restart-schedule-status");
    var rSchedBtn = document.getElementById("restart-sched-btn");
    if (data.restart_schedule_active) {
        var rCountdown = formatCountdown(data.restart_next_at);
        rSched.textContent = rCountdown ? "Active — next in " + rCountdown : "Active — scheduling...";
        rSched.style.color = "#b7e4c7";
        rSchedBtn.textContent = "Schedule Off";
        rSchedBtn.className = "btn-off";
    } else {
        rSched.textContent = "Idle";
        rSched.style.color = "";
        rSchedBtn.textContent = "Schedule On";
        rSchedBtn.className = "btn-on";
    }

    // Restart one-shot + toggle button
    var rOnce = document.getElementById("restart-once-status");
    var rOnceBtn = document.getElementById("restart-once-btn");
    if (data.restart_pending_once) {
        rOnce.textContent = "Pending — in " + formatCountdown(data.restart_once_at);
        rOnce.style.color = "#ffd166";
        rOnceBtn.textContent = "Cancel Restart";
        rOnceBtn.className = "btn-off";
    } else {
        rOnce.textContent = "None";
        rOnce.style.color = "";
        rOnceBtn.textContent = "Restart Now";
        rOnceBtn.className = "btn-on";
    }

    // Lock schedule + toggle button
    var lSched = document.getElementById("lock-schedule-status");
    var lSchedBtn = document.getElementById("lock-sched-btn");
    if (data.lock_schedule_active) {
        var lCountdown = formatCountdown(data.lock_next_at);
        lSched.textContent = lCountdown ? "Active — next in " + lCountdown : "Active — scheduling...";
        lSched.style.color = "#b7e4c7";
        lSchedBtn.textContent = "Schedule Off";
        lSchedBtn.className = "btn-off";
    } else {
        lSched.textContent = "Idle";
        lSched.style.color = "";
        lSchedBtn.textContent = "Schedule On";
        lSchedBtn.className = "btn-on";
    }

    // Lock one-shot + toggle button
    var lOnce = document.getElementById("lock-once-status");
    var lOnceBtn = document.getElementById("lock-once-btn");
    if (data.lock_pending_once) {
        lOnce.textContent = "Pending — in " + formatCountdown(data.lock_once_at);
        lOnce.style.color = "#ffd166";
        lOnceBtn.textContent = "Cancel Lock";
        lOnceBtn.className = "btn-off";
    } else {
        lOnce.textContent = "None";
        lOnce.style.color = "";
        lOnceBtn.textContent = "Lock Now";
        lOnceBtn.className = "btn-on";
    }

    var upgradeBtn = document.getElementById("upgrade-btn");
    if (data.update_available && data.update_version) {
        document.getElementById("upgrade-title").textContent = "Upgrade Available";
        document.getElementById("upgrade-info").style.display = "";
        document.getElementById("upgrade-version").textContent = "v" + data.update_version;
        document.getElementById("upgrade-status").style.display = "none";
        upgradeBtn.disabled = false;
        upgradeBtn.textContent = "Check for Updates";
    } else {
        document.getElementById("upgrade-title").textContent = "Updates";
        document.getElementById("upgrade-info").style.display = "none";
        document.getElementById("upgrade-status").style.display = "";
        document.getElementById("upgrade-status-text").textContent = "You're running the latest version";
        upgradeBtn.disabled = false;
        upgradeBtn.textContent = "Check for Updates";
    }
}

function poll() {
    fetch("/api/status")
        .then(r => {
            if (r.status === 401) { window.location.reload(); return null; }
            if (!r.ok) throw new Error(r.status);
            document.getElementById("connection").className = "badge badge-ok";
            document.getElementById("connection").textContent = "connected";
            return r.json();
        })
        .then(data => { if (data) updateUI(data); })
        .catch(() => {
            document.getElementById("connection").className = "badge badge-err";
            document.getElementById("connection").textContent = "disconnected";
        });
}

function checkForUpdateManual() {
    showToast('Checking for updates...', 'ok');
    fetch('/api/update/check', { method: 'POST' })
        .then(r => {
            if (r.status === 401) { window.location.reload(); return null; }
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.json();
        })
        .then(data => {
            if (data === null) return;
            if (!data.available) {
                showToast('You are running the latest version.', 'ok');
                return;
            }
            document.getElementById('modal-version').textContent = 'v' + data.version;
            var bytes = data.size || 0;
            document.getElementById('modal-size').textContent = (bytes / 1024 / 1024).toFixed(1) + ' MB';
            document.getElementById('modal-notes-link').href = 'https://github.com/arabkin/winctl/releases/tag/v' + data.version;
            document.getElementById('modal-actions').style.display = '';
            document.getElementById('modal-progress').style.display = 'none';
            document.getElementById('update-modal').style.display = '';
        })
        .catch(function(err) {
            showToast('Update check failed: ' + err.message, 'error');
        });
}

function closeUpdateModal(event) {
    if (event && event.target !== event.currentTarget) return;
    document.getElementById('update-modal').style.display = 'none';
}

function applyUpgradeFromModal() {
    document.getElementById('modal-actions').style.display = 'none';
    document.getElementById('modal-progress').style.display = '';
    document.getElementById('modal-progress-text').textContent = 'Downloading and verifying update...';

    fetch('/api/update/apply', { method: 'POST' })
        .then(r => {
            if (r.status === 401) { window.location.reload(); return null; }
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.json();
        })
        .then(data => {
            if (data === null) return;
            if (data.error) {
                document.getElementById('modal-progress-text').textContent = 'Error: ' + data.error;
                showToast('Upgrade failed: ' + data.error, 'error');
                return;
            }
            document.getElementById('modal-progress-text').textContent =
                'Update v' + data.version + ' downloaded and verified. Service will restart shortly.';
            showToast('Update downloaded. Service restarting...', 'ok');
        })
        .catch(function(err) {
            document.getElementById('modal-progress-text').textContent = 'Error: ' + err.message;
            showToast('Upgrade failed: ' + err.message, 'error');
        });
}

poll();
fetchConfig();
setInterval(poll, 2000);
