/**
 * Client for the Storage API (/storage/v1).
 */
class StoragePlayground {
  constructor(baseUrl = '') {
    this.baseUrl = (baseUrl || '').replace(/\/+$/, '');
    this.onLog = null;
  }

  /** @param {'info'|'ok'|'warn'|'err'} level */
  _log(level, method, url, status, detail) {
    if (this.onLog) {
      this.onLog({ level, method, url, status, detail, ts: new Date() });
    }
  }

  async _fetch(method, path, { body, headers = {}, raw = false } = {}) {
    const url = path.startsWith('http') ? path : `${this.baseUrl}${path}`;
    const opts = { method, headers: { ...headers } };
    if (body !== undefined && body !== null) {
      if (typeof body === 'string' || body instanceof Blob || body instanceof ArrayBuffer) {
        opts.body = body;
      } else {
        opts.headers['Content-Type'] = 'application/json';
        opts.body = JSON.stringify(body);
      }
    }
    const res = await fetch(url, opts);
    let data = null;
    const ct = res.headers.get('content-type') || '';
    if (raw || res.status === 204) {
      data = null;
    } else if (ct.includes('application/json')) {
      data = await res.json();
    } else {
      data = await res.text();
    }
    const detail = data !== null ? data : res.statusText;
    this._log(res.ok ? 'ok' : 'err', method, url, res.status, detail);
    if (!res.ok) {
      const msg =
        data && typeof data === 'object' && data.error
          ? data.error
          : `HTTP ${res.status}`;
      const err = new Error(msg);
      err.status = res.status;
      err.body = data;
      throw err;
    }
    return data;
  }

  /** GET /health */
  health() {
    return this._fetch('GET', '/health');
  }

  /** GET /storage/v1/buckets */
  listBuckets() {
    return this._fetch('GET', '/storage/v1/buckets');
  }

  /** GET /storage/v1/buckets/{bucketID}/objects */
  listObjects(bucketID, prefix = '') {
    const q = prefix ? `?prefix=${encodeURIComponent(prefix)}` : '';
    return this._fetch('GET', `/storage/v1/buckets/${encodeURIComponent(bucketID)}/objects${q}`);
  }

  /**
   * Presign → PUT → complete upload flow.
   * @param {string} bucketID
   * @param {File|Blob} file
   * @param {string} [objectName]
   */
  async uploadFile(bucketID, file, objectName) {
    const name = objectName || (file.name ? file.name : 'upload.bin');
    const contentType = file.type || 'application/octet-stream';

    const presign = await this._fetch(
      'POST',
      `/storage/v1/buckets/${encodeURIComponent(bucketID)}/uploads/presign`,
      { body: { object_name: name, content_type: contentType } }
    );

    await this._fetch('PUT', presign.presigned_url, {
      body: file,
      headers: { 'Content-Type': contentType },
      raw: true,
    });

    const obj = await this._fetch(
      'POST',
      `/storage/v1/buckets/${encodeURIComponent(bucketID)}/uploads/complete`,
      { body: { complete_token: presign.complete_token } }
    );
    return obj;
  }

  /** GET /storage/v1/objects/{objectID}/download-url */
  downloadURL(objectID) {
    return this._fetch('GET', `/storage/v1/objects/${objectID}/download-url`);
  }

  /**
   * Cloudinary 风格按需变换 URL（直接用于 img src）
   * @param {string} objectID
   * @param {{ w?: number, h?: number, c?: string, q?: number, f?: string, t?: number }} params
   */
  imageURL(objectID, params = {}) {
    const q = new URLSearchParams();
    if (params.w != null) q.set('w', String(params.w));
    if (params.h != null) q.set('h', String(params.h));
    if (params.c) q.set('c', params.c);
    if (params.q != null) q.set('q', String(params.q));
    if (params.f) q.set('f', params.f);
    if (params.t != null) q.set('t', String(params.t));
    if (params.page != null) q.set('page', String(params.page));
    if (params.dpi != null) q.set('dpi', String(params.dpi));
    const qs = q.toString();
    return `${this.baseUrl}/storage/v1/objects/${objectID}/image${qs ? `?${qs}` : ''}`;
  }

  /** DELETE /storage/v1/buckets/{bucketID}/objects/{objectName} */
  deleteObject(bucketID, objectName) {
    return this._fetch(
      'DELETE',
      `/storage/v1/buckets/${encodeURIComponent(bucketID)}/objects/${encodeURIComponent(objectName)}`
    );
  }

  /** Classify MIME type into a coarse kind. */
  static mimeKind(mimetype) {
    const m = (mimetype || '').toLowerCase();
    if (m.startsWith('image/')) return 'image';
    if (m.startsWith('video/')) return 'video';
    if (m === 'application/pdf') return 'pdf';
    if (m.startsWith('audio/')) return 'audio';
    if (
      m === 'application/pdf' ||
      m.startsWith('text/') ||
      m.includes('document') ||
      m.includes('word') ||
      m.includes('sheet') ||
      m.includes('presentation')
    ) {
      return 'document';
    }
    if (
      m.includes('zip') ||
      m.includes('tar') ||
      m.includes('gzip') ||
      m.includes('x-7z') ||
      m.includes('rar')
    ) {
      return 'archive';
    }
    return 'other';
  }

  /** Emoji icon for a MIME type or kind string. */
  static fileIcon(mimetypeOrKind) {
    const kind =
      ['image', 'video', 'pdf', 'audio', 'document', 'archive', 'other'].includes(mimetypeOrKind)
        ? mimetypeOrKind
        : StoragePlayground.mimeKind(mimetypeOrKind);
    const icons = {
      image: '🖼️',
      video: '🎬',
      pdf: '📕',
      audio: '🎵',
      document: '📄',
      archive: '📦',
      other: '📎',
    };
    return icons[kind] || icons.other;
  }

  /** Human-readable byte size. */
  static formatBytes(bytes) {
    if (bytes == null || isNaN(bytes)) return '—';
    const n = Number(bytes);
    if (n === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1);
    const val = n / Math.pow(1024, i);
    return `${val < 10 && i > 0 ? val.toFixed(1) : Math.round(val * 10) / 10} ${units[i]}`;
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { StoragePlayground };
}
