import { CircleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { desktopActions, useDesktopSelector } from "@/lib/desktop-store";
import { createT } from "@/lib/i18n";
import { typeClass } from "@/lib/size-context";
import { fontWeights } from "@/lib/font-weight";
import { voiceprintErrorCopy, voiceprintQualityCopy } from "@/lib/voiceprint-copy";

const voiceprintSampleSeconds = 8;
const voiceprintVerifySeconds = 4;
const voiceprintVerifyStep = 3;
// The window canvas is transparent; primary labels need an opaque surface.
const primaryActionClass = "text-surface-1 [--background:var(--surface-1)]";

const sameVoiceprint = <T extends object>(left: T, right: T): boolean => {
  if (Object.is(left, right)) return true;
  const keys = Object.keys(left) as (keyof T)[];
  if (keys.length !== Object.keys(right).length) return false;
  return keys.every((key) => Object.is(left[key], right[key]));
};

export function VoiceprintGuide() {
  const language = useDesktopSelector((current) => current.settings.desktop.language);
  const pendingSetup = useDesktopSelector((current) => current.settings.desktop.voiceprintSetup === "pending");
  const voice = useDesktopSelector((current) => current.voiceprint, sameVoiceprint);
  const t = createT(language);
  const phase = voice.phase;
  const intro = phase === "idle" || phase === "ready";
  const pending = pendingSetup;
  const speaking = phase === "recording" || phase === "sample-ready";
  const prompts = [t("voiceprintPromptA"), t("voiceprintPromptB"), t("voiceprintPromptC"), t("voiceprintPromptVerify")];
  const prompt = prompts[Math.min(Math.max(voice.step, 0), prompts.length - 1)];
  const target = voice.targetSeconds > 0 ? voice.targetSeconds : voice.step >= voiceprintVerifyStep ? voiceprintVerifySeconds : voiceprintSampleSeconds;
  const ratio = target > 0 ? Math.min(1, Math.max(0, voice.speechSeconds / target)) : 0;
  const modelProgress = voice.progress === null ? null : Math.min(1, Math.max(0, voice.progress > 1 ? voice.progress / 100 : voice.progress));
  const error = voice.error ? voiceprintErrorCopy(voice.error, t, target) : null;
  const quality = voiceprintQualityCopy(voice.message, t);
  const ambient = quality?.kind === "ambient";
  const paused = quality?.kind === "paused";
  const cancel = () => {
    void desktopActions.cancelVoiceprintEnrollment();
  };
  const skip = () => {
    void desktopActions.skipVoiceprintSetup();
  };

  if (voice.enrolled && intro) return null;

  return (
    <section className="mb-4 space-y-3 rounded-2xl border border-[color:var(--border)] bg-[color:var(--surface)] px-4 py-4" data-testid="voiceprint-guide" aria-labelledby="voiceprint-guide-title">
      <h2 id="voiceprint-guide-title" className={typeClass("subtitle")} style={{ fontVariationSettings: fontWeights.semibold }}>{t("voiceprintGuideTitle")}</h2>
      <p data-testid="voiceprint-description" className={`${typeClass("body")} text-[color:var(--muted)]`}>
        {t(intro ? voice.enrolled ? "voiceprintEnrolledDescription" : "voiceprintIntroDescription" : "voiceprintQuiet")}
      </p>
      {intro && (
        <div className="flex flex-wrap gap-2">
          <Button size="compact" className={voice.enrolled ? undefined : primaryActionClass} variant={voice.enrolled ? "secondary" : "primary"} onClick={() => void desktopActions.startVoiceprintEnrollment()}>{t(voice.enrolled ? "voiceprintRerecord" : "voiceprintStart")}</Button>
          {pending ? (
            <Button size="compact" variant="ghost" onClick={skip}>{t("voiceprintSkip")}</Button>
          ) : (
            <Button size="compact" variant="ghost" onClick={cancel}>{t("voiceprintCancel")}</Button>
          )}
        </div>
      )}
      {!intro && (
        <div className="space-y-4 border-t border-[color:var(--border)] pt-4">
          <p className={typeClass("body")} style={{ fontVariationSettings: fontWeights.semibold }}>{voice.step >= voiceprintVerifyStep ? t("voiceprintStepVerify") : t("voiceprintStepSample", { step: voice.step + 1 })}</p>
          {error && (
            <div className="flex items-start gap-2" role="alert">
              <CircleAlert aria-hidden className="mt-0.5 size-4 shrink-0 text-[color:var(--foreground)]" />
              <div className={`min-w-0 space-y-1 ${typeClass("body")}`}>
                <p style={{ fontVariationSettings: fontWeights.semibold }}>{t("voiceprintErrorTitle")}</p>
                <p>{error.message} {voice.enrolled && t("voiceprintKeepExisting")}</p>
                {error.detail && <details><summary className="cursor-pointer text-[color:var(--muted)]">{t("voiceprintErrorDetails")}</summary><p className="mt-2 break-words">{error.detail}</p></details>}
              </div>
            </div>
          )}
          {speaking && ambient && quality && (
            <p role="status" className={typeClass("body")} style={{ fontVariationSettings: fontWeights.semibold }}>{quality.text}</p>
          )}
          {speaking && paused && quality && (
            <p role="alert" className={typeClass("body")}>{quality.text}</p>
          )}
          {speaking && !ambient && !paused && (
            <div className="space-y-2" data-testid="voiceprint-reading">
              <p role="status" className={typeClass("body")} style={{ fontVariationSettings: fontWeights.semibold }}>
                {t(phase === "sample-ready" ? "voiceprintSampleReady" : "voiceprintReadAloud")}
              </p>
              <blockquote className={`${typeClass("title")} m-0 max-w-2xl leading-relaxed`} data-testid="voiceprint-passage">{prompt}</blockquote>
              {phase === "recording" && <p className={`${typeClass("body")} text-[color:var(--muted)]`}>{t("voiceprintReadHint")}</p>}
              {quality?.kind === "reference" && <p data-testid="voiceprint-quality" className={`${typeClass("body")} text-[color:var(--muted)]`}>{quality.text}</p>}
            </div>
          )}
          {(phase === "preparing" || phase === "processing") && (
            <p role="status" className={typeClass("body")}>{t(phase === "preparing" ? "voiceprintPreparing" : voice.step >= voiceprintVerifyStep ? "voiceprintVerifying" : "voiceprintProcessing")}</p>
          )}
          {modelProgress !== null && (phase === "preparing" || phase === "processing") && (
            <div className="h-1 overflow-hidden rounded-full bg-[color:var(--border)]" role="progressbar" aria-label={t("voiceprintPreparing")} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(modelProgress * 100)}>
              <div className="h-full origin-left bg-[color:var(--accent)]" style={{ transform: `scaleX(${modelProgress})` }} />
            </div>
          )}
          {speaking && (
            <div className="max-w-2xl space-y-2">
              <p className={`${typeClass("body")} tabular-nums`}>{t("voiceprintSpeechSeconds", { speech: voice.speechSeconds.toFixed(1), target: String(Math.round(target)) })}</p>
              <div className="h-1 overflow-hidden rounded-full bg-[color:var(--border)]" role="progressbar" aria-label={t("voiceprintSpeechProgress")} aria-valuemin={0} aria-valuemax={target} aria-valuenow={voice.speechSeconds}>
                <div className="h-full origin-left bg-[color:var(--accent)]" style={{ transform: `scaleX(${ratio})` }} />
              </div>
              {phase === "recording" && <div className={`flex items-center gap-3 ${typeClass("caption")} text-[color:var(--muted)]`}>
                <span>{t("metricLevel")}</span>
                <div className="h-1 w-24 overflow-hidden rounded-full bg-[color:var(--border)]" role="meter" aria-label={t("metricLevel")} aria-valuemin={0} aria-valuemax={1} aria-valuenow={Math.min(1, Math.max(0, voice.level))}>
                  <div className="h-full origin-left bg-[color:var(--muted)]" style={{ transform: `scaleX(${Math.min(1, Math.max(0, voice.level))})` }} />
                </div>
              </div>}
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            {phase === "sample-ready" && <Button size="compact" className={primaryActionClass} onClick={() => void desktopActions.advanceVoiceprintEnrollment()}>{t(voice.step === voiceprintVerifyStep - 1 ? "voiceprintStartVerify" : "voiceprintContinue")}</Button>}
            {phase === "error" && <Button size="compact" className={primaryActionClass} onClick={() => void desktopActions.startVoiceprintEnrollment()}>{t("voiceprintRetry")}</Button>}
            {phase === "error" && !voice.enrolled
              ? <Button size="compact" variant="ghost" onClick={skip}>{t("voiceprintSkip")}</Button>
              : <Button size="compact" variant="ghost" onClick={cancel}>{t("voiceprintCancel")}</Button>}
          </div>
        </div>
      )}
    </section>
  );
}
