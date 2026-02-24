import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { fmtTime, trunc } from "@/lib/format";
import { getAdminToken } from "@/lib/storage";

type AgentRef = {
  agent_id: string;
  name: string;
  status: string;
};

type Lease = {
  agent: AgentRef;
  expires_at: string;
};

type WorkItem = {
  work_item_id: string;
  run_id: string;
  stage: string;
  kind: string;
  status: string;
  offers: AgentRef[];
  lease?: Lease;
  created_at: string;
  updated_at: string;
  run_goal: string;
  required_tags: string[];
};

type WorkItemsResponse = {
  items: WorkItem[];
  has_more: boolean;
  next_offset: number;
};

type Candidate = {
  agent_id: string;
  name: string;
  tags: string[];
  hits: number;
  matched_tags: string[];
  missing_tags: string[];
};

type CandidatesResponse = {
  work_item_id: string;
  run_id: string;
  required_tags: string[];
  matched: Candidate[];
  fallback: Candidate[];
};

export function AdminAssignPage() {
  const adminToken = getAdminToken();
  const { toast } = useToast();

  const [items, setItems] = useState<WorkItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [selected, setSelected] = useState<WorkItem | null>(null);
  const [candidates, setCandidates] = useState<CandidatesResponse | null>(null);
  const [candidatesLoading, setCandidatesLoading] = useState(false);
  const [assigning, setAssigning] = useState(false);

  const candidatesList = useMemo(() => {
    const list: Candidate[] = [];
    if (candidates?.matched?.length) list.push(...candidates.matched);
    if (candidates?.fallback?.length) list.push(...candidates.fallback);
    return list;
  }, [candidates]);

  async function load() {
    if (!adminToken) {
      setItems([]);
      setError("缺少管理员 Token，请先在「我的」里保存。");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const res = await apiFetchJson<WorkItemsResponse>("/v1/admin/work-items?limit=50&offset=0", {
        apiKey: adminToken,
      });
      setItems(res.items ?? []);
    } catch (e: any) {
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  async function openAssign(it: WorkItem) {
    if (!adminToken) return;
    setSelected(it);
    setCandidates(null);
    setCandidatesLoading(true);
    try {
      const res = await apiFetchJson<CandidatesResponse>(
        `/v1/admin/work-items/${encodeURIComponent(it.work_item_id)}/candidates?limit=50`,
        { apiKey: adminToken },
      );
      setCandidates(res);
    } catch (e: any) {
      toast({
        title: "加载候选人失败",
        description: String(e?.message ?? "请稍后再试"),
        variant: "destructive",
      });
    } finally {
      setCandidatesLoading(false);
    }
  }

  async function assignTo(it: WorkItem, candidate: Candidate) {
    if (!adminToken) return;
    const ok = window.confirm(`确定将任务指派给「${candidate.name || "未命名"}」吗？`);
    if (!ok) return;

    setAssigning(true);
    try {
      await apiFetchJson(`/v1/admin/work-items/${encodeURIComponent(it.work_item_id)}/assign`, {
        method: "POST",
        apiKey: adminToken,
        body: { agent_ids: [candidate.agent_id], mode: "add", reason: "" },
      });
      toast({ title: "已指派" });
      setSelected(null);
      setCandidates(null);
      load();
    } catch (e: any) {
      toast({
        title: "指派失败",
        description: String(e?.message ?? "请稍后再试"),
        variant: "destructive",
      });
    } finally {
      setAssigning(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [adminToken]);

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">任务指派</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Button variant="secondary" className="w-full" disabled={loading} onClick={load}>
            {loading ? "加载中…" : "刷新"}
          </Button>
          {error ? <div className="text-sm text-destructive">{error}</div> : null}
          {!loading && !error && !items.length ? (
            <div className="text-sm text-muted-foreground">暂无待处理任务。</div>
          ) : null}
        </CardContent>
      </Card>

      {items.map((it) => (
        <Card key={it.work_item_id}>
          <CardContent className="pt-4">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <Badge variant="secondary">{it.status}</Badge>
              <Badge variant="outline">{it.stage}</Badge>
              <Badge variant="outline">{it.kind}</Badge>
              <span>{fmtTime(it.created_at)}</span>
            </div>
            <div className="mt-2 text-sm">{trunc(it.run_goal || "（无摘要）", 240)}</div>
            <div className="mt-3">
              <Button size="sm" onClick={() => openAssign(it)} disabled={!adminToken}>
                查看候选人 / 指派
              </Button>
            </div>
          </CardContent>
        </Card>
      ))}

      <Dialog
        open={Boolean(selected)}
        onOpenChange={(open) => {
          if (!open) {
            setSelected(null);
            setCandidates(null);
          }
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>指派候选人</DialogTitle>
          </DialogHeader>
          <div className="space-y-2 text-sm">
            {selected?.required_tags?.length ? (
              <div className="flex flex-wrap gap-1">
                {selected.required_tags.slice(0, 10).map((t) => (
                  <Badge key={t} variant="secondary">
                    {t}
                  </Badge>
                ))}
              </div>
            ) : (
              <div className="text-xs text-muted-foreground">该任务未设置标签。</div>
            )}

            {candidatesLoading ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
            {!candidatesLoading && selected && !candidatesList.length ? (
              <div className="text-sm text-muted-foreground">暂无候选人。</div>
            ) : null}

            <div className="space-y-2">
              {candidatesList.map((c) => (
                <Card key={c.agent_id}>
                  <CardContent className="pt-4">
                    <div className="flex items-center justify-between gap-2">
                      <div className="min-w-0">
                        <div className="truncate text-sm font-semibold">{c.name || "未命名"}</div>
                        <div className="mt-1 text-xs text-muted-foreground">
                          命中标签：{c.hits ?? 0}
                        </div>
                      </div>
                      <Button
                        size="sm"
                        disabled={!selected || assigning}
                        onClick={() => selected && assignTo(selected, c)}
                      >
                        指派
                      </Button>
                    </div>
                    {c.matched_tags?.length ? (
                      <div className="mt-3 flex flex-wrap gap-1">
                        {c.matched_tags.slice(0, 8).map((t) => (
                          <Badge key={t} variant="secondary">
                            {t}
                          </Badge>
                        ))}
                      </div>
                    ) : null}
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
