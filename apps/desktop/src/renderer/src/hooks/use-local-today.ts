import { useEffect, useState } from "react";
import { localDateKey } from "@/lib/speech-activity";

export function useLocalToday() {
  const [today, setToday] = useState(() => localDateKey(new Date()));
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    const update = () => {
      clearTimeout(timer);
      const now = new Date();
      setToday(localDateKey(now));
      const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
      timer = setTimeout(update, midnight.getTime() - now.getTime() + 100);
    };
    update();
    window.addEventListener("focus", update);
    document.addEventListener("visibilitychange", update);
    return () => {
      clearTimeout(timer);
      window.removeEventListener("focus", update);
      document.removeEventListener("visibilitychange", update);
    };
  }, []);
  return today;
}
