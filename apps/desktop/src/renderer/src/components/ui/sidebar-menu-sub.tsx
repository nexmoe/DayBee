"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useContext, useState, useRef, forwardRef, type ReactElement, type CSSProperties, type HTMLAttributes, type AnchorHTMLAttributes } from "react";
import { motion } from "framer-motion";

import { cn } from "@/lib/utils";
import { spring } from "@/lib/springs";

import { useShape } from "@/lib/shape-context";
import { useSize, typeClass } from "@/lib/size-context";

import type { IconComponent } from "@/lib/icon-context";
import { resolveSlotTemplate, SlotElement } from "@/components/ui/sidebar-core";

import { MenuScopeContext } from "./sidebar-menu-context";
import { useIsoLayoutEffect } from "./sidebar-menu-context";
import { MenuItemContext } from "./sidebar-menu-context";
import { rowGutter } from "./sidebar-menu-rows";
import { MenuRowLabel } from "./sidebar-menu-button";

// ─── SidebarMenuSub ──────────────────────────────────────────────────────────

export interface SidebarMenuSubProps extends HTMLAttributes<HTMLUListElement> {
  /** Built-in measured-height collapse. Omitted, the sub-menu is always
   *  visible; wire it to state (with a toggling SidebarMenuButton) for a
   *  collapsible tree. */
  open?: boolean;
}

const SidebarMenuSub = forwardRef<HTMLUListElement, SidebarMenuSubProps>(
  ({ className, open = true, children, ...props }, ref) => {
    const containerRef = useRef<HTMLUListElement>(null);
    // The sub-menu is NOT its own highlight scope: its rows register with the
    // surrounding SidebarMenu, so one hover background glides from a parent
    // row into its children. Toggling flips the rows' visibility in place
    // (they stay registered), so the scope re-reads what is visible.
    const scope = useContext(MenuScopeContext);
    const refreshVisibility = scope?.refreshVisibility;
    useIsoLayoutEffect(() => {
      refreshVisibility?.();
    }, [open, refreshVisibility]);

    // Measured-height collapse: animate between 0 and the content's real
    // offsetHeight — never to "auto", which framer measures wrong under a
    // scaled ancestor.
    const [contentHeight, setContentHeight] = useState<number | null>(null);
    useIsoLayoutEffect(() => {
      const el = containerRef.current;
      if (!el || typeof ResizeObserver === "undefined") return;
      const measure = () => setContentHeight(el.offsetHeight);
      measure();
      const ro = new ResizeObserver(measure);
      ro.observe(el);
      return () => ro.disconnect();
    }, []);
    const measured = contentHeight !== null;

    // Same rule as SidebarGroup: spring only when this sub-tree itself
    // toggles. A height change coming from a nested sub collapsing inside it
    // snaps, so the wrapper tracks its content instead of chasing it with a
    // second spring and moving everything below late.
    const [prevOpen, setPrevOpen] = useState(open);
    const [toggling, setToggling] = useState(false);
    if (prevOpen !== open) {
      setPrevOpen(open);
      setToggling(true);
    }

    return (
      <motion.div
        data-slot="sidebar-menu-sub-wrapper"
        // `animate` stays defined from the first render — framer ignores an
        // animate prop that appears later in the element's life. Until the
        // content is measured, a closed sub collapses via the h-0 class and
        // an open one keeps its natural height.
        className={cn("overflow-hidden", !measured && !open && "h-0")}
        initial={false}
        animate={
          measured
            ? { height: open ? contentHeight : 0, opacity: open ? 1 : 0 }
            : { opacity: open ? 1 : 0 }
        }
        // Do NOT simplify this to `open ? spring.moderate : …` — see the
        // toggling note above. Springing on a re-measure stacks a second
        // spring on a nested sub's own collapse.
        transition={
          toggling
            ? open
              ? spring.moderate
              : spring.moderate.exit
            : { duration: 0 }
        }
        onAnimationComplete={() => {
          setToggling(false);
        }}
      >
        <ul
          ref={(node) => {
            containerRef.current = node;
            if (typeof ref === "function") ref(node);
            else if (ref) (ref as React.MutableRefObject<HTMLUListElement | null>).current = node;
          }}
          data-sidebar="menu-sub"
          data-state={open ? "open" : "closed"}
          aria-hidden={open ? undefined : true}
          className={cn(
            // ml-[15px] (a margin, not a translate, so the rows' measured
            // rects include it) + 1px border + pl-2 lands the sub-row label
            // (+ the row's own pl-2 = 32px) exactly on the parent label's x
            // (px-2 + 16px icon + gap-2 = 32px).
            "relative ml-[15px] flex min-w-0 flex-col border-l border-border pl-2 select-none",
            className
          )}
          {...props}
        >
          {children}
        </ul>
      </motion.div>
    );
  }
);

SidebarMenuSub.displayName = "SidebarMenuSub";

// ─── SidebarMenuSubButton ────────────────────────────────────────────────────

export interface SidebarMenuSubButtonProps extends AnchorHTMLAttributes<HTMLAnchorElement> {
  isActive?: boolean;
  size?: "sm" | "md";
  icon?: IconComponent;
  render?: ReactElement;
  asChild?: boolean;
}

const SidebarMenuSubButton = forwardRef<HTMLAnchorElement, SidebarMenuSubButtonProps>(
  ({ isActive = false, size = "md", icon: Icon, render, asChild, className, children, ...props }, ref) => {
    const item = useContext(MenuItemContext);
    const shape = useShape();
    const sizeClasses = useSize();
    const buttonRef = useRef<HTMLElement | null>(null);

    const setActive = item?.setActive;
    useIsoLayoutEffect(() => {
      setActive?.(isActive);
      return () => setActive?.(false);
    }, [isActive, setActive]);

    const setButtonEl = item?.setButtonEl;
    useIsoLayoutEffect(() => {
      setButtonEl?.(buttonRef.current);
      return () => setButtonEl?.(null);
    }, [setButtonEl]);

    const lit = isActive || (item?.isHovered ?? false);
    const tabIdx = isActive ? 0 : -1;

    const gutterHover = rowGutter(item?.actionCount ?? 0, item?.hasBadge ?? false);
    const gutterRest = item?.actionsShowOnHover
      ? rowGutter(0, item?.hasBadge ?? false)
      : gutterHover;
    const gutterVars = {
      "--row-gutter": `${gutterRest}px`,
      "--row-gutter-hover": `${gutterHover}px`,
    } as CSSProperties;

    const { template, content } = resolveSlotTemplate(render, asChild, children);

    return (
      <SlotElement
        template={template}
        defaultTag="a"
        elementRef={(node: HTMLElement | null) => {
          buttonRef.current = node;
          if (typeof ref === "function") ref(node as HTMLAnchorElement | null);
          else if (ref) (ref as React.MutableRefObject<HTMLElement | null>).current = node;
        }}
        props={{
          "data-sidebar": "menu-sub-button",
          "data-size": size,
          "data-active": isActive ? "true" : undefined,
          "aria-current": isActive ? "page" : undefined,
          tabIndex: tabIdx,
          className: cn(
            "relative z-10 flex w-full cursor-pointer select-none items-center gap-2 pl-2 text-left outline-none",
            "transition-[padding] duration-80 pr-[var(--row-gutter)] group-hover/menu-sub-item:pr-[var(--row-gutter-hover)] group-focus-within/menu-sub-item:pr-[var(--row-gutter-hover)] group-has-[[data-sidebar=menu-action]:is([data-state=open],[data-popup-open],[aria-expanded=true])]/menu-sub-item:pr-[var(--row-gutter-hover)]",
            size === "sm" ? "h-6" : sizeClasses.variant === "compact" ? "h-6" : "h-7",
            shape.item,
            className
          ),
          ...props,
          style: { ...gutterVars, ...(props.style ?? {}) },
        }}
      >
        <>
          {Icon && (
            <Icon
              size={sizeClasses.icon}
              strokeWidth={lit ? 2 : 1.5}
              className={cn(
                "shrink-0 transition-[color,stroke-width] duration-80",
                lit ? "text-foreground" : "text-muted-foreground"
              )}
            />
          )}
          {/* Sub-rows keep the parent rows' type size — only the row height
              steps down. */}
          <MenuRowLabel
            content={content}
            lit={lit}
            emphasized={isActive}
            textClass={size === "sm" ? typeClass("body", "compact") : sizeClasses.text}
          />
        </>
      </SlotElement>
    );
  }
);

SidebarMenuSubButton.displayName = "SidebarMenuSubButton";
export { SidebarMenuSub, SidebarMenuSubButton };
