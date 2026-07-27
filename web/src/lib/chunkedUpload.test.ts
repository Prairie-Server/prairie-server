import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientError, api } from "@/api/client";
import { uploadFileInChunks, type ChunkedUploadSession } from "./chunkedUpload";

vi.mock("@/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/client")>()),
  api: vi.fn(),
}));

const apiMock = vi.mocked(api);

describe("uploadFileInChunks", () => {
  beforeEach(() => {
    apiMock.mockReset();
  });

  it("creates an upload session, sends file slices, then completes the upload", async () => {
    const uploadedChunks: string[] = [];

    apiMock.mockImplementation(async (path: string, options?: RequestInit) => {
      if (path === "/uploads") {
        return session({ received_chunks: 0, received_bytes: 0 });
      }
      if (path.startsWith("/uploads/session-1/chunks/")) {
        const body = options?.body;
        if (!(body instanceof Blob)) {
          throw new Error("chunk body must be a Blob");
        }
        uploadedChunks.push(await body.text());
        return session({
          received_chunks: uploadedChunks.length,
          received_bytes: uploadedChunks.join("").length,
        });
      }
      if (path === "/uploads/session-1/complete") {
        return { installed: true };
      }
      throw new Error(`unexpected path ${path}`);
    });

    const progress: number[] = [];
    const result = await uploadFileInChunks<{ installed: boolean }>({
      file: new File(["abcdefghij"], "plugin.bin"),
      createPath: "/uploads",
      chunkPath: (uploadId, index) => `/uploads/${uploadId}/chunks/${index}`,
      completePath: (uploadId) => `/uploads/${uploadId}/complete`,
      chunkSize: 4,
      onProgress: (next) => progress.push(next.percent),
    });

    expect(result).toEqual({ installed: true });
    expect(uploadedChunks).toEqual(["abcd", "efgh", "ij"]);
    expect(progress).toEqual([0, 40, 80, 100]);
    expect(apiMock).toHaveBeenCalledWith("/uploads", {
      method: "POST",
      body: JSON.stringify({
        filename: "plugin.bin",
        size_bytes: 10,
        chunk_size: 4,
      }),
    });
  });

  it("retries with a smaller chunk size when the proxy rejects the first chunk", async () => {
    const file = new File([new Uint8Array(300 * 1024)], "plugin.bin");
    const createChunkSizes: number[] = [];

    apiMock.mockImplementation(async (path: string, options?: RequestInit) => {
      if (path === "/uploads") {
        const body = JSON.parse(String(options?.body)) as { chunk_size: number };
        createChunkSizes.push(body.chunk_size);
        const uploadID = `session-${createChunkSizes.length}`;
        return sessionForFile(file, uploadID, body.chunk_size, 0);
      }

      if (path === "/uploads/session-1/chunks/0") {
        throw new ApiClientError(413, "unknown", "Request Entity Too Large");
      }

      if (path === "/uploads/session-1") {
        return undefined;
      }

      if (path.startsWith("/uploads/session-2/chunks/")) {
        const parts = path.split("/");
        const chunkIndex = Number(parts[parts.length - 1]);
        const chunkSize = createChunkSizes[createChunkSizes.length - 1] ?? 128 * 1024;
        return sessionForFile(file, "session-2", chunkSize, chunkIndex + 1);
      }

      if (path === "/uploads/session-2/complete") {
        return { installed: true };
      }

      throw new Error(`unexpected path ${path}`);
    });

    const result = await uploadFileInChunks<{ installed: boolean }>({
      file,
      createPath: "/uploads",
      chunkPath: (uploadId, index) => `/uploads/${uploadId}/chunks/${index}`,
      completePath: (uploadId) => `/uploads/${uploadId}/complete`,
      cancelPath: (uploadId) => `/uploads/${uploadId}`,
      chunkSize: 256 * 1024,
    });

    expect(result).toEqual({ installed: true });
    expect(createChunkSizes).toEqual([256 * 1024, 128 * 1024]);
    expect(apiMock).toHaveBeenCalledWith("/uploads/session-1", { method: "DELETE" });
  });

  it("does not retry request-too-large errors once the minimum chunk size is reached", async () => {
    const tooLarge = new ApiClientError(413, "unknown", "Request Entity Too Large");

    apiMock.mockImplementation(async (path: string) => {
      if (path === "/uploads") {
        return session({ chunk_size: 128 * 1024, total_chunks: 1 });
      }
      if (path === "/uploads/session-1/chunks/0") {
        throw tooLarge;
      }
      if (path === "/uploads/session-1") {
        return undefined;
      }
      throw new Error(`unexpected path ${path}`);
    });

    await expect(
      uploadFileInChunks({
        file: new File([new Uint8Array(129 * 1024)], "plugin.bin"),
        createPath: "/uploads",
        chunkPath: (uploadId, index) => `/uploads/${uploadId}/chunks/${index}`,
        completePath: (uploadId) => `/uploads/${uploadId}/complete`,
        cancelPath: (uploadId) => `/uploads/${uploadId}`,
        chunkSize: 128 * 1024,
      }),
    ).rejects.toBe(tooLarge);

    expect(apiMock).toHaveBeenCalledTimes(3);
  });

  it("preserves the upload failure when best-effort cancellation also fails", async () => {
    const uploadError = new Error("chunk failed");

    apiMock.mockImplementation(async (path: string) => {
      if (path === "/uploads") {
        return session({ received_chunks: 0, received_bytes: 0 });
      }
      if (path === "/uploads/session-1/chunks/0") {
        throw uploadError;
      }
      if (path === "/uploads/session-1/cancel") {
        throw new Error("cleanup failed");
      }
      throw new Error(`unexpected path ${path}`);
    });

    await expect(
      uploadFileInChunks({
        file: new File(["abcdefghij"], "plugin.bin"),
        createPath: "/uploads",
        chunkPath: (uploadId, index) => `/uploads/${uploadId}/chunks/${index}`,
        completePath: (uploadId) => `/uploads/${uploadId}/complete`,
        cancelPath: (uploadId) => `/uploads/${uploadId}/cancel`,
        chunkSize: 4,
      }),
    ).rejects.toBe(uploadError);

    expect(apiMock).toHaveBeenCalledWith("/uploads/session-1/cancel", { method: "DELETE" });
  });

  it("uses the file size fallback and clamps reported progress", async () => {
    apiMock.mockImplementation(async (path: string) => {
      if (path === "/uploads") {
        return session({
          size_bytes: 0,
          total_chunks: 0,
          received_chunks: 0,
          received_bytes: 30,
        });
      }
      if (path === "/uploads/session-1/complete") {
        return { installed: true };
      }
      throw new Error(`unexpected path ${path}`);
    });

    const progress: number[] = [];
    await uploadFileInChunks({
      file: new File(["abcdefghij"], "plugin.bin"),
      createPath: "/uploads",
      chunkPath: (uploadId, index) => `/uploads/${uploadId}/chunks/${index}`,
      completePath: (uploadId) => `/uploads/${uploadId}/complete`,
      onProgress: (next) => progress.push(next.percent),
    });

    expect(progress).toEqual([100]);
  });

  it("best-effort cancels the session when a chunk upload fails", async () => {
    const cancel = vi.fn(async () => undefined);
    apiMock.mockImplementation(async (path: string) => {
      if (path === "/uploads") {
        return session({ upload_id: "session-cancel", chunk_size: 4, total_chunks: 3 });
      }
      if (path === "/uploads/session-cancel/chunks/0") {
        throw new Error("chunk failed");
      }
      if (path === "/uploads/session-cancel") {
        return cancel();
      }
      throw new Error(`unexpected path ${path}`);
    });

    await expect(
      uploadFileInChunks({
        file: new File(["abcdefghij"], "plugin.bin"),
        createPath: "/uploads",
        chunkPath: (uploadId, index) => `/uploads/${uploadId}/chunks/${index}`,
        completePath: (uploadId) => `/uploads/${uploadId}/complete`,
        cancelPath: (uploadId) => `/uploads/${uploadId}`,
      }),
    ).rejects.toThrow("chunk failed");
    expect(cancel).toHaveBeenCalledOnce();
  });

  it("reports 0% progress when session size is unknown", async () => {
    apiMock.mockImplementation(async (path: string) => {
      if (path === "/uploads") {
        return session({ size_bytes: 0, chunk_size: 10, total_chunks: 1 });
      }
      if (path === "/uploads/session-1/chunks/0") {
        return session({
          size_bytes: 0,
          chunk_size: 10,
          total_chunks: 1,
          received_chunks: 1,
          received_bytes: 0,
        });
      }
      if (path === "/uploads/session-1/complete") {
        return { installed: true };
      }
      throw new Error(`unexpected path ${path}`);
    });

    const progress: number[] = [];
    await uploadFileInChunks({
      file: new File([""], "empty.bin"),
      createPath: "/uploads",
      chunkPath: (uploadId, index) => `/uploads/${uploadId}/chunks/${index}`,
      completePath: (uploadId) => `/uploads/${uploadId}/complete`,
      onProgress: (next) => progress.push(next.percent),
    });
    expect(progress.every((p) => p === 0)).toBe(true);
    expect(progress.length).toBeGreaterThan(0);
  });
});

function session(overrides: Partial<ChunkedUploadSession>): ChunkedUploadSession {
  return {
    upload_id: "session-1",
    filename: "plugin.bin",
    size_bytes: 10,
    chunk_size: 4,
    total_chunks: 3,
    received_chunks: 0,
    received_bytes: 0,
    complete: false,
    expires_at: "2026-06-05T12:00:00Z",
    ...overrides,
  };
}

function sessionForFile(
  file: File,
  uploadId: string,
  chunkSize: number,
  receivedChunks: number,
): ChunkedUploadSession {
  return session({
    upload_id: uploadId,
    size_bytes: file.size,
    chunk_size: chunkSize,
    total_chunks: Math.ceil(file.size / chunkSize),
    received_chunks: receivedChunks,
    received_bytes: Math.min(file.size, receivedChunks * chunkSize),
  });
}
