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

function fetchConfig() {
    fetch("/api/config")
        .then(r => {
            if (!r.ok) throw new Error("HTTP " + r.status);
            return r.json();
        })
        .then(data => {
            document.getElementById("cfg-port").textContent = data.port;
            document.getElementById("cfg-username").textContent = data.username;
            document.getElementById("cfg-session-timeout").textContent = data.session_timeout_minutes + " min";
            document.getElementById("cfg-restart-interval").textContent = data.restart_min_minutes + " - " + data.restart_max_minutes + " min";
            document.getElementById("cfg-lock-interval").textContent = data.lock_min_minutes + " - " + data.lock_max_minutes + " min";
            var mins = data.update_check_minutes;
            if (mins >= 60) {
                document.getElementById("cfg-update-check").textContent = (mins / 60) + "h";
            } else {
                document.getElementById("cfg-update-check").textContent = mins + " min";
            }
        })
        .catch(function() {
            showToast("Failed to load config", "error");
        });
}

function reloadConfig() {
    fetch("/api/config/reload", { method: "POST" })
        .then(r => {
            if (r.status === 401) { window.location.reload(); return; }
            if (!r.ok) throw new Error("HTTP " + r.status);
            return r.json();
        })
        .then(() => {
            showToast("Configuration reloaded", "ok");
            fetchConfig();
        })
        .catch(function() {
            showToast("Config reload failed", "error");
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
    // Version badge
    if (data.version) {
        document.getElementById("version-badge").textContent = "v" + data.version;
    }

    // Mode badge
    const modeBadge = document.getElementById("mode-badge");
    if (data.dry_run) {
        modeBadge.textContent = "Dry Run";
        modeBadge.className = "badge badge-mode-dry";
    } else {
        modeBadge.textContent = "Real";
        modeBadge.className = "badge badge-mode-real";
    }

    // Restart schedule
    const rSched = document.getElementById("restart-schedule-status");
    if (data.restart_schedule_active) {
        rSched.textContent = "Active — next in " + formatCountdown(data.restart_next_at);
        rSched.style.color = "#b7e4c7";
    } else {
        rSched.textContent = "Idle";
        rSched.style.color = "";
    }

    // Restart one-shot
    const rOnce = document.getElementById("restart-once-status");
    if (data.restart_pending_once) {
        rOnce.textContent = "Pending — in " + formatCountdown(data.restart_once_at);
        rOnce.style.color = "#ffd166";
    } else {
        rOnce.textContent = "None";
        rOnce.style.color = "";
    }

    // Lock schedule
    const lSched = document.getElementById("lock-schedule-status");
    if (data.lock_schedule_active) {
        lSched.textContent = "Active — next in " + formatCountdown(data.lock_next_at);
        lSched.style.color = "#b7e4c7";
    } else {
        lSched.textContent = "Idle";
        lSched.style.color = "";
    }

    // Lock one-shot
    const lOnce = document.getElementById("lock-once-status");
    if (data.lock_pending_once) {
        lOnce.textContent = "Pending — in " + formatCountdown(data.lock_once_at);
        lOnce.style.color = "#ffd166";
    } else {
        lOnce.textContent = "None";
        lOnce.style.color = "";
    }
}

function poll() {
    fetch("/api/status")
        .then(r => {
            if (!r.ok) throw new Error(r.status);
            document.getElementById("connection").className = "badge badge-ok";
            document.getElementById("connection").textContent = "connected";
            return r.json();
        })
        .then(updateUI)
        .catch(() => {
            document.getElementById("connection").className = "badge badge-err";
            document.getElementById("connection").textContent = "disconnected";
        });
}

function checkForUpdate() {
    fetch('/api/update/status')
        .then(r => r.json())
        .then(data => {
            var card = document.getElementById('upgrade-card');
            if (data.available) {
                card.style.display = '';
                document.getElementById('upgrade-version').textContent = 'v' + data.version;
                card.dataset.version = data.version;
                card.dataset.body = data.body || '';
                card.dataset.size = data.size || 0;
            } else {
                card.style.display = 'none';
            }
        })
        .catch(function() {});
}

function showUpgradeDetails() {
    var card = document.getElementById('upgrade-card');
    document.getElementById('upgrade-body').textContent = card.dataset.body || 'No release notes.';
    var bytes = parseInt(card.dataset.size || '0');
    document.getElementById('upgrade-size').textContent = (bytes / 1024 / 1024).toFixed(1) + ' MB';
    document.getElementById('upgrade-details').style.display = '';
    document.getElementById('upgrade-prompt').style.display = 'none';
}

function cancelUpgrade() {
    document.getElementById('upgrade-details').style.display = 'none';
    document.getElementById('upgrade-prompt').style.display = '';
}

function applyUpgrade() {
    document.getElementById('upgrade-details').style.display = 'none';
    document.getElementById('upgrade-progress').style.display = '';
    document.getElementById('upgrade-progress-text').textContent = 'Downloading and verifying update...';

    fetch('/api/update/apply', { method: 'POST' })
        .then(r => r.json())
        .then(data => {
            if (data.error) {
                document.getElementById('upgrade-progress-text').textContent = 'Error: ' + data.error;
                showToast('Upgrade failed: ' + data.error, 'error');
                return;
            }
            document.getElementById('upgrade-progress-text').textContent =
                'Update v' + data.version + ' downloaded and verified. Service will restart shortly.';
            showToast('Update downloaded. Service restarting...', 'ok');
        })
        .catch(function(err) {
            document.getElementById('upgrade-progress-text').textContent = 'Error: ' + err.message;
            showToast('Upgrade failed: ' + err.message, 'error');
        });
}

poll();
fetchConfig();
checkForUpdate();
setInterval(poll, 2000);
setInterval(checkForUpdate, 60000);
