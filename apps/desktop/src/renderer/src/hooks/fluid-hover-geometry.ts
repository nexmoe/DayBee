"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useEffect, useLayoutEffect, type Dispatch, type RefObject, type SetStateAction } from "react";

export interface ItemRect {
  top: number;
  height: number;
  left: number;
  width: number;
}

export interface UseFluidHoverOptions {
  /**
   * Which direction to resolve the nearest item along.
   *   "y"  — vertical lists (default): closest by top/height
   *   "x"  — horizontal strips: closest by left/width
   *   "xy" — 2-D grids: closest card across both rows AND columns,
   *          measured by Euclidean distance to each item's center
   */
  axis?: "x" | "y" | "xy";
  /**
   * Makes an item invisible to hit-testing without unregistering it — for
   * rows that stay mounted while clipped away (a collapsed sub-tree).
   * Unregistering would invalidate every measurement; a skipped item keeps
   * the set stable. Consulted per mouse move, so keep it cheap.
   */
  isItemDisabled?: (element: HTMLElement) => boolean;
  /**
   * Whether a click that lands between items (a gap, the padding, past the
   * last row) is routed to the highlighted item, so what is lit is what a
   * click hits. On by default: in a menu or a list the highlight is a
   * promise about the click. Pass `false` where empty space should stay
   * inert (rows with destructive actions, generous whitespace), or
   * `{ maxDistance }` to route only clicks within that many pixels of the
   * highlighted item's edge.
   */
  gapClick?: boolean | { maxDistance?: number };
}

export interface UseFluidHoverReturn {
  activeIndex: number | null;
  setActiveIndex: Dispatch<SetStateAction<number | null>>;
  itemRects: ItemRect[];
  /**
   * True once every registered item has been measured and no remeasure is
   * pending, i.e. `itemRects` describes the current item set. Gate absolutely
   * positioned overlays on it: an overlay that mounts against a rect a later
   * pass still corrects animates from the wrong place to the right one, which
   * reads as the highlight sliding in from another row.
   */
  isMeasured: boolean;
  /**
   * Counts pointer entries. It steps when the pointer, having entered the
   * container, first moves the highlight, so the highlight keyed on it fades
   * in at the new row instead of sliding over from where it was last (a row
   * lit from the keyboard, or one still fading out).
   */
  session: number;
  /** The same count as `session`, for code that reads it outside a render. */
  sessionRef: RefObject<number>;
  handlers: {
    /** Picks the item nearest the pointer. Moves that happened in a
     *  portalled child (a row's own dropdown) are ignored. */
    onMouseMove: (e: React.MouseEvent) => void;
    onMouseEnter: () => void;
    onMouseLeave: () => void;
    /**
     * Routes a click that lands between items (a gap, the padding, past the
     * last row) to the highlighted item, so the highlight and the click agree:
     * what is lit is what a click hits. A click inside an item is left to the
     * item, and so is one on a control between items (a search field) or
     * from a portalled child (a row's own dropdown): it never landed between
     * these items. Disabled items (`isItemDisabled`) are never activated.
     */
    onClick: (e: React.MouseEvent) => void;
  };
  registerItem: (index: number, element: HTMLElement | null) => void;
  /**
   * Invalidates the published rects and runs the hook's coalesced measurement
   * pass again, holding `isMeasured` false until it settles. Reach for it when
   * the rects may be wrong and showing an overlay against them would misplace
   * it: a popup that stays mounted between opens keeps its items registered,
   * so nothing else would notice that its rects were taken while it was
   * hidden. Registration, item resize, and container resize already trigger
   * a pass; do not call this on `children` changes.
   */
  remeasure: () => void;
  /**
   * Re-reads the rects synchronously, keeping `isMeasured` as it is. Only for
   * layout that moves the rows under a visible overlay frame by frame (the
   * accordion re-measures inside its height animation). Everything else
   * wants `remeasure`, or nothing.
   */
  measureItems: () => void;
}

export interface PickNearestInput {
  axis: "x" | "y" | "xy";
  /** The pointer, in viewport coordinates. */
  point: { x: number; y: number };
  /** Item rects in the container's layout space (sparse: unregistered slots
   *  are undefined). */
  rects: readonly (ItemRect | undefined)[];
  /** The container's bounding rect and live scroll / border offsets, which
   *  map layout rects into the pointer's viewport space. */
  containerRect: { left: number; top: number; width: number; height: number };
  scroll: { x: number; y: number };
  border: { x: number; y: number };
  /** Layout size of the container, so a cumulative ancestor `transform:
   *  scale` (a popup mid scale-in) can be factored out per axis. */
  layoutSize: { width: number; height: number };
  /** Skips an item without unregistering it. */
  isDisabled?: (index: number) => boolean;
}

/**
 * The rule, as one pure function: an item the pointer is inside wins;
 * otherwise the item whose center is nearest does, so a pointer in a gap, in
 * the padding, or past the last row still lands. `y` and `x` measure one
 * coordinate; `xy` measures the straight line to each center. Ties keep the
 * first item. The hook calls this once per animation frame; the docs page
 * times it.
 */
export function pickNearest({
  axis,
  point,
  rects,
  containerRect,
  scroll,
  border,
  layoutSize,
  isDisabled,
}: PickNearestInput): number | null {
  const scaleX = layoutSize.width > 0 ? containerRect.width / layoutSize.width : 1;
  const scaleY = layoutSize.height > 0 ? containerRect.height / layoutSize.height : 1;
  let closestIndex: number | null = null;
  let closestDistance = Infinity;
  let containingIndex: number | null = null;

  for (let index = 0; index < rects.length; index++) {
    const r = rects[index];
    if (!r) continue;
    if (isDisabled?.(index)) continue;

    if (axis === "xy") {
      const left = containerRect.left + (border.x + r.left - scroll.x) * scaleX;
      const top = containerRect.top + (border.y + r.top - scroll.y) * scaleY;
      const width = r.width * scaleX;
      const height = r.height * scaleY;
      if (
        point.x >= left &&
        point.x <= left + width &&
        point.y >= top &&
        point.y <= top + height
      ) {
        containingIndex = index;
      }
      const distance = Math.hypot(point.x - (left + width / 2), point.y - (top + height / 2));
      if (distance < closestDistance) {
        closestDistance = distance;
        closestIndex = index;
      }
      continue;
    }

    const horizontal = axis === "x";
    const mousePos = horizontal ? point.x : point.y;
    const scale = horizontal ? scaleX : scaleY;
    const itemStart =
      (horizontal ? containerRect.left : containerRect.top) +
      ((horizontal ? border.x : border.y) +
        (horizontal ? r.left : r.top) -
        (horizontal ? scroll.x : scroll.y)) *
        scale;
    const itemSize = (horizontal ? r.width : r.height) * scale;
    if (mousePos >= itemStart && mousePos <= itemStart + itemSize) {
      containingIndex = index;
    }
    const distance = Math.abs(mousePos - (itemStart + itemSize / 2));
    if (distance < closestDistance) {
      closestDistance = distance;
      closestIndex = index;
    }
  }

  return containingIndex ?? closestIndex;
}

/**
 * Whether an event happened inside the element whose handler hears it.
 * React bubbles events from portalled children (a dropdown opened from a
 * row, a submenu, a tooltip) through their React ancestors, so a list's
 * handlers also hear moves, clicks and focus changes that happened in
 * another layer. The hook's own move and click handlers skip those; a
 * consumer with handlers of its own (focus, keys) can call this too. A bare
 * point with no target (a caller re-picking at a remembered position)
 * counts as the list's own.
 */
export function isOwnEvent(e: { currentTarget?: EventTarget | null; target?: EventTarget | null }) {
  const { currentTarget, target } = e;
  if (!(currentTarget instanceof Node) || !(target instanceof Node)) return true;
  return currentTarget.contains(target);
}

/** Set on the highlighted item (boolean attribute). */
export const ACTIVE_ATTR = "data-fluid-hover-active";

/** Set on the container: the highlighted index, or absent. */
export const ACTIVE_INDEX_ATTR = "data-fluid-hover-active-index";

const ACTIVATOR_SELECTOR =
  "a[href], button, [role='menuitem'], [role='menuitemradio'], [role='menuitemcheckbox'], [role='option'], [role='radio'], [role='checkbox'], [role='tab'], [role='link'], [role='button']";

/**
 * The element a routed click should land on. A registered item is usually
 * the interactive row itself; when it is only a box around one (a sidebar
 * row around its button, a card around its link), the first control inside
 * is what a real click on the row would have reached.
 */
function resolveActivator(element: HTMLElement): HTMLElement {
  if (element.matches(ACTIVATOR_SELECTOR) || element.hasAttribute("tabindex")) {
    return element;
  }
  return element.querySelector<HTMLElement>(ACTIVATOR_SELECTOR) ?? element;
}

const useIsoLayoutEffect =
  typeof window !== "undefined" ? useLayoutEffect : useEffect;

/**
 * How many frames the coalesced remeasure retries while the registered items
 * still have no layout box. A popup can be in the DOM one frame before it is
 * laid out; retrying beats publishing zeroed rects, and the cap keeps a list
 * that stays hidden for good from spinning frames forever.
 */
const measurementAttempts = 3;
export { ACTIVATOR_SELECTOR, resolveActivator, useIsoLayoutEffect, measurementAttempts };
