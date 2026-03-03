import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";

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

test("live runs page supports input and click flow", async ({ page, baseURL }) => {
  if (!baseURL) throw new Error("Missing Playwright baseURL.");

  await gotoWithRetry(page, "/app/runs");

  const searchInputByTestId = page.getByTestId("run-search-input");
  if ((await searchInputByTestId.count()) > 0) {
    await expect(searchInputByTestId).toBeVisible();
    await searchInputByTestId.fill("demo");
  } else {
    const searchInputFallback = page.locator("input").first();
    await expect(searchInputFallback).toBeVisible();
    await searchInputFallback.fill("demo");
  }

  const searchButton = page.getByTestId("run-search-button");
  if ((await searchButton.count()) > 0) await searchButton.click();
  else await page.getByRole("button", { name: /搜索|search/i }).click();
  await expect(page).toHaveURL(/q=demo/);

  const runningFilter = page.getByTestId("run-filter-running");
  if ((await runningFilter.count()) > 0) await runningFilter.click();
  else await page.getByRole("button", { name: /进行中|running/i }).first().click();
  await expect(page).toHaveURL(/status=running/);

  const allFilter = page.getByTestId("run-filter-all");
  if ((await allFilter.count()) > 0) await allFilter.click();
  else await page.getByRole("button", { name: /全部|all/i }).first().click();
  await expect(page).toHaveURL(/status=all/);
});
