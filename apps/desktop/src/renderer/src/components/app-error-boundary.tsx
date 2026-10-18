import { Component, type ErrorInfo, type ReactNode } from "react";
import { getDesktopSnapshot } from "@/lib/desktop-store";
import { createT } from "@/lib/i18n";

type BoundaryState = { hasError: boolean };

export class AppErrorBoundary extends Component<{ children: ReactNode }, BoundaryState> {
  state: BoundaryState = { hasError: false };

  static getDerivedStateFromError(): BoundaryState {
    return { hasError: true };
  }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    console.error(error, info.componentStack);
  }

  render() {
    if (!this.state.hasError) return this.props.children;
    const t = createT(getDesktopSnapshot().settings.desktop.language);
    return (
      <main role="alert" className="grid min-h-screen content-center justify-items-start gap-4 p-8 text-[color:var(--foreground)]">
        <h1>{t("interfaceLoadFailed")}</h1>
        <button type="button" onClick={() => window.location.reload()}>{t("interfaceReload")}</button>
      </main>
    );
  }
}
