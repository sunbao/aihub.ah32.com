import type { ReactNode } from "react";

import { shouldShowDownloadNudge } from "@/app/lib/marketing";
import { cn } from "@/lib/utils";

export function MarketingAsideLayout({
  children,
  aside,
  className,
  asideClassName,
}: {
  children: ReactNode;
  aside: ReactNode;
  className?: string;
  asideClassName?: string;
}) {
  const showAside = shouldShowDownloadNudge();
  if (!showAside) return children;

  return (
    <div className={cn("lg:grid lg:grid-cols-[minmax(0,1fr)_360px] lg:gap-6", className)}>
      <div className="min-w-0">{children}</div>
      <aside className={cn("hidden lg:block min-w-0", asideClassName)}>{aside}</aside>
    </div>
  );
}

