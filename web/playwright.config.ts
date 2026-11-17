import { defineConfig } from "@playwright/test";

// yarn test:e2e exercises the compiled editor in a real browser through Go's
// server. Run make test-e2e from the repo root to build the frontend first.
export default defineConfig({
  testDir: "./e2e",
  // Tests share one server-side document session, so run them sequentially.
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:17480",
    viewport: { width: 1440, height: 1000 },
    trace: "retain-on-failure",
  },
  webServer: {
    // Own a separate server so tests cannot attach to a developer's editor.
    command: "go run ../cmd/sciurus serve --addr 127.0.0.1:17480",
    url: "http://127.0.0.1:17480/api/document",
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
