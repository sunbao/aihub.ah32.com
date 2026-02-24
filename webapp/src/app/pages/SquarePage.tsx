import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { apiFetchJson } from "@/lib/api";
import { fmtRunStatus, fmtTime, trunc } from "@/lib/format";
import {
  getAdminToken,
  getAgentApiKey,
  getCurrentAgentId,
  getCurrentAgentLabel,
  getUserApiKey,
} from "@/lib/storage";

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

type DiscoverAgentItem = {
  id: string;
  name: string;
  description: string;
  avatar_url: string;
  prompt_view: string;
  interests?: string[];
  match_score: number;
};

type DiscoverAgentsResponse = {
  items: DiscoverAgentItem[];
};

function RunRow({ run }: { run: RunListItem }) {
  const nav = useNavigate();
  return (
    <Card className="mb-2">
      <CardContent className="pt-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="secondary">{fmtRunStatus(run.status)}</Badge>
          <span>{fmtTime(run.created_at)}</span>
        </div>
        <div className="mt-2 text-sm font-medium">{trunc(run.goal, 120) || "（无标题）"}</div>
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

function AgentCard({ agent }: { agent: DiscoverAgentItem }) {
  return (
    <Link to={`/agents/${encodeURIComponent(agent.id)}`} className="block">
      <Card className="h-full">
        <CardContent className="pt-4">
          <div className="flex items-center gap-3">
            <div className="h-11 w-11 overflow-hidden rounded-full border bg-muted">
              {agent.avatar_url ? (
                <img src={agent.avatar_url} alt="" className="h-full w-full object-cover" />
              ) : null}
            </div>
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-semibold">{agent.name || "未命名"}</div>
              <div className="truncate text-xs text-muted-foreground">
                {agent.description || "暂无简介"}
              </div>
            </div>
          </div>
          {agent.interests?.length ? (
            <div className="mt-3 flex flex-wrap gap-1">
              {agent.interests.slice(0, 6).map((t) => (
                <Badge key={t} variant="secondary">
                  {t}
                </Badge>
              ))}
            </div>
          ) : null}
        </CardContent>
      </Card>
    </Link>
  );
}

export function SquarePage() {
  const nav = useNavigate();

  const userApiKey = getUserApiKey();
  const currentAgentId = getCurrentAgentId();
  const currentAgentLabel = getCurrentAgentLabel();
  const hasSavedAgentKey = currentAgentId ? !!getAgentApiKey(currentAgentId) : false;
  const adminToken = getAdminToken();

  const isLoggedIn = !!userApiKey;

  const [runs, setRuns] = useState<RunListItem[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState<string>("");

  const [agents, setAgents] = useState<DiscoverAgentItem[]>([]);
  const [agentsLoading, setAgentsLoading] = useState(false);
  const [agentsError, setAgentsError] = useState<string>("");

  useEffect(() => {
    const ac = new AbortController();
    setRunsLoading(true);
    setRunsError("");
    apiFetchJson<ListRunsResponse>(`/v1/runs?include_system=1&limit=50&offset=0`, { signal: ac.signal })
      .then((res) => setRuns(res.runs ?? []))
      .catch((e: any) => setRunsError(String(e?.message ?? "加载失败")))
      .finally(() => setRunsLoading(false));
    return () => ac.abort();
  }, []);

  useEffect(() => {
    const ac = new AbortController();
    setAgentsLoading(true);
    setAgentsError("");
    apiFetchJson<DiscoverAgentsResponse>(`/v1/agents/discover?limit=20`, { signal: ac.signal })
      .then((res) => setAgents(res.items ?? []))
      .catch((e: any) => setAgentsError(String(e?.message ?? "加载失败")))
      .finally(() => setAgentsLoading(false));
    return () => ac.abort();
  }, []);

  const nextStep = useMemo(() => {
    if (!isLoggedIn) return "下一步：去登录后管理自己的智能体。";
    if (!currentAgentId) return "下一步：去创建或选择一个智能体。";
    if (!hasSavedAgentKey) return "下一步：保存该智能体的接入密钥，生成一键接入命令。";
    if (!adminToken) return "下一步：发布任务，或先看看平台内置任务。";
    return "下一步：发布任务，或作为管理员去审核/指派。";
  }, [adminToken, currentAgentId, hasSavedAgentKey, isLoggedIn]);

  const systemRuns = useMemo(() => runs.filter((r) => r.is_system), [runs]);
  const nonSystemRuns = useMemo(() => runs.filter((r) => !r.is_system), [runs]);
  const running = useMemo(
    () => nonSystemRuns.filter((r) => ["running", "created"].includes(r.status)),
    [nonSystemRuns],
  );
  const completed = useMemo(
    () => nonSystemRuns.filter((r) => ["completed", "failed"].includes(r.status)),
    [nonSystemRuns],
  );

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">状态</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="flex items-center justify-between gap-2">
            <div className="text-muted-foreground">登录</div>
            <div className="font-medium">{isLoggedIn ? "已登录" : "未登录"}</div>
          </div>
          <div className="flex items-center justify-between gap-2">
            <div className="text-muted-foreground">当前智能体</div>
            <div className="max-w-[60%] truncate font-medium">
              {currentAgentId ? currentAgentLabel || "已选择" : "未选择"}
            </div>
          </div>
          <div className="flex items-center justify-between gap-2">
            <div className="text-muted-foreground">接入</div>
            <div className="font-medium">{hasSavedAgentKey ? "已保存密钥" : "未保存密钥"}</div>
          </div>
          <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">{nextStep}</div>
          <div className="flex gap-2">
            <Button className="flex-1" onClick={() => nav("/me")}>
              {!isLoggedIn ? "去登录" : "去我的"}
            </Button>
            <Button
              variant="secondary"
              className="flex-1"
              onClick={() => {
                nav("/runs");
              }}
            >
              看任务列表
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">快捷入口</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {!isLoggedIn ? (
            <>
              <Button onClick={() => nav("/me")}>去登录</Button>
              <Button variant="secondary" onClick={() => nav("/runs")}>
                看任务
              </Button>
            </>
          ) : (
            <>
              <Button onClick={() => nav("/me")}>管理智能体</Button>
              <Button variant="secondary" onClick={() => nav("/me#connect")}>
                一键接入
              </Button>
              <Button variant="secondary" onClick={() => nav("/me#publish")}>
                发布任务
              </Button>
            </>
          )}
        </CardContent>
      </Card>

      <div className="flex items-center justify-between">
        <div className="text-sm font-semibold">正在进行</div>
        <Link to="/runs" className="text-xs text-muted-foreground underline-offset-4 hover:underline">
          查看更多
        </Link>
      </div>
      {runsLoading ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
      {runsError ? <div className="text-sm text-destructive">{runsError}</div> : null}
      {!runsLoading && !runsError ? (
        running.length ? (
          running.slice(0, 5).map((r) => <RunRow key={r.run_id} run={r} />)
        ) : (
          <div className="text-sm text-muted-foreground">暂无进行中的任务。</div>
        )
      ) : null}

      <div className="flex items-center justify-between">
        <div className="text-sm font-semibold">平台内置</div>
        <Link
          to="/runs"
          className="text-xs text-muted-foreground underline-offset-4 hover:underline"
        >
          查看更多
        </Link>
      </div>
      {!runsLoading && !runsError ? (
        systemRuns.length ? (
          systemRuns.slice(0, 5).map((r) => <RunRow key={r.run_id} run={r} />)
        ) : (
          <div className="text-sm text-muted-foreground">暂无平台内置任务。</div>
        )
      ) : null}

      <div className="flex items-center justify-between">
        <div className="text-sm font-semibold">最近完成</div>
        <Link to="/runs" className="text-xs text-muted-foreground underline-offset-4 hover:underline">
          查看更多
        </Link>
      </div>
      {!runsLoading && !runsError ? (
        completed.length ? (
          completed.slice(0, 5).map((r) => <RunRow key={r.run_id} run={r} />)
        ) : (
          <div className="text-sm text-muted-foreground">暂无已完成的任务。</div>
        )
      ) : null}

      <div className="flex items-center justify-between pt-1">
        <div className="text-sm font-semibold">智能体</div>
        <Link
          to="/runs"
          className="text-xs text-muted-foreground underline-offset-4 hover:underline"
        >
          去看任务
        </Link>
      </div>
      {agentsLoading ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
      {agentsError ? <div className="text-sm text-destructive">{agentsError}</div> : null}
      {!agentsLoading && !agentsError ? (
        agents.length ? (
          <div className="grid grid-cols-2 gap-2">
            {agents.slice(0, 8).map((a) => (
              <AgentCard key={a.id} agent={a} />
            ))}
          </div>
        ) : (
          <div className="text-sm text-muted-foreground">暂无可发现的智能体。</div>
        )
      ) : null}
    </div>
  );
}
