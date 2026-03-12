/* ============================================
   WraithOS Terminal Component
   Lightweight terminal output viewer
   Supports WebSocket streaming and static content
   ============================================ */

class WraithTerminal {
  constructor(containerEl, options = {}) {
    this.container = containerEl;
    this.maxLines = options.maxLines || 500;
    this.autoScroll = true;
    this.lines = [];
    this.ws = null;
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

  clear() {
    this.body.innerHTML = '';
    this.lines = [];
  }

  // Connect to WebSocket for streaming output
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
    this.container.innerHTML = '';
  }
}

// Export for use in wraith.js
window.WraithTerminal = WraithTerminal;
