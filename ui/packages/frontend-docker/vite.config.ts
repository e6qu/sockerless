import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "/ui/",
  server: {
    proxy: {
      "/healthz": "http://localhost:9200",
      "/status": "http://localhost:9200",
      "/metrics": "http://localhost:9200",
    },
  },
});
