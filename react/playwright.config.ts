// SPDX-License-Identifier: MIT
import { defineConfig, devices } from '@playwright/test';

// Base URL for the dev server Playwright drives. Override via env if needed.
const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:5173';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
      // Exclude @narrow-tagged tests: they assert behavior that only exists below
      // the mobile breakpoint and don't apply to desktop widths.
      grepInvert: /@narrow/,
    },
    {
      name: 'iPhone 15 Portrait',
      use: { ...devices['iPhone 15'] },
      testMatch: '**/mobile-responsiveness.spec.ts',
    },
    {
      name: 'iPhone 15 Landscape',
      use: { ...devices['iPhone 15 landscape'] },
      testMatch: '**/mobile-responsiveness.spec.ts',
      // Landscape is above the mobile breakpoint, so @narrow tests don't apply.
      grepInvert: /@narrow/,
    },
  ],
  webServer: {
    command: 'npm run dev',
    url: BASE_URL,
    reuseExistingServer: !process.env.CI,
    env: { VITE_USE_MOCK: 'false' },
  },
});
