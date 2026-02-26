import { Component, ErrorInfo, ReactNode } from "react";
import { Button } from "@/components/ui/button";

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error?: Error;
}

export class ErrorBoundary extends Component<Props, State> {
  public state: State = {
    hasError: false,
  };

  public static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("Uncaught error:", error, errorInfo);
  }

  public render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-[50vh] flex-col items-center justify-center p-4 text-center">
          <h2 className="mb-2 text-lg font-semibold">出错了</h2>
          <p className="mb-4 text-sm text-muted-foreground">
            抱歉，应用程序遇到了一些问题。
          </p>
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={() => window.location.reload()}
            >
              刷新页面
            </Button>
            <Button
                variant="default"
                onClick={() => this.setState({ hasError: false })}
            >
                重试
            </Button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
