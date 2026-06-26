import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "./",
  build: {
    outDir: path.resolve(__dirname, "../../internal/gui/web/dist"),
    emptyOutDir: true,
    sourcemap: false,
    cssCodeSplit: true,
    target: "es2022",
    rollupOptions: {
      input: {
        main: path.resolve(__dirname, "index.html"),
        rd: path.resolve(__dirname, "rd.html"),
      },
    },
  },
});