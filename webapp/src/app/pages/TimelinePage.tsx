import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { apiFetchJson } from "@/lib/api";
import { fmtTime } from "@/lib/format";
import { getCurrentAgentId, getUserApiKey } from "@/lib/storage";

type TimelineEvent = {
  type: string;
  title: string;
  snippet?: string;
  occurred_at: string;
  refs?: Record<string, any>;
  visibility?: string;
};

type TimelineDay = {
  kind: string;
  schema_version: number;
  agent_id: string;
  date: string;
  events: TimelineEvent[];
};

type TimelineResponse = {
  days: TimelineDay[];
  next_cursor?: string;
};

export function TimelinePage() {
  const userApiKey = getUserApiKey();
  const isLoggedIn = !!userApiKey;
  const agentId = getCurrentAgentId();

  const [days, setDays] = useState<TimelineDay[]>([]);
  const [cursor, setCursor] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function load(reset = false) {
    if (!isLoggedIn) return;
    if (!agentId) return;
    setLoading(true);
    setError("");
    try {
      const cur = reset ? "" : cursor;
      const res = await apiFetchJson<TimelineResponse>(
        `/v1/agents/${encodeURIComponent(agentId)}/timeline?limit=10&cursor=${encodeURIComponent(cur)}`,
        { apiKey: userApiKey },
      );
      setDays((prev) => (reset ? res.days ?? [] : [...prev, ...(res.days ?? [])]));
      setCursor(String(res.next_cursor ?? ""));
    } catch (e: any) {
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId, isLoggedIn]);

  if (!isLoggedIn) return <div className="text-sm text-muted-foreground">请先登录。</div>;
  if (!agentId) return <div className="text-sm text-muted-foreground">请先在「我的」选择当前星灵。</div>;

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">时间线</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">只读视图：展示你的星灵近期轨迹。</CardContent>
      </Card>

      {error ? <div className="text-sm text-destructive">{error}</div> : null}

      {days.length ? (
        days.map((d) => (
          <Card key={d.date}>
            <CardContent className="pt-4 space-y-2">
              <div className="text-sm font-semibold">{d.date}</div>
              {d.events?.length ? (
                <div className="space-y-2">
                  {d.events.slice(0, 30).map((ev, idx) => {
                    const runId = String(ev.refs?.run_id ?? "").trim();
                    return (
                      <div key={`${d.date}_${idx}`} className="rounded-md border bg-background px-3 py-2">
                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                          <Badge variant="secondary">{ev.type}</Badge>
                          <span>{fmtTime(ev.occurred_at)}</span>
                        </div>
                        <div className="mt-1 text-sm font-medium">{ev.title}</div>
                        {ev.snippet ? <div className="mt-1 text-xs text-muted-foreground">{ev.snippet}</div> : null}
                        {runId ? (
                          <div className="mt-2">
                            <Link to={`/runs/${encodeURIComponent(runId)}`}>
                              <Button size="sm" variant="secondary">
                                打开相关任务
                              </Button>
                            </Link>
                          </div>
                        ) : null}
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="text-sm text-muted-foreground">暂无事件。</div>
              )}
            </CardContent>
          </Card>
        ))
      ) : (
        <div className="text-sm text-muted-foreground">{loading ? "加载中…" : "暂无记录。"}</div>
      )}

      <Button variant="secondary" className="w-full" disabled={loading || !cursor} onClick={() => load(false)}>
        {cursor ? (loading ? "加载中…" : "加载更多") : "没有更多了"}
      </Button>
    </div>
  );
}

