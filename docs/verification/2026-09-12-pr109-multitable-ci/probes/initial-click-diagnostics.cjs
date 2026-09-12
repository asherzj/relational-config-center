// Observe the existing click without changing its timeout or actionability checks.
async function clickWithDiagnostics(locator, observations) {
  const probe = await locator.evaluateHandle(element => {
    const started = performance.now();
    const samples = [];
    let frames = 0, lastFrame = started, maxFrameGap = 0, frame;
    const tick = now => {
      frames++;
      maxFrameGap = Math.max(maxFrameGap, now - lastFrame);
      lastFrame = now;
      frame = requestAnimationFrame(tick);
    };
    const sample = () => {
      if (samples.length >= 250) return;
      const surface = element.closest('[role="dialog"]');
      samples.push({
        elapsedMs: performance.now() - started,
        visibility: document.visibilityState, hasFocus: document.hasFocus(),
        connected: element.isConnected, rect: element.getBoundingClientRect().toJSON(),
        surfaceTransform: surface ? getComputedStyle(surface).transform : null,
        animations: (surface?.getAnimations() || []).map(animation => ({
          name: animation.animationName, state: animation.playState, time: animation.currentTime,
        })),
        frames, maxFrameGapMs: maxFrameGap,
      });
    };
    sample();
    const timer = setInterval(sample, 100);
    frame = requestAnimationFrame(tick);
    return { stop() {
      clearInterval(timer); cancelAnimationFrame(frame); sample();
      return { samples, frames, maxFrameGapMs: maxFrameGap, lastFrameAgeMs: performance.now() - lastFrame };
    } };
  });
  const started = Date.now();
  let failure = null;
  try {
    await locator.click();
  } catch (error) {
    failure = { name: error.name, message: error.message };
    throw error;
  } finally {
    const observation = await probe.evaluate(value => value.stop()).catch(error => ({ samplingError: String(error) }));
    await probe.dispose().catch(() => {});
    observations.push({ locator: locator.toString(), elapsedMs: Date.now() - started, failure, ...observation });
  }
}
module.exports = { clickWithDiagnostics };
