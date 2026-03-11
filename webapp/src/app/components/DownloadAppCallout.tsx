import { useMemo } from "react";
import { useNavigate } from "react-router-dom";

import { Capacitor } from "@capacitor/core";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import { getUserApiKey } from "@/lib/storage";
import { cn } from "@/lib/utils";

export function DownloadAppCallout({
  className,
  compact,
}: {
  className?: string;
  compact?: boolean;
}) {
  const nav = useNavigate();
  const { t } = useI18n();

  const isNative = useMemo(() => Capacitor.isNativePlatform(), []);
  const isLoggedIn = useMemo(() => Boolean(getUserApiKey()), []);

  const show = !isNative && !isLoggedIn;
  if (!show) return null;

  return (
    <Card className={cn("mx-1", className)} data-testid="download-app-callout">
      <CardContent className={cn("pt-4", compact && "py-3")}>
        <div className="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
          <div className="min-w-0">
            <div className={cn("text-sm font-semibold", compact && "text-xs")}>
              {t({ zh: "想更深度参与？建议下载 App", en: "Want to participate deeply? Get the app" })}
            </div>
            <div className={cn("mt-1 text-xs text-muted-foreground", compact && "mt-0.5")}>
              {t({
                zh: "网页端适合匿名浏览；用 App 参与互动、创建智能体、发布任务更顺滑。",
                en: "Web is great for anonymous browsing. Use the app to participate, create agents, and publish runs.",
              })}
            </div>
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <Button size={compact ? "sm" : "sm"} onClick={() => nav("/download")}>
              {t({ zh: "下载 App", en: "Get the app" })}
            </Button>
            <Button size={compact ? "sm" : "sm"} variant="secondary" onClick={() => nav("/admin")}>
              {t({ zh: "登录/注册", en: "Sign in" })}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

