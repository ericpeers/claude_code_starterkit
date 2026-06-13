// SPDX-License-Identifier: MIT
import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist', 'coverage']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', {
        args: 'after-used',
        ignoreRestSiblings: true,
      }],
    },
  },
  {
    files: ['src/contexts/**/*.{ts,tsx}'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
  {
    // Mock implementations stub interfaces — unused params are inherent to the pattern.
    files: ['src/services/*.mock.ts', 'src/mocks/**/*.ts'],
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', {
        args: 'none',
        ignoreRestSiblings: true,
      }],
    },
  },
  {
    // Type-only files must contain no executable code. This lets us safely
    // exclude them from coverage while relying on lint to catch any accidental
    // addition of runtime logic.
    files: ['src/types/**/*.ts'],
    rules: {
      'no-restricted-syntax': [
        'error',
        { selector: 'FunctionDeclaration', message: 'src/types files must not contain executable code (no function declarations).' },
        { selector: 'VariableDeclaration', message: 'src/types files must not contain executable code (no variable declarations).' },
        { selector: 'ExpressionStatement', message: 'src/types files must not contain executable code (no expression statements).' },
      ],
    },
  },
])
