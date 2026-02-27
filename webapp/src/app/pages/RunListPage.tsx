import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { apiFetchJson } from "@/lib/api";
import { fmtRunStatus, fmtTime, trunc } from "@/lib/format";

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
};

type ListRunsResponse = {
  runs: RunListItem[];
  has_more: boolean;
  next_offset: number;
};

function RunSkeleton() {
  return (
    <div className="mb-2 rounded-xl border bg-card p-4 shadow-sm">
      <div className="flex gap-2">
        <Skeleton className="h-5 w-16" />
        <Skeleton className="h-5 w-24" />
      </div>
      <Skeleton className="mt-3 h-5 w-3/4" />
    </div>
  );
}

function RunRow({ run }: { run: RunListItem }) {
  const nav = useNavigate();
  return (
    <Card
      className="mb-2 cursor-pointer transition-all active:scale-[0.98] active:bg-muted/50"
      onClick={() => nav(`/runs/${encodeURIComponent(run.run_id)}`)}
    >
      <CardContent className="pt-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="secondary">{fmtRunStatus(run.status)}</Badge>
          <span>{fmtTime(run.created_at)}</span>
          {run.is_system ? <Badge variant="outline">平台内置</Badge> : null}
        </div>
        <div className="mt-2 text-sm font-medium">{trunc(run.goal, 140) || "（无标题）"}</div>
      </CardContent>
    </Card>
  );
}

export function RunListPage() {
  const [sp, setSp] = useSearchParams();

  const q = sp.get("q") ?? "";
  const status = sp.get("status") ?? "all";

  const [qInput, setQInput] = useState(q);
  useEffect(() => setQInput(q), [q]);

  const [items, setItems] = useState<RunListItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);
  const observerTarget = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    const base = items.slice();
    const isRunning = (s: string) => ["running", "created"].includes(String(s).toLowerCase());
    const isDone = (s: string) => ["completed", "failed"].includes(String(s).toLowerCase());
    const out =
      status === "running"
        ? base.filter((r) => isRunning(r.status))
        : status === "done"
          ? base.filter((r) => isDone(r.status))
          : base;
    out.sort((a, b) => {
      const ar = isRunning(a.status) ? 0 : 1;
      const br = isRunning(b.status) ? 0 : 1;
      if (ar !== br) return ar - br;
      return String(b.created_at).localeCompare(String(a.created_at));
    });
    return out;
  }, [items, status]);

  function buildUrl(offset: number) {
    const qp = new URLSearchParams();
    qp.set("include_system", "1");
    qp.set("limit", "20");
    qp.set("offset", String(offset));
    if (q.trim()) qp.set("q", q.trim());
    return `/v1/runs?${qp.toString()}`;
  }

  async function load({ reset }: { reset: boolean }) {
    if (loading) return;
    setLoading(true);
    setError("");
    try {
      const offset = reset ? 0 : nextOffset;
      const res = await apiFetchJson<ListRunsResponse>(buildUrl(offset));
      const list = res.runs ?? [];
      setItems((prev) => (reset ? list : prev.concat(list)));
      setHasMore(!!res.has_more);
      setNextOffset(res.next_offset ?? offset + list.length);
    } catch (e: any) {
      console.warn("[AIHub] RunListPage load failed", e);
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load({ reset: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q]);

  // Infinite scroll
  useEffect(() => {
    const currentTarget = observerTarget.current;
    if (!currentTarget) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loading) {
          void load({ reset: false });
        }
      },
      { threshold: 0.1, rootMargin: "120px" },
    );
    observer.observe(currentTarget);
    return () => observer.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasMore, loading, nextOffset]);

  return (
    <div className="space-y-3">
      <Card>
        <CardContent className="pt-4">
          <div className="flex items-center gap-2">
            <Input
              value={qInput}
              onChange={(e) => setQInput(e.target.value)}
              placeholder="搜索任务…"
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  const next = new URLSearchParams(sp);
                  if (qInput.trim()) next.set("q", qInput.trim());
                  else next.delete("q");
                  setSp(next, { replace: true });
                }
              }}
            />
            <Button
              onClick={() => {
                const next = new URLSearchParams(sp);
                if (qInput.trim()) next.set("q", qInput.trim());
                else next.delete("q");
                setSp(next, { replace: true });
              }}
            >
              搜索
            </Button>
          </div>
          <div className="mt-3 flex gap-2">
            {(["all", "running", "done"] as const).map((s) => (
              <Button
                key={s}
                variant={status === s ? "default" : "secondary"}
                size="sm"
                onClick={() => {
                  const next = new URLSearchParams(sp);
                  next.set("status", s);
                  setSp(next, { replace: true });
                }}
              >
                {s === "all" ? "全部" : s === "running" ? "进行中" : "已完成"}
              </Button>
            ))}
          </div>
        </CardContent>
      </Card>

      {error ? <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">{error}</div> : null}

      {/* Initial skeleton */}
      {loading && !items.length && (
        <>
          <RunSkeleton />
          <RunSkeleton />
          <RunSkeleton />
        </>
      )}

      {filtered.map((r) => (
        <RunRow key={r.run_id} run={r} />
      ))}

      {!loading && !error && !filtered.length ? (
        <div className="py-12 text-center text-sm text-muted-foreground">暂无任务。</div>
      ) : null}

      {/* Sentinel */}
      <div ref={observerTarget} className="h-4 w-full" />

      {/* Bottom loading skeletons */}
      {loading && items.length > 0 && (
        <>
          <RunSkeleton />
          <RunSkeleton />
        </>
      )}

      {!hasMore && filtered.length > 0 && (
        <div className="py-4 text-center text-xs text-muted-foreground/50">- 已经到底了 -</div>
      )}
    </div>
  );
}
