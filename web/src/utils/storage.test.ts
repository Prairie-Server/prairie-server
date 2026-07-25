import { beforeEach, describe, expect, it } from "vitest";

import {
  STORAGE_SCHEMA_VERSION,
  UPGRADE_PRESERVED_KEYS,
  ensureStorageSchema,
  isUpgradePreservedKey,
  storage,
} from "./storage";

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

describe("storage upgrade persistence", () => {
  let state: Map<string, string>;

  beforeEach(() => {
    state = installMemoryLocalStorage();
  });

  it("keeps STORAGE_KEYS names stable (auth contract)", () => {
    expect(storage.KEYS.REFRESH_TOKEN).toBe("refresh_token");
    expect(storage.KEYS.PROFILE_ID).toBe("profile_id");
    expect(storage.KEYS.PROFILE_TOKEN).toBe("profile_token");
    expect(storage.KEYS.CURRENT_PROFILE).toBe("current_profile");
    expect(storage.KEYS.DEVICE_ID).toBe("prairie-device-id");
    expect(storage.KEYS.ACCESS_TOKEN).toBe("access_token");
  });

  it("lists upgrade-preserved auth/profile keys", () => {
    expect([...UPGRADE_PRESERVED_KEYS]).toEqual([
      "refresh_token",
      "profile_id",
      "profile_token",
      "current_profile",
      "prairie-device-id",
      "impersonation_admin_session",
    ]);
    for (const key of UPGRADE_PRESERVED_KEYS) {
      expect(isUpgradePreservedKey(key)).toBe(true);
    }
    expect(isUpgradePreservedKey("prairie-theme")).toBe(false);
  });

  it("does not clear auth keys when applying a storage schema bump", () => {
    storage.set(storage.KEYS.REFRESH_TOKEN, "keep-me");
    storage.set(storage.KEYS.PROFILE_ID, "profile-7");
    storage.set(storage.KEYS.PROFILE_TOKEN, "profile-tok");
    storage.set(storage.KEYS.CURRENT_PROFILE, '{"id":"profile-7"}');
    storage.set(storage.KEYS.DEVICE_ID, "device-abc");
    storage.set(
      storage.KEYS.IMPERSONATION_ADMIN_SESSION,
      '{"accessToken":"a","refreshToken":"r","returnPath":"/admin"}',
    );
    storage.set(storage.KEYS.THEME, "dark");

    expect(ensureStorageSchema()).toBe(STORAGE_SCHEMA_VERSION);

    for (const key of UPGRADE_PRESERVED_KEYS) {
      expect(state.has(key)).toBe(true);
    }
    expect(storage.get(storage.KEYS.REFRESH_TOKEN)).toBe("keep-me");
    expect(storage.get(storage.KEYS.PROFILE_ID)).toBe("profile-7");
    expect(storage.get(storage.KEYS.PROFILE_TOKEN)).toBe("profile-tok");
    expect(storage.get(storage.KEYS.CURRENT_PROFILE)).toBe('{"id":"profile-7"}');
    expect(storage.get(storage.KEYS.DEVICE_ID)).toBe("device-abc");
    expect(storage.get(storage.KEYS.IMPERSONATION_ADMIN_SESSION)).toContain("refreshToken");
    expect(storage.get(storage.KEYS.THEME)).toBe("dark");
    expect(localStorage.getItem("prairie-storage-schema-version")).toBe(
      String(STORAGE_SCHEMA_VERSION),
    );
  });

  it("survives a second schema ensure without wiping session state", () => {
    storage.set(storage.KEYS.REFRESH_TOKEN, "still-here");
    ensureStorageSchema();
    ensureStorageSchema();
    expect(storage.get(storage.KEYS.REFRESH_TOKEN)).toBe("still-here");
  });
});

describe("storage get/set/remove", () => {
  beforeEach(() => {
    installMemoryLocalStorage();
  });

  it("round-trips values", () => {
    expect(storage.get(storage.KEYS.VOLUME)).toBeNull();
    storage.set(storage.KEYS.VOLUME, "0.5");
    expect(storage.get(storage.KEYS.VOLUME)).toBe("0.5");
    storage.remove(storage.KEYS.VOLUME);
    expect(storage.get(storage.KEYS.VOLUME)).toBeNull();
  });

  it("swallows localStorage failures", () => {
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

    expect(storage.get(storage.KEYS.THEME)).toBeNull();
    expect(() => storage.set(storage.KEYS.THEME, "dark")).not.toThrow();
    expect(() => storage.remove(storage.KEYS.THEME)).not.toThrow();
    expect(ensureStorageSchema()).toBe(STORAGE_SCHEMA_VERSION);
  });
});
