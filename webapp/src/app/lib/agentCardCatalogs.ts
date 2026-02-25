import { apiFetchJson } from "@/lib/api";
import { getStored, setStored, STORAGE_KEYS } from "@/lib/storage";

export type CatalogLabeledItem = {
  id: string;
  label: string;
  category?: string;
  keywords?: string[];
};

export type CatalogTextTemplate = {
  id: string;
  label: string;
  template: string;
  min_chars?: number;
  max_chars?: number;
};

export type CatalogPersonalityPreset = {
  id: string;
  label: string;
  description?: string;
  values: { extrovert: number; curious: number; creative: number; stable: number };
};

export type AgentCardCatalogs = {
  catalog_version: string;
  personality_presets?: CatalogPersonalityPreset[];
  name_templates?: any[];
  avatar_options?: any[];
  interests?: CatalogLabeledItem[];
  capabilities?: CatalogLabeledItem[];
  bio_templates?: CatalogTextTemplate[];
  greeting_templates?: CatalogTextTemplate[];
};

function loadCachedCatalogs(): AgentCardCatalogs | null {
  const raw = getStored(STORAGE_KEYS.agentCardCatalogsJson);
  if (!raw.trim()) return null;
  try {
    const parsed = JSON.parse(raw) as AgentCardCatalogs;
    if (!parsed || typeof parsed !== "object") return null;
    if (!String(parsed.catalog_version ?? "").trim()) return null;
    return parsed;
  } catch (e) {
    console.warn("Failed to parse cached agent card catalogs", e);
    return null;
  }
}

function saveCachedCatalogs(c: AgentCardCatalogs): void {
  const v = String(c.catalog_version ?? "").trim();
  if (!v) return;
  setStored(STORAGE_KEYS.agentCardCatalogsVersion, v);
  setStored(STORAGE_KEYS.agentCardCatalogsJson, JSON.stringify(c));
}

export async function getAgentCardCatalogs(opts: {
  userApiKey: string;
  forceRefresh?: boolean;
}): Promise<AgentCardCatalogs> {
  const cached = loadCachedCatalogs();
  if (cached && !opts.forceRefresh) return cached;

  const res = await apiFetchJson<AgentCardCatalogs>("/v1/agent-card/catalogs", { apiKey: opts.userApiKey });
  saveCachedCatalogs(res);
  return res;
}

export function renderCatalogTemplate(tmpl: string, vars: { name: string; interests: string[]; capabilities: string[] }): string {
  let out = String(tmpl ?? "");
  out = out.replaceAll("{name}", String(vars.name ?? "").trim());
  out = out.replaceAll("{interests}", (vars.interests ?? []).filter(Boolean).join("、"));
  out = out.replaceAll("{capabilities}", (vars.capabilities ?? []).filter(Boolean).join("、"));
  return out.trim();
}

