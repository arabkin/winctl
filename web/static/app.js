function api(method, path) {
    fetch(path, { method: method })
        .then(r => {
            if (r.status === 401) {
                window.location.reload();
                return;
            }
            if (!r.ok) throw new Error("HTTP " + r.status);
            return r.json();
        })
        .then(() => poll())
        .catch(err => console.error(err));
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
        })
        .catch(err => console.error(err));
}

function reloadConfig() {
    fetch("/api/config/reload", { method: "POST" })
        .then(r => {
            if (r.status === 401) { window.location.reload(); return; }
            if (!r.ok) throw new Error("HTTP " + r.status);
            return r.json();
        })
        .then(() => fetchConfig())
        .catch(err => console.error(err));
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

poll();
fetchConfig();
setInterval(poll, 2000);
