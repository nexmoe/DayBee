import { useState } from "react";
import { CircleArrowUp, LoaderCircle } from "lucide-react";
import { motion } from "framer-motion";
import type { AutoUpdateSnapshot } from "@daybee/shared";
import { Button } from "@/components/ui/legacy-button";
import type { createT } from "@/lib/i18n";
import { spring } from "@/lib/springs";

export function UpdateButton({ updater, onInstall, t }: {
  updater: AutoUpdateSnapshot;
  onInstall: () => Promise<void>;
  t: ReturnType<typeof createT>;
}) {
  const [pending, setPending] = useState(false);
  const restarting = updater.phase === "downloaded";
  const busy = pending || updater.phase === "downloading" || restarting;
  const available = updater.phase === "available" ||
    (updater.phase === "error" && updater.latestVersion !== null);
  if (!available && !busy) return null;

  const install = async () => {
    if (busy) return;
    setPending(true);
    try {
      await onInstall();
    } catch {
      // The desktop action displays the failure in a toast; keep the button retryable.
    } finally {
      setPending(false);
    }
  };

  return (
    <Button
      className="update-button w-8 shrink-0 px-0"
      size="sm"
      variant="ghost"
      aria-label={t(restarting ? "updateRestarting" : busy ? "updateInstalling" : "updateAvailable")}
      aria-busy={busy}
      disabled={busy}
      title={busy
        ? t(restarting ? "updateRestarting" : "updateInstalling")
        : `${t("updateAvailable")} · ${t("installUpdateTitle", { version: updater.latestVersion ?? updater.currentVersion })}`}
      onClick={() => void install()}
    >
      <span aria-hidden="true" className="grid h-4 w-4">
        <motion.span className="col-start-1 row-start-1" initial={false} animate={{ opacity: busy ? 0 : 1 }} transition={spring.fast}>
          <CircleArrowUp className="h-4 w-4" strokeWidth={2.25} />
        </motion.span>
        <motion.span className="col-start-1 row-start-1" initial={false} animate={{ opacity: busy ? 1 : 0 }} transition={spring.fast}>
          <LoaderCircle className={busy ? "h-4 w-4 motion-safe:animate-spin" : "h-4 w-4"} strokeWidth={2.25} />
        </motion.span>
      </span>
    </Button>
  );
}
