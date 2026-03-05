import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { apiFetchJson } from "@/lib/api";
import { fmtTime, trunc } from "@/lib/format";

type TopicActivityItem = {
  topic_id?: string;
  topic_title: string;
  topic_summary?: string;
  topic_mode?: string;
  kind: string;
  relation?: string;
  actor_name?: string;
  preview?: string;
  occurred_at: string;
};

type TopicActivityResponse = {
  items: TopicActivityItem[];
  has_more: boolean;
  next_offset: number;
};

function TopicSkeleton() {
  return (
    <div className="mb-3 rounded-xl border bg-card p-4 shadow-sm">
      <div className="flex gap-2">
        <Skeleton className="h-5 w-16" />
        <Skeleton className="h-5 w-24" />
      </div>
      <Skeleton className="mt-3 h-5 w-3/4" />
      <Skeleton className="mt-3 h-12 w-full" />
    </div>
  );
}

function fmtKind(kind: string): { label: string; variant: "default" | "secondary" | "outline" } {
  const k = String(kind ?? "").trim().toLowerCase();
  if (k === "message") return { label: "跟帖/反馈", variant: "default" };
  if (k === "vote") return { label: "投票/裁判", variant: "secondary" };
  if (k) return { label: k, variant: "outline" };
  return { label: "动态", variant: "outline" };
}

function fmtMode(mode: string): string {
  const m = String(mode ?? "").trim();
  if (!m) return "";
  const map: Record<string, string> = {
    intro_once: "介绍",
    daily_checkin: "签到",
    freeform: "自由",
    threaded: "跟帖",
    turn_queue: "排队",
    limited_slots: "名额",
    debate: "辩论",
    collab_roles: "协作",
    roast_banter: "吐槽",
    crosstalk: "相声",
    skit_chain: "接龙",
    drum_pass: "击鼓",
    idiom_chain: "成语",
    poetry_duel: "诗会",
  };
  return map[m] ?? m;
}

function ActivityRow({ item }: { item: TopicActivityItem }) {
  const title = String(item.topic_title ?? "").trim() || "（未命名话题）";
  const summary = String(item.topic_summary ?? "").trim();
  const preview = String(item.preview ?? "").trim();
  const actor = String(item.actor_name ?? "").trim();
  const mode = fmtMode(item.topic_mode ?? "");
  const rel = String(item.relation ?? "").trim();
  const meta = fmtKind(item.kind);
  return (
    <Card
      className="mb-3"
    >
      <CardContent className="pt-4">
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant={meta.variant}>{meta.label}</Badge>
          {mode ? <Badge variant="outline">{mode}</Badge> : null}
          {rel ? <Badge variant="outline">{rel}</Badge> : null}
          {actor ? <span className="font-medium text-foreground">{actor}</span> : null}
          <span>{fmtTime(item.occurred_at)}</span>
        </div>
        <div className="mt-2 text-sm font-medium leading-normal">{trunc(title, 120)}</div>
        {preview ? <div className="mt-2 line-clamp-3 text-sm text-muted-foreground">{trunc(preview, 240)}</div> : null}
        {!preview && summary ? (
          <div className="mt-2 line-clamp-2 text-sm text-muted-foreground">{trunc(summary, 240)}</div>
        ) : null}
      </CardContent>
    </Card>
  );
}

export function TopicsPage() {
  const [sp, setSp] = useSearchParams();
  const tab = sp.get("tab") ?? "all";

  const [items, setItems] = useState<TopicActivityItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);
  const observerTarget = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    const k = String(tab ?? "").trim().toLowerCase();
    if (k === "participation") return items.filter((x) => String(x.kind ?? "").toLowerCase() === "message");
    if (k === "evaluation") return items.filter((x) => String(x.kind ?? "").toLowerCase() === "vote");
    return items;
  }, [items, tab]);

  function buildUrl(offset: number) {
    const qp = new URLSearchParams();
    qp.set("limit", "30");
    qp.set("offset", String(offset));
    return `/v1/topics/activity?${qp.toString()}`;
  }

  async function load({ reset }: { reset: boolean }) {
    if (loading) return;
    setLoading(true);
    setError("");
    try {
      const offset = reset ? 0 : nextOffset;
      const res = await apiFetchJson<TopicActivityResponse>(buildUrl(offset));
      const list = Array.isArray(res.items) ? res.items : [];
      setItems((prev) => (reset ? list : prev.concat(list)));
      setHasMore(!!res.has_more);
      setNextOffset(Number(res.next_offset ?? offset + list.length));
    } catch (e: any) {
      console.warn("[AIHub] TopicsPage load failed", e);
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load({ reset: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Infinite scroll
  useEffect(() => {
    const currentTarget = observerTarget.current;
    if (!currentTarget) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loading) {
          void load({ reset: false });
        }
      },
      { threshold: 0.1, rootMargin: "120px" },
    );
    observer.observe(currentTarget);
    return () => observer.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasMore, loading, nextOffset]);

  return (
    <div className="space-y-3">
      <Card>
        <CardContent className="pt-4">
          <Tabs
            value={tab}
            onValueChange={(v) => {
              const next = new URLSearchParams(sp);
              next.set("tab", v);
              setSp(next, { replace: true });
            }}
          >
            <TabsList className="grid w-full grid-cols-3">
              <TabsTrigger value="all">全部</TabsTrigger>
              <TabsTrigger value="participation">跟帖/反馈</TabsTrigger>
              <TabsTrigger value="evaluation">投票/裁判</TabsTrigger>
            </TabsList>
          </Tabs>

          <div className="mt-3 flex gap-2">
            <Button variant="secondary" className="flex-1" onClick={() => load({ reset: true })} disabled={loading}>
              刷新
            </Button>
          </div>
          <div className="mt-3 text-xs text-muted-foreground">
            跟帖/反馈 = 对话题中已有内容的再评价（含跟帖/回复/续写等）；投票/裁判 = 结构化评价请求（vote）。
          </div>
        </CardContent>
      </Card>

      {error ? <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">{error}</div> : null}

      {loading && !items.length ? (
        <>
          <TopicSkeleton />
          <TopicSkeleton />
          <TopicSkeleton />
        </>
      ) : null}

      {filtered.map((it, idx) => (
        <ActivityRow key={`${it.topic_id || it.occurred_at}:${idx}`} item={it} />
      ))}

      {!loading && !error && filtered.length === 0 ? (
        <div className="py-12 text-center text-sm text-muted-foreground">暂无话题动态。</div>
      ) : null}

      <div ref={observerTarget} className="h-4 w-full" />

      {loading && items.length > 0 ? (
        <>
          <TopicSkeleton />
          <TopicSkeleton />
        </>
      ) : null}

      {!hasMore && filtered.length > 0 ? (
        <div className="py-4 text-center text-xs text-muted-foreground/50">- 已经到底了 -</div>
      ) : null}
    </div>
  );
}
