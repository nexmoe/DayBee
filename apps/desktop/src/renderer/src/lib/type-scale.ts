type TypeScaleVariant = "default" | "compact";

/** One role at one ladder step, in px. */
interface TypeScalePair {
  size: number;
  leading: number;
}

/** One role's font size at each ladder step, in px. */
interface TypeScaleStep {
  default: number;
  compact: number;
}

const typeStyles = {
  /** Page titles. */
  display: {
    default: { size: 28, leading: 34 },
    compact: { size: 24, leading: 30 },
  },
  /** Section headings, dialog titles. */
  title: {
    default: { size: 16, leading: 22 },
    compact: { size: 15, leading: 20 },
  },
  /** Card titles, chat bubbles, emphasized rows. */
  subtitle: {
    default: { size: 14, leading: 20 },
    compact: { size: 13, leading: 18 },
  },
  /** Control labels, body copy, and paragraphs. 20px also puts a row of
   *  padded text on the 36px ladder (8 + 20 + 8), 18px on the 28px one. */
  body: {
    default: { size: 13, leading: 20 },
    compact: { size: 12, leading: 18 },
  },
  /** Secondary text: descriptions, meta rows, errors, group labels. */
  caption: {
    default: { size: 12, leading: 16 },
    compact: { size: 11, leading: 14 },
  },
  /** Keyboard caps, counters, tiny badges. Single line only. */
  micro: {
    default: { size: 11, leading: 14 },
    compact: { size: 10, leading: 12 },
  },
} as const satisfies Record<string, Record<TypeScaleVariant, TypeScalePair>>;

type TypeScaleRole = keyof typeof typeStyles;

const typeScaleRoles = Object.keys(typeStyles) as TypeScaleRole[];

/** Font sizes alone, per role and step: the shape `typeScale` has always
 *  had, for code that reads it as numbers. */
const typeScale = Object.fromEntries(
  typeScaleRoles.map((role) => [
    role,
    { default: typeStyles[role].default.size, compact: typeStyles[role].compact.size },
  ])
) as Record<TypeScaleRole, TypeScaleStep>;

// Class strings per role and step (size + leading). Arbitrary values over the CSS variables,
// with the px values as fallbacks, so a component renders the right size even
// where the `type-scale` tokens were never installed, and stock
// tailwind-merge reads them as font-size / line-height (a bare `text-caption`
// would be taken for a color and dropped next to `text-muted-foreground`).
// Literal strings on purpose: Tailwind only generates classes it can read in
// source. Written by scripts/generate-type-scale.mjs, do not edit by hand.
// <generated:type-classes>
const typeClasses = {
  default: {
    display: "text-[length:var(--fs-display,28px)] leading-[var(--lh-display,34px)]",
    title: "text-[length:var(--fs-title,16px)] leading-[var(--lh-title,22px)]",
    subtitle: "text-[length:var(--fs-subtitle,14px)] leading-[var(--lh-subtitle,20px)]",
    body: "text-[length:var(--fs-body,13px)] leading-[var(--lh-body,20px)]",
    caption: "text-[length:var(--fs-caption,12px)] leading-[var(--lh-caption,16px)]",
    micro: "text-[length:var(--fs-micro,11px)] leading-[var(--lh-micro,14px)]",
  },
  compact: {
    display: "text-[length:var(--fs-display-compact,24px)] leading-[var(--lh-display-compact,30px)]",
    title: "text-[length:var(--fs-title-compact,15px)] leading-[var(--lh-title-compact,20px)]",
    subtitle: "text-[length:var(--fs-subtitle-compact,13px)] leading-[var(--lh-subtitle-compact,18px)]",
    body: "text-[length:var(--fs-body-compact,12px)] leading-[var(--lh-body-compact,18px)]",
    caption: "text-[length:var(--fs-caption-compact,11px)] leading-[var(--lh-caption-compact,14px)]",
    micro: "text-[length:var(--fs-micro-compact,10px)] leading-[var(--lh-micro-compact,12px)]",
  },
} as const satisfies Record<TypeScaleVariant, Record<TypeScaleRole, string>>;

// </generated:type-classes>

/** The class string for one role at one step (size + leading). */
function typeClass(role: TypeScaleRole, variant: TypeScaleVariant = "default"): string {
  return typeClasses[variant][role];
}

/** Editable fields add this after their role. iOS Safari zooms the page into
 *  a focused field set under 16px, so touch screens get 16px. It sets the
 *  size only: the role's leading and the field's fixed height stay, and
 *  desktop keeps the role. `useSize().field` is body plus this. */
const fieldTouchClass = "pointer-coarse:text-[16px]";

export { typeStyles, typeScale, typeScaleRoles, typeClasses, typeClass, fieldTouchClass };
export type { TypeScaleRole, TypeScaleVariant, TypeScalePair, TypeScaleStep };
