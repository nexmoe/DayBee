"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { createContext, useEffect, useLayoutEffect, type ReactNode, type RefObject } from "react";

import { type ItemRect } from "@/hooks/use-fluid-hover";

// SSR-safe layout effect (client components still server-render in Next).
const useIsoLayoutEffect =
  typeof window !== "undefined" ? useLayoutEffect : useEffect;

// ─── Menu scope ──────────────────────────────────────────────────────────────
//
// One scope per SidebarMenu tree: a single fluid-hover system plus the
// traveling overlays — hover background, active background(s), focus ring —
// that glide between every visible row, sub-menu rows included, so the hover
// moves from a parent into its children as one continuous piece. Sub rows
// live inside positioned ancestors, so their rects are accumulated into the
// menu's own coordinate space by the fluid hover hook. The active background
// stays one per level (the root rows, and each sub-menu) so a current section
// and the current page inside it can both be lit, exactly as before.

interface MenuScopeValue {
  registerRow: (el: HTMLElement) => () => void;
  setRowButton: (row: HTMLElement, button: HTMLElement | null) => void;
  setRowActive: (row: HTMLElement, active: boolean) => void;
  hoveredRowEl: HTMLElement | null;
  /** Every visible active row, in DOM order — a parent section marker and
   *  the current row inside its sub-tree can be active at once. */
  activeRows: HTMLElement[];
  firstRowEl: HTMLElement | null;
  hasActive: boolean;
  /** A sub-menu toggled: rows changed visibility in place, so hover targets
   *  and the visible active set must be recomputed. */
  refreshVisibility: () => void;
}

const MenuScopeContext = createContext<MenuScopeValue | null>(null);

interface MenuItemContextValue {
  rowRef: RefObject<HTMLLIElement | null>;
  /** Ref callback for the row's <li> — also replays the row's active flag
   *  to the scope, covering the windows where the ref is detached. */
  attachRow: (node: HTMLLIElement | null) => void;
  isHovered: boolean;
  isActiveRow: boolean;
  /** True inside SidebarMenuSubItem — actions center on the shorter row. */
  isSubRow: boolean;
  setActive: (active: boolean) => void;
  setButtonEl: (el: HTMLElement | null) => void;
  /** Trailing controls on this row, registered by the action / badge parts.
   *  The button turns them into an exact padding-right reservation. */
  actionCount: number;
  actionsShowOnHover: boolean;
  hasBadge: boolean;
  setActions: (count: number, showOnHover: boolean) => void;
  setHasBadge: (hasBadge: boolean) => void;
}

const MenuItemContext = createContext<MenuItemContextValue | null>(null);

/** True while rendering inside a SidebarMenuActions cluster, where each
 *  action flows in the wrapper's row instead of positioning itself. */
const MenuActionsClusterContext = createContext(false);

/** True while the element sits inside a collapsed sub-tree — clipped away,
 *  so it must be invisible to hover, highlights, and keyboard order. Rows
 *  stay registered either way: unregistering on every toggle would churn the
 *  fluid hover measurements and blink the overlays. */
function rowHidden(el: HTMLElement) {
  return el.closest('[data-sidebar="menu-sub"][data-state="closed"]') !== null;
}

/** True for a disabled row: it stays in the list (and in the layout) but is
 *  never lit, never takes a routed click, and is skipped by the keyboard.
 *  The hook registers the row's button (falling back to the row itself), so
 *  check the element first, then a button directly inside it. A sub-row's
 *  anchor carries aria-disabled instead of `disabled`. */
const DISABLED_CONTROL = ':disabled, [aria-disabled="true"]';

function rowDisabled(el: HTMLElement) {
  return (
    el.matches(DISABLED_CONTROL) ||
    el.querySelector(
      `:scope > [data-sidebar="menu-button"]:is(${DISABLED_CONTROL}), :scope > [data-sidebar="menu-sub-button"]:is(${DISABLED_CONTROL})`
    ) !== null
  );
}

/** Hidden or disabled: what hover, highlights, and the keyboard skip. */
function rowSkipped(el: HTMLElement) {
  return rowHidden(el) || rowDisabled(el);
}

/** Stable keys for the per-level active overlays: one id per sub-menu <ul>
 *  (or the menu root), so the active background glides when the active row
 *  moves within its level instead of remounting. The root level is 0 in
 *  every menu (sub-menu ids start at 1): rows register before the menu's
 *  own ref attaches, so the root can't be looked up by element. */
const ROOT_LEVEL_ID = 0;

let overlayGroupSeq = 0;

const overlayGroupIds = new WeakMap<Element, number>();

function overlayGroupId(el: Element) {
  let id = overlayGroupIds.get(el);
  if (id === undefined) {
    id = ++overlayGroupSeq;
    overlayGroupIds.set(el, id);
  }
  return id;
}

function byDomOrder(a: HTMLElement, b: HTMLElement) {
  return a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1;
}

function sameElements(a: HTMLElement[], b: HTMLElement[]) {
  return a.length === b.length && a.every((el, i) => el === b[i]);
}

/** What the overlays need to know about a row, read from the DOM when the
 *  row set syncs rather than during render: its level's overlay id (the
 *  root, or the sub-menu it sits in) and whether it was registered without a
 *  button (so the hook measured its whole <li>). */
interface RowInfo {
  levelId: number;
  buttonless: boolean;
}

function sameRowInfo(a: Map<HTMLElement, RowInfo>, b: Map<HTMLElement, RowInfo>) {
  if (a.size !== b.size) return false;
  for (const [row, info] of b) {
    const prev = a.get(row);
    if (!prev || prev.levelId !== info.levelId || prev.buttonless !== info.buttonless)
      return false;
  }
  return true;
}

function sameRect(a: ItemRect | null, b: ItemRect | null) {
  if (a === b) return true;
  if (!a || !b) return false;
  return a.top === b.top && a.left === b.left && a.width === b.width && a.height === b.height;
}

/** An overlay's target as of its last change, and whether that change moved
 *  it to a different row (spring) or only moved its row (snap). */
interface OverlayTarget {
  row: HTMLElement | null;
  rect: ItemRect | null;
  rowChanged: boolean;
}

const NO_TARGET: OverlayTarget = { row: null, rect: null, rowChanged: false };

function retarget(prev: OverlayTarget, row: HTMLElement | null, rect: ItemRect | null) {
  if (prev.row === row && sameRect(prev.rect, rect)) return prev;
  return { row, rect, rowChanged: prev.row !== row };
}

/** retarget over the keyed active overlays; the same map when none moved. */
function retargetAll(
  prev: Map<string, OverlayTarget>,
  next: { key: string; row: HTMLElement; rect: ItemRect }[]
) {
  let changed = prev.size !== next.length;
  const out = new Map<string, OverlayTarget>();
  for (const { key, row, rect } of next) {
    const before = prev.get(key);
    const after = retarget(before ?? NO_TARGET, row, rect);
    if (after !== before) changed = true;
    out.set(key, after);
  }
  return changed ? out : prev;
}

interface MenuScope {
  value: MenuScopeValue;
  containerProps: {
    onMouseEnter: () => void;
    onMouseMove: (e: React.MouseEvent) => void;
    onMouseLeave: () => void;
    /** A click between rows lands on the highlighted row's button. */
    onClick: (e: React.MouseEvent) => void;
    onFocus: (e: React.FocusEvent) => void;
    onBlur: (e: React.FocusEvent) => void;
    onPointerDown: () => void;
    onKeyDown?: (e: React.KeyboardEvent) => void;
  };
  overlays: ReactNode;
}

interface MenuScopeOptions {
  /** Draw the traveling keyboard focus ring. Off, keyboard focus moves the
   *  hover background only — for menus whose rows are the whole surface,
   *  like a settings dialog's section list. @default true */
  focusRing?: boolean;
}
export { useIsoLayoutEffect, MenuScopeContext, MenuItemContext, MenuActionsClusterContext, rowHidden, DISABLED_CONTROL, rowDisabled, rowSkipped, ROOT_LEVEL_ID, overlayGroupSeq, overlayGroupIds, overlayGroupId, byDomOrder, sameElements, sameRowInfo, sameRect, NO_TARGET, retarget, retargetAll };
export type { MenuScopeValue, MenuItemContextValue, RowInfo, OverlayTarget, MenuScope, MenuScopeOptions };
