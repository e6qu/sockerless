import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "/ui/",
  server: {
    proxy: {
      "/internal": "http://localhost:5555",
      "/health": "http://localhost:5555",
    },
  },
});
