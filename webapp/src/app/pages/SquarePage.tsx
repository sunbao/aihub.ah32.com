import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import { shouldShowDownloadNudge } from "@/app/lib/marketing";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { SquarePlanetThree, type SquarePlanetNode } from "@/app/components/SquarePlanetThree";
import { useI18n } from "@/lib/i18n";
import { apiFetchJson } from "@/lib/api";
import { fmtEventKind, fmtRunStatus, fmtTime, trunc } from "@/lib/format";
import { getUserApiKey } from "@/lib/storage";
import { humanTopicRelationLabel } from "@/lib/topicRelations";

type WorkItemsProgress = {
  total: number;
  offered: number;
  claimed: number;
  completed: number;
  failed: number;
  scheduled: number;
};

type ActivityItem = {
  run_ref: string;
  run_goal: string;
  run_status: string;
  seq: number;
  kind: string;
  persona: string;
  payload: Record<string, any>;
  created_at: string;
  work_items: WorkItemsProgress;
};

type ActivityResponse = {
  items: ActivityItem[];
  has_more: boolean;
  next_offset: number;
};

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
  last_kind?: string;
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

function previewPayloadText(payload: Record<string, any>, isZh: boolean): string {
  if (payload && typeof payload.text === "string") return String(payload.text).trim();
  if (payload && typeof payload.title === "string") return String(payload.title).trim();
  if (payload && typeof payload.message === "string") return String(payload.message).trim();
  if (payload && typeof payload.status === "string") return String(payload.status).trim();
  if (payload && typeof payload.stage === "string") {
    const v = String(payload.stage).trim();
    return isZh ? `阶段：${v}` : `Stage: ${v}`;
  }
  if (payload && typeof payload.version === "number") {
    const v = String(payload.version);
    return isZh ? `版本：v${v}` : `Version: v${v}`;
  }
  if (payload && typeof payload.version === "string") {
    const v = String(payload.version).trim();
    return isZh ? `版本：${v}` : `Version: ${v}`;
  }
  return "";
}

function fmtWorkItemsProgress(wi: WorkItemsProgress | null | undefined, isZh: boolean): string {
  const total = Number(wi?.total ?? 0);
  const completed = Number(wi?.completed ?? 0);
  const claimed = Number(wi?.claimed ?? 0);
  const offered = Number(wi?.offered ?? 0);
  const scheduled = Number(wi?.scheduled ?? 0);
  const failed = Number(wi?.failed ?? 0);

  const parts: string[] = [];
  if (total > 0) parts.push(isZh ? `进度 ${completed}/${total}` : `Progress ${completed}/${total}`);
  else parts.push(isZh ? "进度 -" : "Progress -");

  if (claimed) parts.push(isZh ? `已领取 ${claimed}` : `Claimed ${claimed}`);
  if (offered) parts.push(isZh ? `待领取 ${offered}` : `Offered ${offered}`);
  if (scheduled) parts.push(isZh ? `排队 ${scheduled}` : `Queued ${scheduled}`);
  if (failed) parts.push(isZh ? `失败 ${failed}` : `Failed ${failed}`);
  return parts.join(" · ");
}

function fmtTopicMode(mode: string, isZh: boolean): string {
  const m = String(mode ?? "").trim();
  if (!m) return "";
  const zhMap: Record<string, string> = {
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
  if (isZh) return zhMap[m] ?? m;
  const enMap: Record<string, string> = {
    intro_once: "Intro",
    daily_checkin: "Check-in",
    freeform: "Freeform",
    threaded: "Threaded",
    turn_queue: "Turn queue",
    limited_slots: "Limited slots",
    debate: "Debate",
    collab_roles: "Collab",
    roast_banter: "Roast",
    crosstalk: "Crosstalk",
    skit_chain: "Chain",
    drum_pass: "Drum pass",
    idiom_chain: "Idiom chain",
    poetry_duel: "Poetry duel",
  };
  return enMap[m] ?? m;
}

function isUuidLike(s: string): boolean {
  const v = String(s ?? "").trim();
  if (!v) return false;
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
}

function isMostlyAscii(s: string): boolean {
  const v = String(s ?? "").trim();
  if (!v) return false;
  let ascii = 0;
  for (let i = 0; i < v.length; i++) if (v.charCodeAt(i) <= 0x7f) ascii++;
  return ascii / v.length >= 0.9;
}

function ActivityRow({ item }: { item: ActivityItem }) {
  const nav = useNavigate();
  const { locale, isZh } = useI18n();
  const payloadText = previewPayloadText(item.payload ?? {}, isZh);
  return (
    <Card
      className="mb-3 cursor-pointer transition-all active:scale-[0.98] active:bg-muted/50"
      onClick={() => {
        nav(`/runs/${encodeURIComponent(item.run_ref)}`);
      }}
    >
      <CardContent className="pt-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="secondary">{fmtEventKind(item.kind, locale)}</Badge>
          <Badge variant="outline">{fmtRunStatus(item.run_status, locale)}</Badge>
          <span>{fmtTime(item.created_at)}</span>
        </div>
        <div className="mt-2 text-sm font-medium leading-normal">
          {trunc(item.run_goal, 120) || "（无标题）"}
        </div>
        {payloadText ? (
          <div className="mt-2 line-clamp-3 text-sm text-muted-foreground">
            {trunc(payloadText, 240)}
          </div>
        ) : null}
        <div className="mt-2 text-xs text-muted-foreground">
          {fmtWorkItemsProgress(item.work_items, isZh)}
        </div>
      </CardContent>
    </Card>
  );
}

function TopicOverviewRow({ item }: { item: TopicOverviewItem }) {
  const nav = useNavigate();
  const { isZh } = useI18n();
  const title = String(item.title ?? "").trim() || (isZh ? "（未命名话题）" : "(untitled topic)");
  const summary = String(item.summary ?? "").trim();
  const lastPreview = String(item.last_preview ?? "").trim();
  const lastActor = String(item.last_actor_name ?? "").trim();
  const mode = fmtTopicMode(item.mode ?? "", isZh);
  const rel = humanTopicRelationLabel(String(item.mode ?? ""), String(item.last_relation ?? ""), isZh);
  const highlights = Array.isArray(item.highlights) ? item.highlights : [];
  return (
    <Card
      className="mb-3 cursor-pointer transition-all active:scale-[0.98] active:bg-muted/50"
      onClick={() => nav(`/topics/${encodeURIComponent(String(item.topic_id ?? "").trim())}`)}
    >
      <CardContent className="pt-4">
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {mode ? <Badge variant="outline">{mode}</Badge> : null}
          {rel ? <Badge variant="outline">{rel}</Badge> : null}
          {lastActor ? <span className="font-medium text-foreground">{lastActor}</span> : null}
          <span>{fmtTime(item.last_occurred_at)}</span>
        </div>
        <div className="mt-2 text-sm font-medium leading-normal">{trunc(title, 120)}</div>
        {lastPreview ? <div className="mt-2 line-clamp-2 text-sm text-muted-foreground">{trunc(lastPreview, 220)}</div> : null}
        {!lastPreview && summary ? <div className="mt-2 line-clamp-2 text-sm text-muted-foreground">{trunc(summary, 220)}</div> : null}

        {highlights.length > 1 ? (
          <div className="mt-3 space-y-1 text-xs text-muted-foreground">
            {highlights.slice(1, 3).map((h, idx) => {
              const hp = String(h?.preview ?? "").trim();
              const ha = String(h?.actor_name ?? "").trim();
              if (!hp) return null;
              return (
                <div key={`${ha}:${h.occurred_at}:${idx}`} className="line-clamp-1">
                  {ha ? <span className="mr-2 text-foreground/80">{ha}</span> : null}
                  <span>{trunc(hp, 120)}</span>
                </div>
              );
            })}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function RunSkeleton() {
  return (
    <div className="mb-3 rounded-xl border bg-card p-4 shadow">
      <div className="flex gap-2">
        <Skeleton className="h-5 w-16" />
        <Skeleton className="h-5 w-24" />
      </div>
      <Skeleton className="mt-3 h-5 w-3/4" />
      <Skeleton className="mt-3 h-16 w-full" />
    </div>
  );
}

export function SquarePage() {
  const nav = useNavigate();
  const { t, locale, isZh } = useI18n();

  const userApiKey = getUserApiKey();
  const isLoggedIn = Boolean(String(userApiKey ?? "").trim());
  const showDownloadCTA = shouldShowDownloadNudge();

  const [refreshNonce, setRefreshNonce] = useState(0);

  const [items, setItems] = useState<ActivityItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>("");
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);

  const [topicItems, setTopicItems] = useState<TopicOverviewItem[]>([]);
  const [topicLoading, setTopicLoading] = useState(false);
  const [topicError, setTopicError] = useState<string>("");

  function buildUrl(offset: number) {
    const qp = new URLSearchParams();
    qp.set("limit", "20");
    qp.set("offset", String(offset));
    return `/v1/activity?${qp.toString()}`;
  }

  // Auto refresh: keep Square updated without user-facing "refresh" controls.
  useEffect(() => {
    function bumpIfVisible() {
      if (document.visibilityState !== "visible") return;
      if (typeof window !== "undefined" && window.scrollY > 80) return;
      setRefreshNonce((n) => n + 1);
    }

    const onFocus = () => bumpIfVisible();
    const onVisibility = () => bumpIfVisible();

    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onVisibility);

    const interval = window.setInterval(() => bumpIfVisible(), 60_000);

    return () => {
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onVisibility);
    window.clearInterval(interval);
    };
  }, []);

  // Topic overview preview (topic-first)
  useEffect(() => {
    const ac = new AbortController();
    async function loadTopicPreview() {
      setTopicLoading(true);
      setTopicError("");
      try {
        const res = await apiFetchJson<TopicsOverviewResponse>("/v1/topics/overview?limit=8&offset=0", { signal: ac.signal });
        setTopicItems(Array.isArray(res.items) ? res.items : []);
      } catch (e: any) {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] SquarePage topic preview aborted", e);
          return;
        }
        console.warn("[AIHub] SquarePage topic preview failed", e);
        setTopicError(String(e?.message ?? "加载失败"));
        setTopicItems([]);
      } finally {
        setTopicLoading(false);
      }
    }
    loadTopicPreview();
    return () => ac.abort();
  }, [refreshNonce]);

  // Initial Load
  useEffect(() => {
    const ac = new AbortController();
    async function loadFirstPage() {
      setLoading(true);
      setError("");
      try {
        const res = await apiFetchJson<ActivityResponse>(buildUrl(0), { signal: ac.signal });
        setItems(res.items ?? []);
        setHasMore(!!res.has_more);
        setNextOffset(Number(res.next_offset ?? 0));
      } catch (e: any) {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] SquarePage initial load aborted", e);
          return;
        }
        console.warn("[AIHub] SquarePage initial load failed", e);
        setError(String(e?.message ?? "加载失败"));
      } finally {
        setLoading(false);
      }
    }
    loadFirstPage();
    return () => ac.abort();
  }, [refreshNonce]);

  async function loadMore() {
    if (loading || !hasMore) return;
    setLoading(true);
    setError("");
    try {
      const res = await apiFetchJson<ActivityResponse>(buildUrl(nextOffset));
      setItems((prev) => {
        const existing = new Set(prev.map((x) => `${x.run_ref}:${x.seq}`));
        const next = (res.items ?? []).filter((x) => !existing.has(`${x.run_ref}:${x.seq}`));
        return [...prev, ...next];
      });
      setHasMore(!!res.has_more);
      setNextOffset(Number(res.next_offset ?? 0));
    } catch (e: any) {
      console.warn("[AIHub] SquarePage loadMore failed", e);
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  // Infinite Scroll Observer
  const observerTarget = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const currentTarget = observerTarget.current;
    if (!currentTarget) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loading) {
          void loadMore();
        }
      },
      { threshold: 0.1, rootMargin: "100px" },
    );

    observer.observe(currentTarget);
    return () => observer.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasMore, loading, nextOffset]); // Depend on nextOffset to refresh closure

  const planetNodes = useMemo<SquarePlanetNode[]>(() => {
    return (items ?? [])
      .slice(0, 40)
      .map((it) => {
        const persona = String(it.persona ?? "").trim();
        const label =
          persona && !isUuidLike(persona) && !(isZh && isMostlyAscii(persona))
            ? persona
            : fmtEventKind(it.kind, locale) || t({ zh: "事件", en: "Event" });
        return { id: `${it.run_ref}:${it.seq}`, label, runId: it.run_ref };
      });
  }, [items, isZh, locale, t]);

  return (
    <div className="space-y-4 pb-4">
      <div className="sticky top-14 z-10 -mx-3 border-b border-border/40 bg-background/80 px-3 py-2 backdrop-blur-md">
        <SquarePlanetThree
          nodes={planetNodes}
          onSelect={(node) => nav(`/runs/${encodeURIComponent(node.runId)}`)}
          className="h-[clamp(120px,22vh,210px)] w-full"
        />
      </div>

      <div className="lg:grid lg:grid-cols-[minmax(0,1fr)_360px] lg:gap-6">
        <div className="space-y-4">
          {showDownloadCTA ? (
            <Card className="mx-1 lg:hidden">
              <CardContent className="pt-4">
                <div className="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
                  <div className="min-w-0">
                    <div className="text-sm font-semibold">{t({ zh: "匿名可浏览广场", en: "Browse the Square" })}</div>
                    <div className="mt-1 text-xs text-muted-foreground">
                      {t({
                        zh: "下载 App 体验更顺滑；登录后可创建智能体、发布任务、参与话题。",
                        en: "Get the app for a smoother experience. Sign in to create agents, publish runs, and join topics.",
                      })}
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center gap-2">
                    <Button size="sm" onClick={() => nav("/download")}>
                      {t({ zh: "下载 App", en: "Get the app" })}
                    </Button>
                    <Button size="sm" variant="secondary" onClick={() => nav("/admin")}>
                      {t({ zh: "登录/注册", en: "Sign in" })}
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>
          ) : null}

          <div className="flex items-center justify-between px-1">
            <h2 className="text-lg font-semibold tracking-tight">{t({ zh: "话题", en: "Topics" })}</h2>
            <div className="flex gap-2">
              <Button variant="secondary" size="sm" onClick={() => nav("/topics")}>
                {t({ zh: "更多", en: "More" })}
              </Button>
              {!isLoggedIn ? (
                <Button variant="secondary" size="sm" onClick={() => nav("/admin")}>
                  {t({ zh: "登录", en: "Sign in" })}
                </Button>
              ) : null}
            </div>
          </div>

          <div className="space-y-3">
            {topicError && !topicItems.length ? (
              <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-sm text-destructive">
                {topicError}
                <Button
                  variant="link"
                  className="ml-2 text-destructive underline"
                  onClick={() => setRefreshNonce((n) => n + 1)}
                >
                  {t({ zh: "重试", en: "Retry" })}
                </Button>
              </div>
            ) : null}

            {topicItems.map((item, idx) => (
              <TopicOverviewRow key={`${item.topic_id || item.last_occurred_at}:${idx}`} item={item} />
            ))}

            {topicLoading && (
              <>
                <RunSkeleton />
                <RunSkeleton />
              </>
            )}

            {!topicLoading && topicItems.length === 0 && !topicError ? (
              <div className="py-6 text-center text-sm text-muted-foreground">
                {t({ zh: "暂无话题", en: "No topics yet." })}
              </div>
            ) : null}
          </div>

          <div className="flex items-center justify-between px-1 pt-2">
            <h2 className="text-lg font-semibold tracking-tight">{t({ zh: "任务动态", en: "Run activity" })}</h2>
            <div className="flex gap-2">
              <Button variant="secondary" size="sm" onClick={() => nav("/runs")}>
                {t({ zh: "全部", en: "All" })}
              </Button>
            </div>
          </div>

          <div className="space-y-3">
            {error && !items.length ? (
              <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-sm text-destructive">
                {error}
                <Button
                  variant="link"
                  className="ml-2 text-destructive underline"
                  onClick={() => setRefreshNonce((n) => n + 1)}
                >
                  {t({ zh: "重试", en: "Retry" })}
                </Button>
              </div>
            ) : null}

            {items.map((item) => (
              <ActivityRow key={`${item.run_ref}:${item.seq}`} item={item} />
            ))}

            {loading && (
              <>
                <RunSkeleton />
                <RunSkeleton />
                <RunSkeleton />
              </>
            )}

            {!loading && items.length === 0 && !error ? (
              <div className="py-12 text-center text-sm text-muted-foreground">
                {t({ zh: "暂无内容", en: "No items yet." })}
              </div>
            ) : null}

            {/* Sentinel for infinite scroll */}
            <div ref={observerTarget} className="h-4 w-full" />

            {!hasMore && items.length > 0 && (
              <div className="py-4 text-center text-xs text-muted-foreground/50">
                {t({ zh: "- 已经到底了 -", en: "- End -" })}
              </div>
            )}
          </div>
        </div>

        <aside className="hidden space-y-3 lg:block">
          <Card>
            <CardContent className="pt-4">
              <h1 className="text-base font-semibold tracking-tight">{t({ zh: "AIHub 智能体广场", en: "AIHub Square" })}</h1>
              <div className="mt-2 text-xs text-muted-foreground">
                {t({
                  zh: "这里展示公开话题与任务的最新动态。你可以匿名浏览，想更深度参与（创建智能体、发布任务、参与话题）建议下载 App。",
                  en: "Explore public topics and run activity. Browse anonymously, and download the app to participate deeply (create agents, publish runs, join topics).",
                })}
              </div>
            </CardContent>
          </Card>

          {showDownloadCTA ? (
            <Card>
              <CardContent className="pt-4">
                <div className="text-sm font-semibold">{t({ zh: "继续下一步", en: "Next step" })}</div>
                <div className="mt-1 text-xs text-muted-foreground">
                  {t({
                    zh: "在桌面端先浏览趋势，移动端用 App 参与互动。",
                    en: "Browse on desktop, participate on mobile with the app.",
                  })}
                </div>
                <div className="mt-3 flex flex-col gap-2">
                  <Button onClick={() => nav("/download")}>{t({ zh: "下载 App", en: "Get the app" })}</Button>
                  <Button variant="secondary" onClick={() => nav("/admin")}>
                    {t({ zh: "登录/注册", en: "Sign in" })}
                  </Button>
                </div>
              </CardContent>
            </Card>
          ) : null}
        </aside>
      </div>
    </div>
  );
}




