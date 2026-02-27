import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { apiFetchJson } from "@/lib/api";
import { fmtRunStatus, fmtTime, trunc } from "@/lib/format";
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

function fmtEventKind(kind: string): string {
  const k = String(kind ?? "").trim().toLowerCase();
  if (k === "stage_changed") return "阶段切换";
  if (k === "decision") return "决策";
  if (k === "summary") return "总结";
  if (k === "artifact_version") return "作品版本";
  if (!k) return "事件";
  return k;
}

function previewPayloadText(payload: Record<string, any>): string {
  if (payload && typeof payload.text === "string") return String(payload.text).trim();
  if (payload && typeof payload.title === "string") return String(payload.title).trim();
  if (payload && typeof payload.stage === "string") return `阶段：${String(payload.stage).trim()}`;
  if (payload && typeof payload.version === "number") return `版本：v${payload.version}`;
  if (payload && typeof payload.version === "string") return `版本：${String(payload.version).trim()}`;
  try {
    const s = JSON.stringify(payload ?? {});
    if (s && s !== "{}") return s;
  } catch {
    // ignore
  }
  return "";
}

function fmtWorkItemsProgress(wi: WorkItemsProgress | null | undefined): string {
  const total = Number(wi?.total ?? 0);
  const completed = Number(wi?.completed ?? 0);
  const claimed = Number(wi?.claimed ?? 0);
  const offered = Number(wi?.offered ?? 0);
  const scheduled = Number(wi?.scheduled ?? 0);
  const failed = Number(wi?.failed ?? 0);

  const parts: string[] = [];
  if (total > 0) parts.push(`进度 ${completed}/${total}`);
  else parts.push("进度 -");

  if (claimed) parts.push(`已领取 ${claimed}`);
  if (offered) parts.push(`待领取 ${offered}`);
  if (scheduled) parts.push(`排队 ${scheduled}`);
  if (failed) parts.push(`失败 ${failed}`);
  return parts.join(" · ");
}

function ActivityRow({ item }: { item: ActivityItem }) {
  const nav = useNavigate();
  const payloadText = previewPayloadText(item.payload ?? {});
  return (
    <Card
      className="mb-3 cursor-pointer transition-all active:scale-[0.98] active:bg-muted/50"
      onClick={() => {
        nav(`/runs/${encodeURIComponent(item.run_id)}`);
      }}
    >
      <CardContent className="pt-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="secondary">{fmtEventKind(item.kind)}</Badge>
          <Badge variant="outline">{fmtRunStatus(item.run_status)}</Badge>
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
          {fmtWorkItemsProgress(item.work_items)}
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

  return (
    <div className="space-y-4 pb-4">
      <div className="flex items-center justify-between px-1">
        <h2 className="text-lg font-semibold tracking-tight">最新动态</h2>
        <div className="flex gap-2">
          <Button variant="secondary" size="sm" onClick={() => nav("/runs")}>
            任务列表
          </Button>
          <Button
            variant={includeSystem ? "default" : "secondary"}
            size="sm"
            onClick={() => setIncludeSystem((v) => !v)}
          >
            {includeSystem ? "含系统" : "不含系统"}
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setRefreshNonce((n) => n + 1)}>
            刷新
          </Button>
          {!isLoggedIn ? (
            <Button variant="secondary" size="sm" onClick={() => nav("/me")}>
              登录
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
              重试
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
            暂无内容
          </div>
        ) : null}

        {/* Sentinel for infinite scroll */}
        <div ref={observerTarget} className="h-4 w-full" />

        {!hasMore && items.length > 0 && (
          <div className="py-4 text-center text-xs text-muted-foreground/50">
            - 已经到底了 -
          </div>
        )}
      </div>
    </div>
  );
}
