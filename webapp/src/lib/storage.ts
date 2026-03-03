const isBrowser = typeof window !== "undefined";

export const STORAGE_KEYS = {
  userApiKey: "aihub_user_api_key",
  agentApiKeys: "aihub_agent_api_keys", // JSON map: { [agent_ref]: api_key }
  baseUrl: "aihub_base_url",
  openclawProfileNames: "aihub_openclaw_profile_names", // JSON map: { [agent_ref]: profile_name }
  agentCardCatalogsVersion: "aihub_agent_card_catalogs_version",
  agentCardCatalogsEtag: "aihub_agent_card_catalogs_etag",
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

export function getAgentApiKey(agentRef: string): string {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return "";
  const map = getJsonMap(STORAGE_KEYS.agentApiKeys);
  return String(map[ref] ?? "").trim();
}

export function setAgentApiKey(agentRef: string, apiKey: string): void {
  const ref = String(agentRef ?? "").trim();
  const k = String(apiKey ?? "").trim();
  if (!ref || !k) return;
  const map = getJsonMap(STORAGE_KEYS.agentApiKeys);
  map[ref] = k;
  setJsonMap(STORAGE_KEYS.agentApiKeys, map);
}

export function deleteAgentApiKey(agentRef: string): void {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return;
  const map = getJsonMap(STORAGE_KEYS.agentApiKeys);
  if (!(ref in map)) return;
  delete map[ref];
  setJsonMap(STORAGE_KEYS.agentApiKeys, map);
}

export function getOpenclawProfileName(agentRef: string): string {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return "";
  const map = getJsonMap(STORAGE_KEYS.openclawProfileNames);
  return String(map[ref] ?? "").trim();
}

export function setOpenclawProfileName(agentRef: string, profileName: string): void {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return;
  const map = getJsonMap(STORAGE_KEYS.openclawProfileNames);
  const name = String(profileName ?? "").trim();
  if (!name) delete map[ref];
  else map[ref] = name;
  setJsonMap(STORAGE_KEYS.openclawProfileNames, map);
}

export function deleteOpenclawProfileName(agentRef: string): void {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return;
  const map = getJsonMap(STORAGE_KEYS.openclawProfileNames);
  if (!(ref in map)) return;
  delete map[ref];
  setJsonMap(STORAGE_KEYS.openclawProfileNames, map);
}
