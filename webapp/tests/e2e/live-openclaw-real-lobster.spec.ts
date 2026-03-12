import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";
import childProcess from "node:child_process";
import fs from "node:fs";
import path from "node:path";
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
    .filter((e) => /^v\d+\./i.test(e.name))
    .map((e) => e.name)
    .sort()
    .reverse();
  for (const v of vers) {
    const p = path.join(nvmDir, v, "openclaw-cn.cmd");
    if (fs.existsSync(p)) return p;
  }
  throw new Error("openclaw-cn.cmd not found under %APPDATA%\\\\nvm\\\\v*.");
}

function locateOpenclawEntryJs(): string {
  // Avoid executing the .cmd shim. On Windows it requires a shell and can split args unexpectedly.
  const cmd = locateOpenclawCmd();
  const dp0 = path.dirname(cmd);
  const entry = path.join(dp0, "node_modules", "openclaw-cn", "dist", "entry.js");
  if (!fs.existsSync(entry)) throw new Error(`openclaw-cn entry.js not found: ${entry}`);
  return entry;
}

function runCmd(file: string, args: string[], opts?: { cwd?: string; env?: NodeJS.ProcessEnv; timeoutMs?: number }): void {
  childProcess.execFileSync(file, args, {
    cwd: opts?.cwd,
    env: { ...process.env, ...(opts?.env ?? {}) },
    stdio: "inherit",
    timeout: opts?.timeoutMs ?? 10 * 60 * 1000,
  });
}

function runOpenclaw(entryJs: string, args: string[], opts?: { cwd?: string; env?: NodeJS.ProcessEnv; timeoutMs?: number }): void {
  runCmd("node", [entryJs, ...args], opts);
}

test.describe("live: real OpenClaw lobster executes an AIHub run (UI-first)", () => {
  test.use({ locale: "zh-CN" });
  test.skip(!isLiveMode(), "Requires a live server (set PLAYWRIGHT_BASE_URL).");

  test("creates agent+run via UI, runs via OpenClaw, verifies Chinese output", async ({ page, baseURL }) => {
    test.setTimeout(35 * 60_000);
    const base = String(baseURL ?? "").trim();
    if (!base) throw new Error("Missing Playwright baseURL.");

    const adminApiKey = requireEnv("ADMIN_API_KEY");
    await initLocalStorageAuth(page, { userApiKey: adminApiKey, baseUrl: base });

    const now = Date.now();
    const tag = `openclaw-real-${now}`;
    const agentName = `龙虾真实跑-${now}`;
    const agentDesc = "用真实 OpenClaw 跑通：创建智能体 -> 发布任务 -> 龙虾执行 -> 广场展示";
    const runGoal = "请用中文写一段 150-220 字的短文，主题是“把测评当作参考而不是入驻门槛”，并给出 3 条可执行建议。";
    const runConstraints = [
      "输出必须全中文。",
      "不要出现任何英文字母（允许标点和数字）。",
      "先给 3 条建议的列表，再给一段短文总结。",
      "执行过程中请发送至少 1 条摘要事件（作为关键节点）。",
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

      // 2) Fill required card steps
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

      // 3) Install the AIHub connector skill into the local OpenClaw workspace for this agent (real lobster).
      if (!agentApiKey) throw new Error("Missing agent API key from /v1/agents create response.");

      const installer = path.resolve(process.cwd(), "..", "bin", "aihub-openclaw.js");
      runCmd("node", [installer, "--apiKey", agentApiKey, "--baseUrl", base, "--name", profileName], { timeoutMs: 2 * 60 * 1000 });

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
      const ocEntry = locateOpenclawEntryJs();
      // Use --force to self-heal Scheduled Task config drift (common cause of flaky gateway startup on Windows).
      runOpenclaw(ocEntry, ["doctor", "--repair", "--yes", "--force"], { timeoutMs: 4 * 60 * 1000 });
      // The gateway scheduled task can take a moment to come up after doctor/repair.
      // Restart it explicitly and probe with retries.
      try {
        runOpenclaw(ocEntry, ["gateway", "restart", "--force"], { timeoutMs: 2 * 60 * 1000 });
      } catch {
        // If restart isn't available/needed, probe retries below will still catch readiness.
      }
      {
        let ok = false;
        for (let i = 0; i < 8; i++) {
          try {
            runOpenclaw(ocEntry, ["gateway", "probe"], { timeoutMs: 30_000 });
            ok = true;
            break;
          } catch (e: any) {
            console.warn("[e2e] openclaw gateway probe failed; retrying", { attempt: i + 1, error: String(e?.message ?? e) });
            await page.waitForTimeout(1200 * (i + 1));
          }
        }
        if (!ok) {
          // Some environments run in embedded mode even when the gateway is down.
          console.warn("[e2e] openclaw gateway probe did not become reachable; continuing with embedded fallback");
        }
      }
      // Allowlist curl execution to avoid interactive approval deadlocks during automation.
      runOpenclaw(ocEntry, ["approvals", "allowlist", "add", "--agent", "main", "curl.exe", "--json"], { timeoutMs: 30_000 });
      runOpenclaw(ocEntry, ["approvals", "allowlist", "add", "--agent", "main", "C:\\\\Windows\\\\System32\\\\curl.exe", "--json"], {
        timeoutMs: 30_000,
      });

      // IMPORTANT: keep the message as a single line (Windows argument parsing is fragile).
      const message = [
        `请使用技能 ${skillKey} 连接到任务系统并执行任务`,
        "要求：全中文输出，不要出现任何英文字母（允许标点和数字）",
        "操作：立刻领取下一条任务并执行；过程中发送至少 1 条摘要事件；提交最终产物；最后结束任务",
        "注意：不要在回复里输出任何内部 ID",
        "命令约束（Windows）：只用 curl.exe；不要使用 timeout；不要使用 -d @file 这种写法（PowerShell 会误解析），JSON 直接用 --data 字符串",
      ].join("；");

      // OpenClaw agent command requires choosing a session: pass a concrete agent id.
      runOpenclaw(ocEntry, ["agent", "--agent", "main", "--message", message, "--timeout", "900", "--json"], {
        timeoutMs: 20 * 60 * 1000,
      });

      // Verify: Run output is visible and contains Chinese characters.
      await gotoWithRetry(page, `/app/runs/${encodeURIComponent(runRef)}`);
      await expect(page.getByText(/作品|Output/i)).toBeVisible();
      await page.getByRole("tab", { name: /作品|Output/i }).click();
      // Validate output from the Output panel (rendered markdown or raw pre) and avoid nav chrome.
      const outEl = page
        .locator("main")
        .locator("pre.whitespace-pre-wrap, div.prose")
        .filter({ hasText: /[\u4e00-\u9fff]/ })
        .first();
      await expect(outEl).toBeVisible({ timeout: 5 * 60_000 });
      const outText = await outEl.innerText();
      if (/[A-Za-z]/.test(outText)) throw new Error("Run output contains English letters; expected Chinese-only output.");

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
