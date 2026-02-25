import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
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
};

function RunRow({ run }: { run: RunListItem }) {
  const nav = useNavigate();
  return (
    <Card className="mb-2">
      <CardContent className="pt-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="secondary">{fmtRunStatus(run.status)}</Badge>
          {run.is_system ? <Badge variant="outline">系统</Badge> : null}
          <span>{fmtTime(run.created_at)}</span>
        </div>
        <div className="mt-2 text-sm font-medium">{trunc(run.goal, 120) || "（无标题）"}</div>
        {run.preview_text ? (
          <div className="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">
            {trunc(run.preview_text, 260)}
          </div>
        ) : null}
        <div className="mt-3">
          <Button
            size="sm"
            onClick={() => {
              nav(`/runs/${encodeURIComponent(run.run_id)}`);
            }}
          >
            进入详情
          </Button>
        </div>
      </CardContent>
    </Card>
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

  useEffect(() => {
    const ac = new AbortController();
    async function loadFirstPage() {
      setRunsLoading(true);
      setRunsError("");
      try {
        const res = await apiFetchJson<ListRunsResponse>(`/v1/runs?include_system=1&limit=30&offset=0`, {
          signal: ac.signal,
        });
        setRuns(res.runs ?? []);
        setRunsHasMore(!!res.has_more);
        setRunsNextOffset(Number(res.next_offset ?? 0));
      } catch (e: any) {
        setRunsError(String(e?.message ?? "加载失败"));
      } finally {
        setRunsLoading(false);
      }
    }
    loadFirstPage();
    return () => ac.abort();
  }, []);

  const sortedRuns = useMemo(() => {
    const copied = [...runs];
    copied.sort((a, b) => String(b.updated_at ?? "").localeCompare(String(a.updated_at ?? "")));
    return copied;
  }, [runs]);

  async function loadMoreRuns() {
    if (runsLoading || !runsHasMore) return;
    setRunsLoading(true);
    setRunsError("");
    try {
      const res = await apiFetchJson<ListRunsResponse>(
        `/v1/runs?include_system=1&limit=30&offset=${encodeURIComponent(String(runsNextOffset))}`,
      );
      setRuns((prev) => [...prev, ...(res.runs ?? [])]);
      setRunsHasMore(!!res.has_more);
      setRunsNextOffset(Number(res.next_offset ?? 0));
    } catch (e: any) {
      setRunsError(String(e?.message ?? "加载失败"));
    } finally {
      setRunsLoading(false);
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-sm font-semibold">任务流</div>
        {!isLoggedIn ? (
          <Button variant="ghost" size="sm" onClick={() => nav("/me")}>
            去登录
          </Button>
        ) : (
          <div className="text-xs text-muted-foreground">阅读优先</div>
        )}
      </div>

      {runsLoading ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
      {runsError ? <div className="text-sm text-destructive">{runsError}</div> : null}
      {!runsLoading && !runsError ? (
        sortedRuns.length ? (
          sortedRuns.map((r) => <RunRow key={r.run_id} run={r} />)
        ) : (
          <div className="text-sm text-muted-foreground">暂无可阅读的任务。</div>
        )
      ) : null}

      {runsHasMore ? (
        <div className="pt-1">
          <Button variant="secondary" className="w-full" disabled={runsLoading} onClick={loadMoreRuns}>
            加载更多
          </Button>
        </div>
      ) : null}
    </div>
  );
}
