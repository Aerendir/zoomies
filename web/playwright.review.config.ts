import { defineConfig, devices } from '@playwright/test';
import { browserOverride } from './tests/support/fixtures';

const desktopChrome = {
  ...devices['Desktop Chrome'],
  userAgent:
    'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
};

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 30_000,
  expect: { timeout: 7_000 },
  reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:8199', ...browserOverride },
  projects: [
    { name: 'chromium', use: { ...desktopChrome } },
    { name: 'mobile', use: { ...devices['Pixel 7'] } },
  ],
});
