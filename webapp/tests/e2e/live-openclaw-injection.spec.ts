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

async function createAgent(request: APIRequestContext, baseURL: string, adminApiKey: string, name: string) {
  const res = await request.post(`${baseURL}/v1/agents`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
    data: { name, description: "e2e: openclaw injection flow", tags: ["e2e", "openclaw"] },
  });
  if (!res.ok()) throw new Error(`Create agent failed, status=${res.status()}`);
  const j = (await res.json()) as { agent_ref?: string; api_key?: string };
  const agentRef = String(j.agent_ref ?? "").trim();
  const agentKey = String(j.api_key ?? "").trim();
  if (!agentRef || !agentKey) throw new Error("Create agent response missing agent_ref/api_key.");
  return { agentRef, agentKey };
}

async function deleteAgent(request: APIRequestContext, baseURL: string, adminApiKey: string, agentRef: string): Promise<void> {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return;
  const res = await request.delete(`${baseURL}/v1/agents/${encodeURIComponent(ref)}`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!res.ok()) throw new Error(`Delete agent failed, status=${res.status()}`);
}

test.describe("live: OpenClaw injection UX", () => {
  test.skip(!isLiveMode(), "Requires a live server (set PLAYWRIGHT_BASE_URL).");

  test("copies injection command", async ({ page, request, context, baseURL }) => {
    const base = String(baseURL ?? "").trim();
    if (!base) throw new Error("Missing Playwright baseURL.");

    const adminApiKey = requireEnv("ADMIN_API_KEY");

    const agentName = `e2e-openclaw-${Date.now()}`;
    const { agentRef, agentKey } = await createAgent(request, base, adminApiKey, agentName);

    await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: base });

    await initLocalStorageAuth(page, {
      userApiKey: adminApiKey,
      agentApiKeys: { [agentRef]: agentKey },
      baseUrl: base,
    });

    try {
      await gotoWithRetry(page, "/app/me");
      await expect(page.locator("#root")).toBeVisible();

      // Find the agent card by name and expand OpenClaw section.
      const agentBlock = page.locator("div").filter({ hasText: agentName }).first();
      await expect(agentBlock).toBeVisible();

      const openclawDetails = agentBlock.locator("details", { hasText: /OpenClaw/i }).first();
      await expect(openclawDetails).toBeVisible();
      await openclawDetails.locator("summary").click();

      // Copy the injection command.
      const cmdTextarea = openclawDetails.locator("textarea").filter({ hasText: /aihub-openclaw|github:sunbao\/aihub\.ah32\.com/i }).first();
      await expect(cmdTextarea).toBeVisible();
      const cmdText = (await cmdTextarea.inputValue()).trim();
      expect(cmdText).toMatch(/aihub-openclaw/i);
      expect(cmdText).toMatch(/--baseUrl/i);

      await openclawDetails.getByRole("button", { name: /复制命令|Copy/i }).click();
      await expect(page.getByText(/已复制命令|复制失败|Copied|Failed/i).first()).toBeVisible({ timeout: 10_000 });
    } finally {
      if (keepE2EData()) {
        recordKeptData({ kind: "agent", suite: "live-openclaw-injection", agent_ref: agentRef, agent_name: agentName });
      } else {
        await deleteAgent(request, base, adminApiKey, agentRef);
      }
    }
  });
});
