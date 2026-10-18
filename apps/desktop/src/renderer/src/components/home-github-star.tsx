import { Github, Star } from "lucide-react";
import type { DesktopSnapshot } from "@daybee/shared";
import { Button } from "@/components/ui/legacy-button";
import { createT } from "@/lib/i18n";
import { desktopActions } from "@/lib/desktop-store";

export function HomeGithubStar({ snapshot }: { snapshot: DesktopSnapshot }) {
  const t = createT(snapshot.settings.desktop.language);

  return (
    <aside className="home-github-star">
      <div className="home-github-star__icon">
        <Github className="h-3.5 w-3.5" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="home-github-star__label">{t("githubStarTitle")}</p>
        <p className="home-github-star__hint">{t("githubStarDescription")}</p>
      </div>
      <Button
        className="shrink-0"
        size="sm"
        variant="subtle"
        onClick={() => void desktopActions.openExternal(snapshot.app.repositoryUrl)}
      >
        <Star className="h-3.5 w-3.5" />
        {t("githubStarAction")}
      </Button>
    </aside>
  );
}
