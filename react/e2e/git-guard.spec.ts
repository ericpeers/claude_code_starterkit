// SPDX-License-Identifier: MIT
import { test, expect } from '@playwright/test';
import { gitEnvVars } from '../tests/helpers/gitGuard';

// playwright.config.ts clears git's hook variables. CLAUDE.md gives the reason.
test('no git hook variable reaches the e2e tests', () => {
  expect(gitEnvVars()).toEqual(['GIT_CEILING_DIRECTORIES']);
});
