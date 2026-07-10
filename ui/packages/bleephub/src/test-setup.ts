import "@testing-library/jest-dom/vitest";

// Install storage without probing window.localStorage first; Node emits an
// experimental warning when that getter is touched without --localstorage-file.
if (typeof window !== "undefined") {
  const store = new Map<string, string>();
  const testLocalStorage: Storage = {
    getItem: (k) => (store.has(k) ? store.get(k)! : null),
    setItem: (k, v) => void store.set(k, String(v)),
    removeItem: (k) => void store.delete(k),
    clear: () => store.clear(),
    key: (i) => Array.from(store.keys())[i] ?? null,
    get length() {
      return store.size;
    },
  };
  Object.defineProperty(window, "localStorage", { value: testLocalStorage, configurable: true });
}
