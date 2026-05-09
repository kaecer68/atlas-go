export class CircuitBreakerPanel {
    constructor(containerId) {
        this.container = document.getElementById(containerId);
        if (!this.container) return;

        this.statusBadge = this.container.querySelector('#cbStatusBadge');
        this.statusDot = this.container.querySelector('#cbStatusDot');
        this.statusText = this.container.querySelector('#cbStatusText');
        this.intradayPeak = this.container.querySelector('#cbIntradayPeak');
        this.consecutiveSL = this.container.querySelector('#cbConsecutiveSL');
        this.cooldown = this.container.querySelector('#cbCooldown');
        this.eventList = this.container.querySelector('#cbEventList');
        this.resetBtn = this.container.querySelector('#cbResetBtn');

        this.events = [];
        this.bindEvents();
        this.fetchState();
    }

    bindEvents() {
        this.resetBtn.addEventListener('click', () => this.handleReset());
    }

    async fetchState() {
        try {
            const res = await fetch('/api/dashboard/circuit-breaker');
            if (res.ok) {
                const data = await res.json();
                this.updateUI(data);
            }
        } catch (e) {
            console.error("Failed to fetch circuit breaker state:", e);
        }
    }

    updateUI(data) {
        // Update Status
        const state = data.state || 'normal';
        this.statusText.textContent = state.charAt(0).toUpperCase() + state.slice(1);
        
        this.statusDot.className = 'cb-status-dot';
        this.resetBtn.className = 'cb-btn-reset';
        
        if (state === 'normal') {
            this.statusDot.classList.add('normal');
            this.resetBtn.disabled = true;
        } else if (state === 'paused') {
            this.statusDot.classList.add('paused');
            this.resetBtn.disabled = false;
        } else if (state === 'halted') {
            this.statusDot.classList.add('halted');
            this.resetBtn.disabled = false;
            this.resetBtn.classList.add('halted');
        }

        // Update Metrics
        if (data.intraday_peak !== undefined && data.day_start_value !== undefined && data.day_start_value > 0) {
            const drawdown = ((data.intraday_peak - data.day_start_value) / data.day_start_value * 100).toFixed(2);
            this.intradayPeak.textContent = `${drawdown}%`;
        } else {
            this.intradayPeak.textContent = '-';
        }

        this.consecutiveSL.textContent = data.consecutive_sl !== undefined ? data.consecutive_sl : '-';
        
        if (data.cooldown_until) {
            const cdDate = new Date(data.cooldown_until);
            if (cdDate > new Date()) {
                this.cooldown.textContent = cdDate.toLocaleTimeString();
            } else {
                this.cooldown.textContent = 'None';
            }
        } else {
            this.cooldown.textContent = 'None';
        }
    }

    addEvent(eventData) {
        this.events.unshift(eventData);
        if (this.events.length > 10) {
            this.events.pop();
        }
        this.renderEvents();
    }

    renderEvents() {
        this.eventList.innerHTML = '';
        if (this.events.length === 0) {
            this.eventList.innerHTML = '<li class="cb-event-item empty text-center" style="text-align: center;">No events yet.</li>';
            return;
        }

        this.events.forEach(ev => {
            const li = document.createElement('li');
            li.className = 'cb-event-item';
            
            const time = new Date(ev.timestamp || Date.now()).toLocaleTimeString();
            const msgSpan = document.createElement('span');
            msgSpan.textContent = ev.message || JSON.stringify(ev);
            
            const timeSpan = document.createElement('span');
            timeSpan.className = 'cb-event-time';
            timeSpan.textContent = time;

            li.appendChild(msgSpan);
            li.appendChild(timeSpan);
            this.eventList.appendChild(li);
        });
    }

    async handleReset() {
        const reason = prompt("Enter reason for manual reset:");
        if (!reason) return;

        try {
            this.resetBtn.disabled = true;
            this.resetBtn.textContent = 'Resetting...';
            
            const res = await fetch('/api/dashboard/circuit-breaker/reset', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ reason })
            });
            
            if (res.ok) {
                await this.fetchState();
            } else {
                alert("Reset failed");
            }
        } catch (e) {
            console.error("Reset request failed:", e);
            alert("Error connecting to server");
        } finally {
            this.resetBtn.textContent = '手動重置 (Reset)';
            // State will be updated by fetchState or SSE
        }
    }

    handleSSE(event) {
        // Look for circuit_breaker specific events or general state updates
        if (event.type === 'circuit_breaker_state_change') {
            this.fetchState();
            this.addEvent(event.data);
        } else if (event.type === 'circuit_breaker_event') {
            this.addEvent(event.data);
        } else if (event.type === 'live_event') {
            // General live events might contain CB info, just an example hook
            if (event.data && event.data.source === 'circuit_breaker') {
                this.addEvent(event.data);
                this.fetchState();
            }
        }
    }
}
