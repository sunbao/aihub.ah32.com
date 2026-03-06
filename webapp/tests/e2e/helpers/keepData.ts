import fs from "node:fs";
import path from "node:path";

export function keepE2EData(): boolean {
  const v = String(process.env.E2E_KEEP_DATA ?? "").trim();
  return v === "1" || v.toLowerCase() === "true";
}

function _findRepoRoot(startDir: string): string {
  // Playwright is usually invoked with `-C webapp`, so process.cwd() becomes `<repo>/webapp`.
  // We want evidence paths to be stable relative to repo root, not to the current package dir.
  let d = path.resolve(startDir);
  for (let i = 0; i < 12; i++) {
    const agents = path.join(d, "AGENTS.md");
    const compose = path.join(d, "docker-compose.yml");
    const openspecDir = path.join(d, "openspec");
    if ((fs.existsSync(agents) && fs.existsSync(compose)) || fs.existsSync(openspecDir)) {
      return d;
    }

    const parent = path.dirname(d);
    if (parent === d) {
      break;
    }
    d = parent;
  }
  return path.resolve(startDir);
}

export function keepLogPath(): string {
  const p = String(process.env.E2E_KEEP_LOG ?? "").trim();
  if (!p) return "";

  // If the user provided a relative path (common), resolve it to repo root so it lands under `<repo>/output/...`.
  if (!path.isAbsolute(p)) {
    const repoRoot = _findRepoRoot(process.cwd());
    return path.resolve(repoRoot, p);
  }
  return p;
}

export function recordKeptData(entry: Record<string, any>): void {
  const p = keepLogPath();
  if (!p) {
    // Fallback to stderr so callers can still locate retained artifacts without a file.
    // Avoid throwing: evidence capture must not hide the real failure.
    // eslint-disable-next-line no-console
    console.warn("[e2e] kept-data", JSON.stringify(entry));
    return;
  }

  const dir = path.dirname(p);
  fs.mkdirSync(dir, { recursive: true });
  fs.appendFileSync(p, JSON.stringify(entry) + "\n", "utf8");
}
