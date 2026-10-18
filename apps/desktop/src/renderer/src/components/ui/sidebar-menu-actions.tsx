"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useContext, useId, forwardRef, Children, type ReactElement, type CSSProperties, type HTMLAttributes, type ButtonHTMLAttributes, type Ref } from "react";

import { cn } from "@/lib/utils";

import { fontWeights } from "@/lib/font-weight";
import { useShape } from "@/lib/shape-context";
import { useSize, typeClass } from "@/lib/size-context";

import { resolveSlotTemplate, SlotElement } from "@/components/ui/sidebar-core";

import { MenuItemContext } from "./sidebar-menu-context";
import { MenuActionsClusterContext } from "./sidebar-menu-context";
import { useIsoLayoutEffect } from "./sidebar-menu-context";

// ─── SidebarMenuAction ───────────────────────────────────────────────────────

export interface SidebarMenuActionProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  showOnHover?: boolean;
  render?: ReactElement;
  asChild?: boolean;
}

const SidebarMenuAction = forwardRef<HTMLButtonElement, SidebarMenuActionProps>(
  ({ className, showOnHover = false, render, asChild, children, onClick, ...props }, ref) => {
    const shape = useShape();
    const sizeClasses = useSize();
    const item = useContext(MenuItemContext);
    const inCluster = useContext(MenuActionsClusterContext);
    const { template, content } = resolveSlotTemplate(render, asChild, children);

    // A lone action registers its own slot; inside a cluster the wrapper
    // registers the whole count and each action flows in its row.
    const setActions = item?.setActions;
    useIsoLayoutEffect(() => {
      if (inCluster || !setActions) return;
      setActions(1, showOnHover);
      return () => setActions(0, false);
    }, [inCluster, setActions, showOnHover]);
    return (
      <SlotElement
        template={template}
        defaultTag="button"
        elementRef={ref as Ref<HTMLElement>}
        props={{
          type: template ? undefined : "button",
          "data-sidebar": "menu-action",
          "data-show-on-hover": showOnHover ? "" : undefined,
          className: cn(
            // right-1.5 centers the 24px hit-box on the same axis as the badge
            // (right-2 + min-w-5): both land 18px from the row's right edge.
            // With a badge on the same row the badge keeps that rightmost spot
            // and the action slides left of it. Inside a cluster the wrapper
            // owns the positioning and actions simply flow.
            inCluster
              ? "relative flex size-6 shrink-0 items-center justify-center text-muted-foreground outline-none"
              : "absolute right-1.5 z-10 flex size-6 items-center justify-center text-muted-foreground outline-none",
            !inCluster &&
              (item?.isSubRow
                ? "group-has-[>[data-sidebar=menu-badge]]/menu-sub-item:right-8.5"
                : "group-has-[>[data-sidebar=menu-badge]]/menu-item:right-8.5"),
            !inCluster &&
              (item?.isSubRow || sizeClasses.variant === "compact" ? "top-0.5" : "top-1"),
            "hover:bg-hover hover:text-foreground transition-[color,background-color,opacity] duration-80",
            "focus-visible:ring-1 focus-visible:ring-[color:var(--focus-ring,#6B97FF)]",
            // One icon size across the sidebar: row actions match the leading
            // icons and the section header's actions, all on the size ladder.
            // Normalize bare icons to the site's 1.5 stroke (library defaults
            // vary), thickening to 2 on hover — Button's icon-only treatment.
            "[&_svg]:size-[var(--icon-size)] [&_svg]:shrink-0 [&_svg]:stroke-[1.5] [&_svg]:transition-[stroke-width] [&_svg]:duration-80 hover:[&_svg]:stroke-[2]",
            shape.item,
            // Reveal on the OWN row only. A sub action must not use the
            // menu-item group — its nearest one is the parent li, which would
            // light every sibling sub action on any hover inside the sub-tree.
            !inCluster &&
              showOnHover &&
              (item?.isSubRow
                ? "opacity-0 group-hover/menu-sub-item:opacity-100 group-focus-within/menu-sub-item:opacity-100 data-[state=open]:opacity-100 aria-expanded:opacity-100"
                // Tracks the row's own button (its peer), not the <li> — a row
                // that hosts a sub-menu wraps its children too, and hovering a
                // child should not light the parent's action.
                : "opacity-0 peer-hover/menu-button:opacity-100 peer-focus-visible/menu-button:opacity-100 hover:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100 aria-expanded:opacity-100"),
            className
          ),
          onClick: (event: React.MouseEvent<HTMLButtonElement>) => {
            // The action often sits on a row composed via `render` — keep its
            // click from also triggering the row.
            event.stopPropagation();
            onClick?.(event);
          },
          ...props,
          style: { ...({ "--icon-size": `${sizeClasses.icon}px` } as CSSProperties), ...(props.style ?? {}) },
        }}
      >
        {content}
      </SlotElement>
    );
  }
);

SidebarMenuAction.displayName = "SidebarMenuAction";

// ─── SidebarMenuActions ──────────────────────────────────────────────────────

export interface SidebarMenuActionsProps extends HTMLAttributes<HTMLDivElement> {
  /** Hide the cluster until the row is hovered or focused. */
  showOnHover?: boolean;
}

/** Row-level cluster for more than one SidebarMenuAction. It owns the
 *  positioning and publishes the whole count to the row, so the button
 *  reserves the exact gutter the cluster occupies. */
const SidebarMenuActions = forwardRef<HTMLDivElement, SidebarMenuActionsProps>(
  ({ className, showOnHover = false, children, ...props }, ref) => {
    const item = useContext(MenuItemContext);
    const sizeClasses = useSize();
    const count = Children.count(children);

    const setActions = item?.setActions;
    useIsoLayoutEffect(() => {
      setActions?.(count, showOnHover);
      return () => setActions?.(0, false);
    }, [setActions, count, showOnHover]);

    return (
      <div
        ref={ref}
        data-sidebar="menu-actions"
        className={cn(
          "absolute right-1.5 z-10 flex items-center gap-1",
          // The badge keeps the rightmost slot; the cluster sits left of it.
          item?.isSubRow
            ? "group-has-[>[data-sidebar=menu-badge]]/menu-sub-item:right-8.5"
            : "group-has-[>[data-sidebar=menu-badge]]/menu-item:right-8.5",
          item?.isSubRow || sizeClasses.variant === "compact" ? "top-0.5" : "top-1",
          showOnHover &&
            (item?.isSubRow
              ? "opacity-0 transition-opacity duration-80 group-hover/menu-sub-item:opacity-100 group-focus-within/menu-sub-item:opacity-100 has-[[data-state=open]]:opacity-100 has-[[data-popup-open]]:opacity-100"
              // Peer-scoped for the same reason as a lone action: the row's
              // <li> also wraps its sub-menu.
              : "opacity-0 transition-opacity duration-80 peer-hover/menu-button:opacity-100 peer-focus-visible/menu-button:opacity-100 hover:opacity-100 focus-within:opacity-100 has-[[data-state=open]]:opacity-100 has-[[data-popup-open]]:opacity-100"),
          className
        )}
        {...props}
      >
        <MenuActionsClusterContext.Provider value={true}>
          {children}
        </MenuActionsClusterContext.Provider>
      </div>
    );
  }
);

SidebarMenuActions.displayName = "SidebarMenuActions";

// ─── SidebarMenuBadge ────────────────────────────────────────────────────────

export type SidebarMenuBadgeProps = HTMLAttributes<HTMLDivElement>;

const SidebarMenuBadge = forwardRef<HTMLDivElement, SidebarMenuBadgeProps>(
  ({ className, ...props }, ref) => {
    const item = useContext(MenuItemContext);
    const sizeClasses = useSize();
    const lit = item?.isActiveRow ?? false;

    const setHasBadge = item?.setHasBadge;
    useIsoLayoutEffect(() => {
      setHasBadge?.(true);
      return () => setHasBadge?.(false);
    }, [setHasBadge]);
    return (
      <div
        ref={ref}
        data-sidebar="menu-badge"
        className={cn(
          "pointer-events-none absolute right-2 z-10 flex h-5 min-w-5 items-center justify-center px-1 tabular-nums",
          sizeClasses.variant === "compact" ? "top-1" : "top-1.5",
          typeClass("micro", sizeClasses.variant),
          "transition-[color,font-variation-settings] duration-80",
          lit ? "text-foreground" : "text-muted-foreground",
          className
        )}
        style={{
          fontVariationSettings: lit ? fontWeights.semibold : fontWeights.normal,
        }}
        {...props}
      />
    );
  }
);

SidebarMenuBadge.displayName = "SidebarMenuBadge";

// ─── SidebarMenuSkeleton ─────────────────────────────────────────────────────

export interface SidebarMenuSkeletonProps extends HTMLAttributes<HTMLDivElement> {
  showIcon?: boolean;
}

// Deterministic width cycle (not Math.random) so server and client render the
// same markup — a random width per render is a hydration mismatch.
const SKELETON_WIDTHS = ["62%", "74%", "55%", "82%", "68%"];

const SidebarMenuSkeleton = forwardRef<HTMLDivElement, SidebarMenuSkeletonProps>(
  ({ className, showIcon = false, ...props }, ref) => {
    const sizeClasses = useSize();
    const id = useId();
    let sum = 0;
    for (let i = 0; i < id.length; i++) sum += id.charCodeAt(i);
    const width = SKELETON_WIDTHS[sum % SKELETON_WIDTHS.length];
    return (
      <div
        ref={ref}
        data-sidebar="menu-skeleton"
        className={cn(
          "flex items-center gap-2 px-2",
          sizeClasses.variant === "compact" ? "h-7" : "h-8",
          className
        )}
        {...props}
      >
        {showIcon && (
          <div
            data-sidebar="menu-skeleton-icon"
            className="size-4 shrink-0 animate-pulse rounded-md bg-hover"
          />
        )}
        <div
          data-sidebar="menu-skeleton-text"
          className="h-4 flex-1 animate-pulse rounded-md bg-hover"
          style={{ maxWidth: width }}
        />
      </div>
    );
  }
);

SidebarMenuSkeleton.displayName = "SidebarMenuSkeleton";
export { SidebarMenuAction, SidebarMenuActions, SidebarMenuBadge, SKELETON_WIDTHS, SidebarMenuSkeleton };
