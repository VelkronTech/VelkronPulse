// Velkron Pulse — Frontend Application
// Connects to the WebSocket for real-time updates and renders the dashboard.

(function () {
    'use strict';

    // --- State ---
    let currentData = null;
    let ws = null;
    let reconnectTimer = null;
    let diskThreshold = parseInt(localStorage.getItem('diskThreshold') || '90', 10);
    let cpuThreshold = parseInt(localStorage.getItem('cpuThreshold') || '90', 10);

    // --- DOM References ---
    const $ = (id) => document.getElementById(id);
    const statusDot = $('statusDot');
    const statusText = $('statusText');

    // --- Utilities ---
    function formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(1024));
        return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
    }

    function formatDuration(seconds) {
        if (seconds < 60) return seconds + 's';
        if (seconds < 3600) return Math.floor(seconds / 60) + 'm ' + (seconds % 60) + 's';
        const h = Math.floor(seconds / 3600);
        const m = Math.floor((seconds % 3600) / 60);
        return h + 'h ' + m + 'm';
    }

    function formatResponseTime(d) {
        if (d === 0) return '-';
        if (d < 1000) return d + 'µs';
        if (d < 1000000) return (d / 1000).toFixed(1) + 'ms';
        return (d / 1000000).toFixed(2) + 's';
    }

    // --- Connection Status ---
    function setConnected(connected) {
        if (connected) {
            statusDot.className = 'status-dot connected';
            statusText.textContent = 'Connected';
        } else {
            statusDot.className = 'status-dot';
            statusText.textContent = 'Disconnected';
        }
    }

    // --- WebSocket ---
    function connectWebSocket() {
        if (ws && ws.readyState === WebSocket.OPEN) return;

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = protocol + '//' + window.location.host + '/ws';

        ws = new WebSocket(wsUrl);

        ws.onopen = function () {
            setConnected(true);
            if (reconnectTimer) {
                clearTimeout(reconnectTimer);
                reconnectTimer = null;
            }
        };

        ws.onmessage = function (event) {
            try {
                const data = JSON.parse(event.data);
                currentData = data;
                renderDashboard(data);
            } catch (e) {
                console.error('Failed to parse WebSocket message:', e);
            }
        };

        ws.onclose = function () {
            setConnected(false);
            scheduleReconnect();
        };

        ws.onerror = function () {
            // onclose will fire after this
        };
    }

    function scheduleReconnect() {
        if (reconnectTimer) return;
        reconnectTimer = setTimeout(function () {
            reconnectTimer = null;
            connectWebSocket();
        }, 3000);
    }

    // --- Initial Fetch (fallback if WS not yet connected) ---
    function fetchInitial() {
        fetch('/api/status')
            .then(function (r) { return r.json(); })
            .then(function (data) {
                currentData = data;
                renderDashboard(data);
            })
            .catch(function (e) {
                console.error('Initial fetch failed:', e);
            });
    }

    // --- Rendering ---
    function renderDashboard(data) {
        if (!data) return;
        renderMetrics(data.metrics);
        renderServices(data.services);
        checkAlerts(data);
    }

    function renderMetrics(metrics) {
        if (!metrics) return;

        // CPU Gauge
        const cpuPct = metrics.cpu ? metrics.cpu.percent : 0;
        const circumference = 314.159;
        const offset = circumference - (cpuPct / 100) * circumference;
        const cpuGauge = $('cpuGauge');
        if (cpuGauge) {
            cpuGauge.style.strokeDashoffset = offset;
            cpuGauge.style.stroke = cpuPct > cpuThreshold ? 'var(--danger)' : cpuPct > 70 ? 'var(--warning)' : 'var(--accent)';
        }
        const cpuText = $('cpuText');
        if (cpuText) cpuText.textContent = cpuPct.toFixed(1) + '%';

        // Memory
        const mem = metrics.memory;
        if (mem) {
            const memPct = mem.percent || 0;
            const memBar = $('memBar');
            if (memBar) memBar.style.width = memPct.toFixed(1) + '%';
            const memPercent = $('memPercent');
            if (memPercent) memPercent.textContent = memPct.toFixed(1) + '%';
            const memUsed = $('memUsed');
            if (memUsed) memUsed.textContent = formatBytes(mem.used);
            const memTotal = $('memTotal');
            if (memTotal) memTotal.textContent = formatBytes(mem.total);
            const memAvail = $('memAvail');
            if (memAvail) memAvail.textContent = formatBytes(mem.available);
        }

        // Uptime
        const uptime = $('uptimeValue');
        if (uptime && metrics.uptime) {
            uptime.textContent = formatDuration(metrics.uptime);
        }

        // Disks
        renderDisks(metrics.disks);

        // Networks
        renderNetworks(metrics.networks);
    }

    function renderDisks(disks) {
        const container = $('diskList');
        if (!container) return;

        if (!disks || disks.length === 0) {
            container.innerHTML = '<div class="disk-item"><span class="text-muted">No disk data</span></div>';
            return;
        }

        container.innerHTML = disks.map(function (d) {
            const pct = d.percent || 0;
            let cls = 'normal';
            if (pct > 90) cls = 'danger';
            else if (pct > 75) cls = 'warning';

            return '<div class="disk-item">' +
                '<div class="disk-header">' +
                '<span class="disk-name">' + escapeHtml(d.mount_point) + '</span>' +
                '<span class="disk-percent ' + cls + '">' + pct.toFixed(1) + '%</span>' +
                '</div>' +
                '<div class="disk-bar">' +
                '<div class="disk-bar-fill ' + cls + '" style="width:' + pct.toFixed(1) + '%"></div>' +
                '</div>' +
                '<div class="disk-details">' +
                '<span>Total: ' + formatBytes(d.total) + '</span>' +
                '<span>Used: ' + formatBytes(d.used) + '</span>' +
                '<span>Free: ' + formatBytes(d.free) + '</span>' +
                '</div>' +
                '</div>';
        }).join('');
    }

    function renderNetworks(networks) {
        const container = $('netList');
        if (!container) return;

        if (!networks || networks.length === 0) {
            container.innerHTML = '<div class="net-item"><span class="text-muted">No network data</span></div>';
            return;
        }

        container.innerHTML = networks.map(function (n) {
            return '<div class="net-item">' +
                '<div class="net-header">' +
                '<span class="net-name">' + escapeHtml(n.name) + '</span>' +
                '</div>' +
                '<div class="net-details">' +
                '<span>⬆ Sent: ' + formatBytes(n.bytes_sent) + '</span>' +
                '<span>⬇ Received: ' + formatBytes(n.bytes_recv) + '</span>' +
                '</div>' +
                '</div>';
        }).join('');
    }

    function renderServices(services) {
        if (!services) return;

        const knownServices = [];
        const customServices = [];

        services.forEach(function (s) {
            if (s.is_custom) {
                customServices.push(s);
            } else {
                knownServices.push(s);
            }
        });

        renderKnownServices(knownServices);
        renderCustomServices(customServices);
    }

    function renderKnownServices(services) {
        const tbody = $('servicesBody');
        if (!tbody) return;

        if (services.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-muted">No services detected</td></tr>';
            return;
        }

        tbody.innerHTML = services.map(function (s) {
            const isUp = s.status === 'UP';
            return '<tr>' +
                '<td><strong>' + escapeHtml(s.name) + '</strong></td>' +
                '<td>' + s.port + '</td>' +
                '<td><span class="badge ' + (isUp ? 'badge-up' : 'badge-down') + '">' + s.status + '</span></td>' +
                '<td>' + formatResponseTime(s.response_time) + '</td>' +
                '</tr>';
        }).join('');
    }

    function renderCustomServices(services) {
        const tbody = $('customBody');
        if (!tbody) return;

        if (services.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" class="text-muted">No custom endpoints configured</td></tr>';
            return;
        }

        tbody.innerHTML = services.map(function (s) {
            const isUp = s.status === 'UP';
            return '<tr>' +
                '<td><strong>' + escapeHtml(s.name) + '</strong></td>' +
                '<td>' + escapeHtml(s.port ? ':' + s.port : '-') + '</td>' +
                '<td>' + (s.is_custom ? 'Custom' : 'Auto') + '</td>' +
                '<td><span class="badge ' + (isUp ? 'badge-up' : 'badge-down') + '">' + s.status + '</span></td>' +
                '<td>' + formatResponseTime(s.response_time) + '</td>' +
                '</tr>';
        }).join('');
    }

    // --- Alerts ---
    function checkAlerts(data) {
        if (!data || !data.metrics || !data.services) return;

        // Check disk thresholds
        if (data.metrics.disks) {
            data.metrics.disks.forEach(function (d) {
                if (d.percent > diskThreshold) {
                    showAlert('Disk usage on ' + d.mount_point + ' is at ' + d.percent.toFixed(1) + '%', 'warning');
                }
            });
        }

        // Check CPU threshold
        if (data.metrics.cpu && data.metrics.cpu.percent > cpuThreshold) {
            showAlert('CPU usage is at ' + data.metrics.cpu.percent.toFixed(1) + '%', 'warning');
        }

        // Check down services
        if (data.services) {
            data.services.forEach(function (s) {
                if (s.status === 'DOWN') {
                    showAlert(s.name + ' on port ' + s.port + ' is DOWN', 'danger');
                }
            });
        }
    }

    var alertTimers = {};

    function showAlert(message, type) {
        // Debounce: don't show the same alert more than once per 30 seconds
        var key = message;
        if (alertTimers[key]) return;
        alertTimers[key] = true;
        setTimeout(function () { delete alertTimers[key]; }, 30000);

        // Try browser notification API
        if ('Notification' in window && Notification.permission === 'granted') {
            new Notification('Velkron Pulse', { body: message });
        }

        // Also show in-page toast
        var toast = document.createElement('div');
        toast.className = 'toast toast-' + type;
        toast.textContent = message;
        toast.style.cssText =
            'position:fixed;bottom:60px;right:20px;padding:12px 20px;border-radius:8px;' +
            'font-size:14px;z-index:1000;animation:slideIn 0.3s ease;' +
            'background:' + (type === 'danger' ? 'var(--danger-bg)' : 'var(--warning-bg)') + ';' +
            'color:' + (type === 'danger' ? 'var(--danger)' : 'var(--warning)') + ';' +
            'border:1px solid ' + (type === 'danger' ? 'var(--danger)' : 'var(--warning)') + ';' +
            'max-width:400px;box-shadow:0 4px 12px rgba(0,0,0,0.4);';
        document.body.appendChild(toast);

        setTimeout(function () {
            toast.style.opacity = '0';
            toast.style.transition = 'opacity 0.3s';
            setTimeout(function () { toast.remove(); }, 300);
        }, 5000);
    }

    // --- Tab Navigation ---
    function initTabs() {
        var tabs = document.querySelectorAll('.nav-tab');
        tabs.forEach(function (tab) {
            tab.addEventListener('click', function () {
                var target = this.getAttribute('data-tab');

                // Update tab buttons
                tabs.forEach(function (t) { t.classList.remove('active'); });
                this.classList.add('active');

                // Update tab content
                document.querySelectorAll('.tab-content').forEach(function (tc) {
                    tc.classList.remove('active');
                });
                var targetTab = document.getElementById('tab-' + target);
                if (targetTab) targetTab.classList.add('active');
            });
        });
    }

    // --- Settings ---
    function loadSettings() {
        fetch('/api/settings')
            .then(function (r) { return r.json(); })
            .then(function (settings) {
                if (settings.disk_threshold) {
                    diskThreshold = parseInt(settings.disk_threshold, 10);
                    var el = $('diskThreshold');
                    if (el) el.value = diskThreshold;
                    localStorage.setItem('diskThreshold', diskThreshold);
                }
                if (settings.cpu_threshold) {
                    cpuThreshold = parseInt(settings.cpu_threshold, 10);
                    var el = $('cpuThreshold');
                    if (el) el.value = cpuThreshold;
                    localStorage.setItem('cpuThreshold', cpuThreshold);
                }
            })
            .catch(function () { /* use defaults */ });
    }

    window.saveThresholds = function () {
        var diskVal = $('diskThreshold') ? parseInt($('diskThreshold').value, 10) : 90;
        var cpuVal = $('cpuThreshold') ? parseInt($('cpuThreshold').value, 10) : 90;

        diskThreshold = diskVal;
        cpuThreshold = cpuVal;
        localStorage.setItem('diskThreshold', diskThreshold);
        localStorage.setItem('cpuThreshold', cpuThreshold);

        fetch('/api/settings', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ key: 'disk_threshold', value: String(diskThreshold) })
        }).catch(function () {});
        fetch('/api/settings', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ key: 'cpu_threshold', value: String(cpuThreshold) })
        }).catch(function () {});

        showAlert('Thresholds saved', 'warning');
    };

    window.addEndpoint = function () {
        var name = $('epName') ? $('epName').value.trim() : '';
        var url = $('epUrl') ? $('epUrl').value.trim() : '';
        var type = $('epType') ? $('epType').value : 'http';

        if (!name || !url) {
            showAlert('Name and URL are required', 'danger');
            return;
        }

        fetch('/api/endpoints', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: name, url: url, type: type })
        })
            .then(function (r) {
                if (r.ok) {
                    showAlert('Endpoint added', 'warning');
                    if ($('epName')) $('epName').value = '';
                    if ($('epUrl')) $('epUrl').value = '';
                } else {
                    showAlert('Failed to add endpoint', 'danger');
                }
            })
            .catch(function () {
                showAlert('Failed to add endpoint', 'danger');
            });
    };

    // --- Helpers ---
    function escapeHtml(str) {
        if (!str) return '';
        var div = document.createElement('div');
        div.appendChild(document.createTextNode(str));
        return div.innerHTML;
    }

    // --- Init ---
    function init() {
        // Request notification permission
        if ('Notification' in window && Notification.permission === 'default') {
            Notification.requestPermission();
        }

        initTabs();
        fetchInitial();
        connectWebSocket();
        loadSettings();

        // Refresh custom endpoints list periodically
        setInterval(function () {
            fetch('/api/endpoints')
                .then(function (r) { return r.json(); })
                .then(function (endpoints) {
                    // Re-render services with updated custom endpoints
                    if (currentData && currentData.services) {
                        var known = currentData.services.filter(function (s) { return !s.is_custom; });
                        var custom = endpoints.map(function (ep) {
                            // Find matching service status
                            var match = currentData.services.find(function (s) {
                                return s.is_custom && s.endpoint_id === ep.id;
                            });
                            return match || {
                                name: ep.name,
                                port: 0,
                                status: 'UNKNOWN',
                                response_time: 0,
                                is_custom: true,
                                endpoint_id: ep.id
                            };
                        });
                        renderKnownServices(known);
                        renderCustomServices(custom);
                    }
                })
                .catch(function () {});
        }, 10000);
    }

    // Run on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
