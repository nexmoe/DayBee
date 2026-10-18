"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useState, useEffect, useRef, forwardRef, type CSSProperties, type HTMLAttributes } from "react";
import { motion, AnimatePresence, useReducedMotion } from "framer-motion";
import { cn } from "@/lib/utils";
import { spring, exitFallbackMs } from "@/lib/springs";

import { useShape } from "@/lib/shape-context";

import { useSurface, SurfaceProvider } from "@/lib/surface-context";
import { surfaceClasses } from "@/lib/surface-classes";

import type { SidebarSide } from "./sidebar-context";
import type { SidebarVariant } from "./sidebar-context";
import { useSidebar } from "./sidebar-context";
import { SidebarRail } from "./sidebar-controls";

// ─── SidebarShell (shared desktop DOM for both flavors) ──────────────────────

// Literal map so Tailwind's scanner emits the utilities: for the standard
// breakpoints the shell is also hidden by CSS, avoiding a pre-hydration flash
// of the rail on small screens. Non-standard breakpoints fall back to a
// JS-driven `hidden` in the shell (no dissolve, but never rail + drawer at
// once).
// Crossing the drawer breakpoint DISSOLVES the desktop rail instead of
// snapping it away: opacity fades while `display` rides the same transition
// with allow-discrete, so none applies only once the fade lands (and
// @starting-style fades it back in when the window grows). Literal classes
// per breakpoint — Tailwind's scanner can't see composed strings.
const BREAKPOINT_FADE_BASE =
  "transition-[opacity,display] ease-out [transition-behavior:allow-discrete] motion-reduce:transition-none";

const BREAKPOINT_HIDDEN: Record<number, string> = {
  640: "max-sm:hidden max-sm:opacity-0 max-sm:duration-160 sm:duration-240 sm:starting:opacity-0",
  768: "max-md:hidden max-md:opacity-0 max-md:duration-160 md:duration-240 md:starting:opacity-0",
  1024: "max-lg:hidden max-lg:opacity-0 max-lg:duration-160 lg:duration-240 lg:starting:opacity-0",
  1280: "max-xl:hidden max-xl:opacity-0 max-xl:duration-160 xl:duration-240 xl:starting:opacity-0",
};

// Props framer-motion redefines with incompatible signatures; they must not
// be forwarded onto a motion.div.
type MotionSafeDivProps = Omit<
  HTMLAttributes<HTMLDivElement>,
  | "onDrag"
  | "onDragStart"
  | "onDragEnd"
  | "onAnimationStart"
  | "onAnimationEnd"
  | "onAnimationIteration"
>;

export interface SidebarShellProps extends MotionSafeDivProps {
  side: SidebarSide;
  variant: SidebarVariant;
  /** The `sidebar` variant's inner-edge border. Default true. */
  bordered?: boolean;
  /** Render the built-in resize/collapse rail handle. `false` hides it and
   *  disables drag-resize — the trigger and keyboard shortcut still toggle. */
  rail?: boolean;
  /** Pin the rail's tooltip open (`true`) or closed (`false`); `undefined`
   *  leaves it on hover. Dragging always hides it. */
  railTooltipOpen?: boolean;
}

/** Internal: the expanded/collapsed desktop rail. An in-flow sticky column
 *  animates its width (this is what reflows the inset) while the fixed-width
 *  panel inside slides out under overflow clipping — container-relative, so
 *  the whole sidebar works inside any bounded frame, not just the viewport.
 *  Ships the resize/collapse rail handle on its inner edge by default. */
const SidebarShell = forwardRef<HTMLDivElement, SidebarShellProps>(
  ({ side, variant, bordered = true, rail = true, railTooltipOpen, className, children, ...props }, ref) => {
    const {
      open,
      width,
      mobileBreakpoint,
      isMobile,
      isResizing,
      peek,
      isPeeking,
      setIsPeeking,
      schedulePeek,
      scheduleDismissPeek,
      cancelPeekTimer,
    } = useSidebar();
    const shape = useShape();
    const shellRef = useRef<HTMLDivElement | null>(null);

    // Collapsed-peek: the edge strip reveals the sidebar as a floating
    // overlay without pinning it. Hover mode uses small intent/leave delays;
    // both modes dismiss on Escape or an outside press.
    // `!isResizing`: a drag can preview the collapsed state mid-gesture; the
    // shell must not swap to the peek strip then, or it would unmount the
    // rail holding the pointer capture and kill the drag.
    const peekEnabled = peek !== "none" && !open && !isResizing;
    useEffect(() => {
      if (!(peekEnabled && isPeeking)) return;
      const onKeyDown = (event: KeyboardEvent) => {
        if (event.key === "Escape") setIsPeeking(false);
      };
      const onPointerDown = (event: PointerEvent) => {
        if (!shellRef.current?.contains(event.target as Node)) setIsPeeking(false);
      };
      // Hover mode holds the peek by geometric containment, not
      // enter/leave: a portalled tooltip or menu covering the card steals
      // the hit-test and fires pointerleave on the shell even though the
      // cursor never left the sidebar. Dismissal is armed only when the
      // pointer's position actually crosses out of the overlay's box (with
      // a little margin), and disarmed the moment it crosses back.
      let wasInside = true;
      let box: DOMRect | null = null;
      let moveFrame = 0;
      let latestMove: PointerEvent | null = null;
      const overlayOf = () =>
        shellRef.current?.querySelector('[data-sidebar="peek"]') ?? shellRef.current;
      const measure = () => {
        const overlay = overlayOf();
        box = overlay ? overlay.getBoundingClientRect() : null;
      };
      const resizeObserver = new ResizeObserver(measure);
      const overlay = overlayOf();
      if (overlay) resizeObserver.observe(overlay);
      measure();
      const flushMove = () => {
        moveFrame = 0;
        const event = latestMove;
        if (!event) return;
        if (!box) measure();
        if (!box) return;
        const inside =
          event.clientX >= box.left - 8 &&
          event.clientX <= box.right + 8 &&
          event.clientY >= box.top - 8 &&
          event.clientY <= box.bottom + 8;
        if (inside) {
          // Unconditional (not transition-gated): another surface's leave —
          // the hover-peek trigger's, say — may have armed a dismissal while
          // the pointer was already inside the box.
          wasInside = true;
          cancelPeekTimer();
        } else if (wasInside) {
          wasInside = false;
          scheduleDismissPeek();
        }
      };
      const onPointerMove = (event: PointerEvent) => {
        latestMove = event;
        if (moveFrame) return;
        moveFrame = requestAnimationFrame(flushMove);
      };
      document.addEventListener("keydown", onKeyDown);
      document.addEventListener("pointerdown", onPointerDown);
      if (peek === "hover") document.addEventListener("pointermove", onPointerMove);
      return () => {
        if (moveFrame) cancelAnimationFrame(moveFrame);
        resizeObserver.disconnect();
        document.removeEventListener("keydown", onKeyDown);
        document.removeEventListener("pointerdown", onPointerDown);
        document.removeEventListener("pointermove", onPointerMove);
      };
    }, [peekEnabled, isPeeking, setIsPeeking, peek, cancelPeekTimer, scheduleDismissPeek]);
    const substrate = useSurface();
    const floatingLevel = Math.min(substrate + 1, 8);
    // Drag-resize needs the panel glued to the pointer; the spring resumes
    // for open/close. Reduced motion snaps instead of sliding — the state
    // change stays legible without the 256px of travel. The open/close ride
    // the SLOW tier: a whole column moving is the largest thing this
    // component animates (the sheet and peek stay on moderate — drawers
    // settle precisely, per the tier notes).
    const reduceMotion = useReducedMotion() ?? false;
    // Mid-drag open flips — the collapse preview and its drag-back rescue —
    // ride the moderate tier instead of the drag's glued duration-0 tracking.
    // The flip is detected synchronously (transition must be right on the
    // very commit whose animate target changes; effects run too late), then
    // `dragFlip` holds the spring through its settle so pointer moves landing
    // right after a flip retarget the spring instead of snapping.
    // Counts drag flips, so each one restarts the hold; 0 is no hold.
    const [dragFlip, setDragFlip] = useState(0);
    // Pinning open from an active peek: the panel is already fully on screen
    // as the overlay card, so while the width spring makes room the shell
    // must not clip — otherwise the visible sidebar wipes in from a mask it
    // never left. The state hold keeps the clip off through the spring.
    const [pinFromPeekHold, setPinFromPeekHold] = useState(false);
    // Both holds start in the render whose `open` flipped (comparing with
    // the last render's `open` and `isPeeking`, kept as state): the provider
    // clears isPeeking in that same render, so only the previous render
    // still knows the card was up.
    const [prevOpen, setPrevOpen] = useState(open);
    const [wasPeeking, setWasPeeking] = useState(isPeeking);
    if (prevOpen !== open) {
      setPrevOpen(open);
      if (isResizing) setDragFlip((n) => n + 1);
      if (open && wasPeeking) setPinFromPeekHold(true);
    }
    if (wasPeeking !== isPeeking) setWasPeeking(isPeeking);
    if (!isResizing && dragFlip !== 0) setDragFlip(0);
    useEffect(() => {
      if (dragFlip === 0) return;
      const id = setTimeout(() => setDragFlip(0), exitFallbackMs(spring.moderate));
      return () => clearTimeout(id);
    }, [dragFlip]);
    useEffect(() => {
      if (!pinFromPeekHold) return;
      const id = setTimeout(() => setPinFromPeekHold(false), exitFallbackMs(spring.slow));
      return () => clearTimeout(id);
    }, [pinFromPeekHold]);
    const pinnedFromPeek = pinFromPeekHold;
    const widthTransition = reduceMotion
      ? { duration: 0 }
      : isResizing
        ? dragFlip !== 0
          ? open
            ? spring.moderate
            : spring.moderate.exit
          : { duration: 0 }
        : open
          ? spring.slow
          : spring.slow.exit;

    return (
      <motion.div
        ref={(node: HTMLDivElement | null) => {
          shellRef.current = node;
          if (typeof ref === "function") ref(node);
          else if (ref) (ref as React.MutableRefObject<HTMLDivElement | null>).current = node;
        }}
        data-slot="sidebar"
        data-state={open ? "expanded" : "collapsed"}
        data-collapsible={open ? "" : "offcanvas"}
        data-variant={variant}
        data-side={side}
        className={cn(
          // No bare `group` here: an unnamed group on the whole rail would
          // fire every descendant's group-hover (Button fills, icon strokes)
          // on rail hover. Named groups (menu-item etc.) handle row states.
          "peer shrink-0 sticky top-0 h-svh",
          // While peek is armed the 0-width shell must not clip the edge
          // strip or the overlay card — and the shell must rise above the
          // inset (a later sibling) so the card paints over it. Pinning from
          // a peek keeps both through the width spring for the same reason.
          peekEnabled || pinnedFromPeek ? "z-40" : "overflow-hidden",
          // Flex order (not DOM order) decides the side, so consumers can
          // keep Sidebar before SidebarInset regardless of `side`.
          side === "right" && "order-last",
          // The inset rail has no card edge of its own, so its scroll hairline
          // hugs the rows' 8px gutter instead of running panel-wide.
          variant === "inset" && "[--scroll-divider-inset:8px]",
          BREAKPOINT_FADE_BASE,
          BREAKPOINT_HIDDEN[mobileBreakpoint],
          // A non-standard breakpoint has no literal utility in the map, so
          // JS hides the shell (no fade, but never rail + drawer at once).
          !BREAKPOINT_HIDDEN[mobileBreakpoint] && isMobile && "hidden",
          className
        )}
        initial={false}
        animate={{ width: open ? width : "0rem" }}
        transition={widthTransition}
        // Hover-mode dismissal lives on the shell root: the pointer can land
        // on the overlay without ever crossing it (the card slides in under
        // a stationary cursor), so per-element leave events are unreliable —
        // leaving the shell subtree is the signal that matters.
        onPointerEnter={peekEnabled && peek === "hover" ? cancelPeekTimer : undefined}
        // While PEEKED, dismissal belongs to the geometric watcher above —
        // leave events lie whenever portalled content (tooltip, menu) covers
        // the card. This leave handler only retires a pending peek-arm when
        // the cursor departs before the intent delay lands.
        onPointerLeave={
          peekEnabled && peek === "hover"
            ? () => {
                if (!isPeeking) cancelPeekTimer();
              }
            : undefined
        }
        {...props}
      >
        {peekEnabled ? (
          <>
            {/* Edge strip: the collapsed sidebar's reveal affordance. A thin
                hairline brightens on hover; hover mode peeks after a short
                intent delay, click mode on press. */}
            <button
              type="button"
              aria-label="Peek sidebar"
              aria-expanded={isPeeking}
              className={cn(
                "group/peek-strip absolute inset-y-0 z-40 w-3 cursor-pointer outline-none",
                side === "left" ? "left-0" : "right-0"
              )}
              onPointerEnter={
                peek === "hover"
                  ? (event) => {
                      if (event.pointerType === "mouse") schedulePeek();
                    }
                  : undefined
              }
              onClick={() => {
                cancelPeekTimer();
                setIsPeeking(true);
              }}
            >
              <span
                aria-hidden="true"
                className={cn(
                  "absolute inset-y-0 w-px bg-border opacity-0 transition-opacity duration-80 group-hover/peek-strip:opacity-100 group-focus-visible/peek-strip:opacity-100",
                  side === "left" ? "left-0" : "right-0"
                )}
              />
            </button>
            <AnimatePresence>
              {isPeeking && (
                <motion.div
                  data-sidebar="peek"
                  className={cn(
                    "absolute inset-y-2 z-50 flex flex-col overflow-hidden",
                    // The card's edge inset matches where the PINNED rail's
                    // content sits, so pinning from a peek never shifts the
                    // rows sideways: floating pins into a card inset by the
                    // same gutter; inset/sidebar pin flush to the edge.
                    side === "left"
                      ? variant === "floating"
                        ? "left-2"
                        : "left-0"
                      : variant === "floating"
                        ? "right-2"
                        : "right-0",
                    shape.container,
                    surfaceClasses(floatingLevel, 3)
                  )}
                  style={{
                    width: `calc(${width} - ${variant === "floating" ? "1rem" : "0.5rem"})`,
                  }}
                  initial={reduceMotion ? false : { x: side === "left" ? "-108%" : "108%" }}
                  animate={{ x: 0 }}
                  exit={{
                    x: side === "left" ? "-108%" : "108%",
                    transition: reduceMotion ? { duration: 0 } : spring.moderate.exit,
                  }}
                  transition={reduceMotion ? { duration: 0 } : spring.moderate}
                >
                  <SurfaceProvider value={floatingLevel}>{children}</SurfaceProvider>
                </motion.div>
              )}
            </AnimatePresence>
          </>
        ) : (
        <motion.div
          className={cn(
            "absolute inset-y-0 flex h-full flex-col",
            side === "left" ? "left-0" : "right-0",
            // Floating floats its card inside a full gutter; inset only needs
            // the vertical inset (horizontal room belongs to the nav rows).
            variant === "floating" && "p-2",
            variant === "inset" && "py-2"
          )}
          style={{ width }}
          initial={false}
          animate={{ x: open ? "0%" : side === "left" ? "-100%" : "100%" }}
          transition={widthTransition}
        >
          {variant === "floating" ? (
            <div
              data-sidebar="sidebar"
              className={cn(
                "flex h-full w-full min-h-0 flex-col",
                shape.container,
                surfaceClasses(floatingLevel, 3)
              )}
            >
              <SurfaceProvider value={floatingLevel}>{children}</SurfaceProvider>
            </div>
          ) : (
            <div
              data-sidebar="sidebar"
              className={cn(
                "flex h-full w-full min-h-0 flex-col",
                bordered &&
                  variant === "sidebar" &&
                  (side === "left" ? "border-r border-border" : "border-l border-border")
              )}
            >
              {children}
            </div>
          )}
          {rail && (
          <SidebarRail
            tooltipOpen={railTooltipOpen}
            className={cn(
              // The floating card sits inside the panel's p-2 gutter, so the
              // grab strip (and its hover hairline) moves in to straddle the
              // card's edge instead of the panel's.
              variant === "floating" &&
                (side === "left" ? "right-1 after:right-[3.5px]" : "left-1 after:left-[3.5px]"),
              // Cards are vertically inset and rounded — the hover hairline
              // hugs the card's straight run: fully transparent through the
              // corner radius, then fading in over 24px (mirrored at the
              // bottom). The radius rides the shape system via CSS vars.
              variant !== "sidebar" &&
                "after:inset-y-2 after:[mask-image:linear-gradient(to_bottom,transparent_var(--rail-fade-start),black_var(--rail-fade-end),black_calc(100%-var(--rail-fade-end)),transparent_calc(100%-var(--rail-fade-start)))]"
            )}
            style={
              variant !== "sidebar"
                ? ({
                    "--rail-fade-start": `${shape.bgRadius >= 20 ? 24 : 12}px`,
                    "--rail-fade-end": `${(shape.bgRadius >= 20 ? 24 : 12) + 24}px`,
                  } as CSSProperties)
                : undefined
            }
          />
          )}
        </motion.div>
        )}
      </motion.div>
    );
  }
);

SidebarShell.displayName = "SidebarShell";
export { BREAKPOINT_FADE_BASE, BREAKPOINT_HIDDEN, SidebarShell };
export type { MotionSafeDivProps };
