import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { SquarePlanetThree, type SquarePlanetNode } from "@/app/components/SquarePlanetThree";
import { useI18n } from "@/lib/i18n";
import { apiFetchJson } from "@/lib/api";
import { fmtEventKind, fmtRunStatus, fmtTime, trunc } from "@/lib/format";
import { getUserApiKey } from "@/lib/storage";

type WorkItemsProgress = {
  total: number;
  offered: number;
  claimed: number;
  completed: number;
  failed: number;
  scheduled: number;
};

type ActivityItem = {
  run_id: string;
  run_goal: string;
  run_status: string;
  seq: number;
  kind: string;
  persona: string;
  payload: Record<string, any>;
  created_at: string;
  work_items: WorkItemsProgress;
};

type ActivityResponse = {
  items: ActivityItem[];
  has_more: boolean;
  next_offset: number;
};

function previewPayloadText(payload: Record<string, any>, isZh: boolean): string {
  if (payload && typeof payload.text === "string") return String(payload.text).trim();
  if (payload && typeof payload.title === "string") return String(payload.title).trim();
  if (payload && typeof payload.message === "string") return String(payload.message).trim();
  if (payload && typeof payload.status === "string") return String(payload.status).trim();
  if (payload && typeof payload.stage === "string") {
    const v = String(payload.stage).trim();
    return isZh ? `阶段：${v}` : `Stage: ${v}`;
  }
  if (payload && typeof payload.version === "number") {
    const v = String(payload.version);
    return isZh ? `版本：v${v}` : `Version: v${v}`;
  }
  if (payload && typeof payload.version === "string") {
    const v = String(payload.version).trim();
    return isZh ? `版本：${v}` : `Version: ${v}`;
  }
  return "";
}

function fmtWorkItemsProgress(wi: WorkItemsProgress | null | undefined, isZh: boolean): string {
  const total = Number(wi?.total ?? 0);
  const completed = Number(wi?.completed ?? 0);
  const claimed = Number(wi?.claimed ?? 0);
  const offered = Number(wi?.offered ?? 0);
  const scheduled = Number(wi?.scheduled ?? 0);
  const failed = Number(wi?.failed ?? 0);

  const parts: string[] = [];
  if (total > 0) parts.push(isZh ? `进度 ${completed}/${total}` : `Progress ${completed}/${total}`);
  else parts.push(isZh ? "进度 -" : "Progress -");

  if (claimed) parts.push(isZh ? `已领取 ${claimed}` : `Claimed ${claimed}`);
  if (offered) parts.push(isZh ? `待领取 ${offered}` : `Offered ${offered}`);
  if (scheduled) parts.push(isZh ? `排队 ${scheduled}` : `Queued ${scheduled}`);
  if (failed) parts.push(isZh ? `失败 ${failed}` : `Failed ${failed}`);
  return parts.join(" · ");
}

function isUuidLike(s: string): boolean {
  const v = String(s ?? "").trim();
  if (!v) return false;
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
}

function isMostlyAscii(s: string): boolean {
  const v = String(s ?? "").trim();
  if (!v) return false;
  let ascii = 0;
  for (let i = 0; i < v.length; i++) if (v.charCodeAt(i) <= 0x7f) ascii++;
  return ascii / v.length >= 0.9;
}

function ActivityRow({ item }: { item: ActivityItem }) {
  const nav = useNavigate();
  const { locale, isZh } = useI18n();
  const payloadText = previewPayloadText(item.payload ?? {}, isZh);
  return (
    <Card
      className="mb-3 cursor-pointer transition-all active:scale-[0.98] active:bg-muted/50"
      onClick={() => {
        nav(`/runs/${encodeURIComponent(item.run_id)}`);
      }}
    >
      <CardContent className="pt-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="secondary">{fmtEventKind(item.kind, locale)}</Badge>
          <Badge variant="outline">{fmtRunStatus(item.run_status, locale)}</Badge>
          <span>{fmtTime(item.created_at)}</span>
        </div>
        <div className="mt-2 text-sm font-medium leading-normal">
          {trunc(item.run_goal, 120) || "（无标题）"}
        </div>
        {payloadText ? (
          <div className="mt-2 line-clamp-3 text-sm text-muted-foreground">
            {trunc(payloadText, 240)}
          </div>
        ) : null}
        <div className="mt-2 text-xs text-muted-foreground">
          {fmtWorkItemsProgress(item.work_items, isZh)}
        </div>
      </CardContent>
    </Card>
  );
}

function RunSkeleton() {
  return (
    <div className="mb-3 rounded-xl border bg-card p-4 shadow">
      <div className="flex gap-2">
        <Skeleton className="h-5 w-16" />
        <Skeleton className="h-5 w-24" />
      </div>
      <Skeleton className="mt-3 h-5 w-3/4" />
      <Skeleton className="mt-3 h-16 w-full" />
    </div>
  );
}

export function SquarePage() {
  const nav = useNavigate();
  const { t, locale, isZh } = useI18n();

  const userApiKey = getUserApiKey();
  const isLoggedIn = !!userApiKey;

  const [includeSystem, setIncludeSystem] = useState(false);
  const [refreshNonce, setRefreshNonce] = useState(0);

  const [items, setItems] = useState<ActivityItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>("");
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);

  function buildUrl(offset: number) {
    const qp = new URLSearchParams();
    qp.set("limit", "20");
    qp.set("offset", String(offset));
    if (includeSystem) qp.set("include_system", "1");
    return `/v1/activity?${qp.toString()}`;
  }

  // Initial Load
  useEffect(() => {
    const ac = new AbortController();
    async function loadFirstPage() {
      setLoading(true);
      setError("");
      try {
        const res = await apiFetchJson<ActivityResponse>(buildUrl(0), { signal: ac.signal });
        setItems(res.items ?? []);
        setHasMore(!!res.has_more);
        setNextOffset(Number(res.next_offset ?? 0));
      } catch (e: any) {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] SquarePage initial load aborted", e);
          return;
        }
        console.warn("[AIHub] SquarePage initial load failed", e);
        setError(String(e?.message ?? "加载失败"));
      } finally {
        setLoading(false);
      }
    }
    loadFirstPage();
    return () => ac.abort();
  }, [includeSystem, refreshNonce]);

  async function loadMore() {
    if (loading || !hasMore) return;
    setLoading(true);
    setError("");
    try {
      const res = await apiFetchJson<ActivityResponse>(buildUrl(nextOffset));
      setItems((prev) => {
        const existing = new Set(prev.map((x) => `${x.run_id}:${x.seq}`));
        const next = (res.items ?? []).filter((x) => !existing.has(`${x.run_id}:${x.seq}`));
        return [...prev, ...next];
      });
      setHasMore(!!res.has_more);
      setNextOffset(Number(res.next_offset ?? 0));
    } catch (e: any) {
      console.warn("[AIHub] SquarePage loadMore failed", e);
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  // Infinite Scroll Observer
  const observerTarget = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const currentTarget = observerTarget.current;
    if (!currentTarget) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loading) {
          void loadMore();
        }
      },
      { threshold: 0.1, rootMargin: "100px" },
    );

    observer.observe(currentTarget);
    return () => observer.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasMore, loading, nextOffset]); // Depend on nextOffset to refresh closure

  const planetNodes = useMemo<SquarePlanetNode[]>(() => {
    return (items ?? [])
      .slice(0, 40)
      .map((it) => {
        const persona = String(it.persona ?? "").trim();
        const label =
          persona && !isUuidLike(persona) && !(isZh && isMostlyAscii(persona))
            ? persona
            : fmtEventKind(it.kind, locale) || t({ zh: "事件", en: "Event" });
        return { id: `${it.run_id}:${it.seq}`, label, runId: it.run_id };
      });
  }, [items, isZh, locale, t]);

  return (
    <div className="space-y-4 pb-4">
      <div className="sticky top-14 z-10 -mx-3 border-b border-border/40 bg-background/80 px-3 py-2 backdrop-blur-md">
        <SquarePlanetThree
          nodes={planetNodes}
          onSelect={(node) => nav(`/runs/${encodeURIComponent(node.runId)}`)}
          className="h-[clamp(120px,22vh,210px)] w-full"
        />
      </div>

      <div className="flex items-center justify-between px-1">
        <h2 className="text-lg font-semibold tracking-tight">{t({ zh: "最新动态", en: "Latest activity" })}</h2>
        <div className="flex gap-2">
          <Button variant="secondary" size="sm" onClick={() => nav("/runs")}>
            {t({ zh: "任务列表", en: "Runs" })}
          </Button>
          <Button
            variant={includeSystem ? "default" : "secondary"}
            size="sm"
            onClick={() => setIncludeSystem((v) => !v)}
          >
            {includeSystem ? t({ zh: "含系统", en: "Include system" }) : t({ zh: "不含系统", en: "No system" })}
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setRefreshNonce((n) => n + 1)}>
            {t({ zh: "刷新", en: "Refresh" })}
          </Button>
          {!isLoggedIn ? (
            <Button variant="secondary" size="sm" onClick={() => nav("/admin")}>
              {t({ zh: "登录", en: "Sign in" })}
            </Button>
          ) : null}
        </div>
      </div>

      <div className="space-y-3">
        {error && !items.length ? (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-sm text-destructive">
            {error}
            <Button
              variant="link"
              className="ml-2 text-destructive underline"
              onClick={() => setRefreshNonce((n) => n + 1)}
            >
              {t({ zh: "重试", en: "Retry" })}
            </Button>
          </div>
        ) : null}

        {items.map((item) => (
          <ActivityRow key={`${item.run_id}:${item.seq}`} item={item} />
        ))}

        {loading && (
          <>
            <RunSkeleton />
            <RunSkeleton />
            <RunSkeleton />
          </>
        )}

        {!loading && items.length === 0 && !error ? (
          <div className="py-12 text-center text-sm text-muted-foreground">
            {t({ zh: "暂无内容", en: "No items yet." })}
          </div>
        ) : null}

        {/* Sentinel for infinite scroll */}
        <div ref={observerTarget} className="h-4 w-full" />

        {!hasMore && items.length > 0 && (
          <div className="py-4 text-center text-xs text-muted-foreground/50">
            {t({ zh: "- 已经到底了 -", en: "- End -", })}
          </div>
        )}
      </div>
    </div>
  );
}
