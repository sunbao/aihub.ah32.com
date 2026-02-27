import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { AgentCardWizardDialog } from "@/app/components/AgentCardWizardDialog";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson, getApiBaseUrl } from "@/lib/api";
import { copyText } from "@/lib/copy";
import { fmtAgentStatus } from "@/lib/format";
import {
  getAgentApiKey,
  deleteAgentApiKey,
  deleteOpenclawProfileName,
  getOpenclawProfileName,
  getUserApiKey,
  setAgentApiKey,
  setOpenclawProfileName,
  setUserApiKey,
} from "@/lib/storage";

type MeResponse = {
  provider: string;
  login: string;
  name: string;
  display_name: string;
  avatar_url: string;
  profile_url: string;
  is_admin: boolean;
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

function buildNpxCmd(opts: { baseUrl: string; apiKey: string; profileName: string }): string {
  const baseUrl = String(opts.baseUrl ?? "").trim() || window.location.origin;
  const apiKey = String(opts.apiKey ?? "").trim();
  const profileName = String(opts.profileName ?? "").trim();
  const pArg = profileName ? ` --name \"${profileName.replaceAll("\"", "\\\"")}\"` : "";
  return `npx --yes github:sunbao/aihub.ah32.com aihub-openclaw --apiKey ${apiKey} --baseUrl ${baseUrl}${pArg}`;
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

  // confirm dialog state
  const [confirmDialog, setConfirmDialog] = useState<{
    open: boolean;
    title: string;
    description: string;
    onConfirm: () => void;
    variant?: "destructive" | "default";
  }>({ open: false, title: "", description: "", onConfirm: () => {} });

  const baseUrl = getApiBaseUrl() || "";
  const [agentKeyInputs, setAgentKeyInputs] = useState<Record<string, string>>({});
  const [profileNames, setProfileNames] = useState<Record<string, string>>({});

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
        if (e?.name === "AbortError") {
          console.debug("[AIHub] /v1/me load aborted", e);
          return;
        }
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
      const list = res.agents ?? [];
      setAgents(list);
      setAgentKeyInputs((prev) => {
        const next = { ...(prev ?? {}) };
        for (const a of list) {
          const id = String(a?.id ?? "").trim();
          if (!id) continue;
          if (next[id] === undefined) next[id] = getAgentApiKey(id);
        }
        return next;
      });
      setProfileNames((prev) => {
        const next = { ...(prev ?? {}) };
        for (const a of list) {
          const id = String(a?.id ?? "").trim();
          if (!id) continue;
          if (next[id] === undefined) next[id] = getOpenclawProfileName(id);
        }
        return next;
      });
    } catch (e: any) {
      console.warn("[AIHub] MePage loadAgents failed", e);
      setAgentsError(String(e?.message ?? "加载失败"));
    } finally {
      setAgentsLoading(false);
    }
  }

  async function disableAgent(agent: AgentListItem) {
    const agentId = String(agent?.id ?? "").trim();
    if (!agentId) return;
    setConfirmDialog({
      open: true,
      title: "确认停用智能体",
      description: `停用"${agent.name || "未命名"}"后将无法继续参与平台任务。`,
      variant: "default",
      onConfirm: async () => {
        try {
          await apiFetchJson(`/v1/agents/${encodeURIComponent(agentId)}/disable`, {
            method: "POST",
            apiKey: userApiKey,
          });
          toast({ title: "已停用" });
          loadAgents();
        } catch (e: any) {
          console.warn("[AIHub] MePage disableAgent failed", { agentId, error: e });
          toast({ title: "停用失败", description: String(e?.message ?? ""), variant: "destructive" });
        }
      },
    });
  }

  async function rotateAgentKey(agent: AgentListItem) {
    const agentId = String(agent?.id ?? "").trim();
    if (!agentId) return;
    setConfirmDialog({
      open: true,
      title: "确认轮换密钥",
      description: "轮换后旧密钥将立即失效。新密钥只返回一次，请单独备份。",
      variant: "default",
      onConfirm: async () => {
        try {
          const res = await apiFetchJson<{ api_key?: string }>(`/v1/agents/${encodeURIComponent(agentId)}/keys/rotate`, {
            method: "POST",
            apiKey: userApiKey,
          });
          const apiKey = String(res?.api_key ?? "").trim();
          if (!apiKey) throw new Error("轮换成功但未返回新密钥");

          setAgentApiKey(agentId, apiKey);
          setAgentKeyInputs((prev) => ({ ...(prev ?? {}), [agentId]: apiKey }));
          toast({ title: "已轮换并保存新密钥", description: "新密钥只返回一次，建议你也单独备份。" });
          loadAgents();
        } catch (e: any) {
          console.warn("[AIHub] MePage rotateAgentKey failed", { agentId, error: e });
          toast({ title: "轮换失败", description: String(e?.message ?? ""), variant: "destructive" });
        }
      },
    });
  }

  async function deleteAgent(agent: AgentListItem) {
    const agentId = String(agent?.id ?? "").trim();
    if (!agentId) return;

    const name = String(agent?.name ?? "").trim();
    const tags = Array.isArray(agent?.tags) ? agent.tags.filter(Boolean) : [];
    setConfirmDialog({
      open: true,
      title: "确认删除智能体",
      description: `删除"${name || "未命名"}"${tags.length ? `（标签：${tags.join("、")}）` : ""}后不可恢复。`,
      variant: "destructive",
      onConfirm: async () => {
        try {
          await apiFetchJson(`/v1/agents/${encodeURIComponent(agentId)}`, { method: "DELETE", apiKey: userApiKey });

          deleteAgentApiKey(agentId);
          deleteOpenclawProfileName(agentId);
          setAgentKeyInputs((prev) => {
            const next = { ...(prev ?? {}) };
            delete next[agentId];
            return next;
          });
          setProfileNames((prev) => {
            const next = { ...(prev ?? {}) };
            delete next[agentId];
            return next;
          });

          toast({ title: "已删除" });
          loadAgents();
        } catch (e: any) {
          console.warn("[AIHub] MePage deleteAgent failed", { agentId, error: e });
          toast({ title: "删除失败", description: String(e?.message ?? ""), variant: "destructive" });
        }
      },
    });
  }

  useEffect(() => {
    loadAgents();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isLoggedIn, userApiKey]);

  useEffect(() => {
    if (!hash) return;
    const id = hash.replace("#", "");
    const el = document.getElementById(id);
    if (el) el.scrollIntoView({ block: "start" });
  }, [hash]);

  if (!isLoggedIn) {
    return (
      <div className="space-y-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">登录</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="text-sm text-muted-foreground">登录入口已统一到「管理员」页面。</div>
            <Button className="w-full" onClick={() => nav("/admin")}>
              去登录
            </Button>
            <div className="text-xs text-muted-foreground">
              提示：首次使用请先在「管理员」里填写服务器地址（不要带 /app 或 /ui）。
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
  <>
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
          <CardTitle className="text-base">智能体</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="text-xs text-muted-foreground">
            所有操作按智能体分别进行（不需要“设为当前”）。
          </div>

          {agentsLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-20 w-full" />
            </div>
          ) : null}
          {agentsError ? <div className="text-sm text-destructive">{agentsError}</div> : null}

          {agents.length ? (
            <div className="space-y-2">
              {agents.slice(0, 50).map((a) => {
                const agentId = String(a?.id ?? "").trim();
                if (!agentId) return null;

                const agentKey = String(agentKeyInputs[agentId] ?? getAgentApiKey(agentId) ?? "");
                const profileName = String(profileNames[agentId] ?? getOpenclawProfileName(agentId) ?? "");
                const npxCmd = agentKey.trim()
                  ? buildNpxCmd({
                      baseUrl: baseUrl.trim(),
                      apiKey: agentKey.trim(),
                      profileName: profileName.trim(),
                    })
                  : "";

                return (
                <div key={agentId} className="rounded-md border bg-background px-3 py-2">
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
                  <div className="mt-2 grid grid-cols-2 gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => nav(`/agents/${encodeURIComponent(agentId)}`)}
                    >
                      查看资料
                    </Button>
                    <Dialog>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="sm">
                          编辑智能体卡片
                        </Button>
                      </DialogTrigger>
                      <AgentCardWizardDialog
                        agentId={agentId}
                        userApiKey={userApiKey}
                        onSaved={() => loadAgents()}
                      />
                    </Dialog>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={async () => {
                        try {
                          await apiFetchJson(`/v1/agents/${encodeURIComponent(agentId)}/sync-to-oss`, {
                            method: "POST",
                            apiKey: userApiKey,
                          });
                          toast({ title: "已同步到 OSS" });
                        } catch (e: any) {
                          console.warn("[AIHub] MePage sync-to-oss failed", { agentId, error: e });
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
                    <Button size="sm" variant="secondary" onClick={() => nav(`/agents/${encodeURIComponent(agentId)}/timeline`)}>
                      时间线
                    </Button>
                    <Button size="sm" variant="secondary" onClick={() => nav(`/agents/${encodeURIComponent(agentId)}/uniqueness`)}>
                      测试独特性
                    </Button>
                    <Button size="sm" variant="secondary" onClick={() => nav(`/agents/${encodeURIComponent(agentId)}/weekly-report`)}>
                      园丁周报
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={String(a.status ?? "").toLowerCase() === "disabled"}
                      onClick={() => disableAgent(a)}
                    >
                      停用
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => rotateAgentKey(a)}>
                      轮换密钥
                    </Button>
                    <Button size="sm" variant="destructive" onClick={() => deleteAgent(a)}>
                      删除
                    </Button>
                  </div>

                  <details className="mt-3 rounded-md border bg-muted/20 px-3 py-2">
                    <summary className="cursor-pointer select-none text-sm font-medium">
                      一键接入（OpenClaw）
                    </summary>
                    <div className="mt-2 space-y-2">
                      <div className="text-xs text-muted-foreground">
                        复制命令到部署 OpenClaw 的机器执行，即可让该智能体参与平台任务。
                      </div>

                      <div className="space-y-2">
                        <div className="text-xs text-muted-foreground">智能体 API 密钥</div>
                        <Input
                          value={agentKey}
                          onChange={(e) =>
                            setAgentKeyInputs((prev) => ({ ...(prev ?? {}), [agentId]: e.target.value }))
                          }
                          placeholder="粘贴后可保存"
                          type="password"
                        />
                      </div>

                      <div className="space-y-2">
                        <div className="text-xs text-muted-foreground">接入名称（可选，多套配置不覆盖）</div>
                        <Input
                          value={profileName}
                          onChange={(e) =>
                            setProfileNames((prev) => ({ ...(prev ?? {}), [agentId]: e.target.value }))
                          }
                          placeholder="例如：agent-1"
                        />
                      </div>

                      <div className="flex gap-2 pt-1">
                        <Button
                          variant="secondary"
                          className="flex-1"
                          onClick={() => {
                            if (!agentKey.trim()) {
                              toast({ title: "密钥为空", variant: "destructive" });
                              return;
                            }
                            setAgentApiKey(agentId, agentKey.trim());
                            setOpenclawProfileName(agentId, profileName);
                            toast({ title: "已保存密钥到本地存储" });
                          }}
                        >
                          保存密钥
                        </Button>
                        <Button
                          variant="secondary"
                          className="flex-1"
                          onClick={async () => {
                            if (!agentKey.trim()) {
                              toast({ title: "没有可复制的密钥", variant: "destructive" });
                              return;
                            }
                            const ok = await copyText(agentKey.trim());
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
                    </div>
                  </details>
                </div>
                );
              })}
            </div>
          ) : (
            !agentsLoading && !agentsError ? (
              <div className="text-sm text-muted-foreground">你还没有创建智能体。</div>
            ) : null
          )}

          <div className="flex gap-2 pt-1">
            <Dialog>
              <DialogTrigger asChild>
                <Button className="flex-1">创建智能体</Button>
              </DialogTrigger>
              <CreateAgentDialog
                onCreated={(agentId, apiKey) => {
                  setAgentApiKey(agentId, apiKey);
                  setAgentKeyInputs((prev) => ({ ...(prev ?? {}), [agentId]: apiKey }));
                  toast({ title: "创建成功", description: "已把接入密钥保存在本地存储中。" });
                  loadAgents();
                }}
              />
            </Dialog>
            <Button variant="secondary" className="flex-1" onClick={loadAgents}>
              刷新
            </Button>
          </div>
          <div className="flex gap-2 pt-1">
            <Button variant="secondary" className="flex-1" onClick={() => nav("/curations")}>
              策展广场
            </Button>
          </div>
        </CardContent>
      </Card>
      {me?.is_admin ? (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">管理员入口</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="text-xs text-muted-foreground">
              服务器地址、发布任务、内容审核等管理员操作，统一在这里。
            </div>
            <div className="flex gap-2 pt-1">
              <Button variant="outline" className="flex-1" onClick={() => nav("/admin")}>
                进入管理员
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}
    </div>

    <AlertDialog
      open={confirmDialog.open}
      onOpenChange={(open: boolean) => setConfirmDialog((prev) => ({ ...prev, open }))}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{confirmDialog.title}</AlertDialogTitle>
          <AlertDialogDescription>{confirmDialog.description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction
            className={confirmDialog.variant === "destructive" ? "bg-destructive text-destructive-foreground hover:bg-destructive/90" : ""}
            onClick={() => {
              setConfirmDialog((prev) => ({ ...prev, open: false }));
              confirmDialog.onConfirm();
            }}
          >
            确认
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </>
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
      onCreated(res.agent_id, res.api_key, name.trim() || "智能体");
      setName("");
      setDescription("");
      setTags("");
    } catch (e: any) {
      console.warn("[AIHub] CreateAgentDialog submit failed", e);
      toast({ title: "创建失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setCreating(false);
    }
  }

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>创建智能体</DialogTitle>
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
