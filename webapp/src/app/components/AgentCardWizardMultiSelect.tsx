import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useI18n } from "@/lib/i18n";

import type { CatalogLabeledItem } from "@/app/lib/agentCardCatalogs";

export type AgentCardWizardTone = "cyan" | "violet" | "indigo" | "green" | "sky" | "amber" | "slate";

export function toneClasses(tone?: AgentCardWizardTone): { card: string; active: string; inactive: string } {
  const card =
    tone === "cyan"
      ? "border-l-4 border-cyan-500/40 dark:border-cyan-300/30"
      : tone === "violet"
        ? "border-l-4 border-violet-500/40 dark:border-violet-300/30"
        : tone === "indigo"
          ? "border-l-4 border-indigo-500/40 dark:border-indigo-300/30"
          : tone === "green"
            ? "border-l-4 border-green-500/40 dark:border-green-300/30"
            : tone === "sky"
              ? "border-l-4 border-sky-500/40 dark:border-sky-300/30"
              : tone === "amber"
                ? "border-l-4 border-amber-500/40 dark:border-amber-300/30"
                : tone === "slate"
                  ? "border-l-4 border-slate-500/40 dark:border-slate-300/30"
                  : "";

  const active =
    tone === "cyan"
      ? "border border-cyan-500/35 bg-cyan-500/15 text-cyan-950 hover:bg-cyan-500/20 dark:border-cyan-200/30 dark:bg-cyan-200/15 dark:text-cyan-50 dark:hover:bg-cyan-200/20"
      : tone === "violet"
        ? "border border-violet-500/35 bg-violet-500/15 text-violet-950 hover:bg-violet-500/20 dark:border-violet-200/30 dark:bg-violet-200/15 dark:text-violet-50 dark:hover:bg-violet-200/20"
        : tone === "indigo"
          ? "border border-indigo-500/35 bg-indigo-500/15 text-indigo-950 hover:bg-indigo-500/20 dark:border-indigo-200/30 dark:bg-indigo-200/15 dark:text-indigo-50 dark:hover:bg-indigo-200/20"
          : tone === "green"
            ? "border border-green-500/35 bg-green-500/15 text-green-950 hover:bg-green-500/20 dark:border-green-200/30 dark:bg-green-200/15 dark:text-green-50 dark:hover:bg-green-200/20"
            : tone === "sky"
              ? "border border-sky-500/35 bg-sky-500/15 text-sky-950 hover:bg-sky-500/20 dark:border-sky-200/30 dark:bg-sky-200/15 dark:text-sky-50 dark:hover:bg-sky-200/20"
              : tone === "amber"
                ? "border border-amber-500/35 bg-amber-500/15 text-amber-950 hover:bg-amber-500/20 dark:border-amber-200/30 dark:bg-amber-200/15 dark:text-amber-50 dark:hover:bg-amber-200/20"
                : tone === "slate"
                  ? "border border-slate-500/35 bg-slate-500/15 text-slate-950 hover:bg-slate-500/20 dark:border-slate-200/30 dark:bg-slate-200/15 dark:text-slate-50 dark:hover:bg-slate-200/20"
                  : "";

  const inactive =
    tone === "cyan"
      ? "border border-cyan-500/20 bg-cyan-500/5 text-cyan-950 hover:bg-cyan-500/10 dark:border-cyan-300/25 dark:bg-cyan-300/10 dark:text-cyan-50 dark:hover:bg-cyan-300/15"
      : tone === "violet"
        ? "border border-violet-500/20 bg-violet-500/5 text-violet-950 hover:bg-violet-500/10 dark:border-violet-300/25 dark:bg-violet-300/10 dark:text-violet-50 dark:hover:bg-violet-300/15"
        : tone === "indigo"
          ? "border border-indigo-500/20 bg-indigo-500/5 text-indigo-950 hover:bg-indigo-500/10 dark:border-indigo-300/25 dark:bg-indigo-300/10 dark:text-indigo-50 dark:hover:bg-indigo-300/15"
          : tone === "green"
            ? "border border-green-500/20 bg-green-500/5 text-green-950 hover:bg-green-500/10 dark:border-green-300/25 dark:bg-green-300/10 dark:text-green-50 dark:hover:bg-green-300/15"
            : tone === "sky"
              ? "border border-sky-500/20 bg-sky-500/5 text-sky-950 hover:bg-sky-500/10 dark:border-sky-300/25 dark:bg-sky-300/10 dark:text-sky-50 dark:hover:bg-sky-300/15"
              : tone === "amber"
                ? "border border-amber-500/20 bg-amber-500/5 text-amber-950 hover:bg-amber-500/10 dark:border-amber-300/25 dark:bg-amber-300/10 dark:text-amber-50 dark:hover:bg-amber-300/15"
                : tone === "slate"
                  ? "border border-slate-500/20 bg-slate-500/5 text-slate-950 hover:bg-slate-500/10 dark:border-slate-300/25 dark:bg-slate-300/10 dark:text-slate-50 dark:hover:bg-slate-300/15"
                  : "";

  return { card, active, inactive };
}

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

  const styles = useMemo(() => toneClasses(tone), [tone]);

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
    <Card className={`mt-2 ${styles.card}`}>
      <CardContent className="pt-4">
        <div className="text-sm font-medium">{title}</div>
        {selected.length ? (
          <div className="mt-2 flex flex-wrap gap-1">
            {selected.map((v) => (
              <Badge key={v} variant="secondary" className={`cursor-pointer ${styles.active}`} onClick={() => toggle(v)}>
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
                  className={active ? styles.active : styles.inactive}
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
