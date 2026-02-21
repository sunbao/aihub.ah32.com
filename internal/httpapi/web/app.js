function $(id) { return document.getElementById(id); }

export function getStored(key) {
  try { return localStorage.getItem(key) || ""; } catch { return ""; }
}
export function setStored(key, val) {
  try { localStorage.setItem(key, val); } catch {}
}

export async function apiFetch(path, { method = "GET", body = null, apiKey = "" } = {}) {
  const headers = { "Content-Type": "application/json" };
  if (apiKey) headers["Authorization"] = "Bearer " + apiKey;
  const res = await fetch(path, { method, headers, body: body ? JSON.stringify(body) : null });
  const text = await res.text();
  let json = null;
  try { json = text ? JSON.parse(text) : null; } catch {}
  if (!res.ok) {
    const msg = json?.error ? json.error : (text || ("HTTP " + res.status));
    const err = new Error(msg);
    err.status = res.status;
    err.body = json || text;
    throw err;
  }
  return json;
}

export function renderJSON(el, obj) {
  el.textContent = JSON.stringify(obj, null, 2);
}

export function renderNotice(el, msg, isError = false) {
  el.className = "notice" + (isError ? " error" : "");
  el.textContent = msg;
}

export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

export function fmtRunStatus(status) {
  const v = (status || "").trim().toLowerCase();
  switch (v) {
    case "created": return "已创建";
    case "running": return "进行中";
    case "completed": return "已完成";
    case "failed": return "失败";
    default: return status || "";
  }
}

export function fmtAgentStatus(status) {
  const v = (status || "").trim().toLowerCase();
  switch (v) {
    case "enabled": return "启用";
    case "disabled": return "停用";
    default: return status || "";
  }
}

export function fmtEventKind(kind) {
  const v = (kind || "").trim().toLowerCase();
  switch (v) {
    case "message": return "消息";
    case "stage_changed": return "阶段切换";
    case "decision": return "决策";
    case "summary": return "总结";
    case "artifact_version": return "作品版本";
    case "system": return "系统";
    default: return kind || "";
  }
}

export function fmtArtifactKind(kind) {
  const v = (kind || "").trim().toLowerCase();
  switch (v) {
    case "draft": return "草稿";
    case "final": return "最终";
    default: return kind || "";
  }
}

export function fmtOutputVersion(version) {
  const v = Number(version || 0);
  return v > 0 ? `作品版本 ${v}` : "暂无作品";
}

export { $ };
