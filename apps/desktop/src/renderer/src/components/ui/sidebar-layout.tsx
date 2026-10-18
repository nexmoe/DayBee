"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "@/lib/utils";

import { useShape } from "@/lib/shape-context";
import { useSize } from "@/lib/size-context";

// ─── SidebarInset ────────────────────────────────────────────────────────────

export type SidebarInsetProps = HTMLAttributes<HTMLElement>;

const SidebarInset = forwardRef<HTMLElement, SidebarInsetProps>(
  ({ className, ...props }, ref) => {
    const shape = useShape();
    return (
      <main
        ref={ref}
        data-slot="sidebar-inset"
        className={cn(
          "relative flex min-h-0 w-full min-w-0 flex-1 flex-col bg-background",
          "peer-data-[variant=inset]:m-2 peer-data-[variant=inset]:peer-data-[side=left]:ml-0 peer-data-[variant=inset]:peer-data-[side=right]:mr-0",
          // With the rail collapsed away, restore the sidebar-side margin so
          // the card keeps symmetric insets.
          "peer-data-[variant=inset]:peer-data-[state=collapsed]:peer-data-[side=left]:ml-2 peer-data-[variant=inset]:peer-data-[state=collapsed]:peer-data-[side=right]:mr-2",
          "transition-[margin] duration-80",
          // Container radius follows the shape system (literal classes so
          // Tailwind's scanner emits both).
          shape.bgRadius >= 20
            ? "peer-data-[variant=inset]:rounded-3xl"
            : "peer-data-[variant=inset]:rounded-xl",
          "peer-data-[variant=inset]:bg-surface-2 peer-data-[variant=inset]:shadow-surface-2",
          className
        )}
        {...props}
      />
    );
  }
);

SidebarInset.displayName = "SidebarInset";

// ─── SidebarInput ────────────────────────────────────────────────────────────

export type SidebarInputProps = React.InputHTMLAttributes<HTMLInputElement>;

const SidebarInput = forwardRef<HTMLInputElement, SidebarInputProps>(
  ({ className, ...props }, ref) => {
    const shape = useShape();
    const size = useSize();
    return (
      <input
        ref={ref}
        data-sidebar="input"
        className={cn(
          // Mirrors the InputGroup field ladder: transparent at rest,
          // muted fill + border ring on hover, card fill when focused.
          "w-full bg-transparent px-3 text-foreground placeholder:text-muted-foreground outline-none",
          "ring-1 ring-transparent transition-[background-color,box-shadow] duration-80",
          "hover:bg-muted/50 hover:ring-border",
          "focus:bg-card focus:ring-border",
          "focus-visible:ring-[color:var(--focus-ring,#6B97FF)]",
          size.variant === "compact" ? "h-7" : "h-8",
          size.field,
          shape.input,
          className
        )}
        {...props}
      />
    );
  }
);

SidebarInput.displayName = "SidebarInput";

// ─── SidebarHeader / SidebarFooter / SidebarSeparator ────────────────────────

export type SidebarSectionProps = HTMLAttributes<HTMLDivElement>;

const SidebarHeader = forwardRef<HTMLDivElement, SidebarSectionProps>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-sidebar="header"
      className={cn("flex shrink-0 flex-col gap-2 p-2", className)}
      {...props}
    />
  )
);

SidebarHeader.displayName = "SidebarHeader";

const SidebarFooter = forwardRef<HTMLDivElement, SidebarSectionProps>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-sidebar="footer"
      className={cn("mt-auto flex shrink-0 flex-col gap-2 p-2", className)}
      {...props}
    />
  )
);

SidebarFooter.displayName = "SidebarFooter";

const SidebarSeparator = forwardRef<HTMLDivElement, SidebarSectionProps>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-sidebar="separator"
      role="separator"
      aria-orientation="horizontal"
      className={cn("mx-2 h-px shrink-0 bg-border", className)}
      {...props}
    />
  )
);

SidebarSeparator.displayName = "SidebarSeparator";
export { SidebarInset, SidebarInput, SidebarHeader, SidebarFooter, SidebarSeparator };
