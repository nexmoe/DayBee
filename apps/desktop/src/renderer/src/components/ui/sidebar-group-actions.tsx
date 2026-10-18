"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { createContext, useContext, forwardRef, type ReactElement, type CSSProperties, type HTMLAttributes, type Ref } from "react";

import { cn } from "@/lib/utils";

import { useShape } from "@/lib/shape-context";
import { useSize } from "@/lib/size-context";

import { resolveSlotTemplate } from "./sidebar-slot";
import { SlotElement } from "./sidebar-slot";
import type { SidebarSectionProps } from "./sidebar-layout";

export interface SidebarGroupActionProps extends HTMLAttributes<HTMLButtonElement> {
  render?: ReactElement;
  asChild?: boolean;
}

/** True while rendering inside a SidebarGroupActions cluster, where each
 *  action sits in the flex row instead of positioning itself absolutely. */
const GroupActionsContext = createContext(false);

const SidebarGroupAction = forwardRef<HTMLButtonElement, SidebarGroupActionProps>(
  ({ className, render, asChild, children, ...props }, ref) => {
    const shape = useShape();
    const sizeClasses = useSize();
    const inCluster = useContext(GroupActionsContext);
    const { template, content } = resolveSlotTemplate(render, asChild, children);
    return (
      <SlotElement
        template={template}
        defaultTag="button"
        elementRef={ref as Ref<HTMLElement>}
        props={{
          type: template ? undefined : "button",
          "data-sidebar": "group-action",
          className: cn(
            inCluster
              ? "relative flex size-6 items-center justify-center text-muted-foreground outline-none"
              // size-6 matches the rows' action hit-box, and right-3.5 puts that
              // 24px box's centre 26px from the sidebar's inner edge — the axis
              // the rows' badges and actions already sit on.
              : "absolute right-3.5 top-3 flex size-6 items-center justify-center text-muted-foreground outline-none",
            "hover:bg-hover hover:text-foreground transition-colors duration-80",
            "focus-visible:ring-1 focus-visible:ring-[color:var(--focus-ring,#6B97FF)]",
            // Normalize bare icons to the site's 1.5 stroke (library defaults
            // vary), thickening to 2 on hover — the same treatment Button's
            // icon-only span applies.
            "[&_svg]:size-[var(--icon-size)] [&_svg]:shrink-0 [&_svg]:stroke-[1.5] [&_svg]:transition-[stroke-width] [&_svg]:duration-80 hover:[&_svg]:stroke-[2]",
            shape.item,
            className
          ),
          ...props,
          // After ...props: the spread would otherwise replace this object
          // wholesale and drop the icon-size the glyph is sized from.
          style: {
            ...({ "--icon-size": `${sizeClasses.icon}px` } as CSSProperties),
            ...(props.style ?? {}),
          },
        }}
      >
        {content}
      </SlotElement>
    );
  }
);

SidebarGroupAction.displayName = "SidebarGroupAction";

/** Header action cluster: 1–3 SidebarGroupActions laid out in a row over the
 *  group label's right edge. Use instead of a lone SidebarGroupAction when a
 *  section needs several controls. */
export type SidebarGroupActionsProps = HTMLAttributes<HTMLDivElement>;

const SidebarGroupActions = forwardRef<HTMLDivElement, SidebarGroupActionsProps>(
  ({ className, children, ...props }, ref) => {
    return (
      <div
        ref={ref}
        data-sidebar="group-actions"
        className={cn(
          // right-3.5 lands the last 24px action's centre 26px from the
          // sidebar's inner edge — the rows' badge/action axis, so the header's
          // controls line up with the column below them.
          "absolute right-3.5 top-2 z-10 flex h-8 items-center gap-1",
          className
        )}
        {...props}
      >
        <GroupActionsContext.Provider value={true}>
          {children}
        </GroupActionsContext.Provider>
      </div>
    );
  }
);

SidebarGroupActions.displayName = "SidebarGroupActions";

const SidebarGroupContent = forwardRef<HTMLDivElement, SidebarSectionProps>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-sidebar="group-content"
      className={cn("w-full", className)}
      {...props}
    />
  )
);

SidebarGroupContent.displayName = "SidebarGroupContent";
export { GroupActionsContext, SidebarGroupAction, SidebarGroupActions, SidebarGroupContent };
