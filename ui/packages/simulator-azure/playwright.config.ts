import { defineConfig } from "@playwright/test";

const PORT = 19330;
const BIN = process.env.SERVER_BIN || "../../../simulators/azure/simulator-azure";
const HEALTH = `http://localhost:${PORT}/health`;

export default defineConfig({
  testDir: "./e2e",
  testMatch: "**/*.spec.ts",
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
    command: `SERVER_BIN="${BIN}" SERVER_PORT=${PORT} HEALTH_URL="${HEALTH}" SIM_MODE=1 bash ../core/e2e/start-backend.sh`,
    port: PORT,
    reuseExistingServer: false,
    timeout: 15_000,
  },
});
