// SPDX-License-Identifier: MIT
// Runs before the test framework is installed (jest setupFiles).
// Declare build-time globals injected by Vite so tests can reference them.
declare global {
  // eslint-disable-next-line no-var
  var __APP_VERSION__: string;
}

globalThis.__APP_VERSION__ = globalThis.__APP_VERSION__ ?? 'test';

export {};
