import type { Page } from "@playwright/test";

export function requireEnv(name: string): string {
  const v = String(process.env[name] ?? "").trim();
  if (!v) throw new Error(`Missing required env var: ${name}`);
  return v;
}

export function isLiveMode(): boolean {
  return Boolean(String(process.env.PLAYWRIGHT_BASE_URL ?? "").trim());
}

export async function initLocalStorageAuth(
  page: Page,
  args: {
    userApiKey: string;
    agentApiKeys?: Record<string, string>;
    baseUrl?: string;
  },
): Promise<void> {
  const userApiKey = String(args.userApiKey ?? "").trim();
  if (!userApiKey) throw new Error("Missing userApiKey for localStorage auth.");
  const agentApiKeys = args.agentApiKeys ?? {};
  const baseUrl = String(args.baseUrl ?? "").trim();

  await page.addInitScript(
    ({ userApiKey, agentApiKeys, baseUrl }) => {
      try {
        window.localStorage.setItem("aihub_user_api_key", String(userApiKey ?? ""));
        if (baseUrl) window.localStorage.setItem("aihub_base_url", String(baseUrl));
        if (agentApiKeys && typeof agentApiKeys === "object") {
          window.localStorage.setItem("aihub_agent_api_keys", JSON.stringify(agentApiKeys));
        }
      } catch {
        // If localStorage is blocked, the app will surface "Login required" and tests should fail fast.
      }
    },
    { userApiKey, agentApiKeys, baseUrl },
  );
}

