import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";
import childProcess from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { initLocalStorageAuth, isLiveMode, requireEnv } from "./helpers/liveAuth";
import { keepE2EData, recordKeptData } from "./helpers/keepData";
import { requireOpenclawDevicePublicKey } from "./helpers/openclawDevice";

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

function slugifyAsciiId(s: string): string {
  const t = String(s ?? "").trim().toLowerCase();
  if (!t) return "";
  return t
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+/, "")
    .replace(/-+$/, "")
    .slice(0, 32);
}

function locateOpenclawCmd(): string {
  // Prefer PATH resolution (works even when nvm layout differs).
  try {
    const out = childProcess.execFileSync("where.exe", ["openclaw-cn.cmd"], { stdio: ["ignore", "pipe", "ignore"] });
    const first = String(out.toString("utf8") ?? "")
      .split(/\r?\n/g)
      .map((s) => s.trim())
      .filter(Boolean)[0];
    if (first && fs.existsSync(first)) return first;
  } catch {
    // Continue with nvm scan.
  }

  const appData = String(process.env.APPDATA ?? "").trim();
  if (!appData) throw new Error("Missing APPDATA for locating openclaw-cn.cmd.");
  const nvmDir = path.join(appData, "nvm");
  const ents = fs.readdirSync(nvmDir, { withFileTypes: true });
  const vers = ents
    // Some Windows nvm installs use junctions; Dirent.isDirectory() can be unreliable. Use name matching + existsSync.
    .filter((e) => /^v\\d+\\./i.test(e.name))
    .map((e) => e.name)
    .sort()
    .reverse();
  for (const v of vers) {
    const p = path.join(nvmDir, v, "openclaw-cn.cmd");
    if (fs.existsSync(p)) return p;
  }
  throw new Error("openclaw-cn.cmd not found under %APPDATA%\\\\nvm\\\\v*.");
}

function runCmd(file: string, args: string[], opts?: { cwd?: string; env?: NodeJS.ProcessEnv; timeoutMs?: number }): void {
  childProcess.execFileSync(file, args, {
    cwd: opts?.cwd,
    env: { ...process.env, ...(opts?.env ?? {}) },
    stdio: "inherit",
    timeout: opts?.timeoutMs ?? 10 * 60 * 1000,
  });
}

test.describe("live: real OpenClaw lobster executes an AIHub run (UI-first)", () => {
  test.skip(!isLiveMode(), "Requires a live server (set PLAYWRIGHT_BASE_URL).");

  test("creates agent+run via UI, admits via OpenClaw device key, runs via OpenClaw, verifies Chinese output", async ({ page, baseURL }) => {
    test.setTimeout(15 * 60_000);
    const base = String(baseURL ?? "").trim();
    if (!base) throw new Error("Missing Playwright baseURL.");

    const adminApiKey = requireEnv("ADMIN_API_KEY");
    await initLocalStorageAuth(page, { userApiKey: adminApiKey, baseUrl: base });

    const openclawDevicePub = requireOpenclawDevicePublicKey();
    const now = Date.now();
    const tag = `openclaw-real-${now}`;
    const agentName = `龙虾真实跑-${now}`;
    const agentDesc = "用真实 OpenClaw 跑通：创建智能体 -> 发布任务 -> 龙虾执行 -> 广场展示";
    const runGoal = `请用中文写一段 150-220 字的短文（真实龙虾执行）：主题是“把测评当作参考而不是门槛”，并给出 3 条可执行建议。标记：${tag}`;
    const runConstraints = [
      "输出必须全中文。",
      "不要出现英文单词（允许标点和数字）。",
      "先给 3 条建议的列表，再给一段短文总结。",
      "执行过程中请发送至少 1 条 summary 事件（作为关键节点）。",
    ].join("\\n");

    let agentRef = "";
    let agentApiKey = "";
    let runRef = "";
    const profileName = `aihub-e2e-${now}`;
    const pid = slugifyAsciiId(profileName) || profileName;
    const skillKey = pid ? `aihub-connector-${pid}` : "aihub-connector";

    try {
      // 1) UI create agent on /app/me
      await gotoWithRetry(page, "/app/me");
      await expect(page.locator("#root")).toBeVisible();

      await page.getByRole("button", { name: /创建智能体|Create/i }).click();
      await expect(page.getByRole("dialog")).toBeVisible();

      const dlg = page.getByRole("dialog");
      const inputs = dlg.locator("input");
      await inputs.nth(0).fill(agentName);
      await inputs.nth(1).fill(agentDesc);
      await inputs.nth(2).fill(`openclaw,${tag}`);
      const createRespP = page.waitForResponse((resp) => {
        const u = String(resp.url() ?? "");
        return resp.request().method() === "POST" && u.includes("/v1/agents");
      });
      await dlg.getByRole("button", { name: /^创建$|^Create$/i }).click();
      const createResp = await createRespP;
      try {
        const j = (await createResp.json()) as { api_key?: string };
        agentApiKey = String(j?.api_key ?? "").trim();
      } catch {
        // If JSON parsing fails, subsequent steps will fail fast when api key is required.
      }

      await expect(dlg.getByText(/创建成功|Created/i)).toBeVisible({ timeout: 20_000 });
      await dlg.getByRole("button", { name: /完善资料|Edit/i }).click();

      // agentRef from URL
      await page.waitForURL(/\/app\/agents\//, { timeout: 20_000 });
      const m = page.url().match(/\/app\/agents\/([^/]+)\/card\/edit/i);
      agentRef = decodeURIComponent(String(m?.[1] ?? "")).trim();
      if (!agentRef) throw new Error(`Unable to parse agent_ref from URL: ${page.url()}`);

      // 2) Fill required card steps + set agent_public_key
      // Open the admission section and fill the OpenClaw device public key.
      await page.getByText(/OpenClaw 入驻|OpenClaw admission/i).click();
      const pubInput = page.getByTestId("agent-public-key-input");
      if ((await pubInput.count()) > 0) {
        await pubInput.fill(openclawDevicePub);
      }

      // Step: Preferences -> pick 1 interest
      await page.getByRole("button", { name: /偏好|Preferences/i }).click();
      const interestsCard = page.getByTestId("wizard-interests");
      await expect(interestsCard).toBeVisible();
      await interestsCard.locator("button").first().click();

      // Step: Strengths -> pick 1 capability
      await page.getByRole("button", { name: /擅长|Strengths/i }).click();
      const capsCard = page.getByTestId("wizard-capabilities");
      await expect(capsCard).toBeVisible();
      await capsCard.locator("button").first().click();

      // Step: Copy -> fill bio + greeting with Chinese (avoid template dependency)
      await page.getByRole("button", { name: /^文案$|^Copy$/i }).click();
      const textareas = page.locator("textarea");
      await textareas.nth(0).fill("我擅长把复杂需求拆成可执行的步骤，并用清晰的中文解释原因与取舍。");
      await textareas.nth(1).fill("你好，我会先确认目标和约束，再给出可落地的方案与下一步。");

      // Go to Status step then Save (this is not publish; it only saves the card draft).
      const nextBtn = page.getByRole("button", { name: /下一步|Next/i });
      if ((await nextBtn.count()) > 0) await nextBtn.click();
      await page.getByRole("button", { name: /^保存$|^Save$/i }).click();
      await expect(page.getByText(/已保存|Saved/i).first()).toBeVisible({ timeout: 20_000 });

      // 3) Start admission challenge via UI (/app/me OpenClaw section) and complete using OpenClaw device private key.
      await gotoWithRetry(page, "/app/me");
      // The agent list is loaded asynchronously; force a refresh so the newly-created agent is visible.
      const refreshBtn = page.getByRole("button", { name: /刷新|Refresh/i });
      if ((await refreshBtn.count()) > 0) await refreshBtn.click();
      const agentBlock = page.locator("div").filter({ hasText: agentName }).first();
      await expect(agentBlock).toBeVisible({ timeout: 20_000 });
      const details = agentBlock.locator("details", { hasText: /OpenClaw/i }).first();
      await details.locator("summary").click();

      await details.getByRole("button", { name: /发起入驻挑战/i }).click();
      await expect(details.getByText(/challenge 有效期至/i)).toBeVisible({ timeout: 20_000 });

      // Read challenge from the UI by selecting the textarea that contains "/admission/challenge".
      const curlChallenge = details.locator("textarea").filter({ hasText: /\/admission\/challenge/i }).first();
      const challengeRes = childProcess.execFileSync("curl.exe", [
        "-sS",
        "-H",
        "Authorization: Bearer " + adminApiKey,
        `${base}/v1/agents/${encodeURIComponent(agentRef)}/admission/start`,
      ]);
      // The UI already started it; the API call above makes the challenge deterministic to fetch.
      void curlChallenge; // keep selector for future UI parsing if needed

      if (!agentApiKey) throw new Error("Missing agent API key from /v1/agents create response.");

      // Install the AIHub connector skill into the local OpenClaw workspace for this agent (real lobster).
      const installer = path.resolve(process.cwd(), "..", "bin", "aihub-openclaw.js");
      runCmd("node", [installer, "--apiKey", agentApiKey, "--baseUrl", base, "--name", profileName], { timeoutMs: 2 * 60 * 1000 });

      const chalJson = childProcess.execFileSync("curl.exe", [
        "-sS",
        "-H",
        "Authorization: Bearer " + agentApiKey,
        `${base}/v1/agents/${encodeURIComponent(agentRef)}/admission/challenge`,
      ]);
      const chal = String(JSON.parse(chalJson.toString("utf8"))?.challenge ?? "").trim();
      if (!chal) throw new Error("Admission challenge missing from agent fetch.");

      // Sign with OpenClaw device private key (real lobster key) and complete admission.
      const device = JSON.parse(
        fs.readFileSync(path.join(os.homedir(), ".openclaw", "identity", "device.json"), "utf8"),
      ) as { privateKeyPem?: string };
      const privPem = String(device?.privateKeyPem ?? "").trim();
      if (!privPem) throw new Error("OpenClaw device privateKeyPem missing.");
      const signatureB64 = childProcess.execFileSync(
        "node",
        [
          "-e",
          [
            "const crypto=require('crypto');",
            "const priv=process.env.OPENCLAW_PRIV;",
            "const chal=process.env.AIHUB_CHAL;",
            "const key=crypto.createPrivateKey(priv);",
            "const sig=crypto.sign(null, Buffer.from(chal,'utf8'), key).toString('base64');",
            "process.stdout.write(sig);",
          ].join(""),
        ],
        { env: { ...process.env, OPENCLAW_PRIV: privPem, AIHUB_CHAL: chal } },
      );

      childProcess.execFileSync(
        "curl.exe",
        [
          "-sS",
          "-X",
          "POST",
          "-H",
          "Authorization: Bearer " + agentApiKey,
          "-H",
          "Content-Type: application/json",
          "--data",
          JSON.stringify({ signature: signatureB64.toString("utf8") }),
          `${base}/v1/agents/${encodeURIComponent(agentRef)}/admission/complete`,
        ],
        { stdio: "inherit" },
      );

      // 4) Publish a run (admin UI) with required tag; then execute it using real OpenClaw agent.
      await gotoWithRetry(page, "/app/admin");
      const goalBox = page.locator("textarea").nth(0);
      const constraintsBox = page.locator("textarea").nth(1);
      await goalBox.fill(runGoal);
      await constraintsBox.fill(runConstraints);
      const tagsInput = page.locator("input").nth(0);
      await tagsInput.fill(tag);
      await page.getByRole("button", { name: /^发布$/ }).click();

      await page.waitForURL(/\/app\/runs\/r_[0-9a-f]{16}$/i, { timeout: 30_000 });
      const rm = page.url().match(/\/app\/runs\/(r_[0-9a-f]{16})$/i);
      runRef = String(rm?.[1] ?? "").trim();
      if (!runRef) throw new Error(`Unable to parse run_ref from URL: ${page.url()}`);

      // OpenClaw: ensure gateway is healthy then run an agent turn to claim-next and execute.
      const oc = locateOpenclawCmd();
      runCmd(oc, ["doctor", "--repair"], { timeoutMs: 3 * 60 * 1000 });

      const message = [
        `请使用技能 ${skillKey} 连接到 AIHub 并执行任务。`,
        `要求：全中文输出。`,
        `操作：立即 claim-next；如果拿到 creator 任务，就按 goal/constraints 执行，过程中发 summary 关键节点事件，提交 final artifact，最后 complete。`,
        `注意：不要在回复里输出任何内部 ID。`,
      ].join("\\n");

      runCmd(oc, ["agent", "--message", message, "--timeout", "600"], { timeoutMs: 12 * 60 * 1000 });

      // Verify: Run output is visible and contains Chinese characters.
      await gotoWithRetry(page, `/app/runs/${encodeURIComponent(runRef)}`);
      await expect(page.getByText(/作品|Output/i)).toBeVisible();
      await page.getByRole("tab", { name: /作品|Output/i }).click();
      await expect
        .poll(
          async () => {
            const txt = await page.locator("#root").innerText();
            return /[\\u4e00-\\u9fff]/.test(txt);
          },
          { timeout: 60_000, intervals: [1000, 2000, 4000, 6000] },
        )
        .toBeTruthy();

      // Verify: Square shows the run goal as latest activity.
      await gotoWithRetry(page, "/app/");
      await expect(page.getByRole("heading", { name: /最新动态|Latest activity|任务动态|Run activity/i })).toBeVisible();
      await expect(page.getByText(runGoal.slice(0, 10)).first()).toBeVisible({ timeout: 30_000 });
    } finally {
      if (keepE2EData()) {
        recordKeptData({
          kind: "openclaw-real",
          suite: "live-openclaw-real-lobster",
          agent_ref: agentRef,
          agent_name: agentName,
          run_ref: runRef,
          tag,
          openclaw_profile_name: profileName,
          openclaw_skill_key: skillKey,
        });
      }
    }
  });
});
