import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

type BeforeInstallPromptEvent = Event & {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed"; platform?: string }>;
};

const dismissKey = "aihub_pwa_install_dismissed_v1";

function isStandalone(): boolean {
  try {
    if (window.matchMedia?.("(display-mode: standalone)")?.matches) return true;
  } catch {
    // ignore
  }
  return Boolean((window.navigator as any).standalone);
}

function isIOS(): boolean {
  const ua = String(window.navigator.userAgent || "");
  const iOSDevice = /iPad|iPhone|iPod/.test(ua);
  const iPadOS = /Macintosh/.test(ua) && "ontouchend" in document;
  return iOSDevice || iPadOS;
}

function getDismissed(): boolean {
  try {
    return localStorage.getItem(dismissKey) === "1";
  } catch {
    return false;
  }
}

function setDismissed() {
  try {
    localStorage.setItem(dismissKey, "1");
  } catch {
    // eslint-disable-next-line no-console
    console.warn("[AIHub] failed to persist PWA banner dismissal");
  }
}

export function PwaInstallBanner({ className }: { className?: string }) {
  const [dismissed, setDismissedState] = useState(getDismissed);
  const [deferred, setDeferred] = useState<BeforeInstallPromptEvent | null>(null);
  const [installing, setInstalling] = useState(false);

  useEffect(() => {
    if (dismissed) return;
    if (isStandalone()) return;

    const onBeforeInstallPrompt = (e: Event) => {
      e.preventDefault();
      setDeferred(e as BeforeInstallPromptEvent);
    };
    const onAppInstalled = () => {
      setDeferred(null);
      setDismissed();
      setDismissedState(true);
    };

    window.addEventListener("beforeinstallprompt", onBeforeInstallPrompt as any);
    window.addEventListener("appinstalled", onAppInstalled);
    return () => {
      window.removeEventListener("beforeinstallprompt", onBeforeInstallPrompt as any);
      window.removeEventListener("appinstalled", onAppInstalled);
    };
  }, [dismissed]);

  const showIOSHint = useMemo(() => !dismissed && !isStandalone() && isIOS(), [dismissed]);
  const show = Boolean(!dismissed && !isStandalone() && (deferred || showIOSHint));
  if (!show) return null;

  return (
    <div
      className={cn("fixed left-0 right-0 z-40 px-3", className)}
      style={{ bottom: "calc(env(safe-area-inset-bottom) + 72px)" }}
    >
      <div className="mx-auto max-w-md">
        <Card>
          <CardContent className="flex items-start justify-between gap-3 py-3">
            <div className="min-w-0">
              <div className="text-sm font-semibold">添加到主屏幕</div>
              <div className="mt-1 text-xs text-muted-foreground">
                {deferred
                  ? "把 AIHub 添加到主屏幕，像 App 一样打开。"
                  : "iOS：在 Safari 中点“分享”→“添加到主屏幕”。"}
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              {deferred ? (
                <Button
                  size="sm"
                  disabled={installing}
                  onClick={async () => {
                    setInstalling(true);
                    try {
                      await deferred.prompt();
                      const choice = await deferred.userChoice;
                      if (choice?.outcome === "accepted") {
                        setDismissed();
                        setDismissedState(true);
                        setDeferred(null);
                      }
                    } catch (err) {
                      // eslint-disable-next-line no-console
                      console.warn("[AIHub] PWA install prompt failed", err);
                    } finally {
                      setInstalling(false);
                      setDeferred(null);
                    }
                  }}
                >
                  安装
                </Button>
              ) : null}
              <Button
                size="sm"
                variant="secondary"
                onClick={() => {
                  setDismissed();
                  setDismissedState(true);
                }}
              >
                {deferred ? "稍后" : "知道了"}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

