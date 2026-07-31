/**
 * Browser localStorage accessors for the Prairie web client.
 *
 * STORAGE_KEYS string values are a durable client contract — do not rename them.
 * Renaming a key silently orphans existing user data (auth session, prefs, etc.).
 *
 * Auth / profile session keys listed in UPGRADE_PRESERVED_KEYS must survive
 * client upgrades, deploys, and any storage schema migration. There is no
 * clear-on-deploy / version-bump wipe path for these keys.
 */

const STORAGE_KEYS = {
  ACCESS_TOKEN: "access_token",
  REFRESH_TOKEN: "refresh_token",
  PROFILE_ID: "profile_id",
  PROFILE_TOKEN: "profile_token",
  CURRENT_PROFILE: "current_profile",
  DEVICE_ID: "prairie-device-id",
  /** Admin session stash while impersonating — must survive schema bumps. */
  IMPERSONATION_ADMIN_SESSION: "impersonation_admin_session",
  VOLUME: "player-volume",
  MUTED: "player-muted",
  AUDIOBOOK_SKIP_BACK: "audiobook-skip-back",
  AUDIOBOOK_SKIP_FORWARD: "audiobook-skip-forward",
  AUDIOBOOK_SMART_REWIND: "audiobook-smart-rewind",
  AUDIOBOOK_RATES: "audiobook-rates",
  THEME: "prairie-theme",
  UI_TEXT_SCALE: "prairie-ui-text-scale",
  UI_TEXT_WEIGHT: "prairie-ui-text-weight",
  UI_HIGH_CONTRAST: "prairie-ui-high-contrast",
  UI_CUSTOM_THEME_VARS: "prairie-custom-theme-vars",
  UI_DATE_FORMAT: "prairie-ui-date-format",
  UI_TIME_FORMAT: "prairie-ui-time-format",
  UI_CACHE_OWNER: "prairie-ui-cache-owner",
  UI_CUSTOM_CSS: "prairie-custom-css",
  CALENDAR_PRESET: "calendar:preset",
} as const;

export type StorageKey = (typeof STORAGE_KEYS)[keyof typeof STORAGE_KEYS];

/**
 * Keys that must never be cleared by a client upgrade / storage schema bump.
 * refresh_token + profile identity keep the user signed in across deploys.
 */
export const UPGRADE_PRESERVED_KEYS = [
  STORAGE_KEYS.REFRESH_TOKEN,
  STORAGE_KEYS.PROFILE_ID,
  STORAGE_KEYS.PROFILE_TOKEN,
  STORAGE_KEYS.CURRENT_PROFILE,
  STORAGE_KEYS.DEVICE_ID,
  STORAGE_KEYS.IMPERSONATION_ADMIN_SESSION,
] as const;

export type UpgradePreservedKey = (typeof UPGRADE_PRESERVED_KEYS)[number];

/**
 * Storage schema version for future migrations.
 * Bumping this must not clear UPGRADE_PRESERVED_KEYS — migrations may only
 * rewrite or remove non-session preference keys.
 */
export const STORAGE_SCHEMA_VERSION = 1;
const STORAGE_SCHEMA_VERSION_KEY = "prairie-storage-schema-version";

/**
 * Record the current storage schema version without touching session keys.
 * Intentionally does not clear localStorage on version change.
 */
export function ensureStorageSchema(): number {
  try {
    const raw = localStorage.getItem(STORAGE_SCHEMA_VERSION_KEY);
    const current = raw == null || raw === "" ? 0 : Number(raw);
    const isValidInteger = Number.isFinite(current) && Number.isInteger(current) && current >= 0;
    if (!isValidInteger || current < STORAGE_SCHEMA_VERSION) {
      localStorage.setItem(STORAGE_SCHEMA_VERSION_KEY, String(STORAGE_SCHEMA_VERSION));
    }
    return STORAGE_SCHEMA_VERSION;
  } catch {
    return STORAGE_SCHEMA_VERSION;
  }
}

/** True when a storage key is an auth/session key that survives upgrades. */
export function isUpgradePreservedKey(key: string): key is UpgradePreservedKey {
  return (UPGRADE_PRESERVED_KEYS as readonly string[]).includes(key);
}

function getRaw(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function setRaw(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Storage full or unavailable
  }
}

function removeRaw(key: string): void {
  try {
    localStorage.removeItem(key);
  } catch {
    // Storage unavailable
  }
}

function get(key: StorageKey): string | null {
  return getRaw(key);
}

function set(key: StorageKey, value: string): void {
  setRaw(key, value);
}

function remove(key: StorageKey): void {
  try {
    localStorage.removeItem(key);
  } catch {
    // Storage unavailable
  }
}

export const storage = { KEYS: STORAGE_KEYS, get, set, remove };

/**
 * Namespace used before anyone has ever signed in on this browser. Values
 * written here are the device's own defaults, not any account's.
 */
const DEVICE_NAMESPACE = "device";

/**
 * Which namespace a read or write belongs to.
 *
 * A known account always uses its own. A `null` owner means auth is still
 * bootstrapping or nobody is signed in, and we fall back to the last account
 * that wrote here so the login screen and the pre-auth first paint keep the
 * look this device last used. That fallback cannot leak into a signed-in
 * session: the moment auth resolves, the owner is exact.
 */
function namespaceFor(owner: string | null): string {
  if (owner !== null) return owner;
  return getRaw(STORAGE_KEYS.UI_CACHE_OWNER) ?? DEVICE_NAMESPACE;
}

/**
 * Device-local mirrors of server-side, per-account settings, so the UI can
 * paint before the settings request resolves. Covers theme, text scale, text
 * weight, high contrast, custom theme tokens, custom CSS, and date/time format.
 *
 * Values are namespaced by the identity that owns them — user id plus active
 * profile id (`prairie-theme:7:p1`) — so a second account or a sibling profile
 * signing in on a shared browser simply finds nothing where the first one's
 * values would have been. A miss is just a miss: every caller
 * already parses a missing value into the correct default, and the settings
 * response repopulates the namespace when it lands.
 *
 * Namespacing rather than tagging-and-clearing matters for three reasons.
 * Nothing is ever deleted, so returning to the first account still paints their
 * look with no cold start. There is no shared stamp for a second tab, a stale
 * debounce timer, or an out-of-order effect to race on. And widening ownership
 * — appearance moved from user scope to profile scope with the settings
 * contract, and the owner token widened with it — is a change to
 * `appearanceCacheOwner` alone, which no caller can forget to apply.
 *
 * Values written before namespacing existed sit at the bare key and are simply
 * ignored. Those users take one cold paint, after which the mirror below has
 * repopulated their namespace from the server, which holds all of these
 * settings anyway.
 */
export const appearanceCache = {
  /** The cached value for `owner`, or null when they have none. */
  get(key: StorageKey, owner: string | null): string | null {
    return getRaw(`${key}:${namespaceFor(owner)}`);
  },
  /** Write a value into `owner`'s namespace. */
  set(key: StorageKey, value: string, owner: string | null): void {
    setRaw(`${key}:${namespaceFor(owner)}`, value);
    if (owner !== null) setRaw(STORAGE_KEYS.UI_CACHE_OWNER, owner);
  },
  /**
   * Drop `owner`'s cached value, so the next cold start falls back rather than
   * painting a preference the server no longer holds. Needed because the
   * server's answer is authoritative once loaded: when another client deletes
   * an appearance setting, the effective response simply omits it, and a cache
   * that only ever grows would keep the removed value alive on this browser
   * forever. Deliberately scoped to one owner — clearing another identity's
   * namespace is what the ownership tests exist to prevent.
   */
  remove(key: StorageKey, owner: string | null): void {
    removeRaw(`${key}:${namespaceFor(owner)}`);
  },
};
