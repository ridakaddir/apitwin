import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  base: "/__ui/",
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      "/__api": "http://localhost:4000",
    },
  },
  build: {
    outDir: "dist",
  },
});
