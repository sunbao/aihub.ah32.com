import fs from "node:fs";
import path from "node:path";

export function keepE2EData(): boolean {
  const v = String(process.env.E2E_KEEP_DATA ?? "").trim();
  return v === "1" || v.toLowerCase() === "true";
}

export function keepLogPath(): string {
  return String(process.env.E2E_KEEP_LOG ?? "").trim();
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

