import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { fmtAgentStatus, trunc } from "@/lib/format";
import { getAgentCardCatalogs, renderCatalogTemplate, type AgentCardCatalogs, type CatalogLabeledItem, type CatalogTextTemplate } from "@/app/lib/agentCardCatalogs";

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
  persona_template_id?: string;
  discovery?: { public: boolean };
  autonomous?: Autonomous;
};

type ApprovedPersonaTemplate = {
  template_id: string;
  review_status: "approved";
  persona: any;
  updated_at: string;
};

function normalizeText(s: string): string {
  return String(s ?? "")
    .trim()
    .replace(/\s+/g, " ");
}

function findMatchingTemplateId(templates: CatalogTextTemplate[] | undefined, renderedText: string, vars: { name: string; interests: string[]; capabilities: string[] }): string {
  const want = normalizeText(renderedText);
  if (!want) return "";
  for (const t of templates ?? []) {
    const got = normalizeText(renderCatalogTemplate(t.template, vars));
    if (got && got === want) return String(t.id ?? "").trim();
  }
  return "";
}

function buildLabelSet(items: CatalogLabeledItem[] | undefined): Set<string> {
  const s = new Set<string>();
  for (const it of items ?? []) {
    const lbl = String(it.label ?? "").trim();
    if (lbl) s.add(lbl);
  }
  return s;
}

function MultiSelect({
  title,
  options,
  selected,
  onChange,
  maxSelected = 24,
}: {
  title: string;
  options: CatalogLabeledItem[];
  selected: string[];
  onChange: (next: string[]) => void;
  maxSelected?: number;
}) {
  const [q, setQ] = useState("");

  const filtered = useMemo(() => {
    const term = q.trim().toLowerCase();
    if (!term) return options;
    return options.filter((o) => {
      const hay = `${o.label ?? ""} ${(o.keywords ?? []).join(" ")}`.toLowerCase();
      return hay.includes(term);
    });
  }, [options, q]);

  const selectedSet = useMemo(() => new Set(selected.map((x) => x.trim()).filter(Boolean)), [selected]);

  function toggle(label: string) {
    const t = label.trim();
    if (!t) return;
    const next = new Set(selectedSet);
    if (next.has(t)) next.delete(t);
    else {
      if (next.size >= maxSelected) return;
      next.add(t);
    }
    onChange(Array.from(next.values()));
  }

  return (
    <Card className="mt-2">
      <CardContent className="pt-4">
        <div className="text-sm font-medium">{title}</div>
        {selected.length ? (
          <div className="mt-2 flex flex-wrap gap-1">
            {selected.map((t) => (
              <Badge key={t} variant="secondary" className="cursor-pointer" onClick={() => toggle(t)}>
                {t}
              </Badge>
            ))}
          </div>
        ) : (
          <div className="mt-2 text-xs text-muted-foreground">未选择</div>
        )}

        <div className="mt-3">
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="搜索…" />
        </div>

        <div className="mt-3 max-h-[34vh] overflow-y-auto rounded-md border p-2">
          <div className="flex flex-wrap gap-2">
            {filtered.slice(0, 200).map((o) => {
              const lbl = String(o.label ?? "").trim();
              const active = selectedSet.has(lbl);
              return (
                <Button
                  key={o.id}
                  size="sm"
                  variant={active ? "default" : "secondary"}
                  onClick={() => toggle(lbl)}
                >
                  {lbl}
                </Button>
              );
            })}
          </div>
        </div>
        <div className="mt-2 text-xs text-muted-foreground">
          已选 {selected.length}/{maxSelected}
        </div>
      </CardContent>
    </Card>
  );
}

export function AgentCardWizardDialog({
  agentId,
  userApiKey,
  onSaved,
}: {
  agentId: string;
  userApiKey: string;
  onSaved?: () => void;
}) {
  const { toast } = useToast();

  const [step, setStep] = useState(0);
  const [agent, setAgent] = useState<AgentFull | null>(null);
  const [catalogs, setCatalogs] = useState<AgentCardCatalogs | null>(null);
  const [personaTemplates, setPersonaTemplates] = useState<ApprovedPersonaTemplate[]>([]);

  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [avatarUrl, setAvatarUrl] = useState("");

  const [personaTemplateId, setPersonaTemplateId] = useState<string>("");
  const [personaTouched, setPersonaTouched] = useState(false);

  const [personalityPresetId, setPersonalityPresetId] = useState<string>("");
  const [pExtrovert, setPExtrovert] = useState(0.5);
  const [pCurious, setPCurious] = useState(0.5);
  const [pCreative, setPCreative] = useState(0.5);
  const [pStable, setPStable] = useState(0.5);

  const [interests, setInterests] = useState<string[]>([]);
  const [capabilities, setCapabilities] = useState<string[]>([]);

  const [bioTemplateId, setBioTemplateId] = useState<string>("");
  const [greetingTemplateId, setGreetingTemplateId] = useState<string>("");
  const [bioCustom, setBioCustom] = useState(false);
  const [greetingCustom, setGreetingCustom] = useState(false);
  const [bio, setBio] = useState("");
  const [greeting, setGreeting] = useState("");

  const interestLabelSet = useMemo(() => buildLabelSet(catalogs?.interests), [catalogs]);
  const capabilityLabelSet = useMemo(() => buildLabelSet(catalogs?.capabilities), [catalogs]);

  const templateVars = useMemo(
    () => ({ name: name.trim(), interests: interests ?? [], capabilities: capabilities ?? [] }),
    [capabilities, interests, name],
  );

  function recomputeTemplatesIfNeeded(nextName: string, nextInterests: string[], nextCapabilities: string[]) {
    const vars = { name: nextName.trim(), interests: nextInterests, capabilities: nextCapabilities };
    if (!bioCustom && bioTemplateId) {
      const t = (catalogs?.bio_templates ?? []).find((x) => String(x.id ?? "") === bioTemplateId);
      if (t?.template) setBio(renderCatalogTemplate(t.template, vars));
    }
    if (!greetingCustom && greetingTemplateId) {
      const t = (catalogs?.greeting_templates ?? []).find((x) => String(x.id ?? "") === greetingTemplateId);
      if (t?.template) setGreeting(renderCatalogTemplate(t.template, vars));
    }
  }

  async function loadAll(forceCatalogRefresh = false) {
    if (!agentId) return;
    setLoading(true);
    setError("");
    try {
      const [a, c, p] = await Promise.all([
        apiFetchJson<AgentFull>(`/v1/agents/${encodeURIComponent(agentId)}`, { apiKey: userApiKey }),
        getAgentCardCatalogs({ userApiKey, forceRefresh: forceCatalogRefresh }),
        apiFetchJson<{ items: ApprovedPersonaTemplate[] }>(`/v1/persona-templates?limit=200`, { apiKey: userApiKey }),
      ]);

      setAgent(a);
      setCatalogs(c);
      setPersonaTemplates(p.items ?? []);

      setName(String(a.name ?? ""));
      setDescription(String(a.description ?? ""));
      setAvatarUrl(String(a.avatar_url ?? ""));

      setPExtrovert(Number(a.personality?.extrovert ?? 0.5));
      setPCurious(Number(a.personality?.curious ?? 0.5));
      setPCreative(Number(a.personality?.creative ?? 0.5));
      setPStable(Number(a.personality?.stable ?? 0.5));

      setInterests(Array.isArray(a.interests) ? a.interests : []);
      setCapabilities(Array.isArray(a.capabilities) ? a.capabilities : []);

      setBio(String(a.bio ?? ""));
      setGreeting(String(a.greeting ?? ""));

      const vars = { name: String(a.name ?? "").trim(), interests: a.interests ?? [], capabilities: a.capabilities ?? [] };
      const bioMatch = findMatchingTemplateId(c.bio_templates, a.bio ?? "", vars);
      const greetMatch = findMatchingTemplateId(c.greeting_templates, a.greeting ?? "", vars);
      setBioTemplateId(bioMatch);
      setGreetingTemplateId(greetMatch);
      setBioCustom(!bioMatch);
      setGreetingCustom(!greetMatch);

      const preset = (c.personality_presets ?? []).find((pp) => {
        const v = pp.values;
        return (
          Number(v.extrovert) === Number(a.personality?.extrovert) &&
          Number(v.curious) === Number(a.personality?.curious) &&
          Number(v.creative) === Number(a.personality?.creative) &&
          Number(v.stable) === Number(a.personality?.stable)
        );
      });
      setPersonalityPresetId(preset?.id ?? "");
    } catch (e: any) {
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadAll(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId]);

  const willNeedReview = useMemo(() => {
    if (!catalogs) return true;
    for (const t of interests) {
      if (t && !interestLabelSet.has(String(t).trim())) return true;
    }
    for (const t of capabilities) {
      if (t && !capabilityLabelSet.has(String(t).trim())) return true;
    }
    const bioOk = !!findMatchingTemplateId(catalogs.bio_templates, bio, templateVars);
    const greetingOk = !!findMatchingTemplateId(catalogs.greeting_templates, greeting, templateVars);
    return bioCustom || greetingCustom || !bioOk || !greetingOk;
  }, [bio, bioCustom, capabilities, capabilityLabelSet, catalogs, greeting, greetingCustom, interestLabelSet, interests, templateVars]);

  async function save() {
    if (!agentId) return;
    setSaving(true);
    setError("");
    try {
      const req: UpdateAgentRequest = {
        name: name.trim(),
        description: description.trim(),
        avatar_url: avatarUrl.trim(),
        personality: {
          extrovert: pExtrovert,
          curious: pCurious,
          creative: pCreative,
          stable: pStable,
        },
        interests,
        capabilities,
        bio: bio.trim(),
        greeting: greeting.trim(),
      };
      if (personaTouched) req.persona_template_id = personaTemplateId;

      await apiFetchJson(`/v1/agents/${encodeURIComponent(agentId)}`, {
        method: "PATCH",
        apiKey: userApiKey,
        body: req,
      });

      toast({ title: "已保存" });
      onSaved?.();
      await loadAll(false);
      setStep(6);
    } catch (e: any) {
      setError(String(e?.message ?? "保存失败"));
      toast({ title: "保存失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setSaving(false);
    }
  }

  function stepTitle(): string {
    switch (step) {
      case 0:
        return "基础信息";
      case 1:
        return "Persona（风格参考）";
      case 2:
        return "性格预设";
      case 3:
        return "兴趣";
      case 4:
        return "能力";
      case 5:
        return "简介与问候";
      default:
        return "预览与状态";
    }
  }

  const canGoPrev = step > 0;
  const canGoNext = step < 6;

  return (
    <DialogContent className="max-h-[80vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>Agent Card 向导：{stepTitle()}</DialogTitle>
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

      {agent && catalogs ? (
        <>
          {step === 0 ? (
            <Card className="mt-2">
              <CardContent className="pt-4 space-y-3">
                <div className="space-y-2">
                  <div className="text-xs text-muted-foreground">名字</div>
                  <Input
                    value={name}
                    onChange={(e) => {
                      const v = e.target.value;
                      setName(v);
                      recomputeTemplatesIfNeeded(v, interests, capabilities);
                    }}
                    placeholder="例如：星尘"
                  />
                </div>

                <div className="space-y-2">
                  <div className="text-xs text-muted-foreground">一句话介绍</div>
                  <Textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
                </div>

                <div className="space-y-2">
                  <div className="text-xs text-muted-foreground">头像 URL（可选）</div>
                  <Input value={avatarUrl} onChange={(e) => setAvatarUrl(e.target.value)} placeholder="https://..." />
                </div>
              </CardContent>
            </Card>
          ) : null}

          {step === 1 ? (
            <Card className="mt-2">
              <CardContent className="pt-4 space-y-2">
                <div className="text-sm font-medium">选择 Persona 模板（可选）</div>
                <div className="text-xs text-muted-foreground">
                  仅允许“风格参考”。禁止冒充/自称为任何真实人物、虚构角色或具体动物个体。
                </div>

                <div className="mt-2 flex flex-wrap gap-2">
                  <Button
                    variant={personaTouched && !personaTemplateId ? "default" : "secondary"}
                    size="sm"
                    onClick={() => {
                      setPersonaTemplateId("");
                      setPersonaTouched(true);
                    }}
                  >
                    不设置
                  </Button>
                  {personaTemplates.slice(0, 40).map((t) => (
                    <Button
                      key={t.template_id}
                      variant={personaTemplateId === t.template_id ? "default" : "secondary"}
                      size="sm"
                      onClick={() => {
                        setPersonaTemplateId(t.template_id);
                        setPersonaTouched(true);
                      }}
                      title={t.template_id}
                    >
                      {trunc(t.template_id, 20)}
                    </Button>
                  ))}
                </div>

                {!personaTouched && agent.persona ? (
                  <div className="mt-2 rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                    当前已设置 persona（未显示模板 id；如需更换请在此步选择）。
                  </div>
                ) : null}
              </CardContent>
            </Card>
          ) : null}

          {step === 2 ? (
            <Card className="mt-2">
              <CardContent className="pt-4 space-y-2">
                <div className="text-sm font-medium">选择性格预设</div>
                <div className="text-xs text-muted-foreground">选一个就能开始；后续可再微调。</div>

                <div className="mt-2 grid grid-cols-1 gap-2">
                  {(catalogs.personality_presets ?? []).map((pp) => (
                    <Button
                      key={pp.id}
                      variant={personalityPresetId === pp.id ? "default" : "secondary"}
                      className="justify-start"
                      onClick={() => {
                        setPersonalityPresetId(pp.id);
                        setPExtrovert(Number(pp.values.extrovert));
                        setPCurious(Number(pp.values.curious));
                        setPCreative(Number(pp.values.creative));
                        setPStable(Number(pp.values.stable));
                      }}
                    >
                      <div className="text-left">
                        <div className="text-sm font-medium">{pp.label}</div>
                        {pp.description ? (
                          <div className="text-xs text-muted-foreground">{pp.description}</div>
                        ) : null}
                      </div>
                    </Button>
                  ))}
                </div>
              </CardContent>
            </Card>
          ) : null}

          {step === 3 ? (
            <MultiSelect
              title="选择兴趣（多选）"
              options={catalogs.interests ?? []}
              selected={interests}
              onChange={(next) => {
                setInterests(next);
                recomputeTemplatesIfNeeded(name, next, capabilities);
              }}
            />
          ) : null}

          {step === 4 ? (
            <MultiSelect
              title="选择能力（多选）"
              options={catalogs.capabilities ?? []}
              selected={capabilities}
              onChange={(next) => {
                setCapabilities(next);
                recomputeTemplatesIfNeeded(name, interests, next);
              }}
            />
          ) : null}

          {step === 5 ? (
            <Card className="mt-2">
              <CardContent className="pt-4 space-y-4">
                <div className="space-y-2">
                  <div className="text-sm font-medium">简介（bio）</div>
                  <div className="flex flex-wrap gap-2">
                    {(catalogs.bio_templates ?? []).slice(0, 40).map((t) => (
                      <Button
                        key={t.id}
                        size="sm"
                        variant={!bioCustom && bioTemplateId === t.id ? "default" : "secondary"}
                        onClick={() => {
                          setBioCustom(false);
                          setBioTemplateId(t.id);
                          setBio(renderCatalogTemplate(t.template, templateVars));
                        }}
                      >
                        {t.label}
                      </Button>
                    ))}
                    <Button
                      size="sm"
                      variant={bioCustom ? "default" : "secondary"}
                      onClick={() => setBioCustom((v) => !v)}
                    >
                      高级：自定义
                    </Button>
                  </div>
                  {bioCustom ? (
                    <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                      自定义内容需要审核：未通过前不可公开发现、不可同步到 OSS。
                    </div>
                  ) : null}
                  <Textarea value={bio} onChange={(e) => setBio(e.target.value)} rows={4} />
                </div>

                <div className="space-y-2">
                  <div className="text-sm font-medium">问候语（greeting）</div>
                  <div className="flex flex-wrap gap-2">
                    {(catalogs.greeting_templates ?? []).slice(0, 40).map((t) => (
                      <Button
                        key={t.id}
                        size="sm"
                        variant={!greetingCustom && greetingTemplateId === t.id ? "default" : "secondary"}
                        onClick={() => {
                          setGreetingCustom(false);
                          setGreetingTemplateId(t.id);
                          setGreeting(renderCatalogTemplate(t.template, templateVars));
                        }}
                      >
                        {t.label}
                      </Button>
                    ))}
                    <Button
                      size="sm"
                      variant={greetingCustom ? "default" : "secondary"}
                      onClick={() => setGreetingCustom((v) => !v)}
                    >
                      高级：自定义
                    </Button>
                  </div>
                  {greetingCustom ? (
                    <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                      自定义内容需要审核：未通过前不可公开发现、不可同步到 OSS。
                    </div>
                  ) : null}
                  <Textarea value={greeting} onChange={(e) => setGreeting(e.target.value)} rows={3} />
                </div>
              </CardContent>
            </Card>
          ) : null}

          {step === 6 ? (
            <Card className="mt-2">
              <CardContent className="pt-4 space-y-2 text-sm">
                <div className="flex items-center justify-between gap-2">
                  <div className="text-muted-foreground">当前审核状态</div>
                  <div className="font-medium">{agent.card_review_status || "-"}</div>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-muted-foreground">预计本次保存后</div>
                  <div className="font-medium">{willNeedReview ? "pending（需审核）" : "approved（自动通过）"}</div>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-muted-foreground">可同步到 OSS</div>
                  <div className="font-medium">{agent.card_review_status === "approved" ? "是" : "否"}</div>
                </div>
                {agent.card_review_status !== "approved" ? (
                  <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                    提示：只有 `card_review_status=approved` 才能同步到 OSS 并进入公开发现。
                  </div>
                ) : null}

                <div className="pt-2">
                  <Button variant="secondary" size="sm" onClick={() => loadAll(true)} disabled={loading || saving}>
                    刷新目录数据
                  </Button>
                </div>
              </CardContent>
            </Card>
          ) : null}
        </>
      ) : null}

      <DialogFooter className="gap-2 sm:gap-0">
        <Button variant="secondary" disabled={!canGoPrev || saving} onClick={() => setStep((s) => Math.max(0, s - 1))}>
          上一步
        </Button>
        {canGoNext ? (
          <Button disabled={saving} onClick={() => setStep((s) => Math.min(6, s + 1))}>
            下一步
          </Button>
        ) : (
          <Button disabled={saving || loading || !agent} onClick={save}>
            {saving ? "保存中…" : "保存"}
          </Button>
        )}
      </DialogFooter>
    </DialogContent>
  );
}
