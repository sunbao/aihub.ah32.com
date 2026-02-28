import { useEffect, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { fmtTime } from "@/lib/format";
import { useI18n } from "@/lib/i18n";
import { getUserApiKey } from "@/lib/storage";

type CurationEntry = {
  kind: string;
  schema_version: number;
  curation_id: string;
  review_status: string;
  owner_id: string;
  reason: string;
  refs?: Record<string, any>;
  created_at: string;
  updated_at: string;
};

export function CurationPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const userApiKey = getUserApiKey();
  const isLoggedIn = !!userApiKey;

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

  const [items, setItems] = useState<CurationEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [reason, setReason] = useState("");
  const [posting, setPosting] = useState(false);
  const [postDialogOpen, setPostDialogOpen] = useState(false);
  const [postError, setPostError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      const res = await apiFetchJson<{ items: CurationEntry[] }>("/v1/curations?limit=30");
      setItems(res.items ?? []);
    } catch (e: any) {
      console.warn("[AIHub] CurationPage load failed", e);
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function post() {
    const text = reason.trim();
    setPostError("");
    if (!text) {
      setPostError("请输入策展理由");
      toast({ title: "请输入策展理由", variant: "destructive" });
      return;
    }
    setPosting(true);
    try {
      await apiFetchJson("/v1/curations", { method: "POST", apiKey: userApiKey, body: { reason: text } });
      toast({ title: "已提交（待审核）" });
      setReason("");
      setPostDialogOpen(false);
    } catch (e: any) {
      console.warn("[AIHub] CurationPage post failed", e);
      setPostError(String(e?.message ?? "提交失败"));
      toast({ title: "提交失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setPosting(false);
      load();
    }
  }

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">策展广场</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="text-sm text-muted-foreground">展示园丁们推荐的精彩瞬间（仅展示已审核通过）。</div>
          {isLoggedIn ? (
            <Dialog
              open={postDialogOpen}
              onOpenChange={(open) => {
                setPostDialogOpen(open);
                setPostError("");
              }}
            >
              <DialogTrigger asChild>
                <Button size="sm" disabled={posting}>
                  发布策展
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>发布策展（需审核）</DialogTitle>
                </DialogHeader>
                <div className="space-y-2">
                  <div className="text-xs text-muted-foreground">一句话说清楚你为什么推荐</div>
                  <Textarea value={reason} onChange={(e) => setReason(e.target.value)} rows={4} />
                  {postError ? <div className="text-sm text-destructive">{postError}</div> : null}
                </div>
                <DialogFooter>
                  <Button onClick={post} disabled={posting}>
                    {posting ? "提交中…" : "提交"}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          ) : (
            <div className="text-xs text-muted-foreground">登录后可发布策展。</div>
          )}
        </CardContent>
      </Card>

      {loading ? (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <div key={i} className="rounded-xl border bg-card p-4 shadow-sm space-y-2">
              <div className="flex gap-2">
                <Skeleton className="h-5 w-16" />
                <Skeleton className="h-5 w-24" />
              </div>
              <Skeleton className="h-12 w-full" />
            </div>
          ))}
        </div>
      ) : null}
      {error ? <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">{error}</div> : null}

      {!loading && !error ? (
        items.length ? (
          items.map((it) => (
            <Card key={it.curation_id}>
              <CardContent className="pt-4 space-y-2">
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant="secondary">{fmtReviewStatus(it.review_status)}</Badge>
                  <span>{fmtTime(it.created_at)}</span>
                </div>
                <div className="text-sm leading-relaxed">{it.reason}</div>
              </CardContent>
            </Card>
          ))
        ) : (
          <div className="text-sm text-muted-foreground">暂无内容。</div>
        )
      ) : null}
    </div>
  );
}
