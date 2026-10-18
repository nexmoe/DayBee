import { memo, useState, type ComponentProps, type ReactNode } from "react";
import { SpeechActivity } from "@/components/speech-activity";
import { TranscriptionTimeline } from "@/components/transcription-timeline";
import { useScrollAreaViewport } from "@/components/ui/legacy-scroll-area";

// The activity chart and timeline share date selection and filtering.
type SpeechTimelineProps = Omit<ComponentProps<typeof TranscriptionTimeline>, "dateKey"> & {
  children?: (content: { activity: ReactNode; timeline: ReactNode }) => ReactNode;
};

export const SpeechTimeline = memo(function SpeechTimeline({ items, microphoneActivity, language, active = true, children }: SpeechTimelineProps) {
  const [date, setDate] = useState<string | null>(null);
  const viewport = useScrollAreaViewport();
  const activity = active ? <SpeechActivity items={items} language={language}
    value={date} onChange={next => { setDate(next); viewport?.scrollTo(0, 0); }} /> : null;
  const timeline = <TranscriptionTimeline key={date}
    items={items} microphoneActivity={microphoneActivity} language={language} dateKey={date} active={active} />;
  return children ? children({ activity, timeline }) : <>{activity}{timeline}</>;
});
