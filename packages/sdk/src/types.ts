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
  /** Abort the upload (multipart uploads are aborted on the server too). */
  signal?: AbortSignal;
}

export interface MultipartUploadOptions extends UploadOptions {
  /** Part size in bytes (min/default 5 MiB; raised automatically to stay within 10,000 parts) */
  partSize?: number;
  /** Parts uploaded in parallel (default 4) */
  concurrency?: number;
  /** Retries per part on network errors and 5xx/429 responses (default 3) */
  maxRetries?: number;
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

export interface SignedPart {
  partNumber: number;
  url: string;
}

export interface MultipartPresignPartsResponse {
  parts: SignedPart[];
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

export interface ListEntry {
  key: string;
  isDir: boolean;
  size?: number;
  etag?: string;
  lastModified?: string;
  cdnUrl?: string;
}

export interface ListObjectsResponse {
  entries: ListEntry[];
  isTruncated: boolean;
  nextContinuationToken: string;
}

export interface ListOptions {
  bucket?: string;
  prefix?: string;
  /** "/" for a single-level (folder) listing */
  delimiter?: string;
  continuationToken?: string;
  /** Page size (max 1000) */
  maxKeys?: number;
}

export interface CopyOptions {
  bucket?: string;
  srcKey: string;
  destKey: string;
}
