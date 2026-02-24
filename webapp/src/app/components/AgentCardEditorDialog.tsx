import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { fmtAgentStatus } from "@/lib/format";
import { setAgentApiKey } from "@/lib/storage";

type Personality = {
  extrovert: number;
  curious: number;
  creative: number;
  stable: number;
};

type Discovery = {
  public: boolean;
  oss_endpoint?: string;
  last_synced_at?: string;
};

type Autonomous = {
  enabled: boolean;
  poll_interval_seconds: number;
  auto_accept_matching: boolean;
};

type AgentFull = {
  id: string;
  name: string;
  description: string;
  status: string;
  tags: string[];
  avatar_url: string;
  personality: Personality;
  interests: string[];
  capabilities: string[];
  bio: string;
  greeting: string;
  persona?: unknown;
  prompt_view: string;
  card_version: number;
  card_review_status: string;
  agent_public_key: string;
  admission: { status: string; admitted_at?: string };
  discovery: Discovery;
  autonomous: Autonomous;
};

type UpdateAgentRequest = {
  name?: string;
  description?: string;
  avatar_url?: string;
  personality?: Personality;
  interests?: string[];
  capabilities?: string[];
  bio?: string;
  greeting?: string;
  discovery?: { public: boolean };
  autonomous?: Autonomous;
};

type RotateAgentKeyResponse = { api_key: string };

export function AgentCardEditorDialog({
  agentId,
  userApiKey,
  onSaved,
}: {
  agentId: string;
  userApiKey: string;
  onSaved?: () => void;
}) {
  const { toast } = useToast();

  const [agent, setAgent] = useState<AgentFull | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [avatarUrl, setAvatarUrl] = useState("");
  const [bio, setBio] = useState("");
  const [greeting, setGreeting] = useState("");
  const [interests, setInterests] = useState("");
  const [capabilities, setCapabilities] = useState("");

  const [pExtrovert, setPExtrovert] = useState(0.5);
  const [pCurious, setPCurious] = useState(0.5);
  const [pCreative, setPCreative] = useState(0.5);
  const [pStable, setPStable] = useState(0.5);

  const [discoveryPublic, setDiscoveryPublic] = useState(false);
  const [autonomousEnabled, setAutonomousEnabled] = useState(false);
  const [pollIntervalSeconds, setPollIntervalSeconds] = useState(60);
  const [autoAcceptMatching, setAutoAcceptMatching] = useState(false);

  const normalizedInterests = useMemo(
    () =>
      interests
        .split(/[\\s,，]+/g)
        .map((t) => t.trim())
        .filter(Boolean)
        .slice(0, 24),
    [interests],
  );
  const normalizedCapabilities = useMemo(
    () =>
      capabilities
        .split(/[\\s,，]+/g)
        .map((t) => t.trim())
        .filter(Boolean)
        .slice(0, 24),
    [capabilities],
  );

  async function load() {
    if (!agentId) return;
    setLoading(true);
    setError("");
    try {
      const res = await apiFetchJson<AgentFull>(`/v1/agents/${encodeURIComponent(agentId)}`, {
        apiKey: userApiKey,
      });
      setAgent(res);
      setName(res.name ?? "");
      setDescription(res.description ?? "");
      setAvatarUrl(res.avatar_url ?? "");
      setBio(res.bio ?? "");
      setGreeting(res.greeting ?? "");
      setInterests((res.interests ?? []).join(", "));
      setCapabilities((res.capabilities ?? []).join(", "));
      setPExtrovert(Number(res.personality?.extrovert ?? 0.5));
      setPCurious(Number(res.personality?.curious ?? 0.5));
      setPCreative(Number(res.personality?.creative ?? 0.5));
      setPStable(Number(res.personality?.stable ?? 0.5));
      setDiscoveryPublic(!!res.discovery?.public);
      setAutonomousEnabled(!!res.autonomous?.enabled);
      setPollIntervalSeconds(Number(res.autonomous?.poll_interval_seconds ?? 60));
      setAutoAcceptMatching(!!res.autonomous?.auto_accept_matching);
    } catch (e: any) {
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId]);

  async function save() {
    if (!agentId) return;
    setSaving(true);
    setError("");
    try {
      const req: UpdateAgentRequest = {
        name: name.trim(),
        description: description.trim(),
        avatar_url: avatarUrl.trim(),
        bio: bio.trim(),
        greeting: greeting.trim(),
        interests: normalizedInterests,
        capabilities: normalizedCapabilities,
        personality: {
          extrovert: pExtrovert,
          curious: pCurious,
          creative: pCreative,
          stable: pStable,
        },
        discovery: { public: discoveryPublic },
        autonomous: {
          enabled: autonomousEnabled,
          poll_interval_seconds: pollIntervalSeconds,
          auto_accept_matching: autoAcceptMatching,
        },
      };
      await apiFetchJson(`/v1/agents/${encodeURIComponent(agentId)}`, {
        method: "PATCH",
        apiKey: userApiKey,
        body: req,
      });
      toast({ title: "已保存" });
      onSaved?.();
      await load();
    } catch (e: any) {
      setError(String(e?.message ?? "保存失败"));
      toast({ title: "保存失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setSaving(false);
    }
  }

  async function syncToOSS() {
    if (!agentId) return;
    setSaving(true);
    setError("");
    try {
      await apiFetchJson(`/v1/agents/${encodeURIComponent(agentId)}/sync-to-oss`, {
        method: "POST",
        apiKey: userApiKey,
      });
      toast({ title: "已同步到 OSS" });
      await load();
    } catch (e: any) {
      setError(String(e?.message ?? "同步失败"));
      toast({ title: "同步失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setSaving(false);
    }
  }

  async function rotateKey() {
    if (!agentId) return;
    setSaving(true);
    setError("");
    try {
      const res = await apiFetchJson<RotateAgentKeyResponse>(
        `/v1/agents/${encodeURIComponent(agentId)}/keys/rotate`,
        { method: "POST", apiKey: userApiKey },
      );
      setAgentApiKey(agentId, res.api_key);
      toast({ title: "已旋转密钥", description: "新密钥已保存在本地存储中。" });
    } catch (e: any) {
      setError(String(e?.message ?? "旋转失败"));
      toast({ title: "旋转失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setSaving(false);
    }
  }

  return (
    <DialogContent className="max-h-[80vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>Agent Card 管理</DialogTitle>
      </DialogHeader>

      {loading && !agent ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
      {error ? <div className="text-sm text-destructive">{error}</div> : null}

      {agent ? (
        <Card className="mt-2">
          <CardContent className="pt-4 text-xs text-muted-foreground">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="secondary">{fmtAgentStatus(agent.status)}</Badge>
              <Badge variant="outline">卡片 v{agent.card_version}</Badge>
              {agent.card_review_status ? <Badge variant="outline">{agent.card_review_status}</Badge> : null}
              {agent.admission?.status ? <Badge variant="outline">{agent.admission.status}</Badge> : null}
            </div>
            <div className="mt-2">
              <div className="font-medium text-foreground">prompt_view</div>
              <div className="mt-1 whitespace-pre-wrap">{agent.prompt_view || "（空）"}</div>
            </div>
          </CardContent>
        </Card>
      ) : null}

      <div className="mt-3 space-y-2">
        <div className="text-xs text-muted-foreground">名称</div>
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="智能体名称" />

        <div className="text-xs text-muted-foreground">简介</div>
        <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="一句话介绍…" />

        <div className="text-xs text-muted-foreground">头像 URL（可选）</div>
        <Input value={avatarUrl} onChange={(e) => setAvatarUrl(e.target.value)} placeholder="https://…" />

        <div className="text-xs text-muted-foreground">性格参数（0-1）</div>
        <div className="space-y-2 rounded-md border bg-background px-3 py-3">
          <div className="space-y-1">
            <div className="flex items-center justify-between text-xs">
              <span>外向</span>
              <span className="text-muted-foreground">{pExtrovert.toFixed(2)}</span>
            </div>
            <input
              className="w-full"
              type="range"
              min={0}
              max={1}
              step={0.05}
              value={pExtrovert}
              onChange={(e) => setPExtrovert(Number(e.target.value))}
            />
          </div>
          <div className="space-y-1">
            <div className="flex items-center justify-between text-xs">
              <span>好奇</span>
              <span className="text-muted-foreground">{pCurious.toFixed(2)}</span>
            </div>
            <input
              className="w-full"
              type="range"
              min={0}
              max={1}
              step={0.05}
              value={pCurious}
              onChange={(e) => setPCurious(Number(e.target.value))}
            />
          </div>
          <div className="space-y-1">
            <div className="flex items-center justify-between text-xs">
              <span>创造</span>
              <span className="text-muted-foreground">{pCreative.toFixed(2)}</span>
            </div>
            <input
              className="w-full"
              type="range"
              min={0}
              max={1}
              step={0.05}
              value={pCreative}
              onChange={(e) => setPCreative(Number(e.target.value))}
            />
          </div>
          <div className="space-y-1">
            <div className="flex items-center justify-between text-xs">
              <span>稳定</span>
              <span className="text-muted-foreground">{pStable.toFixed(2)}</span>
            </div>
            <input
              className="w-full"
              type="range"
              min={0}
              max={1}
              step={0.05}
              value={pStable}
              onChange={(e) => setPStable(Number(e.target.value))}
            />
          </div>
        </div>

        <div className="text-xs text-muted-foreground">兴趣（逗号分隔）</div>
        <Input value={interests} onChange={(e) => setInterests(e.target.value)} placeholder="例如：诗歌, 相声…" />

        <div className="text-xs text-muted-foreground">能力（逗号分隔）</div>
        <Input
          value={capabilities}
          onChange={(e) => setCapabilities(e.target.value)}
          placeholder="例如：写作, 总结, 编程…"
        />

        <div className="text-xs text-muted-foreground">简介（bio）</div>
        <Textarea value={bio} onChange={(e) => setBio(e.target.value)} placeholder="更完整的介绍…" />

        <div className="text-xs text-muted-foreground">问候语（greeting）</div>
        <Input value={greeting} onChange={(e) => setGreeting(e.target.value)} placeholder="一句友好问候…" />

        <div className="flex items-center justify-between rounded-md border bg-background px-3 py-2">
          <div className="text-sm">公开可发现</div>
          <input
            type="checkbox"
            checked={discoveryPublic}
            onChange={(e) => setDiscoveryPublic(e.target.checked)}
          />
        </div>

        <div className="flex items-center justify-between rounded-md border bg-background px-3 py-2">
          <div className="text-sm">自主模式</div>
          <input
            type="checkbox"
            checked={autonomousEnabled}
            onChange={(e) => setAutonomousEnabled(e.target.checked)}
          />
        </div>
        {autonomousEnabled ? (
          <div className="space-y-2 rounded-md border bg-background px-3 py-3">
            <div className="text-xs text-muted-foreground">轮询间隔（秒）</div>
            <Input
              value={String(pollIntervalSeconds)}
              onChange={(e) => setPollIntervalSeconds(Number(e.target.value || 0))}
              inputMode="numeric"
              className="w-32"
            />
            <div className="flex items-center justify-between">
              <div className="text-sm">自动接受匹配</div>
              <input
                type="checkbox"
                checked={autoAcceptMatching}
                onChange={(e) => setAutoAcceptMatching(e.target.checked)}
              />
            </div>
          </div>
        ) : null}
      </div>

      <DialogFooter className="mt-4 flex-col gap-2 sm:flex-row">
        <Button variant="secondary" disabled={saving} onClick={rotateKey}>
          旋转密钥
        </Button>
        <Button variant="secondary" disabled={saving} onClick={syncToOSS}>
          同步到 OSS
        </Button>
        <Button disabled={saving} onClick={save}>
          {saving ? "保存中…" : "保存"}
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
