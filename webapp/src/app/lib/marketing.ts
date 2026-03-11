import { Capacitor } from "@capacitor/core";

import { getUserApiKey } from "@/lib/storage";

export function shouldShowDownloadNudge(): boolean {
  if (Capacitor.isNativePlatform()) return false;
  return !String(getUserApiKey() ?? "").trim();
}

