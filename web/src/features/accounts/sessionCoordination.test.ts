import { afterEach, expect, it, vi } from "vitest";
import { publishSessionEvent, subscribeSessionEvents } from "./sessionCoordination";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

it("uses BroadcastChannel when local storage cannot publish a cross-tab session event", () => {
  class FakeBroadcastChannel {
    static instances: FakeBroadcastChannel[] = [];
    private listeners = new Set<(event: MessageEvent) => void>();

    constructor(readonly name: string) { FakeBroadcastChannel.instances.push(this); }
    addEventListener(_type: "message", listener: (event: MessageEvent) => void) { this.listeners.add(listener); }
    removeEventListener(_type: "message", listener: (event: MessageEvent) => void) { this.listeners.delete(listener); }
    postMessage(data: unknown) {
      for (const channel of FakeBroadcastChannel.instances) {
        if (channel !== this && channel.name === this.name) {
          for (const listener of channel.listeners) listener(new MessageEvent("message", { data }));
        }
      }
    }
    close() { FakeBroadcastChannel.instances = FakeBroadcastChannel.instances.filter((channel) => channel !== this); }
  }
  vi.stubGlobal("BroadcastChannel", FakeBroadcastChannel);
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("storage denied"); });
  const received = vi.fn();
  const unsubscribe = subscribeSessionEvents(received);

  publishSessionEvent("ended");

  expect(received).toHaveBeenCalledWith("ended");
  unsubscribe();
});
