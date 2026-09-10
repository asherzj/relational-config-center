import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach } from "vitest";
import { IDBFactory } from "fake-indexeddb";
import {hydrateReleaseRequests} from "../features/release-orders/release-journal";
beforeEach(async () => {
  globalThis.indexedDB = new IDBFactory();
  if (typeof window !== "undefined") await hydrateReleaseRequests("ab09850e-ef9a-4317-a000-d67465416b5b");
});
import { cleanup } from "@testing-library/react";

// jsdom has no layout engine. Radix observes checkbox dimensions for its form input;
// real sizing and pointer behavior are covered by the browser acceptance suite.
globalThis.ResizeObserver = class implements ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
};
if (typeof window !== "undefined") {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({ matches: false, media: query, onchange: null, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {}, dispatchEvent: () => false }),
  });
}

afterEach(() => {
  cleanup();
});
