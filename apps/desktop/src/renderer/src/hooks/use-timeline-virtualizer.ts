import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useScrollAreaViewport } from "@/components/ui/legacy-scroll-area";
import type { TimelineEntry } from "@/lib/activity-timeline";

export function useTimelineVirtualizer(events: TimelineEntry[], active: boolean) {
  const viewport = useScrollAreaViewport();
  const listRef = useRef<HTMLOListElement>(null);
  const [scrollMargin, setScrollMargin] = useState(0);
  const marginRef = useRef(0);
  const pendingDeltaRef = useRef<number | null>(null);
  const enabled = active && events.length > 0;
  const getItemKey = useCallback((index: number) => events[index].id, [events]);
  const virtualizer = useVirtualizer<HTMLDivElement, HTMLLIElement>({
    count: events.length,
    getScrollElement: () => viewport,
    getItemKey,
    estimateSize: () => 160,
    overscan: 6,
    scrollMargin,
    enabled,
    // New speech stays visible at the top; readers further down keep their place.
    anchorTo: (viewport?.scrollTop ?? 0) > scrollMargin + 1 ? "end" : "start"
  });

  useLayoutEffect(() => {
    const list = listRef.current;
    if (!enabled || !list || !viewport) return;
    const updateGeometry = () => {
      const margin = list.getBoundingClientRect().top - viewport.getBoundingClientRect().top + viewport.scrollTop;
      const previousMargin = marginRef.current;
      if (Math.abs(previousMargin - margin) > 0.5) {
        marginRef.current = margin;
        pendingDeltaRef.current = previousMargin > 0 && viewport.scrollTop > previousMargin + 1
          ? margin - previousMargin : null;
        setScrollMargin(margin);
      }
    };
    updateGeometry();
    const observer = new ResizeObserver(updateGeometry);
    observer.observe(list);
    // Status controls, notices and active apps can move the list without resizing it.
    for (let parent = list.parentElement; parent && parent !== viewport; parent = parent.parentElement) {
      observer.observe(parent);
      for (const sibling of parent.children) if (!sibling.contains(list)) observer.observe(sibling);
    }
    observer.observe(viewport);
    viewport.addEventListener("animationend", updateGeometry);
    const previousAnchor = viewport.style.overflowAnchor;
    viewport.style.overflowAnchor = "none";
    return () => {
      observer.disconnect();
      viewport.removeEventListener("animationend", updateGeometry);
      viewport.style.overflowAnchor = previousAnchor;
    };
  }, [enabled, viewport, virtualizer]);

  useLayoutEffect(() => {
    // Row measurements may already have adjusted the reading position. Apply
    // only the header's delta after commit, then let later measurements anchor
    // normally instead of pinning an absolute target based on old estimates.
    const delta = pendingDeltaRef.current;
    pendingDeltaRef.current = null;
    if (enabled && delta !== null) {
      virtualizer.scrollBy(delta);
      virtualizer.cancelScroll();
    }
  }, [scrollMargin, enabled, virtualizer]);

  return { listRef, virtualizer };
}
