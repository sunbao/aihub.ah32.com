import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { Capacitor } from "@capacitor/core";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { apiFetchJson } from "@/lib/api";
import { fmtTime } from "@/lib/format";
import { useI18n } from "@/lib/i18n";
import { getUserApiKey } from "@/lib/storage";
import { humanThreadRelationLabel } from "@/lib/topicRelations";

type TopicThreadMessage = {
  text: string;
  actor_name?: string;
  relation?: string;
  created_at: string;
  reply_to?: { agent_ref: string; message_id: string };
  thread_root?: { agent_ref: string; message_id: string };

  // Internal only; must not be rendered.
  actor_ref?: string;
  message_id: string;
  occurred_at?: string;
};

type TopicThreadTopic = {
  topic_id: string;
  title: string;
  summary?: string;
  mode?: string;
  visibility?: string;
};

type TopicThreadResponse = {
  topic: TopicThreadTopic;
  messages: TopicThreadMessage[];
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

function ThreadSkeleton() {
  return (
    <div className="space-y-3">
      <div className="rounded-xl border bg-card p-4 shadow-sm">
        <Skeleton className="h-6 w-2/3" />
        <Skeleton className="mt-3 h-4 w-full" />
        <Skeleton className="mt-2 h-4 w-5/6" />
      </div>
      <div className="rounded-xl border bg-card p-4 shadow-sm">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="mt-3 h-4 w-full" />
        <Skeleton className="mt-2 h-4 w-5/6" />
      </div>
      <div className="rounded-xl border bg-card p-4 shadow-sm">
        <Skeleton className="h-4 w-28" />
        <Skeleton className="mt-3 h-4 w-full" />
        <Skeleton className="mt-2 h-4 w-2/3" />
      </div>
    </div>
  );
}

function refKey(ref: { agent_ref: string; message_id: string } | undefined): string {
  if (!ref) return "";
  return `${String(ref.agent_ref ?? "").trim()}:${String(ref.message_id ?? "").trim()}`;
}

function msgKey(m: TopicThreadMessage): string {
  const a = String(m.actor_ref ?? "").trim();
  const id = String(m.message_id ?? "").trim();
  return `${a}:${id}`;
}

export function TopicDetailPage() {
  const nav = useNavigate();
  const { topicID } = useParams();
  const { t, isZh } = useI18n();

  const tid = String(topicID ?? "").trim();
  const userApiKey = getUserApiKey();
  const isLoggedIn = !!userApiKey;
  const isNative = Capacitor.isNativePlatform();
  const gatedForAnonymousWeb = !isLoggedIn && !isNative;

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [data, setData] = useState<TopicThreadResponse | null>(null);

  async function load() {
    if (!tid) return;
    if (gatedForAnonymousWeb) return;
    setLoading(true);
    setError("");
    try {
      const res = await apiFetchJson<TopicThreadResponse>(`/v1/topics/${encodeURIComponent(tid)}/thread?limit=300`);
      setData(res ?? null);
    } catch (e: any) {
      console.warn("[AIHub] TopicDetailPage load failed", e);
      setError(String(e?.message ?? t({ zh: "加载失败", en: "Load failed" })));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tid, gatedForAnonymousWeb]);

  const tree = useMemo(() => {
    const msgs = Array.isArray(data?.messages) ? data!.messages : [];

    const byKey = new Map<string, TopicThreadMessage>();
    const children = new Map<string, TopicThreadMessage[]>();
    const roots: TopicThreadMessage[] = [];

    for (const m of msgs) {
      byKey.set(msgKey(m), m);
    }
    for (const m of msgs) {
      const parent = refKey(m.reply_to);
      const self = msgKey(m);
      if (!self) continue;
      if (!parent) {
        roots.push(m);
        continue;
      }
      if (!children.has(parent)) children.set(parent, []);
      children.get(parent)!.push(m);
    }

    // Stable ordering: use occurred_at/created_at as available.
    const sortByTime = (a: TopicThreadMessage, b: TopicThreadMessage) => {
      const ta = String(a.occurred_at ?? a.created_at ?? "").trim();
      const tb = String(b.occurred_at ?? b.created_at ?? "").trim();
      return ta.localeCompare(tb);
    };
    roots.sort(sortByTime);
    for (const list of children.values()) list.sort(sortByTime);

    return { roots, children };
  }, [data]);

  const topic = data?.topic;
  const title = String(topic?.title ?? "").trim() || (isZh ? "（未命名话题）" : "(untitled topic)");
  const summary = String(topic?.summary ?? "").trim();
  const mode = fmtMode(String(topic?.mode ?? ""), isZh);

  if (gatedForAnonymousWeb) {
    return (
      <div className="space-y-3">
        <Card>
          <CardContent className="pt-4">
            <div className="text-base font-semibold">{t({ zh: "话题详情需在 App 内查看", en: "Open in the app" })}</div>
            <div className="mt-2 text-sm text-muted-foreground">
              {t({
                zh: "网页端支持匿名浏览广场；进入话题详情阅读完整讨论与参与互动，请下载 App。",
                en: "Web supports anonymous Square browsing. Download the app to read the full thread and participate.",
              })}
            </div>
            <div className="mt-3 flex flex-wrap gap-2">
              <Button onClick={() => nav("/download")}>{t({ zh: "下载 App", en: "Get the app" })}</Button>
              <Button variant="secondary" onClick={() => nav("/admin")}>
                {t({ zh: "登录/注册", en: "Sign in" })}
              </Button>
              <Button variant="ghost" onClick={() => nav("/")}>
                {t({ zh: "返回广场", en: "Back to Square" })}
              </Button>
            </div>
            <div className="mt-3 text-xs text-muted-foreground">
              {t({ zh: "话题编号：", en: "Topic id: " })}
              <span className="ml-1 font-mono text-foreground/80">{tid}</span>
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  function renderNode(m: TopicThreadMessage, depth: number) {
    const actor = String(m.actor_name ?? "").trim();
    const text = String(m.text ?? "").trim();
    const rel = humanThreadRelationLabel(String(topic?.mode ?? ""), m.reply_to, m.thread_root, isZh) || String(m.relation ?? "").trim();
    const time = String(m.created_at ?? m.occurred_at ?? "").trim();

    const isReply = depth > 0;
    const kids = tree.children.get(msgKey(m)) ?? [];

    return (
      <div key={`${msgKey(m)}:${time}`} className="space-y-2">
        <div className="relative">
          {isReply ? (
            <>
              {/* Thread rail connector: dot on the rail + elbow into this message. */}
              <div className="pointer-events-none absolute left-[-30px] top-6 h-2 w-2 rounded-full bg-muted-foreground/40" />
              <div className="pointer-events-none absolute left-[-24px] top-[27px] h-px w-4 bg-muted-foreground/25" />
            </>
          ) : null}
          <Card>
            <CardContent className="pt-4">
              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                {rel ? <Badge variant="outline">{rel}</Badge> : null}
                {actor ? <span className="font-medium text-foreground">{actor}</span> : null}
                {time ? <span>{fmtTime(time)}</span> : null}
              </div>
              <div className="mt-2 whitespace-pre-wrap text-sm leading-relaxed">{text}</div>
            </CardContent>
          </Card>
        </div>
        {kids.length ? (
          <div className="relative ml-4 space-y-2 border-l border-muted-foreground/20 pl-6">
            {kids.map((k) => renderNode(k, depth + 1))}
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <Card>
        <CardContent className="pt-4">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                {mode ? <Badge variant="outline">{mode}</Badge> : null}
                <Badge variant="secondary">{t({ zh: "话题", en: "Topic" })}</Badge>
              </div>
              <div className="mt-2 truncate text-base font-semibold">{title}</div>
              {summary ? <div className="mt-1 line-clamp-2 text-sm text-muted-foreground">{summary}</div> : null}
            </div>
            <div className="flex shrink-0 gap-2">
              <Button variant="secondary" onClick={() => nav("/topics")}>
                {t({ zh: "返回", en: "Back" })}
              </Button>
              <Button variant="secondary" onClick={() => load()} disabled={loading}>
                {t({ zh: "刷新", en: "Refresh" })}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {error ? <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">{error}</div> : null}

      {loading && !data ? <ThreadSkeleton /> : null}

      {!loading && data ? (
        <div className="space-y-3">
          {tree.roots.length ? (
            tree.roots.map((m) => renderNode(m, 0))
          ) : (
            <div className="py-12 text-center text-sm text-muted-foreground">{t({ zh: "暂无内容", en: "No messages yet." })}</div>
          )}
        </div>
      ) : null}
    </div>
  );
}
