#!/usr/bin/env node
const crypto = require("crypto");
const fs = require("fs");
const path = require("path");
const os = require("os");

function parseArgs(argv) {
  const out = {
    apiKey: "",
    baseUrl: "",
    name: "",
    skillsDir: "",
    cron: "*/5 * * * *",  // default: every 5 minutes
    cronEnabled: true
  };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--apiKey" || a === "--api-key") out.apiKey = argv[++i] || "";
    else if (a === "--baseUrl" || a === "--base-url") out.baseUrl = argv[++i] || out.baseUrl;
    else if (a === "--name" || a === "--profile") out.name = argv[++i] || "";
    else if (a === "--skillsDir" || a === "--skills-dir") out.skillsDir = argv[++i] || "";
    else if (a === "--cron" || a === "--cron-expr") out.cron = argv[++i] || out.cron;
    else if (a === "--no-cron") out.cronEnabled = false;
    else if (a === "--help" || a === "-h") out.help = true;
  }
  return out;
}

function die(msg, code = 1) {
  process.stderr.write(msg + "\n");
  process.exit(code);
}

function copyDir(src, dst) {
  fs.mkdirSync(dst, { recursive: true });
  for (const ent of fs.readdirSync(src, { withFileTypes: true })) {
    const s = path.join(src, ent.name);
    const d = path.join(dst, ent.name);
    if (ent.isDirectory()) copyDir(s, d);
    else fs.copyFileSync(s, d);
  }
}

function backupFile(p) {
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const backup = p + ".bak." + stamp;
  fs.copyFileSync(p, backup);
  return backup;
}

function syncSkillToSandboxes(skillKey, skillDir) {
  const openclawHome = path.join(os.homedir(), ".openclaw");
  const sandboxesDir = path.join(openclawHome, "sandboxes");
  if (!fs.existsSync(sandboxesDir)) return;

  let entries = [];
  try {
    entries = fs.readdirSync(sandboxesDir, { withFileTypes: true });
  } catch (e) {
    process.stderr.write(
      "WARN: 无法读取 OpenClaw sandboxes 目录，跳过 sandbox skill 同步。错误：" +
        (e && e.message ? e.message : String(e)) +
        "\n"
    );
    return;
  }

  for (const ent of entries) {
    if (!ent.isDirectory()) continue;
    const dst = path.join(sandboxesDir, ent.name, "skills", skillKey);
    if (!fs.existsSync(dst)) continue;
    try {
      fs.rmSync(dst, { recursive: true, force: true });
      copyDir(skillDir, dst);
    } catch (e) {
      process.stderr.write(
        `WARN: sandbox skill 同步失败（${dst}）。错误：` +
          (e && e.message ? e.message : String(e)) +
          "\n"
      );
    }
  }
}

function slugifyAsciiId(s) {
  const t = (s || "").trim().toLowerCase();
  if (!t) return "";
  return t
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+/, "")
    .replace(/-+$/, "")
    .slice(0, 32);
}

function profileId(profileName) {
  const raw = (profileName || "").trim();
  if (!raw) return "";
  const slug = slugifyAsciiId(raw);
  if (slug) return slug;
  return "p" + crypto.createHash("sha256").update(raw, "utf8").digest("hex").slice(0, 8);
}

function rewriteSkillMdConfigPaths(skillMdPath, skillKey) {
  const md = fs.readFileSync(skillMdPath, "utf8");
  const next = md
    .replace(/^name:\s*aihub-connector\s*$/m, `name: ${skillKey}`)
    .replace(/skills\.entries\.aihub-connector/g, `skills.entries.${skillKey}`);
  fs.writeFileSync(skillMdPath, next, "utf8");
}

function ensureObject(v) {
  if (v && typeof v === "object" && !Array.isArray(v)) return v;
  return {};
}

function firstExistingDir(dirs) {
  for (const d of dirs) {
    if (!d) continue;
    try {
      if (fs.existsSync(d) && fs.statSync(d).isDirectory()) return d;
    } catch (e) {
      process.stderr.write(
        "WARN: 目录检测失败（将继续尝试其他路径）。错误：" +
          (e && e.message ? e.message : String(e)) +
          "\n"
      );
    }
  }
  return "";
}

function ensureOpenclawGatewayCmd(homeDir) {
  const openclawDir = path.join(homeDir, ".openclaw");
  const gatewayCmdPath = path.join(openclawDir, "gateway.cmd");
  try {
    fs.mkdirSync(openclawDir, { recursive: true });
    // Pick the newest %APPDATA%\\nvm\\v* that contains openclaw-cn.cmd.
    // Avoid hardcoding the Node version because many users manage Node via nvm.
    const content = [
      "@echo off",
      "setlocal enableextensions",
      "",
      'set "NVMDIR=%APPDATA%\\nvm"',
      'if not exist "%NVMDIR%" (',
      "  echo ERROR: nvm dir not found: %NVMDIR% 1>&2",
      "  exit /b 1",
      ")",
      "",
      'set "VER="',
      'for /f "delims=" %%D in (\'dir /b /ad "%NVMDIR%\\v*" 2^>nul ^| sort /r\') do (',
      '  set "VER=%%D"',
      "  goto :havever",
      ")",
      "",
      ":havever",
      'if "%VER%"=="" (',
      "  echo ERROR: no Node versions found under %NVMDIR% 1>&2",
      "  exit /b 1",
      ")",
      "",
      'set "OCCMD=%NVMDIR%\\%VER%\\openclaw-cn.cmd"',
      'if not exist "%OCCMD%" (',
      "  echo ERROR: openclaw-cn.cmd not found: %OCCMD% 1>&2",
      "  exit /b 1",
      ")",
      "",
      'call "%OCCMD%" gateway',
      "exit /b %ERRORLEVEL%",
      "",
    ].join("\r\n");
    fs.writeFileSync(gatewayCmdPath, content, "ascii");
  } catch (e) {
    process.stderr.write(
      "WARN: 写入 OpenClaw gateway.cmd 失败（不会影响当前配置写入，但会影响开机/登录自启）。错误：" +
        (e && e.message ? e.message : String(e)) +
        "\n"
    );
  }
  return gatewayCmdPath;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    process.stdout.write(
      [
        "AIHub OpenClaw one-command installer",
        "",
        "Usage:",
        "  npx --yes github:sunbao/aihub.ah32.com aihub-openclaw --apiKey <AGENT_API_KEY>",
        "",
        "Options:",
        "  --apiKey <key>           (required) AIHub Agent API key",
        "  --baseUrl <url>          (optional) default: https://ah32.com (or env AIHUB_BASE_URL)",
        "  --name <profile>         (optional) profile name (multi-config, no overwrite)",
        "  --skillsDir <dir>        (optional) override OpenClaw skills directory",
        "  --cron <expr>            (optional) cron expression, default: */5 * * * * (every 5 min)",
        "  --no-cron                 (optional) disable automatic cron job setup",
        "",
        "What it does:",
        "  - Installs skill to your OpenClaw workspace skills directory (auto-detected)",
        "  - Writes config to: %USERPROFILE%\\.openclaw\\openclaw.json",
        "  - Creates a cron job to automatically poll AIHub for new tasks",
        ""
      ].join("\n")
    );
    return;
  }

  const apiKey = (args.apiKey || "").trim();
  if (!apiKey) die("Missing --apiKey (AIHub Agent API key).");

  const baseUrlRaw = (args.baseUrl || "").trim() || (process.env.AIHUB_BASE_URL || "").trim() || "https://ah32.com";
  const baseUrl = baseUrlRaw.replace(/\/+$/, "");
  if (!/^https?:\/\//i.test(baseUrl)) die("Invalid --baseUrl: must start with http:// or https://");

  const profileName = (args.name || "").trim();
  const pid = profileId(profileName);
  const skillKey = pid ? `aihub-connector-${pid}` : "aihub-connector";

  const home = os.homedir();
  const cfgPath = path.join(home, ".openclaw", "openclaw.json");

  if (!fs.existsSync(cfgPath)) {
    die("OpenClaw config not found: " + cfgPath + "\nPlease run OpenClaw onboarding/configure first.");
  }

  const repoRoot = path.resolve(__dirname, "..");
  const skillSrc = path.join(repoRoot, "openclaw", "skills", "aihub-connector");
  if (!fs.existsSync(skillSrc)) die("Skill source missing in package: " + skillSrc);

  const raw = fs.readFileSync(cfgPath, "utf8");
  let cfg;
  try {
    cfg = JSON.parse(raw);
  } catch (e) {
    die("Failed to parse OpenClaw config JSON: " + cfgPath);
  }

  const workspace = cfg?.agents?.defaults?.workspace || "";
  const autoSkillsDir =
    firstExistingDir([
      (args.skillsDir || "").trim(),
      workspace ? path.join(workspace, "skills") : "",
      path.join(home, "clawd", "skills"),
      path.join(home, "openclaw", "skills"),
      path.join(home, ".openclaw", "skills")
    ]) || "";

  if (!autoSkillsDir) {
    die(
      [
        "Unable to determine OpenClaw skills directory.",
        "Tried:",
        "  - --skillsDir",
        workspace ? `  - ${path.join(workspace, "skills")}` : "  - <workspace>/skills (workspace missing in config)",
        `  - ${path.join(home, "clawd", "skills")}`,
        `  - ${path.join(home, "openclaw", "skills")}`,
        `  - ${path.join(home, ".openclaw", "skills")}`,
        "",
        "Fix: pass --skillsDir <dir> explicitly."
      ].join("\n")
    );
  }

  const skillDst = path.join(autoSkillsDir, skillKey);

  cfg.skills = ensureObject(cfg.skills);
  cfg.skills.entries = ensureObject(cfg.skills.entries);

  const prevEntry = ensureObject(cfg.skills.entries[skillKey]);
  const prevConfig = ensureObject(prevEntry.config);
  const nextConfig = { ...prevConfig, baseUrl };
  if (typeof nextConfig.apiKey === "string") delete nextConfig.apiKey;

  cfg.skills.entries[skillKey] = {
    ...prevEntry,
    enabled: true,
    apiKey,
    config: nextConfig
  };

  try {
    fs.rmSync(skillDst, { recursive: true, force: true });
  } catch (e) {
    process.stderr.write(
      "WARN: 删除旧 skill 目录失败（将继续覆盖写入）。错误：" +
        (e && e.message ? e.message : String(e)) +
        "\n"
    );
  }
  copyDir(skillSrc, skillDst);
  rewriteSkillMdConfigPaths(path.join(skillDst, "SKILL.md"), skillKey);
  // OpenClaw isolated runs may use sandbox-copied skills. If the sandbox already exists,
  // sync the updated skill into it so changes take effect immediately.
  syncSkillToSandboxes(skillKey, skillDst);

  const backup = backupFile(cfgPath);
  fs.writeFileSync(cfgPath, JSON.stringify(cfg, null, 2) + "\n", "utf8");
  const gatewayCmdPath = ensureOpenclawGatewayCmd(home);

  // Setup cron job for automatic polling
  if (args.cronEnabled) {
    const cronDir = path.join(home, ".openclaw", "cron");
    const cronJobsFile = path.join(cronDir, "jobs.json");

    let jobs = [];
    if (fs.existsSync(cronJobsFile)) {
      try {
        const data = JSON.parse(fs.readFileSync(cronJobsFile, "utf8"));
        // Handle both formats: array or {version: 1, jobs: [...]}
        jobs = Array.isArray(data) ? data : (data.jobs || []);
      } catch (e) {
        process.stderr.write(
          "WARN: OpenClaw cron jobs 文件解析失败，将重建 jobs.json（不影响 AIHub 平台数据）。错误：" +
            (e && e.message ? e.message : String(e)) +
            "\n"
        );
      }
    }

    // Normalize legacy schedule format: {kind:"cron", cron:"*/5 * * * *"} -> {kind:"cron", expr:"*/5 * * * *"}
    for (const job of jobs) {
      if (!job || typeof job !== "object") continue;
      const schedule = job.schedule;
      if (!schedule || typeof schedule !== "object") continue;
      if (schedule.kind !== "cron") continue;
      if (typeof schedule.expr !== "string" && typeof schedule.cron === "string") schedule.expr = schedule.cron;
      if ("cron" in schedule) delete schedule.cron;
    }

    const jobName = profileName ? `AIHub 拉取任务（${profileName}）` : "AIHub 拉取任务";
    // Check if AIHub cron job already exists (support legacy names)
    const legacyNames = profileName
      ? [jobName, `AIHub Poll - ${profileName}`]
      : [jobName, "AIHub Poll", "AIHub定时拉取任务", "AIHub定时拉取任务"];
    const existingIndex = jobs.findIndex(j => legacyNames.includes(j.name));
    const nowMs = Date.now();
    const existingJob = existingIndex >= 0 ? jobs[existingIndex] : null;
    const existingId =
      existingJob && typeof existingJob.id === "string"
        ? existingJob.id
        : (existingJob && typeof existingJob.jobId === "string" ? existingJob.jobId : "");
    const id = existingId || (profileName ? `aihub-poll-${profileId(profileName)}` : "aihub-poll");
    const createdAtMs =
      existingJob && typeof existingJob.createdAtMs === "number" ? existingJob.createdAtMs : nowMs;
    const newJob = {
      id,
      createdAtMs,
      updatedAtMs: nowMs,
      name: jobName,
      enabled: true,
      wakeMode: "now",
      schedule: {
        kind: "cron",
        expr: args.cron
      },
      sessionTarget: "isolated",
      payload: {
        kind: "agentTurn",
        message:
          `检查 AIHub 任务并执行（使用技能：${skillKey}）。必须按顺序执行：` +
          `1）优先使用 claim-next 一步领取（有任务就立刻领取，不要问用户“要不要领”）；` +
          `2）按返回的任务说明与上下文执行；` +
          `3）发送事件（进度/总结）；` +
          `4）提交产物（如需）；` +
          `5）完成任务项。` +
          `输出/日志要求：只用中文；不要输出任何 UUID/内部 ID（包括 agent_id/work_item_id/run_id 等），如必须提及请统一写成“<id>”；不要把 poll/claim 的原始 JSON 整段贴出来，只做结论性摘要；任何错误必须明确写日志，不允许静默失败。`
      },
      delivery: {
        mode: "announce"
      },
      deleteAfterRun: false,
      state: {}
    };

    if (existingIndex >= 0) {
      jobs[existingIndex] = newJob;
    } else {
      jobs.push(newJob);
    }

    fs.mkdirSync(cronDir, { recursive: true });
    // Preserve the {version: 1, jobs: [...]} format
    fs.writeFileSync(cronJobsFile, JSON.stringify({ version: 1, jobs }, null, 2) + "\n", "utf8");
  }

  process.stdout.write(
    [
      "OK: 已安装并配置 AIHub connector。",
      "Skill: " + skillDst,
      "Config: " + cfgPath,
      "Backup: " + backup,
      "GatewayCmd: " + gatewayCmdPath,
      "BaseUrl: " + baseUrl,
      profileName ? ("Profile: " + profileName) : "Profile: default",
      args.cronEnabled ? ("Cron: " + args.cron + "（已启用定时拉取）") : "Cron: disabled",
      "Next: 重启 OpenClaw / 重新加载技能。"
    ].join("\n") + "\n"
  );
}

main();
