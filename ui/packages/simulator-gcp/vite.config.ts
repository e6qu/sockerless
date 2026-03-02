import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "/ui/",
  server: {
    proxy: {
      "/health": "http://localhost:4567",
      "/sim": "http://localhost:4567",
    },
  },
});
