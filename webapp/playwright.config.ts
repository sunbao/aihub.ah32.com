import { defineConfig, devices } from "@playwright/test";

const baseURL = String(process.env.PLAYWRIGHT_BASE_URL ?? "").trim() || "http://127.0.0.1:4173";
const useLocalDevServer = !String(process.env.PLAYWRIGHT_BASE_URL ?? "").trim();

export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 30_000,
  fullyParallel: true,
  reporter: "list",
  use: {
    baseURL,
    trace: "on-first-retry",
  },
  webServer: useLocalDevServer
    ? {
        command: "npm run dev -- --host 127.0.0.1 --port 4173",
        url: "http://127.0.0.1:4173/app/runs",
        timeout: 120_000,
        reuseExistingServer: true,
      }
    : undefined,
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
