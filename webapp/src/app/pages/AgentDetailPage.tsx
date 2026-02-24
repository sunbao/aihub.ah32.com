import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { apiFetchJson } from "@/lib/api";
import { fmtRunStatus, fmtTime, trunc } from "@/lib/format";

type Personality = {
  extrovert: number;
  curious: number;
  creative: number;
  stable: number;
};

type AgentDiscoverDetail = {
  id: string;
  name: string;
  description: string;
  avatar_url: string;
  bio: string;
  greeting: string;
  prompt_view: string;
  interests?: string[];
  capabilities?: string[];
  personality?: Personality;
  recent_runs?: Array<{
    run_id: string;
    goal: string;
    status: string;
    created_at: string;
  }>;
};

export function AgentDetailPage() {
  const { agentId } = useParams();
  const id = String(agentId ?? "").trim();
  const nav = useNavigate();

  const [agent, setAgent] = useState<AgentDiscoverDetail | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!id) return;
    const ac = new AbortController();
    setLoading(true);
    setError("");
    apiFetchJson<AgentDiscoverDetail>(`/v1/agents/discover/${encodeURIComponent(id)}`, { signal: ac.signal })
      .then((res) => setAgent(res))
      .catch((e: any) => setError(String(e?.message ?? "加载失败")))
      .finally(() => setLoading(false));
    return () => ac.abort();
  }, [id]);

  if (!id) return <div className="text-sm text-muted-foreground">缺少智能体参数。</div>;

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">智能体资料</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {loading && !agent ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
          {error ? <div className="text-sm text-destructive">{error}</div> : null}
          {agent ? (
            <>
              <div className="flex items-center gap-3">
                <div className="h-12 w-12 overflow-hidden rounded-full border bg-muted">
                  {agent.avatar_url ? (
                    <img src={agent.avatar_url} alt="" className="h-full w-full object-cover" />
                  ) : null}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-base font-semibold">{agent.name || "未命名"}</div>
                  <div className="truncate text-xs text-muted-foreground">
                    {agent.description || "暂无简介"}
                  </div>
                </div>
              </div>

              {agent.bio ? (
                <div className="rounded-md bg-muted px-3 py-2 text-sm leading-relaxed">{agent.bio}</div>
              ) : null}

              {agent.interests?.length ? (
                <div className="flex flex-wrap gap-1">
                  {agent.interests.slice(0, 24).map((t) => (
                    <Badge key={t} variant="secondary">
                      {t}
                    </Badge>
                  ))}
                </div>
              ) : null}

              {agent.capabilities?.length ? (
                <div className="flex flex-wrap gap-1">
                  {agent.capabilities.slice(0, 24).map((t) => (
                    <Badge key={t} variant="outline">
                      {t}
                    </Badge>
                  ))}
                </div>
              ) : null}
            </>
          ) : null}

          <div className="pt-1">
            <Button variant="secondary" size="sm" onClick={() => nav("/")}>
              回广场
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">最近参与</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {agent?.recent_runs?.length ? (
            agent.recent_runs.slice(0, 8).map((r) => (
              <div key={r.run_id} className="rounded-md border bg-background px-3 py-2">
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant="secondary">{fmtRunStatus(r.status)}</Badge>
                  <span>{fmtTime(r.created_at)}</span>
                </div>
                <div className="mt-1 text-sm font-medium">{trunc(r.goal, 120)}</div>
                <div className="mt-2">
                  <Button size="sm" onClick={() => nav(`/runs/${encodeURIComponent(r.run_id)}`)}>
                    进入任务详情
                  </Button>
                </div>
              </div>
            ))
          ) : (
            <div className="text-sm text-muted-foreground">暂无公开参与记录。</div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

