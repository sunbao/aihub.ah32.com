import { getStored, getUserApiKey, STORAGE_KEYS } from "@/lib/storage";
import { getPreferredLocale, isZhLocale } from "@/lib/i18n";

export type ApiErrorCode = string;

export type ApiFetchOptions = {
  method?: string;
  body?: unknown;
  apiKey?: string;
  signal?: AbortSignal;
};

function tryParseUrl(urlStr: string): URL | null {
  try {
    return new URL(urlStr);
  } catch (error) {
    console.debug("[AIHub] Invalid URL", { urlStr, error });
    return null;
  }
}

export function normalizeApiBaseUrl(input: string): string {
  const v = String(input ?? "").trim();
  if (!v) return "";
  const raw = v.endsWith("/") ? v.slice(0, -1) : v;
  const u = tryParseUrl(raw) ?? tryParseUrl(`http://${raw}`);
  if (!u) return raw;
  const origin = `${u.protocol}//${u.host}`;
  return origin.endsWith("/") ? origin.slice(0, -1) : origin;
}

export function getApiBaseUrl(): string {
  const env = normalizeApiBaseUrl(String(import.meta.env.VITE_API_BASE_URL ?? ""));
  if (env) return env;

  const origin = normalizeApiBaseUrl(window.location.origin);
  if (origin) {
    try {
      const u = new URL(origin);
      const scheme = String(u.protocol || "").replace(":", "").toLowerCase();
      const host = String(u.hostname || "").toLowerCase();
      if (scheme === "http" || scheme === "https") {
        if (host !== "localhost" && host !== "127.0.0.1" && host !== "::1") return origin;
      }
      if (scheme === "capacitor" || scheme === "ionic") {
        // fall through to stored override (native / local assets)
      } else if (host === "localhost" || host === "127.0.0.1" || host === "::1") {
        // fall through to stored override (local dev)
      } else {
        // Unknown scheme but we do have an origin; use it as a best-effort default.
        return origin;
      }
    } catch (error) {
      console.debug("[AIHub] getApiBaseUrl failed to parse origin as URL", { origin, error });
      return origin;
    }
  }

  const stored = normalizeApiBaseUrl(getStored(STORAGE_KEYS.baseUrl));
  if (stored) return stored;

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

  const isZh = isZhLocale(getPreferredLocale());
  const map: Record<string, string> = isZh
    ? {
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
        "no evaluation judges configured": "测评智能体未配置，请联系管理员。",
        "evaluation limit reached": "今日测评次数已达上限，请稍后再试。",
      }
    : {
        unauthorized: "Not logged in or session expired. Please sign in.",
        forbidden: "You don't have permission to perform this action.",
        publish_gated: "Publishing is not available yet (platform gating).",
        "invalid run id": "Invalid run parameter. Please go back and retry.",
        "invalid agent id": "Invalid agent parameter. Please go back and retry.",
        "invalid version": "Invalid artifact version.",
        "not found": "Not found.",
        "no output": "No output yet.",
        "platform signing not configured": "Platform signing is not configured. Please contact the admin.",
        "oss not configured": "OSS is not configured. Please contact the admin.",
        "no evaluation judges configured": "No judge agents configured. Please contact the admin.",
        "evaluation limit reached": "Evaluation limit reached for today. Please try again later.",
      };

  if (c) {
    if (map[c]) return map[c];
    if (status === 401) return map.unauthorized;
    if (status === 403) return map.forbidden;
    if (status === 404) return map["not found"];
    if (status >= 500) return isZh ? "服务繁忙，请稍后再试。" : "Server is busy. Please try again later.";
    return isZh ? "操作失败，请稍后再试。" : "Request failed. Please try again later.";
  }

  if (status === 401) return map.unauthorized;
  if (status === 403) return map.forbidden;
  if (status === 404) return map["not found"];
  if (status >= 500) return isZh ? "服务繁忙，请稍后再试。" : "Server is busy. Please try again later.";
  if (t) return t;
  return isZh ? "请求失败，请稍后再试。" : "Request failed. Please try again later.";
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
  const isZh = isZhLocale(getPreferredLocale());

  const baseUrl = getApiBaseUrl();
  if (!baseUrl) {
    throw new Error(
      isZh
        ? "无法确定服务器地址：请从 AIHub 服务端的 /app 入口打开（例如：http://你的服务器:8080/app/）。"
        : "Server address is unavailable. Please open the console from your AIHub server at /app (e.g. http://your-server:8080/app/).",
    );
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
      throw new Error(
        isZh
          ? "接口返回了网页而不是数据：当前接口地址可能配置错误（不要把 /app 当作接口地址）。"
          : "The API returned HTML instead of JSON. The API base URL might be wrong (do not use /app as the API base URL).",
      );
    }
    throw new Error(
      isZh
        ? "接口响应为空或不是 JSON，请检查服务器是否可访问、以及服务器地址是否正确。"
        : "The API response is empty or not JSON. Please check server connectivity and the server address.",
    );
  }
  return json as T;
}
