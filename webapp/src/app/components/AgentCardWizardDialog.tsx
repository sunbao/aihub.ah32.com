import { useEffect, useMemo, useState } from "react";

import { useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
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
import { DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { fmtAgentStatus, fmtRunStatus, fmtTime } from "@/lib/format";
import { useI18n } from "@/lib/i18n";
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

type PreReviewEvaluation = {
  evaluation_id: string;
  agent_id: string;
  run_id: string;
  topic: string;
  status: string;
  created_at: string;
  expires_at: string;
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
  open,
  onRequestClose,
}: {
  agentId: string;
  userApiKey: string;
  onSaved?: () => void;
  open?: boolean;
  onRequestClose?: () => void;
}) {
  const { toast } = useToast();
  const { t } = useI18n();
  const nav = useNavigate();

  const isOpen = open ?? true;

  const [step, setStep] = useState(0);
  const [agent, setAgent] = useState<AgentFull | null>(null);
  const [catalogs, setCatalogs] = useState<AgentCardCatalogs | null>(null);
  const [personaTemplates, setPersonaTemplates] = useState<ApprovedPersonaTemplate[]>([]);

  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const [evalTopic, setEvalTopic] = useState("");
  const [evals, setEvals] = useState<PreReviewEvaluation[]>([]);
  const [evalLoading, setEvalLoading] = useState(false);
  const [evalCreating, setEvalCreating] = useState(false);
  const [evalDeletingId, setEvalDeletingId] = useState("");
  const [evalConfirmDelete, setEvalConfirmDelete] = useState<PreReviewEvaluation | null>(null);
  const [evalError, setEvalError] = useState("");

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

  function fmtReviewStatus(status: string): string {
    const v = String(status ?? "").trim().toLowerCase();
    switch (v) {
      case "pending":
        return t({ zh: "待审核", en: "Pending" });
      case "approved":
        return t({ zh: "已通过", en: "Approved" });
      case "rejected":
        return t({ zh: "已拒绝", en: "Rejected" });
      default:
        return status || "";
    }
  }

  function fmtAdmissionStatus(status: string): string {
    const v = String(status ?? "").trim().toLowerCase();
    switch (v) {
      case "not_requested":
        return t({ zh: "未发起入驻", en: "Not started" });
      case "pending":
        return t({ zh: "待入驻", en: "Pending" });
      case "admitted":
        return t({ zh: "已入驻", en: "Admitted" });
      case "rejected":
        return t({ zh: "已拒绝", en: "Rejected" });
      default:
        return status || "";
    }
  }

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
      console.warn("[AIHub] AgentCardWizardDialog loadAll failed", { agentId, error: e });
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!isOpen) return;
    setStep(0);
    setError("");
    setPersonaTouched(false);
    setPersonaTemplateId("");
    void loadAll(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId, isOpen]);

  async function loadEvaluations() {
    if (!agentId) return;
    setEvalLoading(true);
    setEvalError("");
    try {
      const res = await apiFetchJson<{ items: PreReviewEvaluation[] }>(
        `/v1/agents/${encodeURIComponent(agentId)}/pre-review-evaluations?limit=20`,
        { apiKey: userApiKey },
      );
      setEvals(Array.isArray(res.items) ? res.items : []);
    } catch (e: any) {
      console.warn("[AIHub] AgentCardWizardDialog load evaluations failed", { agentId, error: e });
      setEvalError(String(e?.message ?? t({ zh: "测评记录加载失败", en: "Failed to load evaluations" })));
    } finally {
      setEvalLoading(false);
    }
  }

  useEffect(() => {
    if (step !== 6) return;
    loadEvaluations();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step, agentId]);

  async function createEvaluation() {
    if (!agentId) return;
    setEvalCreating(true);
    setEvalError("");
    try {
      await apiFetchJson<{ evaluation_id: string; run_id: string; expires_at: string }>(
        `/v1/agents/${encodeURIComponent(agentId)}/pre-review-evaluations`,
        {
          method: "POST",
          apiKey: userApiKey,
          body: { topic: evalTopic.trim() },
        },
      );
      toast({ title: t({ zh: "已发起测评", en: "Evaluation started" }) });
      setEvalTopic("");
      await loadEvaluations();
    } catch (e: any) {
      console.warn("[AIHub] AgentCardWizardDialog create evaluation failed", { agentId, error: e });
      setEvalError(String(e?.message ?? t({ zh: "发起测评失败", en: "Failed to start evaluation" })));
    } finally {
      setEvalCreating(false);
    }
  }

  async function deleteEvaluation(ev: PreReviewEvaluation) {
    if (!agentId) return;
    if (!ev?.evaluation_id) return;
    setEvalDeletingId(ev.evaluation_id);
    setEvalError("");
    try {
      await apiFetchJson(`/v1/agents/${encodeURIComponent(agentId)}/pre-review-evaluations/${encodeURIComponent(ev.evaluation_id)}`, {
        method: "DELETE",
        apiKey: userApiKey,
      });
      toast({ title: t({ zh: "已删除", en: "Deleted" }) });
      await loadEvaluations();
    } catch (e: any) {
      console.warn("[AIHub] AgentCardWizardDialog delete evaluation failed", { agentId, error: e });
      setEvalError(String(e?.message ?? t({ zh: "删除失败", en: "Delete failed" })));
    } finally {
      setEvalDeletingId("");
    }
  }

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
      onRequestClose?.();
    } catch (e: any) {
      console.warn("[AIHub] AgentCardWizardDialog save failed", { agentId, error: e });
      setError(String(e?.message ?? "保存失败"));
    } finally {
      setSaving(false);
    }
  }

  function fmtPersonaTemplateLabel(tpl: ApprovedPersonaTemplate, idx: number): string {
    const p = (tpl?.persona ?? {}) as any;
    const cand = [
      String(p?.label ?? "").trim(),
      String(p?.name ?? "").trim(),
      String(p?.title ?? "").trim(),
      String(p?.display_name ?? "").trim(),
    ].filter(Boolean);
    if (cand.length) return cand[0];
    return t({ zh: `模板 ${idx + 1}`, en: `Template ${idx + 1}` });
  }

  function stepTitle(): string {
    switch (step) {
      case 0:
        return "基础信息";
      case 1:
        return t({ zh: "人设（风格参考）", en: "Persona (style reference)" });
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
    <DialogContent className="max-h-[80vh] p-0">
      <div className="flex max-h-[80vh] flex-col">
        <DialogHeader className="px-6 pt-6">
          <DialogTitle>{t({ zh: "智能体卡片向导", en: "Agent card wizard" })}</DialogTitle>
          <div className="mt-1 text-sm text-muted-foreground">{stepTitle()}</div>
          <div className="mt-3 flex flex-wrap gap-2">
            {[
              t({ zh: "基础", en: "Basics" }),
              t({ zh: "人设", en: "Persona" }),
              t({ zh: "性格", en: "Traits" }),
              t({ zh: "兴趣", en: "Interests" }),
              t({ zh: "能力", en: "Capabilities" }),
              t({ zh: "文案", en: "Copy" }),
              t({ zh: "状态", en: "Status" }),
            ].map((lbl, idx) => (
              <Button
                key={idx}
                size="sm"
                variant={step === idx ? "default" : "outline"}
                onClick={() => setStep(idx)}
                disabled={saving || loading}
              >
                {lbl}
              </Button>
            ))}
          </div>
        </DialogHeader>

        <div className="flex-1 overflow-y-auto px-6 pb-4">
          <div className="space-y-3 pt-3">
            {loading && !agent ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
            {error ? <div className="text-sm text-destructive">{error}</div> : null}

            {agent ? (
              <Card className="shadow-none">
                <CardContent className="pt-3 pb-3 text-xs text-muted-foreground">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="secondary">{fmtAgentStatus(agent.status)}</Badge>
                    {agent.card_review_status ? <Badge variant="outline">{fmtReviewStatus(agent.card_review_status)}</Badge> : null}
                    {agent.admission?.status ? <Badge variant="outline">{fmtAdmissionStatus(agent.admission.status)}</Badge> : null}
                  </div>
                  <details className="mt-2">
                    <summary className="cursor-pointer select-none font-medium text-foreground">
                      {t({ zh: "提示预览", en: "Prompt preview" })}
                    </summary>
                    <div className="mt-1 whitespace-pre-wrap">{agent.prompt_view || "（空）"}</div>
                  </details>
                </CardContent>
              </Card>
            ) : null}

            {agent && catalogs ? (
              <>
          {step === 0 ? (
            <Card className="shadow-none">
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
            <Card className="shadow-none">
              <CardContent className="pt-4 space-y-2">
                <div className="text-sm font-medium">选择人设模板（可选）</div>
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
                  {personaTemplates.slice(0, 40).map((tpl, idx) => (
                    <Button
                      key={tpl.template_id}
                      variant={personaTemplateId === tpl.template_id ? "default" : "secondary"}
                      size="sm"
                      onClick={() => {
                        setPersonaTemplateId(tpl.template_id);
                        setPersonaTouched(true);
                      }}
                    >
                      {fmtPersonaTemplateLabel(tpl, idx)}
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
            <Card className="shadow-none">
              <CardContent className="pt-4 space-y-4">
                <div className="space-y-2">
                  <div className="text-sm font-medium">简介</div>
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
                      自定义
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
                  <div className="text-sm font-medium">问候语</div>
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
                      自定义
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
            <Card className="shadow-none">
              <CardContent className="pt-4 space-y-2 text-sm">
                <div className="flex items-center justify-between gap-2">
                  <div className="text-muted-foreground">当前审核状态</div>
                  <div className="font-medium">{agent.card_review_status ? fmtReviewStatus(agent.card_review_status) : "-"}</div>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-muted-foreground">预计本次保存后</div>
                  <div className="font-medium">
                    {willNeedReview
                      ? t({ zh: "待审核（需要审核）", en: "Pending (needs review)" })
                      : t({ zh: "已通过（自动通过）", en: "Approved (auto)" })}
                  </div>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-muted-foreground">可同步到 OSS</div>
                  <div className="font-medium">
                    {agent.card_review_status === "approved" ? t({ zh: "是", en: "Yes" }) : t({ zh: "否", en: "No" })}
                  </div>
                </div>
                {agent.card_review_status !== "approved" ? (
                  <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                    {t({
                      zh: "提示：只有审核通过后才能同步到 OSS 并进入公开发现。",
                      en: "Tip: only approved cards can be synced to OSS and shown in discovery.",
                    })}
                  </div>
                ) : null}

                <div className="pt-2">
                  <Button variant="secondary" size="sm" onClick={() => loadAll(true)} disabled={loading || saving}>
                    {t({ zh: "刷新目录数据", en: "Refresh catalogs" })}
                  </Button>
                </div>

                <div className="pt-3 border-t">
                  <div className="font-medium text-foreground">{t({ zh: "提审前测评", en: "Pre-review evaluation" })}</div>
                  <div className="mt-1 text-xs text-muted-foreground">
                    {t({
                      zh: "会创建一条不公开的测评任务，由平台配置的“测评智能体”执行。测评数据可随时删除，默认 7 天后自动清理。",
                      en: "Creates an unlisted evaluation task executed by admin-configured judge agents. You can delete it anytime; it expires in 7 days by default.",
                    })}
                  </div>

                  <div className="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center">
                    <Input
                      value={evalTopic}
                      onChange={(e) => setEvalTopic(e.target.value)}
                      placeholder={t({ zh: "输入要测的话题（可空）", en: "Topic to test (optional)" })}
                    />
                    <Button size="sm" onClick={createEvaluation} disabled={evalCreating || saving || loading || evalLoading}>
                      {evalCreating ? t({ zh: "发起中…", en: "Starting…" }) : t({ zh: "发起测评", en: "Start" })}
                    </Button>
                  </div>

                  {evalError ? <div className="mt-2 text-sm text-destructive">{evalError}</div> : null}

                  <div className="mt-2 space-y-2">
                    {evalLoading ? <div className="text-xs text-muted-foreground">{t({ zh: "加载测评记录中…", en: "Loading…" })}</div> : null}
                    {!evalLoading && evals.length ? (
                      <div className="space-y-2">
                        {evals.slice(0, 5).map((ev) => (
                          <div key={ev.evaluation_id} className="rounded-md border bg-background px-3 py-2">
                            <div className="flex flex-wrap items-center justify-between gap-2">
                              <div className="min-w-0">
                                <div className="truncate text-sm font-medium">{ev.topic || t({ zh: "（未命名话题）", en: "(untitled topic)" })}</div>
                                <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                                  <Badge variant="secondary">{fmtRunStatus(ev.status)}</Badge>
                                  <span>{fmtTime(ev.created_at)}</span>
                                  <span>
                                    {t({ zh: "到期：", en: "Expires: " })}
                                    {fmtTime(ev.expires_at)}
                                  </span>
                                </div>
                              </div>
                              <div className="flex shrink-0 gap-2">
                                <Button size="sm" variant="secondary" onClick={() => nav(`/runs/${encodeURIComponent(ev.run_id)}`)}>
                                  {t({ zh: "查看结果", en: "Open" })}
                                </Button>
                                <Button
                                  size="sm"
                                  variant="destructive"
                                  disabled={evalDeletingId === ev.evaluation_id}
                                  onClick={() => setEvalConfirmDelete(ev)}
                                >
                                  {evalDeletingId === ev.evaluation_id ? t({ zh: "删除中…", en: "Deleting…" }) : t({ zh: "删除", en: "Delete" })}
                                </Button>
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : null}
                    {!evalLoading && !evals.length ? (
                      <div className="text-xs text-muted-foreground">{t({ zh: "暂无测评记录", en: "No evaluations yet." })}</div>
                    ) : null}
                  </div>
                </div>
              </CardContent>
            </Card>
          ) : null}
              </>
            ) : null}
          </div>
        </div>

        <DialogFooter className="gap-2 border-t bg-background/95 px-6 py-4 sm:gap-0">
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
      </div>

      <AlertDialog
        open={Boolean(evalConfirmDelete)}
        onOpenChange={(open) => {
          if (!open) setEvalConfirmDelete(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t({ zh: "删除测评数据？", en: "Delete evaluation?" })}</AlertDialogTitle>
            <AlertDialogDescription>
              {t({ zh: "删除后不可恢复。对应的测评任务也会一起删除。", en: "This cannot be undone. The evaluation run will be deleted as well." })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={!!evalDeletingId}>{t({ zh: "取消", en: "Cancel" })}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              disabled={!evalConfirmDelete || evalDeletingId === evalConfirmDelete?.evaluation_id}
              onClick={() => {
                const ev = evalConfirmDelete;
                setEvalConfirmDelete(null);
                if (ev) void deleteEvaluation(ev);
              }}
            >
              {evalDeletingId && evalConfirmDelete?.evaluation_id && evalDeletingId === evalConfirmDelete.evaluation_id
                ? t({ zh: "删除中…", en: "Deleting…" })
                : t({ zh: "删除", en: "Delete" })}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DialogContent>
  );
}
