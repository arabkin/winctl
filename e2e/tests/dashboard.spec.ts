import { test, expect } from '@playwright/test';

// Helper: call API endpoint directly
async function apiCall(request, method: string, path: string) {
    return request.fetch(path, { method });
}

// Reset state before each test
test.beforeEach(async ({ request }) => {
    await apiCall(request, 'POST', '/api/reset');
});

// ─── Page load & auth ───

test.describe('Dashboard load', () => {
    test('loads the dashboard with correct title', async ({ page }) => {
        await page.goto('/');
        await expect(page).toHaveTitle('WinCtl Dashboard');
    });

    test('shows the header', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('h1')).toHaveText('WinCtl Dashboard');
    });

    test('shows connection badge as connected', async ({ page }) => {
        await page.goto('/');
        const badge = page.locator('#connection');
        await expect(badge).toHaveText('connected');
        await expect(badge).toHaveClass(/badge-ok/);
    });

    test('shows all three sections', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('h2', { hasText: 'Restart' })).toBeVisible();
        await expect(page.locator('h2', { hasText: 'Screen Lock' })).toBeVisible();
        await expect(page.locator('h2', { hasText: 'Global' })).toBeVisible();
    });

    test('shows all buttons', async ({ page }) => {
        await page.goto('/');
        await expect(page.getByRole('button', { name: 'Restart Now (60s delay)' })).toBeVisible();
        await expect(page.getByRole('button', { name: 'Lock Now (60s delay)' })).toBeVisible();
        await expect(page.getByRole('button', { name: 'Reset All' })).toBeVisible();
        // Schedule buttons (2 each for restart and lock)
        const schedOnButtons = page.locator('button.btn-on');
        await expect(schedOnButtons).toHaveCount(2);
        const schedOffButtons = page.locator('button.btn-off');
        await expect(schedOffButtons).toHaveCount(2);
    });
});

// ─── Auth ───

test.describe('Authentication', () => {
    test('API returns 401 without credentials', async () => {
        const port = process.env.WINCTL_PORT || '8443';
        const res = await fetch(`http://localhost:${port}/api/status`);
        expect(res.status).toBe(401);
    });
});

// ─── Initial status ───

test.describe('Initial status', () => {
    test('restart schedule shows Idle', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('#restart-schedule-status')).toHaveText('Idle');
    });

    test('restart one-shot shows None', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('#restart-once-status')).toHaveText('None');
    });

    test('lock schedule shows Idle', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('#lock-schedule-status')).toHaveText('Idle');
    });

    test('lock one-shot shows None', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('#lock-once-status')).toHaveText('None');
    });
});

// ─── Restart actions ───

test.describe('Restart controls', () => {
    test('Restart Now sets one-shot to Pending', async ({ page }) => {
        await page.goto('/');
        await page.getByRole('button', { name: 'Restart Now (60s delay)' }).click();

        // Wait for status poll to update
        await expect(page.locator('#restart-once-status')).toContainText('Pending', {
            timeout: 5000,
        });
    });

    test('Schedule On activates restart schedule', async ({ page }) => {
        await page.goto('/');
        // Click the restart section's Schedule On button (first .btn-on)
        await page.locator('section').filter({ hasText: 'Restart' }).locator('.btn-on').click();

        await expect(page.locator('#restart-schedule-status')).toContainText('Active', {
            timeout: 5000,
        });
    });

    test('Schedule Off deactivates restart schedule', async ({ page }) => {
        await page.goto('/');
        const section = page.locator('section').filter({ hasText: 'Restart' });

        // Enable first
        await section.locator('.btn-on').click();
        await expect(page.locator('#restart-schedule-status')).toContainText('Active', {
            timeout: 5000,
        });

        // Disable
        await section.locator('.btn-off').click();
        await expect(page.locator('#restart-schedule-status')).toHaveText('Idle', {
            timeout: 5000,
        });
    });
});

// ─── Lock actions ───

test.describe('Lock controls', () => {
    test('Lock Now sets one-shot to Pending', async ({ page }) => {
        await page.goto('/');
        await page.getByRole('button', { name: 'Lock Now (60s delay)' }).click();

        await expect(page.locator('#lock-once-status')).toContainText('Pending', {
            timeout: 5000,
        });
    });

    test('Schedule On activates lock schedule', async ({ page }) => {
        await page.goto('/');
        await page.locator('section').filter({ hasText: 'Screen Lock' }).locator('.btn-on').click();

        await expect(page.locator('#lock-schedule-status')).toContainText('Active', {
            timeout: 5000,
        });
    });

    test('Schedule Off deactivates lock schedule', async ({ page }) => {
        await page.goto('/');
        const section = page.locator('section').filter({ hasText: 'Screen Lock' });

        await section.locator('.btn-on').click();
        await expect(page.locator('#lock-schedule-status')).toContainText('Active', {
            timeout: 5000,
        });

        await section.locator('.btn-off').click();
        await expect(page.locator('#lock-schedule-status')).toHaveText('Idle', {
            timeout: 5000,
        });
    });
});

// ─── Reset ───

test.describe('Reset', () => {
    test('Reset All clears all active schedules and pending actions', async ({ page }) => {
        await page.goto('/');

        // Activate everything
        await page.locator('section').filter({ hasText: 'Restart' }).locator('.btn-on').click();
        await page.locator('section').filter({ hasText: 'Screen Lock' }).locator('.btn-on').click();
        await page.getByRole('button', { name: 'Restart Now (60s delay)' }).click();
        await page.getByRole('button', { name: 'Lock Now (60s delay)' }).click();

        // Wait for at least one status to show active
        await expect(page.locator('#restart-schedule-status')).toContainText('Active', {
            timeout: 5000,
        });

        // Reset
        await page.getByRole('button', { name: 'Reset All' }).click();

        // Everything should go back to idle/none
        await expect(page.locator('#restart-schedule-status')).toHaveText('Idle', {
            timeout: 5000,
        });
        await expect(page.locator('#lock-schedule-status')).toHaveText('Idle', {
            timeout: 5000,
        });
        await expect(page.locator('#restart-once-status')).toHaveText('None', {
            timeout: 5000,
        });
        await expect(page.locator('#lock-once-status')).toHaveText('None', {
            timeout: 5000,
        });
    });
});

// ─── Status polling ───

test.describe('Status polling', () => {
    test('status updates automatically via polling', async ({ page, request }) => {
        await page.goto('/');
        await expect(page.locator('#restart-schedule-status')).toHaveText('Idle');

        // Enable restart schedule via API directly
        await apiCall(request, 'POST', '/api/restart/schedule');

        // The UI should update within ~3 seconds (poll interval is 2s)
        await expect(page.locator('#restart-schedule-status')).toContainText('Active', {
            timeout: 5000,
        });

        // Clean up
        await apiCall(request, 'POST', '/api/reset');
    });

    test('countdown text contains time format', async ({ page }) => {
        await page.goto('/');

        // Trigger a one-shot restart
        await page.getByRole('button', { name: 'Restart Now (60s delay)' }).click();

        // Wait for Pending text, then check it contains a time-like pattern
        await expect(page.locator('#restart-once-status')).toContainText('Pending', {
            timeout: 5000,
        });
        const text = await page.locator('#restart-once-status').textContent();
        // Should contain something like "Pending — in 59s" or "Pending — in 0m 59s"
        expect(text).toMatch(/Pending.*\d+s/);
    });
});

// ─── API direct tests ───

test.describe('API endpoints', () => {
    test('GET /api/status returns valid JSON', async ({ request }) => {
        const res = await apiCall(request, 'GET', '/api/status');
        expect(res.ok()).toBeTruthy();
        const data = await res.json();
        expect(data).toHaveProperty('restart_schedule_active');
        expect(data).toHaveProperty('lock_schedule_active');
        expect(data).toHaveProperty('restart_pending_once');
        expect(data).toHaveProperty('lock_pending_once');
    });

    test('POST /api/restart/once returns status message', async ({ request }) => {
        const res = await apiCall(request, 'POST', '/api/restart/once');
        expect(res.ok()).toBeTruthy();
        const data = await res.json();
        expect(data.status).toContain('restart');
    });

    test('POST then DELETE /api/restart/schedule toggles state', async ({ request }) => {
        // Enable
        let res = await apiCall(request, 'POST', '/api/restart/schedule');
        expect(res.ok()).toBeTruthy();

        let status = await (await apiCall(request, 'GET', '/api/status')).json();
        expect(status.restart_schedule_active).toBe(true);

        // Disable
        res = await apiCall(request, 'DELETE', '/api/restart/schedule');
        expect(res.ok()).toBeTruthy();

        status = await (await apiCall(request, 'GET', '/api/status')).json();
        expect(status.restart_schedule_active).toBe(false);
    });

    test('POST then DELETE /api/lock/schedule toggles state', async ({ request }) => {
        let res = await apiCall(request, 'POST', '/api/lock/schedule');
        expect(res.ok()).toBeTruthy();

        let status = await (await apiCall(request, 'GET', '/api/status')).json();
        expect(status.lock_schedule_active).toBe(true);

        res = await apiCall(request, 'DELETE', '/api/lock/schedule');
        expect(res.ok()).toBeTruthy();

        status = await (await apiCall(request, 'GET', '/api/status')).json();
        expect(status.lock_schedule_active).toBe(false);
    });

    test('POST /api/reset clears all state', async ({ request }) => {
        // Set up some state
        await apiCall(request, 'POST', '/api/restart/schedule');
        await apiCall(request, 'POST', '/api/lock/schedule');
        await apiCall(request, 'POST', '/api/restart/once');
        await apiCall(request, 'POST', '/api/lock/once');

        // Reset
        const res = await apiCall(request, 'POST', '/api/reset');
        expect(res.ok()).toBeTruthy();

        const status = await (await apiCall(request, 'GET', '/api/status')).json();
        expect(status.restart_schedule_active).toBe(false);
        expect(status.lock_schedule_active).toBe(false);
        expect(status.restart_pending_once).toBe(false);
        expect(status.lock_pending_once).toBe(false);
    });

    test('wrong HTTP method returns 405', async ({ request }) => {
        const res = await request.fetch('/api/status', { method: 'POST' });
        expect(res.status()).toBe(405);
    });
});
