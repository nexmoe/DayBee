import type { ButtonHTMLAttributes } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 rounded-lg text-sm font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--ring)] disabled:pointer-events-none disabled:opacity-50",
  {
    defaultVariants: {
      size: "default",
      variant: "default"
    },
    variants: {
      size: {
        default: "h-10 px-4",
        sm: "h-8 px-3.5 text-xs"
      },
      variant: {
        default:
          "bg-gradient-to-b from-[color:var(--accent)] to-[color:color-mix(in_srgb,var(--accent)_85%,black)] text-[color:var(--accent-foreground)] shadow-[var(--shadow-raised)] hover:brightness-110 active:shadow-[var(--shadow-inset)] active:brightness-95",
        ghost:
          "bg-transparent text-[color:var(--foreground)] hover:bg-[color:var(--surface-soft)] hover:shadow-[var(--shadow-raised-sm)]",
        subtle:
          "bg-[color:var(--surface)] text-[color:var(--foreground)] ring-1 ring-[color:var(--border)] shadow-[var(--shadow-raised-sm)] active:shadow-[var(--shadow-inset)]"
      }
    }
  }
);

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof buttonVariants>;

export function Button({ className, size, variant, ...props }: ButtonProps) {
  return (
    <button
      className={cn(buttonVariants({ className, size, variant }))}
      {...props}
    />
  );
}
