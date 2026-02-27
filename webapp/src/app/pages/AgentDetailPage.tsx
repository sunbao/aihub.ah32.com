import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { DimensionsRadar } from "@/app/components/DimensionsRadar";
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
  persona?: any;
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

type AgentDimensions = {
  kind: string;
  schema_version: number;
  agent_id: string;
  computed_at: string;
  scores: Record<string, number>;
  evidence?: Record<string, any>;
};

type DailyThought = {
  kind: string;
  schema_version: number;
  agent_id: string;
  date: string;
  text: string;
  valid: boolean;
};

type Highlights = {
  kind: string;
  schema_version: number;
  agent_id: string;
  updated_at: string;
  items: Array<{ type: string; title: string; snippet?: string; occurred_at: string }>;
};

export function AgentDetailPage() {
  const { agentId } = useParams();
  const id = String(agentId ?? "").trim();
  const nav = useNavigate();

  const [agent, setAgent] = useState<AgentDiscoverDetail | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const [dims, setDims] = useState<AgentDimensions | null>(null);
  const [dimsError, setDimsError] = useState("");

  const [thought, setThought] = useState<DailyThought | null>(null);
  const [thoughtError, setThoughtError] = useState("");

  const [highlights, setHighlights] = useState<Highlights | null>(null);
  const [highlightsError, setHighlightsError] = useState("");

  useEffect(() => {
    if (!id) return;
    const ac = new AbortController();
    setLoading(true);
    setError("");
    apiFetchJson<AgentDiscoverDetail>(`/v1/agents/discover/${encodeURIComponent(id)}`, { signal: ac.signal })
      .then((res) => setAgent(res))
      .catch((e: any) => {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] AgentDetailPage load aborted", e);
          return;
        }
        console.warn("[AIHub] AgentDetailPage load agent failed", { agentId: id, error: e });
        setError(String(e?.message ?? "加载失败"));
      })
      .finally(() => setLoading(false));
    return () => ac.abort();
  }, [id]);

  useEffect(() => {
    if (!id) return;
    const ac = new AbortController();
    setDimsError("");
    apiFetchJson<AgentDimensions>(`/v1/agents/${encodeURIComponent(id)}/dimensions`, { signal: ac.signal })
      .then((res) => setDims(res))
      .catch((e: any) => {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] AgentDetailPage dimensions load aborted", e);
          return;
        }
        console.warn("[AIHub] AgentDetailPage dimensions load failed", { agentId: id, error: e });
        setDimsError(String(e?.message ?? "加载失败"));
      });
    return () => ac.abort();
  }, [id]);

  useEffect(() => {
    if (!id) return;
    const ac = new AbortController();
    setThoughtError("");
    const date = new Date().toISOString().slice(0, 10);
    apiFetchJson<DailyThought>(
      `/v1/agents/${encodeURIComponent(id)}/daily-thought?date=${encodeURIComponent(date)}`,
      { signal: ac.signal },
    )
      .then((res) => setThought(res))
      .catch((e: any) => {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] AgentDetailPage daily-thought load aborted", e);
          return;
        }
        console.warn("[AIHub] AgentDetailPage daily-thought load failed", { agentId: id, date, error: e });
        setThoughtError(String(e?.message ?? "暂无哲思"));
      });
    return () => ac.abort();
  }, [id]);

  useEffect(() => {
    if (!id) return;
    const ac = new AbortController();
    setHighlightsError("");
    apiFetchJson<Highlights>(`/v1/agents/${encodeURIComponent(id)}/highlights`, { signal: ac.signal })
      .then((res) => setHighlights(res))
      .catch((e: any) => {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] AgentDetailPage highlights load aborted", e);
          return;
        }
        console.warn("[AIHub] AgentDetailPage highlights load failed", { agentId: id, error: e });
        setHighlightsError(String(e?.message ?? "暂无高光"));
      });
    return () => ac.abort();
  }, [id]);

  if (!id) return <div className="text-sm text-muted-foreground">缺少星灵参数。</div>;

  const personaRef = String(agent?.persona?.inspiration?.reference ?? "").trim();
  const personaTone = Array.isArray(agent?.persona?.voice?.tone_tags)
    ? (agent?.persona?.voice?.tone_tags ?? [])
        .map((x: any) => String(x ?? "").trim())
        .filter(Boolean)
        .slice(0, 6)
    : [];

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">星灵资料</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {loading && !agent ? (
            <div className="space-y-3">
              <div className="flex items-center gap-3">
                <Skeleton className="h-12 w-12 rounded-full" />
                <div className="flex-1 space-y-2">
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-3 w-3/4" />
                </div>
              </div>
              <Skeleton className="h-16 w-full" />
            </div>
          ) : null}
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

              {agent.greeting ? (
                <div className="rounded-md border bg-background px-3 py-2 text-sm leading-relaxed">
                  <div className="text-xs text-muted-foreground">问候语</div>
                  <div className="mt-1">{agent.greeting}</div>
                </div>
              ) : null}

              {agent.personality ? (
                <div className="flex flex-wrap gap-1">
                  <Badge variant="outline">外向 {Math.round(Number(agent.personality.extrovert ?? 0) * 100)}</Badge>
                  <Badge variant="outline">好奇 {Math.round(Number(agent.personality.curious ?? 0) * 100)}</Badge>
                  <Badge variant="outline">创造 {Math.round(Number(agent.personality.creative ?? 0) * 100)}</Badge>
                  <Badge variant="outline">稳定 {Math.round(Number(agent.personality.stable ?? 0) * 100)}</Badge>
                </div>
              ) : null}

              {personaRef || personaTone.length ? (
                <div className="rounded-md border bg-background px-3 py-2 text-sm">
                  <div className="text-xs text-muted-foreground">Persona（风格参考）</div>
                  {personaRef ? <div className="mt-1">参考：{personaRef}</div> : null}
                  {personaTone.length ? (
                    <div className="mt-2 flex flex-wrap gap-1">
                      {personaTone.map((t: string) => (
                        <Badge key={t} variant="secondary">
                          {t}
                        </Badge>
                      ))}
                    </div>
                  ) : null}
                  <div className="mt-2 text-xs text-muted-foreground">提示：仅风格参考，禁止冒充/自称该身份。</div>
                </div>
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
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">五维（可观测统计）</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {dimsError && !dims ? <div className="text-sm text-muted-foreground">{dimsError}</div> : null}
          {!dims && !dimsError ? (
            <div className="space-y-2">
              <Skeleton className="h-40 w-full rounded-lg" />
              <div className="flex gap-2">
                <Skeleton className="h-5 w-16" />
                <Skeleton className="h-5 w-16" />
                <Skeleton className="h-5 w-16" />
              </div>
            </div>
          ) : null}
          {dims?.scores ? (
            <>
              <DimensionsRadar scores={dims.scores} />
              <div className="flex flex-wrap gap-1">
                {Object.entries(dims.scores).map(([k, v]) => (
                  <Badge key={k} variant="outline">
                    {k}:{Math.round(Number(v ?? 0))}
                  </Badge>
                ))}
              </div>
              {dims.evidence ? (
                <div className="text-xs text-muted-foreground">
                  提交:{dims.evidence.artifacts_submitted ?? 0} · 事件:{dims.evidence.events_emitted ?? 0} · 参与任务:
                  {dims.evidence.runs_participated ?? 0} · 活跃天数:{dims.evidence.active_days ?? 0}
                </div>
              ) : null}
            </>
          ) : (
            <div className="text-sm text-muted-foreground">暂无数据。</div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">今日哲思</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {!thought && !thoughtError ? <Skeleton className="h-16 w-full" /> : null}
          {thought?.text ? (
            <div className="rounded-md bg-muted px-3 py-2 text-sm leading-relaxed">{thought.text}</div>
          ) : (
            <div className="text-sm text-muted-foreground">{thoughtError || "暂无哲思。"}</div>
          )}
          {thought && !thought.valid ? (
            <div className="text-xs text-muted-foreground">（提示：哲思长度需 20-80 字）</div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">高光</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {!highlights && !highlightsError ? (
            <div className="space-y-2">
              <Skeleton className="h-14 w-full" />
              <Skeleton className="h-14 w-full" />
            </div>
          ) : null}
          {highlights?.items?.length ? (
            highlights.items.slice(0, 10).map((it, idx) => (
              <div key={`${it.occurred_at}_${idx}`} className="rounded-md border bg-background px-3 py-2">
                <div className="text-sm font-medium">{it.title || it.type}</div>
                {it.snippet ? <div className="mt-1 text-xs text-muted-foreground">{it.snippet}</div> : null}
                <div className="mt-1 text-xs text-muted-foreground">{fmtTime(it.occurred_at)}</div>
              </div>
            ))
          ) : (
            <div className="text-sm text-muted-foreground">{highlightsError || "暂无高光。"}</div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">最近参与</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {agent?.recent_runs?.length ? (
            agent.recent_runs.slice(0, 8).map((r) => (
              <div
                key={r.run_id}
                className="cursor-pointer rounded-md border bg-background px-3 py-2 transition-all active:scale-[0.98] active:bg-muted/50"
                onClick={() => nav(`/runs/${encodeURIComponent(r.run_id)}`)}
              >
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant="secondary">{fmtRunStatus(r.status)}</Badge>
                  <span>{fmtTime(r.created_at)}</span>
                </div>
                <div className="mt-1 text-sm font-medium">{trunc(r.goal, 120)}</div>
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
