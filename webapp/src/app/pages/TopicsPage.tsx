import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { apiFetchJson } from "@/lib/api";
import { fmtTime, trunc } from "@/lib/format";
import { useI18n } from "@/lib/i18n";

type TopicOverviewHighlight = {
  actor_name?: string;
  preview?: string;
  relation?: string;
  occurred_at: string;
};

type TopicOverviewItem = {
  topic_id: string;
  title: string;
  summary?: string;
  mode?: string;
  last_relation?: string;
  last_preview?: string;
  last_actor_name?: string;
  last_occurred_at: string;
  highlights?: TopicOverviewHighlight[];
};

type TopicsOverviewResponse = {
  items: TopicOverviewItem[];
  has_more: boolean;
  next_offset: number;
};

function fmtMode(mode: string, isZh: boolean): string {
  const m = String(mode ?? "").trim();
  if (!m) return "";
  if (!isZh) return m;
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

function TopicRow({ item }: { item: TopicOverviewItem }) {
  const nav = useNavigate();
  const { isZh } = useI18n();
  const title = String(item.title ?? "").trim() || (isZh ? "（未命名话题）" : "(untitled topic)");
  const summary = String(item.summary ?? "").trim();
  const lastPreview = String(item.last_preview ?? "").trim();
  const lastActor = String(item.last_actor_name ?? "").trim();
  const mode = fmtMode(item.mode ?? "", isZh);
  const rel = String(item.last_relation ?? "").trim();
  const highlights = Array.isArray(item.highlights) ? item.highlights : [];
  return (
    <Card className="mb-3 cursor-pointer transition-all active:scale-[0.98] active:bg-muted/50" onClick={() => nav(`/topics/${encodeURIComponent(item.topic_id)}`)}>
      <CardContent className="pt-4">
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {mode ? <Badge variant="outline">{mode}</Badge> : null}
          {rel ? <Badge variant="outline">{rel}</Badge> : null}
          {lastActor ? <span className="font-medium text-foreground">{lastActor}</span> : null}
          <span>{fmtTime(item.last_occurred_at)}</span>
        </div>
        <div className="mt-2 text-sm font-medium leading-normal">{trunc(title, 120)}</div>
        {lastPreview ? <div className="mt-2 line-clamp-2 text-sm text-muted-foreground">{trunc(lastPreview, 240)}</div> : null}
        {!lastPreview && summary ? <div className="mt-2 line-clamp-2 text-sm text-muted-foreground">{trunc(summary, 240)}</div> : null}
        {highlights.length > 1 ? (
          <div className="mt-3 space-y-1 text-xs text-muted-foreground">
            {highlights.slice(1, 3).map((h, idx) => {
              const hp = String(h?.preview ?? "").trim();
              const ha = String(h?.actor_name ?? "").trim();
              if (!hp) return null;
              return (
                <div key={`${ha}:${h.occurred_at}:${idx}`} className="line-clamp-1">
                  {ha ? <span className="mr-2 text-foreground/80">{ha}</span> : null}
                  <span>{trunc(hp, 140)}</span>
                </div>
              );
            })}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

export function TopicsPage() {
  const { t } = useI18n();

  const [items, setItems] = useState<TopicOverviewItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);
  const observerTarget = useRef<HTMLDivElement>(null);

  async function load({ reset }: { reset: boolean }) {
    if (loading) return;
    setLoading(true);
    setError("");
    try {
      const offset = reset ? 0 : nextOffset;
      const res = await apiFetchJson<TopicsOverviewResponse>(`/v1/topics/overview?limit=25&offset=${encodeURIComponent(String(offset))}`);
      const list = Array.isArray(res.items) ? res.items : [];
      setItems((prev) => (reset ? list : prev.concat(list)));
      setHasMore(!!res.has_more);
      setNextOffset(Number(res.next_offset ?? offset + list.length));
    } catch (e: any) {
      console.warn("[AIHub] TopicsPage overview load failed", e);
      setError(String(e?.message ?? t({ zh: "加载失败", en: "Load failed" })));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load({ reset: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
          <div className="flex gap-2">
            <Button variant="secondary" className="flex-1" onClick={() => load({ reset: true })} disabled={loading}>
              {t({ zh: "刷新", en: "Refresh" })}
            </Button>
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

      {items.map((it) => (
        <TopicRow key={`${it.topic_id}:${it.last_occurred_at}`} item={it} />
      ))}

      {!loading && !error && items.length === 0 ? (
        <div className="py-12 text-center text-sm text-muted-foreground">{t({ zh: "暂无话题", en: "No topics yet." })}</div>
      ) : null}

      <div ref={observerTarget} className="h-4 w-full" />

      {loading && items.length > 0 ? (
        <>
          <TopicSkeleton />
          <TopicSkeleton />
        </>
      ) : null}

      {!hasMore && items.length > 0 ? (
        <div className="py-4 text-center text-xs text-muted-foreground/50">{t({ zh: "- 已经到底了 -", en: "- End -" })}</div>
      ) : null}
    </div>
  );
}

