"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useState, useCallback, useMemo, useRef, type RefObject } from "react";
import { motion, AnimatePresence } from "framer-motion";

import { spring } from "@/lib/springs";

import { useShape } from "@/lib/shape-context";

import { useFluidHover, type ItemRect } from "@/hooks/use-fluid-hover";

import { FluidHoverHighlight } from "@/components/fluid-hover-highlight";
import type { MenuScopeOptions } from "./sidebar-menu-context";
import type { MenuScope } from "./sidebar-menu-context";
import { rowSkipped } from "./sidebar-menu-context";
import type { RowInfo } from "./sidebar-menu-context";
import { sameElements } from "./sidebar-menu-context";
import { byDomOrder } from "./sidebar-menu-context";
import { overlayGroupId } from "./sidebar-menu-context";
import { ROOT_LEVEL_ID } from "./sidebar-menu-context";
import { sameRowInfo } from "./sidebar-menu-context";
import type { MenuScopeValue } from "./sidebar-menu-context";
import type { OverlayTarget } from "./sidebar-menu-context";
import { NO_TARGET } from "./sidebar-menu-context";
import { retarget } from "./sidebar-menu-context";
import { retargetAll } from "./sidebar-menu-context";

function useMenuScope(
  containerRef: RefObject<HTMLElement | null>,
  { focusRing = true }: MenuScopeOptions = {}
): MenuScope {
  const {
    activeIndex,
    setActiveIndex,
    itemRects,
    isMeasured,
    session,
    handlers,
    registerItem,
  } = useFluidHover(containerRef, { isItemDisabled: rowSkipped });

  const rowsRef = useRef<Set<HTMLElement>>(new Set());
  const rowButtonsRef = useRef<Map<HTMLElement, HTMLElement>>(new Map());
  const activeMapRef = useRef<Map<HTMLElement, boolean>>(new Map());
  const [orderedRows, setOrderedRows] = useState<HTMLElement[]>([]);
  // syncRows keeps this current the moment the row set changes; the state
  // above follows on the next render.
  const orderedRowsRef = useRef(orderedRows);
  const [rowInfo, setRowInfo] = useState<Map<HTMLElement, RowInfo>>(() => new Map());
  const registeredCountRef = useRef(0);
  const [activeRows, setActiveRows] = useState<HTMLElement[]>([]);
  const [focusedRowEl, setFocusedRowEl] = useState<HTMLElement | null>(null);

  const recomputeActive = useCallback(() => {
    const next = orderedRowsRef.current.filter(
      (el) => activeMapRef.current.get(el) && !rowSkipped(el)
    );
    setActiveRows((prev) => (sameElements(prev, next) ? prev : next));
  }, []);

  const rowButton = useCallback(
    (row: HTMLElement) =>
      rowButtonsRef.current.get(row) ??
      row.querySelector<HTMLElement>(
        ':scope > [data-sidebar="menu-button"], :scope > [data-sidebar="menu-sub-button"]'
      ),
    []
  );

  // Rows register by element; indexes are derived from DOM order so consumers
  // never pass an index prop and conditional rows just work. The fluid hover
  // system measures the row's BUTTON, not the <li>: a row hosting an expanded
  // sub-tree is a tall <li>, and hit-testing against that whole box would hand
  // the sub-tree's gaps and gutter to the parent — the button strip is the
  // only part that is really "the row".
  const syncRows = useCallback(() => {
    const sorted = [...rowsRef.current].sort(byDomOrder);
    // The ref updates synchronously (not just at the next render): callers in
    // the same commit — a row registering, its button turning active — must
    // see the row set they just changed, or the first recompute of a mount
    // filters every row out against the previous render's empty list.
    orderedRowsRef.current = sorted;
    setOrderedRows((prev) => (sameElements(prev, sorted) ? prev : sorted));
    // Every active row gets its own background, keyed by the row's level
    // (root, or its sub-menu): see activeRects below.
    const info = new Map<HTMLElement, RowInfo>();
    for (const el of sorted) {
      const sub = el.closest('[data-sidebar="menu-sub"]');
      info.set(el, {
        levelId: sub ? overlayGroupId(sub) : ROOT_LEVEL_ID,
        buttonless: !rowButton(el),
      });
    }
    setRowInfo((prev) => (sameRowInfo(prev, info) ? prev : info));
    sorted.forEach((el, i) => registerItem(i, rowButton(el) ?? el));
    for (let i = sorted.length; i < registeredCountRef.current; i++) {
      registerItem(i, null);
    }
    registeredCountRef.current = sorted.length;
    recomputeActive();
  }, [registerItem, recomputeActive, rowButton]);

  const registerRow = useCallback(
    (el: HTMLElement) => {
      rowsRef.current.add(el);
      syncRows();
      return () => {
        rowsRef.current.delete(el);
        rowButtonsRef.current.delete(el);
        activeMapRef.current.delete(el);
        syncRows();
      };
    },
    [syncRows]
  );

  const setRowButton = useCallback(
    (row: HTMLElement, button: HTMLElement | null) => {
      if (button) rowButtonsRef.current.set(row, button);
      else rowButtonsRef.current.delete(row);
      // The button is the row's measured element, so a button arriving after
      // its row registered must re-sync what the fluid hover system observes.
      syncRows();
    },
    [syncRows]
  );

  const setRowActive = useCallback(
    (row: HTMLElement, active: boolean) => {
      activeMapRef.current.set(row, active);
      recomputeActive();
    },
    [recomputeActive]
  );

  const refreshVisibility = useCallback(() => {
    recomputeActive();
    // A hover riding a row that just collapsed away has nothing under it.
    setActiveIndex((prev) => {
      const row = prev !== null ? orderedRowsRef.current[prev] : undefined;
      return row && rowSkipped(row) ? null : prev;
    });
  }, [recomputeActive, setActiveIndex]);

  // The hook measures each row's button, so a rect is the button strip. A
  // row registered without one was measured as its whole <li> — which grows
  // when it hosts an expanded sub-menu — so that rect is clamped to 48px
  // (the tallest row, size="lg"): the highlight can never cover an expanded
  // sub-tree.
  const overlayRect = (row: HTMLElement | null): ItemRect | null => {
    if (!row) return null;
    const idx = orderedRows.indexOf(row);
    const rect = idx === -1 ? null : itemRects[idx];
    if (!rect) return null;
    return rowInfo.get(row)?.buttonless ? { ...rect, height: Math.min(rect.height, 48) } : rect;
  };

  // While a popup anchored in the sidebar is open (a row action's or the
  // header/footer rows' dropdown), hover tracking freezes across every menu
  // scope — otherwise a non-modal popup lets rows underneath keep
  // highlighting. Popup triggers are detected by the primitives' open
  // attributes (Radix data-state, Base UI data-popup-open); collapsible rows
  // only set aria-expanded, so they never match.
  const popupOpen = useCallback(() => {
    const container = containerRef.current;
    if (!container) return false;
    const root = container.closest('[data-slot="sidebar-wrapper"]') ?? container;
    return !!root.querySelector(
      '[data-sidebar="menu-button"][data-state="open"], [data-sidebar="menu-button"][data-popup-open], [data-sidebar="menu-action"][data-state="open"], [data-sidebar="menu-action"][data-popup-open]'
    );
  }, [containerRef]);

  const onMouseMove = useCallback(
    (e: React.MouseEvent) => {
      if (popupOpen()) return;
      handlers.onMouseMove(e);
    },
    [popupOpen, handlers]
  );

  const onFocus = useCallback(
    (e: React.FocusEvent) => {
      const target = e.target as HTMLElement;
      // Only the row's main button drives the traveling highlight and ring —
      // actions keep their own static focus rings.
      if (!target.closest('[data-sidebar="menu-button"],[data-sidebar="menu-sub-button"]')) return;
      const row = target.closest(
        '[data-sidebar="menu-item"],[data-sidebar="menu-sub-item"]'
      ) as HTMLElement | null;
      if (!row) return;
      const idx = orderedRowsRef.current.indexOf(row);
      if (idx === -1) return;
      setActiveIndex(idx);
      setFocusedRowEl(target.matches(":focus-visible") ? row : null);
    },
    [setActiveIndex]
  );

  const onPointerDown = useCallback(() => {
    setFocusedRowEl(null);
  }, []);

  const onBlur = useCallback(
    (e: React.FocusEvent) => {
      if (containerRef.current?.contains(e.relatedTarget as Node)) return;
      setFocusedRowEl(null);
      setActiveIndex(null);
    },
    [containerRef, setActiveIndex]
  );

  // Arrow/Home/End over every button in DOM order, sub rows included — only
  // the root scope binds it so nested scopes don't double-handle.
  const onKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (!["ArrowDown", "ArrowUp", "ArrowRight", "ArrowLeft", "Home", "End"].includes(e.key))
        return;
      const container = containerRef.current;
      if (!container) return;
      const items = Array.from(
        container.querySelectorAll<HTMLElement>(
          '[data-sidebar="menu-button"], [data-sidebar="menu-sub-button"]'
        )
      ).filter((el) => !el.closest('[data-sidebar="menu-sub"][data-state="closed"]'));
      const currentIdx = items.indexOf(e.target as HTMLElement);
      if (currentIdx === -1) return;
      e.preventDefault();
      // Keep handled arrows from also reaching window-level listeners (the
      // docs site's ←/→ page navigation) — same rule as AskUserQuestions.
      e.stopPropagation();
      if (e.key === "Home") items[0]?.focus();
      else if (e.key === "End") items[items.length - 1]?.focus();
      else {
        const next = ["ArrowDown", "ArrowRight"].includes(e.key)
          ? (currentIdx + 1) % items.length
          : (currentIdx - 1 + items.length) % items.length;
        items[next]?.focus();
      }
    },
    [containerRef]
  );

  const hoveredRowEl = activeIndex !== null ? orderedRows[activeIndex] ?? null : null;

  const value = useMemo<MenuScopeValue>(
    () => ({
      registerRow,
      setRowButton,
      setRowActive,
      hoveredRowEl,
      activeRows,
      firstRowEl: orderedRows[0] ?? null,
      hasActive: activeRows.length > 0,
      refreshVisibility,
    }),
    [registerRow, setRowButton, setRowActive, hoveredRowEl, activeRows, orderedRows, refreshVisibility]
  );

  const shape = useShape();
  // Every active row gets its own background — the buttons' own text styling
  // already lights each active row, so the overlays must match. Keys are the
  // row's level (root, or its sub-menu) plus its occurrence within that
  // level: the usual case — one active per level, e.g. a current section
  // marker plus the current page inside its sub-tree — keeps a stable key,
  // so the background GLIDES when the selection moves instead of remounting.
  const levelOccurrence = new Map<number, number>();
  const activeRects: { key: string; rect: ItemRect; row: HTMLElement }[] = [];
  for (const row of activeRows) {
    const levelId = rowInfo.get(row)?.levelId;
    if (levelId === undefined) continue;
    const occurrence = levelOccurrence.get(levelId) ?? 0;
    levelOccurrence.set(levelId, occurrence + 1);
    const rect = overlayRect(row);
    if (rect) activeRects.push({ key: `${levelId}:${occurrence}`, rect, row });
  }
  const hoverRect = overlayRect(hoveredRowEl);
  const focusRect = focusRing ? overlayRect(focusedRowEl) : null;

  // A rect change has two causes with two right answers. The highlight moving
  // to a DIFFERENT row springs — that's the glide. The same row itself moving
  // — a sibling sub-tree collapsing above reflows every row below on every
  // frame of its own spring — must snap, or the overlay chases the row it is
  // sitting on with a trailing second spring. Each overlay keeps its target
  // as of its last change, and which kind of change that was; updated during
  // render, so the render that moves an overlay already carries the answer.
  const [targets, setTargets] = useState<{
    hover: OverlayTarget;
    focus: OverlayTarget;
    actives: Map<string, OverlayTarget>;
  }>(() => ({ hover: NO_TARGET, focus: NO_TARGET, actives: new Map() }));
  const hoverTarget = retarget(targets.hover, hoveredRowEl, hoverRect);
  const focusTarget = retarget(targets.focus, focusedRowEl, focusRect);
  const activeTargets = retargetAll(targets.actives, activeRects);
  if (
    hoverTarget !== targets.hover ||
    focusTarget !== targets.focus ||
    activeTargets !== targets.actives
  ) {
    setTargets({ hover: hoverTarget, focus: focusTarget, actives: activeTargets });
  }
  const hoverRowChanged = hoverTarget.rowChanged;
  const focusRowChanged = focusTarget.rowChanged;

  const overlays = isMeasured ? (
    <>
      {/* Active row backgrounds — one per active row (see activeRects above) */}
      <AnimatePresence>
        {activeRects.map(({ key, rect }) => (
          <motion.div
            key={key}
            className={`absolute ${shape.bg} bg-active pointer-events-none`}
            initial={false}
            animate={{
              top: rect.top,
              left: rect.left,
              width: rect.width,
              height: rect.height,
              opacity: 1,
            }}
            exit={{ opacity: 0, transition: spring.moderate.exit }}
            transition={
              activeTargets.get(key)?.rowChanged
                ? { ...spring.moderate, opacity: { duration: 0.08 } }
                : { duration: 0 }
            }
          />
        ))}
      </AnimatePresence>

      {/* Hover background. Fades in at the first hovered row; snaps (no
          travel) when only a reflow moved the rows underneath. */}
      <FluidHoverHighlight
        rect={hoverRect}
        session={session}
        className={shape.bg}
        transition={hoverRowChanged ? undefined : false}
      />

      {/* Focus ring */}
      <AnimatePresence>
        {focusRect && (
          <motion.div
            className={`absolute ${shape.focusRing} pointer-events-none z-20 border border-[color:var(--focus-ring,#6B97FF)]`}
            initial={false}
            animate={{
              left: focusRect.left - 2,
              top: focusRect.top - 2,
              width: focusRect.width + 4,
              height: focusRect.height + 4,
            }}
            exit={{ opacity: 0, transition: spring.fast.exit }}
            transition={
              focusRowChanged
                ? { ...spring.fast, opacity: { duration: 0.08 } }
                : { duration: 0 }
            }
          />
        )}
      </AnimatePresence>
    </>
  ) : null;

  return {
    value,
    containerProps: {
      onMouseEnter: handlers.onMouseEnter,
      onMouseMove,
      onMouseLeave: handlers.onMouseLeave,
      onClick: handlers.onClick,
      onFocus,
      onBlur,
      // Pointer interaction switches modality back to pointer. Clicking the
      // already-focused row never re-fires focus, so without this the
      // keyboard ring would stick until focus left the menu.
      onPointerDown,
      onKeyDown,
    },
    overlays,
  };
}
export { useMenuScope };
