import ReactDOM from "react-dom/client";
import { MotionConfig } from "framer-motion";
import { AppErrorBoundary } from "@/components/app-error-boundary";
import { App } from "./app";
import "./styles.css";

document.body.dataset.platform = navigator.userAgent.toLowerCase().includes("windows")
  ? "win32"
  : "other";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <AppErrorBoundary>
    <MotionConfig reducedMotion="user"><App /></MotionConfig>
  </AppErrorBoundary>
);
