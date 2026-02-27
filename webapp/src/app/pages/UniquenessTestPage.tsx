import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { apiFetchJson } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
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
  const { t } = useI18n();
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
      if (!sid) {
        throw new Error(
          t({
            zh: "生成失败：未返回测试ID",
            en: "Generation failed: missing test id",
          }),
        );
      }
      setSwapTestId(sid);
      toast({ title: t({ zh: "已生成", en: "Generated" }) });
    } catch (e: any) {
      console.warn("[AIHub] UniquenessTestPage create failed", e);
      setError(String(e?.message ?? t({ zh: "生成失败", en: "Generation failed" })));
      toast({
        title: t({ zh: "生成失败", en: "Generation failed" }),
        description: String(e?.message ?? ""),
        variant: "destructive",
      });
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
      .catch((e: any) => {
        if (e?.name === "AbortError") {
          console.debug("[AIHub] UniquenessTestPage load aborted", e);
          return;
        }
        console.warn("[AIHub] UniquenessTestPage load failed", e);
        setError(String(e?.message ?? "加载失败"));
      });
    return () => ac.abort();
  }, [id, swapTestId, userApiKey]);

  if (!id) return <div className="text-sm text-muted-foreground">{t({ zh: "缺少智能体参数。", en: "Missing agent parameter." })}</div>;
  if (!isLoggedIn) return <div className="text-sm text-muted-foreground">{t({ zh: "请先登录。", en: "Please sign in first." })}</div>;

  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">{t({ zh: "独特性测试（交换测试）", en: "Uniqueness test (swap test)" })}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="text-xs text-muted-foreground">
            {t({
              zh: "这是“风格参考”的测试视角：禁止冒充/自称为任何真实人物或角色。",
              en: "This is a “style reference” perspective test. Do not impersonate or claim to be any real person or character.",
            })}
          </div>
          <Button size="sm" onClick={create} disabled={loading}>
            {loading ? t({ zh: "生成中…", en: "Generating…" }) : t({ zh: "生成一次测试", en: "Generate a test" })}
          </Button>
          {error ? <div className="text-sm text-destructive">{error}</div> : null}
        </CardContent>
      </Card>

      {data ? (
        <Card>
          <CardContent className="pt-4 space-y-3">
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Badge variant="secondary">{t({ zh: "交换测试", en: "Swap test" })}</Badge>
              <span>{t({ zh: "已生成", en: "Generated" })}</span>
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
        <div className="text-sm text-muted-foreground">{t({ zh: "尚未生成。", en: "Not generated yet." })}</div>
      )}
    </div>
  );
}

