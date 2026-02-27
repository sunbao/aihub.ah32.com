import { useEffect, useMemo, useRef } from "react";
import type { PropsWithChildren } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";

import { App as CapApp } from "@capacitor/app";
import { Browser } from "@capacitor/browser";
import { Capacitor } from "@capacitor/core";

import { ChevronLeft, Home, Moon, Sun, User } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/toaster";
import { PwaInstallBanner } from "@/app/components/PwaInstallBanner";
import { useTheme } from "@/hooks/use-theme";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { NAMING } from "@/lib/naming";
import { setUserApiKey } from "@/lib/storage";
import { cn } from "@/lib/utils";

function useAppTitle(pathname: string): { title: string; showBack: boolean; backTo?: string } {
  if (pathname.startsWith("/runs/")) return { title: "任务详情", showBack: true, backTo: "/" };
  if (pathname === "/runs") return { title: "任务列表", showBack: true, backTo: "/" };
  if (pathname.startsWith("/agents/") && pathname.endsWith("/timeline")) return { title: "时间线", showBack: true, backTo: "/me" };
  if (pathname.startsWith("/agents/")) return { title: NAMING.nouns.agent, showBack: true, backTo: "/" };
  if (pathname.startsWith("/curations")) return { title: "策展广场", showBack: true, backTo: "/" };
  if (pathname.startsWith("/admin/")) return { title: "管理员", showBack: true, backTo: "/me" };
  if (pathname.startsWith("/me")) return { title: NAMING.tabs.me, showBack: false };
  return { title: NAMING.tabs.square, showBack: false };
}

function parseAppGitHubExchangeToken(urlStr: string): string {
  try {
    const u = new URL(urlStr);
    const scheme = String(u.protocol || "").replace(":", "").toLowerCase();
    const host = String(u.hostname || "").toLowerCase();
    const pathname = String(u.pathname || "").toLowerCase();
    if (scheme !== "aihub") return "";
    if (host !== "auth") return "";
    if (!pathname.startsWith("/github")) return "";
    return String(u.searchParams.get("exchange_token") || "").trim();
  } catch (e) {
    console.debug("[AIHub] parse deep link failed", e);
    return "";
  }
}

function BottomNav() {
  const { pathname } = useLocation();
  const active = pathname.startsWith("/me") || pathname.startsWith("/admin") ? "me" : "square";
  return (
    <nav
      className={cn(
        "fixed bottom-0 left-0 right-0 z-30 border-t border-border/40 bg-background/80 backdrop-blur-md",
        "pb-[env(safe-area-inset-bottom)] transition-all duration-300",
      )}
    >
      <div className="mx-auto grid max-w-md grid-cols-2 px-3 py-1">
        <Link to="/" className="block">
          <Button
            variant="ghost"
            className={cn("w-full flex-col gap-0.5 h-14 justify-center", active === "square" && "text-primary")}
          >
            <Home className={cn("h-5 w-5", active === "square" ? "fill-primary stroke-primary" : "stroke-muted-foreground")} />
            <span className={cn("text-[10px] font-medium", active === "square" ? "text-primary" : "text-muted-foreground")}>
              {NAMING.tabs.square}
            </span>
          </Button>
        </Link>
        <Link to="/me" className="block">
          <Button
            variant="ghost"
            className={cn("w-full flex-col gap-0.5 h-14 justify-center", active === "me" && "text-primary")}
          >
            <User className={cn("h-5 w-5", active === "me" ? "fill-primary stroke-primary" : "stroke-muted-foreground")} />
            <span className={cn("text-[10px] font-medium", active === "me" ? "text-primary" : "text-muted-foreground")}>
              {NAMING.tabs.me}
            </span>
          </Button>
        </Link>
      </div>
    </nav>
  );
}

export function AppShell({ children }: PropsWithChildren) {
  const { pathname } = useLocation();
  const nav = useNavigate();
  const { toast } = useToast();
  const { resolved, setTheme } = useTheme();
  const meta = useMemo(() => useAppTitle(pathname), [pathname]);
  const lastExchangeToken = useRef("");

  const showBack = meta.showBack;
  const backTo = meta.backTo ?? "/";

  useEffect(() => {
    if (!Capacitor.isNativePlatform()) return;

    let disposed = false;

    async function applyExchangeToken(exchangeToken: string) {
      if (!exchangeToken) return;
      if (lastExchangeToken.current === exchangeToken) return;
      lastExchangeToken.current = exchangeToken;

      try {
        const res = await apiFetchJson<{ api_key?: string }>("/v1/auth/app/exchange", {
          method: "POST",
          apiKey: "",
          body: { exchange_token: exchangeToken },
        });
        const apiKey = String(res?.api_key ?? "").trim();
        if (!apiKey) throw new Error("登录失败：缺少 api_key");

        setUserApiKey(apiKey);
        try {
          await Browser.close();
        } catch (e) {
          console.debug("[AIHub] browser close skipped", e);
        }

        if (disposed) return;
        toast({ title: "登录成功" });
        nav("/me", { replace: true });
      } catch (e: any) {
        console.warn("[AIHub] app exchange failed", e);
        if (disposed) return;
        toast({
          title: "登录失败",
          description: String(e?.message ?? ""),
          variant: "destructive",
        });
      }
    }

    function handleUrl(urlStr: string) {
      const token = parseAppGitHubExchangeToken(urlStr);
      void applyExchangeToken(token);
    }

    const subPromise = CapApp.addListener("appUrlOpen", (event) => {
      if (event?.url) handleUrl(event.url);
    });

    CapApp.getLaunchUrl()
      .then((res) => {
        if (res?.url) handleUrl(res.url);
      })
      .catch((err: unknown) => {
        console.debug("[AIHub] getLaunchUrl skipped", err);
      });

    return () => {
      disposed = true;
      subPromise
        .then((sub) => sub.remove())
        .catch((err: unknown) => console.debug("[AIHub] remove appUrlOpen listener failed", err));
    };
  }, [nav, toast]);

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="sticky top-0 z-20 border-b border-border/40 bg-background/80 backdrop-blur-md transition-all duration-300">

        <div className="mx-auto flex max-w-md items-center gap-2 px-3 py-3">
          {showBack ? (
            <Button
              variant="ghost"
              size="sm"
              className="w-[52px] px-0"
              onClick={() => {
                if (window.history.length > 1) nav(-1);
                else nav(backTo, { replace: true });
              }}
            >
              <ChevronLeft className="h-5 w-5" />
            </Button>
          ) : (
            <div className="w-[52px]" />
          )}
          <div className="flex-1 text-center text-sm font-semibold">{meta.title}</div>
          <Button
            variant="ghost"
            size="sm"
            className="w-[52px]"
            onClick={() => setTheme(resolved === "dark" ? "light" : "dark")}
            aria-label="切换主题"
          >
            {resolved === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </Button>
        </div>
      </header>

      <main className="mx-auto max-w-md px-3 py-3 pb-24">
        <div
          key={pathname}
          className="animate-in fade-in slide-in-from-bottom-4 duration-500 fill-mode-both"
        >
          {children ?? <Outlet />}
        </div>
      </main>

      <PwaInstallBanner />
      <BottomNav />
      <Toaster />
    </div>
  );
}
