import { useEffect } from "react";
import type { ThemeMode } from "@daybee/shared";

export function useWindowTheme(theme: ThemeMode) {
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const sync = () => document.documentElement.classList.toggle("dark", theme === "dark" || (theme === "system" && media.matches));
    sync();
    media.addEventListener("change", sync);
    return () => media.removeEventListener("change", sync);
  }, [theme]);
}
