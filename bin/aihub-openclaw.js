#!/usr/bin/env node
const crypto = require("crypto");
const fs = require("fs");
const path = require("path");
const os = require("os");

function parseArgs(argv) {
  const out = {
    apiKey: "",
    baseUrl: "http://192.168.1.154:8080",
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
    } catch {
      // ignore
    }
  }
  return "";
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
        "  --baseUrl <url>          (optional) default: http://192.168.1.154:8080",
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

  const baseUrl = (args.baseUrl || "").trim() || "http://192.168.1.154:8080";
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

  cfg.skills.entries[skillKey] = {
    enabled: true,
    config: { baseUrl, apiKey }
  };

  try {
    fs.rmSync(skillDst, { recursive: true, force: true });
  } catch {
    // ignore
  }
  copyDir(skillSrc, skillDst);
  rewriteSkillMdConfigPaths(path.join(skillDst, "SKILL.md"), skillKey);

  const backup = backupFile(cfgPath);
  fs.writeFileSync(cfgPath, JSON.stringify(cfg, null, 2) + "\n", "utf8");

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

    const jobName = profileName ? `AIHub 拉取任务（${profileName}）` : "AIHub 拉取任务";
    // Check if AIHub cron job already exists (support legacy names)
    const legacyNames = profileName
      ? [jobName, `AIHub Poll - ${profileName}`]
      : [jobName, "AIHub Poll", "AIHub定时拉取任务", "AIHub定时拉取任务"];
    const existingIndex = jobs.findIndex(j => legacyNames.includes(j.name));
    const newJob = {
      jobId: existingIndex >= 0 ? jobs[existingIndex].jobId : (pid ? `aihub-${pid}` : ("aihub-" + Date.now())),
      name: jobName,
      schedule: {
        kind: "cron",
        cron: args.cron
      },
      sessionTarget: "isolated",
      payload: {
        kind: "agentTurn",
        message: `检查 AIHub 任务并执行（使用技能：${skillKey}）。必须按顺序执行：1）优先使用 claim-next 一步领取（有任务就立刻领取，不要问用户“要不要领”）；2）按返回的任务说明与上下文执行；3）发送事件（进度/总结）；4）提交作品（如需）；5）完成任务项。若看到有 offered 任务但领取失败，必须把失败原因写出来。`
      },
      delivery: {
        mode: "announce"
      },
      enabled: true,
      deleteAfterRun: false
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
      "BaseUrl: " + baseUrl,
      profileName ? ("Profile: " + profileName) : "Profile: default",
      args.cronEnabled ? ("Cron: " + args.cron + "（已启用定时拉取）") : "Cron: disabled",
      "Next: 重启 OpenClaw / 重新加载技能。"
    ].join("\n") + "\n"
  );
}

main();
