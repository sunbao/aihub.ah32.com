export function $(id) {
  const el = document.getElementById(id);
  if (!el) throw new Error(`Missing element: #${id}`);
  return el;
}

export function getStored(key) {
  try {
    return window.localStorage.getItem(key) ?? "";
  } catch (error) {
    console.warn("localStorage.getItem failed", { key, error });
    return "";
  }
}

export function setStored(key, value) {
  try {
    window.localStorage.setItem(key, String(value ?? ""));
  } catch (error) {
    console.warn("localStorage.setItem failed", { key, error });
  }
}

export function renderNotice(el, message, danger = false) {
  if (!el) return;
  el.textContent = String(message ?? "");
  el.className = `notice${danger ? " danger" : ""}`;
}

export async function copyText(text) {
  const value = String(text ?? "");
  if (!value) return false;
  try {
    await navigator.clipboard.writeText(value);
    return true;
  } catch (error) {
    console.debug("navigator.clipboard.writeText failed, fallback to execCommand", error);
  }

  try {
    const ta = document.createElement("textarea");
    ta.value = value;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    ta.style.top = "-9999px";
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, ta.value.length);
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch (error) {
    console.warn("copy fallback failed", error);
    return false;
  }
}

export async function apiFetch(path, opts = {}) {
  const method = String(opts.method ?? "GET").toUpperCase();
  const apiKey = String(opts.apiKey ?? "").trim();
  const body = opts.body === undefined ? undefined : JSON.stringify(opts.body);

  const headers = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (apiKey) headers.Authorization = `Bearer ${apiKey}`;

  const res = await fetch(path, { method, headers, body });
  const text = await res.text();
  let json;
  try {
    json = text ? JSON.parse(text) : undefined;
  } catch (error) {
    console.debug("Failed to parse response as JSON", { path, status: res.status, error });
    json = undefined;
  }

  if (!res.ok) {
    const code = typeof json?.error === "string" ? String(json.error) : "";
    const err = new Error(code || text || `HTTP ${res.status}`);
    err.status = res.status;
    err.body = json;
    err.code = code;
    console.warn("API request failed", { path, status: res.status, code });
    throw err;
  }

  return json;
}

export function fmtAgentStatus(status) {
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

export function fmtEventKind(kind) {
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

export function fmtArtifactKind(kind) {
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

