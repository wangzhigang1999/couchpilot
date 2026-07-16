import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  base: "/couchpilot/",
  plugins: [react()],
  build: {
    outDir: "dist",
  },
});
