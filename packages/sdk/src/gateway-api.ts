import type {
  Meta,
  MultipartCompleteResponse,
  MultipartCreateResponse,
  MultipartPresignPartResponse,
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
}

export class GatewayApi {
  constructor(
    private readonly endpoint: string,
    private readonly token: string,
    private readonly fetchImpl: typeof fetch = fetch,
  ) {}

  private url(path: string): string {
    return `${this.endpoint.replace(/\/$/, "")}${path}`;
  }

  private async post<T>(path: string, body: unknown): Promise<T> {
    const res = await this.fetchImpl(this.url(path), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.token}`,
      },
      body: JSON.stringify(body),
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

  presignPut(input: {
    bucket: string;
    key: string;
    contentType?: string;
    meta?: Meta;
    expireSec?: number;
  }): Promise<PresignPutResponse> {
    return this.post("/presign/put", input);
  }

  presignGet(input: {
    bucket: string;
    key: string;
    expireSec?: number;
  }): Promise<PresignGetResponse> {
    return this.post("/presign/get", input);
  }

  multipartCreate(input: {
    bucket: string;
    key: string;
    contentType?: string;
    meta?: Meta;
    expireSec?: number;
  }): Promise<MultipartCreateResponse> {
    return this.post("/multipart/create", input);
  }

  multipartPresignPart(input: {
    bucket: string;
    key: string;
    uploadId: string;
    partNumber: number;
    expireSec?: number;
  }): Promise<MultipartPresignPartResponse> {
    return this.post("/multipart/presign-part", input);
  }

  multipartComplete(input: {
    bucket: string;
    key: string;
    uploadId: string;
    parts: Array<{ partNumber: number; etag: string }>;
  }): Promise<MultipartCompleteResponse> {
    return this.post("/multipart/complete", input);
  }

  multipartAbort(input: {
    bucket: string;
    key: string;
    uploadId: string;
  }): Promise<{ status: string }> {
    return this.post("/multipart/abort", input);
  }

  objectHead(input: {
    bucket: string;
    key: string;
  }): Promise<ObjectHeadResponse> {
    return this.post("/object/head", input);
  }
}
