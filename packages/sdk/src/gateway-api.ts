import type {
  ListObjectsResponse,
  Meta,
  MultipartCompleteResponse,
  MultipartCreateResponse,
  MultipartPresignPartResponse,
  MultipartPresignPartsResponse,
  ObjectHeadResponse,
  PresignGetResponse,
  PresignPutResponse,
} from "./types.js";

export class GatewayApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly body?: unknown,
  ) {
    super(message);
    this.name = "GatewayApiError";
  }

  /** Machine-readable error code from the gateway (e.g. "not_found", "forbidden"). */
  get code(): string | undefined {
    const b = this.body;
    return typeof b === "object" && b && "error" in b ? String((b as { error: unknown }).error) : undefined;
  }
}

export class GatewayApi {
  constructor(
    private readonly endpoint: string,
    private readonly token: string,
    // Bind to globalThis: bare `fetch` as a default arg loses Window receiver → Illegal invocation.
    private readonly fetchImpl: typeof fetch = globalThis.fetch.bind(globalThis),
  ) {}

  private url(path: string): string {
    return `${this.endpoint.replace(/\/$/, "")}${path}`;
  }

  private async post<T>(path: string, body: unknown, signal?: AbortSignal): Promise<T> {
    const res = await this.fetchImpl(this.url(path), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.token}`,
      },
      body: JSON.stringify(body),
      signal,
    });
    const text = await res.text();
    let data: unknown = undefined;
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        data = text;
      }
    }
    if (!res.ok) {
      const msg =
        typeof data === "object" && data && "message" in data
          ? String((data as { message: string }).message)
          : `gateway ${res.status}`;
      throw new GatewayApiError(msg, res.status, data);
    }
    return data as T;
  }

  presignPut(
    input: {
      bucket: string;
      key: string;
      contentType?: string;
      meta?: Meta;
      expireSec?: number;
      /** Exact upload size; signed as Content-Length */
      size?: number;
    },
    signal?: AbortSignal,
  ): Promise<PresignPutResponse> {
    return this.post("/presign/put", input, signal);
  }

  presignGet(input: { bucket: string; key: string; expireSec?: number }): Promise<PresignGetResponse> {
    return this.post("/presign/get", input);
  }

  multipartCreate(
    input: {
      bucket: string;
      key: string;
      contentType?: string;
      meta?: Meta;
      expireSec?: number;
      size?: number;
    },
    signal?: AbortSignal,
  ): Promise<MultipartCreateResponse> {
    return this.post("/multipart/create", input, signal);
  }

  multipartPresignPart(
    input: {
      bucket: string;
      key: string;
      uploadId: string;
      partNumber: number;
      expireSec?: number;
    },
    signal?: AbortSignal,
  ): Promise<MultipartPresignPartResponse> {
    return this.post("/multipart/presign-part", input, signal);
  }

  /** Sign up to 1000 part URLs in one call. */
  multipartPresignParts(
    input: {
      bucket: string;
      key: string;
      uploadId: string;
      partNumbers: number[];
      expireSec?: number;
    },
    signal?: AbortSignal,
  ): Promise<MultipartPresignPartsResponse> {
    return this.post("/multipart/presign-parts", input, signal);
  }

  multipartComplete(
    input: {
      bucket: string;
      key: string;
      uploadId: string;
      parts: Array<{ partNumber: number; etag: string }>;
    },
    signal?: AbortSignal,
  ): Promise<MultipartCompleteResponse> {
    return this.post("/multipart/complete", input, signal);
  }

  multipartAbort(input: { bucket: string; key: string; uploadId: string }): Promise<{ status: string }> {
    return this.post("/multipart/abort", input);
  }

  objectHead(input: { bucket: string; key: string }): Promise<ObjectHeadResponse> {
    return this.post("/object/head", input);
  }

  async objectList(input: {
    bucket: string;
    prefix?: string;
    delimiter?: string;
    continuationToken?: string;
    maxKeys?: number;
  }): Promise<ListObjectsResponse> {
    const res = await this.post<ListObjectsResponse>("/object/list", input);
    return { ...res, entries: res.entries ?? [] };
  }

  objectDelete(input: { bucket: string; key: string }): Promise<{ status: string }> {
    return this.post("/object/delete", input);
  }

  objectCopy(input: { bucket: string; srcKey: string; destKey: string }): Promise<{ status: string; cdnUrl: string }> {
    return this.post("/object/copy", input);
  }
}
