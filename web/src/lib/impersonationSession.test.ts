import { beforeEach, describe, expect, it } from "vitest";
import { getAccessToken, restoreUserSession, setAccessToken } from "../api/client";
import {
  clearStoredImpersonationAdminSession,
  loadStoredImpersonationAdminSession,
  saveStoredImpersonationAdminSession,
} from "./impersonationSession";

describe("impersonationSession", () => {
  beforeEach(() => {
    const sessionStorageState = new Map<string, string>();
    const localStorageState = new Map<string, string>();
    Object.defineProperty(globalThis, "sessionStorage", {
      value: {
        get length() {
          return sessionStorageState.size;
        },
        getItem: (key: string) => sessionStorageState.get(key) ?? null,
        key: (index: number) => Array.from(sessionStorageState.keys())[index] ?? null,
        setItem: (key: string, value: string) => {
          sessionStorageState.set(key, value);
        },
        removeItem: (key: string) => {
          sessionStorageState.delete(key);
        },
        clear: () => {
          sessionStorageState.clear();
        },
      } satisfies Storage,
      configurable: true,
    });

    Object.defineProperty(globalThis, "localStorage", {
      value: {
        get length() {
          return localStorageState.size;
        },
        getItem: (key: string) => localStorageState.get(key) ?? null,
        key: (index: number) => Array.from(localStorageState.keys())[index] ?? null,
        setItem: (key: string, value: string) => {
          localStorageState.set(key, value);
        },
        removeItem: (key: string) => {
          localStorageState.delete(key);
        },
        clear: () => {
          localStorageState.clear();
        },
      } satisfies Storage,
      configurable: true,
    });

    sessionStorage.clear();
    localStorage.clear();
  });

  it("round-trips preserved admin tokens and return path", () => {
    saveStoredImpersonationAdminSession({
      accessToken: "admin-access",
      refreshToken: "admin-refresh",
      returnPath: "/admin/users/42",
    });

    expect(loadStoredImpersonationAdminSession()).toEqual({
      accessToken: "admin-access",
      refreshToken: "admin-refresh",
      returnPath: "/admin/users/42",
    });
  });

  it("returns null when nothing is stored", () => {
    expect(loadStoredImpersonationAdminSession()).toBeNull();
  });

  it("returns null for malformed or incomplete stored JSON", () => {
    localStorage.setItem("impersonation_admin_session", "{not-json");
    expect(loadStoredImpersonationAdminSession()).toBeNull();

    localStorage.setItem(
      "impersonation_admin_session",
      JSON.stringify({ accessToken: "a", refreshToken: 1, returnPath: "/x" }),
    );
    expect(loadStoredImpersonationAdminSession()).toBeNull();

    localStorage.setItem(
      "impersonation_admin_session",
      JSON.stringify({ accessToken: "a", refreshToken: "b" }),
    );
    expect(loadStoredImpersonationAdminSession()).toBeNull();
  });

  it("swallows localStorage failures without throwing", () => {
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

    expect(() =>
      saveStoredImpersonationAdminSession({
        accessToken: "a",
        refreshToken: "b",
        returnPath: "/",
      }),
    ).not.toThrow();
    expect(loadStoredImpersonationAdminSession()).toBeNull();
    expect(() => clearStoredImpersonationAdminSession()).not.toThrow();
  });

  it("stores and clears the preserved admin session in localStorage to match auth scope", () => {
    saveStoredImpersonationAdminSession({
      accessToken: "admin-access",
      refreshToken: "admin-refresh",
      returnPath: "/admin/users/42",
    });

    expect(localStorage.getItem("impersonation_admin_session")).toBe(
      JSON.stringify({
        accessToken: "admin-access",
        refreshToken: "admin-refresh",
        returnPath: "/admin/users/42",
      }),
    );
    expect(sessionStorage.getItem("impersonation_admin_session")).toBeNull();

    clearStoredImpersonationAdminSession();

    expect(localStorage.getItem("impersonation_admin_session")).toBeNull();
  });

  it("validates a stored admin session without mutating global auth state", async () => {
    setAccessToken("impersonated-access");

    const fetchCalls: Array<{ url: string; authorization: string | null }> = [];
    const fetchMock: typeof fetch = async (input, init) => {
      const url = String(input);
      const headers = new Headers(init?.headers);
      fetchCalls.push({
        url,
        authorization: headers.get("Authorization"),
      });

      if (
        url.endsWith("/auth/me") &&
        headers.get("Authorization") === "Bearer expired-admin-access"
      ) {
        return new Response(JSON.stringify({ error: "unauthorized", message: "expired" }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        });
      }

      if (url.endsWith("/auth/refresh")) {
        return new Response(
          JSON.stringify({
            access_token: "fresh-admin-access",
            refresh_token: "fresh-admin-refresh",
            expires_in: 3600,
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }

      if (
        url.endsWith("/auth/me") &&
        headers.get("Authorization") === "Bearer fresh-admin-access"
      ) {
        return new Response(
          JSON.stringify({
            id: 1,
            username: "admin",
            email: "admin@example.com",
            role: "admin",
            impersonation: null,
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }

      throw new Error(`Unhandled fetch call: ${url}`);
    };

    const restoredSession = await restoreUserSession({
      accessToken: "expired-admin-access",
      refreshToken: "admin-refresh",
      fetchImpl: fetchMock,
    });

    expect(restoredSession).toEqual({
      accessToken: "fresh-admin-access",
      refreshToken: "fresh-admin-refresh",
      user: {
        id: 1,
        username: "admin",
        email: "admin@example.com",
        role: "admin",
        impersonation: null,
      },
    });
    expect(getAccessToken()).toBe("impersonated-access");
    expect(fetchCalls).toEqual([
      {
        url: "/api/v1/auth/me",
        authorization: "Bearer expired-admin-access",
      },
      {
        url: "/api/v1/auth/refresh",
        authorization: null,
      },
      {
        url: "/api/v1/auth/me",
        authorization: "Bearer fresh-admin-access",
      },
    ]);
  });
});
