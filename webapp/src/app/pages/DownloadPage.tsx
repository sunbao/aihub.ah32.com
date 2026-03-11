import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Capacitor } from "@capacitor/core";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { copyText } from "@/lib/copy";
import { useI18n } from "@/lib/i18n";
import { trunc } from "@/lib/format";

type PlatformMetaPublic = {
  app_download_url?: string;
};

function openExternal(url: string) {
  const v = String(url ?? "").trim();
  if (!v) return;

  try {
    window.open(v, "_blank", "noopener,noreferrer");
  } catch (error) {
    console.warn("[AIHub] openExternal failed, falling back to location.href", { url: v, error });
    window.location.href = v;
  }
}

export function DownloadPage() {
  const nav = useNavigate();
  const { toast } = useToast();
  const { t, isZh } = useI18n();

  const isNative = useMemo(() => Capacitor.isNativePlatform(), []);

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [downloadUrl, setDownloadUrl] = useState("");

  useEffect(() => {
    const ac = new AbortController();

    async function load() {
      setLoading(true);
      setError("");
      try {
        const res = await apiFetchJson<PlatformMetaPublic>("/v1/platform/meta", { signal: ac.signal });
        setDownloadUrl(String(res?.app_download_url ?? "").trim());
      } catch (e: any) {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] DownloadPage meta load aborted", e);
          return;
        }
        console.warn("[AIHub] DownloadPage meta load failed", e);
        setError(String(e?.message ?? (isZh ? "加载失败" : "Load failed")));
      } finally {
        setLoading(false);
      }
    }

    void load();
    return () => ac.abort();
  }, [isZh]);

  return (
    <div className="space-y-4 pb-4">
      <Card className="mx-1">
        <CardContent className="pt-4">
          <div className="text-sm font-semibold">{t({ zh: "下载 AIHub App", en: "Get the AIHub app" })}</div>
          <div className="mt-1 text-xs text-muted-foreground">
            {t({
              zh: "手机端更适合日常使用（通知、沉浸体验）。如果你只是浏览内容，也可以直接用网页。",
              en: "Mobile is better for daily use (notifications, immersive experience). You can also browse directly on the web.",
            })}
          </div>
          {isNative ? (
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <Button variant="secondary" size="sm" onClick={() => nav("/")}>
                {t({ zh: "返回广场", en: "Back to Square" })}
              </Button>
              <div className="text-xs text-muted-foreground">{t({ zh: "你正在 App 内，无需下载。", en: "You're already in the app." })}</div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {!isNative ? (
        <Card className="mx-1">
          <CardContent className="pt-4">
            <div className="text-sm font-semibold">{t({ zh: "安装链接", en: "Install link" })}</div>

            {loading ? (
              <div className="mt-3 space-y-2">
                <Skeleton className="h-4 w-48" />
                <Skeleton className="h-9 w-40" />
              </div>
            ) : null}

            {error ? (
              <div className="mt-3 rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
                {error}
              </div>
            ) : null}

            {!loading && !error ? (
              <>
                {downloadUrl ? (
                  <>
                    <div className="mt-2 text-xs text-muted-foreground">
                      {t({ zh: "下载地址：", en: "Download URL:" })}{" "}
                      <span className="font-mono text-foreground/80">{trunc(downloadUrl, 90)}</span>
                    </div>
                    <div className="mt-3 flex flex-wrap items-center gap-2">
                      <Button size="sm" onClick={() => openExternal(downloadUrl)}>
                        {t({ zh: "打开下载", en: "Open download" })}
                      </Button>
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={async () => {
                          const ok = await copyText(downloadUrl);
                          toast({
                            title: ok ? t({ zh: "已复制链接", en: "Link copied" }) : t({ zh: "复制失败，请手动复制", en: "Copy failed" }),
                            variant: ok ? "default" : "destructive",
                          });
                        }}
                      >
                        {t({ zh: "复制链接", en: "Copy link" })}
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => nav("/")}>
                        {t({ zh: "继续逛广场", en: "Keep browsing" })}
                      </Button>
                    </div>
                  </>
                ) : (
                  <div className="mt-3 rounded-lg border bg-muted/40 p-3 text-sm">
                    <div className="font-medium">{t({ zh: "暂未配置下载地址", en: "Download link is not configured yet" })}</div>
                    <div className="mt-1 text-xs text-muted-foreground">
                      {t({
                        zh: "请联系管理员配置 AIHUB_APP_DOWNLOAD_URL（/v1/platform/meta）。你仍可匿名浏览广场，登录后解锁管理能力。",
                        en: "Ask the admin to set AIHUB_APP_DOWNLOAD_URL (served via /v1/platform/meta). You can still browse the Square anonymously; sign in to unlock management features.",
                      })}
                    </div>
                    <div className="mt-3 flex flex-wrap items-center gap-2">
                      <Button size="sm" variant="secondary" onClick={() => nav("/admin")}>
                        {t({ zh: "登录/注册", en: "Sign in" })}
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => nav("/")}>
                        {t({ zh: "返回广场", en: "Back to Square" })}
                      </Button>
                    </div>
                  </div>
                )}
              </>
            ) : null}
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}

