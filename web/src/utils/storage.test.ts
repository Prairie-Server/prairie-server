import { beforeEach, describe, expect, it } from "vitest";
import { STORAGE_SCHEMA_VERSION, appearanceCache, ensureStorageSchema, storage } from "./storage";

const KEYS = storage.KEYS;

function installMemoryLocalStorage() {
  const state = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", {
    value: {
      get length() {
        return state.size;
      },
      getItem: (key: string) => state.get(key) ?? null,
      key: (index: number) => Array.from(state.keys())[index] ?? null,
      setItem: (key: string, value: string) => {
        state.set(key, value);
      },
      removeItem: (key: string) => {
        state.delete(key);
      },
      clear: () => {
        state.clear();
      },
    } satisfies Storage,
    configurable: true,
  });
  return state;
}

function installBlockedLocalStorage() {
  Object.defineProperty(globalThis, "localStorage", {
    value: {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
      removeItem: () => {
        throw new Error("blocked");
      },
      clear: () => {},
      key: () => null,
      length: 0,
    } satisfies Storage,
    configurable: true,
  });
}

describe("appearance cache namespacing", () => {
  beforeEach(() => {
    installMemoryLocalStorage();
  });

  it("reads back what the same account wrote", () => {
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "large", "1");

    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "1")).toBe("large");
  });

  it("hides another account's cached values", () => {
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "large", "1");
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "large", "1");

    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "2")).toBeNull();
    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "2")).toBeNull();
  });

  it("keeps both accounts' values, so returning to the first still warm starts", () => {
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "large", "1");
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "x-large", "2");

    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "1")).toBe("large");
    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "2")).toBe("x-large");
  });

  it("ignores values written before namespacing existed", () => {
    storage.set(KEYS.UI_TEXT_SCALE, "large");

    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "1")).toBeNull();
  });

  it("falls back to the last account that wrote while nobody is signed in", () => {
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "large", "1");

    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, null)).toBe("large");
  });

  it("follows the pointer to the most recent account, not the first", () => {
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "large", "1");
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "x-large", "2");

    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, null)).toBe("x-large");
  });

  it("keeps a never-signed-in device's values in their own namespace", () => {
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "large", null);

    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, null)).toBe("large");
    // Not the bare key, and not visible to a real account.
    expect(storage.get(KEYS.UI_TEXT_SCALE)).toBeNull();
    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "1")).toBeNull();
  });

  it("routes a signed-out write into the last account's namespace without moving the pointer", () => {
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "large", "1");
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "default", null);

    // Reads and writes resolve their namespace the same way, so a change
    // made on the login screen is the one the next read sees. It lands in the
    // last account's cache, which is only a warm start: their server value
    // still wins once the settings request resolves.
    expect(storage.get(KEYS.UI_CACHE_OWNER)).toBe("1");
    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, null)).toBe("default");
    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "1")).toBe("default");
    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "2")).toBeNull();
  });

  it("keeps every key in the group independently namespaced", () => {
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "large", "1");
    appearanceCache.set(KEYS.UI_HIGH_CONTRAST, "true", "1");
    appearanceCache.set(KEYS.UI_DATE_FORMAT, "iso", "2");

    expect(appearanceCache.get(KEYS.UI_HIGH_CONTRAST, "1")).toBe("true");
    expect(appearanceCache.get(KEYS.UI_HIGH_CONTRAST, "2")).toBeNull();
    expect(appearanceCache.get(KEYS.UI_DATE_FORMAT, "2")).toBe("iso");
    expect(appearanceCache.get(KEYS.UI_DATE_FORMAT, "1")).toBeNull();
  });

  it("drops only the requested owner's cached value", () => {
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "cobalt-studio", "1");
    appearanceCache.set(KEYS.UI_TEXT_SCALE, "oxblood-noir", "2");

    appearanceCache.remove(KEYS.UI_TEXT_SCALE, "1");

    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "1")).toBeNull();
    expect(appearanceCache.get(KEYS.UI_TEXT_SCALE, "2")).toBe("oxblood-noir");
  });
});

describe("ensureStorageSchema", () => {
  beforeEach(() => {
    installMemoryLocalStorage();
  });

  it("writes the schema version when missing", () => {
    expect(ensureStorageSchema()).toBe(STORAGE_SCHEMA_VERSION);
    expect(localStorage.getItem("prairie-storage-schema-version")).toBe(
      String(STORAGE_SCHEMA_VERSION),
    );
  });

  it("survives repeated calls without rewriting session keys", () => {
    storage.set(KEYS.REFRESH_TOKEN, "keep-me");
    ensureStorageSchema();
    ensureStorageSchema();
    expect(storage.get(KEYS.REFRESH_TOKEN)).toBe("keep-me");
  });

  it("rewrites malformed schema-version markers", () => {
    for (const bad of ["1junk", "1.5", "NaN", "-1"]) {
      localStorage.setItem("prairie-storage-schema-version", bad);
      expect(ensureStorageSchema()).toBe(STORAGE_SCHEMA_VERSION);
      expect(localStorage.getItem("prairie-storage-schema-version")).toBe(
        String(STORAGE_SCHEMA_VERSION),
      );
    }
  });

  it("returns the schema version when localStorage is unavailable", () => {
    installBlockedLocalStorage();

    expect(ensureStorageSchema()).toBe(STORAGE_SCHEMA_VERSION);
  });
});

describe("storage get/set/remove", () => {
  beforeEach(() => {
    installMemoryLocalStorage();
  });

  it("round-trips values", () => {
    expect(storage.get(KEYS.VOLUME)).toBeNull();
    storage.set(KEYS.VOLUME, "0.5");
    expect(storage.get(KEYS.VOLUME)).toBe("0.5");
    storage.remove(KEYS.VOLUME);
    expect(storage.get(KEYS.VOLUME)).toBeNull();
  });

  it("swallows localStorage failures", () => {
    installBlockedLocalStorage();

    expect(storage.get(KEYS.UI_TEXT_SCALE)).toBeNull();
    expect(() => storage.set(KEYS.UI_TEXT_SCALE, "dark")).not.toThrow();
    expect(() => storage.remove(KEYS.UI_TEXT_SCALE)).not.toThrow();
  });
});
