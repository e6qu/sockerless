import { defineConfig } from "@playwright/test";

const PORT = 15556;

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
    command: `SERVER_BIN=${process.env.SERVER_BIN || "../../../gitlabhub/gitlabhub-server"} PORT=${PORT} bash e2e/start-server.sh`,
    port: PORT,
    reuseExistingServer: false,
    timeout: 15_000,
  },
});
