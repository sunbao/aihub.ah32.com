import { expect, test } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";
import type { APIResponse } from "@playwright/test";

const completedChanges = [
  { name: "mobile-square-reading-first", path: "/app/" },
  { name: "public-refs", path: "/app/runs" },
  { name: "cosmology-five-dimensions", path: "/app/curations" },
  { name: "agent-card-authoring-selection", path: "/app/admin" },
];

async function gotoWithRetry(page: Page, url: string): Promise<number> {
  let lastStatus = 0;
  for (let i = 0; i < 4; i++) {
    const res = await page.goto(url, { waitUntil: "domcontentloaded" });
    const status = Number(res?.status() ?? 0);
    lastStatus = status;
    if (status > 0 && status < 400) return status;
    if (status !== 429) break;
    await page.waitForTimeout(1200 * (i + 1));
  }
  return lastStatus;
}

async function requestWithRetry(request: APIRequestContext, url: string): Promise<APIResponse> {
  let last = await request.get(url);
  if (last.ok()) return last;
  for (let i = 0; i < 3; i++) {
    if (last.status() !== 429) break;
    await new Promise((r) => setTimeout(r, 1200 * (i + 1)));
    last = await request.get(url);
    if (last.ok()) return last;
  }
  return last;
}

test("completed OpenSpec changes: public routes are reachable", async ({ page, baseURL }) => {
  if (!baseURL) throw new Error("Missing Playwright baseURL.");

  for (const c of completedChanges) {
    const status = await gotoWithRetry(page, c.path);
    expect(status, `Route status is not healthy for ${c.name}`).toBeGreaterThan(0);
    expect(status, `Route status is not healthy for ${c.name}`).toBeLessThan(400);
    await expect(page).toHaveURL(new RegExp(c.path.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
    await expect(page.locator("#root")).toBeVisible();
  }
});

test("public-refs: run deep link opens from live run list", async ({ page, request, baseURL }) => {
  if (!baseURL) throw new Error("Missing Playwright baseURL.");

  const runsRes = await requestWithRetry(request, `${baseURL}/v1/runs?include_system=1&limit=1&offset=0`);
  expect(runsRes.ok(), "Failed to fetch live runs for deep-link test").toBeTruthy();
  const runsData = (await runsRes.json()) as { runs?: Array<{ run_ref?: string }> };
  const runRef = String(runsData?.runs?.[0]?.run_ref ?? "").trim();

  test.skip(!runRef, "No live run available on server for deep-link assertion.");

  await gotoWithRetry(page, "/app/runs");
  const status = await gotoWithRetry(page, `/app/runs/${encodeURIComponent(runRef)}`);
  expect(status, "Run detail route status is not healthy").toBeLessThan(400);
  await expect(page).toHaveURL(new RegExp(`/app/runs/${runRef}`.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  await expect(page.locator("#root")).toBeVisible();
});
