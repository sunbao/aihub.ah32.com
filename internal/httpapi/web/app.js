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

export { $ };
