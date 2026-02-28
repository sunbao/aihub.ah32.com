import { useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { apiFetchJson, getApiBaseUrl } from "@/lib/api";
import { fmtArtifactKind, fmtEventKind, fmtRunStatus, fmtTime, trunc } from "@/lib/format";

type RunPublic = {
  id: string;
  goal: string;
  constraints: string;
  status: string;
  created_at: string;
};

type EventDTO = {
  run_id: string;
  seq: number;
  kind: string;
  persona: string;
  payload: Record<string, unknown>;
  is_key_node: boolean;
  created_at: string;
};

type ReplayResponse = {
  run_id: string;
  events: EventDTO[];
  key_nodes: EventDTO[];
  after_seq: number;
  limit: number;
};

type RunOutput = {
  run_id: string;
  version: number;
  kind: string;
  author?: string;
  created_at?: string;
  content: string;
};

type RunArtifact = {
  run_id: string;
  version: number;
  kind: string;
  author?: string;
  content: string;
  created_at: string;
  linked_seq?: number | null;
  replay_url?: string;
};

function safeText(payload: Record<string, unknown>): string {
  const v = payload?.text;
  if (typeof v === "string") return v;
  try {
    return JSON.stringify(payload ?? {}, null, 2);
  } catch (error) {
    console.warn("[AIHub] Failed to stringify event payload", error);
    return String(payload ?? "");
  }
}

// Time-axis style event card
function EventCard({ ev }: { ev: EventDTO }) {
  return (
    <div className="flex gap-3">
      {/* timeline line */}
      <div className="flex flex-col items-center">
        <div
          className={`mt-1 h-2.5 w-2.5 shrink-0 rounded-full border-2 ${
            ev.is_key_node ? "border-primary bg-primary" : "border-muted-foreground/40 bg-background"
          }`}
        />
        <div className="mt-1 w-px flex-1 bg-border" />
      </div>
      {/* content */}
      <div className="mb-4 min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant={ev.is_key_node ? "default" : "secondary"} className="shrink-0">
            {fmtEventKind(ev.kind)}
          </Badge>
          {ev.persona ? <span className="font-medium text-foreground">{ev.persona}</span> : null}
          <span>{fmtTime(ev.created_at)}</span>
        </div>
        <pre className="mt-1.5 whitespace-pre-wrap break-words text-sm leading-relaxed text-foreground/80">
          {safeText(ev.payload)}
        </pre>
      </div>
    </div>
  );
}

function ProgressView({ runId }: { runId: string }) {
  const [events, setEvents] = useState<EventDTO[]>([]);
  const [error, setError] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);
  const [onlyKeyNodes, setOnlyKeyNodes] = useState(false);
  const [usePolling, setUsePolling] = useState(false);
  const bottomRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const base = getApiBaseUrl();
    const url = `${base}/v1/runs/${encodeURIComponent(runId)}/stream?after_seq=0`;

    setEvents([]);
    setError("");
    setUsePolling(false);

    const es = new EventSource(url);
    es.addEventListener("event", (ev) => {
      try {
        const data = JSON.parse((ev as MessageEvent).data) as EventDTO;
        setEvents((prev) => {
          const next = prev.concat(data);
          return next.length > 1000 ? next.slice(next.length - 1000) : next;
        });
      } catch (e: any) {
        console.warn("Failed to parse SSE event", e);
      }
    });
    es.addEventListener("error", () => {
      console.warn("[AIHub] SSE stream error, falling back to polling", { runId, url });
      setError("进度流不可用，已切换为轮询模式（可切到【记录】查看历史）。");
      setUsePolling(true);
      es.close();
    });

    return () => es.close();
  }, [runId]);

  useEffect(() => {
    if (!usePolling) return;
    let afterSeq = 0;
    let alive = true;

    async function tick() {
      try {
        const res = await apiFetchJson<ReplayResponse>(
          `/v1/runs/${encodeURIComponent(runId)}/replay?after_seq=${afterSeq}&limit=200`,
        );
        if (!alive) return;
        const list = Array.isArray(res.events) ? res.events : [];
        if (!list.length) return;
        const last = list[list.length - 1];
        afterSeq = Math.max(afterSeq, Number(last.seq ?? afterSeq));
        setEvents((prev) => {
          const next = prev.concat(list);
          return next.length > 1000 ? next.slice(next.length - 1000) : next;
        });
      } catch (e: any) {
        if (!alive) return;
        console.warn("[AIHub] RunDetailPage polling failed", { runId, error: e });
      }
    }

    tick();
    const timer = window.setInterval(tick, 2500);
    return () => {
      alive = false;
      window.clearInterval(timer);
    };
  }, [runId, usePolling]);

  useEffect(() => {
    if (!autoScroll) return;
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [autoScroll, events.length]);

  const shown = useMemo(() => (onlyKeyNodes ? events.filter((e) => e.is_key_node) : events), [events, onlyKeyNodes]);

  return (
    <div className="space-y-3">
      <Card>
        <CardContent className="pt-4">
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant={autoScroll ? "default" : "secondary"}
              onClick={() => setAutoScroll((v) => !v)}
            >
              {autoScroll ? "自动滚动：开" : "自动滚动：关"}
            </Button>
            <Button
              size="sm"
              variant={onlyKeyNodes ? "default" : "secondary"}
              onClick={() => setOnlyKeyNodes((v) => !v)}
            >
              {onlyKeyNodes ? "仅关键节点" : "显示全部"}
            </Button>
          </div>
          {error ? <div className="mt-3 text-sm text-destructive">{error}</div> : null}
          <div className="mt-3 text-xs text-muted-foreground">仅展示最近 1000 条事件。</div>
        </CardContent>
      </Card>

      {shown.length ? (
        <div className="pl-1">
          {shown.map((ev) => (
            <EventCard key={ev.seq} ev={ev} />
          ))}
          <div ref={bottomRef} />
        </div>
      ) : (
        <div className="text-sm text-muted-foreground">暂无事件。</div>
      )}
    </div>
  );
}

function ReplayView({ runId }: { runId: string }) {
  const [events, setEvents] = useState<EventDTO[]>([]);
  const [keyNodes, setKeyNodes] = useState<EventDTO[]>([]);
  const [afterSeq, setAfterSeq] = useState(0);
  const [loading, setLoading] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [error, setError] = useState("");
  const observerTarget = useRef<HTMLDivElement>(null);

  async function load({ reset }: { reset: boolean }) {
    if (loading) return;
    setLoading(true);
    setError("");
    try {
      const after = reset ? 0 : afterSeq;
      const res = await apiFetchJson<ReplayResponse>(
        `/v1/runs/${encodeURIComponent(runId)}/replay?after_seq=${after}&limit=200`,
      );
      const list = res.events ?? [];
      setEvents((prev) => (reset ? list : prev.concat(list)));
      if (reset) setKeyNodes(res.key_nodes ?? []);
      const last = list.length ? list[list.length - 1].seq : after;
      setAfterSeq(last);
      setHasMore(list.length >= 200);
    } catch (e: any) {
      console.warn("[AIHub] RunDetailPage replay load failed", { runId, reset, error: e });
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load({ reset: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runId]);

  // Infinite scroll for replay
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
  }, [hasMore, loading, afterSeq]);

  return (
    <div className="space-y-3">
      {error ? <div className="text-sm text-destructive">{error}</div> : null}

      {keyNodes.length ? (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">关键节点</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {keyNodes.slice(0, 20).map((ev) => (
              <div key={ev.seq} className="rounded-md border bg-background px-3 py-2 text-sm">
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant="default">{fmtEventKind(ev.kind)}</Badge>
                  {ev.persona ? <span className="font-medium text-foreground">{ev.persona}</span> : null}
                  <span>{fmtTime(ev.created_at)}</span>
                </div>
                <div className="mt-1 whitespace-pre-wrap text-sm">{trunc(safeText(ev.payload), 180)}</div>
              </div>
            ))}
          </CardContent>
        </Card>
      ) : null}

      {events.length ? (
        <div className="pl-1">
          {events.map((ev) => (
            <EventCard key={ev.seq} ev={ev} />
          ))}
        </div>
      ) : (
        !loading && !error ? <div className="text-sm text-muted-foreground">暂无记录。</div> : null
      )}

      {loading && (
        <div className="space-y-3 pl-1">
          {[0, 1, 2].map((i) => (
            <div key={i} className="flex gap-3">
              <div className="flex flex-col items-center">
                <Skeleton className="mt-1 h-2.5 w-2.5 rounded-full" />
                <div className="mt-1 w-px flex-1 bg-border" />
              </div>
              <div className="mb-4 flex-1 space-y-2">
                <Skeleton className="h-4 w-1/3" />
                <Skeleton className="h-12 w-full" />
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Sentinel */}
      <div ref={observerTarget} className="h-4 w-full" />

      {!hasMore && events.length > 0 && (
        <div className="py-4 text-center text-xs text-muted-foreground/50">- 已经到底了 -</div>
      )}
    </div>
  );
}

function OutputView({ runId }: { runId: string }) {
  const [latest, setLatest] = useState<RunOutput | null>(null);
  const [selectedVersion, setSelectedVersion] = useState<number>(0);
  const [selected, setSelected] = useState<RunArtifact | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [renderMarkdown, setRenderMarkdown] = useState(true);

  async function loadLatest() {
    setLoading(true);
    setError("");
    try {
      const out = await apiFetchJson<RunOutput>(`/v1/runs/${encodeURIComponent(runId)}/output`);
      setLatest(out);
      if (!selectedVersion) setSelectedVersion(out.version);
    } catch (e: any) {
      console.warn("[AIHub] RunDetailPage output loadLatest failed", { runId, error: e });
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadLatest();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runId]);

  useEffect(() => {
    if (!selectedVersion) return;
    setSelected(null);
    setError("");
    apiFetchJson<RunArtifact>(
      `/v1/runs/${encodeURIComponent(runId)}/artifacts/${encodeURIComponent(String(selectedVersion))}`,
    )
      .then((a) => setSelected(a))
      .catch((e: any) => {
        console.warn("[AIHub] RunDetailPage artifact load failed", { runId, version: selectedVersion, error: e });
        setError(String(e?.message ?? "加载失败"));
      });
  }, [runId, selectedVersion]);

  const maxVersion = latest?.version ?? 0;
  const versionOptions = useMemo(() => {
    if (!maxVersion) return [];
    const out: number[] = [];
    for (let i = 1; i <= maxVersion; i++) out.push(i);
    return out;
  }, [maxVersion]);

  const content = selected?.content ?? latest?.content ?? "";
  const kind = selected?.kind ?? latest?.kind ?? "";
  const author = selected?.author ?? latest?.author ?? "";
  const createdAt = selected?.created_at ?? latest?.created_at ?? "";

  return (
    <div className="space-y-3">
      <Card>
        <CardContent className="pt-4">
          <div className="flex items-center gap-2">
            <div className="text-sm font-medium">版本</div>
            <Input
              value={selectedVersion ? String(selectedVersion) : ""}
              onChange={(e) => setSelectedVersion(Number(e.target.value || 0))}
              inputMode="numeric"
              placeholder={maxVersion ? `1-${maxVersion}` : "暂无"}
              className="w-24"
            />
            <div className="text-xs text-muted-foreground">（1-{maxVersion || "?"}）</div>
            <Button size="sm" variant="secondary" disabled={loading} onClick={loadLatest}>
              刷新
            </Button>
          </div>
          {versionOptions.length ? (
            <div className="mt-2 flex flex-wrap gap-2">
              {versionOptions.slice(Math.max(0, versionOptions.length - 6)).map((v) => (
                <Button
                  key={v}
                  size="sm"
                  variant={v === selectedVersion ? "default" : "secondary"}
                  onClick={() => setSelectedVersion(v)}
                >
                  {v}
                </Button>
              ))}
            </div>
          ) : null}
          {error ? <div className="mt-3 text-sm text-destructive">{error}</div> : null}
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
            <div className="flex flex-wrap gap-2">
              {kind ? <span>类型：{fmtArtifactKind(kind)}</span> : null}
              {author ? <span>作者：{author}</span> : null}
              {createdAt ? <span>时间：{fmtTime(createdAt)}</span> : null}
            </div>
            {content ? (
              <Button size="sm" variant="ghost" onClick={() => setRenderMarkdown((v) => !v)} className="h-6 px-2 text-xs">
                {renderMarkdown ? "原文" : "渲染"}
              </Button>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-4">
          {loading && !content ? (
            <div className="space-y-2">
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-5/6" />
              <Skeleton className="h-4 w-4/6" />
              <Skeleton className="h-24 w-full" />
            </div>
          ) : content ? (
            renderMarkdown ? (
              <div className="prose prose-sm dark:prose-invert max-w-none break-words leading-relaxed">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
              </div>
            ) : (
              <pre className="whitespace-pre-wrap break-words text-sm leading-relaxed">{content}</pre>
            )
          ) : (
            <div className="text-sm text-muted-foreground">（暂无作品）</div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export function RunDetailPage() {
  const { runId } = useParams();
  const rid = String(runId ?? "").trim();

  const [run, setRun] = useState<RunPublic | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const defaultTab = useMemo(() => {
    const s = String(run?.status ?? "").toLowerCase();
    if (s === "completed") return "output";
    if (s === "failed") return "replay";
    return "progress";
  }, [run?.status]);

  const [tab, setTab] = useState<string>("progress");
  useEffect(() => setTab(defaultTab), [defaultTab]);

  useEffect(() => {
    if (!rid) return;
    const ac = new AbortController();
    setLoading(true);
    setError("");
    apiFetchJson<RunPublic>(`/v1/runs/${encodeURIComponent(rid)}`, { signal: ac.signal })
      .then((res) => setRun(res))
      .catch((e: any) => {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] RunDetailPage load aborted", e);
          return;
        }
        console.warn("[AIHub] RunDetailPage load failed", { runId: rid, error: e });
        setError(String(e?.message ?? "加载失败"));
      })
      .finally(() => setLoading(false));
    return () => ac.abort();
  }, [rid]);

  if (!rid) return <div className="text-sm text-muted-foreground">缺少任务参数。</div>;

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">任务摘要</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {loading && !run ? (
            <div className="space-y-2">
              <Skeleton className="h-5 w-1/3" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-3/4" />
            </div>
          ) : null}
          {error ? <div className="text-sm text-destructive">{error}</div> : null}
          {run ? (
            <>
              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                <Badge variant="secondary">{fmtRunStatus(run.status)}</Badge>
                <span>{fmtTime(run.created_at)}</span>
              </div>
              <div className="text-sm font-medium">{trunc(run.goal, 200) || "（无标题）"}</div>
              {run.constraints ? (
                <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                  {trunc(run.constraints, 260)}
                </div>
              ) : null}
            </>
          ) : null}
        </CardContent>
      </Card>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="grid w-full grid-cols-3">
          <TabsTrigger value="progress">进度</TabsTrigger>
          <TabsTrigger value="replay">记录</TabsTrigger>
          <TabsTrigger value="output">作品</TabsTrigger>
        </TabsList>
        <TabsContent value="progress" className="mt-3">
          <ProgressView runId={rid} />
        </TabsContent>
        <TabsContent value="replay" className="mt-3">
          <ReplayView runId={rid} />
        </TabsContent>
        <TabsContent value="output" className="mt-3">
          <OutputView runId={rid} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
