"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useContext, useState, useCallback, useMemo, useRef, forwardRef, type HTMLAttributes, type LiHTMLAttributes, type RefObject } from "react";

import { cn } from "@/lib/utils";

import { SizeProvider, type SizeVariant } from "@/lib/size-context";

import { useMenuScope } from "./sidebar-menu-scope";
import { MenuScopeContext } from "./sidebar-menu-context";
import { useIsoLayoutEffect } from "./sidebar-menu-context";
import { MenuItemContext } from "./sidebar-menu-context";

// ─── SidebarMenu ─────────────────────────────────────────────────────────────

export interface SidebarMenuProps extends HTMLAttributes<HTMLUListElement> {
  /** Pins the menu's rows to one step of the size ladder. Omitted, they
   *  follow the surrounding SizeProvider. */
  size?: SizeVariant;
  /** Draw the traveling keyboard focus ring. Off, keyboard focus moves the
   *  hover background only. @default true */
  focusRing?: boolean;
}

const SidebarMenu = forwardRef<HTMLUListElement, SidebarMenuProps>(
  ({ className, size, focusRing, children, ...props }, ref) => {
    const containerRef = useRef<HTMLUListElement>(null);
    const { value, containerProps, overlays } = useMenuScope(containerRef, { focusRing });

    const content = (
      <MenuScopeContext.Provider value={value}>
        <ul
          ref={(node) => {
            containerRef.current = node;
            if (typeof ref === "function") ref(node);
            else if (ref) (ref as React.MutableRefObject<HTMLUListElement | null>).current = node;
          }}
          data-sidebar="menu"
          className={cn("relative flex w-full min-w-0 flex-col select-none", className)}
          {...containerProps}
          {...props}
        >
          {overlays}
          {children}
        </ul>
      </MenuScopeContext.Provider>
    );

    return size ? <SizeProvider size={size}>{content}</SizeProvider> : content;
  }
);

SidebarMenu.displayName = "SidebarMenu";

// ─── SidebarMenuItem / SidebarMenuSubItem ────────────────────────────────────

export type SidebarMenuItemProps = LiHTMLAttributes<HTMLLIElement>;

function useMenuRow(rowRef: RefObject<HTMLLIElement | null>, isSubRow = false) {
  const scope = useContext(MenuScopeContext);
  const registerRow = scope?.registerRow;
  const setRowButton = scope?.setRowButton;
  const setRowActive = scope?.setRowActive;

  // The button's setActive effect can fire while this row's <li> ref is
  // detached: a child's layout effects run before its parent's ref attaches
  // — at mount, and on EVERY re-render whose inline ref identity changes
  // (React detaches the old callback, nulling the ref, before the layout
  // phase). The flag holds the truth through that window, and attachRow
  // re-syncs the scope whenever the <li> lands.
  const activeFlagRef = useRef(false);

  useIsoLayoutEffect(() => {
    const el = rowRef.current;
    if (!el || !registerRow) return;
    return registerRow(el);
  }, [registerRow, rowRef]);

  // The <li> as state too, for the render's "am I the hovered / an active
  // row" checks against the scope's elements.
  const [rowEl, setRowEl] = useState<HTMLLIElement | null>(null);

  /** The <li>'s ref callback: tracks the element and replays the active flag
   *  the scope may have missed while the ref was detached. */
  const attachRow = useCallback(
    (node: HTMLLIElement | null) => {
      rowRef.current = node;
      setRowEl(node);
      if (node && setRowActive) setRowActive(node, activeFlagRef.current);
    },
    [setRowActive, rowRef]
  );

  const setActive = useCallback(
    (active: boolean) => {
      activeFlagRef.current = active;
      if (rowRef.current && setRowActive) setRowActive(rowRef.current, active);
    },
    [setRowActive, rowRef]
  );

  const setButtonEl = useCallback(
    (el: HTMLElement | null) => {
      if (rowRef.current && setRowButton) setRowButton(rowRef.current, el);
    },
    [setRowButton, rowRef]
  );

  const isHovered = rowEl !== null && scope?.hoveredRowEl === rowEl;
  const isActiveRow = rowEl !== null && (scope?.activeRows.includes(rowEl) ?? false);

  const [trailing, setTrailing] = useState({
    actionCount: 0,
    actionsShowOnHover: false,
    hasBadge: false,
  });
  const setActions = useCallback(
    (count: number, showOnHover: boolean) =>
      setTrailing((prev) =>
        prev.actionCount === count && prev.actionsShowOnHover === showOnHover
          ? prev
          : { ...prev, actionCount: count, actionsShowOnHover: showOnHover }
      ),
    []
  );
  const setHasBadge = useCallback(
    (hasBadge: boolean) =>
      setTrailing((prev) => (prev.hasBadge === hasBadge ? prev : { ...prev, hasBadge })),
    []
  );

  return useMemo(
    () => ({
      rowRef,
      attachRow,
      isHovered,
      isActiveRow,
      isSubRow,
      setActive,
      setButtonEl,
      ...trailing,
      setActions,
      setHasBadge,
    }),
    [
      rowRef,
      attachRow,
      isHovered,
      isActiveRow,
      isSubRow,
      setActive,
      setButtonEl,
      trailing,
      setActions,
      setHasBadge,
    ]
  );
}

// ─── Trailing-gutter math ────────────────────────────────────────────────────
//
// The label reserves exactly the trailing run it has to clear, plus one gap
// — the same rule the section header's label follows, so a row's chevron and
// a section header's chevron each sit one 4px gap from their action run.
// A run is: the badge's 24px slot (rightmost when present), the action
// cluster (24px apiece, 4px between), and a gap where both appear.
const ROW_BASE_PAD = 8;

const ROW_SLOT = 24;

const ROW_GAP = 4;

/** Where the run's rightmost element sits, measured from the row's right
 *  edge: a badge at right-2, an action cluster at right-1.5 (its wider box
 *  puts both on the same centre line). */
const ROW_BADGE_INSET = 8;

const ROW_ACTION_INSET = 6;

function rowGutter(actionCount: number, hasBadge: boolean) {
  if (!actionCount && !hasBadge) return ROW_BASE_PAD;
  const actionsWidth = actionCount
    ? actionCount * ROW_SLOT + (actionCount - 1) * ROW_GAP
    : 0;
  const runWidth =
    (hasBadge ? ROW_SLOT : 0) +
    actionsWidth +
    (hasBadge && actionCount ? ROW_GAP : 0);
  const inset = hasBadge ? ROW_BADGE_INSET : ROW_ACTION_INSET;
  return inset + runWidth + ROW_GAP;
}

const SidebarMenuItem = forwardRef<HTMLLIElement, SidebarMenuItemProps>(
  ({ className, children, ...props }, ref) => {
    const rowRef = useRef<HTMLLIElement>(null);
    const item = useMenuRow(rowRef);
    const { attachRow } = item;
    // Stable ref callback: an inline one is detached and re-attached around
    // every re-render, and child layout effects fire inside that null window.
    const refCb = useCallback(
      (node: HTMLLIElement | null) => {
        attachRow(node);
        if (typeof ref === "function") ref(node);
        else if (ref) (ref as React.MutableRefObject<HTMLLIElement | null>).current = node;
      },
      [attachRow, ref]
    );
    return (
      <MenuItemContext.Provider value={item}>
        <li
          ref={refCb}
          data-sidebar="menu-item"
          className={cn("group/menu-item relative", className)}
          {...props}
        >
          {children}
        </li>
      </MenuItemContext.Provider>
    );
  }
);

SidebarMenuItem.displayName = "SidebarMenuItem";

export type SidebarMenuSubItemProps = LiHTMLAttributes<HTMLLIElement>;

const SidebarMenuSubItem = forwardRef<HTMLLIElement, SidebarMenuSubItemProps>(
  ({ className, children, ...props }, ref) => {
    const rowRef = useRef<HTMLLIElement>(null);
    const item = useMenuRow(rowRef, true);
    const { attachRow } = item;
    // Stable ref callback — same reason as SidebarMenuItem's.
    const refCb = useCallback(
      (node: HTMLLIElement | null) => {
        attachRow(node);
        if (typeof ref === "function") ref(node);
        else if (ref) (ref as React.MutableRefObject<HTMLLIElement | null>).current = node;
      },
      [attachRow, ref]
    );
    return (
      <MenuItemContext.Provider value={item}>
        <li
          ref={refCb}
          data-sidebar="menu-sub-item"
          className={cn("group/menu-sub-item relative", className)}
          {...props}
        >
          {children}
        </li>
      </MenuItemContext.Provider>
    );
  }
);

SidebarMenuSubItem.displayName = "SidebarMenuSubItem";
export { SidebarMenu, useMenuRow, ROW_BASE_PAD, ROW_SLOT, ROW_GAP, ROW_BADGE_INSET, ROW_ACTION_INSET, rowGutter, SidebarMenuItem, SidebarMenuSubItem };
