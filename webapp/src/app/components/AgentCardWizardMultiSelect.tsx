import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useI18n } from "@/lib/i18n";

import type { CatalogLabeledItem } from "@/app/lib/agentCardCatalogs";

export type AgentCardWizardTone = "cyan" | "violet" | "indigo" | "green" | "sky" | "amber" | "slate";

export function MultiSelect({
  title,
  options,
  selected,
  onChange,
  maxSelected = 24,
  required = false,
  tone,
}: {
  title: string;
  options: CatalogLabeledItem[];
  selected: string[];
  onChange: (next: string[]) => void;
  maxSelected?: number;
  required?: boolean;
  tone?: AgentCardWizardTone;
}) {
  const [q, setQ] = useState("");
  const { t, isZh } = useI18n();

  const toneCard =
    tone === "cyan"
      ? "border-l-4 border-cyan-500/50"
      : tone === "violet"
        ? "border-l-4 border-violet-500/50"
        : tone === "indigo"
          ? "border-l-4 border-indigo-500/50"
          : tone === "green"
            ? "border-l-4 border-green-500/50"
            : tone === "sky"
              ? "border-l-4 border-sky-500/50"
              : tone === "amber"
                ? "border-l-4 border-amber-500/50"
                : tone === "slate"
                  ? "border-l-4 border-slate-500/50"
                  : "";

  const toneActive =
    tone === "cyan"
      ? "bg-cyan-600 text-white hover:bg-cyan-600/90"
      : tone === "violet"
        ? "bg-violet-600 text-white hover:bg-violet-600/90"
        : tone === "indigo"
          ? "bg-indigo-600 text-white hover:bg-indigo-600/90"
          : tone === "green"
            ? "bg-green-600 text-white hover:bg-green-600/90"
            : tone === "sky"
              ? "bg-sky-600 text-white hover:bg-sky-600/90"
              : tone === "amber"
                ? "bg-amber-600 text-white hover:bg-amber-600/90"
                : tone === "slate"
                  ? "bg-slate-700 text-white hover:bg-slate-700/90"
                  : "";

  const toneInactive =
    tone === "cyan"
      ? "border-cyan-200 bg-cyan-50 text-cyan-700 hover:bg-cyan-100"
      : tone === "violet"
        ? "border-violet-200 bg-violet-50 text-violet-700 hover:bg-violet-100"
        : tone === "indigo"
          ? "border-indigo-200 bg-indigo-50 text-indigo-700 hover:bg-indigo-100"
          : tone === "green"
            ? "border-green-200 bg-green-50 text-green-700 hover:bg-green-100"
            : tone === "sky"
              ? "border-sky-200 bg-sky-50 text-sky-700 hover:bg-sky-100"
              : tone === "amber"
                ? "border-amber-200 bg-amber-50 text-amber-700 hover:bg-amber-100"
                : tone === "slate"
                  ? "border-slate-200 bg-slate-50 text-slate-700 hover:bg-slate-100"
                  : "";

  const labelMap = useMemo(() => {
    const m = new Map<string, string>();
    for (const o of options ?? []) {
      const key = String(o.label ?? "").trim();
      if (!key) continue;
      const display = isZh ? key : String(o.label_en ?? "").trim() || key;
      m.set(key, display);
    }
    return m;
  }, [isZh, options]);

  function displayLabel(key: string): string {
    const k = String(key ?? "").trim();
    if (!k) return "";
    return String(labelMap.get(k) ?? k).trim();
  }

  const filtered = useMemo(() => {
    const term = q.trim().toLowerCase();
    if (!term) return options;
    return options.filter((o) => {
      const parts = [
        o.label ?? "",
        o.label_en ?? "",
        o.category ?? "",
        o.category_en ?? "",
        ...(o.keywords ?? []),
        ...(o.keywords_en ?? []),
      ];
      const hay = parts.filter(Boolean).join(" ").toLowerCase();
      return hay.includes(term);
    });
  }, [options, q]);

  const selectedSet = useMemo(() => new Set(selected.map((x) => x.trim()).filter(Boolean)), [selected]);

  function toggle(label: string) {
    const value = label.trim();
    if (!value) return;
    const next = new Set(selectedSet);
    if (next.has(value)) next.delete(value);
    else {
      if (next.size >= maxSelected) return;
      next.add(value);
    }
    onChange(Array.from(next.values()));
  }

  return (
    <Card className={`mt-2 ${toneCard}`}>
      <CardContent className="pt-4">
        <div className="text-sm font-medium">{title}</div>
        {selected.length ? (
          <div className="mt-2 flex flex-wrap gap-1">
            {selected.map((v) => (
              <Badge key={v} variant="secondary" className={`cursor-pointer ${toneInactive}`} onClick={() => toggle(v)}>
                {displayLabel(v)}
              </Badge>
            ))}
          </div>
        ) : (
          <div className="mt-2 text-xs text-muted-foreground">
            {required
              ? t({ zh: "未选择（必填）", en: "No selection (required)" })
              : t({ zh: "暂未选择（可跳过）", en: "Optional — you can skip" })}
          </div>
        )}

        <div className="mt-3">
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t({ zh: "搜索…", en: "Search…" })} />
        </div>

        <div className="mt-3 max-h-[34vh] overflow-y-auto rounded-md border p-2">
          <div className="flex flex-wrap gap-2">
            {filtered.slice(0, 200).map((o) => {
              const valueLabel = String(o.label ?? "").trim();
              const active = selectedSet.has(valueLabel);
              return (
                <Button
                  key={o.id}
                  size="sm"
                  variant={active ? "secondary" : "outline"}
                  className={active ? toneActive : toneInactive}
                  onClick={() => toggle(valueLabel)}
                >
                  {displayLabel(valueLabel)}
                </Button>
              );
            })}
          </div>
        </div>
        <div className="mt-2 text-xs text-muted-foreground">
          {t({ zh: `已选 ${selected.length}/${maxSelected}`, en: `${selected.length}/${maxSelected} selected` })}
        </div>
      </CardContent>
    </Card>
  );
}
