"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useState, useEffect, useCallback, useMemo, useRef, forwardRef, type CSSProperties, type HTMLAttributes } from "react";

import { cn } from "@/lib/utils";

import { SIDEBAR_WIDTH } from "./sidebar-context";
import { SIDEBAR_WIDTH_MOBILE } from "./sidebar-context";
import { useIsMobile } from "./sidebar-context";
import type { SidebarSide } from "./sidebar-context";
import { mountedProviders } from "./sidebar-context";
import { SIDEBAR_KEYBOARD_SHORTCUT_RIGHT } from "./sidebar-context";
import { SIDEBAR_KEYBOARD_SHORTCUT } from "./sidebar-context";
import { useIsoLayoutEffect } from "./sidebar-context";
import { SIDEBAR_COOKIE_NAME } from "./sidebar-context";
import { SIDEBAR_COOKIE_MAX_AGE } from "./sidebar-context";
import type { SidebarContextValue } from "./sidebar-context";
import { SidebarContext } from "./sidebar-context";

// ─── SidebarProvider ─────────────────────────────────────────────────────────

export interface SidebarProviderProps extends HTMLAttributes<HTMLDivElement> {
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  /** Persist the desktop open state to the `sidebar_state` cookie so a server
   *  layout can read it back into `defaultOpen`. Mobile state never persists. */
  persist?: boolean;
  /** Bare-key toggle shortcut. Defaults to "[" for a left sidebar and "]"
   *  for a right one; `null` disables it. */
  shortcut?: string | null;
  /** Viewport width (px) below which the sidebar renders as a drawer. */
  mobileBreakpoint?: number;
  /** While collapsed, reveal the sidebar as a floating overlay from the
   *  edge — on hover (with intent delay) or on click of the edge strip.
   *  Peeking never pins the sidebar or writes the cookie. @default "none" */
  peek?: "hover" | "click" | "none";
  width?: string;
  widthMobile?: string;
}

const SidebarProvider = forwardRef<HTMLDivElement, SidebarProviderProps>(
  (
    {
      defaultOpen = true,
      open: openProp,
      onOpenChange,
      persist = true,
      shortcut: shortcutProp,
      mobileBreakpoint = 768,
      peek = "none",
      width: widthProp = SIDEBAR_WIDTH,
      widthMobile = SIDEBAR_WIDTH_MOBILE,
      className,
      style,
      children,
      ...props
    },
    ref
  ) => {
    const isMobile = useIsMobile(mobileBreakpoint);
    const [openMobile, setOpenMobile] = useState(false);
    const [side, setSide] = useState<SidebarSide>("left");
    const wrapperRef = useRef<HTMLDivElement | null>(null);

    useEffect(() => {
      const el = wrapperRef.current;
      if (!el) return;
      mountedProviders.push(el);
      return () => {
        const i = mountedProviders.indexOf(el);
        if (i !== -1) mountedProviders.splice(i, 1);
      };
    }, []);
    const registerSide = useCallback((next: SidebarSide) => setSide(next), []);

    // Live width: the prop is the starting point, the rail's drag-resize
    // updates it at runtime. A new prop resets it, during that render.
    const [width, setWidth] = useState(widthProp);
    const [syncedWidthProp, setSyncedWidthProp] = useState(widthProp);
    if (syncedWidthProp !== widthProp) {
      setSyncedWidthProp(widthProp);
      setWidth(widthProp);
    }
    const [isResizing, setIsResizing] = useState(false);

    // Default shortcut mirrors the sidebar's edge: "[" left, "]" right.
    const shortcut =
      shortcutProp === undefined
        ? side === "right"
          ? SIDEBAR_KEYBOARD_SHORTCUT_RIGHT
          : SIDEBAR_KEYBOARD_SHORTCUT
        : shortcutProp;

    const [internalOpen, setInternalOpen] = useState(defaultOpen);
    const open = openProp ?? internalOpen;
    const openRef = useRef(open);
    useIsoLayoutEffect(() => {
      openRef.current = open;
    });

    const setOpen = useCallback(
      (value: boolean | ((prev: boolean) => boolean)) => {
        const next = typeof value === "function" ? value(openRef.current) : value;
        if (onOpenChange) onOpenChange(next);
        else setInternalOpen(next);
        if (persist) {
          document.cookie = `${SIDEBAR_COOKIE_NAME}=${next}; path=/; max-age=${SIDEBAR_COOKIE_MAX_AGE}`;
        }
      },
      [onOpenChange, persist]
    );

    const toggleSidebar = useCallback(() => {
      if (isMobile) setOpenMobile((prev) => !prev);
      else setOpen((prev) => !prev);
    }, [isMobile, setOpen]);

    // Collapsed-peek overlay state. Pinning the sidebar open (or disabling
    // the mode) always dismisses the peek — including a PENDING intent
    // timer, or a hover armed just before the pin would fire setIsPeeking on
    // an open sidebar (the effect's deps never re-run for the late timer).
    // The peek itself is cleared during the render that pins or disables
    // it; the timer is cleared after that commit.
    const [isPeeking, setIsPeeking] = useState(false);
    const peekTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    if ((open || peek === "none") && isPeeking) setIsPeeking(false);
    useEffect(() => {
      if (open || peek === "none") {
        if (peekTimerRef.current) clearTimeout(peekTimerRef.current);
        peekTimerRef.current = null;
      }
    }, [open, peek]);
    const cancelPeekTimer = useCallback(() => {
      if (peekTimerRef.current) clearTimeout(peekTimerRef.current);
      peekTimerRef.current = null;
    }, []);
    const schedulePeek = useCallback(() => {
      cancelPeekTimer();
      peekTimerRef.current = setTimeout(() => setIsPeeking(true), 150);
    }, [cancelPeekTimer]);
    const scheduleDismissPeek = useCallback(() => {
      cancelPeekTimer();
      peekTimerRef.current = setTimeout(() => setIsPeeking(false), 250);
    }, [cancelPeekTimer]);
    useEffect(() => cancelPeekTimer, [cancelPeekTimer]);

    // The bare shortcut key toggles the sidebar app-wide. Bound to the
    // provider's lifetime (not a docs-only global), skipped while typing, and
    // skipped when a modifier is held so ⌘[ / ⌘] keep their browser meaning.
    useEffect(() => {
      if (shortcut == null) return;
      const onKeyDown = (event: KeyboardEvent) => {
        if (event.key.toLowerCase() !== shortcut.toLowerCase()) return;
        if (event.metaKey || event.ctrlKey || event.altKey) return;
        const target = event.target as HTMLElement;
        if (
          target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable
        )
          return;
        // Only one mounted provider answers (see mountedProviders). Providers
        // can NEST (an app shell wrapping doc previews), so containment alone
        // isn't enough: the innermost provider containing focus wins.
        const root = wrapperRef.current;
        if (!root) return;
        if (root.contains(target)) {
          if (
            mountedProviders.some(
              (el) => el !== root && root.contains(el) && el.contains(target)
            )
          )
            return;
          // The focused element itself WRAPS another provider (a docs
          // preview frame holding keyboard scope for the demo inside it —
          // see click-to-focus): the wrapped provider answers, not this
          // ancestor shell.
          if (mountedProviders.some((el) => el !== root && target.contains(el)))
            return;
        } else {
          // Focus outside this provider. A focused element that WRAPS
          // providers — a docs preview frame holding keyboard scope for the
          // demo inside it (see click-to-focus) — scopes the key to what it
          // wraps: the outermost wrapped provider answers, and this check
          // must come BEFORE the containment guard below (an app-shell
          // provider always contains the preview frame, and must not steal
          // the key from the demo the frame scopes to). Focus on <body>
          // wraps every provider, which degenerates to the original rule:
          // the OUTERMOST provider overall answers — mount order is
          // unreliable here (a persistent app-shell provider mounts once,
          // while doc demos mount later on client-side navigation).
          const wrapped = mountedProviders.filter((el) => target.contains(el));
          if (wrapped.length > 0) {
            const outermost = wrapped.find(
              (el) => !wrapped.some((other) => other !== el && other.contains(el))
            );
            if (outermost !== root) return;
          } else {
            if (mountedProviders.some((el) => el !== root && el.contains(target)))
              return;
            const outermost = mountedProviders.find(
              (el) =>
                !mountedProviders.some((other) => other !== el && other.contains(el))
            );
            if (outermost !== root) return;
          }
        }
        event.preventDefault();
        toggleSidebar();
      };
      window.addEventListener("keydown", onKeyDown);
      return () => window.removeEventListener("keydown", onKeyDown);
    }, [shortcut, toggleSidebar]);

    const value = useMemo<SidebarContextValue>(
      () => ({
        state: open ? "expanded" : "collapsed",
        open,
        setOpen,
        openMobile,
        setOpenMobile,
        isMobile,
        toggleSidebar,
        width,
        setWidth,
        widthMobile,
        mobileBreakpoint,
        side,
        registerSide,
        shortcut,
        peek,
        isPeeking,
        setIsPeeking,
        schedulePeek,
        scheduleDismissPeek,
        cancelPeekTimer,
        isResizing,
        setIsResizing,
      }),
      [
        open,
        setOpen,
        openMobile,
        isMobile,
        toggleSidebar,
        width,
        widthMobile,
        mobileBreakpoint,
        side,
        registerSide,
        shortcut,
        peek,
        isPeeking,
        schedulePeek,
        scheduleDismissPeek,
        cancelPeekTimer,
        isResizing,
      ]
    );

    return (
      <SidebarContext.Provider value={value}>
        <div
          ref={(node) => {
            wrapperRef.current = node;
            if (typeof ref === "function") ref(node);
            else if (ref) (ref as React.MutableRefObject<HTMLDivElement | null>).current = node;
          }}
          data-slot="sidebar-wrapper"
          className={cn("group/sidebar-wrapper relative flex min-h-svh w-full", className)}
          style={
            {
              "--sidebar-width": width,
              "--sidebar-width-mobile": widthMobile,
              ...style,
            } as CSSProperties
          }
          {...props}
        >
          {children}
        </div>
      </SidebarContext.Provider>
    );
  }
);

SidebarProvider.displayName = "SidebarProvider";
export { SidebarProvider };
