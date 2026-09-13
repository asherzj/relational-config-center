import { EventEmitter } from 'node:events';
import { describe, expect, it, afterEach } from 'vitest';
import { mkdtempSync, readFileSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
const directories = [];
const outputDirectory = () => { const value = mkdtempSync(join(tmpdir(), 'rcc-evidence-control-')); directories.push(value); return value; };
afterEach(() => { for (const path of directories.splice(0)) rmSync(path, { recursive: true, force: true }); });
import evidence from './accessibility-evidence.cjs';
const { createLifecycleRecorder } = evidence;

describe('accessibility evidence', () => {
  it('attributes old-page callbacks and distinguishes cleanup from the business phase', () => {
    let phase = 'first-page';
    const lines = [];
    const recorder = createLifecycleRecorder(line => lines.push(JSON.parse(line)), () => phase, () => 10);
    const context = new EventEmitter();
    const first = new EventEmitter();
    context.pages = () => [first];
    recorder.observeContext(context, 'applicant');
    phase = 'lf-conversion';
    const second = new EventEmitter();
    context.emit('page', second);
    first.emit('crash', { url: 'private', message: 'secret' });
    recorder.recordFailure(second);
    recorder.startCleanup(second);
    second.emit('close');
    expect(lines.map(e => [e.event, e.pageId, e.pageCreatedPhase, e.phase, e.cleanupStarted])).toEqual([
      ['page-crash', 1, 'first-page', 'lf-conversion', false],
      ['failure', 2, 'lf-conversion', 'lf-conversion', false],
      ['cleanup-start', 2, 'lf-conversion', 'lf-conversion', true],
      ['page-close', 2, 'lf-conversion', 'lf-conversion', true],
    ]);
    expect(JSON.stringify(lines)).not.toMatch(/private|secret/);
    expect(recorder.snapshot().complete).toBe(true);
  });
});

it('observes existing pages once and records both contexts and browser shutdown', () => {
  const recorder = createLifecycleRecorder(() => {}, () => 'final-assertions', () => 1);
  const browser = new EventEmitter();
  recorder.observeBrowser(browser);
  for (const label of ['applicant', 'approver']) {
    const page = new EventEmitter();
    const context = new EventEmitter();
    context.pages = () => [page];
    recorder.observeContext(context, label);
    context.emit('page', page);
    page.emit('close');
    context.emit('close');
  }
  browser.emit('disconnected');
  expect(recorder.snapshot().events.map(e => [e.event, e.contextLabel, e.pageId])).toEqual([
    ['page-close', 'applicant', 1], ['context-close', 'applicant', null],
    ['page-close', 'approver', 2], ['context-close', 'approver', null],
    ['browser-disconnected', null, null],
  ]);
});

it('retains lifecycle facts when writing fails without throwing into the browser callback', () => {
  const recorder = createLifecycleRecorder(() => { throw Error('private path'); }, () => 'lf-conversion', () => 1);
  expect(() => recorder.recordFailure()).not.toThrow();
  recorder.startCleanup();
  expect(recorder.complete).toBe(false);
  expect(recorder.snapshot().events).toHaveLength(2);
  expect(recorder.snapshot().errors).toEqual([
    { kind: 'lifecycle-write-failed', sequence: 1 }, { kind: 'lifecycle-write-failed', sequence: 2 },
  ]);
  expect(JSON.stringify(recorder.snapshot())).not.toContain('private path');
});

it('bounds page and event capture, marks truncation, and returns independent snapshots', () => {
  const recorder = createLifecycleRecorder(() => {}, () => 'scenario', () => 1);
  const context = new EventEmitter();
  context.pages = () => [];
  recorder.observeContext(context, 'applicant');
  for (let i = 0; i < 9; i++) context.emit('page', new EventEmitter());
  for (let i = 0; i < 40; i++) recorder.recordFailure();
  const snapshot = recorder.snapshot();
  expect(snapshot.complete).toBe(false);
  expect(snapshot.events).toHaveLength(32);
  expect(snapshot.errors).toEqual([{ kind: 'capture-limit' }]);
  snapshot.events[0].phase = 'mutated';
  expect(recorder.snapshot().events[0].phase).toBe('scenario');
});

it('keeps a browser cleanup rejection visible', () => {
  const recorder = createLifecycleRecorder(() => {}, () => 'lf-conversion', () => 1);
  recorder.startCleanup();
  recorder.cleanupFailed();
  expect(recorder.complete).toBe(false);
  expect(recorder.snapshot().events.at(-1)).toMatchObject({ event: 'browser-close-failed', phase: 'lf-conversion', cleanupStarted: true });
});

it('records capture and synchronous persistence outcomes separately', async () => {
  const { captureArtifact } = evidence;
  const file = join(outputDirectory(), 'failure.png');
  expect(await captureArtifact(() => 'pixels', file)).toEqual({ status: 'captured', writeCompleted: true });
  expect(readFileSync(file, 'utf8')).toBe('pixels');
  expect(await captureArtifact(() => { throw Error('private'); })).toEqual({ status: 'unavailable', writeCompleted: false });
  expect(await captureArtifact(() => 'pixels', join(file, 'missing'))).toEqual({ status: 'write-failed', writeCompleted: false });
  expect(await captureArtifact(() => ({ visibility: 'visible' }))).toEqual({ status: 'captured', value: { visibility: 'visible' } });
});

it.each(['resolve', 'reject'])('does not write or revise a timed-out capture after late %s', async action => {
  let resolve, reject;
  const file = join(outputDirectory(), 'failure.png');
  const result = await evidence.captureArtifact(() => new Promise((yes, no) => { resolve = yes; reject = no; }), file, 5);
  const frozen = JSON.stringify(result);
  expect(result).toEqual({ status: 'timeout', writeCompleted: false });
  if (action === 'resolve') resolve('late pixels');
  else reject(Error('late unavailable'));
  await new Promise(resolve => setTimeout(resolve, 0));
  expect(existsSync(file)).toBe(false);
  expect(JSON.stringify(result)).toBe(frozen);
});

it('turns serialization failure and synchronous access exceptions into incomplete evidence', async () => {
  const cyclic = {}; cyclic.self = cyclic;
  expect(await evidence.captureArtifact(() => cyclic)).toEqual({ status: 'unavailable', writeCompleted: false });
  const page = Object.defineProperty({}, 'getByRole', { get() { throw Error('unavailable'); } });
  expect((await evidence.captureFailureArtifacts(page, outputDirectory())).complete).toBe(false);
});

it('seals lifecycle output before result serialization, ignoring later events', () => {
  const lines = [];
  const recorder = createLifecycleRecorder(line => lines.push(JSON.parse(line)), () => 'lf-conversion', () => 1);
  const context = new EventEmitter();
  const page = new EventEmitter();
  context.pages = () => [page];
  recorder.observeContext(context, 'applicant');
  recorder.recordFailure(page);
  recorder.startCleanup(page);
  page.emit('close');
  context.emit('close');
  const sealed = recorder.seal();
  const serialized = JSON.stringify(sealed);
  page.emit('crash');
  context.emit('page', new EventEmitter());
  recorder.recordFailure(page);
  recorder.cleanupFailed();
  expect(JSON.stringify(recorder.snapshot())).toBe(serialized);
  expect(lines).toEqual(sealed.events);
});

it('reports missing and unavailable pages without throwing', async () => {
  const absent = await evidence.captureFailureArtifacts(undefined, '/not-used');
  expect(absent.complete).toBe(false);
  const page = { getByRole() { throw Error('page closed'); }, screenshot() { throw Error('page closed'); }, locator() { throw Error('page closed'); } };
  const result = await evidence.captureFailureArtifacts(page, '/not-used');
  expect(result.complete).toBe(false);
  expect([result.snapshot.status, result.screenshot.status, result.body.status]).toEqual(['unavailable', 'unavailable', 'unavailable']);
});

it('collects one post-failure window and confirms actual artifact writes', async () => {
  let reads = 0;
  const output = outputDirectory();
  const page = {
    getByRole(role, options) {
      expect([role, options]).toEqual(['button', { name: 'note 申请值：转换为 LF 再编辑', exact: true }]);
      return { evaluateAll(fn) { reads++; return fn([]); } };
    },
    screenshot: async options => { expect(options).toEqual({ fullPage: true, timeout: 2000 }); return 'png'; },
    locator: name => { expect(name).toBe('body'); return { innerText: async options => { expect(options.timeout).toBe(2000); return 'body'; } }; },
  };
  const result = await evidence.captureFailureArtifacts(page, output);
  expect(result.complete).toBe(true);
  expect(reads).toBe(1);
  expect(result.snapshot.value).toMatchObject({ window: 'single-post-failure-snapshot', duringClick: 'unknown', target: { status: 'absent' } });
  expect(readFileSync(join(output, 'failure.png'), 'utf8')).toBe('png');
  expect(readFileSync(join(output, 'failure-body.txt'), 'utf8')).toBe('body');
});

it('rejects invalid output before starting captures so assembly failure cannot leave late writes', async () => {
  let started = false;
  const page = { getByRole() { started = true; throw Error('must not start'); } };
  const output = { toString() { throw Error('invalid output'); } };
  const result = await evidence.captureFailureArtifacts(page, output);
  expect(result.complete).toBe(false);
  expect(started).toBe(false);
});
