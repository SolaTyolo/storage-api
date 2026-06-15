export type Meta = Record<string, string>;

export interface ObjGatewaySDKOptions {
  /** Gateway base URL, e.g. http://localhost:8080/api/v1 */
  gatewayEndpoint: string;
  /** JWT Bearer token */
  token: string;
  /** Default bucket when not passed per call */
  defaultBucket?: string;
}

export interface ProgressEvent {
  loaded: number;
  total: number;
  percent: number;
}

export interface UploadOptions {
  key: string;
  bucket?: string;
  contentType?: string;
  meta?: Meta;
  expireSec?: number;
  onProgress?: (ev: ProgressEvent) => void;
}

export interface MultipartUploadOptions extends UploadOptions {
  /** Part size in bytes (default 5 MiB) */
  partSize?: number;
}

export interface UploadResult {
  bucket: string;
  key: string;
  cdnUrl: string;
  etag: string;
  size: number;
  contentType: string;
  meta: Meta;
}

export interface PresignPutResponse {
  uploadUrl: string;
  cdnUrl: string;
}

export interface PresignGetResponse {
  downloadUrl: string;
}

export interface MultipartCreateResponse {
  uploadId: string;
}

export interface MultipartPresignPartResponse {
  partUploadUrl: string;
}

export interface MultipartCompleteResponse {
  etag: string;
}

export interface ObjectHeadResponse {
  size: number;
  contentType: string;
  meta: Meta;
  etag: string;
  cdnUrl: string;
}
