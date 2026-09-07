import { defineConfig } from "@playwright/test";

process.env.BACKEND_TITLE = "Docker Backend";

const PORT = 19280;
const BIN = process.env.BACKEND_BIN;
const HEALTH = `http://localhost:${PORT}/internal/v1/healthz`;
const chromiumExecutable = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;

export default defineConfig({
  testDir: "../core/e2e",
  testMatch: "backend-app.spec.ts",
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL: `http://localhost:${PORT}`,
    headless: true,
  },
  projects: [
    { name: "chromium", use: { browserName: "chromium", launchOptions: chromiumExecutable ? { executablePath: chromiumExecutable } : {} } },
  ],
  webServer: {
    command: `bash ../core/e2e/start-backend.sh`,
    env: {
      SERVER_PORT: String(PORT),
      HEALTH_URL: HEALTH,
      SERVER_PACKAGE: "backends/docker",
      SERVER_NAME: "sockerless-backend-docker",
      ...(BIN ? { SERVER_BIN: BIN } : {}),
    },
    port: PORT,
    reuseExistingServer: false,
    // The start script compiles the pinned simulator and the backend; the
    // first run after a pin bump compiles them from a cold module cache,
    // which takes several minutes on a hosted runner.
    timeout: 600_000,
  },
});
