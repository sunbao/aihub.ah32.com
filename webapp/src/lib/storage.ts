const isBrowser = typeof window !== "undefined";

export const STORAGE_KEYS = {
  userApiKey: "aihub_user_api_key",
  currentAgentId: "aihub_current_agent_id",
  currentAgentLabel: "aihub_current_agent_label",
  agentApiKeys: "aihub_agent_api_keys", // JSON map: { [agent_id]: api_key }
  baseUrl: "aihub_base_url",
  openclawProfileNames: "aihub_openclaw_profile_names", // JSON map: { [agent_id]: profile_name }
  adminToken: "aihub_admin_token",
  agentCardCatalogsVersion: "aihub_agent_card_catalogs_version",
  agentCardCatalogsJson: "aihub_agent_card_catalogs_json",
} as const;

export function getStored(key: string): string {
  if (!isBrowser) return "";
  try {
    return window.localStorage.getItem(key) ?? "";
  } catch (error) {
    console.warn("localStorage.getItem failed", { key, error });
    return "";
  }
}

export function setStored(key: string, value: string): void {
  if (!isBrowser) return;
  try {
    window.localStorage.setItem(key, value);
  } catch (error) {
    console.warn("localStorage.setItem failed", { key, error });
  }
}

export function removeStored(key: string): void {
  if (!isBrowser) return;
  try {
    window.localStorage.removeItem(key);
  } catch (error) {
    console.warn("localStorage.removeItem failed", { key, error });
  }
}

export function getUserApiKey(): string {
  return getStored(STORAGE_KEYS.userApiKey).trim();
}

export function setUserApiKey(apiKey: string): void {
  setStored(STORAGE_KEYS.userApiKey, String(apiKey ?? ""));
}

export function getAdminToken(): string {
  return getStored(STORAGE_KEYS.adminToken).trim();
}

export function setAdminToken(token: string): void {
  setStored(STORAGE_KEYS.adminToken, String(token ?? ""));
}

function getJsonMap(key: string): Record<string, string> {
  const raw = getStored(key);
  if (!raw.trim()) return {};
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return {};
    return parsed as Record<string, string>;
  } catch (error) {
    console.error("Failed to parse stored JSON map", { key, error });
    return {};
  }
}

function setJsonMap(key: string, map: Record<string, string>): void {
  setStored(key, JSON.stringify(map ?? {}));
}

export function getAgentApiKey(agentId: string): string {
  const id = String(agentId ?? "").trim();
  if (!id) return "";
  const map = getJsonMap(STORAGE_KEYS.agentApiKeys);
  return String(map[id] ?? "").trim();
}

export function setAgentApiKey(agentId: string, apiKey: string): void {
  const id = String(agentId ?? "").trim();
  const k = String(apiKey ?? "").trim();
  if (!id || !k) return;
  const map = getJsonMap(STORAGE_KEYS.agentApiKeys);
  map[id] = k;
  setJsonMap(STORAGE_KEYS.agentApiKeys, map);
}

export function deleteAgentApiKey(agentId: string): void {
  const id = String(agentId ?? "").trim();
  if (!id) return;
  const map = getJsonMap(STORAGE_KEYS.agentApiKeys);
  if (!(id in map)) return;
  delete map[id];
  setJsonMap(STORAGE_KEYS.agentApiKeys, map);
}

export function getOpenclawProfileName(agentId: string): string {
  const id = String(agentId ?? "").trim();
  if (!id) return "";
  const map = getJsonMap(STORAGE_KEYS.openclawProfileNames);
  return String(map[id] ?? "").trim();
}

export function setOpenclawProfileName(agentId: string, profileName: string): void {
  const id = String(agentId ?? "").trim();
  if (!id) return;
  const map = getJsonMap(STORAGE_KEYS.openclawProfileNames);
  const name = String(profileName ?? "").trim();
  if (!name) delete map[id];
  else map[id] = name;
  setJsonMap(STORAGE_KEYS.openclawProfileNames, map);
}

export function deleteOpenclawProfileName(agentId: string): void {
  const id = String(agentId ?? "").trim();
  if (!id) return;
  const map = getJsonMap(STORAGE_KEYS.openclawProfileNames);
  if (!(id in map)) return;
  delete map[id];
  setJsonMap(STORAGE_KEYS.openclawProfileNames, map);
}

export function getCurrentAgentId(): string {
  return getStored(STORAGE_KEYS.currentAgentId).trim();
}

export function setCurrentAgent(agentId: string, label: string): void {
  setStored(STORAGE_KEYS.currentAgentId, String(agentId ?? "").trim());
  setStored(STORAGE_KEYS.currentAgentLabel, sanitizeLabel(label));
}

export function clearCurrentAgent(): void {
  setStored(STORAGE_KEYS.currentAgentId, "");
  setStored(STORAGE_KEYS.currentAgentLabel, "");
}

export function getCurrentAgentLabel(): string {
  return sanitizeLabel(getStored(STORAGE_KEYS.currentAgentLabel));
}

function isUUIDLike(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    String(value ?? "").trim(),
  );
}

export function sanitizeLabel(label: string): string {
  const t = String(label ?? "").trim();
  if (!t) return "";
  const parts = t
    .split(" / ")
    .map((s) => s.trim())
    .filter(Boolean);
  if (parts.length >= 2 && isUUIDLike(parts[parts.length - 1])) {
    return parts.slice(0, -1).join(" / ");
  }
  return t;
}
