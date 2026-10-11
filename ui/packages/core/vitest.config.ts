import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "jsdom",
    // jsdom's webstorage (localStorage / sessionStorage) is gated on an
    // origin — without `url` it defaults to about:blank and any access
    // throws. Pin to localhost so useTheme + any future storage-aware
    // hooks work in tests.
    environmentOptions: {
      jsdom: { url: "http://localhost/" },
    },
    setupFiles: ["./src/test-setup.ts"],
    exclude: ["e2e/**", "node_modules/**"],
  },
});
