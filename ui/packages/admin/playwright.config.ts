import { defineConfig } from "@playwright/test";

const ADMIN_PORT = 19090;
const MOCK_BACKEND_PORT = 19100;

export default defineConfig({
  testDir: "./e2e",
  testMatch: "**/*.spec.ts",
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL: `http://localhost:${ADMIN_PORT}`,
    headless: true,
  },
  projects: [
    { name: "chromium", use: { browserName: "chromium" } },
  ],
  webServer: {
    command: `ADMIN_BIN=${process.env.ADMIN_BIN || "../../../cmd/sockerless-admin/sockerless-admin"} MOCK_BACKEND_PORT=${MOCK_BACKEND_PORT} ADMIN_PORT=${ADMIN_PORT} bash e2e/start-server.sh`,
    port: ADMIN_PORT,
    reuseExistingServer: false,
    timeout: 15_000,
  },
});
