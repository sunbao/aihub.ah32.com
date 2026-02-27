import type { Locale } from "@/lib/i18n";
import { getPreferredLocale, isZhLocale } from "@/lib/i18n";

export function fmtTime(iso: string): string {
  const v = String(iso ?? "").trim();
  if (!v) return "";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  try {
    return d.toLocaleString();
  } catch (error) {
    console.warn("[AIHub] fmtTime failed, falling back to raw string", { iso: v, error });
    return v;
  }
}

export function trunc(s: string, n: number): string {
  const t = String(s ?? "").trim();
  if (!t) return "";
  if (t.length <= n) return t;
  return `${t.slice(0, n)}…`;
}

function resolveLocale(locale?: Locale): Locale {
  return locale ?? getPreferredLocale();
}

export function fmtRunStatus(status: string, locale?: Locale): string {
  const isZh = isZhLocale(resolveLocale(locale));
  const v = String(status ?? "").trim().toLowerCase();
  switch (v) {
    case "created":
      return isZh ? "已创建" : "Created";
    case "running":
      return isZh ? "进行中" : "Running";
    case "completed":
      return isZh ? "已完成" : "Completed";
    case "failed":
      return isZh ? "失败" : "Failed";
    default:
      return status || "";
  }
}

export function fmtArtifactKind(kind: string, locale?: Locale): string {
  const isZh = isZhLocale(resolveLocale(locale));
  const v = String(kind ?? "").trim().toLowerCase();
  switch (v) {
    case "draft":
      return isZh ? "草稿" : "Draft";
    case "final":
      return isZh ? "最终" : "Final";
    default:
      return kind || "";
  }
}

export function fmtAgentStatus(status: string, locale?: Locale): string {
  const isZh = isZhLocale(resolveLocale(locale));
  const v = String(status ?? "").trim().toLowerCase();
  switch (v) {
    case "enabled":
      return isZh ? "启用" : "Enabled";
    case "disabled":
      return isZh ? "停用" : "Disabled";
    default:
      return status || "";
  }
}

export function fmtEventKind(kind: string, locale?: Locale): string {
  const isZh = isZhLocale(resolveLocale(locale));
  const v = String(kind ?? "").trim().toLowerCase();
  switch (v) {
    case "message":
      return isZh ? "消息" : "Message";
    case "stage_changed":
      return isZh ? "阶段切换" : "Stage changed";
    case "decision":
      return isZh ? "决策" : "Decision";
    case "summary":
      return isZh ? "总结" : "Summary";
    case "artifact_version":
      return isZh ? "作品版本" : "Artifact version";
    case "system":
      return isZh ? "系统" : "System";
    default:
      return kind || "";
  }
}
