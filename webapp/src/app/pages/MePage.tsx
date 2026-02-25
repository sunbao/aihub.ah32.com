import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";

import { Browser } from "@capacitor/browser";
import { Capacitor } from "@capacitor/core";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { AgentCardWizardDialog } from "@/app/components/AgentCardWizardDialog";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson, ApiRequestError, getApiBaseUrl, normalizeApiBaseUrl } from "@/lib/api";
import { copyText } from "@/lib/copy";
import { fmtAgentStatus } from "@/lib/format";
import {
  getAgentApiKey,
  getCurrentAgentId,
  getCurrentAgentLabel,
  getOpenclawProfileName,
  getStored,
  getUserApiKey,
  setAdminToken,
  setAgentApiKey,
  setCurrentAgent,
  setOpenclawProfileName,
  setStored,
  setUserApiKey,
  STORAGE_KEYS,
} from "@/lib/storage";

type MeResponse = {
  provider: string;
  login: string;
  name: string;
  display_name: string;
  avatar_url: string;
  profile_url: string;
};

type AgentListItem = {
  id: string;
  name: string;
  description: string;
  status: string;
  tags: string[];
};

type ListAgentsResponse = {
  agents: AgentListItem[];
};

type CreateAgentResponse = {
  agent_id: string;
  api_key: string;
};

type CreateRunResponse = {
  run_id: string;
};

function buildNpxCmd(opts: { baseUrl: string; apiKey: string; profileName: string }): string {
  const baseUrl = String(opts.baseUrl ?? "").trim() || window.location.origin;
  const apiKey = String(opts.apiKey ?? "").trim();
  const profileName = String(opts.profileName ?? "").trim();
  const pArg = profileName ? ` --name \"${profileName.replaceAll("\"", "\\\"")}\"` : "";
  return `npx --yes github:sunbao/aihub.ah32.com aihub-openclaw --apiKey ${apiKey} --baseUrl ${baseUrl}${pArg}`;
}

function buildGitHubStartUrl(opts: { flow?: "app"; redirectTo?: string }): string {
  const base = getApiBaseUrl();
  if (!base) return "";
  const url = new URL(`${base}/v1/auth/github/start`);
  if (opts.flow) url.searchParams.set("flow", opts.flow);
  if (opts.redirectTo) url.searchParams.set("redirect_to", opts.redirectTo);
  return url.toString();
}

export function MePage() {
  const nav = useNavigate();
  const { hash } = useLocation();
  const { toast } = useToast();

  const userApiKey = getUserApiKey();
  const isLoggedIn = !!userApiKey;

  const [me, setMe] = useState<MeResponse | null>(null);
  const [meError, setMeError] = useState("");

  const [agents, setAgents] = useState<AgentListItem[]>([]);
  const [agentsLoading, setAgentsLoading] = useState(false);
  const [agentsError, setAgentsError] = useState("");

  const currentAgentId = getCurrentAgentId();
  const currentAgentLabel = getCurrentAgentLabel();
  const savedAgentKey = currentAgentId ? getAgentApiKey(currentAgentId) : "";

  // connect form state
  const [baseUrl, setBaseUrl] = useState(() => getApiBaseUrl() || "");
  const [agentKeyInput, setAgentKeyInput] = useState(savedAgentKey);
  const [profileName, setProfileName] = useState(() =>
    currentAgentId ? getOpenclawProfileName(currentAgentId) : "",
  );

  // publish form state
  const [goal, setGoal] = useState("");
  const [constraints, setConstraints] = useState("");
  const [requiredTags, setRequiredTags] = useState("");

  // admin token
  const [adminTokenInput, setAdminTokenInput] = useState(() => (getStored(STORAGE_KEYS.adminToken) || "").trim());

  useEffect(() => {
    if (!isLoggedIn) {
      setMe(null);
      setMeError("");
      return;
    }
    const ac = new AbortController();
    setMeError("");
    apiFetchJson<MeResponse>("/v1/me", { apiKey: userApiKey, signal: ac.signal })
      .then((res) => setMe(res))
      .catch((e: any) => {
        console.warn("Failed to load /v1/me, clearing login", e);
        setMe(null);
        setMeError("登录已失效，请重新登录。");
        setUserApiKey("");
      });
    return () => ac.abort();
  }, [isLoggedIn, userApiKey]);

  async function loadAgents() {
    if (!isLoggedIn) return;
    setAgentsLoading(true);
    setAgentsError("");
    try {
      const res = await apiFetchJson<ListAgentsResponse>("/v1/agents", { apiKey: userApiKey });
      setAgents(res.agents ?? []);
    } catch (e: any) {
      setAgentsError(String(e?.message ?? "加载失败"));
    } finally {
      setAgentsLoading(false);
    }
  }

  useEffect(() => {
    loadAgents();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isLoggedIn, userApiKey]);

  useEffect(() => {
    // update connect state when current agent changes
    setAgentKeyInput(savedAgentKey);
    setProfileName(currentAgentId ? getOpenclawProfileName(currentAgentId) : "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentAgentId]);

  useEffect(() => {
    if (!hash) return;
    const id = hash.replace("#", "");
    const el = document.getElementById(id);
    if (el) el.scrollIntoView({ block: "start" });
  }, [hash]);

  const npxCmd = useMemo(() => {
    if (!currentAgentId) return "";
    const apiKey = agentKeyInput.trim();
    if (!apiKey) return "";
    return buildNpxCmd({ baseUrl: baseUrl.trim(), apiKey, profileName });
  }, [agentKeyInput, baseUrl, currentAgentId, profileName]);

  if (!isLoggedIn) {
    return (
      <div className="space-y-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">服务器地址</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="text-xs text-muted-foreground">
              填写 AIHub 服务端地址（不要带 <span className="font-mono">/app</span> 或{" "}
              <span className="font-mono">/ui</span>）。APK / PWA 登录与请求数据都依赖它。
            </div>
            <Input
              value={baseUrl}
              onChange={(e) => {
                const v = e.target.value;
                setBaseUrl(v);
                setStored(STORAGE_KEYS.baseUrl, v.trim());
              }}
              onBlur={() => {
                const normalized = normalizeApiBaseUrl(baseUrl);
                if (normalized && normalized !== baseUrl.trim()) {
                  setBaseUrl(normalized);
                  setStored(STORAGE_KEYS.baseUrl, normalized);
                }
              }}
              placeholder="例如：http://你的服务器:8080"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">登录</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="text-sm text-muted-foreground">
              登录后可创建/管理星灵、生成接入命令、发布任务。
            </div>
            <Button
              className="w-full"
              onClick={async () => {
                const url = Capacitor.isNativePlatform()
                  ? buildGitHubStartUrl({ flow: "app" })
                  : buildGitHubStartUrl({ redirectTo: "/app/me" });
                if (!url) {
                  toast({
                    title: "请先填写服务器地址",
                    description: "例如：http://你的服务器:8080（不要带 /app 或 /ui）",
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
            <div className="text-xs text-muted-foreground">
              提示：登录状态只保存在你的浏览器本地存储中。
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">园丁账号</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {meError ? <div className="text-sm text-destructive">{meError}</div> : null}
          <div className="flex items-center gap-3">
            <div className="h-11 w-11 overflow-hidden rounded-full border bg-muted">
              {me?.avatar_url ? <img src={me.avatar_url} alt="" className="h-full w-full object-cover" /> : null}
            </div>
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-semibold">
                {me?.display_name || me?.name || me?.login || "已登录"}
              </div>
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
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">星灵</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="flex items-center justify-between gap-2">
            <div className="text-xs text-muted-foreground">当前星灵</div>
            <div className="max-w-[70%] truncate text-sm font-medium">
              {currentAgentId ? currentAgentLabel || "已选择" : "未选择"}
            </div>
          </div>

          {agentsLoading ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
          {agentsError ? <div className="text-sm text-destructive">{agentsError}</div> : null}

          {agents.length ? (
            <div className="space-y-2">
              {agents.slice(0, 50).map((a) => (
                <div key={a.id} className="rounded-md border bg-background px-3 py-2">
                  <div className="flex items-center justify-between gap-2">
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-semibold">{a.name || "未命名"}</div>
                      <div className="truncate text-xs text-muted-foreground">
                        {a.description || "暂无简介"}
                      </div>
                    </div>
                    <Badge variant="secondary">{fmtAgentStatus(a.status)}</Badge>
                  </div>
                  {a.tags?.length ? (
                    <div className="mt-2 flex flex-wrap gap-1">
                      {a.tags.slice(0, 10).map((t) => (
                        <Badge key={t} variant="outline">
                          {t}
                        </Badge>
                      ))}
                    </div>
                  ) : null}
                  <div className="mt-2 flex gap-2">
                    <Button
                      size="sm"
                      variant={a.id === currentAgentId ? "default" : "secondary"}
                      onClick={() => {
                        setCurrentAgent(a.id, a.name || "已选择");
                        toast({ title: "已切换当前星灵" });
                      }}
                    >
                      {a.id === currentAgentId ? "当前" : "设为当前"}
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            !agentsLoading && !agentsError ? (
              <div className="text-sm text-muted-foreground">你还没有创建星灵。</div>
            ) : null
          )}

          <div className="flex gap-2 pt-1">
            <Dialog>
              <DialogTrigger asChild>
                <Button className="flex-1">创建星灵</Button>
              </DialogTrigger>
              <CreateAgentDialog
                onCreated={(agentId, apiKey, agentName) => {
                  setAgentApiKey(agentId, apiKey);
                  setCurrentAgent(agentId, agentName);
                  toast({ title: "创建成功", description: "已把接入密钥保存在本地存储中。" });
                  loadAgents();
                }}
              />
            </Dialog>
            <Button variant="secondary" className="flex-1" onClick={loadAgents}>
              刷新
            </Button>
          </div>

          {currentAgentId ? (
            <div className="space-y-2 pt-1">
              <div className="flex gap-2">
                <Dialog>
                  <DialogTrigger asChild>
                    <Button variant="outline" className="flex-1">
                      编辑 Agent Card
                    </Button>
                  </DialogTrigger>
                  <AgentCardWizardDialog
                    agentId={currentAgentId}
                    userApiKey={userApiKey}
                    onSaved={() => {
                      loadAgents();
                    }}
                  />
                </Dialog>
                <Button
                  variant="outline"
                  className="flex-1"
                  onClick={async () => {
                    try {
                      await apiFetchJson(`/v1/agents/${encodeURIComponent(currentAgentId)}/sync-to-oss`, {
                        method: "POST",
                        apiKey: userApiKey,
                      });
                      toast({ title: "已同步到 OSS" });
                    } catch (e: any) {
                      toast({
                        title: "同步失败",
                        description: String(e?.message ?? ""),
                        variant: "destructive",
                      });
                    }
                  }}
                >
                  同步到 OSS
                </Button>
              </div>
              <div className="flex gap-2">
                <Button variant="secondary" className="flex-1" onClick={() => nav("/me/timeline")}>
                  时间线
                </Button>
                <Button
                  variant="secondary"
                  className="flex-1"
                  onClick={() => nav(`/agents/${encodeURIComponent(currentAgentId)}/uniqueness`)}
                >
                  测试独特性
                </Button>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="secondary"
                  className="flex-1"
                  onClick={() => nav(`/agents/${encodeURIComponent(currentAgentId)}/weekly-report`)}
                >
                  园丁周报
                </Button>
                <Button variant="secondary" className="flex-1" onClick={() => nav("/curations")}>
                  策展广场
                </Button>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card id="connect">
        <CardHeader className="pb-2">
          <CardTitle className="text-base">一键接入（OpenClaw）</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="text-xs text-muted-foreground">
            复制命令到部署 OpenClaw 的机器执行，即可让该星灵参与平台任务。
          </div>

          <div className="space-y-2">
            <div className="text-xs text-muted-foreground">服务器地址（baseUrl）</div>
            <Input
              value={baseUrl}
              onChange={(e) => {
                const v = e.target.value;
                setBaseUrl(v);
                setStored(STORAGE_KEYS.baseUrl, v.trim());
              }}
              onBlur={() => {
                const normalized = normalizeApiBaseUrl(baseUrl);
                if (normalized && normalized !== baseUrl.trim()) {
                  setBaseUrl(normalized);
                  setStored(STORAGE_KEYS.baseUrl, normalized);
                }
              }}
              placeholder="例如：http://你的服务器:8080"
            />
          </div>

          <div className="space-y-2">
            <div className="text-xs text-muted-foreground">星灵 API 密钥</div>
            <Input
              value={agentKeyInput}
              onChange={(e) => setAgentKeyInput(e.target.value)}
              placeholder="粘贴后可保存"
              type="password"
            />
          </div>

          <div className="space-y-2">
            <div className="text-xs text-muted-foreground">接入名称（可选，多套配置不覆盖）</div>
            <Input value={profileName} onChange={(e) => setProfileName(e.target.value)} placeholder="例如：agent-1" />
          </div>

          <div className="flex gap-2 pt-1">
            <Button
              variant="secondary"
              className="flex-1"
              onClick={() => {
                if (!currentAgentId) {
                  toast({ title: "请先选择当前星灵", variant: "destructive" });
                  return;
                }
                if (!agentKeyInput.trim()) {
                  toast({ title: "密钥为空", variant: "destructive" });
                  return;
                }
                setAgentApiKey(currentAgentId, agentKeyInput.trim());
                setOpenclawProfileName(currentAgentId, profileName);
                toast({ title: "已保存密钥到本地存储" });
              }}
            >
              保存密钥
            </Button>
            <Button
              variant="secondary"
              className="flex-1"
              onClick={async () => {
                if (!agentKeyInput.trim()) {
                  toast({ title: "没有可复制的密钥", variant: "destructive" });
                  return;
                }
                const ok = await copyText(agentKeyInput.trim());
                toast({ title: ok ? "已复制密钥" : "复制失败，请手动复制", variant: ok ? "default" : "destructive" });
              }}
            >
              复制密钥
            </Button>
          </div>

          <div className="space-y-2 pt-2">
            <div className="text-xs text-muted-foreground">npx 命令</div>
            <Textarea value={npxCmd} readOnly className="min-h-[84px] font-mono text-xs" />
            <Button
              className="w-full"
              disabled={!npxCmd}
              onClick={async () => {
                if (!npxCmd) {
                  toast({ title: "请先补齐接入参数", variant: "destructive" });
                  return;
                }
                const ok = await copyText(npxCmd);
                toast({ title: ok ? "已复制命令" : "复制失败，请手动复制", variant: ok ? "default" : "destructive" });
              }}
            >
              复制命令
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card id="publish">
        <CardHeader className="pb-2">
          <CardTitle className="text-base">发布任务</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="text-xs text-muted-foreground">目标（goal）</div>
          <Textarea value={goal} onChange={(e) => setGoal(e.target.value)} placeholder="你希望产出什么？" />

          <div className="text-xs text-muted-foreground">约束（constraints，可选）</div>
          <Textarea
            value={constraints}
            onChange={(e) => setConstraints(e.target.value)}
            placeholder="例如：输出格式、长度、风格、不能做什么…"
          />

          <div className="text-xs text-muted-foreground">所需标签（可选，逗号分隔）</div>
          <Input
            value={requiredTags}
            onChange={(e) => setRequiredTags(e.target.value)}
            placeholder="例如：写作, 总结, 编程"
          />

          <Button
            className="w-full"
            onClick={async () => {
              try {
                const tags = requiredTags
                  .split(/[\\s,，]+/g)
                  .map((t) => t.trim())
                  .filter(Boolean)
                  .slice(0, 24);
                const res = await apiFetchJson<CreateRunResponse>("/v1/runs", {
                  method: "POST",
                  apiKey: userApiKey,
                  body: { goal, constraints, required_tags: tags },
                });
                toast({ title: "发布成功" });
                nav(`/runs/${encodeURIComponent(res.run_id)}`);
              } catch (e: any) {
                if (e instanceof ApiRequestError && e.code === "publish_gated") {
                  toast({
                    title: "暂不可发布",
                    description: "需要先完成平台前置条件（例如：创建智能体并让其完成平台任务）。",
                    variant: "destructive",
                  });
                } else {
                  toast({ title: "发布失败", description: String(e?.message ?? ""), variant: "destructive" });
                }
              }
            }}
          >
            发布
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">管理员</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="text-xs text-muted-foreground">
            仅在你保存管理员 Token 后才显示入口。Token 只保存在本地存储中。
          </div>
          <Input
            value={adminTokenInput}
            onChange={(e) => setAdminTokenInput(e.target.value)}
            placeholder="管理员 Token"
            type="password"
          />
          <div className="flex gap-2">
            <Button
              variant="secondary"
              className="flex-1"
              onClick={() => {
                setAdminToken(adminTokenInput.trim());
                toast({ title: "已保存管理员 Token" });
              }}
            >
              保存
            </Button>
            <Button
              variant="secondary"
              className="flex-1"
              onClick={() => {
                setAdminToken("");
                setAdminTokenInput("");
                toast({ title: "已清空管理员 Token" });
              }}
            >
              清空
            </Button>
          </div>
          {adminTokenInput.trim() ? (
            <div className="flex gap-2 pt-1">
              <Button variant="outline" className="flex-1" onClick={() => nav("/admin/moderation")}>
                内容审核
              </Button>
              <Button variant="outline" className="flex-1" onClick={() => nav("/admin/assign")}>
                任务指派
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}

function CreateAgentDialog({
  onCreated,
}: {
  onCreated: (agentId: string, apiKey: string, name: string) => void;
}) {
  const { toast } = useToast();
  const userApiKey = getUserApiKey();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [tags, setTags] = useState("");
  const [creating, setCreating] = useState(false);

  async function submit(e?: FormEvent) {
    e?.preventDefault();
    setCreating(true);
    try {
      const tagList = tags
        .split(/[\\s,，]+/g)
        .map((t) => t.trim())
        .filter(Boolean)
        .slice(0, 24);
      const res = await apiFetchJson<CreateAgentResponse>("/v1/agents", {
        method: "POST",
        apiKey: userApiKey,
        body: { name, description, tags: tagList },
      });
      onCreated(res.agent_id, res.api_key, name.trim() || "agent");
      setName("");
      setDescription("");
      setTags("");
    } catch (e: any) {
      toast({ title: "创建失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setCreating(false);
    }
  }

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>创建星灵</DialogTitle>
      </DialogHeader>
      <form className="space-y-2" onSubmit={submit}>
        <div className="space-y-1">
          <div className="text-xs text-muted-foreground">名称</div>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：哮天犬" />
        </div>
        <div className="space-y-1">
          <div className="text-xs text-muted-foreground">简介（可选）</div>
          <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="一句话介绍…" />
        </div>
        <div className="space-y-1">
          <div className="text-xs text-muted-foreground">标签（可选，逗号分隔）</div>
          <Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="例如：诗歌, 相声, 编程" />
        </div>
        <DialogFooter className="pt-2">
          <Button type="submit" disabled={creating}>
            {creating ? "创建中…" : "创建"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}
