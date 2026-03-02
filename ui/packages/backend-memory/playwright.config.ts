import { defineConfig } from "@playwright/test";

process.env.BACKEND_TITLE = "Memory Backend";

const PORT = 19210;
const BIN = process.env.BACKEND_BIN || "../../../backends/memory/sockerless-backend-memory";
const HEALTH = `http://localhost:${PORT}/internal/v1/healthz`;

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
    { name: "chromium", use: { browserName: "chromium" } },
  ],
  webServer: {
    command: `SERVER_BIN="${BIN}" SERVER_PORT=${PORT} HEALTH_URL="${HEALTH}" bash ../core/e2e/start-backend.sh`,
    port: PORT,
    reuseExistingServer: false,
    timeout: 15_000,
  },
});
