import { useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
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
  } catch {
    return String(payload ?? "");
  }
}

function ProgressView({ runId }: { runId: string }) {
  const [events, setEvents] = useState<EventDTO[]>([]);
  const [error, setError] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);
  const [onlyKeyNodes, setOnlyKeyNodes] = useState(false);
  const bottomRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const base = getApiBaseUrl();
    const url = `${base}/v1/runs/${encodeURIComponent(runId)}/stream?after_seq=0`;

    setEvents([]);
    setError("");

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
      setError("进度流连接中断（可切到“记录”查看历史）。");
    });

    return () => es.close();
  }, [runId]);

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
        <div className="space-y-2">
          {shown.map((ev) => (
            <Card key={ev.seq}>
              <CardContent className="pt-4">
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant={ev.is_key_node ? "default" : "secondary"}>{fmtEventKind(ev.kind)}</Badge>
                  {ev.persona ? <span className="font-medium text-foreground">{ev.persona}</span> : null}
                  <span>{fmtTime(ev.created_at)}</span>
                </div>
                <pre className="mt-2 whitespace-pre-wrap text-sm leading-relaxed">{safeText(ev.payload)}</pre>
              </CardContent>
            </Card>
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
  const [error, setError] = useState("");

  async function load({ reset }: { reset: boolean }) {
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
    } catch (e: any) {
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load({ reset: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runId]);

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
        <div className="space-y-2">
          {events.map((ev) => (
            <Card key={ev.seq}>
              <CardContent className="pt-4">
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant={ev.is_key_node ? "default" : "secondary"}>{fmtEventKind(ev.kind)}</Badge>
                  {ev.persona ? <span className="font-medium text-foreground">{ev.persona}</span> : null}
                  <span>{fmtTime(ev.created_at)}</span>
                </div>
                <pre className="mt-2 whitespace-pre-wrap text-sm leading-relaxed">{safeText(ev.payload)}</pre>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        !loading && !error ? <div className="text-sm text-muted-foreground">暂无记录。</div> : null
      )}

      <Button
        disabled={loading}
        variant="secondary"
        className="w-full"
        onClick={() => load({ reset: false })}
      >
        {loading ? "加载中…" : "加载更多"}
      </Button>
    </div>
  );
}

function OutputView({ runId }: { runId: string }) {
  const [latest, setLatest] = useState<RunOutput | null>(null);
  const [selectedVersion, setSelectedVersion] = useState<number>(0);
  const [selected, setSelected] = useState<RunArtifact | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function loadLatest() {
    setLoading(true);
    setError("");
    try {
      const out = await apiFetchJson<RunOutput>(`/v1/runs/${encodeURIComponent(runId)}/output`);
      setLatest(out);
      if (!selectedVersion) setSelectedVersion(out.version);
    } catch (e: any) {
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
      .catch((e: any) => setError(String(e?.message ?? "加载失败")));
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
          <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">
            {kind ? <span>类型：{fmtArtifactKind(kind)}</span> : null}
            {author ? <span>作者：{author}</span> : null}
            {createdAt ? <span>时间：{fmtTime(createdAt)}</span> : null}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-4">
          <pre className="whitespace-pre-wrap text-sm leading-relaxed">{content || "（暂无作品）"}</pre>
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
      .catch((e: any) => setError(String(e?.message ?? "加载失败")))
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
          {loading && !run ? <div className="text-sm text-muted-foreground">加载中…</div> : null}
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

