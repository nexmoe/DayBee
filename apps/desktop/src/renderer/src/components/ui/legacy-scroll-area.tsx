import * as ScrollAreaPrimitive from "@radix-ui/react-scroll-area";
import { createContext, useContext, useState } from "react";
import { cn } from "@/lib/cn";

const ScrollViewportContext = createContext<HTMLDivElement | null>(null);

export const useScrollAreaViewport = () => useContext(ScrollViewportContext);

export function ScrollArea({
  className,
  children,
  ...props
}: ScrollAreaPrimitive.ScrollAreaProps) {
  const [viewport, setViewport] = useState<HTMLDivElement | null>(null);
  return (
    <ScrollAreaPrimitive.Root
      className={cn("relative w-full overflow-hidden", className)}
      {...props}
    >
      <ScrollAreaPrimitive.Viewport ref={setViewport} className="h-full w-full overflow-x-hidden rounded-[inherit] [&>div]:!block [&>div]:!w-full [&>div]:!min-w-0 [&>div]:!max-w-full">
        <ScrollViewportContext.Provider value={viewport}>
          <div className="min-h-full w-full min-w-0 max-w-full">{children}</div>
        </ScrollViewportContext.Provider>
      </ScrollAreaPrimitive.Viewport>
      <ScrollBar orientation="vertical" />
      <ScrollAreaPrimitive.Corner />
    </ScrollAreaPrimitive.Root>
  );
}

function ScrollBar({
  className,
  orientation = "vertical",
  ...props
}: ScrollAreaPrimitive.ScrollAreaScrollbarProps) {
  return (
    <ScrollAreaPrimitive.Scrollbar
      className={cn(
        "flex touch-none select-none p-1 transition-colors",
        orientation === "vertical" && "absolute inset-y-0 right-0 w-3.5",
        orientation === "horizontal" && "absolute inset-x-0 bottom-0 h-3.5 flex-col",
        className
      )}
      orientation={orientation}
      {...props}
    >
      <ScrollAreaPrimitive.Thumb className="relative flex-1 rounded-full bg-[color:var(--border)]/80" />
    </ScrollAreaPrimitive.Scrollbar>
  );
}
