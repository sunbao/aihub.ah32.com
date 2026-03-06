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

async function createAgent(request: APIRequestContext, baseURL: string, adminApiKey: string, name: string, tags: string[]) {
  const res = await request.post(`${baseURL}/v1/agents`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
    data: { name, description: "e2e: square latest activity", tags },
  });
  if (!res.ok()) throw new Error(`Create agent failed, status=${res.status()}`);
  const j = (await res.json()) as { agent_ref?: string; api_key?: string };
  const agentRef = String(j.agent_ref ?? "").trim();
  const agentKey = String(j.api_key ?? "").trim();
  if (!agentRef || !agentKey) throw new Error("Create agent response missing agent_ref/api_key.");
  return { agentRef, agentKey };
}

async function adminCreateRun(request: APIRequestContext, baseURL: string, adminApiKey: string, goal: string, requiredTags: string[]) {
  const res = await request.post(`${baseURL}/v1/admin/runs`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
    data: { goal, constraints: "e2e: square activity", required_tags: requiredTags },
  });
  if (!res.ok()) throw new Error(`Create run failed, status=${res.status()}`);
  const j = (await res.json()) as { run_ref?: string };
  const runRef = String(j.run_ref ?? "").trim();
  if (!runRef) throw new Error("Create run response missing run_ref.");
  return { runRef };
}

async function emitKeyEvent(request: APIRequestContext, baseURL: string, agentKey: string, runRef: string, text: string) {
  const res = await request.post(`${baseURL}/v1/gateway/runs/${encodeURIComponent(runRef)}/events`, {
    headers: { Authorization: `Bearer ${agentKey}` },
    data: { kind: "summary", payload: { text } },
  });
  if (!res.ok()) throw new Error(`Emit event failed, status=${res.status()}`);
}

async function adminDeleteRun(request: APIRequestContext, baseURL: string, adminApiKey: string, runRef: string): Promise<void> {
  const ref = String(runRef ?? "").trim();
  if (!ref) return;
  const res = await request.delete(`${baseURL}/v1/admin/runs/${encodeURIComponent(ref)}`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!res.ok()) throw new Error(`Admin delete run failed, status=${res.status()}`);
}

async function adminDeleteAgent(request: APIRequestContext, baseURL: string, adminApiKey: string, agentRef: string): Promise<void> {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return;
  const res = await request.delete(`${baseURL}/v1/agents/${encodeURIComponent(ref)}`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!res.ok()) throw new Error(`Delete agent failed, status=${res.status()}`);
}

test.describe("live: Square shows latest activity", () => {
  test.skip(!isLiveMode(), "Requires a live server (set PLAYWRIGHT_BASE_URL).");

  test("new key-node event appears on /app/ latest activity", async ({ page, request, baseURL }) => {
    test.setTimeout(120_000);
    const base = String(baseURL ?? "").trim();
    if (!base) throw new Error("Missing Playwright baseURL.");

    const adminApiKey = requireEnv("ADMIN_API_KEY");
    await initLocalStorageAuth(page, { userApiKey: adminApiKey, baseUrl: base });

    const tag = `square-${Date.now()}`;
    const goal = `广场最新动态（中文）${Date.now()}`;
    const agentName = `广场测试智能体-${Date.now()}`;
    let agentRef = "";
    let agentKey = "";
    let runRef = "";
    try {
      const a = await createAgent(request, base, adminApiKey, agentName, ["e2e", "square", tag]);
      agentRef = a.agentRef;
      agentKey = a.agentKey;

      const run = await adminCreateRun(request, base, adminApiKey, goal, [tag]);
      runRef = run.runRef;

      await emitKeyEvent(request, base, agentKey, runRef, "中文关键节点：用于触发广场最新动态展示");

      await gotoWithRetry(page, "/app/");
      // The Square page copy may evolve; assert the feed section header exists.
      await expect(page.getByRole("heading", { name: /最新动态|Latest activity|Run activity/i })).toBeVisible({ timeout: 20_000 });

      // Poll until the activity feed includes this run goal (event is a key node).
      await expect
        .poll(
          async () => {
            const res = await request.get(`${base}/v1/activity?limit=20&offset=0`);
            if (!res.ok()) return false;
            const j = (await res.json()) as { items?: Array<{ run_goal?: string }> };
            return Boolean((j.items ?? []).some((it) => String(it?.run_goal ?? "").includes(goal)));
          },
          { timeout: 20_000, intervals: [400, 800, 1200, 1600] },
        )
        .toBeTruthy();

      await expect(page.getByText(goal).first()).toBeVisible({ timeout: 20_000 });
    } finally {
      if (keepE2EData()) {
        recordKeptData({
          kind: "square-activity",
          suite: "live-square-latest-activity",
          agent_ref: agentRef,
          agent_name: agentName,
          run_ref: runRef,
          run_goal: goal,
          tag,
        });
      } else {
        await adminDeleteRun(request, base, adminApiKey, runRef);
        await adminDeleteAgent(request, base, adminApiKey, agentRef);
      }
    }
  });
});
