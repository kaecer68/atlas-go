import { fmtSafeNumber } from '../shared/format-metric.js';
import { postJSON } from '../shared/app-utils.js';

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
        if (this.resetBtn) {
            this.resetBtn.addEventListener('click', () => this.handleReset());
        }
    }

    async fetchState() {
        try {
            const res = await fetch('/api/dashboard/circuit-breaker');
            if (res.ok) {
                const data = await res.json();
                this.updateUI(data);
            } else {
                console.warn("Circuit breaker API returned status:", res.status);
                this.showEmptyState();
            }
        } catch (e) {
            console.error("Failed to fetch circuit breaker state:", e);
            this.showEmptyState();
        }
    }

    showEmptyState() {
        if (this.statusText) this.statusText.textContent = '未連線';
        if (this.statusDot) {
            this.statusDot.className = 'cb-status-dot';
            this.statusDot.classList.add('unknown');
        }
        if (this.intradayPeak) this.intradayPeak.textContent = '-';
        if (this.consecutiveSL) this.consecutiveSL.textContent = '-';
        if (this.cooldown) this.cooldown.textContent = '-';
        if (this.eventList) {
            this.eventList.innerHTML = '<li class="cb-event-item empty text-center" style="text-align: center;">暫無事件</li>';
        }
        if (this.resetBtn) {
            this.resetBtn.disabled = true;
        }
    }

    showUninitializedState() {
        if (this.statusText) this.statusText.textContent = '未初始化';
        if (this.statusDot) {
            this.statusDot.className = 'cb-status-dot';
            this.statusDot.classList.add('uninitialized');
        }
        if (this.intradayPeak) this.intradayPeak.textContent = '無數據';
        if (this.consecutiveSL) this.consecutiveSL.textContent = '無數據';
        if (this.cooldown) this.cooldown.textContent = '無數據';
        if (this.eventList) {
            this.eventList.innerHTML = '<li class="cb-event-item empty text-center" style="text-align: center;">尚無實盤交易紀錄</li>';
        }
        if (this.resetBtn) {
            this.resetBtn.disabled = true;
            this.resetBtn.textContent = '未啟用';
        }
    }

    updateUI(data) {
        if (!data || typeof data !== 'object') {
            this.showEmptyState();
            return;
        }

        if (data.initialized === false) {
            this.showUninitializedState();
            return;
        }

        const state = data.state || 'normal';
        const stateLabels = {
            'normal': '正常',
            'paused': '暫停',
            'halted': '停止',
            'uninitialized': '未初始化',
            'unknown': '未知'
        };
        
        if (this.statusText) {
            this.statusText.textContent = stateLabels[state] || state;
        }
        
        if (this.statusDot) {
            this.statusDot.className = 'cb-status-dot';
            if (state === 'normal') {
                this.statusDot.classList.add('normal');
            } else if (state === 'paused') {
                this.statusDot.classList.add('paused');
            } else if (state === 'halted') {
                this.statusDot.classList.add('halted');
            } else {
                this.statusDot.classList.add('unknown');
            }
        }
        
        if (this.resetBtn) {
            this.resetBtn.className = 'cb-btn-reset';
            if (state === 'normal') {
                this.resetBtn.disabled = true;
            } else {
                this.resetBtn.disabled = false;
                if (state === 'halted') {
                    this.resetBtn.classList.add('halted');
                }
            }
        }

        if (this.intradayPeak) {
            if (data.intraday_peak !== undefined && data.day_start_value !== undefined && data.day_start_value > 0) {
                const drawdown = ((data.intraday_peak - data.day_start_value) / data.day_start_value * 100);
                this.intradayPeak.textContent = fmtSafeNumber(drawdown, { decimals: 2, suffix: '%' });
            } else if (state === 'normal') {
                this.intradayPeak.textContent = '—';
            } else {
                this.intradayPeak.textContent = '-';
            }
        }

        if (this.consecutiveSL) {
            if (data.consecutive_sl !== undefined) {
                this.consecutiveSL.textContent = data.consecutive_sl;
            } else if (state === 'normal') {
                this.consecutiveSL.textContent = '0';
            } else {
                this.consecutiveSL.textContent = '-';
            }
        }
        
        if (this.cooldown) {
            if (data.cooldown_until) {
                const cdDate = new Date(data.cooldown_until);
                if (cdDate > new Date()) {
                    this.cooldown.textContent = cdDate.toLocaleTimeString('zh-TW');
                } else {
                    this.cooldown.textContent = '無';
                }
            } else {
                this.cooldown.textContent = '無';
            }
        }

        if (data.events && Array.isArray(data.events) && data.events.length > 0) {
            this.events = data.events;
            this.renderEvents();
        } else {
            this.events = [];
            if (this.eventList) {
                this.eventList.innerHTML = '<li class="cb-event-item empty text-center" style="text-align: center;">暫無事件</li>';
            }
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
        if (!this.eventList) return;
        
        this.eventList.innerHTML = '';
        if (this.events.length === 0) {
            this.eventList.innerHTML = '<li class="cb-event-item empty text-center" style="text-align: center;">暫無事件</li>';
            return;
        }

        this.events.forEach(ev => {
            const li = document.createElement('li');
            li.className = 'cb-event-item';
            
            const time = new Date(ev.timestamp || Date.now()).toLocaleTimeString('zh-TW');
            const msgSpan = document.createElement('span');
            msgSpan.textContent = ev.reason || ev.message || JSON.stringify(ev);
            
            const timeSpan = document.createElement('span');
            timeSpan.className = 'cb-event-time';
            timeSpan.textContent = time;

            li.appendChild(msgSpan);
            li.appendChild(timeSpan);
            this.eventList.appendChild(li);
        });
    }

    async handleReset() {
        const reason = prompt("輸入手動重置原因:");
        if (!reason) return;

        try {
            if (this.resetBtn) {
                this.resetBtn.disabled = true;
                this.resetBtn.textContent = '重置中...';
            }
            
            await postJSON('/api/dashboard/circuit-breaker/reset', { reason });
            await this.fetchState();
        } catch (e) {
            console.error("Reset request failed:", e);
            if (e.status) alert("重置失敗");
            else alert("連線錯誤");
        } finally {
            if (this.resetBtn) {
                this.resetBtn.textContent = '手動重置 (Reset)';
            }
        }
    }

    handleSSE(event) {
        if (event.type === 'circuit_breaker_state_change') {
            this.fetchState();
            this.addEvent(event.data);
        } else if (event.type === 'circuit_breaker_event') {
            this.addEvent(event.data);
        } else if (event.type === 'live_event') {
            if (event.data && event.data.source === 'circuit_breaker') {
                this.addEvent(event.data);
                this.fetchState();
            }
        }
    }
}
