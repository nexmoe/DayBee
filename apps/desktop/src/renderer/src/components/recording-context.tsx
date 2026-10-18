import { useState } from "react";
import { AppWindow, Mic } from "lucide-react";
import type { AppLanguage, MicrophoneActivitySnapshot } from "@daybee/shared";
import { availableRecordingContexts, type RecordingContext } from "@/lib/recording-context";
import { createT } from "@/lib/i18n";
import { typeClass } from "@/lib/size-context";

export function ApplicationIcon({ app }: { app: RecordingContext }) {
  const [failedSource, setFailedSource] = useState("");
  return app.appIcon.startsWith("data:image/png;base64,") && failedSource !== app.appIcon ?
    <img src={app.appIcon} alt="" width={18} height={18} onError={() => setFailedSource(app.appIcon)} /> :
    <AppWindow size={18} aria-hidden="true" className="recording-context-fallback" />;
}

export function RecordingContexts({ apps, language, onSelect, label, selectableAppIds }: {
  apps: RecordingContext[]; language: AppLanguage; onSelect?: (appId: string) => void; label?: string;
  selectableAppIds?: ReadonlySet<string>;
}) {
  const t = createT(language);
  if (apps.length === 0) return null;
  return <aside aria-label={label ?? t("recordingContextLabel")} className={`recording-contexts ${typeClass("caption")}`}>
    {apps.map(app => onSelect && (!selectableAppIds || selectableAppIds.has(app.appId)) ?
      <button type="button" key={app.appId} className="recording-context" aria-label={t("filterRecordingApp", { app: app.appName })}
        title={app.appName} onClick={() => onSelect(app.appId)}>
        <ApplicationIcon app={app} /><span>{app.appName}</span>
      </button> :
      <span key={app.appId} className="recording-context" title={app.appName}>
        <ApplicationIcon app={app} /><span>{app.appName}</span>
      </span>)}
  </aside>;
}

export function ActiveMicrophoneApps({ activity, language, onSelect, selectableAppIds }: {
  activity: MicrophoneActivitySnapshot; language: AppLanguage; onSelect?: (appId: string) => void;
  selectableAppIds?: ReadonlySet<string>;
}) {
  const t = createT(language);
  const apps = availableRecordingContexts(activity.items.filter(item => item.endedAt === null));
  if (!activity.supported || activity.error || apps.length === 0) return null;
  return <div className="active-microphone-apps" role="status" aria-label={t("activeMicrophoneApps")}>
    <p className={`active-microphone-label ${typeClass("caption")}`}>
      <Mic size={14} aria-hidden="true" />{t("activeMicrophoneApps")}
    </p>
    <RecordingContexts apps={apps} language={language} onSelect={onSelect} selectableAppIds={selectableAppIds} label={t("activeMicrophoneApps")} />
  </div>;
}

export function RecordingContextFilter({ apps, value, onChange, language }: {
  apps: RecordingContext[]; value: string; onChange: (appId: string) => void; language: AppLanguage;
}) {
  const t = createT(language);
  const selected = apps.find(app => app.appId === value);
  if (apps.length === 0 && !value) return null;
  return <label className="recording-context-filter">
    {selected && <ApplicationIcon app={selected} />}
    <select aria-label={t("filterRecordingContext")} value={value} onChange={event => onChange(event.target.value)}>
      <option value="">{t("allRecordingContexts")}</option>
      {apps.map(app => <option key={app.appId} value={app.appId}>{app.appName}</option>)}
    </select>
  </label>;
}
