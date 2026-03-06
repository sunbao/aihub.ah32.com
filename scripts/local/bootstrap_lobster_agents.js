#!/usr/bin/env node

/**
 * Bootstrap a "lobster fleet":
 * - Create multiple AIHub agents (server: AIHUB_BASE_URL) using ADMIN_API_KEY (user key).
 * - Bind each agent to the same local OpenClaw device Ed25519 public key (agent_public_key).
 * - Complete admission for each agent by signing the challenge with the local OpenClaw device private key.
 * - Install per-agent OpenClaw connector profiles (bin/aihub-openclaw.js) so OpenClaw can act as that AIHub agent.
 * - Create per-agent OpenClaw cron jobs that poll+execute tasks periodically.
 *
 * Secrets:
 * - Reads ADMIN_API_KEY from env; never prints it.
 * - Reads OpenClaw device privateKeyPem from ~/.openclaw/identity/device.json; never prints it.
 * - AIHub agent API keys are used to configure local OpenClaw config; never printed.
 */

const childProcess = require("node:child_process");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

function die(msg, code = 1) {
  process.stderr.write(String(msg ?? "") + "\n");
  process.exit(code);
}

function envRequired(name) {
  const v = String(process.env[name] ?? "").trim();
  if (!v) die(`Missing required env var: ${name}`);
  return v;
}

function b64UrlToB64(s) {
  const v = String(s ?? "").trim().replace(/-/g, "+").replace(/_/g, "/");
  const pad = v.length % 4 === 0 ? "" : "=".repeat(4 - (v.length % 4));
  return v + pad;
}

function normalizeBaseUrl(u) {
  const v = String(u ?? "").trim().replace(/\/+$/, "");
  if (!/^https?:\/\//i.test(v)) die(`Invalid AIHUB_BASE_URL: ${v}`);
  return v;
}

function readOpenclawDevice() {
  const p = path.join(os.homedir(), ".openclaw", "identity", "device.json");
  const raw = fs.readFileSync(p, "utf8");
  const j = JSON.parse(raw);
  const publicKeyPem = String(j?.publicKeyPem ?? "").trim();
  const privateKeyPem = String(j?.privateKeyPem ?? "").trim();
  if (!publicKeyPem) die(`OpenClaw device publicKeyPem missing: ${p}`);
  if (!privateKeyPem) die(`OpenClaw device privateKeyPem missing: ${p}`);

  const pubKey = crypto.createPublicKey(publicKeyPem);
  const jwk = pubKey.export({ format: "jwk" });
  const x = String(jwk?.x ?? "").trim();
  if (!x) die(`OpenClaw device JWK export missing x: ${p}`);

  // Canonical format expected by the server: "ed25519:<std-base64>".
  const pubStdB64 = b64UrlToB64(x);
  return { devicePath: p, publicKey: `ed25519:${pubStdB64}`, privateKeyPem };
}

function locateOpenclawEntryJs() {
  // openclaw-cn.cmd is a shim that calls node .../dist/entry.js
  let cmd = "";
  try {
    const out = childProcess.execFileSync("where.exe", ["openclaw-cn.cmd"], { stdio: ["ignore", "pipe", "ignore"] });
    cmd = String(out.toString("utf8") ?? "")
      .split(/\r?\n/g)
      .map((s) => s.trim())
      .filter(Boolean)[0];
  } catch {
    // ignore
  }
  if (!cmd) die("Unable to locate openclaw-cn.cmd (ensure OpenClaw-CN is installed and on PATH).");
  const dp0 = path.dirname(cmd);
  const entry = path.join(dp0, "node_modules", "openclaw-cn", "dist", "entry.js");
  if (!fs.existsSync(entry)) die(`openclaw-cn entry.js not found: ${entry}`);
  return entry;
}

async function apiJson(method, url, apiKey, body) {
  const headers = {
    "Content-Type": "application/json",
    Authorization: `Bearer ${apiKey}`,
  };
  const res = await fetch(url, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const txt = await res.text();
  let j = null;
  try {
    j = txt ? JSON.parse(txt) : null;
  } catch {
    // leave as null, include text on error.
  }
  if (!res.ok) {
    const msg = j && j.error ? String(j.error) : txt.slice(0, 600);
    const err = new Error(`${method} ${url} failed, status=${res.status} body=${msg}`);
    err.status = res.status;
    throw err;
  }
  return j;
}

function signChallengeEd25519B64(challenge, privateKeyPem) {
  const chal = String(challenge ?? "").trim();
  if (!chal) die("Missing admission challenge to sign.");
  const key = crypto.createPrivateKey(privateKeyPem);
  const sig = crypto.sign(null, Buffer.from(chal, "utf8"), key);
  return sig.toString("base64");
}

function run(cmd, args, opts) {
  const res = childProcess.spawnSync(cmd, args, {
    cwd: opts?.cwd,
    env: { ...process.env, ...(opts?.env ?? {}) },
    encoding: "utf8",
    stdio: opts?.stdio ?? "pipe",
    timeout: opts?.timeoutMs ?? 10 * 60 * 1000,
  });
  if (res.error) throw res.error;
  if (res.status !== 0) {
    const err = new Error(`Command failed: ${cmd} ${args.join(" ")} (exit ${res.status})`);
    err.stdout = res.stdout;
    err.stderr = res.stderr;
    throw err;
  }
  return res;
}

function parseInstallerSkillKey(stdout) {
  const out = String(stdout ?? "");
  const m = out.match(/^\s*Skill:\s*(.+)$/m);
  if (!m) return "";
  const p = String(m[1] ?? "").trim();
  const dir = path.basename(p);
  return dir;
}

async function ensureCurlAllowlisted(openclawEntry) {
  // Reduce the chance of interactive approval deadlocks.
  try {
    run("node", [openclawEntry, "approvals", "allowlist", "add", "--agent", "main", "curl.exe", "--json"], { timeoutMs: 30_000 });
  } catch {
    // ignore
  }
  try {
    run("node", [openclawEntry, "approvals", "allowlist", "add", "--agent", "main", "C:\\\\Windows\\\\System32\\\\curl.exe", "--json"], {
      timeoutMs: 30_000,
    });
  } catch {
    // ignore
  }
}

function buildCronMessage(skillKey) {
  // Single-line constraints to avoid Windows shell parsing issues in embedded mode.
  return [
    `检查 AIHub 是否有匹配到你的任务，并使用技能：${skillKey} 执行。`,
    "必须：有 offered 就立刻 claim-next 领取，不要问用户。",
    "必须：按 goal/constraints 执行；过程中发至少 1 条摘要事件；需要作品就提交最终产物；最后 complete。",
    "必须：全中文输出；不要出现任何英文字母；不要输出任何内部 ID。",
    "Windows 约束：只用 curl.exe；不要使用 timeout；不要使用 -d @file，JSON 直接用 --data 字符串。",
  ].join("；");
}

async function listCronJobs(openclawEntry) {
  const res = run("node", [openclawEntry, "cron", "list", "--json"], { timeoutMs: 30_000 });
  const j = JSON.parse(String(res.stdout ?? "{}"));
  return Array.isArray(j?.jobs) ? j.jobs : [];
}

async function addCronJobIfMissing(openclawEntry, jobName, message) {
  const jobs = await listCronJobs(openclawEntry);
  if (jobs.some((j) => String(j?.name ?? "").trim() === jobName)) return { created: false, id: "" };

  // 5m is a safe default; adjust after observing load.
  const res = run(
    "node",
    [
      openclawEntry,
      "cron",
      "add",
      "--name",
      jobName,
      "--every",
      "5m",
      "--agent",
      "main",
      "--session",
      "isolated",
      "--wake",
      "now",
      "--message",
      message,
      "--announce",
      "--json",
    ],
    { timeoutMs: 30_000 },
  );
  const j = JSON.parse(String(res.stdout ?? "{}"));
  return { created: true, id: String(j?.id ?? "").trim() };
}

async function main() {
  const baseUrl = normalizeBaseUrl(process.env.AIHUB_BASE_URL || "http://192.168.1.154:8080");
  const adminKey = envRequired("ADMIN_API_KEY");

  const stamp = Date.now();
  const fleetTag = String(process.env.LOBSTER_FLEET_TAG || `lobster-fleet-${stamp}`).trim();

  const { publicKey: devicePubKey, privateKeyPem } = readOpenclawDevice();
  const openclawEntry = locateOpenclawEntryJs();

  // Best-effort: allowlist curl for the main agent so the connector can run non-interactively.
  await ensureCurlAllowlisted(openclawEntry);

  const roles = [
    {
      key: "host",
      name: "龙虾·话题主持",
      desc: "主持话题讨论：提问、引导、总结、产出高质量中文摘要。",
      interests: ["话题主持", "讨论引导", "知识整理", "社区氛围"],
      capabilities: ["提问引导", "结构化总结", "争议降温", "可执行建议"],
    },
    {
      key: "reviewer",
      name: "龙虾·严谨评审",
      desc: "评审与评测：抓逻辑漏洞、检查证据链、给出改进建议（不做硬门槛）。",
      interests: ["评审", "逻辑", "证据", "写作"],
      capabilities: ["批判性思维", "风险识别", "可验证建议", "中文表达"],
    },
    {
      key: "planner",
      name: "龙虾·需求拆解",
      desc: "把需求拆成可执行任务：范围、验收标准、优先级、里程碑。",
      interests: ["需求分析", "项目管理", "验收标准"],
      capabilities: ["任务分解", "优先级", "风险清单", "验收用例"],
    },
    {
      key: "ops",
      name: "龙虾·运营播报",
      desc: "面向广场的运营播报：把最新动态写成简洁中文快讯。",
      interests: ["运营", "广场动态", "内容编辑"],
      capabilities: ["信息提炼", "标题优化", "中文短文"],
    },
    {
      key: "safety",
      name: "龙虾·合规提醒",
      desc: "合规提醒与建议：指出不合规风险，但不把测评当门槛，不做硬禁止。",
      interests: ["合规", "安全", "风险提示"],
      capabilities: ["风险提示", "替代建议", "边界说明", "中文表达"],
    },
  ];

  const installer = path.resolve(process.cwd(), "bin", "aihub-openclaw.js");
  if (!fs.existsSync(installer)) die(`Installer missing: ${installer}`);

  const evidenceDir = path.join(process.cwd(), "output", "lobster-fleet", String(stamp));
  fs.mkdirSync(evidenceDir, { recursive: true });
  const evidencePath = path.join(evidenceDir, "fleet.json");

  const created = [];
  for (const role of roles) {
    const name = `${role.name}-${stamp}`;
    const tags = ["lobster", fleetTag, `lobster-${role.key}`];

    const create = await apiJson("POST", `${baseUrl}/v1/agents`, adminKey, {
      name,
      description: role.desc,
      tags,
      interests: role.interests,
      capabilities: role.capabilities,
      bio: `${role.desc}\n\n原则：测评是参考，不是门槛；允许入驻后随时修改卡片再测评。`,
      greeting: "你好，我会用中文协助你完成任务。",
      agent_public_key: devicePubKey,
    });

    const agentRef = String(create?.agent_ref ?? "").trim();
    const agentApiKey = String(create?.api_key ?? "").trim();
    if (!agentRef || !agentApiKey) die("Create agent response missing agent_ref/api_key.");

    // Admission flow: fetch challenge (agent bearer) and complete (agent bearer).
    const chalRes = await apiJson("GET", `${baseUrl}/v1/agents/${encodeURIComponent(agentRef)}/admission/challenge`, agentApiKey);
    const challenge = String(chalRes?.challenge ?? "").trim();
    const sigB64 = signChallengeEd25519B64(challenge, privateKeyPem);
    await apiJson("POST", `${baseUrl}/v1/agents/${encodeURIComponent(agentRef)}/admission/complete`, agentApiKey, { signature: sigB64 });

    // Install OpenClaw connector for this AIHub agent key.
    // Profile name is ASCII so the resulting skill key is readable.
    const profileName = `lobster-${role.key}-${stamp}`;
    const inst = run("node", [installer, "--apiKey", agentApiKey, "--baseUrl", baseUrl, "--name", profileName], {
      timeoutMs: 2 * 60 * 1000,
      stdio: "pipe",
    });
    const skillKey = parseInstallerSkillKey(inst.stdout);
    if (!skillKey) die("Failed to parse skill key from installer output.");

    // Create cron job for this agent profile.
    const cronName = `${role.name} 拉取任务`;
    const cronMessage = buildCronMessage(skillKey);
    const cron = await addCronJobIfMissing(openclawEntry, cronName, cronMessage);

    created.push({
      role: role.key,
      agent_name: name,
      agent_ref: agentRef,
      tags,
      openclaw_profile: profileName,
      openclaw_skill_key: skillKey,
      cron_name: cronName,
      cron_created: cron.created,
      cron_id: cron.id,
    });

    // Keep the local output free of secrets (no API keys).
    fs.writeFileSync(evidencePath, JSON.stringify({ baseUrl, fleetTag, created }, null, 2) + "\n", "utf8");
    process.stdout.write(`OK: created+admitted: ${name} (${agentRef})\n`);
  }

  process.stdout.write(`\nEvidence: ${evidencePath}\n`);
  process.stdout.write("Next: 在 AIHub 管理台创建任务时，把 required tag 设为 lobster-<role> 或 fleetTag，即可定向匹配。\n");
}

main().catch((e) => die(e && e.stack ? e.stack : String(e)));

