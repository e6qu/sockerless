import { defineConfig } from "@playwright/test";

process.env.BACKEND_TITLE = "Cloud Functions Backend";

const PORT = 19250;
const SIMULATOR_PORT = 19350;
const SIMULATOR_GRPC_PORT = 19351;
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
      SERVER_PACKAGE: "backends/cloudrun-functions",
      SERVER_NAME: "sockerless-backend-gcf",
      SERVER_HELPER_PACKAGE: "agent",
      SERVER_HELPER_COMMAND: "./cmd/sockerless-gcf-bootstrap",
      SERVER_HELPER_NAME: "sockerless-gcf-bootstrap",
      SERVER_HELPER_ENV: "SOCKERLESS_GCF_BOOTSTRAP",
      SERVER_HELPER_GOOS: "linux",
      SERVER_HELPER_GOARCH: "arm64",
      ...(BIN ? { SERVER_BIN: BIN } : {}),
      SIMULATOR_PACKAGE: "simulators/gcp",
      SIMULATOR_NAME: "simulator-gcp",
      SIMULATOR_PORT: String(SIMULATOR_PORT),
      SIMULATOR_GRPC_PORT: String(SIMULATOR_GRPC_PORT),
      SIMULATOR_SETUP: "gcp",
      SOCKERLESS_ENDPOINT_URL: `http://127.0.0.1:${SIMULATOR_PORT}`,
      SOCKERLESS_GCP_LOGADMIN_ENDPOINT: `127.0.0.1:${SIMULATOR_GRPC_PORT}`,
      SOCKERLESS_GCF_PROJECT: "sockerless-e2e",
      SOCKERLESS_GCP_BUILD_BUCKET: "sockerless-e2e-build",
      SOCKERLESS_CALLBACK_URL: `ws://127.0.0.1:${PORT}/v1/gcf/reverse`,
    },
    port: PORT,
    reuseExistingServer: false,
    timeout: 60_000,
  },
});
