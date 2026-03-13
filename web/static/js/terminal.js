/* ============================================
   WraithOS Terminal Component
   Lightweight terminal output viewer
   Supports SSE streaming, WebSocket, and static content
   ============================================ */

class WraithTerminal {
  constructor(containerEl, options = {}) {
    this.container = containerEl;
    this.maxLines = options.maxLines || 500;
    this.autoScroll = true;
    this.lines = [];
    this.ws = null;
    this.sse = null;
    this._build();
  }

  _build() {
    this.container.innerHTML = '';
    this.container.classList.add('terminal-panel');

    // Header
    const header = document.createElement('div');
    header.className = 'terminal-header';
    header.innerHTML = `
      <div class="terminal-title">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>
        </svg>
        <span>Output</span>
      </div>
      <div style="display:flex;gap:var(--gap-xs)">
        <button class="btn btn-sm btn-secondary term-clear" title="Clear">Clear</button>
      </div>
    `;
    this.container.appendChild(header);

    // Body
    this.body = document.createElement('div');
    this.body.className = 'terminal-body';
    this.container.appendChild(this.body);

    // Events
    header.querySelector('.term-clear').addEventListener('click', () => this.clear());
    this.body.addEventListener('scroll', () => {
      const { scrollTop, scrollHeight, clientHeight } = this.body;
      this.autoScroll = scrollHeight - scrollTop - clientHeight < 40;
    });
  }

  // Write a line with optional classification
  writeLine(text, type = '') {
    const line = document.createElement('div');
    if (type) line.className = `line-${type}`;
    line.textContent = text;
    this.body.appendChild(line);
    this.lines.push(line);

    // Trim old lines
    while (this.lines.length > this.maxLines) {
      const old = this.lines.shift();
      old.remove();
    }

    if (this.autoScroll) {
      this.body.scrollTop = this.body.scrollHeight;
    }
  }

  // Write raw text, auto-classify lines
  write(text) {
    const lines = text.split('\n');
    for (const line of lines) {
      if (!line.trim()) continue;
      let type = '';
      const lower = line.toLowerCase();
      if (lower.includes('error') || lower.includes('fatal') || lower.startsWith('err')) {
        type = 'err';
      } else if (lower.includes('warning') || lower.includes('warn')) {
        type = 'warn';
      } else if (lower.includes('success') || lower.includes('started') || lower.includes('running') || lower.includes('done')) {
        type = 'ok';
      } else if (lower.startsWith('---') || lower.startsWith('step') || lower.startsWith('==>')) {
        type = 'info';
      }
      this.writeLine(line, type);
    }
  }

  // Map SSE event types from backend to terminal line classes
  _sseTypeToClass(type) {
    switch (type) {
      case 'error':   return 'err';
      case 'warning': return 'warn';
      case 'pull':    return 'info';
      case 'success': return 'ok';
      default:        return '';
    }
  }

  // Connect to a streaming endpoint via POST fetch.
  // Reads the SSE-formatted response body as a stream.
  // Returns a Promise that resolves with {success: bool, error?: string}
  // when the operation completes.
  connectSSE(url) {
    this.disconnectSSE();
    this._sseAbort = new AbortController();
    const signal = this._sseAbort.signal;
    return (async () => {
      try {
        const resp = await fetch(url, { method: 'POST', signal });
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({ error: 'HTTP ' + resp.status }));
          this.writeLine('Error: ' + (err.error || 'request failed'), 'err');
          return { success: false, error: err.error || 'request failed' };
        }
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        let result = { success: false, error: null };
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          // Parse SSE lines: "data: {...}\n\n"
          const parts = buffer.split('\n\n');
          buffer = parts.pop(); // keep incomplete part
          for (const part of parts) {
            const line = part.replace(/^data: /, '').trim();
            if (!line) continue;
            try {
              const data = JSON.parse(line);
              if (data.type === 'complete') {
                if (data.success) {
                  this.writeLine('Operation completed successfully.', 'ok');
                  result = { success: true, error: null };
                } else {
                  this.writeLine('Operation failed: ' + (data.error || 'unknown error'), 'err');
                  result = { success: false, error: data.error || 'unknown error' };
                }
              } else {
                const cls = this._sseTypeToClass(data.type);
                this.writeLine(data.line || '', cls);
              }
            } catch {
              if (line) this.write(line);
            }
          }
        }
        this._sseAbort = null;
        return result;
      } catch (err) {
        if (err.name === 'AbortError') return { success: false, error: 'cancelled' };
        this.writeLine('Stream error: ' + err.message, 'err');
        return { success: false, error: err.message };
      }
    })();
  }

  disconnectSSE() {
    if (this._sseAbort) {
      this._sseAbort.abort();
      this._sseAbort = null;
    }
  }

  clear() {
    this.body.innerHTML = '';
    this.lines = [];
  }

  // Connect to WebSocket for streaming output (legacy, kept for compatibility)
  connectWS(url) {
    this.disconnectWS();
    try {
      this.ws = new WebSocket(url);
      this.ws.onmessage = (e) => {
        try {
          const data = JSON.parse(e.data);
          this.write(data.data || data.output || data.message || e.data);
        } catch {
          this.write(e.data);
        }
      };
      this.ws.onopen = () => {
        this.writeLine('Connected to output stream', 'info');
      };
      this.ws.onclose = () => {
        this.writeLine('Stream disconnected', 'warn');
      };
      this.ws.onerror = () => {
        this.writeLine('Stream connection error', 'err');
      };
    } catch (err) {
      this.writeLine(`WebSocket error: ${err.message}`, 'err');
    }
  }

  disconnectWS() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  destroy() {
    this.disconnectWS();
    this.disconnectSSE();
    this.container.innerHTML = '';
  }
}

// Export for use in wraith.js
window.WraithTerminal = WraithTerminal;
