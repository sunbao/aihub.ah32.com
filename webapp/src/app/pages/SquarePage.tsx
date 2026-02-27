import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { apiFetchJson } from "@/lib/api";
import { fmtRunStatus, fmtTime, trunc } from "@/lib/format";
import { getUserApiKey } from "@/lib/storage";

type RunListItem = {
  run_id: string;
  goal: string;
  constraints: string;
  status: string;
  created_at: string;
  updated_at: string;
  output_version: number;
  output_kind: string;
  is_system?: boolean;
  preview_text?: string;
};

type ListRunsResponse = {
  runs: RunListItem[];
  has_more: boolean;
  next_offset: number;
  total?: number;
};

function RunRow({ run }: { run: RunListItem }) {
  const nav = useNavigate();
  return (
    <Card
      className="mb-3 cursor-pointer transition-all active:scale-[0.98] active:bg-muted/50"
      onClick={() => {
        nav(`/runs/${encodeURIComponent(run.run_id)}`);
      }}
    >
      <CardContent className="pt-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="secondary">{fmtRunStatus(run.status)}</Badge>
          {run.is_system ? <Badge variant="outline">系统</Badge> : null}
          <span>{fmtTime(run.created_at)}</span>
        </div>
        <div className="mt-2 text-sm font-medium leading-normal">
          {trunc(run.goal, 120) || "（无标题）"}
        </div>
        {run.preview_text ? (
          <div className="mt-2 line-clamp-3 text-sm text-muted-foreground">
            {run.preview_text}
          </div>
        ) : null}
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

  const [runs, setRuns] = useState<RunListItem[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState<string>("");
  const [runsHasMore, setRunsHasMore] = useState(false);
  const [runsNextOffset, setRunsNextOffset] = useState(0);

  // Initial Load
  useEffect(() => {
    const ac = new AbortController();
    async function loadFirstPage() {
      setRunsLoading(true);
      setRunsError("");
      try {
        const res = await apiFetchJson<ListRunsResponse>(
          `/v1/runs?include_system=1&limit=20&offset=0`,
          {
            signal: ac.signal,
          },
        );
        setRuns(res.runs ?? []);
        setRunsHasMore(!!res.has_more);
        setRunsNextOffset(Number(res.next_offset ?? 0));
      } catch (e: any) {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] SquarePage initial load aborted", e);
          return;
        }
        console.warn("[AIHub] SquarePage initial load failed", e);
        setRunsError(String(e?.message ?? "加载失败"));
      } finally {
        setRunsLoading(false);
      }
    }
    loadFirstPage();
    return () => ac.abort();
  }, []);

  const sortedRuns = useMemo(() => {
    // Stable sort by updated_at desc
    const copied = [...runs];
    copied.sort((a, b) =>
      String(b.updated_at ?? "").localeCompare(String(a.updated_at ?? "")),
    );
    return copied;
  }, [runs]);

  async function loadMoreRuns() {
    if (runsLoading || !runsHasMore) return;
    setRunsLoading(true);
    setRunsError("");
    try {
      const res = await apiFetchJson<ListRunsResponse>(
        `/v1/runs?include_system=1&limit=20&offset=${encodeURIComponent(String(runsNextOffset))}`,
      );
      setRuns((prev) => {
        // De-duplicate just in case
        const existingIds = new Set(prev.map((r) => r.run_id));
        const newRuns = (res.runs ?? []).filter((r) => !existingIds.has(r.run_id));
        return [...prev, ...newRuns];
      });
      setRunsHasMore(!!res.has_more);
      setRunsNextOffset(Number(res.next_offset ?? 0));
    } catch (e: any) {
      console.warn("[AIHub] SquarePage loadMoreRuns failed", e);
      setRunsError(String(e?.message ?? "加载失败"));
    } finally {
      setRunsLoading(false);
    }
  }

  // Infinite Scroll Observer
  const observerTarget = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const currentTarget = observerTarget.current;
    if (!currentTarget) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && runsHasMore && !runsLoading) {
          void loadMoreRuns();
        }
      },
      { threshold: 0.1, rootMargin: "100px" },
    );

    observer.observe(currentTarget);
    return () => observer.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runsHasMore, runsLoading, runsNextOffset]); // Depend on runsNextOffset to refresh closure

  return (
    <div className="space-y-4 pb-4">
      <div className="flex items-center justify-between px-1">
        <h2 className="text-lg font-semibold tracking-tight">发现</h2>
        {!isLoggedIn ? (
          <Button variant="secondary" size="sm" onClick={() => nav("/me")}>
            登录
          </Button>
        ) : null}
      </div>

      <div className="space-y-3">
        {runsError && !runs.length ? (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-sm text-destructive">
            {runsError}
            <Button
              variant="link"
              className="ml-2 text-destructive underline"
              onClick={() => window.location.reload()}
            >
              重试
            </Button>
          </div>
        ) : null}

        {sortedRuns.map((r) => (
          <RunRow key={r.run_id} run={r} />
        ))}

        {runsLoading && (
          <>
            <RunSkeleton />
            <RunSkeleton />
            <RunSkeleton />
          </>
        )}

        {!runsLoading && sortedRuns.length === 0 && !runsError ? (
          <div className="py-12 text-center text-sm text-muted-foreground">
            暂无内容
          </div>
        ) : null}

        {/* Sentinel for infinite scroll */}
        <div ref={observerTarget} className="h-4 w-full" />

        {!runsHasMore && sortedRuns.length > 0 && (
          <div className="py-4 text-center text-xs text-muted-foreground/50">
            - 已经到底了 -
          </div>
        )}
      </div>
    </div>
  );
}
