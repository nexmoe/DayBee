import { useEffect, useState } from "react";
import { Copy } from "lucide-react";
import { Button } from "@/components/ui/legacy-button";
import { SettingField } from "@/components/setting-field";
import { desktopActions } from "@/lib/desktop-store";
import type { createT } from "@/lib/i18n";

export function MCPSettings({ t }: { t: ReturnType<typeof createT> }) {
  const [configuration, setConfiguration] = useState("");
  const [error, setError] = useState(false);
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    let active = true;
    void desktopActions.getMCPConfig().then(value => {
      if (active) setConfiguration(value);
    }).catch(() => { if (active) setError(true); });
    return () => { active = false; };
  }, []);

  return <SettingField title={t("mcpTitle")} description={t("mcpDescription")} layout="stacked" testId="mcp-settings"
    control={<div className="min-w-0 space-y-3">
      <p className="text-xs leading-5 text-[color:var(--muted)]">{t("mcpCapabilities")}</p>
      <Button size="sm" variant="subtle" disabled={!configuration} onClick={() => {
        void desktopActions.copyMCPConfig().then(() => setCopied(true)).catch(() => setCopied(false));
      }}><Copy aria-hidden className="h-3.5 w-3.5" />{t(copied ? "mcpCopied" : "mcpCopyConfiguration")}</Button>
      {copied && <p className="sr-only" role="status">{t("mcpCopied")}</p>}
      {error && <p role="alert" className="text-xs leading-5">{t("mcpConfigFailed")}</p>}
      {!configuration && !error && <p role="status" className="text-xs leading-5">{t("interfaceLoading")}</p>}
      {configuration && <details>
        <summary className="cursor-pointer text-xs text-[color:var(--muted)]">{t("mcpShowConfiguration")}</summary>
        <textarea aria-label={t("mcpConfigurationLabel")} readOnly value={configuration} spellCheck={false}
          className="mt-2 block min-h-48 w-full min-w-0 resize-y rounded-lg border border-[color:var(--border)] bg-[color:var(--surface)] p-3 text-xs leading-5 text-[color:var(--foreground)]" />
      </details>}
    </div>} />;
}
