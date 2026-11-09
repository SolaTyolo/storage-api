import { afterEach, describe, expect, it, vi } from "vitest";
import { ObjGatewaySDK } from "./client.js";
import { choosePartSize } from "./upload.js";

const MIB = 1024 * 1024;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

interface FakeState {
  parts: Map<number, Uint8Array>;
  failOnce: Set<number>;
  completed?: Array<{ partNumber: number; etag: string }>;
  aborted: boolean;
  createSize?: number;
  calls: string[];
}

function fakeBackend(state: FakeState) {
  const gatewayFetch = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
    const path = String(url).replace("http://gw/api/v1", "");
    state.calls.push(path);
    const body = init?.body ? JSON.parse(String(init.body)) : {};
    switch (path) {
      case "/multipart/create":
        state.createSize = body.size;
        return json({ uploadId: "uid-1" });
      case "/multipart/presign-parts":
        return json({
          parts: body.partNumbers.map((n: number) => ({ partNumber: n, url: `http://s3/part/${n}` })),
        });
      case "/multipart/complete":
        state.completed = body.parts;
        return json({ etag: "final" });
      case "/multipart/abort":
        state.aborted = true;
        return json({ status: "ok" });
      case "/object/head":
        return json({ size: 0, contentType: "", meta: {}, etag: "final", cdnUrl: "https://cdn/x" });
      default:
        return json({ error: "not_found", message: path }, 404);
    }
  });
  const storageFetch = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
    const n = Number(String(url).split("/").pop());
    if (state.failOnce.has(n)) {
      state.failOnce.delete(n);
      return new Response("busy", { status: 503 });
    }
    const buf = new Uint8Array(await (init!.body as Blob).arrayBuffer());
    state.parts.set(n, buf);
    return new Response(null, { status: 200, headers: { ETag: `"etag-${n}"` } });
  });
  return { gatewayFetch, storageFetch };
}

function newState(): FakeState {
  return { parts: new Map(), failOnce: new Set(), aborted: false, calls: [] };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("multipartUpload", () => {
  it("uploads parts concurrently, retries transient failures and completes in order", async () => {
    const state = newState();
    state.failOnce.add(2);
    const { gatewayFetch, storageFetch } = fakeBackend(state);
    vi.stubGlobal("fetch", storageFetch);

    const data = new Uint8Array(2 * 5 * MIB + 123).map((_, i) => i % 251);
    const sdk = new ObjGatewaySDK(
      { gatewayEndpoint: "http://gw/api/v1", token: "tok", defaultBucket: "uploads" },
      gatewayFetch as unknown as typeof fetch,
    );
    const progress: number[] = [];
    const res = await sdk.multipartUpload(data, {
      key: "big.bin",
      concurrency: 3,
      onProgress: (ev) => progress.push(ev.loaded),
    });

    expect(res.etag).toBe("final");
    expect(state.createSize).toBe(data.length);
    expect(state.completed).toEqual([
      { partNumber: 1, etag: "etag-1" },
      { partNumber: 2, etag: "etag-2" },
      { partNumber: 3, etag: "etag-3" },
    ]);
    const chunks = [1, 2, 3].map((n) => state.parts.get(n)!);
    const assembled = new Uint8Array(chunks.reduce((sum, c) => sum + c.length, 0));
    chunks.reduce((offset, c) => (assembled.set(c, offset), offset + c.length), 0);
    expect(assembled.length).toBe(data.length);
    expect(assembled.every((b, i) => b === data[i])).toBe(true);
    expect(progress.at(-1)).toBe(data.length);
    expect(state.calls.filter((c) => c === "/multipart/presign-parts")).toHaveLength(1);
  });

  it("aborts the multipart upload when a part keeps failing", async () => {
    const state = newState();
    const { gatewayFetch } = fakeBackend(state);
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("denied", { status: 403 })),
    );
    const sdk = new ObjGatewaySDK(
      { gatewayEndpoint: "http://gw/api/v1", token: "tok", defaultBucket: "uploads" },
      gatewayFetch as unknown as typeof fetch,
    );
    await expect(sdk.multipartUpload(new Uint8Array(10), { key: "x" })).rejects.toThrow(/403/);
    expect(state.aborted).toBe(true);
    expect(state.completed).toBeUndefined();
  });
});

describe("choosePartSize", () => {
  it("keeps part count within S3 limits", () => {
    expect(choosePartSize(1)).toBe(5 * MIB);
    expect(choosePartSize(100 * MIB, 8 * MIB)).toBe(8 * MIB);
    expect(choosePartSize(100 * 1024 * MIB)).toBe(11 * MIB);
    expect(() => choosePartSize(1, 6 * 1024 * MIB)).toThrow();
  });
});

describe("object APIs", () => {
  it("lists all pages, deletes and copies", async () => {
    const calls: Array<{ path: string; body: Record<string, unknown> }> = [];
    const fetchImpl = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const path = String(url).replace("http://gw/api/v1", "");
      const body = JSON.parse(String(init!.body));
      calls.push({ path, body });
      if (path === "/object/list") {
        return body.continuationToken
          ? json({ entries: [{ key: "a/2", isDir: false, size: 2 }], isTruncated: false, nextContinuationToken: "" })
          : json({
              entries: [
                { key: "a/1", isDir: false, size: 1 },
                { key: "a/sub/", isDir: true },
              ],
              isTruncated: true,
              nextContinuationToken: "t1",
            });
      }
      if (path === "/object/copy") return json({ status: "ok", cdnUrl: "https://cdn/b" });
      return json({ status: "ok" });
    });
    const sdk = new ObjGatewaySDK(
      { gatewayEndpoint: "http://gw/api/v1", token: "tok", defaultBucket: "uploads" },
      fetchImpl as unknown as typeof fetch,
    );

    const all = await sdk.listAll("a/");
    expect(all.map((e) => e.key)).toEqual(["a/1", "a/2"]);

    await sdk.delete("a/1");
    expect(calls.at(-1)).toEqual({ path: "/object/delete", body: { bucket: "uploads", key: "a/1" } });

    const copied = await sdk.copy({ srcKey: "a/2", destKey: "b" });
    expect(copied.cdnUrl).toBe("https://cdn/b");
    expect(calls.at(-1)?.body).toEqual({ bucket: "uploads", srcKey: "a/2", destKey: "b" });
  });
});
