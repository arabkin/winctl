import { defineConfig } from '@playwright/test';

export default defineConfig({
    testDir: './tests',
    timeout: 30_000,
    expect: { timeout: 5_000 },
    fullyParallel: false,
    retries: 0,
    use: {
        baseURL: `http://localhost:${process.env.WINCTL_PORT || '8443'}`,
        httpCredentials: {
            username: process.env.WINCTL_USER || 'admin',
            password: process.env.WINCTL_PASS || 'changeme',
        },
    },
    projects: [
        { name: 'chromium', use: { browserName: 'chromium' } },
    ],
});
