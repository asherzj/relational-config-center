const { writeFileSync } = require('node:fs');

// Host callbacks never evaluate the page or inspect event payloads.
function createLifecycleRecorder(write, readPhase, clock) {
  const events = [];
  const errors = [];
  const pages = new WeakMap();
  let pageSequence = 0;
  let cleanupStarted = false;
  let limited = false;
  let sealed = false;
  const limit = () => {
    if (!limited) errors.push({ kind: 'capture-limit' });
    limited = true;
  };
  function record(event, identity = {}) {
    if (sealed) return;
    // The suite opens three pages. Keep bounded room for unexpected popups/events.
    if (events.length >= 32) { limit(); return; }
    const entry = { sequence: events.length + 1, elapsedMs: clock(), phase: readPhase(), cleanupStarted,
      event, contextLabel: identity.contextLabel ?? null, pageId: identity.pageId ?? null,
      pageCreatedPhase: identity.pageCreatedPhase ?? null };
    events.push(entry);
    try { write(JSON.stringify(entry) + '\n'); }
    catch { errors.push({ kind: 'lifecycle-write-failed', sequence: entry.sequence }); }
  }
  return {
    observeContext(context, contextLabel) {
      if (!['applicant', 'approver'].includes(contextLabel)) throw new Error('Unknown evidence context');
      const observePage = page => {
        if (sealed) return;
        if (pages.has(page)) return;
        if (pageSequence >= 8) { limit(); return; }
        const identity = { contextLabel, pageId: ++pageSequence, pageCreatedPhase: readPhase() };
        pages.set(page, identity);
        page.on('crash', () => record('page-crash', identity));
        page.on('close', () => record('page-close', identity));
      };
      context.on('page', observePage);
      for (const page of context.pages()) observePage(page);
      context.on('close', () => record('context-close', { contextLabel }));
    },
    observeBrowser(browser) { browser.on('disconnected', () => record('browser-disconnected')); },
    recordFailure(page) { record('failure', pages.get(page)); },
    startCleanup(page) { cleanupStarted = true; record('cleanup-start', pages.get(page)); },
    cleanupFailed() { if (!sealed) { record('browser-close-failed'); errors.push({ kind: 'browser-close-failed' }); } },
    seal() { sealed = true; return this.snapshot(); },
    get complete() { return errors.length === 0; },
    snapshot() {
      return { complete: errors.length === 0, events: events.map(event => ({ ...event })), errors: errors.map(error => ({ ...error })) };
    },
  };
}

// Capture stays in memory until it wins the deadline. Only synchronous persistence
// is accepted here, so a timed-out browser Promise cannot write an artifact later.
async function captureArtifact(capture, artifactPath, deadlineMs = 2000) {
  const started = performance.now();
  let timer;
  try {
    const result = await Promise.race([
      Promise.resolve().then(capture).then(value => ({ status: 'captured', value }),
        () => ({ status: 'unavailable', writeCompleted: false })),
      new Promise(resolve => {
        timer = setTimeout(() => resolve({ status: 'timeout', writeCompleted: false }), deadlineMs);
      }),
    ]);
    if (result.status !== 'captured') return result;
    if (performance.now() - started >= deadlineMs) return { status: 'timeout', writeCompleted: false };
    if (artifactPath) {
      try {
        writeFileSync(artifactPath, result.value);
        // Synchronous disk I/O cannot be preempted; report an overrun accurately.
        return { status: performance.now() - started >= deadlineMs ? 'timeout' : 'captured', writeCompleted: true };
      } catch { return { status: 'write-failed', writeCompleted: false }; }
    }
    // Detach the result from late/mutable objects; serialization failure is evidence loss.
    return { status: 'captured', value: JSON.parse(JSON.stringify(result.value)) };
  } catch { return { status: 'unavailable', writeCompleted: false }; }
  finally { clearTimeout(timer); }
}

// A single post-failure window: these values say nothing about the preceding click.
function failureSnapshot(elements) {
  const rect = element => {
    if (!element) return null;
    const { x, y, width, height } = element.getBoundingClientRect();
    return { x, y, width, height };
  };
  const target = elements.length === 1 ? elements[0] : null;
  const surface = target?.closest('[role="dialog"]');
  const active = document.activeElement;
  return {
    window: 'single-post-failure-snapshot', duringClick: 'unknown',
    visibility: document.visibilityState, hasFocus: document.hasFocus(),
    activeElement: active ? { tag: active.tagName, role: active.getAttribute('role') } : null,
    viewport: { width: innerWidth, height: innerHeight, scrollX, scrollY },
    document: { scrollWidth: document.documentElement.scrollWidth, scrollHeight: document.documentElement.scrollHeight },
    target: { status: elements.length === 1 ? 'observed' : elements.length === 0 ? 'absent' : 'ambiguous',
      rect: rect(target), focused: target ? target === active : null },
    surface: surface ? { rect: rect(surface), scrollTop: surface.scrollTop, scrollLeft: surface.scrollLeft,
      scrollHeight: surface.scrollHeight, clientHeight: surface.clientHeight,
      transform: getComputedStyle(surface).transform, containsFocus: surface.contains(active) } : null,
  };
}

async function captureFailureArtifacts(page, output) {
  const unavailable = () => ({ complete: false, boundary: 'post-failure-only; during-click state unknown',
    snapshot: { status: 'unavailable' }, screenshot: { status: 'unavailable' }, body: { status: 'unavailable' } });
  try {
    if (!page || typeof output !== 'string') return unavailable();
    const [snapshot, screenshot, body] = await Promise.all([
      captureArtifact(() => page.getByRole('button', { name: 'note 申请值：转换为 LF 再编辑', exact: true }).evaluateAll(failureSnapshot)),
      captureArtifact(() => page.screenshot({ fullPage: true, timeout: 2000 }), `${output}/failure.png`),
      captureArtifact(() => page.locator('body').innerText({ timeout: 2000 }), `${output}/failure-body.txt`),
    ]);
    return { complete: [snapshot, screenshot, body].every(item => item.status === 'captured'),
      boundary: 'post-failure-only; during-click state unknown', snapshot, screenshot, body };
  } catch { return unavailable(); }
}

module.exports = { createLifecycleRecorder, captureArtifact, captureFailureArtifacts };
