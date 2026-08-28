import "@testing-library/jest-dom";

// Radix UI and layout hooks expect ResizeObserver in jsdom.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = ResizeObserverStub as typeof ResizeObserver;
}
