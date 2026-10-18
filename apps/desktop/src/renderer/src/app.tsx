import { useDesktopSelector } from "@/lib/desktop-store";
import { lazy, Suspense, useEffect } from "react";
import { createT } from "@/lib/i18n";
import { useWindowTheme } from "@/lib/use-window-theme";

const CompactWindow = lazy(() => import("@/components/compact-window").then(module => ({ default: module.CompactWindow })));

export function App() {
  const language = useDesktopSelector((current) => current.settings.desktop.language);
  const theme = useDesktopSelector((current) => current.settings.desktop.theme);
  const appName = useDesktopSelector((current) => current.app.name);
  const t = createT(language);
  useWindowTheme(theme);
  useEffect(() => {
    document.title = appName;
  }, [appName]);

  return <Suspense fallback={<main className="h-screen content-center p-8" aria-busy="true" role="status">{t("interfaceLoading")}</main>}><CompactWindow /></Suspense>;
}
