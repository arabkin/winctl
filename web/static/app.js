function api(method, path) {
    fetch(path, { method: method })
        .then(r => r.json())
        .then(() => poll())
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
setInterval(poll, 2000);
