import { getApiBaseUrl } from "@/lib/api";
import { getPreferredLocale, isZhLocale } from "@/lib/i18n";
import { getStored, setStored, STORAGE_KEYS } from "@/lib/storage";

export type CatalogLabeledItem = {
  id: string;
  label: string;
  label_en?: string;
  category?: string;
  category_en?: string;
  keywords?: string[];
  keywords_en?: string[];
};

export type CatalogTextTemplate = {
  id: string;
  label: string;
  label_en?: string;
  template: string;
  template_en?: string;
  min_chars?: number;
  max_chars?: number;
};

export type CatalogPersonalityPreset = {
  id: string;
  label: string;
  label_en?: string;
  description?: string;
  description_en?: string;
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

type CachedCatalogs = {
  catalogs: AgentCardCatalogs;
  etag: string;
};

function joinUrl(baseUrl: string, path: string): string {
  const b = String(baseUrl ?? "").trim().replace(/\/+$/, "");
  const p = String(path ?? "");
  if (!p) return b;
  if (!p.startsWith("/")) return `${b}/${p}`;
  return `${b}${p}`;
}

function humanizeCatalogsError(opts: { status: number; code: string; fallbackText: string; isZh: boolean }): string {
  if (opts.code === "unauthorized" || opts.status === 401) {
    return opts.isZh ? "未登录或登录已过期，请先登录。" : "Not logged in or session expired. Please sign in.";
  }
  if (opts.code === "forbidden" || opts.status === 403) {
    return opts.isZh ? "没有权限执行该操作。" : "You don't have permission to perform this action.";
  }
  if (opts.status >= 500) return opts.isZh ? "服务繁忙，请稍后再试。" : "Server is busy. Please try again later.";
  const t = String(opts.fallbackText ?? "").trim();
  return t || (opts.isZh ? "请求失败，请稍后再试。" : "Request failed. Please try again later.");
}

function loadCachedCatalogs(): CachedCatalogs | null {
  const raw = getStored(STORAGE_KEYS.agentCardCatalogsJson);
  if (!raw.trim()) return null;
  try {
    const parsed = JSON.parse(raw) as AgentCardCatalogs;
    if (!parsed || typeof parsed !== "object") return null;
    if (!String(parsed.catalog_version ?? "").trim()) return null;
    return {
      catalogs: parsed,
      etag: String(getStored(STORAGE_KEYS.agentCardCatalogsEtag) ?? "").trim(),
    };
  } catch (e) {
    console.warn("Failed to parse cached agent card catalogs", e);
    return null;
  }
}

function saveCachedCatalogs(c: AgentCardCatalogs, etag: string): void {
  const v = String(c.catalog_version ?? "").trim();
  if (!v) return;
  setStored(STORAGE_KEYS.agentCardCatalogsVersion, v);
  setStored(STORAGE_KEYS.agentCardCatalogsJson, JSON.stringify(c));
  if (etag) setStored(STORAGE_KEYS.agentCardCatalogsEtag, etag);
}

export async function getAgentCardCatalogs(opts: {
  userApiKey: string;
  forceRefresh?: boolean;
}): Promise<AgentCardCatalogs> {
  const cached = loadCachedCatalogs();
  const isZh = isZhLocale(getPreferredLocale());

  const baseUrl = getApiBaseUrl();
  if (!baseUrl) {
    throw new Error(
      isZh
        ? "无法确定服务器地址：请从 AIHub 服务端的 /app 入口打开（例如：http://你的服务器:8080/app/）。"
        : "Server address is unavailable. Please open the console from your AIHub server at /app (e.g. http://your-server:8080/app/).",
    );
  }

  const url = joinUrl(baseUrl, "/v1/agent-card/catalogs");
  const apiKey = String(opts.userApiKey ?? "").trim();
  const headers: Record<string, string> = { Accept: "application/json" };
  if (apiKey) headers.Authorization = `Bearer ${apiKey}`;
  if (cached?.etag && !opts.forceRefresh) headers["If-None-Match"] = cached.etag;

  let res: Response;
  try {
    res = await fetch(url, { method: "GET", headers, cache: "no-cache" });
  } catch (error) {
    console.warn("Failed to fetch agent card catalogs; falling back to cache", { error });
    if (cached) return cached.catalogs;
    throw error;
  }

  if (res.status === 304 && cached) return cached.catalogs;

  const etag = String(res.headers.get("etag") ?? "").trim();
  const text = await res.text();
  let json: unknown = undefined;
  try {
    json = text ? JSON.parse(text) : undefined;
  } catch (error) {
    console.warn("Failed to parse agent card catalogs response as JSON", { status: res.status, error });
  }

  if (!res.ok) {
    const code = typeof (json as any)?.error === "string" ? String((json as any).error) : "";
    const msg = humanizeCatalogsError({ status: res.status, code, fallbackText: text, isZh });
    if (cached) {
      console.warn("Agent card catalogs fetch failed; using cache", { status: res.status, code });
      return cached.catalogs;
    }
    throw new Error(msg);
  }

  if (!json || typeof json !== "object") {
    if (cached) return cached.catalogs;
    throw new Error(isZh ? "目录数据不可用" : "Catalogs unavailable");
  }

  const catalogs = json as AgentCardCatalogs;
  if (!String(catalogs.catalog_version ?? "").trim()) {
    if (cached) return cached.catalogs;
    throw new Error(isZh ? "目录数据不可用" : "Catalogs unavailable");
  }

  saveCachedCatalogs(catalogs, etag);
  return catalogs;
}

export function renderCatalogTemplate(
  tmpl: string,
  vars: { name: string; interests: string[]; capabilities: string[] },
  opts: { joiner?: string } = {},
): string {
  const joiner = String(opts.joiner ?? "、");
  let out = String(tmpl ?? "");
  out = out.replaceAll("{name}", String(vars.name ?? "").trim());
  out = out.replaceAll("{interests}", (vars.interests ?? []).filter(Boolean).join(joiner));
  out = out.replaceAll("{capabilities}", (vars.capabilities ?? []).filter(Boolean).join(joiner));
  return out.trim();
}
