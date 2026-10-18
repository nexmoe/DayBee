"use client";
// Public API for the installed Fluid Functionalism component.
export { SIDEBAR_COOKIE_NAME, SIDEBAR_COOKIE_MAX_AGE, SIDEBAR_WIDTH, SIDEBAR_WIDTH_MOBILE, SIDEBAR_KEYBOARD_SHORTCUT, SIDEBAR_KEYBOARD_SHORTCUT_RIGHT, SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH, SIDEBAR_COLLAPSE_SLOP, useSidebar } from "./sidebar-context";
export type { SidebarSide, SidebarVariant, SidebarCollapsible, SidebarContextValue } from "./sidebar-context";
export { SidebarProvider } from "./sidebar-provider";
export type { SidebarProviderProps } from "./sidebar-provider";
export { resolveSlotTemplate, SlotElement } from "./sidebar-slot";
export { SidebarShell } from "./sidebar-shell";
export type { SidebarShellProps } from "./sidebar-shell";
export { SidebarTrigger, SidebarRail } from "./sidebar-controls";
export type { SidebarTriggerProps, SidebarRailProps } from "./sidebar-controls";
export { SidebarInset, SidebarInput, SidebarHeader, SidebarFooter, SidebarSeparator } from "./sidebar-layout";
export type { SidebarInsetProps, SidebarInputProps, SidebarSectionProps } from "./sidebar-layout";
export { SidebarGroup, SidebarGroupLabel } from "./sidebar-group";
export type { SidebarGroupProps, SidebarGroupLabelProps } from "./sidebar-group";
export { SidebarGroupAction, SidebarGroupActions, SidebarGroupContent } from "./sidebar-group-actions";
export type { SidebarGroupActionProps, SidebarGroupActionsProps } from "./sidebar-group-actions";
