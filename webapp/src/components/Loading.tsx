import { Loader2 } from "lucide-react";

export function Loading() {
  return (
    <div className="flex min-h-[50vh] flex-col items-center justify-center p-8 text-muted-foreground">
      <Loader2 className="h-8 w-8 animate-spin" />
      <span className="mt-2 text-sm">加载中...</span>
    </div>
  );
}
