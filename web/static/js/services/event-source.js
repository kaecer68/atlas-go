class EventSourceService {
  constructor() {
    this.es = null;
    this.listeners = new Map();
    this.status = 'disconnected';
    this.retryCount = 0;
    this.maxRetries = 10;
    this.baseBackoff = 1000;
    this.maxBackoff = 30000;
    this.retryTimer = null;
    this.statusListeners = new Set();
    this.boundMessageHandler = this.handleMessage.bind(this);
  }

  connect(url = '/api/events/stream') {
    if (this.status === 'connected' || this.status === 'connecting') return;
    
    this.status = 'connecting';
    this.notifyStatusChange();

    try {
      this.es = new EventSource(url);

      this.es.onopen = () => {
        this.status = 'connected';
        this.retryCount = 0;
        this.notifyStatusChange();
        console.log('[SSE] Connected to event stream');
      };

      this.es.addEventListener('message', this.boundMessageHandler);
      this.es.addEventListener('simulation.start', this.boundMessageHandler);
      this.es.addEventListener('simulation.complete', this.boundMessageHandler);
      this.es.addEventListener('market.regime.change', this.boundMessageHandler);
      this.es.addEventListener('agent.recommendation', this.boundMessageHandler);
      this.es.addEventListener('guard.outcome', this.boundMessageHandler);
      this.es.addEventListener('portfolio.position.update', this.boundMessageHandler);
      this.es.addEventListener('system.start', this.boundMessageHandler);
      this.es.addEventListener('system.complete', this.boundMessageHandler);

      this.es.onerror = (err) => {
        this.es.close();
        this.es = null;
        
        if (this.retryCount >= this.maxRetries) {
          this.status = 'error';
          this.notifyStatusChange();
          console.error(`[SSE] Max retries (${this.maxRetries}) reached. Giving up.`);
          return;
        }

        this.status = 'connecting';
        this.notifyStatusChange();
        
        let backoff = this.baseBackoff * Math.pow(2, this.retryCount);
        backoff = Math.min(backoff, this.maxBackoff);
        
        console.warn(`[SSE] Connection error. Retrying in ${backoff}ms (attempt ${this.retryCount + 1}/${this.maxRetries})...`);
        
        this.retryTimer = setTimeout(() => {
          this.retryCount++;
          this.connect(url);
        }, backoff);
      };
    } catch (err) {
      console.error('[SSE] Initialization failed', err);
      this.status = 'error';
      this.notifyStatusChange();
    }
  }

  handleMessage(e) {
    console.log('[SSE] Raw event received:', e.type, e.data);
    try {
      const data = JSON.parse(e.data);
      const eventType = data.type || e.type || 'message';
      console.log('[SSE] Parsed event:', eventType, data);
      this.emit(eventType, data);
      this.emit('*', data);
    } catch (err) {
      console.error('[SSE] Failed to parse message', err);
    }
  }

  disconnect() {
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
    if (this.es) {
      this.es.close();
      this.es = null;
    }
    this.status = 'disconnected';
    this.notifyStatusChange();
  }

  on(eventType, callback) {
    if (!this.listeners.has(eventType)) {
      this.listeners.set(eventType, new Set());
    }
    this.listeners.get(eventType).add(callback);
  }

  off(eventType, callback) {
    if (this.listeners.has(eventType)) {
      this.listeners.get(eventType).delete(callback);
    }
  }

  emit(eventType, data) {
    if (this.listeners.has(eventType)) {
      for (const callback of this.listeners.get(eventType)) {
        try {
          callback(data);
        } catch (err) {
          console.error(`[SSE] Error in listener for ${eventType}`, err);
        }
      }
    }
  }

  onStatusChange(callback) {
    this.statusListeners.add(callback);
  }

  offStatusChange(callback) {
    this.statusListeners.delete(callback);
  }

  notifyStatusChange() {
    for (const callback of this.statusListeners) {
      try {
        callback(this.status);
      } catch (err) {
        console.error('[SSE] Error in status listener', err);
      }
    }
  }
}

export const eventSource = new EventSourceService();
