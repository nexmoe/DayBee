"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useState, useRef, forwardRef, type ReactNode, type HTMLAttributes } from "react";

import { cn } from "@/lib/utils";

import { fontWeights } from "@/lib/font-weight";

import { useSizeVariant } from "@/lib/size-context";
import { useIcons } from "@/lib/icon-context";

import { Button, type ButtonProps } from "@/components/ui/button";
import { Tooltip } from "@/components/ui/tooltip";
import { useSidebar } from "./sidebar-context";
import { SIDEBAR_KEYBOARD_SHORTCUT_RIGHT } from "./sidebar-context";
import { SIDEBAR_KEYBOARD_SHORTCUT } from "./sidebar-context";
import { SIDEBAR_MIN_WIDTH } from "./sidebar-context";
import { SIDEBAR_COLLAPSE_SLOP } from "./sidebar-context";
import { SIDEBAR_MAX_WIDTH } from "./sidebar-context";

// ─── SidebarTrigger ──────────────────────────────────────────────────────────

export type SidebarTriggerProps = ButtonProps;

/** Keystroke chip rendered inside the (inverted) tooltip surface. */
function ShortcutKbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="-my-1 flex h-4 min-w-4 items-center justify-center rounded border border-background/30 px-1 font-sans text-[length:var(--fs-micro-compact,10px)] leading-[var(--lh-micro-compact,12px)] text-background">
      {children}
    </kbd>
  );
}

/** The tooltips always show the toggle keystroke, falling back to the
 *  side's default key even when the provider's binding is disabled. */
function useShortcutKey(): string {
  const { side, shortcut } = useSidebar();
  return (
    shortcut ??
    (side === "right" ? SIDEBAR_KEYBOARD_SHORTCUT_RIGHT : SIDEBAR_KEYBOARD_SHORTCUT)
  );
}

/** Ghost icon button calling toggleSidebar(). The icon mirrors the
 *  sidebar's side, and its tooltip names the action with the toggle
 *  keystroke by default. */
const SidebarTrigger = forwardRef<HTMLButtonElement, SidebarTriggerProps>(
  ({ onClick, size, children, ...props }, ref) => {
    const {
      toggleSidebar,
      open,
      openMobile,
      isMobile,
      side,
      peek,
      isPeeking,
      schedulePeek,
      cancelPeekTimer,
    } = useSidebar();
    const shortcutKey = useShortcutKey();
    // With hover-peek enabled, the COLLAPSED trigger is a peek affordance
    // too: resting on it floats the rail out exactly like the edge strip —
    // same shared intent timer, so moving from the trigger into the peeked
    // card (or back) cancels the pending dismissal.
    const hoverPeek = peek === "hover" && !isMobile && !open;
    const icons = useIcons();
    const PanelLeftIcon = icons["panel-left"];
    const PanelRightIcon = icons["panel-right"];
    const TriggerIcon = side === "right" ? PanelRightIcon : PanelLeftIcon;
    const iconSize = useSizeVariant() === "compact" ? ("icon-compact" as const) : ("icon" as const);
    const collapsed = isMobile ? !openMobile : !open;

    return (
      <Tooltip
        side="bottom"
        content={
          <span className="flex items-center gap-1.5">
            {/* A flex row escapes the surface's text-box trim, so the label
                re-applies it — otherwise the shortcut row would sit taller
                than a tooltip without a chip. */}
            <span className="[text-box:trim-both_cap_alphabetic]">
              {collapsed ? "Expand sidebar" : "Collapse sidebar"}
            </span>
            <ShortcutKbd>{shortcutKey}</ShortcutKbd>
          </span>
        }
      >
        <Button
          ref={ref}
          variant="ghost"
          size={size ?? iconSize}
          data-sidebar="trigger"
          aria-label="Toggle Sidebar"
          onClick={(event) => {
            onClick?.(event);
            toggleSidebar();
          }}
          onPointerEnter={
            hoverPeek
              ? (event: React.PointerEvent) => {
                  if (event.pointerType !== "mouse") return;
                  if (isPeeking) cancelPeekTimer();
                  else schedulePeek();
                }
              : undefined
          }
          // While PEEKED the shell's geometric watcher owns dismissal — a
          // leave fired here can be the peek card sliding over a stationary
          // cursor (layout-driven boundary event, no accompanying move to
          // disarm it), which would flicker the peek closed and open again.
          // This leave only retires a pending intent timer.
          onPointerLeave={
            hoverPeek
              ? () => {
                  if (!isPeeking) cancelPeekTimer();
                }
              : undefined
          }
          {...props}
        >
          {children ?? <TriggerIcon />}
        </Button>
      </Tooltip>
    );
  }
);

SidebarTrigger.displayName = "SidebarTrigger";

// ─── SidebarRail ─────────────────────────────────────────────────────────────

export interface SidebarRailProps extends HTMLAttributes<HTMLButtonElement> {
  /** Pin the tooltip open/closed; `undefined` leaves it on hover. */
  tooltipOpen?: boolean;
}

/** The grab strip on the sidebar's inner edge, rendered by default inside
 *  the desktop shell: drag it to resize (clamped), click it to collapse, and
 *  its tooltip explains both with the toggle keystroke. Hovering it
 *  brightens the edge border. */
const SidebarRail = forwardRef<HTMLButtonElement, SidebarRailProps>(
  ({ className, tooltipOpen, ...props }, ref) => {
    const { toggleSidebar, setOpen, setWidth, side, setIsResizing } = useSidebar();
    const shortcutKey = useShortcutKey();
    const railRef = useRef<HTMLButtonElement | null>(null);
    const dragRef = useRef<{ startX: number; startWidth: number; moved: boolean; collapsed: boolean } | null>(null);
    const [dragging, setDragging] = useState(false);

    const onPointerDown = (event: React.PointerEvent<HTMLButtonElement>) => {
      const panel = railRef.current?.closest('[data-slot="sidebar"]') as HTMLElement | null;
      if (!panel) return;
      dragRef.current = { startX: event.clientX, startWidth: panel.offsetWidth, moved: false, collapsed: false };
      event.currentTarget.setPointerCapture(event.pointerId);
    };

    const onPointerMove = (event: React.PointerEvent<HTMLButtonElement>) => {
      const drag = dragRef.current;
      if (!drag) return;
      const dx = event.clientX - drag.startX;
      if (!drag.moved && Math.abs(dx) < 4) return;
      if (!drag.moved) {
        drag.moved = true;
        setDragging(true);
        setIsResizing(true);
      }
      const delta = side === "left" ? dx : -dx;
      const raw = drag.startWidth + delta;
      // Dragged well past the minimum, toward the edge: preview the collapse
      // but keep the drag session alive — pulling back past the threshold
      // re-expands, so an overshoot isn't committed until release.
      if (raw < SIDEBAR_MIN_WIDTH - SIDEBAR_COLLAPSE_SLOP) {
        if (!drag.collapsed) {
          drag.collapsed = true;
          setWidth(`${SIDEBAR_MIN_WIDTH}px`);
          setOpen(false);
        }
        return;
      }
      if (drag.collapsed) {
        drag.collapsed = false;
        setOpen(true);
      }
      const next = Math.max(SIDEBAR_MIN_WIDTH, Math.min(SIDEBAR_MAX_WIDTH, raw));
      setWidth(`${next}px`);
    };

    const onPointerUp = (event: React.PointerEvent<HTMLButtonElement>) => {
      const drag = dragRef.current;
      dragRef.current = null;
      event.currentTarget.releasePointerCapture(event.pointerId);
      setDragging(false);
      setIsResizing(false);
      // A press that never turned into a drag is the collapse click.
      if (drag && !drag.moved) toggleSidebar();
    };

    // A cancelled pointer (touch interruption, capture loss) ends the drag
    // where it stands — no collapse-click, capture is already released.
    const onPointerCancel = () => {
      dragRef.current = null;
      setDragging(false);
      setIsResizing(false);
    };

    const semibold = { fontVariationSettings: fontWeights.semibold };

    return (
      <Tooltip
        side={side === "left" ? "right" : "left"}
        sideOffset={8}
        followCursor="y"
        forceOpen={dragging ? false : tooltipOpen}
        content={
          // The flex column escapes the tooltip surface's text-box trim, so
          // BOTH lines re-apply it and the gap alone carries the line rhythm
          // — an untrimmed line would smuggle its half-leading back in as
          // lopsided padding.
          <span className="flex flex-col items-start gap-2">
            <span className="[text-box:trim-both_cap_alphabetic]">
              <span style={semibold}>Drag</span> to resize
            </span>
            <span className="flex items-center gap-1.5">
              <span className="[text-box:trim-both_cap_alphabetic]">
                <span style={semibold}>Click</span> to collapse
              </span>
              <ShortcutKbd>{shortcutKey}</ShortcutKbd>
            </span>
          </span>
        }
      >
        <button
          ref={(node) => {
            railRef.current = node;
            if (typeof ref === "function") ref(node);
            else if (ref) (ref as React.MutableRefObject<HTMLButtonElement | null>).current = node;
          }}
          type="button"
          data-sidebar="rail"
          aria-label="Resize or collapse sidebar"
          tabIndex={-1}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerCancel}
          className={cn(
            // touch-none: without it the browser claims a touch drag for
            // scrolling and pointercancels the resize mid-gesture.
            "absolute inset-y-0 z-20 w-2 cursor-col-resize touch-none outline-none",
            // Positioned from context (not group-data selectors) so variant
            // offsets passed via className can win the merge.
            side === "left" ? "right-0" : "left-0",
            // Hovering brightens the edge border the shell draws by default;
            // a pinned tooltip brightens it too, so the spotlight reads as
            // the hover it stands in for.
            "after:absolute after:inset-y-0 after:w-px after:bg-transparent hover:after:bg-foreground/25 after:transition-colors after:duration-80",
            tooltipOpen && "after:bg-foreground/25",
            side === "left" ? "after:right-0" : "after:left-0",
            className
          )}
          {...props}
        />
      </Tooltip>
    );
  }
);

SidebarRail.displayName = "SidebarRail";
export { ShortcutKbd, useShortcutKey, SidebarTrigger, SidebarRail };
