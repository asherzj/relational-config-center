async function diagnosticDeadline(operation) {
  let timer;
  try {
    return await Promise.race([operation, new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error('diagnostic operation exceeded 2000 ms')), 2000);
    })]);
  } finally { clearTimeout(timer); }
}

async function stopProbe(probe) {
  const observation = await diagnosticDeadline(probe.evaluate(value => value.stop()))
    .catch(error => ({ samplingError: String(error) }));
  await diagnosticDeadline(probe.dispose()).catch(error => { observation.disposeError = String(error); });
  return observation;
}

// Observe the existing click without changing its timeout or actionability checks.
async function clickWithDiagnostics(locator, observations) {
  let probe, setupError;
  const pendingProbe = locator.evaluateHandle(element => {
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
  }, undefined, { timeout: 2000 });
  try { probe = await diagnosticDeadline(pendingProbe); } catch (error) {
    setupError = String(error);
    // A renderer can answer after our deadline; clean up that late observer too.
    pendingProbe.then(stopProbe).catch(() => {});
  }
  const started = Date.now();
  let failure = null;
  try {
    await locator.click();
  } catch (error) {
    failure = { name: error.name, message: error.message };
    throw error;
  } finally {
    const observation = probe
      ? await stopProbe(probe)
      : { setupError };
    observations.push({ locator: locator.toString(), elapsedMs: Date.now() - started, failure, ...observation });
  }
}
module.exports = { clickWithDiagnostics, diagnosticDeadline };
