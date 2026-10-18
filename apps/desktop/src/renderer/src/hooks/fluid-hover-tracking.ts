"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useRef, useState, useCallback, useEffect, type RefObject } from "react";
import type { UseFluidHoverOptions } from "./fluid-hover-geometry";
import type { UseFluidHoverReturn } from "./fluid-hover-geometry";
import { useIsoLayoutEffect } from "./fluid-hover-geometry";
import { ACTIVE_INDEX_ATTR } from "./fluid-hover-geometry";
import { ACTIVE_ATTR } from "./fluid-hover-geometry";
import type { ItemRect } from "./fluid-hover-geometry";
import { measurementAttempts } from "./fluid-hover-geometry";
import { isOwnEvent } from "./fluid-hover-geometry";
import { pickNearest } from "./fluid-hover-geometry";
import { resolveActivator } from "./fluid-hover-geometry";

export function useFluidHover<T extends HTMLElement>(
  containerRef: RefObject<T | null>,
  options: UseFluidHoverOptions = {}
): UseFluidHoverReturn {
  const { axis = "y", isItemDisabled, gapClick = true } = options;
  const gapClickMaxDistance =
    typeof gapClick === "object" ? (gapClick.maxDistance ?? Infinity) : Infinity;
  const itemsRef = useRef(new Map<number, HTMLElement>());
  const [activeIndex, setActiveIndex] = useState<number | null>(null);
  // Mirrored for handlers that read it outside a render (the gap click, a
  // row registering). Written in a layout effect, which runs before any
  // row's registration effect in the same commit.
  const activeIndexRef = useRef<number | null>(null);
  useIsoLayoutEffect(() => {
    activeIndexRef.current = activeIndex;
  }, [activeIndex]);

  // The state, in the DOM: `data-fluid-hover-active` on the highlighted item
  // and `data-fluid-hover-active-index` on the container. Devtools shows it
  // and a test asserts on it without waiting for a frame. React does not
  // manage these attributes, so it never clobbers them.
  useEffect(() => {
    const container = containerRef.current;
    // The registry map itself, which is never replaced: the cleanup reads
    // who holds the index by then, not who held it now.
    const items = itemsRef.current;
    if (activeIndex === null) container?.removeAttribute(ACTIVE_INDEX_ATTR);
    else container?.setAttribute(ACTIVE_INDEX_ATTR, String(activeIndex));
    const active = activeIndex === null ? undefined : items.get(activeIndex);
    active?.setAttribute(ACTIVE_ATTR, "");
    return () => {
      active?.removeAttribute(ACTIVE_ATTR);
      // A row that re-registered under this index while it was highlighted
      // (a remount under a new key) was marked by registerItem, not by this
      // effect: drop the mark from whatever element holds the index now.
      if (activeIndex !== null) items.get(activeIndex)?.removeAttribute(ACTIVE_ATTR);
    };
  }, [activeIndex, containerRef]);
  const [itemRects, setItemRects] = useState<ItemRect[]>([]);
  const [isMeasured, setIsMeasured] = useState(false);
  const itemRectsRef = useRef<ItemRect[]>([]);
  const [session, setSession] = useState(0);
  const sessionRef = useRef(0);
  // Set when the pointer enters; the pointer's first change of highlight
  // starts the new session, in the same update as the change itself.
  const sessionPendingRef = useRef(false);
  const rafIdRef = useRef<number | null>(null);
  const remeasureRafIdRef = useRef<number | null>(null);

  /**
   * Publishes a rect for every registered item. Returns false when the
   * measurement could not be completed (no container, or an item without a
   * layout box) — nothing is published in that case, so the last complete
   * measurement stands instead of being overwritten with zeroes.
   */
  const runMeasurement = useCallback(() => {
    const container = containerRef.current;
    if (!container) return false;
    const rects: ItemRect[] = [];
    let everyItemHasLayout = true;
    itemsRef.current.forEach((element, index) => {
      // An element inside a display:none / not-yet-laid-out popup has no
      // offsetParent and reports every offset as 0. Publishing that would pin
      // overlays to the top of the list, so treat the whole pass as
      // incomplete. A boxless element is the only case: `position: fixed`
      // items also have no offsetParent but do have a size.
      const hasLayoutBox =
        element.offsetParent !== null ||
        element.offsetWidth > 0 ||
        element.offsetHeight > 0;
      if (!hasLayoutBox) {
        everyItemHasLayout = false;
        return;
      }
      // Use offset* instead of getBoundingClientRect so measurements are
      // unaffected by CSS transforms (e.g. scaleY animation on the parent
      // motion.div). offsetTop/offsetLeft are layout values relative to the
      // offsetParent (the scroll container), matching the coordinate space
      // used by `position: absolute` children. Items nested inside positioned
      // descendants of the container (a sidebar sub-menu's rows live inside a
      // positioned row) accumulate those ancestors' offsets, so every rect
      // lands in the container's own coordinate space; for a flat list the
      // loop never runs and this is exactly the plain offsetTop/offsetLeft.
      let top = element.offsetTop;
      let left = element.offsetLeft;
      let ancestor = element.offsetParent as HTMLElement | null;
      while (ancestor && ancestor !== container && container.contains(ancestor)) {
        top += ancestor.offsetTop + ancestor.clientTop;
        left += ancestor.offsetLeft + ancestor.clientLeft;
        ancestor = ancestor.offsetParent as HTMLElement | null;
      }
      rects[index] = {
        top,
        height: element.offsetHeight,
        left,
        width: element.offsetWidth,
      };
    });
    if (!everyItemHasLayout) return false;
    // Skip the state update when nothing moved (a cheap top/left/width/height
    // compare) so redundant remeasures don't churn re-renders.
    const prev = itemRectsRef.current;
    let changed = prev.length !== rects.length;
    for (let i = 0; !changed && i < rects.length; i++) {
      const p = prev[i];
      const r = rects[i];
      if (p === r) continue; // both undefined (sparse slot)
      changed =
        !p ||
        !r ||
        p.top !== r.top ||
        p.left !== r.left ||
        p.width !== r.width ||
        p.height !== r.height;
    }
    if (changed) {
      itemRectsRef.current = rects;
      setItemRects(rects);
    }
    return true;
  }, [containerRef]);

  const measureItems = useCallback(() => {
    runMeasurement();
  }, [runMeasurement]);

  /**
   * The hook's single measurement pass: coalesces every trigger (item
   * registration, container resize) into one remeasure on the next frame and
   * is the only place readiness is reported, so `isMeasured` can never turn
   * true while another pass is still queued.
   */
  const scheduleMeasurement = useCallback(
    (attempts: number) => {
      const attempt = (attemptsLeft: number) => {
        if (remeasureRafIdRef.current !== null) {
          cancelAnimationFrame(remeasureRafIdRef.current);
        }
        remeasureRafIdRef.current = requestAnimationFrame(() => {
          remeasureRafIdRef.current = null;
          if (runMeasurement()) {
            setIsMeasured(true);
          } else if (attemptsLeft > 1) {
            attempt(attemptsLeft - 1);
          }
        });
      };
      attempt(attempts);
    },
    [runMeasurement]
  );

  const remeasure = useCallback(() => {
    // Readiness drops first: until the pass below settles, the published rects
    // may not describe what is on screen, and an overlay positioned from them
    // would be corrected after mounting — which animates as a slide.
    setIsMeasured(false);
    scheduleMeasurement(measurementAttempts);
  }, [scheduleMeasurement]);

  // Observes the registered items themselves (not just the container): rows
  // that change size in place — e.g. the site-wide size step flipping while a
  // selection background is up — must invalidate the published rects even when
  // the container the effect below captured has since been remounted and the
  // ref points at a different element than the one being observed.
  const itemRoRef = useRef<ResizeObserver | null>(null);
  const getItemRo = useCallback(() => {
    if (itemRoRef.current === null && typeof ResizeObserver !== "undefined") {
      itemRoRef.current = new ResizeObserver(() =>
        scheduleMeasurement(measurementAttempts)
      );
    }
    return itemRoRef.current;
  }, [scheduleMeasurement]);

  const registerItem = useCallback(
    (index: number, element: HTMLElement | null) => {
      if (element) {
        itemsRef.current.set(index, element);
        getItemRo()?.observe(element);
        if (index === activeIndexRef.current) element.setAttribute(ACTIVE_ATTR, "");
      } else {
        const previous = itemsRef.current.get(index);
        if (previous) itemRoRef.current?.unobserve(previous);
        // The mark leaves with the element: a row that only moved to another
        // index (a filtering list re-ordering) must not carry it there.
        previous?.removeAttribute(ACTIVE_ATTR);
        itemsRef.current.delete(index);
        // The highlighted row is gone: nothing should stay lit or receive a
        // routed click until the pointer picks again. Decided when the
        // update applies, after this commit's registrations, so a row that
        // only moved index hands the highlight to the row now under it.
        if (index === activeIndexRef.current) {
          setActiveIndex((current) =>
            current === index && !itemsRef.current.has(index) ? null : current
          );
        }
      }
      // Coalesce rapid register/unregister calls (e.g. when an AnimatePresence
      // remounts a list of rows) into a single remeasure on the next frame,
      // so consumers don't have to manually call measureItems after the
      // container's children swap.
      remeasure();
    },
    [remeasure, getItemRo]
  );

  const handleMouseMove = useCallback(
    (e: React.MouseEvent) => {
      if (!isOwnEvent(e)) return;
      const mouseX = e.clientX;
      const mouseY = e.clientY;

      if (rafIdRef.current !== null) {
        cancelAnimationFrame(rafIdRef.current);
      }

      rafIdRef.current = requestAnimationFrame(() => {
        rafIdRef.current = null;
        const container = containerRef.current;
        if (!container) return;
        const next = pickNearest({
          axis,
          point: { x: mouseX, y: mouseY },
          rects: itemRectsRef.current,
          containerRect: container.getBoundingClientRect(),
          scroll: { x: container.scrollLeft, y: container.scrollTop },
          border: { x: container.clientLeft, y: container.clientTop },
          layoutSize: { width: container.offsetWidth, height: container.offsetHeight },
          isDisabled: isItemDisabled
            ? (index) => {
                const el = itemsRef.current.get(index);
                return !!el && isItemDisabled(el);
              }
            : undefined,
        });
        if (sessionPendingRef.current && next !== activeIndexRef.current) {
          sessionPendingRef.current = false;
          sessionRef.current += 1;
          setSession(sessionRef.current);
        }
        setActiveIndex(next);
      });
    },
    [axis, containerRef, isItemDisabled]
  );

  const handleMouseEnter = useCallback(() => {
    sessionPendingRef.current = true;
  }, []);

  const handleMouseLeave = useCallback(() => {
    if (rafIdRef.current !== null) {
      cancelAnimationFrame(rafIdRef.current);
      rafIdRef.current = null;
    }
    setActiveIndex(null);
  }, []);

  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      const target = e.target as Node | null;
      if (!target) return;
      // Inside an item: the item owns the click.
      for (const element of itemsRef.current.values()) {
        if (element.contains(target)) return;
      }
      // A row that unmounted while its own click was still bubbling (a pick
      // whose primitive re-renders the list synchronously, like a "create"
      // row that becomes a real item) already landed; it is not a gap.
      if (!target.isConnected) return;
      // Nor is a click from something rendered through a portal (a dropdown
      // opened from a row, a submenu): it bubbles here through React, but it
      // never happened between these rows.
      if (!isOwnEvent(e)) return;
      // A control that sits between the rows (a search field at the top of
      // a menu, a footer button) keeps its own click too.
      const control = (target as Element).closest?.(
        "input, textarea, select, button, a, summary, [contenteditable], [role='textbox'], [role='searchbox'], [role='button']"
      );
      if (control) return;
      if (gapClick === false) return;
      const index = activeIndexRef.current;
      if (index === null) return;
      const element = itemsRef.current.get(index);
      if (!element || isItemDisabled?.(element)) return;
      if (gapClickMaxDistance !== Infinity) {
        const r = element.getBoundingClientRect();
        const dx = Math.max(r.left - e.clientX, 0, e.clientX - r.right);
        const dy = Math.max(r.top - e.clientY, 0, e.clientY - r.bottom);
        if (Math.hypot(dx, dy) > gapClickMaxDistance) return;
      }
      // A real DOM click on the item, so its own handlers (and the primitive
      // wrapping it, if any) run exactly as if the pointer had been inside.
      resolveActivator(element).click();
    },
    [isItemDisabled, gapClick, gapClickMaxDistance]
  );

  // Remeasure when the container resizes — a reflow moves items even though
  // the registered set is unchanged, which would otherwise leave itemRects
  // stale. Coalesced through the same rAF as register/unregister. Readiness is
  // deliberately not dropped: the item set is unchanged, so the published rects
  // stay usable, and hiding overlays on every reflow would flicker them.
  useEffect(() => {
    const container = containerRef.current;
    if (!container || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => scheduleMeasurement(measurementAttempts));
    ro.observe(container);
    return () => ro.disconnect();
  }, [containerRef, scheduleMeasurement]);

  // Clean up rAF and the item observer on unmount
  useEffect(() => {
    return () => {
      if (rafIdRef.current !== null) {
        cancelAnimationFrame(rafIdRef.current);
      }
      if (remeasureRafIdRef.current !== null) {
        cancelAnimationFrame(remeasureRafIdRef.current);
      }
      itemRoRef.current?.disconnect();
      itemRoRef.current = null;
    };
  }, []);

  return {
    activeIndex,
    setActiveIndex,
    itemRects,
    isMeasured,
    session,
    sessionRef,
    handlers: {
      onMouseMove: handleMouseMove,
      onMouseEnter: handleMouseEnter,
      onMouseLeave: handleMouseLeave,
      onClick: handleClick,
    },
    registerItem,
    remeasure,
    measureItems,
  };
}

/**
 * Registers an item's element with its list for as long as it is mounted.
 * The one way rows join a list: pass the hook's `registerItem` (or the copy
 * a context hands down), the row's index, and its ref. Either may be
 * missing for a row rendered outside a list (a standalone card, an accordion
 * item that is not grouped); then nothing is registered.
 */
export function useRegisterFluidHoverItem(
  registerItem: ((index: number, element: HTMLElement | null) => void) | undefined,
  index: number | undefined,
  ref: RefObject<HTMLElement | null>
) {
  useEffect(() => {
    if (!registerItem || index === undefined) return;
    registerItem(index, ref.current);
    return () => registerItem(index, null);
  }, [index, registerItem, ref]);
}
