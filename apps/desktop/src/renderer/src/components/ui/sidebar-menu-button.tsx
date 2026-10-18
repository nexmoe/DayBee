"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { useContext, useRef, forwardRef, Children, type ReactNode, type ReactElement, type CSSProperties, type ButtonHTMLAttributes } from "react";

import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

import { fontWeights } from "@/lib/font-weight";
import { useShape } from "@/lib/shape-context";
import { useSize, typeClass } from "@/lib/size-context";

import type { IconComponent } from "@/lib/icon-context";
import { resolveSlotTemplate, SlotElement } from "@/components/ui/sidebar-core";

import { MenuScopeContext } from "./sidebar-menu-context";
import { MenuItemContext } from "./sidebar-menu-context";
import { useIsoLayoutEffect } from "./sidebar-menu-context";
import { rowGutter } from "./sidebar-menu-rows";

// ─── Row label (ghost-span weight animation) ─────────────────────────────────

/** Splits leading string children out as the label so it can get the
 *  ghost-span weight treatment; remaining element children (dots, trailing
 *  icons) render as flex siblings after it — outside the text-box-trimmed
 *  span, which would clip an inline SVG, and where `ml-auto` can push a
 *  trailing control to the row's end. */
function MenuRowLabel({
  content,
  lit,
  emphasized,
  textClass,
}: {
  content: ReactNode;
  lit: boolean;
  emphasized: boolean;
  textClass: string;
}) {
  const nodes = Children.toArray(content);
  const textParts: string[] = [];
  let i = 0;
  while (i < nodes.length && (typeof nodes[i] === "string" || typeof nodes[i] === "number")) {
    textParts.push(String(nodes[i]));
    i++;
  }
  const label = textParts.join("");
  const rest = nodes.slice(i);

  if (!label) {
    return (
      <span
        className={cn(
          "flex min-w-0 flex-1 items-center gap-2 transition-colors duration-80",
          lit ? "text-foreground" : "text-muted-foreground",
          textClass
        )}
      >
        {content}
      </span>
    );
  }

  return (
    <>
      <span className={cn("inline-grid min-w-0 text-left", textClass)}>
        {/* Ghost: reserves width at the heaviest weight, hidden from AT.
            Both cells truncate so a long label clips with an ellipsis
            instead of wrapping the row. The trim box spans cap height to
            baseline, so the overflow clip would shave ascenders and
            descenders — symmetric padding extends the clip box past both
            and the negative margins cancel it out of the row's height. */}
        <span
          className="col-start-1 row-start-1 invisible truncate pt-[0.25em] -mt-[0.25em] pb-[0.25em] -mb-[0.25em] [text-box:trim-both_cap_alphabetic]"
          style={{ fontVariationSettings: fontWeights.semibold }}
          aria-hidden="true"
        >
          {label}
        </span>
        {/* Visible: animates between weights in the same cell */}
        <span
          className={cn(
            "col-start-1 row-start-1 truncate pt-[0.25em] -mt-[0.25em] pb-[0.25em] -mb-[0.25em] transition-[color,font-variation-settings] duration-80 [text-box:trim-both_cap_alphabetic]",
            lit ? "text-foreground" : "text-muted-foreground"
          )}
          style={{
            fontVariationSettings: emphasized ? fontWeights.semibold : fontWeights.normal,
          }}
        >
          {label}
        </span>
      </span>
      {rest}
    </>
  );
}

// ─── SidebarMenuButton ───────────────────────────────────────────────────────

export const sidebarMenuButtonVariants = cva(
  // The trailing gutter is an exact reservation published by the row (see
  // rowGutter): --row-gutter at rest, --row-gutter-hover once hover-revealed
  // actions are showing. One rule per state instead of a class per
  // count/badge/reveal combination.
  // Disabled stays in the layout, and pointer events pass through it to the
  // row, since a browser sends no mouse events to a disabled button: the
  // container keeps seeing the moves, and fluid hover simply never lights it.
  "peer/menu-button relative z-10 flex w-full cursor-pointer select-none items-center gap-2 pl-2 text-left outline-none disabled:opacity-50 disabled:pointer-events-none transition-[padding] duration-80 pr-[var(--row-gutter)] group-hover/menu-item:pr-[var(--row-gutter-hover)] group-focus-within/menu-item:pr-[var(--row-gutter-hover)] group-hover/menu-sub-item:pr-[var(--row-gutter-hover)] group-focus-within/menu-sub-item:pr-[var(--row-gutter-hover)] group-has-[[data-sidebar=menu-action]:is([data-state=open],[data-popup-open],[aria-expanded=true])]/menu-item:pr-[var(--row-gutter-hover)] group-has-[[data-sidebar=menu-action]:is([data-state=open],[data-popup-open],[aria-expanded=true])]/menu-sub-item:pr-[var(--row-gutter-hover)]",
  {
    variants: {
      variant: {
        default: "",
        outline: "border border-border bg-background",
      },
    },
    defaultVariants: { variant: "default" },
  }
);

export interface SidebarMenuButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof sidebarMenuButtonVariants> {
  isActive?: boolean;
  size?: "default" | "sm" | "lg";
  icon?: IconComponent;
  /** Semantic thread state for status-dot navigation. Drives the dot
   *  visuals (`active`/`unread` → filled, `idle` → ring), stamps
   *  `data-status` on the button, appends visually-hidden "unread" text for
   *  screen readers, and `"active"` implies `isActive`. */
  status?: "active" | "unread" | "idle";
  /** Visual-only dot in the icon column — the escape hatch when the
   *  semantic `status` vocabulary doesn't fit. Overrides the dot derived
   *  from `status`. Ignored when `icon` is set. */
  dot?: "filled" | "ring";
  render?: ReactElement;
  asChild?: boolean;
}

const SidebarMenuButton = forwardRef<HTMLButtonElement, SidebarMenuButtonProps>(
  (
    {
      isActive = false,
      size = "default",
      variant,
      icon: Icon,
      status,
      dot,
      render,
      asChild,
      className,
      children,
      ...props
    },
    ref
  ) => {
    const scope = useContext(MenuScopeContext);
    const item = useContext(MenuItemContext);
    const shape = useShape();
    const sizeClasses = useSize();
    const buttonRef = useRef<HTMLElement | null>(null);

    // status="active" implies the row-active treatment; an explicit dot
    // overrides the status-derived one.
    const effectiveActive = isActive || status === "active";

    const setActive = item?.setActive;
    useIsoLayoutEffect(() => {
      setActive?.(effectiveActive);
      return () => setActive?.(false);
    }, [effectiveActive, setActive]);

    const setButtonEl = item?.setButtonEl;
    useIsoLayoutEffect(() => {
      setButtonEl?.(buttonRef.current);
      return () => setButtonEl?.(null);
    }, [setButtonEl]);
    const resolvedDot =
      dot ?? (status ? (status === "idle" ? "ring" : "filled") : undefined);
    const lit = effectiveActive || (item?.isHovered ?? false);
    const heightClass =
      size === "sm"
        ? "h-7"
        : size === "lg"
          ? "h-12"
          : sizeClasses.variant === "compact"
            ? "h-7"
            : "h-8";
    const textClass = size === "sm" ? typeClass("body", "compact") : sizeClasses.text;

    // Roving tabindex: the active rows' buttons are the menu's tab stops; with
    // no active row, the menu's first row keeps it keyboard-reachable.
    const row = item?.rowRef.current ?? null;
    const tabIdx = effectiveActive
      ? 0
      : scope?.hasActive
        ? -1
        : row !== null && row === scope?.firstRowEl
          ? 0
          : -1;

    // Exact trailing reservation: at rest, hover-revealed actions claim no
    // width (the label owns the row); once revealed the row widens to
    // --row-gutter-hover.
    const gutterHover = rowGutter(item?.actionCount ?? 0, item?.hasBadge ?? false);
    const gutterRest = item?.actionsShowOnHover
      ? rowGutter(0, item?.hasBadge ?? false)
      : gutterHover;
    const gutterVars = {
      "--row-gutter": `${gutterRest}px`,
      "--row-gutter-hover": `${gutterHover}px`,
    } as CSSProperties;

    const { template, content } = resolveSlotTemplate(render, asChild, children);

    const inner = (
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
        {!Icon && resolvedDot && (
          <span
            className="flex shrink-0 items-center justify-center"
            style={{ width: sizeClasses.icon, height: sizeClasses.icon }}
          >
            <span
              className={cn(
                "size-2 rounded-full transition-colors duration-80",
                resolvedDot === "filled"
                  ? lit
                    ? "bg-foreground/60"
                    : "bg-muted-foreground/50"
                  : lit
                    ? "border border-foreground/60"
                    : "border border-muted-foreground/50"
              )}
            />
          </span>
        )}
        <MenuRowLabel content={content} lit={lit} emphasized={effectiveActive} textClass={textClass} />
        {status === "unread" && <span className="sr-only">, unread</span>}
      </>
    );

    return (
      <SlotElement
        template={template}
        defaultTag="button"
        elementRef={(node: HTMLElement | null) => {
          buttonRef.current = node;
          if (typeof ref === "function") ref(node as HTMLButtonElement | null);
          else if (ref) (ref as React.MutableRefObject<HTMLElement | null>).current = node;
        }}
        props={{
          type: template ? undefined : "button",
          "data-sidebar": "menu-button",
          "data-size": size,
          "data-active": effectiveActive ? "true" : undefined,
          "data-status": status,
          "aria-current": effectiveActive ? "page" : undefined,
          tabIndex: tabIdx,
          className: cn(
            sidebarMenuButtonVariants({ variant }),
            heightClass,
            shape.item,
            className
          ),
          ...props,
          style: { ...gutterVars, ...(props.style ?? {}) },
        }}
      >
        {inner}
      </SlotElement>
    );
  }
);

SidebarMenuButton.displayName = "SidebarMenuButton";
export { MenuRowLabel, SidebarMenuButton };
