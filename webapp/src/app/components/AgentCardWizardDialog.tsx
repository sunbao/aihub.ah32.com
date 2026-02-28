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
import { MultiSelect, toneClasses, type AgentCardWizardTone } from "@/app/components/AgentCardWizardMultiSelect";

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
  label?: string;
  label_en?: string;
  persona: any;
  updated_at: string;
};

type PreReviewEvaluation = {
  evaluation_id: string;
  agent_id: string;
  run_id: string;
  topic: string;
  source_run_id?: string;
  topic_id?: string;
  work_item_id?: string;
  source?: { kind: string; title?: string; summary?: string };
  status: string;
  created_at: string;
  expires_at: string;
};

type RecentTopicForEvaluation = {
  topic_id: string;
  title: string;
  summary?: string;
  mode?: string;
  opening_question?: string;
  last_message_preview?: string;
  last_message_at?: string;
};

type ListRecentTopicsForEvaluationResponse = {
  items: RecentTopicForEvaluation[];
};

type ActivityItemLite = {
  run_id: string;
  run_goal: string;
  run_status: string;
  payload: Record<string, any>;
  created_at: string;
};

type ActivityResponseLite = {
  items: ActivityItemLite[];
};

type OwnerRunWorkItem = {
  work_item_id: string;
  stage: string;
  kind: string;
  status: string;
  stage_description?: string;
  created_at: string;
};

type OwnerListRunWorkItemsResponse = {
  run_id: string;
  run_goal: string;
  run_status: string;
  items: OwnerRunWorkItem[];
};

type OwnerGetPreReviewEvaluationResponse = {
  evaluation_id: string;
  agent_id: string;
  run_id: string;
  topic: string;
  topic_id?: string;
  work_item_id?: string;
  source_run_id?: string;
  source?: { kind: string; title?: string; summary?: string };
  source_snapshot?: any;
  status: string;
  created_at: string;
  expires_at: string;
};

function isUuidLike(s: string): boolean {
  const v = String(s ?? "").trim();
  if (!v) return false;
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
}

function redactUuids(v: any): any {
  if (v == null) return v;
  if (typeof v === "string") return isUuidLike(v) ? "（已隐藏）" : v;
  if (Array.isArray(v)) return v.map(redactUuids);
  if (typeof v === "object") {
    const out: any = {};
    for (const k of Object.keys(v)) out[k] = redactUuids((v as any)[k]);
    return out;
  }
  return v;
}

function normalizeText(s: string): string {
  return String(s ?? "")
    .trim()
    .replace(/\s+/g, " ");
}

type CatalogTemplateVars = { name: string; interests: string[]; capabilities: string[] };

function findMatchingTemplateId(
  templates: CatalogTextTemplate[] | undefined,
  renderedText: string,
  vars: { zh: CatalogTemplateVars; en: CatalogTemplateVars },
): string {
  const want = normalizeText(renderedText);
  if (!want) return "";
  for (const t of templates ?? []) {
    if (t.template) {
      const got = normalizeText(renderCatalogTemplate(t.template, vars.zh, { joiner: "、" }));
      if (got && got === want) return String(t.id ?? "").trim();
    }
    if (t.template_en) {
      const got = normalizeText(renderCatalogTemplate(t.template_en, vars.en, { joiner: ", " }));
      if (got && got === want) return String(t.id ?? "").trim();
    }
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

function buildLabelToEnMap(items: CatalogLabeledItem[] | undefined): Map<string, string> {
  const m = new Map<string, string>();
  for (const it of items ?? []) {
    const zh = String(it.label ?? "").trim();
    const en = String(it.label_en ?? "").trim();
    if (!zh || !en) continue;
    m.set(zh, en);
  }
  return m;
}

function mapLabelsToEn(labels: string[], map: Map<string, string>): string[] {
  return (labels ?? [])
    .map((v) => {
      const key = String(v ?? "").trim();
      if (!key) return "";
      return String(map.get(key) ?? key).trim();
    })
    .filter(Boolean);
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
  const { t, isZh } = useI18n();
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
  const [evalTopicId, setEvalTopicId] = useState("");
  const [evalWorkItemId, setEvalWorkItemId] = useState("");
  const [evalSourceRunId, setEvalSourceRunId] = useState("");
  const [evalSourceKind, setEvalSourceKind] = useState<"topic" | "work_item" | "run">("topic");
  const [evalSourceTitle, setEvalSourceTitle] = useState("");

  const [recentTopics, setRecentTopics] = useState<RecentTopicForEvaluation[]>([]);
  const [recentTopicsLoading, setRecentTopicsLoading] = useState(false);
  const [recentTopicsError, setRecentTopicsError] = useState("");

  const [recentRuns, setRecentRuns] = useState<ActivityItemLite[]>([]);
  const [recentRunsLoading, setRecentRunsLoading] = useState(false);
  const [recentRunsError, setRecentRunsError] = useState("");

  const [workItemRunId, setWorkItemRunId] = useState("");
  const [workItemRunTitle, setWorkItemRunTitle] = useState("");
  const [workItemRunItems, setWorkItemRunItems] = useState<OwnerRunWorkItem[]>([]);
  const [workItemRunLoading, setWorkItemRunLoading] = useState(false);
  const [workItemRunError, setWorkItemRunError] = useState("");

  const [snapshotEval, setSnapshotEval] = useState<PreReviewEvaluation | null>(null);
  const [snapshotLoading, setSnapshotLoading] = useState(false);
  const [snapshotError, setSnapshotError] = useState("");
  const [snapshotData, setSnapshotData] = useState<OwnerGetPreReviewEvaluationResponse | null>(null);
  const [snapshotShowRaw, setSnapshotShowRaw] = useState(false);
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
  const [showAllPersonaTemplates, setShowAllPersonaTemplates] = useState(false);

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

  const interestLabelToEn = useMemo(() => buildLabelToEnMap(catalogs?.interests), [catalogs]);
  const capabilityLabelToEn = useMemo(() => buildLabelToEnMap(catalogs?.capabilities), [catalogs]);

  const templateVarsZh = useMemo(
    () => ({ name: name.trim(), interests: interests ?? [], capabilities: capabilities ?? [] }),
    [capabilities, interests, name],
  );
  const templateVarsEn = useMemo(
    () => ({
      name: name.trim(),
      interests: mapLabelsToEn(interests ?? [], interestLabelToEn),
      capabilities: mapLabelsToEn(capabilities ?? [], capabilityLabelToEn),
    }),
    [capabilities, capabilityLabelToEn, interestLabelToEn, interests, name],
  );

  function recomputeTemplatesIfNeeded(nextName: string, nextInterests: string[], nextCapabilities: string[]) {
    const varsZh = { name: nextName.trim(), interests: nextInterests, capabilities: nextCapabilities };
    const varsEn = {
      name: nextName.trim(),
      interests: mapLabelsToEn(nextInterests, interestLabelToEn),
      capabilities: mapLabelsToEn(nextCapabilities, capabilityLabelToEn),
    };
    if (!bioCustom && bioTemplateId) {
      const t = (catalogs?.bio_templates ?? []).find((x) => String(x.id ?? "") === bioTemplateId);
      const useEnTemplate = !isZh && !!t?.template_en;
      const tmpl = useEnTemplate ? String(t?.template_en ?? "") : String(t?.template ?? "");
      const joiner = useEnTemplate ? ", " : "、";
      const vars = useEnTemplate ? varsEn : varsZh;
      if (tmpl) setBio(renderCatalogTemplate(tmpl, vars, { joiner }));
    }
    if (!greetingCustom && greetingTemplateId) {
      const t = (catalogs?.greeting_templates ?? []).find((x) => String(x.id ?? "") === greetingTemplateId);
      const useEnTemplate = !isZh && !!t?.template_en;
      const tmpl = useEnTemplate ? String(t?.template_en ?? "") : String(t?.template ?? "");
      const joiner = useEnTemplate ? ", " : "、";
      const vars = useEnTemplate ? varsEn : varsZh;
      if (tmpl) setGreeting(renderCatalogTemplate(tmpl, vars, { joiner }));
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

      const interestToEn = buildLabelToEnMap(c.interests);
      const capabilityToEn = buildLabelToEnMap(c.capabilities);
      const varsZh = { name: String(a.name ?? "").trim(), interests: a.interests ?? [], capabilities: a.capabilities ?? [] };
      const varsEn = {
        name: String(a.name ?? "").trim(),
        interests: mapLabelsToEn(a.interests ?? [], interestToEn),
        capabilities: mapLabelsToEn(a.capabilities ?? [], capabilityToEn),
      };
      const bioMatch = findMatchingTemplateId(c.bio_templates, a.bio ?? "", { zh: varsZh, en: varsEn });
      const greetMatch = findMatchingTemplateId(c.greeting_templates, a.greeting ?? "", { zh: varsZh, en: varsEn });
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
    void loadRecentTopics();
    void loadRecentRuns();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step, agentId]);

  async function loadRecentTopics() {
    if (!agentId) return;
    setRecentTopicsLoading(true);
    setRecentTopicsError("");
    try {
      const res = await apiFetchJson<ListRecentTopicsForEvaluationResponse>(
        `/v1/pre-review-evaluation/sources/recent-topics?limit=10&candidate_agent_id=${encodeURIComponent(agentId)}`,
        { apiKey: userApiKey },
      );
      setRecentTopics(Array.isArray(res.items) ? res.items : []);
    } catch (e: any) {
      console.warn("[AIHub] AgentCardWizardDialog load recent topics failed", { agentId, error: e });
      setRecentTopicsError(String(e?.message ?? t({ zh: "加载失败", en: "Load failed" })));
      setRecentTopics([]);
    } finally {
      setRecentTopicsLoading(false);
    }
  }

  async function loadRecentRuns() {
    setRecentRunsLoading(true);
    setRecentRunsError("");
    try {
      const res = await apiFetchJson<ActivityResponseLite>(`/v1/activity?limit=10&offset=0`, { apiKey: userApiKey });
      setRecentRuns(Array.isArray(res.items) ? res.items : []);
    } catch (e: any) {
      console.warn("[AIHub] AgentCardWizardDialog load recent runs failed", { error: e });
      setRecentRunsError(String(e?.message ?? t({ zh: "加载失败", en: "Load failed" })));
      setRecentRuns([]);
    } finally {
      setRecentRunsLoading(false);
    }
  }

  async function loadWorkItemsForRun(runId: string) {
    const rid = runId.trim();
    if (!rid) return;
    setWorkItemRunLoading(true);
    setWorkItemRunError("");
    try {
      const res = await apiFetchJson<OwnerListRunWorkItemsResponse>(`/v1/runs/${encodeURIComponent(rid)}/work-items?limit=80`, {
        apiKey: userApiKey,
      });
      setWorkItemRunItems(Array.isArray(res.items) ? res.items : []);
    } catch (e: any) {
      console.warn("[AIHub] AgentCardWizardDialog load run work items failed", { runId: rid, error: e });
      setWorkItemRunError(String(e?.message ?? t({ zh: "加载任务失败", en: "Failed to load work items" })));
      setWorkItemRunItems([]);
    } finally {
      setWorkItemRunLoading(false);
    }
  }

  function clearEvalSourceIds() {
    setEvalTopicId("");
    setEvalWorkItemId("");
    setEvalSourceRunId("");
    setEvalSourceTitle("");
  }

  function fmtWorkItemStatus(status: string): string {
    const v = String(status ?? "").trim().toLowerCase();
    switch (v) {
      case "offered":
        return t({ zh: "待领取", en: "Offered" });
      case "claimed":
        return t({ zh: "进行中", en: "Claimed" });
      case "completed":
        return t({ zh: "已完成", en: "Completed" });
      case "failed":
        return t({ zh: "失败", en: "Failed" });
      case "scheduled":
        return t({ zh: "已排队", en: "Scheduled" });
      default:
        return status || "";
    }
  }

  function fmtWorkItemStage(stage: string): string {
    const v = String(stage ?? "").trim().toLowerCase();
    switch (v) {
      case "review":
        return t({ zh: "评审", en: "Review" });
      case "onboarding":
        return t({ zh: "入驻", en: "Onboarding" });
      case "checkin":
        return t({ zh: "签到", en: "Check-in" });
      case "ideation":
        return t({ zh: "构思", en: "Ideation" });
      default:
        return stage || "";
    }
  }

  function fmtWorkItemKind(kind: string): string {
    const v = String(kind ?? "").trim().toLowerCase();
    switch (v) {
      case "draft":
        return t({ zh: "草稿", en: "Draft" });
      case "contribute":
        return t({ zh: "贡献", en: "Contribute" });
      case "review":
        return t({ zh: "评审", en: "Review" });
      default:
        return kind || "";
    }
  }

  function pickTopic(topic: RecentTopicForEvaluation) {
    clearEvalSourceIds();
    setEvalSourceKind("topic");
    setEvalTopicId(String(topic.topic_id ?? "").trim());
    setEvalSourceTitle(String(topic.title ?? "").trim());
    if (!evalTopic.trim() && String(topic.title ?? "").trim()) setEvalTopic(String(topic.title ?? "").trim());
  }

  function pickRunAsScenario(runId: string, runGoal?: string) {
    clearEvalSourceIds();
    setEvalSourceKind("run");
    setEvalSourceRunId(runId.trim());
    setEvalSourceTitle(String(runGoal ?? "").trim());
    if (!evalTopic.trim() && String(runGoal ?? "").trim()) setEvalTopic(String(runGoal ?? "").trim());
  }

  function pickWorkItem(workItem: OwnerRunWorkItem) {
    clearEvalSourceIds();
    setEvalSourceKind("work_item");
    setEvalWorkItemId(String(workItem.work_item_id ?? "").trim());
    setEvalSourceTitle([fmtWorkItemStage(workItem.stage_description || workItem.stage), fmtWorkItemKind(workItem.kind)].filter(Boolean).join(" · "));
  }

  async function createEvaluation() {
    if (!agentId) return;
    setEvalCreating(true);
    setEvalError("");
    try {
      const topicId = evalTopicId.trim();
      const workItemId = evalWorkItemId.trim();
      const sourceRunId = evalSourceRunId.trim();
      const kinds = [topicId, workItemId, sourceRunId].filter(Boolean).length;
      if (kinds === 0) {
        setEvalError(t({ zh: "请选择一个真实话题/任务/场景（任选其一）", en: "Pick a real topic/task/scenario (choose one)." }));
        return;
      }
      if (kinds > 1) {
        setEvalError(t({ zh: "请只选择一个来源（话题 / 任务 / 场景）", en: "Choose only one source (topic / task / scenario)." }));
        return;
      }

      const body: any = { topic: evalTopic.trim() };
      if (topicId) body.topic_id = topicId;
      if (workItemId) body.work_item_id = workItemId;
      if (sourceRunId) body.source_run_id = sourceRunId;

      await apiFetchJson<{ evaluation_id: string; run_id: string; expires_at: string }>(
        `/v1/agents/${encodeURIComponent(agentId)}/pre-review-evaluations`,
        {
          method: "POST",
          apiKey: userApiKey,
          body,
        },
      );
      toast({ title: t({ zh: "已发起测评", en: "Evaluation started" }) });
      setEvalTopic("");
      clearEvalSourceIds();
      setWorkItemRunId("");
      setWorkItemRunTitle("");
      setWorkItemRunItems([]);
      await loadEvaluations();
    } catch (e: any) {
      console.warn("[AIHub] AgentCardWizardDialog create evaluation failed", { agentId, error: e });
      setEvalError(String(e?.message ?? t({ zh: "发起测评失败", en: "Failed to start evaluation" })));
    } finally {
      setEvalCreating(false);
    }
  }

  async function openSnapshot(ev: PreReviewEvaluation) {
    if (!agentId) return;
    if (!ev?.evaluation_id) return;
    setSnapshotEval(ev);
    setSnapshotLoading(true);
    setSnapshotError("");
    setSnapshotData(null);
    setSnapshotShowRaw(false);
    try {
      const res = await apiFetchJson<OwnerGetPreReviewEvaluationResponse>(
        `/v1/agents/${encodeURIComponent(agentId)}/pre-review-evaluations/${encodeURIComponent(ev.evaluation_id)}`,
        { apiKey: userApiKey },
      );
      setSnapshotData(res);
    } catch (e: any) {
      console.warn("[AIHub] AgentCardWizardDialog load evaluation snapshot failed", { agentId, evaluationId: ev.evaluation_id, error: e });
      setSnapshotError(String(e?.message ?? t({ zh: "加载快照失败", en: "Failed to load snapshot" })));
    } finally {
      setSnapshotLoading(false);
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
    const bioOk = !!findMatchingTemplateId(catalogs.bio_templates, bio, { zh: templateVarsZh, en: templateVarsEn });
    const greetingOk = !!findMatchingTemplateId(catalogs.greeting_templates, greeting, { zh: templateVarsZh, en: templateVarsEn });
    return bioCustom || greetingCustom || !bioOk || !greetingOk;
  }, [
    bio,
    bioCustom,
    capabilities,
    capabilityLabelSet,
    catalogs,
    greeting,
    greetingCustom,
    interestLabelSet,
    interests,
    templateVarsEn,
    templateVarsZh,
  ]);

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

      toast({ title: t({ zh: "已保存", en: "Saved" }) });
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
    const direct = isZh ? String(tpl?.label ?? "").trim() : String(tpl?.label_en ?? "").trim();
    if (direct) return direct;

    const p = (tpl?.persona ?? {}) as any;
    const raw = isZh
      ? [
          String(p?.label ?? "").trim(),
          String(p?.name ?? "").trim(),
          String(p?.title ?? "").trim(),
          String(p?.display_name ?? "").trim(),
          String(p?.inspiration?.reference ?? "").trim(),
        ]
      : [
          String(p?.label_en ?? "").trim(),
          String(p?.name_en ?? "").trim(),
          String(p?.title_en ?? "").trim(),
          String(p?.display_name_en ?? "").trim(),
          String(p?.inspiration?.reference_en ?? "").trim(),
          String(p?.label ?? "").trim(),
          String(p?.name ?? "").trim(),
          String(p?.title ?? "").trim(),
          String(p?.display_name ?? "").trim(),
          String(p?.inspiration?.reference ?? "").trim(),
        ];
    const tid = String(tpl?.template_id ?? "").trim();
    const cand = raw
      .filter(Boolean)
      .filter((s) => String(s).trim() !== tid)
      .filter((s) => !/^persona_/.test(String(s)) && !/^custom_/.test(String(s)));
    if (cand.length) return String(cand[0]);
    return t({ zh: `模板 ${idx + 1}`, en: `Template ${idx + 1}` });
  }

  function stepTitle(): string {
    switch (step) {
      case 0:
        return t({ zh: "基础信息", en: "Basics" });
      case 1:
        return t({ zh: "人设（风格参考）", en: "Persona (style reference)" });
      case 2:
        return t({ zh: "性格预设", en: "Personality preset" });
      case 3:
        return t({ zh: "兴趣", en: "Interests" });
      case 4:
        return t({ zh: "能力", en: "Capabilities" });
      case 5:
        return t({ zh: "简介与问候", en: "Bio & greeting" });
      default:
        return t({ zh: "预览与状态", en: "Review & status" });
    }
  }

  const stepTone = useMemo<AgentCardWizardTone>(() => {
    switch (step) {
      case 0:
        return "cyan";
      case 1:
        return "violet";
      case 2:
        return "indigo";
      case 3:
        return "green";
      case 4:
        return "sky";
      case 5:
        return "amber";
      default:
        return "slate";
    }
  }, [step]);

  const toneCard = useMemo(() => toneClasses(stepTone).card || "border-l-4 border-slate-500/40", [stepTone]);

  const basicsValid = useMemo(() => {
    return String(name ?? "").trim().length > 0 && String(description ?? "").trim().length > 0;
  }, [description, name]);
  const interestsValid = useMemo(() => (interests ?? []).filter(Boolean).length > 0, [interests]);
  const capabilitiesValid = useMemo(() => (capabilities ?? []).filter(Boolean).length > 0, [capabilities]);
  const copyValid = useMemo(() => String(bio ?? "").trim().length > 0 && String(greeting ?? "").trim().length > 0, [bio, greeting]);

  const maxUnlockedStep = useMemo(() => {
    // Sequential gating: required steps must be satisfied to unlock subsequent steps.
    if (!basicsValid) return 0;
    // Persona + traits are optional.
    if (!interestsValid) return 3;
    if (!capabilitiesValid) return 4;
    if (!copyValid) return 5;
    return 6;
  }, [basicsValid, capabilitiesValid, copyValid, interestsValid]);

  const canGoPrev = step > 0;
  const canGoNext = step < 6 && step < maxUnlockedStep;

  useEffect(() => {
    if (step > maxUnlockedStep) setStep(maxUnlockedStep);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [maxUnlockedStep]);

  const requiredBlocker = useMemo(() => {
    if (!basicsValid) return t({ zh: "请先完成必填：名字 + 一句话介绍", en: "Complete required fields: name + one-liner." });
    if (!interestsValid) return t({ zh: "请先在第 4 步至少选择 1 项", en: "Step 4: pick at least 1 item." });
    if (!capabilitiesValid) return t({ zh: "请先在第 5 步至少选择 1 项", en: "Step 5: pick at least 1 item." });
    if (!copyValid) return t({ zh: "请先完成必填：简介 + 问候语", en: "Complete required fields: bio + greeting." });
    return "";
  }, [basicsValid, capabilitiesValid, copyValid, interestsValid, t]);

  const promptPreviewText = useMemo(() => {
    const parts: string[] = [];
    const sep = isZh ? "；" : "; ";
    const joiner = isZh ? "、" : ", ";

    function clip(s: string, maxLen: number): string {
      const v = String(s ?? "").trim();
      if (!v) return "";
      if (v.length <= maxLen) return v;
      return v.slice(0, maxLen).trim() + "…";
    }

    const nameV = String(name ?? "").trim();
    const descV = String(description ?? "").trim();
    if (nameV || descV) {
      if (nameV && descV) parts.push(`${nameV}：${descV}`);
      else parts.push(nameV || descV);
    }

    if (String(personaTemplateId ?? "").trim()) {
      const tpl = (personaTemplates ?? []).find((x) => String(x.template_id) === String(personaTemplateId));
      const label = tpl ? fmtPersonaTemplateLabel(tpl, 0) : "";
      if (label) parts.push(label);
    }

    if (String(personalityPresetId ?? "").trim() && catalogs?.personality_presets?.length) {
      const pp = (catalogs.personality_presets ?? []).find((x) => String(x.id) === String(personalityPresetId));
      const label = pp ? (isZh ? String(pp.label ?? "") : String(pp.label_en ?? "").trim() || String(pp.label ?? "")) : "";
      if (label) parts.push(label);
    }

    const traitText = isZh
      ? `外${Math.round(pExtrovert * 100)}/奇${Math.round(pCurious * 100)}/创${Math.round(pCreative * 100)}/稳${Math.round(pStable * 100)}`
      : `E${Math.round(pExtrovert * 100)}/C${Math.round(pCurious * 100)}/Cr${Math.round(pCreative * 100)}/S${Math.round(pStable * 100)}`;
    parts.push(traitText);

    const interestsV = (interests ?? []).map((x) => String(x ?? "").trim()).filter(Boolean);
    if (interestsV.length) parts.push(interestsV.slice(0, 24).join(joiner));

    const capabilitiesV = (capabilities ?? []).map((x) => String(x ?? "").trim()).filter(Boolean);
    if (capabilitiesV.length) parts.push(capabilitiesV.slice(0, 24).join(joiner));

    const bioV = clip(String(bio ?? ""), 120);
    if (bioV) parts.push(bioV);

    const greetV = clip(String(greeting ?? ""), 120);
    if (greetV) parts.push(greetV);

    const evalTopicV = String(evalTopic ?? "").trim();
    const evalSourceTitleV = String(evalSourceTitle ?? "").trim();
    if (evalSourceTitleV) parts.push(evalSourceTitleV);
    if (evalTopicV) parts.push(clip(evalTopicV, 120));

    return parts.filter(Boolean).join(sep);
  }, [
    agent?.persona,
    avatarUrl,
    bio,
    capabilities,
    catalogs,
    description,
    evalSourceKind,
    evalSourceRunId,
    evalSourceTitle,
    evalTopic,
    evalTopicId,
    evalWorkItemId,
    greeting,
    interests,
    isZh,
    name,
    pCreative,
    pCurious,
    pExtrovert,
    pStable,
    personaTemplateId,
    personaTemplates,
    personaTouched,
    personalityPresetId,
    t,
  ]);

  return (
    <DialogContent className="max-h-[80vh] p-0">
      <div className="flex max-h-[80vh] flex-col">
        <DialogHeader className="px-6 pt-6">
          <DialogTitle>{t({ zh: "智能体卡片向导", en: "Agent Card Wizard" })}</DialogTitle>
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
                className={idx > maxUnlockedStep ? "opacity-50" : ""}
                onClick={() => {
                  if (saving || loading) return;
                  if (idx > maxUnlockedStep) {
                    toast({
                      title: t({ zh: "请先完成必填项", en: "Complete required fields first" }),
                      description: requiredBlocker || undefined,
                    });
                    return;
                  }
                  setStep(idx);
                }}
                disabled={saving || loading}
              >
                {lbl}
              </Button>
            ))}
          </div>
        </DialogHeader>

        <div className="flex-1 overflow-y-auto px-6 pb-4">
          <div className="sticky top-0 z-10 -mx-6 px-6 pt-3 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
            {loading && !agent ? <div className="text-sm text-muted-foreground">{t({ zh: "加载中…", en: "Loading…" })}</div> : null}
            {error ? <div className="text-sm text-destructive">{error}</div> : null}

            {agent ? (
              <Card className={`shadow-none ${toneCard}`}>
                <CardContent className="pt-3 pb-3 text-xs text-muted-foreground">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="secondary">{fmtAgentStatus(agent.status)}</Badge>
                    {agent.card_review_status ? <Badge variant="outline">{fmtReviewStatus(agent.card_review_status)}</Badge> : null}
                    {agent.admission?.status ? <Badge variant="outline">{fmtAdmissionStatus(agent.admission.status)}</Badge> : null}
                  </div>
                  <details className="mt-2" open>
                    <summary className="cursor-pointer select-none font-medium text-foreground">
                      {t({ zh: "预览", en: "Preview" })}
                    </summary>
                    {promptPreviewText ? <div className="mt-2 text-xs text-foreground break-words">{promptPreviewText}</div> : null}
                    {requiredBlocker ? <div className="mt-2 text-xs text-destructive">{requiredBlocker}</div> : null}
                    <details className="mt-2">
                      <summary className="cursor-pointer select-none text-xs text-muted-foreground">
                        {t({ zh: "已保存版本（prompt_view）", en: "Saved version (prompt_view)" })}
                      </summary>
                      <div className="mt-1 whitespace-pre-wrap text-xs">{agent.prompt_view || t({ zh: "（空）", en: "(empty)" })}</div>
                    </details>
                  </details>
                </CardContent>
              </Card>
            ) : null}
          </div>

          <div className="space-y-3 pt-3">

            {agent && catalogs ? (
              <>
          {step === 0 ? (
            <Card className={`shadow-none ${toneCard}`}>
              <CardContent className="pt-4 space-y-3">
                <div className="space-y-2">
                  <div className="text-xs text-muted-foreground">{t({ zh: "名字（必填）", en: "Name (required)" })}</div>
                  <Input
                    value={name}
                    onChange={(e) => {
                      const v = e.target.value;
                      setName(v);
                      recomputeTemplatesIfNeeded(v, interests, capabilities);
                    }}
                    placeholder={t({ zh: "例如：星尘", en: "e.g. Stardust" })}
                  />
                </div>

                <div className="space-y-2">
                  <div className="text-xs text-muted-foreground">{t({ zh: "一句话介绍（必填）", en: "One-liner (required)" })}</div>
                  <Textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
                </div>

                <div className="space-y-2">
                  <div className="text-xs text-muted-foreground">{t({ zh: "头像 URL（可选）", en: "Avatar URL (optional)" })}</div>
                  <Input value={avatarUrl} onChange={(e) => setAvatarUrl(e.target.value)} placeholder="https://..." />
                </div>

                {!basicsValid ? (
                  <div className="text-xs text-destructive">{t({ zh: "名字和一句话介绍为必填", en: "Name and one-liner are required." })}</div>
                ) : null}
              </CardContent>
            </Card>
          ) : null}

          {step === 1 ? (
            <Card className={`shadow-none ${toneCard}`}>
              <CardContent className="pt-4 space-y-2">
                <div className="text-sm font-medium">{t({ zh: "选择人设模板（可选）", en: "Pick a persona template (optional)" })}</div>
                <div className="text-xs text-muted-foreground">
                  {t({
                    zh: "仅允许“风格参考”。禁止冒充/自称为任何真实人物、虚构角色或具体动物个体。",
                    en: "Style reference only. No impersonation of real people, fictional characters, or specific animals.",
                  })}
                </div>
                <div className="text-xs text-muted-foreground">
                  {t({ zh: "提示：再次点击已选模板可取消", en: "Tip: click the selected template again to clear." })}
                </div>

                <div className="mt-2 flex flex-wrap gap-2">
                  {(showAllPersonaTemplates ? personaTemplates : personaTemplates.slice(0, 40)).map((tpl, idx) => (
                    <Button
                      key={tpl.template_id}
                      variant={personaTemplateId === tpl.template_id ? "secondary" : "outline"}
                      className={personaTemplateId === tpl.template_id ? toneClasses("violet").active : toneClasses("violet").inactive}
                      size="sm"
                      onClick={() => {
                        setPersonaTemplateId((cur) => (cur === tpl.template_id ? "" : tpl.template_id));
                        setPersonaTouched(true);
                      }}
                    >
                      {fmtPersonaTemplateLabel(tpl, idx)}
                    </Button>
                  ))}
                </div>

                {personaTemplates.length > 40 ? (
                  <div className="pt-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setShowAllPersonaTemplates((v) => !v)}
                      disabled={saving || loading}
                    >
                      {showAllPersonaTemplates
                        ? t({ zh: "收起人设模板", en: "Show fewer templates" })
                        : t({ zh: "显示更多人设模板", en: "Show more templates" })}
                    </Button>
                  </div>
                ) : null}

                {!personaTouched && agent.persona ? (
                  <div className="mt-2 rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                    {t({
                      zh: "当前已设置 persona（未展示模板 ID；如需更换请在此步选择）。",
                      en: "Persona is already set (template ID hidden). Pick a template here to change it.",
                    })}
                  </div>
                ) : null}
              </CardContent>
            </Card>
          ) : null}

          {step === 2 ? (
            <Card className={`mt-2 shadow-none ${toneCard}`}>
              <CardContent className="pt-4 space-y-2">
                <div className="text-sm font-medium">{t({ zh: "选择性格预设", en: "Pick a personality preset" })}</div>
                <div className="text-xs text-muted-foreground">{t({ zh: "选一个就能开始；后续可再微调。", en: "Pick one to start; you can fine-tune later." })}</div>

                <div className="mt-2 grid grid-cols-1 gap-2">
                  {(catalogs.personality_presets ?? []).map((pp) => {
                    const label = isZh ? String(pp.label ?? "") : String(pp.label_en ?? "").trim() || String(pp.label ?? "");
                    const desc = isZh
                      ? String(pp.description ?? "")
                      : String(pp.description_en ?? "").trim() || String(pp.description ?? "");
                    return (
                      <Button
                        key={pp.id}
                        variant={personalityPresetId === pp.id ? "secondary" : "outline"}
                        className={`justify-start ${personalityPresetId === pp.id ? toneClasses("indigo").active : toneClasses("indigo").inactive}`}
                        onClick={() => {
                          setPersonalityPresetId(pp.id);
                          setPExtrovert(Number(pp.values.extrovert));
                          setPCurious(Number(pp.values.curious));
                          setPCreative(Number(pp.values.creative));
                          setPStable(Number(pp.values.stable));
                        }}
                      >
                        <div className="text-left">
                          <div className="text-sm font-medium">{label}</div>
                          {desc ? <div className="text-xs text-muted-foreground">{desc}</div> : null}
                        </div>
                      </Button>
                    );
                  })}
                </div>
              </CardContent>
            </Card>
          ) : null}

          {step === 3 ? (
            <MultiSelect
              title={t({ zh: "选择兴趣（多选）", en: "Select interests (multi)" })}
              options={catalogs.interests ?? []}
              selected={interests}
              tone="green"
              required
              onChange={(next) => {
                setInterests(next);
                recomputeTemplatesIfNeeded(name, next, capabilities);
              }}
            />
          ) : null}

          {step === 4 ? (
            <MultiSelect
              title={t({ zh: "选择能力（多选）", en: "Select capabilities (multi)" })}
              options={catalogs.capabilities ?? []}
              selected={capabilities}
              tone="sky"
              required
              onChange={(next) => {
                setCapabilities(next);
                recomputeTemplatesIfNeeded(name, interests, next);
              }}
            />
          ) : null}

          {step === 5 ? (
            <Card className={`shadow-none ${toneCard}`}>
              <CardContent className="pt-4 space-y-4">
                <div className="space-y-2">
                  <div className="text-sm font-medium">{t({ zh: "简介", en: "Bio" })}</div>
                  <div className="flex flex-wrap gap-2">
                    {(catalogs.bio_templates ?? []).slice(0, 40).map((tpl) => {
                      const useEnTemplate = !isZh && !!tpl.template_en;
                      const tmpl = useEnTemplate ? String(tpl.template_en ?? "") : String(tpl.template ?? "");
                      const joiner = useEnTemplate ? ", " : "、";
                      const vars = useEnTemplate ? templateVarsEn : templateVarsZh;
                      const label = isZh ? String(tpl.label ?? "") : String(tpl.label_en ?? "").trim() || String(tpl.label ?? "");
                      return (
                        <Button
                          key={tpl.id}
                          size="sm"
                          variant={!bioCustom && bioTemplateId === tpl.id ? "secondary" : "outline"}
                          className={!bioCustom && bioTemplateId === tpl.id ? toneClasses("amber").active : toneClasses("amber").inactive}
                          onClick={() => {
                            setBioCustom(false);
                            setBioTemplateId(tpl.id);
                            setBio(renderCatalogTemplate(tmpl, vars, { joiner }));
                          }}
                        >
                          {label}
                        </Button>
                      );
                    })}
                    <Button
                      size="sm"
                      variant={bioCustom ? "secondary" : "outline"}
                      className={bioCustom ? toneClasses("amber").active : toneClasses("amber").inactive}
                      onClick={() => setBioCustom((v) => !v)}
                    >
                      {t({ zh: "自定义", en: "Custom" })}
                    </Button>
                  </div>
                  {bioCustom ? (
                    <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                      {t({
                        zh: "自定义内容需要审核：未通过前不可公开发现、不可同步到 OSS。",
                        en: "Custom content requires review: before approval, it can't be public or synced to OSS.",
                      })}
                    </div>
                  ) : null}
                  <Textarea value={bio} onChange={(e) => setBio(e.target.value)} rows={4} />
                </div>

                <div className="space-y-2">
                  <div className="text-sm font-medium">{t({ zh: "问候语", en: "Greeting" })}</div>
                  <div className="flex flex-wrap gap-2">
                    {(catalogs.greeting_templates ?? []).slice(0, 40).map((tpl) => {
                      const useEnTemplate = !isZh && !!tpl.template_en;
                      const tmpl = useEnTemplate ? String(tpl.template_en ?? "") : String(tpl.template ?? "");
                      const joiner = useEnTemplate ? ", " : "、";
                      const vars = useEnTemplate ? templateVarsEn : templateVarsZh;
                      const label = isZh ? String(tpl.label ?? "") : String(tpl.label_en ?? "").trim() || String(tpl.label ?? "");
                      return (
                        <Button
                          key={tpl.id}
                          size="sm"
                          variant={!greetingCustom && greetingTemplateId === tpl.id ? "secondary" : "outline"}
                          className={!greetingCustom && greetingTemplateId === tpl.id ? toneClasses("amber").active : toneClasses("amber").inactive}
                          onClick={() => {
                            setGreetingCustom(false);
                            setGreetingTemplateId(tpl.id);
                            setGreeting(renderCatalogTemplate(tmpl, vars, { joiner }));
                          }}
                        >
                          {label}
                        </Button>
                      );
                    })}
                    <Button
                      size="sm"
                      variant={greetingCustom ? "secondary" : "outline"}
                      className={greetingCustom ? toneClasses("amber").active : toneClasses("amber").inactive}
                      onClick={() => setGreetingCustom((v) => !v)}
                    >
                      {t({ zh: "自定义", en: "Custom" })}
                    </Button>
                  </div>
                  {greetingCustom ? (
                    <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                      {t({
                        zh: "自定义内容需要审核：未通过前不可公开发现、不可同步到 OSS。",
                        en: "Custom content requires review: before approval, it can't be public or synced to OSS.",
                      })}
                    </div>
                  ) : null}
                  <Textarea value={greeting} onChange={(e) => setGreeting(e.target.value)} rows={3} />
                </div>

                {!copyValid ? (
                  <div className="text-xs text-destructive">
                    {t({ zh: "简介和问候语为必填，填写后才能进入下一步", en: "Bio and greeting are required to continue." })}
                  </div>
                ) : null}
              </CardContent>
            </Card>
          ) : null}

          {step === 6 ? (
            <Card className={`shadow-none ${toneCard}`}>
              <CardContent className="pt-4 space-y-2 text-sm">
                <div className="flex items-center justify-between gap-2">
                  <div className="text-muted-foreground">{t({ zh: "当前审核状态", en: "Current review status" })}</div>
                  <div className="font-medium">{agent.card_review_status ? fmtReviewStatus(agent.card_review_status) : "-"}</div>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-muted-foreground">{t({ zh: "预计本次保存后", en: "Expected after save" })}</div>
                  <div className="font-medium">
                    {willNeedReview
                      ? t({ zh: "待审核（需要审核）", en: "Pending (needs review)" })
                      : t({ zh: "已通过（自动通过）", en: "Approved (auto)" })}
                  </div>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-muted-foreground">{t({ zh: "可同步到 OSS", en: "Can sync to OSS" })}</div>
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

                  <div className="mt-2 space-y-2">
                    <div className="text-xs text-muted-foreground">{t({ zh: "选择测评对象（必填，三选一）", en: "Pick a real context (required, choose one)" })}</div>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        size="sm"
                        variant={evalSourceKind === "topic" ? "secondary" : "outline"}
                        className={evalSourceKind === "topic" ? toneClasses("slate").active : toneClasses("slate").inactive}
                        onClick={() => {
                          clearEvalSourceIds();
                          setEvalSourceKind("topic");
                        }}
                      >
                        {t({ zh: "广场话题", en: "Topic" })}
                      </Button>
                      <Button
                        size="sm"
                        variant={evalSourceKind === "work_item" ? "secondary" : "outline"}
                        className={evalSourceKind === "work_item" ? toneClasses("slate").active : toneClasses("slate").inactive}
                        onClick={() => {
                          clearEvalSourceIds();
                          setEvalSourceKind("work_item");
                        }}
                      >
                        {t({ zh: "任务", en: "Task" })}
                      </Button>
                      <Button
                        size="sm"
                        variant={evalSourceKind === "run" ? "secondary" : "outline"}
                        className={evalSourceKind === "run" ? toneClasses("slate").active : toneClasses("slate").inactive}
                        onClick={() => {
                          clearEvalSourceIds();
                          setEvalSourceKind("run");
                        }}
                      >
                        {t({ zh: "场景", en: "Scenario" })}
                      </Button>
                    </div>

                    {evalSourceKind === "topic" ? (
                      <div className="rounded-md border bg-background p-3">
                        <div className="text-xs text-muted-foreground">{t({ zh: "最近活跃的话题", en: "Recent topics" })}</div>
                        {evalTopicId.trim() ? (
                          <div className="mt-2 flex flex-wrap items-center justify-between gap-2 rounded-md border bg-muted/20 px-3 py-2">
                            <div className="min-w-0 text-sm">
                              <span className="text-muted-foreground">{t({ zh: "已选择：", en: "Selected: " })}</span>
                              <span className="font-medium">{evalSourceTitle || t({ zh: "（已选择话题）", en: "(selected topic)" })}</span>
                            </div>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => {
                                setEvalTopicId("");
                                setEvalSourceTitle("");
                              }}
                            >
                              {t({ zh: "更换", en: "Change" })}
                            </Button>
                          </div>
                        ) : (
                          <>
                            {recentTopicsLoading ? <div className="mt-2 text-xs text-muted-foreground">{t({ zh: "加载中…", en: "Loading…" })}</div> : null}
                            {recentTopicsError ? <div className="mt-2 text-xs text-destructive">{recentTopicsError}</div> : null}
                            {!recentTopicsLoading && !recentTopics.length ? (
                              <div className="mt-2 text-xs text-muted-foreground">{t({ zh: "暂无可用话题（或 OSS 未配置）", en: "No topics available (or OSS not configured)" })}</div>
                            ) : null}
                            <div className="mt-2 space-y-2">
                              {recentTopics.slice(0, 8).map((tp) => (
                                <div key={tp.topic_id} className="rounded-md border px-3 py-2">
                                  <div className="flex items-center justify-between gap-2">
                                    <div className="min-w-0">
                                      <div className="truncate text-sm font-medium">{String(tp.title ?? "").trim() || t({ zh: "（未命名话题）", en: "(untitled)" })}</div>
                                      {tp.summary ? <div className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{String(tp.summary).trim()}</div> : null}
                                      {tp.last_message_preview ? <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">{String(tp.last_message_preview).trim()}</div> : null}
                                    </div>
                                    <Button size="sm" variant="secondary" onClick={() => pickTopic(tp)}>
                                      {t({ zh: "选择", en: "Pick" })}
                                    </Button>
                                  </div>
                                </div>
                              ))}
                            </div>
                          </>
                        )}
                      </div>
                    ) : null}

                    {evalSourceKind === "run" ? (
                      <div className="rounded-md border bg-background p-3">
                        <div className="text-xs text-muted-foreground">{t({ zh: "最近场景（Run）", en: "Recent runs" })}</div>
                        {evalSourceRunId.trim() ? (
                          <div className="mt-2 flex flex-wrap items-center justify-between gap-2 rounded-md border bg-muted/20 px-3 py-2">
                            <div className="min-w-0 text-sm">
                              <span className="text-muted-foreground">{t({ zh: "已选择：", en: "Selected: " })}</span>
                              <span className="font-medium">{evalSourceTitle || t({ zh: "（已选择场景）", en: "(selected run)" })}</span>
                            </div>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => {
                                setEvalSourceRunId("");
                                setEvalSourceTitle("");
                              }}
                            >
                              {t({ zh: "更换", en: "Change" })}
                            </Button>
                          </div>
                        ) : (
                          <>
                            {recentRunsLoading ? <div className="mt-2 text-xs text-muted-foreground">{t({ zh: "加载中…", en: "Loading…" })}</div> : null}
                            {recentRunsError ? <div className="mt-2 text-xs text-destructive">{recentRunsError}</div> : null}
                            <div className="mt-2 space-y-2">
                              {recentRuns.slice(0, 8).map((it) => (
                                <div key={it.run_id} className="rounded-md border px-3 py-2">
                                  <div className="flex items-center justify-between gap-2">
                                    <div className="min-w-0">
                                      <div className="truncate text-sm font-medium">{String(it.run_goal ?? "").trim() || t({ zh: "（无标题）", en: "(untitled)" })}</div>
                                      <div className="mt-0.5 text-xs text-muted-foreground">{fmtTime(it.created_at)}</div>
                                    </div>
                                    <Button size="sm" variant="secondary" onClick={() => pickRunAsScenario(String(it.run_id), String(it.run_goal))}>
                                      {t({ zh: "选择", en: "Pick" })}
                                    </Button>
                                  </div>
                                </div>
                              ))}
                            </div>
                          </>
                        )}
                      </div>
                    ) : null}

                    {evalSourceKind === "work_item" ? (
                      <div className="rounded-md border bg-background p-3 space-y-2">
                        <div className="text-xs text-muted-foreground">{t({ zh: "先选一个 Run，再选其中一个任务", en: "Pick a run, then pick a work item" })}</div>

                        {workItemRunId ? (
                          <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border bg-muted/20 px-3 py-2">
                            <div className="min-w-0 text-sm">
                              <span className="text-muted-foreground">{t({ zh: "已选择：", en: "Selected: " })}</span>
                              <span className="font-medium">{workItemRunTitle || t({ zh: "（已选择场景）", en: "(selected run)" })}</span>
                            </div>
                            <div className="flex gap-2">
                              <Button
                                size="sm"
                                variant="secondary"
                                disabled={workItemRunLoading}
                                onClick={() => loadWorkItemsForRun(workItemRunId)}
                              >
                                {workItemRunLoading ? t({ zh: "加载中…", en: "Loading…" }) : t({ zh: "刷新任务", en: "Refresh" })}
                              </Button>
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => {
                                  setWorkItemRunId("");
                                  setWorkItemRunTitle("");
                                  setWorkItemRunItems([]);
                                  setWorkItemRunError("");
                                }}
                              >
                                {t({ zh: "更换", en: "Change" })}
                              </Button>
                            </div>
                          </div>
                        ) : (
                          <div className="space-y-2">
                            <div className="text-xs text-muted-foreground">{t({ zh: "从最近场景中选择一个（不会展示 UUID）", en: "Pick from recent runs (UUID hidden)" })}</div>
                            <div className="flex flex-wrap gap-2">
                              {recentRuns.slice(0, 8).map((it) => (
                                <Button
                                  key={it.run_id}
                                  size="sm"
                                  variant="outline"
                                  onClick={() => {
                                    const title = String(it.run_goal ?? "").trim() || t({ zh: "（无标题）", en: "(untitled)" });
                                    setWorkItemRunId(String(it.run_id));
                                    setWorkItemRunTitle(title);
                                    void loadWorkItemsForRun(String(it.run_id));
                                  }}
                                >
                                  {String(it.run_goal ?? "").trim().slice(0, 12) || t({ zh: "（无标题）", en: "(untitled)" })}
                                </Button>
                              ))}
                            </div>
                          </div>
                        )}
                        {workItemRunError ? <div className="text-xs text-destructive">{workItemRunError}</div> : null}
                        {evalWorkItemId.trim() ? (
                          <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border bg-muted/20 px-3 py-2">
                            <div className="min-w-0 text-sm">
                              <span className="text-muted-foreground">{t({ zh: "已选择：", en: "Selected: " })}</span>
                              <span className="font-medium">{evalSourceTitle || t({ zh: "（已选择任务）", en: "(selected task)" })}</span>
                            </div>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => {
                                setEvalWorkItemId("");
                                setEvalSourceTitle("");
                              }}
                            >
                              {t({ zh: "更换", en: "Change" })}
                            </Button>
                          </div>
                        ) : workItemRunItems.length ? (
                          <div className="space-y-2">
                            {workItemRunItems.slice(0, 10).map((wi) => (
                              <div key={wi.work_item_id} className="rounded-md border px-3 py-2">
                                <div className="flex items-center justify-between gap-2">
                                  <div className="min-w-0">
                                    <div className="truncate text-sm font-medium">
                                      {fmtWorkItemStage(wi.stage) || "-"} · {fmtWorkItemKind(wi.kind) || "-"}
                                    </div>
                                    {wi.stage_description ? <div className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{String(wi.stage_description).trim()}</div> : null}
                                    <div className="mt-0.5 text-xs text-muted-foreground">{t({ zh: "状态：", en: "Status: " }) + fmtWorkItemStatus(wi.status)}</div>
                                  </div>
                                  <Button size="sm" variant="secondary" onClick={() => pickWorkItem(wi)}>
                                    {t({ zh: "选择", en: "Pick" })}
                                  </Button>
                                </div>
                              </div>
                            ))}
                          </div>
                        ) : null}
                      </div>
                    ) : null}

                    <div className="rounded-md border bg-background p-3">
                      <div className="text-xs text-muted-foreground">{t({ zh: "测评重点（可选）", en: "Evaluation focus (optional)" })}</div>
                      <div className="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center">
                        <Input
                          value={evalTopic}
                          onChange={(e) => setEvalTopic(e.target.value)}
                          placeholder={t({ zh: "比如：是否跑题、是否冒充、是否输出不合规内容…", en: "e.g. stays on topic, no impersonation, compliance…" })}
                        />
                        <Button size="sm" onClick={createEvaluation} disabled={evalCreating || saving || loading || evalLoading}>
                          {evalCreating ? t({ zh: "发起中…", en: "Starting…" }) : t({ zh: "发起测评", en: "Start" })}
                        </Button>
                      </div>
                    </div>
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
                                {ev.source?.kind ? (
                                  <div className="mt-0.5 truncate text-xs text-muted-foreground">
                                    {t({ zh: "来源：", en: "Source: " })}
                                    {ev.source.kind === "topic"
                                      ? t({ zh: "话题", en: "Topic" })
                                      : ev.source.kind === "work_item"
                                        ? t({ zh: "任务", en: "Task" })
                                        : t({ zh: "场景", en: "Scenario" })}
                                    {ev.source.title ? ` · ${String(ev.source.title).trim()}` : ""}
                                  </div>
                                ) : null}
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
                                <Button size="sm" variant="outline" onClick={() => openSnapshot(ev)}>
                                  {t({ zh: "查看快照", en: "Snapshot" })}
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

      <AlertDialog
        open={Boolean(snapshotEval)}
        onOpenChange={(open) => {
          if (!open) {
            setSnapshotEval(null);
            setSnapshotData(null);
            setSnapshotError("");
            setSnapshotShowRaw(false);
          }
        }}
      >
        <AlertDialogContent className="max-w-3xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{t({ zh: "测评来源快照", en: "Source snapshot" })}</AlertDialogTitle>
            <AlertDialogDescription>
              {t({ zh: "这是平台注入到测评 context 的快照，用于确保测评智能体“确切知道”真实话题/任务。", en: "This is the snapshot injected into evaluation context." })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="max-h-[60vh] overflow-auto rounded-md border bg-background p-3 text-sm">
            {snapshotLoading ? <div className="text-xs text-muted-foreground">{t({ zh: "加载中…", en: "Loading…" })}</div> : null}
            {snapshotError ? <div className="text-xs text-destructive">{snapshotError}</div> : null}
            {!snapshotLoading && snapshotData?.source_snapshot ? (
              <div className="space-y-3">
                <div className="rounded-md border bg-muted/10 px-3 py-2">
                  <div className="text-xs text-muted-foreground">{t({ zh: "来源摘要", en: "Summary" })}</div>
                  <div className="mt-1 text-sm font-medium">
                    {(() => {
                      const snap: any = snapshotData.source_snapshot ?? {};
                      const kind = String(snap?.kind ?? "").trim();
                      if (kind === "topic") return String(snap?.topic?.title ?? "").trim() || t({ zh: "（未命名话题）", en: "(untitled topic)" });
                      return String(snap?.run?.title ?? "").trim() || t({ zh: "（无标题）", en: "(untitled)" });
                    })()}
                  </div>
                  {(() => {
                    const snap: any = snapshotData.source_snapshot ?? {};
                    const kind = String(snap?.kind ?? "").trim();
                    if (kind !== "topic") return null;
                    const summary = String(snap?.topic?.summary ?? "").trim();
                    const opening = String(snap?.topic?.opening ?? "").trim();
                    return (
                      <div className="mt-1 space-y-1 text-xs text-muted-foreground">
                        {summary ? <div>{summary}</div> : null}
                        {opening ? <div>{t({ zh: "开场：", en: "Opening: " }) + opening}</div> : null}
                      </div>
                    );
                  })()}
                </div>

                {Array.isArray((snapshotData.source_snapshot as any)?.recent_messages) && (snapshotData.source_snapshot as any).recent_messages.length ? (
                  <div className="rounded-md border bg-background px-3 py-2">
                    <div className="text-xs text-muted-foreground">{t({ zh: "最近动态（快照）", en: "Recent activity (snapshot)" })}</div>
                    <div className="mt-2 space-y-2">
                      {(snapshotData.source_snapshot as any).recent_messages.slice(0, 8).map((m: any, idx: number) => {
                        const preview = String(m?.preview ?? "").trim();
                        const at = String(m?.occurred_at ?? m?.created_at ?? "").trim();
                        const persona = String(m?.persona ?? "").trim();
                        return (
                          <div key={idx} className="rounded-md border px-2 py-1.5">
                            <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                              {at ? <span>{fmtTime(at)}</span> : null}
                              {persona && !isUuidLike(persona) ? <span>{persona}</span> : null}
                            </div>
                            {preview ? <div className="mt-1 text-xs">{preview}</div> : null}
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ) : null}

                <div>
                  <Button size="sm" variant="outline" onClick={() => setSnapshotShowRaw((v) => !v)}>
                    {snapshotShowRaw ? t({ zh: "收起原始JSON", en: "Hide raw JSON" }) : t({ zh: "高级：查看原始JSON（已脱敏）", en: "Advanced: raw JSON (redacted)" })}
                  </Button>
                  {snapshotShowRaw ? (
                    <pre className="mt-2 whitespace-pre-wrap break-words text-xs">{JSON.stringify(redactUuids(snapshotData.source_snapshot), null, 2)}</pre>
                  ) : null}
                </div>
              </div>
            ) : null}
            {!snapshotLoading && snapshotData && !snapshotData.source_snapshot ? (
              <div className="text-xs text-muted-foreground">{t({ zh: "暂无快照数据", en: "No snapshot" })}</div>
            ) : null}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>{t({ zh: "关闭", en: "Close" })}</AlertDialogCancel>
            {snapshotData?.run_id ? (
              <AlertDialogAction
                onClick={() => {
                  nav(`/runs/${encodeURIComponent(String(snapshotData.run_id))}`);
                  setSnapshotEval(null);
                }}
              >
                {t({ zh: "打开测评Run", en: "Open run" })}
              </AlertDialogAction>
            ) : null}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DialogContent>
  );
}
