import { useEffect, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { fmtTime, trunc } from "@/lib/format";
import { getAdminToken } from "@/lib/storage";

type QueueItem = {
  target_type: string;
  id: string;
  created_at: string;
  summary: string;
};

type ModerationQueueResponse = {
  items: QueueItem[];
};

export function AdminModerationPage() {
  const adminToken = getAdminToken();
  const { toast } = useToast();

  const [items, setItems] = useState<QueueItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    if (!adminToken) {
      setItems([]);
      setError("缺少管理员 Token，请先在「我的」里保存。");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const res = await apiFetchJson<ModerationQueueResponse>("/v1/admin/moderation/queue", {
        apiKey: adminToken,
      });
      setItems(res.items ?? []);
    } catch (e: any) {
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [adminToken]);

  async function decide(it: QueueItem, action: "approve" | "reject") {
    if (!adminToken) return;
    const ok = window.confirm(action === "approve" ? "确定通过审核吗？" : "确定驳回吗？");
    if (!ok) return;

    try {
      await apiFetchJson(
        `/v1/admin/moderation/${encodeURIComponent(it.target_type)}/${encodeURIComponent(it.id)}/${action}`,
        { method: "POST", apiKey: adminToken },
      );
      toast({ title: action === "approve" ? "已通过" : "已驳回" });
      load();
    } catch (e: any) {
      toast({
        title: "操作失败",
        description: String(e?.message ?? "请稍后再试"),
        variant: "destructive",
      });
    }
  }

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">内容审核队列</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Button variant="secondary" className="w-full" disabled={loading} onClick={load}>
            {loading ? "加载中…" : "刷新"}
          </Button>
          {error ? <div className="text-sm text-destructive">{error}</div> : null}
          {!loading && !error && !items.length ? (
            <div className="text-sm text-muted-foreground">队列为空。</div>
          ) : null}
        </CardContent>
      </Card>

      {items.map((it) => (
        <Card key={`${it.target_type}:${it.id}`}>
          <CardContent className="pt-4">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <Badge variant="secondary">{it.target_type}</Badge>
              <span>{fmtTime(it.created_at)}</span>
            </div>
            <div className="mt-2 text-sm">{trunc(it.summary || "（无摘要）", 240)}</div>
            <div className="mt-3 flex gap-2">
              <Button size="sm" onClick={() => decide(it, "approve")}>
                通过
              </Button>
              <Button size="sm" variant="secondary" onClick={() => decide(it, "reject")}>
                驳回
              </Button>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
