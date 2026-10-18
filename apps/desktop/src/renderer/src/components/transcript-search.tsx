import { Search, X } from "lucide-react";
import type { AppLanguage } from "@daybee/shared";
import { Button } from "@/components/ui/legacy-button";
import { createT } from "@/lib/i18n";

export function TranscriptSearch({ query, onChange, language }: {
  query: string; onChange: (query: string) => void; language: AppLanguage;
}) {
  const t = createT(language);
  return (
    <form role="search" className="transcript-search" onSubmit={event => event.preventDefault()}>
      <Search size={14} aria-hidden="true" />
      <input
        type="search" value={query} aria-label={t("searchTranscripts")} placeholder={t("searchTranscripts")}
        onChange={event => onChange(event.target.value)}
        onKeyDown={event => {
          if (event.key === "Escape" && query) {
            event.preventDefault();
            event.stopPropagation();
            onChange("");
          }
        }}
      />
      {query && <Button type="button" size="sm" variant="ghost" className="h-6 w-6 shrink-0 p-0"
        aria-label={t("clearTranscriptSearch")} onClick={() => onChange("")}>
        <X size={12} />
      </Button>}
    </form>
  );
}
