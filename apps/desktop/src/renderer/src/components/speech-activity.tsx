import { useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { CalendarDays, ChevronLeft, ChevronRight, RotateCcw } from "lucide-react";
import type { AppLanguage, RecordingHistoryItem } from "@daybee/shared";
import { FluidHoverHighlight } from "@/components/fluid-hover-highlight";
import { StackedSpeechBar } from "@/components/speech-activity-chart";
import { Button } from "@/components/ui/button";
import { Tooltip } from "@/components/ui/tooltip";
import { useFluidHover, useRegisterFluidHoverItem, type UseFluidHoverReturn } from "@/hooks/use-fluid-hover";
import { useLocalToday } from "@/hooks/use-local-today";
import { createT, resolveIntlLocale } from "@/lib/i18n";
import { fontWeights } from "@/lib/font-weight";
import { typeClass } from "@/lib/size-context";
import {
  buildSpeechActivity, dateFromKey, emptySpeechCount, shiftDate,
  totalSpeechCount, recentDateKeys, localDateKey, type SpeechCount
} from "@/lib/speech-activity";
import "@/speech-activity.css";
import "@fontsource-variable/inter/opsz.css";

export function SpeechActivity({ items, language, value, onChange }: {
  items: RecordingHistoryItem[]; language: AppLanguage;
  value: string | null; onChange: (date: string | null) => void;
}) {
  const t = createT(language);
  const locale = resolveIntlLocale(language);
  const today = useLocalToday();
  const [anchor, setAnchor] = useState<string | null>(value);
  const endingOn = anchor ?? today;
  const dates = useMemo(() => recentDateKeys(endingOn), [endingOn]);
  const activity = useMemo(() => buildSpeechActivity(items), [items]);
  const countFormat = useMemo(() => new Intl.NumberFormat(locale), [locale]);
  const calendarStart = value ?? dates[0];
  const calendarEnd = value ?? endingOn;
  const showYear = [calendarStart, calendarEnd].some(date => date.slice(0, 4) !== today.slice(0, 4));
  const dateFormat = useMemo(() => new Intl.DateTimeFormat(locale, {
    month: "short", day: "numeric",
    year: showYear ? "numeric" : undefined
  }), [locale, showYear]);
  const calendarLabel = dateFormat.formatRange(dateFromKey(calendarStart), dateFromKey(calendarEnd));
  const week = dates.reduce<SpeechCount>((count, date) => {
    const day = activity.get(date) ?? emptySpeechCount();
    return { me: count.me + day.me, other: count.other + day.other };
  }, emptySpeechCount());
  const count = value ? activity.get(value) ?? emptySpeechCount() : week;
  const meLabel = `${t("speechActivityMe")} · ${t("speechActivityTotal", { count: countFormat.format(count.me) })}`;
  const otherLabel = `${t("speechActivityOther")} · ${t("speechActivityTotal", { count: countFormat.format(count.other) })}`;
  const maximum = Math.max(1, ...dates.map(date => totalSpeechCount(activity.get(date) ?? emptySpeechCount())));
  const listRef = useRef<HTMLDivElement>(null);
  const hover = useFluidHover(listRef, { axis: "x" });
  const focusDate = dates.includes(value ?? "") ? value : dates.includes(today) ? today : dates[0];

  useLayoutEffect(() => {
    const selected = listRef.current?.querySelector<HTMLElement>('[aria-checked="true"]');
    const viewport = listRef.current?.parentElement;
    if (!selected || !viewport) return;
    const item = selected.getBoundingClientRect();
    const view = viewport.getBoundingClientRect();
    const style = getComputedStyle(viewport);
    const left = view.left + parseFloat(style.paddingLeft);
    const right = view.right - parseFloat(style.paddingRight);
    if (item.left < left) viewport.scrollLeft += item.left - left;
    else if (item.right > right) viewport.scrollLeft += item.right - right;
  }, [dates, value]);

  const selectDate = (date: string) => {
    if (date > today || localDateKey(dateFromKey(date)) !== date) return;
    if (date === today) setAnchor(null);
    else if (!dates.includes(date)) setAnchor(date);
    onChange(date);
  };
  const moveWeek = (offset: number) => {
    const nextEnd = shiftDate(endingOn, offset * 7);
    setAnchor(nextEnd >= today ? null : nextEnd);
    if (value) {
      const nextDate = shiftDate(value, offset * 7);
      onChange(nextDate > today ? today : nextDate);
    }
  };
  const onDateKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    const buttons = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>("[data-speech-date]"));
    const index = buttons.indexOf(event.target as HTMLButtonElement);
    if (index < 0) return;
    event.preventDefault();
    const next = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1
      : Math.max(0, Math.min(buttons.length - 1, index + (event.key === "ArrowRight" ? 1 : -1)));
    buttons[next]?.focus();
    const date = buttons[next]?.dataset.speechDate;
    if (date) selectDate(date);
  };

  return <section className="speech-activity" aria-label={t("speechActivityTitle")} data-testid="speech-activity">
    <header className="speech-activity-header">
      <div className="speech-activity-summary">
        {!value && <span className="sr-only">{t("speechActivityWeek")}</span>}
        <p className="speech-activity-total" data-testid="speech-activity-total">
          <strong className={typeClass("subtitle")} style={{ fontVariationSettings: fontWeights.semibold }}>{countFormat.format(totalSpeechCount(count))}</strong>{" "}
          <span className={`text-muted-foreground ${typeClass("caption")}`}>{t("speechActivityUnit")}</span>
        </p>
        <div className={`speech-activity-legend ${typeClass("caption")}`}>
          <Tooltip content={meLabel} className="speech-chart-tooltip">
            <span className="speech-legend-item" tabIndex={0} aria-label={meLabel}>
              <i className="speech-swatch" data-speaker="me" aria-hidden="true" />{t("speechActivityMe")}
              <span className="sr-only" aria-hidden="true" data-testid="speech-count-me">{countFormat.format(count.me)}</span>
            </span>
          </Tooltip>
          <Tooltip content={<>{otherLabel}<br />{t("speechActivityOtherHint")}</>} className="speech-chart-tooltip">
            <span className="speech-legend-item" tabIndex={0} aria-label={otherLabel}>
              <i className="speech-swatch" data-speaker="other" aria-hidden="true" />{t("speechActivityOther")}
              <span className="sr-only" aria-hidden="true" data-testid="speech-count-other">{countFormat.format(count.other)}</span>
            </span>
          </Tooltip>
        </div>
      </div>
      <div className="speech-activity-navigation">
        <Button variant="ghost" size="icon-compact" aria-label={t("speechActivityPreviousWeek")} onClick={() => moveWeek(-1)}><ChevronLeft aria-hidden="true" /></Button>
        <label className={`speech-calendar ${typeClass("caption")}`} title={calendarLabel}>
          <CalendarDays size={14} aria-hidden="true" />
          <span>{calendarLabel}</span>
          <input type="date" aria-label={t("speechActivityChooseDate")} title={calendarLabel} value={value ?? ""}
            max={today} onChange={event => event.target.value && selectDate(event.target.value)} />
        </label>
        <Button variant="ghost" size="icon-compact" disabled={endingOn >= today}
          aria-label={t("speechActivityNextWeek")} onClick={() => moveWeek(1)}><ChevronRight aria-hidden="true" /></Button>
        <Button variant="ghost" size="compact" className="speech-today" aria-label={t("speechActivityToday")}
          title={t("speechActivityToday")} onClick={() => selectDate(today)}>
          <span className="speech-today-label">{t("speechActivityToday")}</span>
          <RotateCcw className="speech-today-icon" size={14} aria-hidden="true" />
        </Button>
      </div>
    </header>

    <div className="speech-date-viewport">
      <div ref={listRef} {...hover.handlers} onKeyDown={onDateKeyDown} className="speech-date-strip" role="radiogroup" aria-label={t("speechActivityDates")}>
        <FluidHoverHighlight hover={hover} className="speech-date-hover" />
        {dates.map((date, index) => <SpeechDate key={date} date={date} index={index} registerItem={hover.registerItem}
          count={activity.get(date) ?? emptySpeechCount()} maximum={maximum} language={language}
          selected={value === date} today={today === date} tabIndex={focusDate === date ? 0 : -1} onSelect={selectDate} />)}
      </div>
    </div>

    {totalSpeechCount(count) === 0 && <p className={`speech-activity-empty ${typeClass("caption")}`} role="status">
      {t(value ? "speechActivityEmptyDay" : "speechActivityEmptyWeek")}
    </p>}
  </section>;
}

function SpeechDate({ date, index, registerItem, count, maximum, language, selected, today, tabIndex, onSelect }: {
  date: string; index: number; registerItem: UseFluidHoverReturn["registerItem"]; count: SpeechCount; maximum: number;
  language: AppLanguage; selected: boolean; today: boolean; tabIndex: number; onSelect: (date: string) => void;
}) {
  const ref = useRef<HTMLButtonElement>(null);
  useRegisterFluidHoverItem(registerItem, index, ref);
  const t = createT(language);
  const locale = resolveIntlLocale(language);
  const day = dateFromKey(date);
  const label = t("speechActivityDayLabel", {
    date: new Intl.DateTimeFormat(locale, { year: "numeric", month: "long", day: "numeric", weekday: "long" }).format(day),
    me: new Intl.NumberFormat(locale).format(count.me), other: new Intl.NumberFormat(locale).format(count.other)
  });
  const compactCount = new Intl.NumberFormat(locale, { notation: "compact", maximumFractionDigits: 1 }).format(totalSpeechCount(count));
  return <button ref={ref} type="button" role="radio" aria-checked={selected} aria-current={today ? "date" : undefined}
    aria-label={label} title={label} className="speech-date" data-speech-date={date}
    tabIndex={tabIndex} onClick={() => onSelect(date)}>
    <span className="speech-date-label">
      <span className={`speech-weekday ${typeClass("micro")}`}>{new Intl.DateTimeFormat(locale, { weekday: "short" }).format(day)}</span>
      <span className={typeClass("caption")} style={{ fontVariationSettings: fontWeights.normal }}>{day.getDate()}</span>
    </span>
    <StackedSpeechBar count={count} maximum={maximum} />
    <span className={`speech-day-count ${typeClass("micro")}`} aria-hidden="true">{compactCount}</span>
  </button>;
}
