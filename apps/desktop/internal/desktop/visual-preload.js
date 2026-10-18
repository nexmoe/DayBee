// Only loaded with DAYBEE_E2E_TEST=1 and DAYBEE_TEST_VISUAL=1. Both the archived
// Electron baseline and the native WebView use these exact frame inputs.
(() => {
	const NativeResizeObserver = ResizeObserver;
	window.ResizeObserver = class extends NativeResizeObserver {
		constructor(callback) {
			super((entries, observer) => {
				document.getAnimations().forEach(a => {
					if (a.effect.getComputedTiming().iterations !== Infinity) a.finish();
				});
				callback(entries, observer);
			});
		}
	};
  let next = 1;
  let now = 0;
  let callbacks = new Map();
  window.requestAnimationFrame = callback => { const id = next++; callbacks.set(id, callback); return id; };
  window.cancelAnimationFrame = id => callbacks.delete(id);
  window.__daybeeStep = (frames = 1) => {
    for (let frame = 0; frame < frames; frame++) {
      now += 1000 / 60;
      const pending = callbacks; callbacks = new Map();
      for (const callback of pending.values()) callback(now);
    }
  };
  window.__daybeeSpectrum = Array.from({ length: 128 }, (_, i) => (i * 29 + 17) % 256);
  const context = CanvasRenderingContext2D.prototype;
  const clear = context.clearRect;
  context.clearRect = function (...args) {
    window.__daybeeCanvasCommands = [];
    return clear.apply(this, args);
  };
  for (const method of ["roundRect", "fillRect", "moveTo", "lineTo", "arc"]) {
    const original = context[method];
    context[method] = function (...args) {
      (window.__daybeeCanvasCommands ||= []).push([method, ...args]);
      return original.apply(this, args);
    };
  }
})();
