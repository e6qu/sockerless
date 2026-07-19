import { defineConfig } from "@playwright/test";

process.env.BACKEND_TITLE = "Azure Functions Backend";

const PORT = 19270;
const SIMULATOR_PORT = 19370;
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
      SERVER_PACKAGE: "backends/azure-functions",
      SERVER_NAME: "sockerless-backend-azf",
      ...(BIN ? { SERVER_BIN: BIN } : {}),
      SIMULATOR_PACKAGE: "simulators/azure",
      SIMULATOR_NAME: "simulator-azure",
      SIMULATOR_PORT: String(SIMULATOR_PORT),
      SIMULATOR_SETUP: "azure-azf",
      SOCKERLESS_ENDPOINT_URL: `http://127.0.0.1:${SIMULATOR_PORT}`,
      SOCKERLESS_AZF_SUBSCRIPTION_ID: "00000000-0000-0000-0000-000000000001",
      SOCKERLESS_AZF_RESOURCE_GROUP: "sockerless-e2e",
      SOCKERLESS_AZF_STORAGE_ACCOUNT: "sockerlesse2e",
      SOCKERLESS_CALLBACK_URL: `ws://127.0.0.1:${PORT}/v1/azf/reverse`,
    },
    port: PORT,
    reuseExistingServer: false,
    timeout: 60_000,
  },
});
