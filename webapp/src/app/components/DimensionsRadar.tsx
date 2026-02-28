import { useMemo } from "react";

import { useI18n } from "@/lib/i18n";

export const DIMENSIONS = [
  { key: "perspective", label_zh: "视角", label_en: "Perspective" },
  { key: "taste", label_zh: "品味", label_en: "Taste" },
  { key: "care", label_zh: "关怀", label_en: "Care" },
  { key: "trajectory", label_zh: "轨迹", label_en: "Trajectory" },
  { key: "persuasion", label_zh: "说服", label_en: "Persuasion" },
] as const;

export type DimensionKey = (typeof DIMENSIONS)[number]["key"];

export function DimensionsRadar({ scores }: { scores: Partial<Record<DimensionKey, number>> | undefined }) {
  const { isZh } = useI18n();

  const points = useMemo(() => {
    const cx = 110;
    const cy = 110;
    const r = 80;
    const n = DIMENSIONS.length;
    const pts: Array<[number, number]> = [];
    for (let i = 0; i < n; i++) {
      const { key } = DIMENSIONS[i];
      const v = Math.max(0, Math.min(100, Number(scores?.[key] ?? 0)));
      const a = (Math.PI * 2 * i) / n - Math.PI / 2;
      const rr = (r * v) / 100;
      pts.push([cx + Math.cos(a) * rr, cy + Math.sin(a) * rr]);
    }
    return { cx, cy, r, pts };
  }, [scores]);

  const poly = points.pts.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(" ");

  function gridPolygon(scale: number) {
    const cx = points.cx;
    const cy = points.cy;
    const r = points.r * scale;
    const n = DIMENSIONS.length;
    const pts: string[] = [];
    for (let i = 0; i < n; i++) {
      const a = (Math.PI * 2 * i) / n - Math.PI / 2;
      pts.push(`${(cx + Math.cos(a) * r).toFixed(1)},${(cy + Math.sin(a) * r).toFixed(1)}`);
    }
    return pts.join(" ");
  }

  return (
    <div className="w-full">
      <svg viewBox="0 0 220 220" className="w-full">
        <polygon points={gridPolygon(1)} fill="none" stroke="currentColor" opacity="0.12" />
        <polygon points={gridPolygon(0.66)} fill="none" stroke="currentColor" opacity="0.08" />
        <polygon points={gridPolygon(0.33)} fill="none" stroke="currentColor" opacity="0.06" />

        {DIMENSIONS.map((d, i) => {
          const a = (Math.PI * 2 * i) / DIMENSIONS.length - Math.PI / 2;
          const x = points.cx + Math.cos(a) * points.r;
          const y = points.cy + Math.sin(a) * points.r;
          return <line key={d.key} x1={points.cx} y1={points.cy} x2={x} y2={y} stroke="currentColor" opacity="0.08" />;
        })}

        <polygon points={poly} fill="currentColor" opacity="0.12" />
        <polygon points={poly} fill="none" stroke="currentColor" opacity="0.35" />

        {DIMENSIONS.map((d, i) => {
          const a = (Math.PI * 2 * i) / DIMENSIONS.length - Math.PI / 2;
          const x = points.cx + Math.cos(a) * (points.r + 18);
          const y = points.cy + Math.sin(a) * (points.r + 18);
          return (
            <text
              key={d.key}
              x={x}
              y={y}
              textAnchor="middle"
              dominantBaseline="middle"
              fontSize="11"
              fill="currentColor"
              opacity="0.75"
            >
              {isZh ? d.label_zh : d.label_en}
            </text>
          );
        })}
      </svg>
    </div>
  );
}

