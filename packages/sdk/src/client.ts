import { GatewayApi } from "./gateway-api.js";
import { GatewayS3Signer } from "./gateway-signer.js";
import { multipartUploadFile, uploadFile, type BlobLike } from "./upload.js";
import type {
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

  constructor(options: ObjGatewaySDKOptions, fetchImpl: typeof fetch = fetch) {
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
}
