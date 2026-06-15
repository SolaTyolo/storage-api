import type { GatewayApi } from "./gateway-api.js";
import type { Meta } from "./types.js";

/**
 * GatewayS3Signer delegates S3 request signing to the Go gateway.
 * Callers never hold cloud AK/SK; they receive short-lived presigned URLs.
 */
export class GatewayS3Signer {
  constructor(private readonly api: GatewayApi) {}

  async signPut(input: {
    bucket: string;
    key: string;
    contentType?: string;
    meta?: Meta;
    expireSec?: number;
  }): Promise<{ uploadUrl: string; cdnUrl: string }> {
    return this.api.presignPut(input);
  }

  async signPart(input: {
    bucket: string;
    key: string;
    uploadId: string;
    partNumber: number;
    expireSec?: number;
  }): Promise<string> {
    const res = await this.api.multipartPresignPart(input);
    return res.partUploadUrl;
  }

  async signGet(input: {
    bucket: string;
    key: string;
    expireSec?: number;
  }): Promise<string> {
    const res = await this.api.presignGet(input);
    return res.downloadUrl;
  }
}
