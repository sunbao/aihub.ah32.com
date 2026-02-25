import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { apiFetchJson } from "@/lib/api";
import { fmtTime } from "@/lib/format";
import { getUserApiKey } from "@/lib/storage";

type WeeklyReport = {
  kind: string;
  schema_version: number;
  agent_id: string;
  week: string;
  generated_at: string;
  dimensions: Record<string, number>;
  dimensions_delta?: Record<string, number>;
  highlights?: Array<{ type: string; title: string; snippet?: string; occurred_at: string }>;
};

export function WeeklyReportPage() {
  const { agentId } = useParams();
  const id = String(agentId ?? "").trim();
  const userApiKey = getUserApiKey();
  const isLoggedIn = !!userApiKey;

  const [week, setWeek] = useState("");
  const [data, setData] = useState<WeeklyReport | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    if (!id) return;
    setLoading(true);
    setError("");
    try {
      const q = week.trim() ? `?week=${encodeURIComponent(week.trim())}` : "";
      const res = await apiFetchJson<WeeklyReport>(`/v1/agents/${encodeURIComponent(id)}/weekly-reports${q}`, {
        apiKey: userApiKey,
      });
      setData(res);
    } catch (e: any) {
      setError(String(e?.message ?? "加载失败"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!isLoggedIn) return;
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, isLoggedIn]);

  if (!id) return <div className="text-sm text-muted-foreground">缺少星灵参数。</div>;
  if (!isLoggedIn) return <div className="text-sm text-muted-foreground">请先登录。</div>;

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">园丁周报</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="text-xs text-muted-foreground">输入 week（YYYY-WW）可查看指定周；为空则生成/读取当前周。</div>
          <div className="flex gap-2">
            <input
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
              placeholder="例如：2026-08"
              value={week}
              onChange={(e) => setWeek(e.target.value)}
            />
            <Button size="sm" variant="secondary" onClick={load} disabled={loading}>
              {loading ? "加载…" : "查看"}
            </Button>
          </div>
          {error ? <div className="text-sm text-destructive">{error}</div> : null}
        </CardContent>
      </Card>

      {data ? (
        <Card>
          <CardContent className="pt-4 space-y-3">
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Badge variant="secondary">{data.week}</Badge>
              <span>{fmtTime(data.generated_at)}</span>
            </div>
            <div className="flex flex-wrap gap-1">
              {Object.entries(data.dimensions ?? {}).map(([k, v]) => (
                <Badge key={k} variant="outline">
                  {k}:{Math.round(Number(v ?? 0))}
                  {data.dimensions_delta && k in data.dimensions_delta ? ` (${data.dimensions_delta[k] >= 0 ? "+" : ""}${data.dimensions_delta[k]})` : ""}
                </Badge>
              ))}
            </div>
            {data.highlights?.length ? (
              <div className="space-y-2">
                {data.highlights.slice(0, 10).map((h, idx) => (
                  <div key={idx} className="rounded-md border bg-background px-3 py-2">
                    <div className="text-sm font-medium">{h.title || h.type}</div>
                    {h.snippet ? <div className="mt-1 text-xs text-muted-foreground">{h.snippet}</div> : null}
                    <div className="mt-1 text-xs text-muted-foreground">{fmtTime(h.occurred_at)}</div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-sm text-muted-foreground">暂无高光。</div>
            )}
          </CardContent>
        </Card>
      ) : (
        <div className="text-sm text-muted-foreground">暂无数据。</div>
      )}
    </div>
  );
}

