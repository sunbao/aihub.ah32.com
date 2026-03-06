import { expect, test } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";
import { initLocalStorageAuth, isLiveMode, requireEnv } from "./helpers/liveAuth";

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
    data: { name, description: "e2e: pre-review evaluation topic flow", tags: ["e2e", "pre-review"] },
  });
  if (!res.ok()) throw new Error(`Create agent failed, status=${res.status()}`);
  const j = (await res.json()) as { agent_ref?: string; api_key?: string };
  const agentRef = String(j.agent_ref ?? "").trim();
  const agentKey = String(j.api_key ?? "").trim();
  if (!agentRef || !agentKey) throw new Error("Create agent response missing agent_ref/api_key.");
  return { agentRef, agentKey };
}

async function pickSeedTopicTitleForEvaluation(
  request: APIRequestContext,
  baseURL: string,
  adminApiKey: string,
): Promise<string> {
  const res = await request.get(`${baseURL}/v1/pre-review-evaluation/sources/recent-topics?limit=200`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!res.ok()) {
    const body = await res.text();
    throw new Error(`List recent topics failed, status=${res.status()} body=${body.slice(0, 600)}`);
  }
  const j = (await res.json()) as { items?: Array<{ topic_id?: string; title?: string }> };
  const items = Array.isArray(j?.items) ? j.items : [];
  const seed = items.find((it) => String(it?.topic_id ?? "").startsWith("topic_pre_review_seed_") && String(it?.title ?? "").trim());
  const title = String((seed ?? items[0])?.title ?? "").trim();
  if (!title) throw new Error("No recent topics available for evaluation.");
  return title;
}

async function patchAgentForWizardUnlock(request: APIRequestContext, baseURL: string, adminApiKey: string, agentRef: string) {
  const ref = String(agentRef ?? "").trim();
  if (!ref) throw new Error("Missing agentRef.");
  const res = await request.patch(`${baseURL}/v1/agents/${encodeURIComponent(ref)}`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
    data: {
      interests: ["e2e-interest"],
      capabilities: ["e2e-capability"],
      bio: "e2e: pre-review evaluation flow",
      greeting: "e2e: hello",
    },
  });
  if (!res.ok()) throw new Error(`Patch agent failed, status=${res.status()}`);
}

async function deleteAllEvaluationsForAgent(request: APIRequestContext, baseURL: string, adminApiKey: string, agentRef: string): Promise<void> {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return;
  const listRes = await request.get(`${baseURL}/v1/agents/${encodeURIComponent(ref)}/pre-review-evaluations?limit=50`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!listRes.ok()) throw new Error(`List evaluations failed, status=${listRes.status()}`);
  const j = (await listRes.json()) as { items?: Array<{ evaluation_id?: string }> };
  for (const it of j.items ?? []) {
    const id = String(it?.evaluation_id ?? "").trim();
    if (!id) continue;
    const delRes = await request.delete(`${baseURL}/v1/agents/${encodeURIComponent(ref)}/pre-review-evaluations/${encodeURIComponent(id)}`, {
      headers: { Authorization: `Bearer ${adminApiKey}` },
    });
    if (!delRes.ok()) throw new Error(`Delete evaluation failed, status=${delRes.status()}`);
  }
}

async function deleteAgent(request: APIRequestContext, baseURL: string, adminApiKey: string, agentRef: string): Promise<void> {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return;
  const res = await request.delete(`${baseURL}/v1/agents/${encodeURIComponent(ref)}`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!res.ok()) throw new Error(`Delete agent failed, status=${res.status()}`);
}

test.describe("live: pre-review evaluation picks a topic", () => {
  test.skip(!isLiveMode(), "Requires a live server (set PLAYWRIGHT_BASE_URL).");

  test("selects a real topic, starts evaluation, inspects snapshot, then deletes", async ({ page, request, baseURL }) => {
    test.setTimeout(90_000);
    const base = String(baseURL ?? "").trim();
    if (!base) throw new Error("Missing Playwright baseURL.");

    const adminApiKey = requireEnv("ADMIN_API_KEY");

    const agentName = `e2e-eval-topic-${Date.now()}`;
    const { agentRef } = await createAgent(request, base, adminApiKey, agentName);
    await patchAgentForWizardUnlock(request, base, adminApiKey, agentRef);

    // UI auth: treat ADMIN_API_KEY as the user API key for console flows.
    await initLocalStorageAuth(page, { userApiKey: adminApiKey, baseUrl: base });

    const topicTitle = await pickSeedTopicTitleForEvaluation(request, base, adminApiKey);

    try {
      // Go straight to the Status step which contains the pre-review evaluation panel.
      await gotoWithRetry(page, `/app/agents/${encodeURIComponent(agentRef)}/card/edit?step=6`);
      await expect(page.getByRole("button", { name: /话题|Topic/i })).toBeVisible();

      // Choose Topic source.
      await page.getByRole("button", { name: /话题|Topic/i }).click();

      // Pick our seeded topic.
      // Click the Pick button on the same row/card as our exact title (avoid strict-mode collisions
      // with other "Pick" buttons in the recent-topics list).
      const topicRow = page
        .getByText(topicTitle, { exact: true })
        .first()
        .locator("xpath=ancestor::*[.//button[normalize-space()='Pick' or normalize-space()='选择']][1]");
      await expect(topicRow, `Seed topic not visible: "${topicTitle}"`).toBeVisible();
      await topicRow.getByRole("button", { name: /选择|Pick/i }).click();

      // Start evaluation (button text is "发起测评/Start").
      await page.getByRole("button", { name: /发起测评|Start/i }).click();
      await expect(page.getByText(/已发起测评|Evaluation started/i).first()).toBeVisible({ timeout: 10_000 });

      // Find evaluation entry and open snapshot.
      const evalRow = page
        .getByText(topicTitle, { exact: true })
        .first()
        .locator("xpath=ancestor::*[.//button[normalize-space()='Snapshot' or normalize-space()='快照']][1]");
      await expect(evalRow).toBeVisible();
      await evalRow.getByRole("button", { name: /快照|Snapshot/i }).click();

      await expect(page.getByText(/测评来源快照|Source snapshot/i)).toBeVisible();
      const snapDialog = page.getByRole("alertdialog");
      await expect(snapDialog.getByText(new RegExp(topicTitle.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))).first()).toBeVisible();

      // Close snapshot dialog (Esc is the most stable across locales).
      await page.keyboard.press("Escape");

      // Cleanup evaluation via UI delete (also deletes the evaluation run).
      await evalRow.getByRole("button", { name: /删除|Delete/i }).click();
      await expect(page.getByText(/删除测评数据|Delete evaluation/i)).toBeVisible();
      await page.getByRole("alertdialog").getByRole("button", { name: /删除|Delete/i }).click();

      await expect(page.getByText(/暂无测评记录|No evaluations yet/i)).toBeVisible({ timeout: 15_000 });
    } finally {
      let cleanupErr: any = null;
      try {
        await deleteAllEvaluationsForAgent(request, base, adminApiKey, agentRef);
      } catch (e: any) {
        cleanupErr = e;
      }
      try {
        await deleteAgent(request, base, adminApiKey, agentRef);
      } catch (e: any) {
        if (!cleanupErr) cleanupErr = e;
      }
      if (cleanupErr) throw cleanupErr;
    }
  });
});
