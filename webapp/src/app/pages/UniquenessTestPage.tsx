import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { getUserApiKey } from "@/lib/storage";

type SwapTest = {
  kind: string;
  schema_version: number;
  agent_id: string;
  swap_test_id: string;
  created_at: string;
  questions: Array<{ question: string; answer: string }>;
  conclusion: string;
};

export function UniquenessTestPage() {
  const { toast } = useToast();
  const { agentId } = useParams();
  const id = String(agentId ?? "").trim();
  const userApiKey = getUserApiKey();
  const isLoggedIn = !!userApiKey;

  const [swapTestId, setSwapTestId] = useState("");
  const [data, setData] = useState<SwapTest | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function create() {
    if (!id) return;
    setLoading(true);
    setError("");
    try {
      const res = await apiFetchJson<{ swap_test_id: string }>(`/v1/agents/${encodeURIComponent(id)}/swap-tests`, {
        method: "POST",
        apiKey: userApiKey,
      });
      const sid = String(res.swap_test_id ?? "").trim();
      if (!sid) throw new Error("missing swap_test_id");
      setSwapTestId(sid);
      toast({ title: "已生成" });
    } catch (e: any) {
      setError(String(e?.message ?? "生成失败"));
      toast({ title: "生成失败", description: String(e?.message ?? ""), variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!id || !swapTestId) return;
    const ac = new AbortController();
    setError("");
    apiFetchJson<SwapTest>(`/v1/agents/${encodeURIComponent(id)}/swap-tests/${encodeURIComponent(swapTestId)}`, {
      apiKey: userApiKey,
      signal: ac.signal,
    })
      .then((res) => setData(res))
      .catch((e: any) => setError(String(e?.message ?? "加载失败")));
    return () => ac.abort();
  }, [id, swapTestId, userApiKey]);

  if (!id) return <div className="text-sm text-muted-foreground">缺少星灵参数。</div>;
  if (!isLoggedIn) return <div className="text-sm text-muted-foreground">请先登录。</div>;

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">测试独特性（交换测试）</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="text-xs text-muted-foreground">
            这是“风格参考”的测试视角：禁止冒充/自称为任何真实人物或角色。
          </div>
          <Button size="sm" onClick={create} disabled={loading}>
            {loading ? "生成中…" : "生成一次测试"}
          </Button>
          {error ? <div className="text-sm text-destructive">{error}</div> : null}
        </CardContent>
      </Card>

      {data ? (
        <Card>
          <CardContent className="pt-4 space-y-3">
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Badge variant="secondary">{data.kind}</Badge>
              <span>{data.swap_test_id}</span>
            </div>
            {data.questions?.length ? (
              <div className="space-y-2">
                {data.questions.map((q, idx) => (
                  <div key={idx} className="rounded-md border bg-background px-3 py-2">
                    <div className="text-sm font-medium">{q.question}</div>
                    <div className="mt-1 text-sm text-muted-foreground">{q.answer}</div>
                  </div>
                ))}
              </div>
            ) : null}
            {data.conclusion ? (
              <div className="rounded-md bg-muted px-3 py-2 text-sm leading-relaxed">{data.conclusion}</div>
            ) : null}
          </CardContent>
        </Card>
      ) : (
        <div className="text-sm text-muted-foreground">尚未生成。</div>
      )}
    </div>
  );
}

