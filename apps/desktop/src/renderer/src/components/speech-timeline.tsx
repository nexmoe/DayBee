import { memo, useState, type ComponentProps, type ReactNode } from "react";
import { SpeechActivity } from "@/components/speech-activity";
import { TranscriptionTimeline } from "@/components/transcription-timeline";
import { useScrollAreaViewport } from "@/components/ui/legacy-scroll-area";

// Both windows share the date selection and timeline filtering behavior.
type SpeechTimelineProps = Omit<ComponentProps<typeof TranscriptionTimeline>, "dateKey"> & {
  children?: (content: { activity: ReactNode; timeline: ReactNode }) => ReactNode;
};

export const SpeechTimeline = memo(function SpeechTimeline({ items, microphoneActivity, language, active = true, children, ...timelineProps }: SpeechTimelineProps) {
  const [date, setDate] = useState<string | null>(null);
  const viewport = useScrollAreaViewport();
  const activity = active ? <SpeechActivity items={items} microphoneActivity={microphoneActivity.items} language={language}
    value={date} appId={timelineProps.appId} onChange={next => { setDate(next); viewport?.scrollTo(0, 0); }} /> : null;
  const timeline = <TranscriptionTimeline key={JSON.stringify([timelineProps.query, timelineProps.appId, date])}
    {...timelineProps} items={items} microphoneActivity={microphoneActivity} language={language} dateKey={date} active={active} />;
  return children ? children({ activity, timeline }) : <>{activity}{timeline}</>;
});
