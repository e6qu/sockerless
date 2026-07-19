import { defineConfig } from "@playwright/test";

const ADMIN_PORT = 29090;
const BACKEND_PORT = 29100;
const chromiumExecutable = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;

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
    { name: "chromium", use: { browserName: "chromium", launchOptions: chromiumExecutable ? { executablePath: chromiumExecutable } : {} } },
  ],
  webServer: {
    command: `BACKEND_PORT=${BACKEND_PORT} ADMIN_PORT=${ADMIN_PORT} bash e2e/start-server.sh`,
    env: {
      ...(process.env.ADMIN_BIN ? { ADMIN_BIN: process.env.ADMIN_BIN } : {}),
      ...(process.env.BACKEND_BIN ? { BACKEND_BIN: process.env.BACKEND_BIN } : {}),
    },
    port: ADMIN_PORT,
    reuseExistingServer: false,
    timeout: 180_000,
  },
});
