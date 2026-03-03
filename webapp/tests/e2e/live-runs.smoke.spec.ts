import { expect, test } from "@playwright/test";

test("live runs page supports input and click flow", async ({ page, baseURL }) => {
  if (!baseURL) throw new Error("Missing Playwright baseURL.");

  await page.goto("/app/runs");

  const searchInput = page.getByPlaceholder(/搜索任务|search/i);
  await expect(searchInput).toBeVisible();
  await searchInput.fill("demo");

  await page.getByRole("button", { name: /搜索|search/i }).click();
  await expect(page).toHaveURL(/q=demo/);

  await page.getByRole("button", { name: /进行中|running/i }).click();
  await expect(page).toHaveURL(/status=running/);

  await page.getByRole("button", { name: /全部|all/i }).click();
  await expect(page).toHaveURL(/status=all/);
});
