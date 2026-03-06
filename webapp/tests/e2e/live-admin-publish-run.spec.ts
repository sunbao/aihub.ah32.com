import { expect, test } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";
import { initLocalStorageAuth, isLiveMode, requireEnv } from "./helpers/liveAuth";
import { keepE2EData, recordKeptData } from "./helpers/keepData";

async function gotoWithRetry(page: Page, url: string): Promise<void> {
  let lastStatus = 0;
  for (let i = 0; i < 4; i++) {
    const res = await page.goto(url, { waitUntil: "domcontentloaded" });
    const status = Number(res?.status() ?? 0);
    lastStatus = status;
    if (status > 0 && status < 400) return;
    if (status !== 429) break;
    await page.waitForTimeout(1200 * (i + 1));
  }
  throw new Error(`Failed to open ${url}, last status=${lastStatus}`);
}

async function adminDeleteRun(request: APIRequestContext, baseURL: string, adminApiKey: string, runRef: string): Promise<void> {
  const ref = String(runRef ?? "").trim();
  if (!ref) return;
  const res = await request.delete(`${baseURL}/v1/admin/runs/${encodeURIComponent(ref)}`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!res.ok()) throw new Error(`Admin delete run failed, status=${res.status()}`);
}

test.describe("live: admin publish run UI", () => {
  test.skip(!isLiveMode(), "Requires a live server (set PLAYWRIGHT_BASE_URL).");

  test("publishes a run from /app/admin then cleans up", async ({ page, request, baseURL }) => {
    test.setTimeout(90_000);
    const base = String(baseURL ?? "").trim();
    if (!base) throw new Error("Missing Playwright baseURL.");

    const adminApiKey = requireEnv("ADMIN_API_KEY");
    await initLocalStorageAuth(page, { userApiKey: adminApiKey, baseUrl: base });

    const goal = `E2E publish run ${Date.now()}`;

    let runRef = "";
    try {
      await gotoWithRetry(page, "/app/admin");
      await expect(page.locator("#root")).toBeVisible();

      // Publish card: goal/constraints are the first two textareas on the page.
      const goalBox = page.locator("textarea").nth(0);
      const constraintsBox = page.locator("textarea").nth(1);
      await expect(goalBox).toBeVisible();
      await goalBox.fill(goal);
      await constraintsBox.fill("e2e: admin publish flow");

      // Tags input is the first input on the page (before the judge-agent search box).
      const tagsInput = page.locator("input").nth(0);
      await expect(tagsInput).toBeVisible();
      await tagsInput.fill("e2e smoke");

      await page.getByRole("button", { name: /^发布$/ }).click();

      await page.waitForURL(/\/app\/runs\/r_[0-9a-f]{16}$/i, { timeout: 30_000 });
      const m = page.url().match(/\/app\/runs\/(r_[0-9a-f]{16})$/i);
      runRef = String(m?.[1] ?? "").trim();
      if (!runRef) throw new Error(`Unable to parse run_ref from URL: ${page.url()}`);

      // The route should render.
      await expect(page.locator("#root")).toBeVisible();
    } finally {
      if (keepE2EData()) {
        recordKeptData({ kind: "run", suite: "live-admin-publish-run", run_ref: runRef, goal });
      } else {
        await adminDeleteRun(request, base, adminApiKey, runRef);
        const gone = await request.get(`${base}/v1/runs/${encodeURIComponent(runRef)}`);
        if (runRef && gone.status() !== 404) throw new Error(`Expected run to be deleted (404), got ${gone.status()}`);
      }
    }
  });
});
