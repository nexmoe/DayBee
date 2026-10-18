"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { createContext, useContext, useState, useCallback, useMemo, useRef, useId, forwardRef, isValidElement, Children, type ReactNode, type ReactElement, type CSSProperties, type HTMLAttributes, type Ref } from "react";
import { motion } from "framer-motion";
import { cn } from "@/lib/utils";
import { spring } from "@/lib/springs";

import { useShape } from "@/lib/shape-context";
import { useSize, useSizeVariant, typeClass } from "@/lib/size-context";
import { useIcons } from "@/lib/icon-context";

import type { SidebarSectionProps } from "./sidebar-layout";
import { useIsoLayoutEffect } from "./sidebar-context";
import { SidebarGroupAction } from "./sidebar-group-actions";
import { SidebarGroupActions } from "./sidebar-group-actions";
import { resolveSlotTemplate } from "./sidebar-slot";
import { SlotElement } from "./sidebar-slot";

// ─── SidebarGroup family ─────────────────────────────────────────────────────

interface SidebarGroupContextValue {
  open: boolean;
  toggle: () => void;
  contentId: string;
  /** How many header action buttons overlay the label's right edge — the
   *  collapsible label pads itself so its chevron clears them. */
  actionsCount: number;
}

const SidebarGroupContext = createContext<SidebarGroupContextValue | null>(null);

export interface SidebarGroupProps extends SidebarSectionProps {
  /** Makes the group's SidebarGroupLabel a toggle that collapses everything
   *  rendered after it — a group-level accordion. Uncontrolled by default;
   *  pass `open`/`onOpenChange` to control it. */
  collapsible?: boolean;
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
}

const SidebarGroup = forwardRef<HTMLDivElement, SidebarGroupProps>(
  (
    {
      className,
      collapsible = false,
      open: openProp,
      defaultOpen = true,
      onOpenChange,
      children,
      ...props
    },
    ref
  ) => {
    const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
    const open = openProp ?? uncontrolledOpen;
    const contentId = useId();
    const toggle = useCallback(() => {
      const next = !(openProp ?? uncontrolledOpen);
      setUncontrolledOpen(next);
      onOpenChange?.(next);
    }, [openProp, uncontrolledOpen, onOpenChange]);
    // Measured-height collapse: animate between 0 and the content's real
    // offsetHeight — never to "auto", which framer measures wrong under a
    // scaled ancestor.
    const contentRef = useRef<HTMLDivElement>(null);
    const [contentHeight, setContentHeight] = useState<number | null>(null);
    useIsoLayoutEffect(() => {
      if (!collapsible) return;
      const el = contentRef.current;
      if (!el || typeof ResizeObserver === "undefined") return;
      const measure = () => setContentHeight(el.offsetHeight);
      measure();
      const ro = new ResizeObserver(measure);
      ro.observe(el);
      return () => ro.disconnect();
    }, [collapsible]);
    const measured = contentHeight !== null;
    // The collapse wrapper must clip while animating, but a permanently
    // clipped box shaves the 2px focus ring off a group's first and last
    // rows — so clipping lifts once an open group has settled.
    const [settled, setSettled] = useState(open);
    if (!open && settled) setSettled(false);

    // Height animates only when THIS group toggles. When the measured height
    // changes underneath it instead — a nested sub-menu collapsing inside the
    // group — the wrapper must snap: a spring re-targeted every frame chases
    // the child's own animation, lands well after it, and drags everything
    // below the group along late. Tracked against the previous `open` so a
    // controlled `open` is covered too, and cleared once the toggle's
    // animation lands.
    const [prevOpen, setPrevOpen] = useState(open);
    const [toggling, setToggling] = useState(false);
    if (prevOpen !== open) {
      setPrevOpen(open);
      setToggling(true);
    }

    // The label and any header actions stay put; everything else after the
    // label rides in the collapse wrapper. If no SidebarGroupLabel child is
    // found the group renders untouched.
    const isHeaderAction = (k: ReactNode) =>
      isValidElement(k) &&
      (k.type === SidebarGroupAction || k.type === SidebarGroupActions);
    let inner: ReactNode = children;
    let actionsCount = 0;
    if (collapsible) {
      const kids = Children.toArray(children);
      const labelIdx = kids.findIndex(
        (k) => isValidElement(k) && k.type === SidebarGroupLabel
      );
      if (labelIdx !== -1) {
        const tail = kids.slice(labelIdx + 1);
        const headerActions = tail.filter(isHeaderAction);
        const rest = tail.filter((k) => !isHeaderAction(k));
        actionsCount = headerActions.reduce<number>(
          (n, k) =>
            n +
            (isValidElement(k) && k.type === SidebarGroupActions
              ? Children.count(
                  (k.props as { children?: ReactNode }).children
                )
              : 1),
          0
        );
        inner = (
          <>
            {kids.slice(0, labelIdx)}
            {/* Header hover scope: hovering anywhere on the row — the
                overlaid action cluster included, which never :hovers the
                label element itself — reveals the label's chevron. Kept
                position-static so the cluster's absolute box still anchors
                to the group and stays on the rows' action axis. */}
            <div className="group/group-header w-full">
              {kids[labelIdx]}
              {headerActions}
            </div>
            <motion.div
              id={contentId}
              aria-hidden={open ? undefined : true}
              className={cn(
                open && settled ? "overflow-visible" : "overflow-hidden",
                !measured && !open && "h-0"
              )}
              initial={false}
              animate={
                measured
                  ? { height: open ? contentHeight : 0, opacity: open ? 1 : 0 }
                  : { opacity: open ? 1 : 0 }
              }
              // Do NOT simplify this to `open ? spring.moderate : …`. The
              // toggling arm is what stops a re-measure from springing —
              // without it a nested collapse makes this wrapper chase its own
              // child and everything below the group moves late.
              transition={
                toggling
                  ? open
                    ? spring.moderate
                    : spring.moderate.exit
                  : { duration: 0 }
              }
              onAnimationComplete={() => {
                setToggling(false);
                if (open) setSettled(true);
              }}
            >
              <div ref={contentRef} className="flex w-full min-w-0 flex-col">
                {rest}
              </div>
            </motion.div>
          </>
        );
      }
    }

    const ctx = useMemo(
      () => ({ open, toggle, contentId, actionsCount }),
      [open, toggle, contentId, actionsCount]
    );

    return (
      <div
        ref={ref}
        data-sidebar="group"
        data-state={collapsible ? (open ? "open" : "closed") : undefined}
        className={cn("relative flex w-full min-w-0 flex-col p-2", className)}
        {...props}
      >
        <SidebarGroupContext.Provider value={collapsible ? ctx : null}>
          {inner}
        </SidebarGroupContext.Provider>
      </div>
    );
  }
);

SidebarGroup.displayName = "SidebarGroup";

export interface SidebarGroupLabelProps extends HTMLAttributes<HTMLDivElement> {
  render?: ReactElement;
  asChild?: boolean;
}

const SidebarGroupLabel = forwardRef<HTMLDivElement, SidebarGroupLabelProps>(
  ({ className, render, asChild, children, ...props }, ref) => {
    const sizeVariant = useSizeVariant();
    const sizeClasses = useSize();
    const group = useContext(SidebarGroupContext);
    const shape = useShape();
    const icons = useIcons();
    const ChevronRightIcon = icons["chevron-right"];
    const { template, content } = resolveSlotTemplate(render, asChild, children);

    // Truncate only the leading text; element children (count badges,
    // trailing controls) stay flex siblings so the row's gap keeps spacing
    // them — same split MenuRowLabel does for menu rows.
    const nodes = Children.toArray(content);
    let textEnd = 0;
    while (
      textEnd < nodes.length &&
      (typeof nodes[textEnd] === "string" || typeof nodes[textEnd] === "number")
    ) {
      textEnd++;
    }
    const leadingText = nodes.slice(0, textEnd).join("");
    const labelContent = leadingText ? (
      <>
        <span className="min-w-0 truncate">{leadingText}</span>
        {nodes.slice(textEnd)}
      </>
    ) : (
      content
    );

    // Inside a collapsible group the label becomes the toggle. Design
    // treatment is unchanged — hover only raises the label's contrast and
    // reveals a chevron (kept visible while collapsed as the reopen cue).
    if (group) {
      return (
        <SlotElement
          template={template}
          defaultTag="button"
          elementRef={ref as Ref<HTMLElement>}
          props={{
            type: template ? undefined : "button",
            "data-sidebar": "group-label",
            "aria-expanded": group.open,
            "aria-controls": group.contentId,
            onClick: group.toggle,
            // The action cluster overlays the label's right edge, so the label
            // pads past it — far enough that the hover-revealed chevron lands
            // one cluster gap (4px) to its left and the whole trailing run
            // keeps a single rhythm. Cluster width is 24px per action plus 4px
            // between them; add that gap again, less the 8px the group's
            // padding already gives back: 28n + 6. The cluster is always
            // visible, so the reservation is permanent.
            style:
              group.actionsCount > 0
                ? ({ "--group-actions-pad": `${group.actionsCount * 28 + 6}px` } as CSSProperties)
                : undefined,
            className: cn(
              "flex h-8 w-full shrink-0 cursor-pointer select-none items-center gap-2 px-2 text-left text-muted-foreground outline-none",
              "transition-colors duration-80 hover:text-muted-foreground",
              group.actionsCount > 0 && "pr-[var(--group-actions-pad)]",
              "focus-visible:ring-1 focus-visible:ring-[color:var(--focus-ring,#6B97FF)]",
              shape.item,
              typeClass("caption", sizeVariant),
              className
            ),
            ...props,
          }}
        >
          <>
            {labelContent}
            {/* The chevron occupies an action-sized box, so it reads as one more
                icon in the row rather than a smaller glyph tacked on the end.
                One chevron-right glyph for both states, sprung 90° to point
                down while the group is open — the motion wrapper is what
                animates: Tailwind's rotate-* sets the standalone CSS `rotate`
                property, which transition-transform never covers. While open
                the whole box collapses to zero width at rest so the label text
                keeps the full row; hover/focus (or an open action popup)
                reveals it. Collapsed keeps it visible as the reopen cue. */}
            {/* No width/opacity transition: animating the box's width slides
                the glyph in from the side — the chevron should simply be
                there once the header is hovered. */}
            <span
              className={cn(
                "ml-auto flex h-6 shrink-0 items-center justify-center overflow-hidden",
                group.open
                  ? "w-0 opacity-0 group-hover/group-header:w-6 group-hover/group-header:opacity-100 group-focus-within/group-header:w-6 group-focus-within/group-header:opacity-100 group-has-[[data-sidebar=group-action]:is([data-state=open],[data-popup-open],[aria-expanded=true])]/group-header:w-6 group-has-[[data-sidebar=group-action]:is([data-state=open],[data-popup-open],[aria-expanded=true])]/group-header:opacity-100 pointer-coarse:w-6 pointer-coarse:opacity-100"
                  : "w-6 opacity-100"
              )}
            >
              <motion.span
                className="inline-flex"
                animate={{ rotate: group.open ? 90 : 0 }}
                transition={spring.fast}
              >
                <ChevronRightIcon
                  size={sizeClasses.icon}
                  strokeWidth={1.5}
                  className="shrink-0"
                />
              </motion.span>
            </span>
          </>
        </SlotElement>
      );
    }

    return (
      <SlotElement
        template={template}
        defaultTag="div"
        elementRef={ref as Ref<HTMLElement>}
        props={{
          "data-sidebar": "group-label",
          className: cn(
            "flex h-8 shrink-0 items-center gap-2 px-2 text-muted-foreground outline-none",
            typeClass("caption", sizeVariant),
            className
          ),
          ...props,
        }}
      >
        {labelContent}
      </SlotElement>
    );
  }
);

SidebarGroupLabel.displayName = "SidebarGroupLabel";
export { SidebarGroupContext, SidebarGroup, SidebarGroupLabel };
export type { SidebarGroupContextValue };
