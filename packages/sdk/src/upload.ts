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

function putWithProgress(
  url: string,
  body: Blob,
  headers: Record<string, string>,
  onProgress?: (ev: ProgressEvent) => void,
): Promise<{ etag: string }> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url);
    for (const [k, v] of Object.entries(headers)) {
      xhr.setRequestHeader(k, v);
    }
    xhr.upload.onprogress = (e) => {
      if (!onProgress || !e.lengthComputable) return;
      onProgress({
        loaded: e.loaded,
        total: e.total,
        percent: e.total ? Math.round((e.loaded / e.total) * 100) : 0,
      });
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        const etag = (xhr.getResponseHeader("ETag") || "").replaceAll('"', "");
        resolve({ etag });
        return;
      }
      reject(new Error(`PUT failed: ${xhr.status} ${xhr.responseText}`));
    };
    xhr.onerror = () => reject(new Error("PUT network error"));
    xhr.send(body);
  });
}

async function putFetch(
  url: string,
  body: Blob,
  headers: Record<string, string>,
): Promise<{ etag: string }> {
  const res = await fetch(url, { method: "PUT", headers, body });
  if (!res.ok) {
    throw new Error(`PUT failed: ${res.status} ${await res.text()}`);
  }
  const etag = (res.headers.get("ETag") || "").replaceAll('"', "");
  return { etag };
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

export async function uploadFile(
  api: GatewayApi,
  signer: GatewayS3Signer,
  data: BlobLike,
  opts: UploadOptions & { bucket: string },
): Promise<UploadResult> {
  const contentType = guessContentType(data, opts.contentType);
  const blob = asBlob(data, contentType);
  const signed = await signer.signPut({
    bucket: opts.bucket,
    key: opts.key,
    contentType,
    meta: opts.meta,
    expireSec: opts.expireSec,
  });

  const headers: Record<string, string> = {
    "Content-Type": contentType,
    ...metaHeaders(opts.meta),
  };

  let etag: string;
  if (typeof XMLHttpRequest !== "undefined" && opts.onProgress) {
    ({ etag } = await putWithProgress(signed.uploadUrl, blob, headers, opts.onProgress));
  } else {
    ({ etag } = await putFetch(signed.uploadUrl, blob, headers));
    opts.onProgress?.({ loaded: blob.size, total: blob.size, percent: 100 });
  }

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

const DEFAULT_PART_SIZE = 5 * 1024 * 1024;

export async function multipartUploadFile(
  api: GatewayApi,
  signer: GatewayS3Signer,
  data: BlobLike,
  opts: MultipartUploadOptions & { bucket: string },
): Promise<UploadResult> {
  const contentType = guessContentType(data, opts.contentType);
  const blob = asBlob(data, contentType);
  const partSize = opts.partSize && opts.partSize >= 5 * 1024 * 1024 ? opts.partSize : DEFAULT_PART_SIZE;

  const { uploadId } = await api.multipartCreate({
    bucket: opts.bucket,
    key: opts.key,
    contentType,
    meta: opts.meta,
    expireSec: opts.expireSec,
  });

  const parts: Array<{ partNumber: number; etag: string }> = [];
  let uploaded = 0;

  try {
    const totalParts = Math.max(1, Math.ceil(blob.size / partSize));
    for (let i = 0; i < totalParts; i++) {
      const partNumber = i + 1;
      const start = i * partSize;
      const end = Math.min(start + partSize, blob.size);
      const chunk = blob.slice(start, end);
      const partUrl = await signer.signPart({
        bucket: opts.bucket,
        key: opts.key,
        uploadId,
        partNumber,
        expireSec: opts.expireSec,
      });
      const { etag } = await putFetch(partUrl, chunk, {});
      if (!etag) {
        throw new Error(`missing ETag for part ${partNumber}`);
      }
      parts.push({ partNumber, etag });
      uploaded += chunk.size;
      opts.onProgress?.({
        loaded: uploaded,
        total: blob.size,
        percent: blob.size ? Math.round((uploaded / blob.size) * 100) : 100,
      });
    }

    const { etag } = await api.multipartComplete({
      bucket: opts.bucket,
      key: opts.key,
      uploadId,
      parts,
    });

    const head = await api.objectHead({ bucket: opts.bucket, key: opts.key }).catch(() => null);
    return {
      bucket: opts.bucket,
      key: opts.key,
      cdnUrl: head?.cdnUrl || "",
      etag: head?.etag || etag,
      size: head?.size ?? blob.size,
      contentType: head?.contentType || contentType,
      meta: head?.meta || opts.meta || {},
    };
  } catch (err) {
    await api.multipartAbort({ bucket: opts.bucket, key: opts.key, uploadId }).catch(() => undefined);
    throw err;
  }
}
