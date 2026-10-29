// Injected into the desktop window before any page script runs. The window has
// no network listener, so same-origin /api/ calls are carried over the webview's
// message channel to the Go handlers (see bridge.go) instead of the network.
(() => {
  if (window.__lb) return;

  const calls = new Map();
  let seq = 0;
  const viewCalls = new Map();
  window.__lbViewResult = (id, error) => {
    const call = viewCalls.get(id);
    if (!call) return;
    viewCalls.delete(id);
    if (error) call.reject(new Error(error)); else call.resolve();
  };
  window.__lbOpenView = (view, x, y) => new Promise((resolve, reject) => {
    const id = `view-${++seq}`;
    viewCalls.set(id, { resolve, reject });
    window.__lbOpenViewRequest(id, view, x, y).catch(error => {
      viewCalls.delete(id); reject(error);
    });
  });
  const CHUNK = 512 * 1024;

  const toBase64 = (bytes) => {
    let text = '';
    for (let i = 0; i < bytes.length; i += 0x8000) text += String.fromCharCode.apply(null, bytes.subarray(i, i + 0x8000));
    return btoa(text);
  };
  const fromBase64 = (text) => {
    const bin = atob(text);
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  };

  window.__lb = {
    head(id, status, headers) { calls.get(id)?.onHead(status, headers); },
    chunk(id, data) { calls.get(id)?.onChunk(fromBase64(data)); },
    end(id, error) {
      const call = calls.get(id);
      if (!call) return;
      calls.delete(id);
      call.onEnd(error);
    },
  };

  const isAPI = (input) => {
    try {
      const url = new URL(typeof input === 'string' ? input : input.url || String(input), location.href);
      return url.origin === location.origin && url.pathname.startsWith('/api/');
    } catch {
      return false;
    }
  };

  const dropCall = (id) => {
    if (calls.delete(id)) window.__lbCancel(id);
  };

  // Starts a call. body is a Uint8Array or null; onProgress gets the number of
  // body bytes handed to Go so far.
  async function start(id, method, url, headers, body, onProgress) {
    const parsed = new URL(url, location.href);
    if (body) {
      for (let offset = 0; offset < body.length; offset += CHUNK) {
        const end = Math.min(offset + CHUNK, body.length);
        await window.__lbBody(id, toBase64(body.subarray(offset, end)));
        if (onProgress) onProgress(end);
      }
    }
    await window.__lbRequest(id, method, parsed.pathname + parsed.search, headers);
  }

  const nativeFetch = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    if (!isAPI(input)) return nativeFetch(input, init);
    const request = new Request(input, init);
    const body = request.method === 'GET' || request.method === 'HEAD' ? null : new Uint8Array(await request.arrayBuffer());
    const headers = {};
    request.headers.forEach((value, name) => { headers[name] = value; });
    const id = String(++seq);

    return new Promise((resolve, reject) => {
      let controller;
      let settled = false;
      const stream = new ReadableStream({
        start(c) { controller = c; },
        cancel() { dropCall(id); },
      });
      const abort = () => {
        dropCall(id);
        const error = new DOMException('The operation was aborted.', 'AbortError');
        if (settled) { try { controller.error(error); } catch {} } else { settled = true; reject(error); }
      };
      if (request.signal.aborted) return abort();
      request.signal.addEventListener('abort', abort, { once: true });

      calls.set(id, {
        onHead(status, responseHeaders) {
          if (settled) return;
          settled = true;
          const empty = status === 101 || status === 204 || status === 205 || status === 304;
          resolve(new Response(empty ? null : stream, { status, headers: responseHeaders }));
          if (empty) controller.close();
        },
        onChunk(bytes) { try { controller.enqueue(bytes); } catch {} },
        onEnd(error) {
          request.signal.removeEventListener('abort', abort);
          if (!settled) {
            settled = true;
            reject(new TypeError(error || 'Failed to fetch'));
          } else if (error) {
            try { controller.error(new TypeError(error)); } catch {}
          } else {
            try { controller.close(); } catch {}
          }
        },
      });
      start(id, request.method, request.url, headers, body).catch((error) => {
        dropCall(id);
        if (!settled) { settled = true; reject(error); }
      });
    });
  };

  // EventSource for /api/stream, built on the fetch above. The page closes and
  // reopens the stream itself on error, so there is no automatic reconnect.
  const NativeEventSource = window.EventSource;
  class BridgeEventSource extends EventTarget {
    constructor(url) {
      super();
      this.url = new URL(url, location.href).href;
      this.withCredentials = false;
      this.readyState = 0;
      this.onopen = this.onmessage = this.onerror = null;
      this._abort = new AbortController();
      this._run();
    }
    _emit(type, data) {
      const event = new MessageEvent(type, { data });
      const handler = this['on' + type];
      if (typeof handler === 'function') handler.call(this, event);
      this.dispatchEvent(event);
    }
    async _run() {
      let failed = false;
      try {
        const response = await window.fetch(this.url, { headers: { Accept: 'text/event-stream' }, signal: this._abort.signal });
        if (!response.ok || !response.body) throw new Error('stream failed');
        if (this.readyState === 2) return;
        this.readyState = 1;
        this._emit('open');
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let pending = '';
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          pending += decoder.decode(value, { stream: true }).replace(/\r\n?/g, '\n');
          let split;
          while ((split = pending.indexOf('\n\n')) >= 0) {
            const block = pending.slice(0, split);
            pending = pending.slice(split + 2);
            const data = [];
            let type = 'message';
            for (const line of block.split('\n')) {
              if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''));
              else if (line.startsWith('event:')) type = line.slice(6).trim();
            }
            if (data.length && this.readyState === 1) this._emit(type, data.join('\n'));
          }
        }
        failed = true;
      } catch {
        failed = true;
      }
      if (failed && this.readyState !== 2) {
        this.readyState = 2;
        this._emit('error');
      }
    }
    close() {
      this.readyState = 2;
      this._abort.abort();
    }
  }
  BridgeEventSource.CONNECTING = 0;
  BridgeEventSource.OPEN = 1;
  BridgeEventSource.CLOSED = 2;
  window.EventSource = function EventSource(url, options) {
    return isAPI(url) ? new BridgeEventSource(url) : new NativeEventSource(url, options);
  };
  Object.assign(window.EventSource, { CONNECTING: 0, OPEN: 1, CLOSED: 2, prototype: NativeEventSource.prototype });

  // XMLHttpRequest, for the command upload's progress bar. Only the part of the
  // API the page uses is implemented, and only for /api/ URLs.
  class BridgeXHR extends EventTarget {
    constructor() {
      super();
      this.upload = new EventTarget();
      this.readyState = 0;
      this.status = 0;
      this.responseText = '';
      this._headers = {};
      this._id = null;
    }
    open(method, url) {
      this._method = method;
      this._url = url;
      this.readyState = 1;
    }
    setRequestHeader(name, value) { this._headers[name] = value; }
    _fire(target, type, init) {
      const event = typeof ProgressEvent === 'function' ? new ProgressEvent(type, init) : new Event(type);
      const handler = target['on' + type];
      if (typeof handler === 'function') handler.call(target, event);
      target.dispatchEvent(event);
    }
    async send(payload) {
      const id = String(++seq);
      this._id = id;
      let body = null;
      const headers = { ...this._headers };
      if (payload != null) {
        const packed = new Request(location.origin + '/', { method: 'POST', body: payload });
        const type = packed.headers.get('content-type');
        if (type && !Object.keys(headers).some((name) => name.toLowerCase() === 'content-type')) headers['Content-Type'] = type;
        body = new Uint8Array(await packed.arrayBuffer());
      }
      const total = body ? body.length : 0;
      const chunks = [];
      calls.set(id, {
        onHead: (status) => { this.status = status; },
        onChunk: (bytes) => { chunks.push(bytes); },
        onEnd: (error) => {
          this._id = null;
          if (error) {
            this._fire(this, 'error');
          } else {
            this.responseText = new TextDecoder().decode(concat(chunks));
            this.readyState = 4;
            this._fire(this, 'load');
          }
          this._fire(this, 'loadend');
        },
      });
      try {
        await start(id, this._method, this._url, headers, body, (loaded) => {
          this._fire(this.upload, 'progress', { lengthComputable: true, loaded, total });
        });
      } catch {
        if (calls.delete(id)) {
          this._fire(this, 'error');
          this._fire(this, 'loadend');
        }
      }
    }
    abort() {
      if (this._id !== null && calls.has(this._id)) {
        dropCall(this._id);
        this._id = null;
        this._fire(this, 'abort');
        this._fire(this, 'loadend');
      }
    }
  }
  function concat(chunks) {
    const out = new Uint8Array(chunks.reduce((sum, chunk) => sum + chunk.length, 0));
    let offset = 0;
    for (const chunk of chunks) { out.set(chunk, offset); offset += chunk.length; }
    return out;
  }
  window.XMLHttpRequest = BridgeXHR;

  // Anything that is not the app itself opens in the system browser; a second
  // webview window would have no address bar and no way back.
  const external = (href) => {
    try {
      const url = new URL(href, location.href);
      return (url.protocol === 'http:' || url.protocol === 'https:') && url.origin !== location.origin ? url.href : null;
    } catch {
      return null;
    }
  };
  document.addEventListener('click', (event) => {
    const link = event.target instanceof Element ? event.target.closest('a[href]') : null;
    if (!link || link.hasAttribute('download')) return;
    const target = external(link.href);
    if (target || (link.target === '_blank' && /^https?:/i.test(link.href))) {
      event.preventDefault();
      window.__lbOpenExternal(target || link.href);
    }
  }, true);
  window.open = (url) => {
    const target = url ? external(url) : null;
    if (target) window.__lbOpenExternal(target);
    return null;
  };
})();
