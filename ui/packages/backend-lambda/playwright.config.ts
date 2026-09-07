import { defineConfig } from "@playwright/test";

process.env.BACKEND_TITLE = "Lambda Backend";

const PORT = 19230;
const SIMULATOR_PORT = 19330;
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
      SERVER_PACKAGE: "backends/lambda",
      SERVER_NAME: "sockerless-backend-lambda",
      ...(BIN ? { SERVER_BIN: BIN } : {}),
      SIMULATOR_PACKAGE: "simulator-aws",
      SIMULATOR_NAME: "simulator-aws",
      SIMULATOR_PORT: String(SIMULATOR_PORT),
      SOCKERLESS_ENDPOINT_URL: `http://127.0.0.1:${SIMULATOR_PORT}`,
      SOCKERLESS_LAMBDA_ROLE_ARN: "arn:aws:iam::000000000000:role/sockerless-e2e",
      SOCKERLESS_LAMBDA_ARCHITECTURE: "arm64",
      SOCKERLESS_CALLBACK_URL: `ws://127.0.0.1:${PORT}/v1/lambda/reverse`,
    },
    port: PORT,
    reuseExistingServer: false,
    // The start script compiles the pinned simulator and the backend; the
    // first run after a pin bump compiles them from a cold module cache,
    // which takes several minutes on a hosted runner.
    timeout: 600_000,
  },
});
