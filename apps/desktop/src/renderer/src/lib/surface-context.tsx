"use client";

import { createContext, useContext, type ReactNode } from "react";

// ---------------------------------------------------------------------------
// The surface level the children sit on, on an 8-step ladder (1 is the page).
// Raised components (menus, popovers, dialogs) read it and pick their own
// level relative to it instead of hard-coding one, then provide their new
// level to what they contain. A menu that hard-codes its background ends up
// the same color as the dialog it opens in and melts into it; reading the
// substrate lands it one step up wherever it opens, with no prop passed down.
// ---------------------------------------------------------------------------

// No provider means the page itself.
const SurfaceContext = createContext<number>(1);

export function useSurface(): number {
  return useContext(SurfaceContext);
}

export function SurfaceProvider({
  value,
  children,
}: {
  value: number;
  children: ReactNode;
}) {
  // Clamped to the ladder: a deep stack keeps resolving to a real surface
  // token instead of a level with no background or shadow defined.
  return (
    <SurfaceContext.Provider value={Math.max(1, Math.min(8, value))}>
      {children}
    </SurfaceContext.Provider>
  );
}
