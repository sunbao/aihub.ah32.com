import { expect, test } from "@playwright/test";

type Run = {
  run_ref: string;
  goal: string;
  constraints: string;
  status: string;
  created_at: string;
  updated_at: string;
  output_version: number;
  output_kind: string;
  is_system?: boolean;
};

const allRuns: Run[] = [
  {
    run_ref: "run-running-alpha",
    goal: "alpha task",
    constraints: "",
    status: "running",
    created_at: "2026-03-03T08:00:00Z",
    updated_at: "2026-03-03T08:00:00Z",
    output_version: 1,
    output_kind: "text",
  },
  {
    run_ref: "run-done-beta",
    goal: "beta task",
    constraints: "",
    status: "completed",
    created_at: "2026-03-02T08:00:00Z",
    updated_at: "2026-03-02T08:00:00Z",
    output_version: 1,
    output_kind: "text",
  },
];

test("run list supports click, input and filter flow", async ({ page }) => {
  await page.route("**/v1/runs**", async (route) => {
    const url = new URL(route.request().url());
    const q = String(url.searchParams.get("q") ?? "").trim().toLowerCase();
    const runs = q ? allRuns.filter((x) => x.goal.toLowerCase().includes(q)) : allRuns;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ runs, has_more: false, next_offset: runs.length }),
    });
  });

  await page.goto("/app/runs");
  await expect(page.getByTestId("run-row-run-running-alpha")).toBeVisible();
  await expect(page.getByTestId("run-row-run-done-beta")).toBeVisible();

  await page.getByTestId("run-filter-running").click();
  await expect(page).toHaveURL(/status=running/);
  await expect(page.getByTestId("run-row-run-running-alpha")).toBeVisible();
  await expect(page.getByTestId("run-row-run-done-beta")).toHaveCount(0);

  await page.getByTestId("run-search-input").fill("beta");
  await page.getByTestId("run-search-button").click();
  await expect(page).toHaveURL(/q=beta/);
  await expect(page.getByTestId("run-empty-state")).toBeVisible();

  await page.getByTestId("run-filter-all").click();
  await expect(page.getByTestId("run-row-run-done-beta")).toBeVisible();
});
