import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "/ui/",
  server: {
    proxy: {
      "/internal": "http://localhost:8929",
      "/health": "http://localhost:8929",
      "/api": "http://localhost:8929",
    },
  },
});
