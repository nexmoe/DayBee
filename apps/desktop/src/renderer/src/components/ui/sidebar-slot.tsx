"use client";
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
// Fluid Functionalism registry source, split into modules to keep each file under 500 lines.
import { isValidElement, Children, type ReactNode, type ReactElement, type ElementType, type CSSProperties, type Ref } from "react";

import { cn } from "@/lib/utils";

// ─── Slot helpers (render / asChild polymorphism) ────────────────────────────
//
// A local slot instead of a primitive-library one so every menu part exists in
// exactly one flavor-neutral copy: Radix's Slot would leak into the Base UI
// flavor, and Base UI's useRender the other way around. Supports both the
// library's `render={<Link/>}` convention and shadcn's `asChild`.

type SlotProps = {
  className?: string;
  style?: CSSProperties;
  children?: ReactNode;
} & Record<string, unknown>;

function assignRef<T>(target: Ref<T> | undefined, node: T | null) {
  if (typeof target === "function") target(node);
  else if (target) (target as React.MutableRefObject<T | null>).current = node;
}

/** Resolves the element to clone: `render` wins, else `asChild`'s single
 *  element child. `content` is what should render inside it — for `asChild`
 *  the child element's own children, otherwise the caller's. */
export function resolveSlotTemplate(
  render: ReactElement | undefined,
  asChild: boolean | undefined,
  children: ReactNode
): { template: ReactElement<SlotProps> | null; content: ReactNode } {
  if (render && isValidElement(render)) {
    return { template: render as ReactElement<SlotProps>, content: children };
  }
  if (asChild) {
    const only = Children.toArray(children)[0];
    if (isValidElement(only)) {
      return {
        template: only as ReactElement<SlotProps>,
        content: (only.props as SlotProps).children,
      };
    }
  }
  return { template: null, content: children };
}

/** Renders `children` into the template element (merging class/style/
 *  handlers, composing refs) or into the default tag when there is no
 *  template. A component rather than a helper the parts call during their
 *  render: the forwarded ref it composes arrives as the `elementRef` prop,
 *  and only lands on the element at commit. */
export function SlotElement({
  template,
  defaultTag: DefaultTag,
  elementRef,
  props,
  children,
}: {
  template: ReactElement<SlotProps> | null;
  defaultTag: ElementType;
  elementRef?: Ref<HTMLElement>;
  props: SlotProps;
  children?: ReactNode;
}): ReactElement {
  if (!template) {
    const Tag = DefaultTag as ElementType;
    return (
      <Tag {...props} ref={elementRef}>
        {children}
      </Tag>
    );
  }
  const TemplateTag = template.type as ElementType;
  const templateProps = template.props;
  const merged: SlotProps = {
    ...props,
    ...templateProps,
    className: cn(props.className, templateProps.className),
    style: { ...props.style, ...(templateProps.style as CSSProperties | undefined) },
  };
  // Chain duplicated event handlers, template's first (it owns the element).
  for (const key of Object.keys(props)) {
    if (!/^on[A-Z]/.test(key)) continue;
    const ours = props[key];
    const theirs = templateProps[key];
    if (typeof ours === "function" && typeof theirs === "function") {
      merged[key] = (...args: unknown[]) => {
        (theirs as (...a: unknown[]) => void)(...args);
        (ours as (...a: unknown[]) => void)(...args);
      };
    }
  }
  const templateRef =
    (templateProps as { ref?: Ref<HTMLElement> }).ref ??
    (template as unknown as { ref?: Ref<HTMLElement> }).ref;
  // Both refs get the element, through a callback that only runs at commit.
  const composedRef = (node: HTMLElement | null) => {
    assignRef(elementRef, node);
    assignRef(templateRef, node);
  };
  return (
    <TemplateTag key={template.key ?? undefined} {...merged} ref={composedRef}>
      {children}
    </TemplateTag>
  );
}
export { assignRef };
export type { SlotProps };
