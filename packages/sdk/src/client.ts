import { GatewayApi } from "./gateway-api.js";
import { GatewayS3Signer } from "./gateway-signer.js";
import { multipartUploadFile, uploadFile, type BlobLike } from "./upload.js";
import type {
  CopyOptions,
  ListEntry,
  ListObjectsResponse,
  ListOptions,
  MultipartUploadOptions,
  ObjGatewaySDKOptions,
  ObjectHeadResponse,
  UploadOptions,
  UploadResult,
} from "./types.js";

export class ObjGatewaySDK {
  private readonly api: GatewayApi;
  private readonly signer: GatewayS3Signer;
  private readonly defaultBucket?: string;

  constructor(
    options: ObjGatewaySDKOptions,
    // Bind to globalThis: bare `fetch` as a default arg loses Window receiver → Illegal invocation.
    fetchImpl: typeof fetch = globalThis.fetch.bind(globalThis),
  ) {
    this.api = new GatewayApi(options.gatewayEndpoint, options.token, fetchImpl);
    this.signer = new GatewayS3Signer(this.api);
    this.defaultBucket = options.defaultBucket;
  }

  /** Expose signer for advanced AWS SDK integration. */
  getS3Signer(): GatewayS3Signer {
    return this.signer;
  }

  private bucket(opts: { bucket?: string }): string {
    const b = opts.bucket || this.defaultBucket;
    if (!b) {
      throw new Error("bucket is required (pass options.bucket or defaultBucket)");
    }
    return b;
  }

  async upload(file: BlobLike, options: UploadOptions): Promise<UploadResult> {
    return uploadFile(this.api, this.signer, file, {
      ...options,
      bucket: this.bucket(options),
    });
  }

  async multipartUpload(
    file: BlobLike,
    options: MultipartUploadOptions,
  ): Promise<UploadResult> {
    return multipartUploadFile(this.api, this.signer, file, {
      ...options,
      bucket: this.bucket(options),
    });
  }

  async getDownloadUrl(key: string, opts?: { bucket?: string; expireSec?: number }): Promise<string> {
    return this.signer.signGet({
      bucket: this.bucket(opts || {}),
      key,
      expireSec: opts?.expireSec,
    });
  }

  async head(key: string, opts?: { bucket?: string }): Promise<ObjectHeadResponse> {
    return this.api.objectHead({
      bucket: this.bucket(opts || {}),
      key,
    });
  }

  /** One page of objects/folders under prefix. Pass delimiter "/" for a folder view. */
  async list(opts: ListOptions = {}): Promise<ListObjectsResponse> {
    return this.api.objectList({
      bucket: this.bucket(opts),
      prefix: opts.prefix,
      delimiter: opts.delimiter,
      continuationToken: opts.continuationToken,
      maxKeys: opts.maxKeys,
    });
  }

  /** Every object (no folders) under prefix, following pagination. */
  async listAll(prefix: string, opts: Omit<ListOptions, "prefix" | "delimiter" | "continuationToken"> = {}): Promise<ListEntry[]> {
    const out: ListEntry[] = [];
    let continuationToken: string | undefined;
    do {
      const page = await this.list({ ...opts, prefix, continuationToken });
      for (const e of page.entries) {
        if (!e.isDir && e.key && !e.key.endsWith("/")) out.push(e);
      }
      continuationToken = page.isTruncated ? page.nextContinuationToken : undefined;
    } while (continuationToken);
    return out;
  }

  async delete(key: string, opts?: { bucket?: string }): Promise<void> {
    await this.api.objectDelete({ bucket: this.bucket(opts || {}), key });
  }

  /** Server-side copy within a bucket; returns the destination CDN URL (if configured). */
  async copy(opts: CopyOptions): Promise<{ cdnUrl: string }> {
    const res = await this.api.objectCopy({
      bucket: this.bucket(opts),
      srcKey: opts.srcKey,
      destKey: opts.destKey,
    });
    return { cdnUrl: res.cdnUrl };
  }
}
