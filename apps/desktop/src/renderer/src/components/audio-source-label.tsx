import { Mic, Volume2 } from "lucide-react";
import type { AppLanguage, AudioSource } from "@daybee/shared";
import { createT } from "@/lib/i18n";

export function AudioSourceLabel({ source, language }: { source: AudioSource; language: AppLanguage }) {
  const system = source === "system";
  const Icon = system ? Volume2 : Mic;
  return (
    <span className="audio-source-label inline-flex shrink-0 items-center gap-1 text-xs text-[color:var(--muted)]" data-testid="audio-source-label">
      <Icon aria-hidden="true" className="h-3 w-3" />
      {createT(language)(system ? "audioSourceSystem" : "audioSourceMicrophone")}
    </span>
  );
}
