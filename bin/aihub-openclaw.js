#!/usr/bin/env node
const fs = require("fs");
const path = require("path");
const os = require("os");

function parseArgs(argv) {
  const out = { apiKey: "", baseUrl: "http://192.168.1.154:8080", skillsDir: "" };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--apiKey" || a === "--api-key") out.apiKey = argv[++i] || "";
    else if (a === "--baseUrl" || a === "--base-url") out.baseUrl = argv[++i] || out.baseUrl;
    else if (a === "--skillsDir" || a === "--skills-dir") out.skillsDir = argv[++i] || "";
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
        "  --skillsDir <dir>        (optional) override OpenClaw skills directory",
        "",
        "What it does:",
        "  - Installs skill to your OpenClaw workspace skills directory (auto-detected)",
        "  - Writes config to: %USERPROFILE%\\.openclaw\\openclaw.json",
        ""
      ].join("\n")
    );
    return;
  }

  const apiKey = (args.apiKey || "").trim();
  if (!apiKey) die("Missing --apiKey (AIHub Agent API key).");

  const baseUrl = (args.baseUrl || "").trim() || "http://192.168.1.154:8080";
  if (!/^https?:\/\//i.test(baseUrl)) die("Invalid --baseUrl: must start with http:// or https://");

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

  const skillDst = path.join(autoSkillsDir, "aihub-connector");

  cfg.skills = ensureObject(cfg.skills);
  cfg.skills.entries = ensureObject(cfg.skills.entries);

  cfg.skills.entries["aihub-connector"] = {
    enabled: true,
    config: { baseUrl, apiKey }
  };

  copyDir(skillSrc, skillDst);

  const backup = backupFile(cfgPath);
  fs.writeFileSync(cfgPath, JSON.stringify(cfg, null, 2) + "\n", "utf8");

  process.stdout.write(
    [
      "OK: AIHub connector installed & configured.",
      "Skill: " + skillDst,
      "Config: " + cfgPath,
      "Backup: " + backup,
      "BaseUrl: " + baseUrl,
      "Next: restart OpenClaw / reload skills."
    ].join("\n") + "\n"
  );
}

main();
