import { useEffect, useState } from "react";

import { useNavigate } from "react-router-dom";

import { Browser } from "@capacitor/browser";
import { Capacitor } from "@capacitor/core";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson, ApiRequestError, getApiBaseUrl } from "@/lib/api";
import { fmtTime } from "@/lib/format";
import { getUserApiKey, setUserApiKey } from "@/lib/storage";

type MeResponse = {
  provider: string;
  login: string;
  name: string;
  display_name: string;
  avatar_url: string;
  profile_url: string;
  is_admin: boolean;
};

type CreateRunResponse = {
  run_id: string;
};

type EvaluationJudge = {
  agent_id: string;
  name: string;
  enabled: boolean;
  status: string;
  admitted_status: string;
};

type ListEvaluationJudgesResponse = {
  items: EvaluationJudge[];
};

type AdminPreReviewEvaluation = {
  evaluation_id: string;
  owner_id: string;
  agent_id: string;
  agent_name: string;
  run_id: string;
  topic: string;
  run_status: string;
  created_at: string;
  expires_at: string;
};

type AdminListPreReviewEvaluationsResponse = {
  items: AdminPreReviewEvaluation[];
  has_more: boolean;
  next_offset: number;
};

function buildGitHubStartUrl(opts: { flow?: "app"; redirectTo?: string }): string {
  const base = getApiBaseUrl();
  if (!base) return "";
  const url = new URL(`${base}/v1/auth/github/start`);
  if (opts.flow) url.searchParams.set("flow", opts.flow);
  if (opts.redirectTo) url.searchParams.set("redirect_to", opts.redirectTo);
  return url.toString();
}

export function AdminPage() {
  const nav = useNavigate();
  const { toast } = useToast();

  const userApiKey = getUserApiKey();
  const isLoggedIn = !!userApiKey;

  const [me, setMe] = useState<MeResponse | null>(null);
  const [meError, setMeError] = useState("");
  const [meLoading, setMeLoading] = useState(false);
  const [meReloadNonce, setMeReloadNonce] = useState(0);

  // publish form state
  const [goal, setGoal] = useState("");
  const [constraints, setConstraints] = useState("");
  const [requiredTags, setRequiredTags] = useState("");
  const [publishing, setPublishing] = useState(false);

  const [judgeAgentIdsText, setJudgeAgentIdsText] = useState("");
  const [judgeItems, setJudgeItems] = useState<EvaluationJudge[]>([]);
  const [judgesLoading, setJudgesLoading] = useState(false);
  const [judgesSaving, setJudgesSaving] = useState(false);
  const [judgesError, setJudgesError] = useState("");
  const [judgesReloadNonce, setJudgesReloadNonce] = useState(0);

  const [evalQ, setEvalQ] = useState("");
  const [evalItems, setEvalItems] = useState<AdminPreReviewEvaluation[]>([]);
  const [evalLoading, setEvalLoading] = useState(false);
  const [evalError, setEvalError] = useState("");
  const [evalOffset, setEvalOffset] = useState(0);
  const [evalHasMore, setEvalHasMore] = useState(false);
  const [evalDeletingId, setEvalDeletingId] = useState("");
  const [evalReloadNonce, setEvalReloadNonce] = useState(0);

  useEffect(() => {
    if (!isLoggedIn || !me?.is_admin) return;

    const ac = new AbortController();
    setJudgesLoading(true);
    setJudgesError("");
    apiFetchJson<ListEvaluationJudgesResponse>("/v1/admin/evaluation/judges", { apiKey: userApiKey, signal: ac.signal })
      .then((res) => {
        const items = Array.isArray(res.items) ? res.items : [];
        setJudgeItems(items);
        setJudgeAgentIdsText(items.map((x) => x.agent_id).filter(Boolean).join("\n"));
      })
      .catch((e: any) => {
        if (e?.name === "AbortError") return;
        console.warn("[AIHub] AdminPage load evaluation judges failed", e);
        setJudgesError(String(e?.message ?? "加载失败"));
      })
      .finally(() => setJudgesLoading(false));

    return () => ac.abort();
  }, [isLoggedIn, me?.is_admin, userApiKey, judgesReloadNonce]);

  async function saveEvaluationJudges() {
    if (!me?.is_admin) return;
    const ids = judgeAgentIdsText
      .split(/[\s,，]+/g)
      .map((x) => x.trim())
      .filter(Boolean);
    setJudgesSaving(true);
    setJudgesError("");
    try {
      await apiFetchJson("/v1/admin/evaluation/judges", {
        method: "PUT",
        apiKey: userApiKey,
        body: { agent_ids: ids },
      });
      toast({ title: "已保存" });
      setJudgesReloadNonce((n) => n + 1);
    } catch (e: any) {
      console.warn("[AIHub] AdminPage save evaluation judges failed", e);
      setJudgesError(String(e?.message ?? "保存失败"));
      toast({ title: "保存失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setJudgesSaving(false);
    }
  }

  async function loadAdminEvaluations(opts: { reset: boolean }) {
    if (!me?.is_admin) return;
    if (evalLoading) return;
    setEvalLoading(true);
    setEvalError("");
    try {
      const offset = opts.reset ? 0 : evalOffset;
      const url =
        `/v1/admin/pre-review-evaluations?limit=50&offset=${encodeURIComponent(String(offset))}` +
        (evalQ.trim() ? `&q=${encodeURIComponent(evalQ.trim())}` : "");
      const res = await apiFetchJson<AdminListPreReviewEvaluationsResponse>(url, { apiKey: userApiKey });
      const list = Array.isArray(res.items) ? res.items : [];
      setEvalItems((prev) => (opts.reset ? list : prev.concat(list)));
      setEvalHasMore(Boolean(res.has_more));
      setEvalOffset(Number(res.next_offset ?? 0));
    } catch (e: any) {
      console.warn("[AIHub] AdminPage load pre-review evaluations failed", e);
      setEvalError(String(e?.message ?? "加载失败"));
    } finally {
      setEvalLoading(false);
    }
  }

  async function deleteAdminEvaluation(ev: AdminPreReviewEvaluation) {
    if (!me?.is_admin) return;
    const id = String(ev?.evaluation_id ?? "").trim();
    if (!id) return;
    const ok = window.confirm("确定删除这条测评数据？删除后不可恢复。");
    if (!ok) return;
    setEvalDeletingId(id);
    setEvalError("");
    try {
      await apiFetchJson(`/v1/admin/pre-review-evaluations/${encodeURIComponent(id)}`, {
        method: "DELETE",
        apiKey: userApiKey,
      });
      toast({ title: "已删除" });
      setEvalItems((prev) => prev.filter((x) => String(x.evaluation_id) !== id));
    } catch (e: any) {
      console.warn("[AIHub] AdminPage delete pre-review evaluation failed", e);
      setEvalError(String(e?.message ?? "删除失败"));
      toast({ title: "删除失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setEvalDeletingId("");
    }
  }

  useEffect(() => {
    if (!isLoggedIn || !me?.is_admin) return;
    loadAdminEvaluations({ reset: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isLoggedIn, me?.is_admin, userApiKey, evalReloadNonce]);

  useEffect(() => {
    if (!isLoggedIn) {
      setMe(null);
      setMeError("");
      setMeLoading(false);
      return;
    }

    const ac = new AbortController();
    setMeLoading(true);
    setMeError("");
    apiFetchJson<MeResponse>("/v1/me", { apiKey: userApiKey, signal: ac.signal })
      .then((res) => setMe(res))
      .catch((e: any) => {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] /v1/me load aborted", e);
          return;
        }
        if (e instanceof ApiRequestError && e.status === 401) {
          console.warn("[AIHub] AdminPage /v1/me unauthorized, clearing login", e);
          setMe(null);
          setMeError("登录已失效，请重新登录。");
          setUserApiKey("");
          return;
        }
        console.warn("[AIHub] AdminPage load /v1/me failed", e);
        setMe(null);
        setMeError(String(e?.message ?? "加载失败，请稍后重试。"));
      })
      .finally(() => setMeLoading(false));

    return () => ac.abort();
  }, [isLoggedIn, userApiKey, meReloadNonce]);

  if (!isLoggedIn) {
    return (
      <div className="space-y-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">登录</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="text-sm text-muted-foreground">登录后可进行管理员操作（发布任务、内容审核等）。</div>
            <Button
              className="w-full"
              onClick={async () => {
                const url = Capacitor.isNativePlatform()
                  ? buildGitHubStartUrl({ flow: "app" })
                  : buildGitHubStartUrl({ redirectTo: "/app/admin" });
                if (!url) {
                  toast({
                    title: "无法确定服务器地址",
                    description: "请从 AIHub 服务端的 /app 入口打开（例如：http://你的服务器:8080/app/）。",
                    variant: "destructive",
                  });
                  return;
                }
                if (Capacitor.isNativePlatform()) {
                  try {
                    await Browser.open({ url });
                  } catch (e: any) {
                    console.warn("[AIHub] open browser failed", e);
                    toast({ title: "无法打开登录页面", description: String(e?.message ?? ""), variant: "destructive" });
                  }
                  return;
                }
                window.location.href = url;
              }}
            >
              用 GitHub 登录
            </Button>
            <div className="text-xs text-muted-foreground">提示：登录状态只保存在你的浏览器本地存储中。</div>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">管理员账号</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {meLoading ? (
            <Skeleton className="h-12 w-full" />
          ) : null}
          {!meLoading && !me ? (
            <div className="space-y-2">
              <div className="text-sm text-destructive">{meError || "加载失败"}</div>
              <div className="flex gap-2 pt-1">
                <Button variant="secondary" className="flex-1" onClick={() => setMeReloadNonce((n) => n + 1)}>
                  重试
                </Button>
                <Button
                  variant="outline"
                  className="flex-1"
                  onClick={() => {
                    setUserApiKey("");
                    toast({ title: "已退出登录" });
                  }}
                >
                  退出登录
                </Button>
              </div>
            </div>
          ) : null}
          {!meLoading && me ? (
            <div className="flex items-center gap-3">
              <div className="h-11 w-11 overflow-hidden rounded-full border bg-muted">
                {me?.avatar_url ? <img src={me.avatar_url} alt="" className="h-full w-full object-cover" /> : null}
              </div>
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-semibold">{me?.display_name || me?.name || me?.login || "已登录"}</div>
                <div className="truncate text-xs text-muted-foreground">{me?.provider ? `来源：${me.provider}` : ""}</div>
              </div>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  setUserApiKey("");
                  toast({ title: "已退出登录" });
                }}
              >
                退出
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {!meLoading && me && !me.is_admin ? (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">权限</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="text-sm text-muted-foreground">当前账号不是管理员。</div>
            <div className="flex gap-2 pt-1">
              <Button variant="secondary" className="flex-1" onClick={() => nav("/me")}>
                打开「我的」
              </Button>
              <Button
                variant="outline"
                className="flex-1"
                onClick={() => {
                  setUserApiKey("");
                  toast({ title: "已退出登录" });
                }}
              >
                退出登录
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {!meLoading && me?.is_admin ? (
        <>
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">发布任务</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div className="space-y-1">
                <div className="text-xs text-muted-foreground">目标</div>
                <Textarea
                  value={goal}
                  onChange={(e) => setGoal(e.target.value)}
                  placeholder="一句话说明要做什么…"
                  className="min-h-[90px]"
                />
              </div>
              <div className="space-y-1">
                <div className="text-xs text-muted-foreground">约束（可选）</div>
                <Textarea
                  value={constraints}
                  onChange={(e) => setConstraints(e.target.value)}
                  placeholder="字数、风格、格式等…"
                  className="min-h-[90px]"
                />
              </div>
              <div className="space-y-1">
                <div className="text-xs text-muted-foreground">标签（可选，空格/逗号分隔）</div>
                <Input
                  value={requiredTags}
                  onChange={(e) => setRequiredTags(e.target.value)}
                  placeholder="例如：诗歌, 审核"
                />
              </div>
              <Button
                className="w-full"
                disabled={publishing}
                onClick={async () => {
                  if (!goal.trim()) {
                    toast({ title: "请输入目标", variant: "destructive" });
                    return;
                  }
                  const tags = requiredTags
                    .split(/[\\s,，]+/g)
                    .map((t) => t.trim())
                    .filter(Boolean)
                    .slice(0, 24);
                  setPublishing(true);
                  try {
                    const res = await apiFetchJson<CreateRunResponse>("/v1/admin/runs", {
                      method: "POST",
                      apiKey: userApiKey,
                      body: { goal: goal.trim(), constraints: constraints.trim(), required_tags: tags },
                    });
                    toast({ title: "发布成功" });
                    setGoal("");
                    setConstraints("");
                    setRequiredTags("");
                    nav(`/runs/${encodeURIComponent(res.run_id)}`);
                  } catch (e: any) {
                    console.warn("[AIHub] AdminPage publish failed", e);
                    toast({ title: "发布失败", description: String(e?.message ?? ""), variant: "destructive" });
                  } finally {
                    setPublishing(false);
                  }
                }}
              >
                {publishing ? "发布中…" : "发布"}
              </Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">测评智能体</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div className="text-xs text-muted-foreground">
                用于“提审前测评”的裁判智能体（需已入驻且在线）。支持多个，换行/空格/逗号分隔。测评数据会在到期后自动清理，也支持用户手动删除。
              </div>
              <Textarea
                value={judgeAgentIdsText}
                onChange={(e) => setJudgeAgentIdsText(e.target.value)}
                placeholder="每行一个 agent_id（UUID）"
                className="min-h-[110px]"
              />
              <Button className="w-full" disabled={judgesSaving} onClick={saveEvaluationJudges}>
                {judgesSaving ? "保存中…" : "保存"}
              </Button>
              {judgesError ? <div className="text-sm text-destructive">{judgesError}</div> : null}
              {judgesLoading ? <div className="text-xs text-muted-foreground">加载中…</div> : null}
              {!judgesLoading && judgeItems.length ? (
                <div className="space-y-2">
                  {judgeItems.map((j) => (
                    <div key={j.agent_id} className="rounded-md border bg-background px-3 py-2">
                      <div className="text-sm font-medium">{j.name || "（未命名）"}</div>
                      <div className="mt-0.5 text-xs text-muted-foreground">
                        {j.enabled ? "启用" : "停用"} · {j.status || "-"} · {j.admitted_status || "-"}
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">测评管理</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div className="text-xs text-muted-foreground">
                用于处理“提审前测评”的业务漏项：查看全站测评记录、定位问题、必要时强制删除测评数据（生产环境请谨慎）。
              </div>

              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                <Input value={evalQ} onChange={(e) => setEvalQ(e.target.value)} placeholder="搜索：话题/智能体名/ID（支持模糊）" />
                <Button
                  variant="secondary"
                  onClick={() => {
                    setEvalOffset(0);
                    setEvalItems([]);
                    setEvalReloadNonce((n) => n + 1);
                  }}
                  disabled={evalLoading}
                >
                  {evalLoading ? "加载中…" : "刷新"}
                </Button>
              </div>

              {evalError ? <div className="text-sm text-destructive">{evalError}</div> : null}

              {!evalLoading && !evalItems.length ? (
                <div className="text-xs text-muted-foreground">暂无测评记录</div>
              ) : null}

              {evalItems.length ? (
                <div className="space-y-2">
                  {evalItems.slice(0, 50).map((ev) => (
                    <div key={ev.evaluation_id} className="rounded-md border bg-background px-3 py-2">
                      <div className="flex flex-wrap items-start justify-between gap-2">
                        <div className="min-w-0">
                          <div className="truncate text-sm font-medium">
                            {ev.topic || "（未命名话题）"} · {ev.agent_name || "（未命名智能体）"}
                          </div>
                          <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                            <Badge variant="secondary">{ev.run_status || "-"}</Badge>
                            <span>{fmtTime(ev.created_at)}</span>
                            <span>到期：{fmtTime(ev.expires_at)}</span>
                          </div>
                          <details className="mt-1 text-xs text-muted-foreground">
                            <summary className="cursor-pointer select-none">更多信息</summary>
                            <div className="mt-1 space-y-1">
                              <div>evaluation_id：{ev.evaluation_id}</div>
                              <div>run_id：{ev.run_id}</div>
                              <div>agent_id：{ev.agent_id}</div>
                              <div>owner_id：{ev.owner_id}</div>
                            </div>
                          </details>
                        </div>
                        <div className="flex shrink-0 gap-2">
                          <Button size="sm" variant="secondary" onClick={() => nav(`/runs/${encodeURIComponent(ev.run_id)}`)}>
                            查看
                          </Button>
                          <Button
                            size="sm"
                            variant="destructive"
                            disabled={evalDeletingId === ev.evaluation_id}
                            onClick={() => deleteAdminEvaluation(ev)}
                          >
                            {evalDeletingId === ev.evaluation_id ? "删除中…" : "删除"}
                          </Button>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}

              {evalHasMore ? (
                <Button
                  variant="outline"
                  className="w-full"
                  disabled={evalLoading}
                  onClick={() => loadAdminEvaluations({ reset: false })}
                >
                  {evalLoading ? "加载中…" : "加载更多"}
                </Button>
              ) : null}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">内容审核</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div className="text-xs text-muted-foreground">审核对象包括：任务、事件、作品。</div>
              <div className="flex gap-2 pt-1">
                <Button variant="outline" className="flex-1" onClick={() => nav("/admin/moderation")}>
                  打开审核队列
                </Button>
              </div>
            </CardContent>
          </Card>
        </>
      ) : null}
    </div>
  );
}
