import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { fmtArtifactKind, fmtEventKind, fmtRunStatus, fmtTime, trunc } from "@/lib/format";
import { getUserApiKey } from "@/lib/storage";

type QueueItem = {
  target_type: string;
  id: string;
  run_id?: string;
  seq?: number;
  version?: number;
  kind?: string;
  persona?: string;
  review_status: string;
  created_at: string;
  summary: string;
};

type ModerationQueueResponse = {
  items: QueueItem[];
  has_more: boolean;
  next_offset: number;
};

type ModerationAction = {
  action: string;
  reason: string;
  actor_type: string;
  actor_id: string;
  created_at: string;
};

type ModerationGetResponse = {
  target_type: string;
  target_id: string;
  detail: any;
  actions: ModerationAction[];
};

function fmtReviewStatus(status: string): string {
  const v = String(status ?? "").trim().toLowerCase();
  switch (v) {
    case "pending":
      return "待审核";
    case "approved":
      return "已通过";
    case "rejected":
      return "已拒绝";
    default:
      return status || "";
  }
}

function fmtTargetType(t: string): string {
  const v = String(t ?? "").trim().toLowerCase();
  switch (v) {
    case "run":
      return "任务";
    case "event":
      return "事件";
    case "artifact":
      return "作品";
    default:
      return t || "";
  }
}

function fmtModerationAction(action: string): string {
  const v = String(action ?? "").trim().toLowerCase();
  switch (v) {
    case "approve":
      return "通过";
    case "reject":
      return "拒绝";
    case "unreject":
      return "撤销拒绝";
    default:
      return action || "";
  }
}

function fmtKind(it: Pick<QueueItem, "target_type" | "kind">): string {
  const kind = String(it?.kind ?? "").trim();
  if (!kind) return "";
  const t = String(it?.target_type ?? "").trim().toLowerCase();
  if (t === "event") return fmtEventKind(kind);
  if (t === "artifact") return fmtArtifactKind(kind);
  return kind;
}

function truncLine(text: string, n: number): string {
  const s = String(text ?? "").trim().replaceAll("\r", "");
  if (!s) return "";
  const single = s.split("\n")[0];
  return trunc(single, n);
}

function describePayload(payload: any): string {
  if (!payload || typeof payload !== "object") return "";
  const text = String(payload?.text ?? "").trim();
  if (text) return text;
  const keys = Object.keys(payload);
  if (!keys.length) return "（无可展示内容）";
  return `（结构化内容）字段：${keys.slice(0, 20).join("，")}${keys.length > 20 ? "…" : ""}`;
}

function formatDetail(res: ModerationGetResponse | null): string {
  if (!res) return "";
  const targetType = String(res?.target_type ?? "").trim().toLowerCase();
  const detail = res?.detail ?? {};
  const actions = Array.isArray(res?.actions) ? res.actions : [];

  const lines: string[] = [];
  if (targetType === "run") {
    lines.push("【任务】");
    const goal = String(detail?.goal ?? "").trim();
    const constraints = String(detail?.constraints ?? "").trim();
    if (goal) lines.push("\n目标：\n" + goal);
    if (constraints) lines.push("\n约束：\n" + constraints);
    const tags = Array.isArray(detail?.required_tags) ? detail.required_tags.filter(Boolean) : [];
    lines.push(
      "\n状态：" +
        fmtRunStatus(String(detail?.status ?? "")) +
        " · 审核：" +
        fmtReviewStatus(String(detail?.review_status ?? "")),
    );
    lines.push("标签：" + (tags.length ? tags.join("，") : "无"));
    lines.push("创建时间：" + fmtTime(String(detail?.created_at ?? "")));
    lines.push("更新时间：" + fmtTime(String(detail?.updated_at ?? "")));
  } else if (targetType === "event") {
    lines.push("【事件】");
    lines.push("类型：" + fmtEventKind(String(detail?.kind ?? "")));
    const persona = String(detail?.persona ?? "").trim();
    if (persona) lines.push("角色：" + persona);
    lines.push("时间：" + fmtTime(String(detail?.created_at ?? "")));
    const p = describePayload(detail?.payload);
    if (p) lines.push("\n内容：\n" + p);
  } else if (targetType === "artifact") {
    lines.push("【作品】");
    const kind = fmtArtifactKind(String(detail?.kind ?? ""));
    const version = detail?.version ?? "-";
    lines.push(`类型：${kind} · 版本：${version}`);
    lines.push("时间：" + fmtTime(String(detail?.created_at ?? "")));
    const content = String(detail?.content ?? "").trim();
    if (content) lines.push("\n内容：\n" + content);
  } else {
    lines.push("【未知类型】");
  }

  if (actions.length) {
    lines.push("\n---\n审核记录（最近在前）：");
    for (const a of actions) {
      const reason = String(a?.reason ?? "").trim();
      const who = String(a?.actor_type ?? "").trim();
      lines.push(
        `- ${fmtTime(String(a?.created_at ?? ""))} · ${fmtModerationAction(String(a?.action ?? "-"))}${
          who ? ` · ${who}` : ""
        }${reason ? ` · 原因：${truncLine(reason, 120)}` : ""}`,
      );
    }
  }

  return lines.join("\n");
}

export function AdminModerationPage() {
  const userApiKey = getUserApiKey();
  const { toast } = useToast();

  const limit = 30;
  const [status, setStatus] = useState<"pending" | "rejected" | "approved">("pending");
  const [typeRun, setTypeRun] = useState(true);
  const [typeEvent, setTypeEvent] = useState(true);
  const [typeArtifact, setTypeArtifact] = useState(true);
  const [q, setQ] = useState("");

  const [items, setItems] = useState<QueueItem[]>([]);
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");

  const filteredItems = useMemo(() => {
    const v = String(q ?? "").trim();
    if (!v) return items;
    return items.filter((it) => String(it?.summary ?? "").includes(v));
  }, [items, q]);

  const typesParam = useMemo(() => {
    const types: string[] = [];
    if (typeRun) types.push("run");
    if (typeEvent) types.push("event");
    if (typeArtifact) types.push("artifact");
    return types.join(",");
  }, [typeArtifact, typeEvent, typeRun]);

  async function loadQueue(opts: { reset: boolean }) {
    if (!userApiKey) {
      setItems([]);
      setError("未登录，请先在「管理员」里登录。");
      return;
    }
    if (!typesParam) {
      setItems([]);
      setOffset(0);
      setHasMore(false);
      setError("请至少选择一种类型。");
      return;
    }

    const nextOffset = opts.reset ? 0 : offset;
    if (opts.reset) {
      setLoading(true);
    } else {
      setLoadingMore(true);
    }

    setError("");
    try {
      const url =
        `/v1/admin/moderation/queue?status=${encodeURIComponent(status)}` +
        `&types=${encodeURIComponent(typesParam)}` +
        `&limit=${encodeURIComponent(String(limit))}&offset=${encodeURIComponent(String(nextOffset))}`;
      const res = await apiFetchJson<ModerationQueueResponse>(url, { apiKey: userApiKey });
      const newItems = res.items ?? [];
      setItems((prev) => (opts.reset ? newItems : prev.concat(newItems)));
      setHasMore(Boolean(res.has_more));
      setOffset(Number(res.next_offset ?? nextOffset + newItems.length));
    } catch (e: any) {
      console.warn("[AIHub] AdminModerationPage loadQueue failed", e);
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  }

  useEffect(() => {
    loadQueue({ reset: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [userApiKey, status, typesParam]);

  const [selected, setSelected] = useState<QueueItem | null>(null);
  const [detail, setDetail] = useState<ModerationGetResponse | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [reason, setReason] = useState("");
  const [acting, setActing] = useState(false);
  const [actError, setActError] = useState("");

  function closeDialog() {
    setSelected(null);
    setDetail(null);
    setDetailError("");
    setReason("");
    setActError("");
  }

  async function loadDetail(targetType: string, id: string) {
    if (!userApiKey) return;
    setDetailLoading(true);
    setDetailError("");
    try {
      const res = await apiFetchJson<ModerationGetResponse>(
        `/v1/admin/moderation/${encodeURIComponent(targetType)}/${encodeURIComponent(id)}`,
        { apiKey: userApiKey },
      );
      setDetail(res);
    } catch (e: any) {
      console.warn("[AIHub] AdminModerationPage loadDetail failed", { targetType, id, error: e });
      setDetail(null);
      setDetailError(String(e?.message ?? "加载失败"));
    } finally {
      setDetailLoading(false);
    }
  }

  function open(it: QueueItem) {
    setSelected(it);
    setDetail(null);
    setReason("");
    setActError("");
    void loadDetail(it.target_type, it.id);
  }

  async function act(action: "approve" | "reject" | "unreject") {
    if (!userApiKey || !selected) return;

    const targetType = selected.target_type;
    const targetId = selected.id;

    setActing(true);
    setActError("");
    try {
      await apiFetchJson(
        `/v1/admin/moderation/${encodeURIComponent(targetType)}/${encodeURIComponent(targetId)}/${action}`,
        {
          method: "POST",
          apiKey: userApiKey,
          body: { reason: reason.trim() },
        },
      );
      toast({ title: "操作成功" });
      closeDialog();
      void loadQueue({ reset: true });
    } catch (e: any) {
      console.warn("[AIHub] AdminModerationPage act failed", { action, selected, error: e });
      const msg = String(e?.message ?? "操作失败");
      setActError(msg);
    } finally {
      setActing(false);
    }
  }

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">内容审核队列</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1">
              <div className="text-xs text-muted-foreground">状态</div>
              <select
                value={status}
                onChange={(e) => setStatus(e.target.value as any)}
                className="h-9 w-full rounded-md border bg-background px-3 text-sm"
              >
                <option value="pending">待审核（默认可见）</option>
                <option value="rejected">已拒绝（不展示）</option>
                <option value="approved">已通过（可展示）</option>
              </select>
            </div>
            <div className="space-y-1">
              <div className="text-xs text-muted-foreground">本页过滤（仅前端）</div>
              <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="按摘要过滤，例如：涉政/广告" />
            </div>
          </div>

          <div className="space-y-1">
            <div className="text-xs text-muted-foreground">类型</div>
            <div className="flex flex-wrap gap-3 text-sm">
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={typeRun} onChange={(e) => setTypeRun(e.target.checked)} />
                任务
              </label>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={typeEvent} onChange={(e) => setTypeEvent(e.target.checked)} />
                事件
              </label>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={typeArtifact} onChange={(e) => setTypeArtifact(e.target.checked)} />
                作品
              </label>
            </div>
          </div>

          <div className="flex gap-2">
            <Button variant="secondary" className="flex-1" disabled={loading} onClick={() => loadQueue({ reset: true })}>
              {loading ? "加载中…" : "刷新"}
            </Button>
            <Button
              variant="secondary"
              className="flex-1"
              disabled={loadingMore || !hasMore}
              onClick={() => loadQueue({ reset: false })}
            >
              {loadingMore ? "加载中…" : "加载更多"}
            </Button>
          </div>

          {error ? <div className="text-sm text-destructive">{error}</div> : null}
          {!loading && !error && !filteredItems.length ? (
            <div className="text-sm text-muted-foreground">队列为空。</div>
          ) : null}
          {!error ? (
            <div className="text-xs text-muted-foreground">
              已加载 {items.length} 条（当前显示 {filteredItems.length} 条）
            </div>
          ) : null}
        </CardContent>
      </Card>

      {filteredItems.map((it) => (
        <Card key={`${it.target_type}:${it.id}`}>
          <CardContent className="pt-4">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <Badge variant="secondary">{fmtTargetType(it.target_type)}</Badge>
              <Badge variant="outline">{fmtReviewStatus(it.review_status)}</Badge>
              <span>{fmtTime(it.created_at)}</span>
              {it.kind ? <Badge variant="outline">{fmtKind(it)}</Badge> : null}
              {it.persona ? <Badge variant="outline">{it.persona}</Badge> : null}
            </div>
            <div className="mt-2 text-sm">{trunc(it.summary || "（无摘要）", 240)}</div>
            <div className="mt-3 flex gap-2">
              <Button size="sm" variant="secondary" onClick={() => open(it)}>
                查看/操作
              </Button>
            </div>
          </CardContent>
        </Card>
      ))}

      <Dialog
        open={Boolean(selected)}
        onOpenChange={(open) => {
          if (!open) {
            closeDialog();
          }
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>详情与操作</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            {selected ? (
              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                <Badge variant="secondary">{fmtTargetType(selected.target_type)}</Badge>
                <span>{fmtTime(selected.created_at)}</span>
              </div>
            ) : null}

            {detailLoading ? <div className="text-sm text-muted-foreground">加载详情中…</div> : null}
            {detailError ? <div className="text-sm text-destructive">{detailError}</div> : null}
            {detail ? (
              <pre className="max-h-[45vh] overflow-auto whitespace-pre-wrap rounded-md border bg-muted px-3 py-2 font-mono text-xs leading-relaxed">
                {formatDetail(detail)}
              </pre>
            ) : null}

            <div className="space-y-1 pt-1">
              <div className="text-xs text-muted-foreground">原因（仅管理员可见）</div>
              <Textarea
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder="例如：包含个人隐私/广告/不实信息"
              />
            </div>

            {actError ? <div className="text-sm text-destructive">{actError}</div> : null}

            <div className="flex gap-2 pt-1">
              <Button className="flex-1" disabled={acting} onClick={() => act("approve")}>
                通过
              </Button>
              <Button className="flex-1" variant="destructive" disabled={acting} onClick={() => act("reject")}>
                拒绝
              </Button>
            </div>
            <Button variant="secondary" className="w-full" disabled={acting} onClick={() => act("unreject")}>
              撤销拒绝（改为通过）
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
