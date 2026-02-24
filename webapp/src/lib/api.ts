import { getStored, getUserApiKey, STORAGE_KEYS } from "@/lib/storage";

export type ApiErrorCode = string;

export type ApiFetchOptions = {
  method?: string;
  body?: unknown;
  apiKey?: string;
  signal?: AbortSignal;
};

function normalizeBaseUrl(input: string): string {
  const v = String(input ?? "").trim();
  if (!v) return "";
  return v.endsWith("/") ? v.slice(0, -1) : v;
}

export function getApiBaseUrl(): string {
  const env = normalizeBaseUrl(String(import.meta.env.VITE_API_BASE_URL ?? ""));
  if (env) return env;

  const stored = normalizeBaseUrl(getStored(STORAGE_KEYS.baseUrl));
  if (stored) return stored;

  return normalizeBaseUrl(window.location.origin);
}

function joinUrl(baseUrl: string, path: string): string {
  const b = normalizeBaseUrl(baseUrl);
  const p = String(path ?? "");
  if (!p.startsWith("/")) return `${b}/${p}`;
  return `${b}${p}`;
}

function humanizeApiError(code: ApiErrorCode, status: number, fallbackText: string): string {
  const c = String(code ?? "").trim();
  const t = String(fallbackText ?? "").trim();

  const map: Record<string, string> = {
    unauthorized: "未登录或登录已过期，请先登录。",
    forbidden: "没有权限执行该操作。",
    "invalid run id": "任务参数无效，请返回重试。",
    "invalid agent id": "智能体参数无效，请返回重试。",
    "invalid version": "作品版本无效。",
    "not found": "未找到相关内容。",
    "no output": "暂无作品输出。",
    "platform signing not configured": "平台签名配置缺失，请联系管理员。",
    "oss not configured": "OSS 尚未配置，请联系管理员。",
  };

  if (c) {
    if (map[c]) return map[c];
    if (status === 401) return map.unauthorized;
    if (status === 403) return map.forbidden;
    if (status === 404) return map["not found"];
    if (status >= 500) return "服务繁忙，请稍后再试。";
    return "操作失败，请稍后再试。";
  }

  if (status === 401) return map.unauthorized;
  if (status === 403) return map.forbidden;
  if (status === 404) return map["not found"];
  if (status >= 500) return "服务繁忙，请稍后再试。";
  if (t) return t;
  return "请求失败，请稍后再试。";
}

export class ApiRequestError extends Error {
  status: number;
  code: ApiErrorCode;
  constructor(message: string, status: number, code: ApiErrorCode) {
    super(message);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = code;
  }
}

export async function apiFetchJson<T>(path: string, options: ApiFetchOptions = {}): Promise<T> {
  const method = (options.method ?? "GET").toUpperCase();
  const apiKey = String(options.apiKey ?? getUserApiKey()).trim();

  const baseUrl = getApiBaseUrl();
  const url = joinUrl(baseUrl, path);

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (apiKey) headers.Authorization = `Bearer ${apiKey}`;

  const res = await fetch(url, {
    method,
    headers,
    body: options.body ? JSON.stringify(options.body) : undefined,
    signal: options.signal,
  });

  const text = await res.text();
  let json: unknown = undefined;
  try {
    json = text ? JSON.parse(text) : undefined;
  } catch (error) {
    console.debug("Failed to parse response as JSON", { path, status: res.status, error });
  }

  if (!res.ok) {
    const code =
      typeof (json as any)?.error === "string"
        ? String((json as any).error)
        : "";
    const msg = humanizeApiError(code, res.status, text);
    console.warn("API request failed", { path, status: res.status, code });
    throw new ApiRequestError(msg, res.status, code);
  }

  return json as T;
}

