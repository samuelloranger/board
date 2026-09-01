import { svelte } from "@sveltejs/vite-plugin-svelte";
import { svelteTesting } from "@testing-library/svelte/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [svelte(), svelteTesting()],
  build: { outDir: "dist", emptyOutDir: true },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test-setup.js"],
    include: ["src/**/*.test.js"],
  },
});
