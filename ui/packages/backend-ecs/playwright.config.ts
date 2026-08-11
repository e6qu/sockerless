import { defineConfig } from "@playwright/test";

process.env.BACKEND_TITLE = "ECS Backend";

const PORT = 19220;
const SIMULATOR_PORT = 19320;
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
      SERVER_PACKAGE: "backends/ecs",
      SERVER_NAME: "sockerless-backend-ecs",
      ...(BIN ? { SERVER_BIN: BIN } : {}),
      SIMULATOR_PACKAGE: "simulator-aws",
      SIMULATOR_NAME: "simulator-aws",
      SIMULATOR_PORT: String(SIMULATOR_PORT),
      SIMULATOR_SETUP: "aws-ecs",
      SOCKERLESS_ENDPOINT_URL: `http://127.0.0.1:${SIMULATOR_PORT}`,
      SOCKERLESS_ECS_CLUSTER: "sockerless-e2e",
      SOCKERLESS_ECS_SUBNETS: "subnet-0123456789abcdef0",
      SOCKERLESS_ECS_EXECUTION_ROLE_ARN: "arn:aws:iam::000000000000:role/sockerless-e2e",
      SOCKERLESS_ECS_CPU_ARCHITECTURE: "ARM64",
    },
    port: PORT,
    reuseExistingServer: false,
    timeout: 180_000,
  },
});
