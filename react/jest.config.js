// SPDX-License-Identifier: MIT
/** @type {import('jest').Config} */
export default {
  preset: 'ts-jest',
  testEnvironment: 'jsdom',
  setupFiles: ['<rootDir>/src/setupGlobals.ts'],
  setupFilesAfterEnv: ['<rootDir>/src/setupTests.ts'],
  moduleNameMapper: {
    '\\.(css|less|scss|sass)$': '<rootDir>/src/__mocks__/fileMock.cjs',
    '\\.(png|jpg|jpeg|gif|svg|webp|ico)$': '<rootDir>/src/__mocks__/fileMock.cjs',
    '^@/(.*)$': '<rootDir>/src/$1',
  },
  transform: {
    '^.+\\.tsx?$': ['ts-jest', {
      useESM: true,
      tsconfig: 'tsconfig.test.json',
    }],
  },
  extensionsToTreatAsEsm: ['.ts', '.tsx'],
  // Finds both src/__tests__ component tests and top-level tests/ project gates.
  testMatch: ['**/__tests__/**/*.test.ts?(x)', '**/tests/**/*.test.ts?(x)'],
  moduleFileExtensions: ['ts', 'tsx', 'js', 'jsx', 'json'],
  coverageProvider: 'v8',
  collectCoverageFrom: [
    'src/**/*.{ts,tsx}',
    '!src/**/*.d.ts',
    '!src/types/**',
    '!src/mocks/**',
    '!src/services/*.mock.ts',
    '!src/__tests__/**',
    '!src/__mocks__/**',
    '!src/setupTests.ts',
    '!src/setupGlobals.ts',
  ],
};
