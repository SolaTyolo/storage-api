import type { GatewayApi } from "./gateway-api.js";
import type { GatewayS3Signer } from "./gateway-signer.js";
import type {
  Meta,
  MultipartUploadOptions,
  ProgressEvent,
  UploadOptions,
  UploadResult,
} from "./types.js";

export type BlobLike = Blob | ArrayBuffer | Uint8Array | File;

// S3 multipart limits.
const MIN_PART_SIZE = 5 * 1024 * 1024;
const MAX_PART_SIZE = 5 * 1024 * 1024 * 1024;
const MAX_PARTS = 10000;
const MIB = 1024 * 1024;

const DEFAULT_CONCURRENCY = 4;
const DEFAULT_MAX_RETRIES = 3;
const RETRY_BASE_MS = 200;
const RETRY_MAX_MS = 5000;

/** Non-2xx response from object storage for a presigned PUT. */
export class StorageHttpError extends Error {
  constructor(
    public readonly status: number,
    body: string,
  ) {
    super(`PUT failed: ${status} ${body}`);
    this.name = "StorageHttpError";
  }
}

function asBlob(data: BlobLike, contentType?: string): Blob {
  if (typeof Blob !== "undefined" && data instanceof Blob) {
    return data;
  }
  if (data instanceof ArrayBuffer) {
    return new Blob([data], { type: contentType });
  }
  if (data instanceof Uint8Array) {
    const copy = new Uint8Array(data.byteLength);
    copy.set(data);
    return new Blob([copy.buffer], { type: contentType });
  }
  return data as Blob;
}

function guessContentType(data: BlobLike, fallback?: string): string {
  if (fallback) return fallback;
  if (typeof File !== "undefined" && data instanceof File && data.type) {
    return data.type;
  }
  if (typeof Blob !== "undefined" && data instanceof Blob && data.type) {
    return data.type;
  }
  return "application/octet-stream";
}

function abortError(): Error {
  const err = new Error("upload aborted");
  err.name = "AbortError";
  return err;
}

function putWithProgress(
  url: string,
  body: Blob,
  headers: Record<string, string>,
  onProgress: (loaded: number) => void,
  signal?: AbortSignal,
): Promise<{ etag: string }> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(abortError());
      return;
    }
    const xhr = new XMLHttpRequest();
    const onAbort = () => xhr.abort();
    signal?.addEventListener("abort", onAbort, { once: true });
    const cleanup = () => signal?.removeEventListener("abort", onAbort);
    xhr.open("PUT", url);
    for (const [k, v] of Object.entries(headers)) {
      xhr.setRequestHeader(k, v);
    }
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded);
    };
    xhr.onload = () => {
      cleanup();
      if (xhr.status >= 200 && xhr.status < 300) {
        const etag = (xhr.getResponseHeader("ETag") || "").replaceAll('"', "");
        resolve({ etag });
        return;
      }
      reject(new StorageHttpError(xhr.status, xhr.responseText));
    };
    xhr.onerror = () => {
      cleanup();
      reject(new Error("PUT network error"));
    };
    xhr.onabort = () => {
      cleanup();
      reject(abortError());
    };
    xhr.send(body);
  });
}

async function putFetch(
  url: string,
  body: Blob,
  headers: Record<string, string>,
  signal?: AbortSignal,
): Promise<{ etag: string }> {
  const res = await fetch(url, { method: "PUT", headers, body, signal });
  if (!res.ok) {
    throw new StorageHttpError(res.status, await res.text());
  }
  const etag = (res.headers.get("ETag") || "").replaceAll('"', "");
  return { etag };
}

function put(
  url: string,
  body: Blob,
  headers: Record<string, string>,
  onProgress: ((loaded: number) => void) | undefined,
  signal?: AbortSignal,
): Promise<{ etag: string }> {
  if (onProgress && typeof XMLHttpRequest !== "undefined") {
    return putWithProgress(url, body, headers, onProgress, signal);
  }
  return putFetch(url, body, headers, signal);
}

function metaHeaders(meta?: Meta): Record<string, string> {
  if (!meta) return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(meta)) {
    const key = k.toLowerCase().startsWith("x-amz-meta-")
      ? k.toLowerCase()
      : `x-amz-meta-${k.toLowerCase()}`;
    out[key] = v;
  }
  return out;
}

function progressEvent(loaded: number, total: number): ProgressEvent {
  return { loaded, total, percent: total ? Math.round((loaded / total) * 100) : 100 };
}

export async function uploadFile(
  api: GatewayApi,
  signer: GatewayS3Signer,
  data: BlobLike,
  opts: UploadOptions & { bucket: string },
): Promise<UploadResult> {
  const contentType = guessContentType(data, opts.contentType);
  const blob = asBlob(data, contentType);
  const signed = await signer.signPut(
    {
      bucket: opts.bucket,
      key: opts.key,
      contentType,
      meta: opts.meta,
      expireSec: opts.expireSec,
      size: blob.size,
    },
    opts.signal,
  );

  const headers: Record<string, string> = {
    "Content-Type": contentType,
    ...metaHeaders(opts.meta),
  };

  const onProgress = opts.onProgress
    ? (loaded: number) => opts.onProgress?.(progressEvent(loaded, blob.size))
    : undefined;
  const { etag } = await put(signed.uploadUrl, blob, headers, onProgress, opts.signal);
  opts.onProgress?.(progressEvent(blob.size, blob.size));

  const head = await api.objectHead({ bucket: opts.bucket, key: opts.key }).catch(() => null);

  return {
    bucket: opts.bucket,
    key: opts.key,
    cdnUrl: signed.cdnUrl || head?.cdnUrl || "",
    etag: head?.etag || etag,
    size: head?.size ?? blob.size,
    contentType: head?.contentType || contentType,
    meta: head?.meta || opts.meta || {},
  };
}

/** Returns a part size that keeps the upload within S3 multipart limits. */
export function choosePartSize(size: number, requested?: number): number {
  let partSize = Math.max(requested ?? 0, MIN_PART_SIZE);
  if (partSize > MAX_PART_SIZE) {
    throw new Error(`partSize must be <= ${MAX_PART_SIZE}`);
  }
  if (size > partSize * MAX_PARTS) {
    partSize = Math.ceil(Math.ceil(size / MAX_PARTS) / MIB) * MIB;
  }
  if (partSize > MAX_PART_SIZE) {
    throw new Error(`object size ${size} exceeds the multipart limit`);
  }
  return partSize;
}

function isRetryable(err: unknown, signal: AbortSignal): boolean {
  if (signal.aborted) return false;
  if (err instanceof StorageHttpError) {
    return err.status >= 500 || err.status === 429 || err.status === 408;
  }
  return !(err instanceof Error && err.name === "AbortError");
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(abortError());
      return;
    }
    const t = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    const onAbort = () => {
      clearTimeout(t);
      reject(abortError());
    };
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

export async function multipartUploadFile(
  api: GatewayApi,
  signer: GatewayS3Signer,
  data: BlobLike,
  opts: MultipartUploadOptions & { bucket: string },
): Promise<UploadResult> {
  const contentType = guessContentType(data, opts.contentType);
  const blob = asBlob(data, contentType);
  const partSize = choosePartSize(blob.size, opts.partSize);
  const totalParts = Math.max(1, Math.ceil(blob.size / partSize));
  const concurrency = Math.max(1, Math.min(opts.concurrency ?? DEFAULT_CONCURRENCY, totalParts));
  const maxRetries = Math.max(0, opts.maxRetries ?? DEFAULT_MAX_RETRIES);
  const target = { bucket: opts.bucket, key: opts.key };

  const { uploadId } = await api.multipartCreate(
    { ...target, contentType, meta: opts.meta, expireSec: opts.expireSec, size: blob.size },
    opts.signal,
  );

  // Internal controller stops in-flight parts on the first failure or caller abort.
  const controller = new AbortController();
  const onCallerAbort = () => controller.abort();
  opts.signal?.addEventListener("abort", onCallerAbort, { once: true });
  const signal = controller.signal;

  // Sign lazily in small batches so URLs do not expire before their part is sent.
  const signBatch = concurrency * 2;
  const urls = new Map<number, string>();
  let signedUpTo = 0;
  let signing: Promise<void> | null = null;
  const urlFor = async (partNumber: number): Promise<string> => {
    while (!urls.has(partNumber)) {
      if (!signing) {
        const first = signedUpTo + 1;
        const last = Math.min(first + signBatch - 1, totalParts);
        const partNumbers = Array.from({ length: last - first + 1 }, (_, i) => first + i);
        signing = signer
          .signParts({ ...target, uploadId, partNumbers, expireSec: opts.expireSec }, signal)
          .then((parts) => {
            for (const p of parts) urls.set(p.partNumber, p.url);
            signedUpTo = last;
          })
          .finally(() => {
            signing = null;
          });
      }
      await signing;
    }
    const url = urls.get(partNumber)!;
    urls.delete(partNumber);
    return url;
  };

  const etags: string[] = new Array(totalParts);
  const partLoaded = new Map<number, number>();
  let completedBytes = 0;
  const report = () => {
    if (!opts.onProgress) return;
    let inFlight = 0;
    for (const v of partLoaded.values()) inFlight += v;
    opts.onProgress(progressEvent(Math.min(completedBytes + inFlight, blob.size), blob.size));
  };

  const uploadPart = async (partNumber: number) => {
    const start = (partNumber - 1) * partSize;
    const chunk = blob.slice(start, Math.min(start + partSize, blob.size));
    const url = await urlFor(partNumber);
    const onPartProgress = opts.onProgress
      ? (loaded: number) => {
          partLoaded.set(partNumber, loaded);
          report();
        }
      : undefined;
    for (let attempt = 0; ; attempt++) {
      try {
        const { etag } = await put(url, chunk, {}, onPartProgress, signal);
        if (!etag) {
          throw new Error(`missing ETag for part ${partNumber} (expose ETag in storage CORS)`);
        }
        etags[partNumber - 1] = etag;
        partLoaded.delete(partNumber);
        completedBytes += chunk.size;
        report();
        return;
      } catch (err) {
        partLoaded.delete(partNumber);
        if (attempt >= maxRetries || !isRetryable(err, signal)) throw err;
        await sleep(Math.min(RETRY_BASE_MS * 2 ** attempt, RETRY_MAX_MS), signal);
      }
    }
  };

  let nextPart = 1;
  const worker = async () => {
    while (nextPart <= totalParts && !signal.aborted) {
      await uploadPart(nextPart++);
    }
  };

  try {
    await Promise.all(
      Array.from({ length: concurrency }, () =>
        worker().catch((err) => {
          controller.abort();
          throw err;
        }),
      ),
    );
    if (opts.signal?.aborted) throw abortError();

    const parts = etags.map((etag, i) => ({ partNumber: i + 1, etag }));
    const { etag } = await api.multipartComplete({ ...target, uploadId, parts }, opts.signal);

    const head = await api.objectHead(target).catch(() => null);
    return {
      ...target,
      cdnUrl: head?.cdnUrl || "",
      etag: head?.etag || etag,
      size: head?.size ?? blob.size,
      contentType: head?.contentType || contentType,
      meta: head?.meta || opts.meta || {},
    };
  } catch (err) {
    controller.abort();
    await api.multipartAbort({ ...target, uploadId }).catch(() => undefined);
    throw err;
  } finally {
    opts.signal?.removeEventListener("abort", onCallerAbort);
  }
}
