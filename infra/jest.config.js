// SPDX-License-Identifier: MIT
module.exports = {
  testEnvironment: 'node',
  roots: ['<rootDir>/tests'],
  testMatch: ['**/*.test.ts'],
  transform: {
    // ignoreCode 151002: ts-jest + CDK decorator metadata noise.
    '^.+\\.tsx?$': ['ts-jest', { diagnostics: { ignoreCodes: [151002] } }],
  },
  // When you activate the cdk-nag template test, add:
  //   setupFilesAfterEnv: ['aws-cdk-lib/testhelpers/jest-autoclean'],
};
