import { getStored, getUserApiKey, STORAGE_KEYS } from "@/lib/storage";

export type ApiErrorCode = string;

export type ApiFetchOptions = {
  method?: string;
  body?: unknown;
  apiKey?: string;
  signal?: AbortSignal;
};

export function normalizeApiBaseUrl(input: string): string {
  const v = String(input ?? "").trim();
  if (!v) return "";
  const raw = v.endsWith("/") ? v.slice(0, -1) : v;
  let u: URL | null = null;
  try {
    u = new URL(raw);
  } catch (_e) {
    try {
      u = new URL(`http://${raw}`);
    } catch (_e2) {
      u = null;
    }
  }
  if (!u) return raw;
  const origin = `${u.protocol}//${u.host}`;
  return origin.endsWith("/") ? origin.slice(0, -1) : origin;
}

export function getApiBaseUrl(): string {
  const env = normalizeApiBaseUrl(String(import.meta.env.VITE_API_BASE_URL ?? ""));
  if (env) return env;

  const stored = normalizeApiBaseUrl(getStored(STORAGE_KEYS.baseUrl));
  if (stored) return stored;

  const origin = normalizeApiBaseUrl(window.location.origin);
  if (!origin) return "";
  try {
    const u = new URL(origin);
    const scheme = String(u.protocol || "").replace(":", "").toLowerCase();
    const host = String(u.hostname || "").toLowerCase();
    if (scheme === "capacitor" || scheme === "ionic") return "";
    if (host === "localhost" || host === "127.0.0.1" || host === "::1") return "";
  } catch (_e) {
    // ignore
  }
  return origin;
}

function joinUrl(baseUrl: string, path: string): string {
  const b = normalizeApiBaseUrl(baseUrl);
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
    publish_gated: "暂不可发布：未满足平台发布门槛。",
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
  data: unknown;
  constructor(message: string, status: number, code: ApiErrorCode, data: unknown = undefined) {
    super(message);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = code;
    this.data = data;
  }
}

export async function apiFetchJson<T>(path: string, options: ApiFetchOptions = {}): Promise<T> {
  const method = (options.method ?? "GET").toUpperCase();
  const apiKey = String(options.apiKey ?? getUserApiKey()).trim();

  const baseUrl = getApiBaseUrl();
  if (!baseUrl) {
    throw new Error("未配置服务器地址，请到「我的」里先填写服务器地址（例如：http://你的服务器:8080）。");
  }
  const url = joinUrl(baseUrl, path);

  const headers: Record<string, string> = { Accept: "application/json" };
  const body = options.body === undefined ? undefined : JSON.stringify(options.body);
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (apiKey) headers.Authorization = `Bearer ${apiKey}`;

  const res = await fetch(url, {
    method,
    headers,
    body,
    signal: options.signal,
  });

  const contentType = String(res.headers.get("content-type") ?? "").toLowerCase();
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
    throw new ApiRequestError(msg, res.status, code, json);
  }

  if (json === undefined || json === null) {
    const looksLikeHTML =
      contentType.includes("text/html") ||
      /^<!doctype html/i.test(text) ||
      /<html[\s>]/i.test(text);
    if (looksLikeHTML) {
      throw new Error("接口返回了网页而不是数据：服务器地址可能填错了（不要带 /app 或 /ui），请到「我的」里改为 http://你的服务器:8080。");
    }
    throw new Error("接口响应为空或不是 JSON，请检查服务器是否可访问、以及服务器地址是否正确。");
  }
  return json as T;
}
