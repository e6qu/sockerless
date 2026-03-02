import { defineConfig } from "@playwright/test";

process.env.BACKEND_TITLE = "Azure Functions Backend";

const PORT = 19270;
const BIN = process.env.BACKEND_BIN || "../../../backends/azure-functions/sockerless-backend-azf";
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
    command: `SOCKERLESS_ENDPOINT_URL=http://localhost:1 SERVER_BIN="${BIN}" SERVER_PORT=${PORT} HEALTH_URL="${HEALTH}" bash ../core/e2e/start-backend.sh`,
    port: PORT,
    reuseExistingServer: false,
    timeout: 15_000,
  },
});
