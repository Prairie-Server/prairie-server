import { describe, expect, it } from "vitest";
import {
  applyAdminLogAppend,
  applyAdminLogAppends,
  buildAdminLogStreamQuery,
  buildAdminLogStreamUrl,
} from "./useAdminLogStream";

describe("buildAdminLogStreamQuery", () => {
  it("serializes only defined log filters", () => {
    expect(
      buildAdminLogStreamQuery({
        request_id: "req-1",
        component: "api",
        playback_session_id: "playback-123",
        q: "",
        limit: 50,
      }),
    ).toBe(
      "request_id=req-1&component=api&playback_session_id=playback-123&limit=50",
    );
  });
});

describe("buildAdminLogStreamUrl", () => {
  it("includes stream, filters, and auth token", () => {
    expect(
      buildAdminLogStreamUrl(
        "app",
        { request_id: "req-1", component: "api" },
        "token-123",
        {
          protocol: "https:",
          host: "example.com",
        },
      ),
    ).toBe(
      "wss://example.com/api/v1/admin/logs/ws?stream=app&request_id=req-1&component=api&token=token-123",
    );

    expect(
      buildAdminLogStreamUrl(
        "audit",
        { playback_session_id: "playback-123", request_id: "req-9" },
        "token-123",
        {
          protocol: "https:",
          host: "example.com",
        },
      ),
    ).toBe(
      "wss://example.com/api/v1/admin/logs/ws?stream=audit&playback_session_id=playback-123&request_id=req-9&token=token-123",
    );
  });
});

describe("applyAdminLogAppend", () => {
  it("prepends new rows, dedupes by id, and enforces the limit", () => {
    expect(
      applyAdminLogAppend(
        [
          { id: 2, message: "older" },
          { id: 1, message: "oldest" },
        ],
        { id: 3, message: "new" },
        2,
      ),
    ).toEqual([
      { id: 3, message: "new" },
      { id: 2, message: "older" },
    ]);

    expect(
      applyAdminLogAppend(
        [
          { id: 2, message: "older" },
          { id: 1, message: "oldest" },
        ],
        { id: 2, message: "updated" },
        5,
      ),
    ).toEqual([
      { id: 2, message: "updated" },
      { id: 1, message: "oldest" },
    ]);
  });
});

describe("applyAdminLogAppends", () => {
  it("batches appends while keeping newest entries first", () => {
    expect(
      applyAdminLogAppends(
        [
          { id: 3, message: "current newest" },
          { id: 1, message: "oldest" },
        ],
        [
          { id: 4, message: "next" },
          { id: 3, message: "updated current newest" },
          { id: 5, message: "latest" },
        ],
        4,
      ),
    ).toEqual([
      { id: 5, message: "latest" },
      { id: 3, message: "updated current newest" },
      { id: 4, message: "next" },
      { id: 1, message: "oldest" },
    ]);
  });
});
