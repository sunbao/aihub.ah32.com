function $(id) { return document.getElementById(id); }

export function getStored(key) {
  try {
    return localStorage.getItem(key) || "";
  } catch (e) {
    console.warn("localStorage.getItem failed", { key, err: e });
    return "";
  }
}
export function setStored(key, val) {
  try {
    localStorage.setItem(key, val);
  } catch (e) {
    console.warn("localStorage.setItem failed", { key, err: e });
  }
}

function humanizeApiError(code, status, text) {
  const c = String(code || "").trim();
  const t = String(text || "").trim();

  const map = {
    unauthorized: "未登录或登录已过期，请先登录。",
    forbidden: "没有权限执行该操作。",
    "invalid run id": "任务参数无效。",
    "invalid version": "作品版本无效。",
    "not found": "未找到相关内容。",
    "no output": "暂无作品输出。",
  };
  if (c) {
    if (map[c]) return map[c];
    if (status === 401) return "未登录或登录已过期，请先登录。";
    if (status === 403) return "没有权限执行该操作。";
    if (status === 404) return "未找到相关内容。";
    if (status >= 500) return "服务器忙，请稍后再试。";

    const lc = c.toLowerCase();
    if (lc.includes("invalid") || lc.includes("missing")) return "参数不正确，请检查后重试。";
    if (lc.includes("not found")) return "未找到相关内容。";
    if (lc.includes("unauthorized")) return "未登录或登录已过期，请先登录。";

    // Unknown structured error code: do not expose technical codes to end users.
    return "操作失败，请稍后再试。";
  }

  if (status === 401) return "未登录或登录已过期，请先登录。";
  if (status === 403) return "没有权限执行该操作。";
  if (status === 404) return "未找到相关内容。";
  if (status >= 500) return "服务器忙，请稍后再试。";
  if (t) return t;

  return "请求失败，请稍后再试。";
}

export async function apiFetch(path, { method = "GET", body = null, apiKey = "" } = {}) {
  const headers = { "Content-Type": "application/json" };
  if (apiKey) headers["Authorization"] = "Bearer " + apiKey;
  const res = await fetch(path, { method, headers, body: body ? JSON.stringify(body) : null });
  const text = await res.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch (e) {
    console.debug("Failed to parse response as JSON", { path, status: res.status, err: e });
  }
  if (!res.ok) {
    const code = json?.error ? String(json.error) : "";
    const msg = humanizeApiError(code, res.status, text);
    const err = new Error(msg || "请求失败");
    err.status = res.status;
    err.code = code;
    err.body = json || text;
    console.warn("API request failed", { path, status: res.status, code });
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
  const v = String(text || "");

  // 1) Prefer async Clipboard API when available.
  // NOTE: This often fails on non-HTTPS origins (e.g. http://192.168.x.x),
  // so we keep a legacy fallback below.
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(v);
      return true;
    }
  } catch (e) {
    console.warn("navigator.clipboard.writeText failed", e);
  }

  // 2) Fallback: deprecated execCommand("copy") still works in many browsers
  // in insecure contexts, as long as it's triggered by a user gesture.
  try {
    const ta = document.createElement("textarea");
    ta.value = v;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    ta.style.left = "-1000px";
    ta.style.opacity = "0";
    document.body.appendChild(ta);

    ta.focus({ preventScroll: true });
    ta.select();
    ta.setSelectionRange(0, ta.value.length);

    const ok = document.execCommand && document.execCommand("copy");
    document.body.removeChild(ta);
    if (ok) return true;
    console.warn('document.execCommand("copy") returned false');
  } catch (e) {
    console.warn('document.execCommand("copy") failed', e);
  }

  // 3) Last resort: show a prompt so users can long-press/select to copy on mobile.
  try {
    window.prompt("复制失败，请手动复制：", v);
  } catch (e) {
    console.warn("window.prompt copy fallback failed", e);
  }
  return false;
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
