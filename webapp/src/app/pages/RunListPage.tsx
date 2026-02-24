import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
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

function RunRow({ run }: { run: RunListItem }) {
  const nav = useNavigate();
  return (
    <Card className="mb-2">
      <CardContent className="pt-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="secondary">{fmtRunStatus(run.status)}</Badge>
          <span>{fmtTime(run.created_at)}</span>
          {run.is_system ? <Badge variant="outline">平台内置</Badge> : null}
        </div>
        <div className="mt-2 text-sm font-medium">{trunc(run.goal, 140) || "（无标题）"}</div>
        <div className="mt-3">
          <Button size="sm" onClick={() => nav(`/runs/${encodeURIComponent(run.run_id)}`)}>
            进入详情
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function RunListPage() {
  const [sp, setSp] = useSearchParams();
  const nav = useNavigate();

  const q = sp.get("q") ?? "";
  const status = sp.get("status") ?? "all";

  const [qInput, setQInput] = useState(q);
  useEffect(() => setQInput(q), [q]);

  const [items, setItems] = useState<RunListItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);

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
    // Default ordering: running first, then newest.
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
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load({ reset: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q]);

  return (
    <div className="space-y-3">
      <Card>
        <CardContent className="pt-4">
          <div className="flex items-center gap-2">
            <Input
              value={qInput}
              onChange={(e) => setQInput(e.target.value)}
              placeholder="搜索任务…"
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
            <Button
              variant={status === "all" ? "default" : "secondary"}
              size="sm"
              onClick={() => {
                const next = new URLSearchParams(sp);
                next.set("status", "all");
                setSp(next, { replace: true });
              }}
            >
              全部
            </Button>
            <Button
              variant={status === "running" ? "default" : "secondary"}
              size="sm"
              onClick={() => {
                const next = new URLSearchParams(sp);
                next.set("status", "running");
                setSp(next, { replace: true });
              }}
            >
              进行中
            </Button>
            <Button
              variant={status === "done" ? "default" : "secondary"}
              size="sm"
              onClick={() => {
                const next = new URLSearchParams(sp);
                next.set("status", "done");
                setSp(next, { replace: true });
              }}
            >
              已完成
            </Button>
          </div>
          <div className="mt-3 flex gap-2">
            <Button variant="outline" size="sm" onClick={() => nav("/")}>
              回广场
            </Button>
            <Link
              to="/me"
              className="inline-flex items-center text-xs text-muted-foreground underline-offset-4 hover:underline"
            >
              去我的
            </Link>
          </div>
        </CardContent>
      </Card>

      {loading && !items.length ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
      {error ? <div className="text-sm text-destructive">{error}</div> : null}

      {filtered.length ? filtered.map((r) => <RunRow key={r.run_id} run={r} />) : null}

      {!loading && !error && !filtered.length ? (
        <div className="text-sm text-muted-foreground">暂无任务。</div>
      ) : null}

      <div className="py-2">
        {hasMore ? (
          <Button disabled={loading} variant="secondary" className="w-full" onClick={() => load({ reset: false })}>
            {loading ? "加载中…" : "加载更多"}
          </Button>
        ) : (
          <div className="text-center text-xs text-muted-foreground">没有更多了。</div>
        )}
      </div>
    </div>
  );
}

