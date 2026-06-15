/**
 * Client for Supabase-compatible Storage API (/storage/v1).
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
        data && typeof data === 'object' && (data.message || data.error)
          ? data.message || data.error
          : `HTTP ${res.status}`;
      const err = new Error(msg);
      err.status = res.status;
      err.body = data;
      throw err;
    }
    return data;
  }

  health() {
    return this._fetch('GET', '/health');
  }

  listBuckets() {
    return this._fetch('GET', '/storage/v1/bucket');
  }

  createBucket(id, public = true) {
    return this._fetch('POST', '/storage/v1/bucket', {
      body: { id, name: id, public },
    });
  }

  listObjects(bucketID, prefix = '') {
    return this._fetch('POST', `/storage/v1/object/list/${encodeURIComponent(bucketID)}`, {
      body: { prefix, limit: 100, offset: 0 },
    });
  }

  /**
   * Supabase Storage upload: POST /object/{bucket}/{path}
   * @param {string} bucketID bucket id, supports engine:bucket (e.g. rustfs:uploads)
   * @param {File|Blob} file
   * @param {string} [objectPath]
   */
  async uploadFile(bucketID, file, objectPath) {
    const path = objectPath || (file.name ? file.name : 'upload.bin');
    const contentType = file.type || 'application/octet-stream';
    const encPath = path.split('/').map(encodeURIComponent).join('/');

    const data = await this._fetch('POST', `/storage/v1/object/${encodeURIComponent(bucketID)}/${encPath}`, {
      body: file,
      headers: { 'Content-Type': contentType, 'x-upsert': 'true' },
    });
    return data;
  }

  /**
   * On-demand image transform URL (Supabase render API + extended PDF/Office/video)
   */
  renderURL(bucketID, objectPath, params = {}) {
    const encPath = objectPath.split('/').map(encodeURIComponent).join('/');
    const q = new URLSearchParams();
    if (params.width != null) q.set('width', String(params.width));
    if (params.height != null) q.set('height', String(params.height));
    if (params.resize) q.set('resize', params.resize);
    if (params.quality != null) q.set('quality', String(params.quality));
    if (params.format) q.set('format', params.format);
    if (params.page != null) q.set('page', String(params.page));
    if (params.dpi != null) q.set('dpi', String(params.dpi));
    if (params.t != null) q.set('t', String(params.t));
    const qs = q.toString();
    return `${this.baseUrl}/storage/v1/render/image/public/${encodeURIComponent(bucketID)}/${encPath}${qs ? `?${qs}` : ''}`;
  }

  downloadURL(bucketID, objectPath) {
    const encPath = objectPath.split('/').map(encodeURIComponent).join('/');
    return `${this.baseUrl}/storage/v1/object/public/${encodeURIComponent(bucketID)}/${encPath}`;
  }

  deleteObject(bucketID, objectPath) {
    const encPath = objectPath.split('/').map(encodeURIComponent).join('/');
    return this._fetch('DELETE', `/storage/v1/object/${encodeURIComponent(bucketID)}/${encPath}`);
  }

  static mimeKind(mimetype) {
    const m = (mimetype || '').toLowerCase();
    if (m.startsWith('image/')) return 'image';
    if (m.startsWith('video/')) return 'video';
    if (m === 'application/pdf') return 'pdf';
    if (m.startsWith('audio/')) return 'audio';
    if (
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
