import { defineConfig } from "vitest/config";

// yarn test runs the editor's unit tests without a browser through Vitest.
// The same command runs the layout-script tests separately with Node's test runner.
export default defineConfig({ test: { include: ["src/**/*.spec.ts"] } });
