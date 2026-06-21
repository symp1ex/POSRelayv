import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],

  // Важно для WebView/embedded-сценария.
  // Vite должен генерировать относительные пути к assets:
  // ./assets/...
  // а не /assets/...
  base: "./",

  build: {
    outDir: path.resolve(__dirname, "../../internal/gui/web/dist"),
    emptyOutDir: true,
  },
});