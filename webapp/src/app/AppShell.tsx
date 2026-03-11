import { useEffect, useRef } from "react";
import type { PropsWithChildren } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";

import { App as CapApp } from "@capacitor/app";
import { Browser } from "@capacitor/browser";
import { Capacitor } from "@capacitor/core";

import { ChevronLeft, Download, Home, Moon, Sun, User } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/toaster";
import { PwaInstallBanner } from "@/app/components/PwaInstallBanner";
import { shouldShowDownloadNudge } from "@/app/lib/marketing";
import { useTheme } from "@/hooks/use-theme";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { getUserApiKey, setUserApiKey } from "@/lib/storage";
import { cn } from "@/lib/utils";

function getAppTitleMeta(pathname: string, isZh: boolean): { title: string; showBack: boolean; backTo?: string } {
  if (pathname.startsWith("/runs/")) return { title: isZh ? "任务详情" : "Run details", showBack: true, backTo: "/" };
  if (pathname === "/runs") return { title: isZh ? "任务列表" : "Runs", showBack: true, backTo: "/" };
  if (pathname.startsWith("/topics")) return { title: isZh ? "话题" : "Topics", showBack: true, backTo: "/" };
  if (pathname === "/download") return { title: isZh ? "下载 App" : "Download", showBack: true, backTo: "/" };
  if (pathname.startsWith("/agents/") && pathname.endsWith("/timeline"))
    return { title: isZh ? "时间线" : "Timeline", showBack: true, backTo: "/me" };
  if (pathname.startsWith("/agents/")) return { title: isZh ? "智能体" : "Agent", showBack: true, backTo: "/" };
  if (pathname.startsWith("/curations")) return { title: isZh ? "策展广场" : "Curations", showBack: true, backTo: "/" };
  if (pathname === "/admin") return { title: isZh ? "管理员" : "Admin", showBack: false };
  if (pathname.startsWith("/admin/")) return { title: isZh ? "管理员" : "Admin", showBack: true, backTo: "/admin" };
  if (pathname.startsWith("/me")) return { title: isZh ? "我的" : "Me", showBack: false };
  return { title: isZh ? "广场" : "Square", showBack: false };
}

function parseAppGitHubExchangeToken(urlStr: string): string {
  try {
    const u = new URL(urlStr);
    const scheme = String(u.protocol || "").replace(":", "").toLowerCase();
    const host = String(u.hostname || "").toLowerCase();
    const pathname = String(u.pathname || "").toLowerCase();
    // Some Android browsers may deliver an `intent://...` URL string; accept it as well.
    if (scheme !== "aihub" && scheme !== "intent") return "";
    if (host !== "auth") return "";
    if (!pathname.startsWith("/github")) return "";
    return String(u.searchParams.get("exchange_token") || "").trim();
  } catch (e) {
    console.debug("[AIHub] parse deep link failed", e);
    return "";
  }
}

function BottomNav({ squareLabel, meLabel }: { squareLabel: string; meLabel: string }) {
  const { pathname } = useLocation();
  const isLoggedIn = !!getUserApiKey();
  const meHref = isLoggedIn ? "/me" : "/admin";
  const active = pathname.startsWith("/me") || pathname.startsWith("/admin") ? "me" : "square";
  return (
    <nav
      className={cn(
        "fixed bottom-0 left-0 right-0 z-30 border-t border-border/40 bg-background/80 backdrop-blur-md",
        "lg:hidden",
        "pb-[env(safe-area-inset-bottom)] transition-all duration-300",
      )}
    >
      <div className="mx-auto grid max-w-md grid-cols-2 px-3 py-1 lg:max-w-5xl">
        <Link to="/" className="block">
          <Button
            variant="ghost"
            className={cn("w-full flex-col gap-0.5 h-14 justify-center", active === "square" && "text-primary")}
          >
            <Home className={cn("h-5 w-5", active === "square" ? "fill-primary stroke-primary" : "stroke-muted-foreground")} />
            <span className={cn("text-[10px] font-medium", active === "square" ? "text-primary" : "text-muted-foreground")}>
              {squareLabel}
            </span>
          </Button>
        </Link>
        <Link to={meHref} className="block">
          <Button
            variant="ghost"
            className={cn("w-full flex-col gap-0.5 h-14 justify-center", active === "me" && "text-primary")}
          >
            <User className={cn("h-5 w-5", active === "me" ? "fill-primary stroke-primary" : "stroke-muted-foreground")} />
            <span className={cn("text-[10px] font-medium", active === "me" ? "text-primary" : "text-muted-foreground")}>
              {meLabel}
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
  const { t, isZh } = useI18n();
  const { resolved, setTheme } = useTheme();
  const meta = getAppTitleMeta(pathname, isZh);
  const lastExchangeToken = useRef("");
  const showDownloadNudge = shouldShowDownloadNudge();
  const isLoggedIn = Boolean(String(getUserApiKey() ?? "").trim());

  const showBack = meta.showBack;
  const backTo = meta.backTo ?? "/";

  useEffect(() => {
    const title = String(meta.title ?? "").trim();
    if (!title) return;
    try {
      document.title = `AIHub · ${title}`;
    } catch (error) {
      console.debug("[AIHub] document.title update skipped", error);
    }
  }, [meta.title]);

  useEffect(() => {
    if (!Capacitor.isNativePlatform()) return;

    let disposed = false;

    async function applyExchangeToken(exchangeToken: string) {
      if (!exchangeToken) return;
      if (lastExchangeToken.current === exchangeToken) return;
      lastExchangeToken.current = exchangeToken;

      let apiKey = "";
      try {
        const res = await apiFetchJson<{ api_key?: string }>("/v1/auth/app/exchange", {
          method: "POST",
          apiKey: "",
          body: { exchange_token: exchangeToken },
        });
        apiKey = String(res?.api_key ?? "").trim();
      } catch (e: any) {
        console.warn("[AIHub] app exchange failed", e);
        if (disposed) return;
        toast({
          title: t({ zh: "登录失败", en: "Sign-in failed" }),
          description: String(e?.message ?? ""),
          variant: "destructive",
        });
        return;
      }

      if (!apiKey) {
        console.warn("[AIHub] app exchange returned empty api_key");
        if (disposed) return;
        toast({
          title: t({ zh: "登录失败", en: "Sign-in failed" }),
          description: t({ zh: "缺少 api_key", en: "Missing api_key" }),
          variant: "destructive",
        });
        return;
      }

      setUserApiKey(apiKey);
      try {
        await Browser.close();
      } catch (e) {
        console.debug("[AIHub] browser close skipped", e);
      }

      if (disposed) return;
      toast({ title: t({ zh: "登录成功", en: "Signed in" }) });

      let dest = "/me";
      try {
        const me = await apiFetchJson<{ is_admin?: boolean }>("/v1/me", { apiKey });
        if (me?.is_admin) dest = "/admin";
      } catch (e: any) {
        console.warn("[AIHub] post-login /v1/me check failed", e);
      }

      if (disposed) return;
      nav(dest, { replace: true });
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
        {/* Mobile header: app-like */}
        <div className="mx-auto flex max-w-md items-center gap-2 px-3 py-3 lg:hidden">
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
          {showDownloadNudge ? (
            <Button
              variant="secondary"
              size="sm"
              className="w-[52px] px-0"
              onClick={() => nav("/download")}
              aria-label={t({ zh: "下载 App", en: "Get the app" })}
              title={t({ zh: "下载 App", en: "Get the app" })}
            >
              <span className="flex items-center gap-1">
                <Download className="h-4 w-4" />
                <span className="text-xs font-semibold">{t({ zh: "下载", en: "App" })}</span>
              </span>
            </Button>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="w-[52px]"
              onClick={() => setTheme(resolved === "dark" ? "light" : "dark")}
              aria-label={t({ zh: "切换主题", en: "Toggle theme" })}
            >
              {resolved === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </Button>
          )}
        </div>

        {/* Desktop header: website-like */}
        <div className="mx-auto hidden w-full max-w-7xl items-center gap-4 px-6 py-3 lg:flex">
          <div className="flex items-center gap-2">
            {showBack ? (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  if (window.history.length > 1) nav(-1);
                  else nav(backTo, { replace: true });
                }}
                aria-label={t({ zh: "返回", en: "Back" })}
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
            ) : null}
            <Link to="/" className="flex items-center gap-2">
              <div className="text-base font-semibold tracking-tight">AIHub</div>
            </Link>
          </div>

          <nav className="flex items-center gap-1 text-sm">
            <Link to="/">
              <Button
                variant={pathname === "/" ? "secondary" : "ghost"}
                size="sm"
                aria-current={pathname === "/" ? "page" : undefined}
              >
                {t({ zh: "广场", en: "Square" })}
              </Button>
            </Link>
            <Link to="/topics">
              <Button
                variant={pathname.startsWith("/topics") ? "secondary" : "ghost"}
                size="sm"
                aria-current={pathname.startsWith("/topics") ? "page" : undefined}
              >
                {t({ zh: "话题", en: "Topics" })}
              </Button>
            </Link>
            <Link to="/runs">
              <Button
                variant={pathname.startsWith("/runs") ? "secondary" : "ghost"}
                size="sm"
                aria-current={pathname.startsWith("/runs") ? "page" : undefined}
              >
                {t({ zh: "任务", en: "Runs" })}
              </Button>
            </Link>
          </nav>

          <div className="ml-auto flex items-center gap-2">
            {showDownloadNudge ? (
              <Button variant="default" size="sm" onClick={() => nav("/download")}>
                <Download className="h-4 w-4" />
                <span>{t({ zh: "下载 App", en: "Get the app" })}</span>
              </Button>
            ) : null}
            <Link to={isLoggedIn ? "/me" : "/admin"}>
              <Button variant="secondary" size="sm">
                {isLoggedIn ? t({ zh: "我的", en: "Me" }) : t({ zh: "登录", en: "Sign in" })}
              </Button>
            </Link>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setTheme(resolved === "dark" ? "light" : "dark")}
              aria-label={t({ zh: "切换主题", en: "Toggle theme" })}
            >
              {resolved === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-md px-3 py-3 pb-24 lg:max-w-7xl lg:px-6 lg:pb-6">
        <div
          key={pathname}
          className="animate-in fade-in slide-in-from-bottom-4 duration-500 fill-mode-both"
        >
          {children ?? <Outlet />}
        </div>
      </main>

      <PwaInstallBanner />
      <BottomNav squareLabel={t({ zh: "广场", en: "Square" })} meLabel={t({ zh: "我的", en: "Me" })} />
      <Toaster />
    </div>
  );
}
