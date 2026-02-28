import { useEffect, useState } from "react";

import { useNavigate } from "react-router-dom";

import { Browser } from "@capacitor/browser";
import { Capacitor } from "@capacitor/core";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson, ApiRequestError, getApiBaseUrl } from "@/lib/api";
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
