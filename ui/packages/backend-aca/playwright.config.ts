import { defineConfig } from "@playwright/test";

process.env.BACKEND_TITLE = "Container Apps Backend";

const PORT = 19260;
const SIMULATOR_PORT = 19360;
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
      SERVER_PACKAGE: "backends/aca",
      SERVER_NAME: "sockerless-backend-aca",
      ...(BIN ? { SERVER_BIN: BIN } : {}),
      SIMULATOR_PACKAGE: "simulators/azure",
      SIMULATOR_NAME: "simulator-azure",
      SIMULATOR_PORT: String(SIMULATOR_PORT),
      SIMULATOR_SETUP: "azure-aca",
      SOCKERLESS_ENDPOINT_URL: `http://127.0.0.1:${SIMULATOR_PORT}`,
      SOCKERLESS_ACA_SUBSCRIPTION_ID: "00000000-0000-0000-0000-000000000001",
      SOCKERLESS_ACA_RESOURCE_GROUP: "sockerless-e2e",
      SOCKERLESS_ACA_STORAGE_ACCOUNT: "sockerlesse2e",
      SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE: "sockerless-e2e",
      SOCKERLESS_CALLBACK_URL: `ws://127.0.0.1:${PORT}/v1/aca/reverse`,
      // Managed-identity coordinate the real Azure platform injects into an
      // ACA app container. The backend's DefaultAzureCredential reads these to
      // acquire a real ARM bearer from the simulator's /msi/token endpoint —
      // the same code path used against real Azure; only the coordinate value
      // differs. Without it the ARM control plane (now bearer-enforced) 401s.
      IDENTITY_ENDPOINT: `http://127.0.0.1:${SIMULATOR_PORT}/msi/token`,
      IDENTITY_HEADER: "sim-identity-header",
    },
    port: PORT,
    reuseExistingServer: false,
    timeout: 180_000,
  },
});
