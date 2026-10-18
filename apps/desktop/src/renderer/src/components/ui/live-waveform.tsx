import { desktopEvents } from "@/lib/desktop-api";
import { cn } from "@/lib/cn";
import { type HTMLAttributes, useEffect, useRef } from "react";

export type LiveWaveformProps = HTMLAttributes<HTMLDivElement> & {
  active?: boolean;
  processing?: boolean;
  barWidth?: number;
  barGap?: number;
  barRadius?: number;
  barColor?: string;
  fadeEdges?: boolean;
  fadeWidth?: number;
  height?: string | number;
  sensitivity?: number;
  historySize?: number;
  updateRate?: number;
  mode?: "scrolling" | "static";
  centerStaticBars?: boolean;
  processingSpeed?: number;
};
/**
 * LiveWaveform renders the native frequency spectrum using the original canvas.
 */
export const LiveWaveform = ({
  active = false,
  processing = false,
  barWidth = 3,
  barGap = 1,
  barRadius = 1.5,
  barColor,
  fadeEdges = true,
  fadeWidth = 24,
  height = 64,
  sensitivity = 1,
  historySize = 60,
  updateRate = 30,
  mode = "static",
  centerStaticBars = false,
  processingSpeed = 0.03,
  className,
  ...props
}: LiveWaveformProps) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const historyRef = useRef<number[]>([]);
  const lastUpdateRef = useRef<number>(0);
  const processingAnimationRef = useRef<number | null>(null);
  const lastActiveDataRef = useRef<number[]>([]);
  const transitionProgressRef = useRef(0);
  const staticBarsRef = useRef<number[]>([]);
  const needsRedrawRef = useRef(true);
  const gradientCacheRef = useRef<CanvasGradient | null>(null);
  const lastWidthRef = useRef(0);
  const frequencyDataRef = useRef<Uint8Array<ArrayBuffer> | null>(null);
  const rectRef = useRef({ width: 0, height: 0 });
  const colorRef = useRef("#000");
  const fadeRafRef = useRef<number | null>(null);
  const scheduleDrawRef = useRef<() => void>(() => {});

  const heightStyle = typeof height === "number" ? `${height}px` : height;

  // Handle canvas resizing
  useEffect(() => {
    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!(canvas && container)) {
      return;
    }

    const readColor = () => {
      colorRef.current = barColor || getComputedStyle(canvas).color || "#000";
    };
    const applyRect = () => {
      const rect = container.getBoundingClientRect();
      rectRef.current = { width: rect.width, height: rect.height };
      const dpr = window.devicePixelRatio || 1;

      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      canvas.style.width = `${rect.width}px`;
      canvas.style.height = `${rect.height}px`;

      const ctx = canvas.getContext("2d");
      if (ctx) {
        ctx.scale(dpr, dpr);
      }

      gradientCacheRef.current = null;
      lastWidthRef.current = rect.width;
      readColor();
      needsRedrawRef.current = true;
      scheduleDrawRef.current();
    };

    const onTheme = () => {
      readColor();
      needsRedrawRef.current = true;
      scheduleDrawRef.current();
    };
    const resizeObserver = new ResizeObserver(applyRect);
    const themeObserver = new MutationObserver(onTheme);
    const colorScheme = window.matchMedia("(prefers-color-scheme: dark)");

    resizeObserver.observe(container);
    themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
    colorScheme.addEventListener("change", onTheme);
    applyRect();
    return () => {
      resizeObserver.disconnect();
      themeObserver.disconnect();
      colorScheme.removeEventListener("change", onTheme);
    };
  }, [barColor]);

  useEffect(() => {
    const cancelFade = () => {
      if (fadeRafRef.current !== null) {
        cancelAnimationFrame(fadeRafRef.current);
        fadeRafRef.current = null;
      }
    };
    cancelFade();

    if (processing && !active) {
      let time = 0;
      transitionProgressRef.current = 0;

      const animateProcessing = () => {
        time += processingSpeed;
        transitionProgressRef.current = Math.min(1, transitionProgressRef.current + 0.02);

        const processingData: number[] = [];
        const barCount = Math.floor((rectRef.current.width || 200) / (barWidth + barGap));

        if (mode === "static") {
          const halfCount = Math.floor(barCount / 2);

          for (let i = 0; i < barCount; i++) {
            const normalizedPosition = (i - halfCount) / halfCount;
            const centerWeight = 1 - Math.abs(normalizedPosition) * 0.4;

            const wave1 = Math.sin(time * 1.5 + normalizedPosition * 3) * 0.25;
            const wave2 = Math.sin(time * 0.8 - normalizedPosition * 2) * 0.2;
            const wave3 = Math.cos(time * 2 + normalizedPosition) * 0.15;
            const combinedWave = wave1 + wave2 + wave3;
            const processingValue = (0.2 + combinedWave) * centerWeight;

            let finalValue = processingValue;
            if (lastActiveDataRef.current.length > 0 && transitionProgressRef.current < 1) {
              const lastDataIndex = Math.min(i, lastActiveDataRef.current.length - 1);
              const lastValue = lastActiveDataRef.current[lastDataIndex] || 0;
              finalValue =
                lastValue * (1 - transitionProgressRef.current) +
                processingValue * transitionProgressRef.current;
            }

            processingData.push(Math.max(0.05, Math.min(1, finalValue)));
          }
        } else {
          for (let i = 0; i < barCount; i++) {
            const normalizedPosition = (i - barCount / 2) / (barCount / 2);
            const centerWeight = 1 - Math.abs(normalizedPosition) * 0.4;

            const wave1 = Math.sin(time * 1.5 + i * 0.15) * 0.25;
            const wave2 = Math.sin(time * 0.8 - i * 0.1) * 0.2;
            const wave3 = Math.cos(time * 2 + i * 0.05) * 0.15;
            const combinedWave = wave1 + wave2 + wave3;
            const processingValue = (0.2 + combinedWave) * centerWeight;

            let finalValue = processingValue;
            if (lastActiveDataRef.current.length > 0 && transitionProgressRef.current < 1) {
              const lastDataIndex = Math.floor((i / barCount) * lastActiveDataRef.current.length);
              const lastValue = lastActiveDataRef.current[lastDataIndex] || 0;
              finalValue =
                lastValue * (1 - transitionProgressRef.current) +
                processingValue * transitionProgressRef.current;
            }

            processingData.push(Math.max(0.05, Math.min(1, finalValue)));
          }
        }

        if (mode === "static") {
          staticBarsRef.current = processingData;
        } else {
          historyRef.current = processingData;
        }

        needsRedrawRef.current = true;
        if (document.hidden) return;
        processingAnimationRef.current = requestAnimationFrame(animateProcessing);
        scheduleDrawRef.current();
      };

      const onVisibility = () => {
        if (!document.hidden) animateProcessing();
      };
      document.addEventListener("visibilitychange", onVisibility);
      if (!document.hidden) animateProcessing();

      return () => {
        document.removeEventListener("visibilitychange", onVisibility);
        if (processingAnimationRef.current) {
          cancelAnimationFrame(processingAnimationRef.current);
        }
        cancelFade();
      };
    }
    if (!(active || processing)) {
      const hasData =
        mode === "static" ? staticBarsRef.current.length > 0 : historyRef.current.length > 0;

      if (hasData) {
        let fadeProgress = 0;
        const fadeToIdle = () => {
          fadeProgress += 0.03;
          if (fadeProgress < 1) {
            if (mode === "static") {
              staticBarsRef.current = staticBarsRef.current.map(
                (value) => value * (1 - fadeProgress)
              );
            } else {
              historyRef.current = historyRef.current.map((value) => value * (1 - fadeProgress));
            }
            needsRedrawRef.current = true;
            fadeRafRef.current = requestAnimationFrame(fadeToIdle);
            scheduleDrawRef.current();
          } else if (mode === "static") {
            staticBarsRef.current = [];
          } else {
            historyRef.current = [];
          }
        };
        if (!document.hidden) fadeToIdle();
      }
    }
    return () => {
      cancelFade();
    };
  }, [processing, active, barWidth, barGap, mode, processingSpeed]);

  useEffect(() => desktopEvents.onWaveform(({ spectrum }) => {
    frequencyDataRef.current = Uint8Array.from(spectrum);
  }), []);

  // Animation loop
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) {
      return;
    }

    const ctx = canvas.getContext("2d");
    if (!ctx) {
      return;
    }

    let rafId = 0;
    let queued = false;

    const hasBars = () =>
      mode === "static" ? staticBarsRef.current.length > 0 : historyRef.current.length > 0;
    const wantsFrame = () =>
      !document.hidden && (active || processing || needsRedrawRef.current || hasBars());
    const schedule = () => {
      if (queued || !wantsFrame()) return;
      queued = true;
      rafId = requestAnimationFrame(animate);
    };
    scheduleDrawRef.current = schedule;

    const animate = (currentTime: number) => {
      queued = false;
      if (!wantsFrame()) return;
      const rect = rectRef.current;

      // Update audio data if active
      if (active && currentTime - lastUpdateRef.current > updateRate) {
        lastUpdateRef.current = currentTime;

        if (frequencyDataRef.current) {
          const dataArray = frequencyDataRef.current;

          if (mode === "static") {
            // For static mode, update bars in place
            const startFreq = Math.floor(dataArray.length * 0.05);
            const endFreq = Math.floor(dataArray.length * 0.4);
            const relevantLen = endFreq - startFreq;

            const barCount = Math.max(1, Math.floor(rect.width / (barWidth + barGap)));
            const centerIndex = (barCount - 1) / 2;
            const newBars: number[] = [];

            // Build mirrored bars around center, while keeping odd/even counts symmetric.
            for (let i = 0; i < barCount; i++) {
              const normalizedDistance =
                centerIndex > 0 ? Math.abs(i - centerIndex) / centerIndex : 0;
              const dataIndex = Math.min(
                relevantLen - 1,
                Math.floor(normalizedDistance * (relevantLen - 1))
              );
              const value = Math.min(1, (dataArray[startFreq + dataIndex] / 255) * sensitivity);
              newBars.push(Math.max(0.05, value));
            }

            staticBarsRef.current = newBars;
            lastActiveDataRef.current = newBars;
          } else {
            // Scrolling mode - original behavior
            let sum = 0;
            const startFreq = Math.floor(dataArray.length * 0.05);
            const endFreq = Math.floor(dataArray.length * 0.4);
            const relevantLen = endFreq - startFreq;

            for (let i = startFreq; i < endFreq; i++) {
              sum += dataArray[i];
            }
            const average = (sum / relevantLen / 255) * sensitivity;

            // Add to history
            historyRef.current.push(Math.min(1, Math.max(0.05, average)));
            lastActiveDataRef.current = historyRef.current;

            // Maintain history size
            if (historyRef.current.length > historySize) {
              historyRef.current.shift();
            }
          }
          needsRedrawRef.current = true;
        }
      }

      if (!(needsRedrawRef.current || active)) {
        schedule();
        return;
      }

      needsRedrawRef.current = active;
      ctx.clearRect(0, 0, rect.width, rect.height);

      const computedBarColor = colorRef.current;

      const step = barWidth + barGap;
      const barCount = Math.floor(rect.width / step);
      const centerY = rect.height / 2;

      // Draw bars based on mode
      if (mode === "static") {
        // Static mode - bars in fixed positions
        const dataToRender = processing
          ? staticBarsRef.current
          : active
            ? staticBarsRef.current
            : staticBarsRef.current.length > 0
              ? staticBarsRef.current
              : [];
        const renderedBarCount = Math.min(barCount, dataToRender.length);
        const renderedBarsWidth =
          renderedBarCount > 0 ? renderedBarCount * barWidth + (renderedBarCount - 1) * barGap : 0;
        const staticStartX = centerStaticBars
          ? Math.max(0, (rect.width - renderedBarsWidth) / 2)
          : 0;

        for (let i = 0; i < renderedBarCount; i++) {
          const value = dataToRender[i] || 0.1;
          const x = staticStartX + i * step;
          const barHeight = Math.max(4, value * rect.height * 0.8);
          const y = centerY - barHeight / 2;

          ctx.fillStyle = computedBarColor;
          ctx.globalAlpha = 0.4 + value * 0.6;

          if (barRadius > 0) {
            ctx.beginPath();
            ctx.roundRect(x, y, barWidth, barHeight, barRadius);
            ctx.fill();
          } else {
            ctx.fillRect(x, y, barWidth, barHeight);
          }
        }
      } else {
        // Scrolling mode - original behavior
        for (let i = 0; i < barCount && i < historyRef.current.length; i++) {
          const dataIndex = historyRef.current.length - 1 - i;
          const value = historyRef.current[dataIndex] || 0.1;
          const x = rect.width - (i + 1) * step;
          const barHeight = Math.max(4, value * rect.height * 0.8);
          const y = centerY - barHeight / 2;

          ctx.fillStyle = computedBarColor;
          ctx.globalAlpha = 0.4 + value * 0.6;

          if (barRadius > 0) {
            ctx.beginPath();
            ctx.roundRect(x, y, barWidth, barHeight, barRadius);
            ctx.fill();
          } else {
            ctx.fillRect(x, y, barWidth, barHeight);
          }
        }
      }

      // Apply edge fading
      if (fadeEdges && fadeWidth > 0 && rect.width > 0) {
        // Cache gradient if width hasn't changed
        if (!gradientCacheRef.current || lastWidthRef.current !== rect.width) {
          const gradient = ctx.createLinearGradient(0, 0, rect.width, 0);
          const fadePercent = Math.min(0.3, fadeWidth / rect.width);

          // destination-out: removes destination where source alpha is high
          // We want: fade edges out, keep center solid
          // Left edge: start opaque (1) = remove, fade to transparent (0) = keep
          gradient.addColorStop(0, "rgba(255,255,255,1)");
          gradient.addColorStop(fadePercent, "rgba(255,255,255,0)");
          // Center stays transparent = keep everything
          gradient.addColorStop(1 - fadePercent, "rgba(255,255,255,0)");
          // Right edge: fade from transparent (0) = keep to opaque (1) = remove
          gradient.addColorStop(1, "rgba(255,255,255,1)");

          gradientCacheRef.current = gradient;
          lastWidthRef.current = rect.width;
        }

        ctx.globalCompositeOperation = "destination-out";
        ctx.fillStyle = gradientCacheRef.current;
        ctx.fillRect(0, 0, rect.width, rect.height);
        ctx.globalCompositeOperation = "source-over";
      }

      ctx.globalAlpha = 1;

      schedule();
    };

    document.addEventListener("visibilitychange", schedule);
    schedule();

    return () => {
      scheduleDrawRef.current = () => {};
      document.removeEventListener("visibilitychange", schedule);
      if (rafId) cancelAnimationFrame(rafId);
    };
  }, [
    active,
    processing,
    sensitivity,
    updateRate,
    historySize,
    barWidth,
    barGap,
    barRadius,
    barColor,
    fadeEdges,
    fadeWidth,
    mode,
    centerStaticBars
  ]);

  return (
    <div
      aria-label={
        active ? "Live audio waveform" : processing ? "Processing audio" : "Audio waveform idle"
      }
      className={cn("relative h-full w-full", className)}
      ref={containerRef}
      role="img"
      style={{ height: heightStyle }}
      {...props}
    >
      {!(active || processing) && (
        <div className="absolute top-1/2 right-0 left-0 -translate-y-1/2 border-[color:var(--border)] border-t border-dashed" />
      )}
      <canvas className="block h-full w-full text-[color:var(--foreground)]" ref={canvasRef} />
    </div>
  );
};
