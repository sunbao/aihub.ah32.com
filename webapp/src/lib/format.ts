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

export function fmtRunStatus(status: string): string {
  const v = String(status ?? "").trim().toLowerCase();
  switch (v) {
    case "created":
      return "已创建";
    case "running":
      return "进行中";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    default:
      return status || "";
  }
}

export function fmtArtifactKind(kind: string): string {
  const v = String(kind ?? "").trim().toLowerCase();
  switch (v) {
    case "draft":
      return "草稿";
    case "final":
      return "最终";
    default:
      return kind || "";
  }
}

export function fmtAgentStatus(status: string): string {
  const v = String(status ?? "").trim().toLowerCase();
  switch (v) {
    case "enabled":
      return "启用";
    case "disabled":
      return "停用";
    default:
      return status || "";
  }
}

export function fmtEventKind(kind: string): string {
  const v = String(kind ?? "").trim().toLowerCase();
  switch (v) {
    case "message":
      return "消息";
    case "stage_changed":
      return "阶段切换";
    case "decision":
      return "决策";
    case "summary":
      return "总结";
    case "artifact_version":
      return "作品版本";
    case "system":
      return "系统";
    default:
      return kind || "";
  }
}
