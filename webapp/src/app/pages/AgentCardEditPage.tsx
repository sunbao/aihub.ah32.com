import { useNavigate, useParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import { getUserApiKey } from "@/lib/storage";
import { AgentCardWizard } from "@/app/components/AgentCardWizard";

export function AgentCardEditPage() {
  const { agentId } = useParams();
  const id = String(agentId ?? "").trim();
  const nav = useNavigate();
  const { t } = useI18n();

  const userApiKey = getUserApiKey();

  if (!id) {
    return <div className="text-sm text-muted-foreground">{t({ zh: "缺少智能体参数。", en: "Missing agent id." })}</div>;
  }

  if (!userApiKey) {
    return (
      <div className="mx-auto max-w-3xl space-y-3 p-6">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">{t({ zh: "需要登录", en: "Login required" })}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <div className="text-muted-foreground">
              {t({ zh: "编辑智能体卡片需要用户 API Key。请先在“我的”页面登录。", en: "Editing an agent card requires a user API key. Please login on the Me page first." })}
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="secondary" onClick={() => nav("/me")}>
                {t({ zh: "去登录", en: "Go to login" })}
              </Button>
              <Button variant="outline" onClick={() => nav(-1)}>
                {t({ zh: "返回", en: "Back" })}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl space-y-4 p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="text-base font-semibold">{t({ zh: "编辑智能体卡片", en: "Edit agent card" })}</div>
        <Button variant="outline" onClick={() => nav(-1)}>
          {t({ zh: "返回", en: "Back" })}
        </Button>
      </div>

      <AgentCardWizard agentId={id} userApiKey={userApiKey} />
    </div>
  );
}
