import "@testing-library/jest-dom/vitest";

// This jsdom build does not provide window.localStorage; the app reads it
// (authHeaders → getToken). Install a minimal in-memory store so components
// exercise the real auth-header path under test, as they would in a browser.
if (typeof window !== "undefined" && !window.localStorage) {
  const store = new Map<string, string>();
  const localStorageMock: Storage = {
    getItem: (k) => (store.has(k) ? store.get(k)! : null),
    setItem: (k, v) => void store.set(k, String(v)),
    removeItem: (k) => void store.delete(k),
    clear: () => store.clear(),
    key: (i) => Array.from(store.keys())[i] ?? null,
    get length() {
      return store.size;
    },
  };
  Object.defineProperty(window, "localStorage", { value: localStorageMock, configurable: true });
}
