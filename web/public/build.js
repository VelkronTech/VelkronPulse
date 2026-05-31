// ============================================
// VELKRON PULSE — Frontend Application
// Futuristic real-time system monitoring dashboard
// ============================================

(function () {
    'use strict';

    // --- Configuration ---
    const CONFIG = {
        wsReconnectDelay: 3000,
        tickerInterval: 1000,
        customRefreshInterval: 10000,
    };

    // --- State ---
    let currentData = null;
    let ws = null;
    let reconnectTimer = null;
    let diskThreshold = parseInt(localStorage.getItem('diskThreshold') || '90', 10);
    let cpuThreshold = parseInt(localStorage.getItem('cpuThreshold') || '90', 10);
    let endpointType = 'http';
    let alertTimers = {};
    let lastAlertTimes = {};

    // --- DOM Cache ---
    const $ = (id) => document.getElementById(id);
    const $$ = (sel) => document.querySelectorAll(sel);

    // --- Utilities ---
    function formatBytes(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(1024));
        return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
    }

    function formatDuration(seconds) {
        if (!seconds) return '0s';
        if (seconds < 60) return seconds + 's';
        if (seconds < 3600) return Math.floor(seconds / 60) + 'm ' + (seconds % 60) + 's';
        const h = Math.floor(seconds / 3600);
        const m = Math.floor((seconds % 3600) / 60);
        return h + 'h ' + m + 'm';
    }

    function formatResponseTime(ns) {
        if (!ns || ns === 0) return '-';
        if (ns < 1000) return ns.toFixed(0) + 'µs';
        if (ns < 1000000) return (ns / 1000).toFixed(1) + 'ms';
        return (ns / 1000000).toFixed(2) + 's';
    }

    function escapeHtml(str) {
        if (!str) return '';
        var div = document.createElement('div');
        div.appendChild(document.createTextNode(String(str)));
        return div.innerHTML;
    }

    // --- Particle Canvas ---
    function initParticles() {
        var canvas = document.getElementById('particleCanvas');
        if (!canvas) return;
        var ctx = canvas.getContext('2d');
        var particles = [];
        var mouse = { x: 0, y: 0 };
        var animId;

        function resize() {
            canvas.width = window.innerWidth;
            canvas.height = window.innerHeight;
        }

        window.addEventListener('resize', resize);
        resize();

        document.addEventListener('mousemove', function (e) {
            mouse.x = e.clientX;
            mouse.y = e.clientY;
        });

        for (var i = 0; i < 80; i++) {
            particles.push({
                x: Math.random() * canvas.width,
                y: Math.random() * canvas.height,
                vx: (Math.random() - 0.5) * 0.5,
                vy: (Math.random() - 0.5) * 0.5,
                size: Math.random() * 2 + 0.5,
                alpha: Math.random() * 0.5 + 0.1,
            });
        }

        function draw() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);

            for (var i = 0; i < particles.length; i++) {
                var p = particles[i];
                p.x += p.vx;
                p.y += p.vy;

                if (p.x < 0) p.x = canvas.width;
                if (p.x > canvas.width) p.x = 0;
                if (p.y < 0) p.y = canvas.height;
                if (p.y > canvas.height) p.y = 0;

                // Mouse interaction
                var dx = mouse.x - p.x;
                var dy = mouse.y - p.y;
                var dist = Math.sqrt(dx * dx + dy * dy);
                if (dist < 150) {
                    p.vx += dx * 0.00005;
                    p.vy += dy * 0.00005;
                    // Cap velocity
                    var speed = Math.sqrt(p.vx * p.vx + p.vy * p.vy);
                    if (speed > 1) {
                        p.vx = (p.vx / speed) * 1;
                        p.vy = (p.vy / speed) * 1;
                    }
                }

                ctx.beginPath();
                ctx.arc(p.x, p.y, p.size, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(0, 245, 212, ' + p.alpha + ')';
                ctx.fill();

                // Connections
                for (var j = i + 1; j < particles.length; j++) {
                    var p2 = particles[j];
                    var dx2 = p.x - p2.x;
                    var dy2 = p.y - p2.y;
                    var dist2 = Math.sqrt(dx2 * dx2 + dy2 * dy2);
                    if (dist2 < 120) {
                        ctx.beginPath();
                        ctx.moveTo(p.x, p.y);
                        ctx.lineTo(p2.x, p2.y);
                        ctx.strokeStyle = 'rgba(0, 245, 212, ' + (0.08 * (1 - dist2 / 120)) + ')';
                        ctx.lineWidth = 0.5;
                        ctx.stroke();
                    }
                }
            }

            animId = requestAnimationFrame(draw);
        }

        draw();
    }

    // --- WebSocket ---
    function connectWebSocket() {
        if (ws && ws.readyState === WebSocket.OPEN) return;

        var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(protocol + '//' + window.location.host + '/ws');

        ws.onopen = function () {
            setConnected(true);
            if (reconnectTimer) {
                clearTimeout(reconnectTimer);
                reconnectTimer = null;
            }
        };

        ws.onmessage = function (event) {
            try {
                var data = JSON.parse(event.data);
                currentData = data;
                renderDashboard(data);
            } catch (e) {
                console.error('WS parse error:', e);
            }
        };

        ws.onclose = function () {
            setConnected(false);
            scheduleReconnect();
        };

        ws.onerror = function () { /* onclose will fire */ };
    }

    function scheduleReconnect() {
        if (reconnectTimer) return;
        reconnectTimer = setTimeout(function () {
            reconnectTimer = null;
            connectWebSocket();
        }, CONFIG.wsReconnectDelay);
    }

    function setConnected(connected) {
        var dot = document.getElementById('statusDot');
        var text = document.getElementById('statusText');
        if (dot) {
            dot.className = 'pulse-dot' + (connected ? ' connected' : '');
        }
        if (text) {
            text.textContent = connected ? 'LIVE' : 'OFFLINE';
        }
    }

    // --- Initial Fetch ---
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

    // --- Dashboard Rendering ---
    function renderDashboard(data) {
        if (!data) return;
        renderMetrics(data.metrics);
        renderServices(data.services);
        checkAlerts(data);
    }

    function renderMetrics(metrics) {
        if (!metrics) return;

        var cpuPct = metrics.cpu ? metrics.cpu.percent : 0;
        var mem = metrics.memory || {};
        var memPct = mem.percent || 0;

        // --- CPU Gauge ---
        var circumference = 534.07;
        var cpuOffset = circumference - (cpuPct / 100) * circumference;
        var cpuRing = document.getElementById('cpuRing');
        if (cpuRing) {
            cpuRing.style.strokeDashoffset = cpuOffset;

            var gradient;
            if (cpuPct > cpuThreshold) {
                gradient = 'url(#cpuGradientDanger)';
                setBadge('cpuBadge', 'CRITICAL', 'danger');
            } else if (cpuPct > 70) {
                gradient = 'url(#cpuGradientWarn)';
                setBadge('cpuBadge', 'HIGH', 'warning');
            } else {
                gradient = 'url(#cpuGradient)';
                setBadge('cpuBadge', 'NORMAL', '');
            }
            cpuRing.setAttribute('stroke', gradient);
        }

        var cpuValue = document.getElementById('cpuValue');
        if (cpuValue) cpuValue.textContent = cpuPct.toFixed(1);

        // --- Memory Gauge ---
        var memOffset = circumference - (memPct / 100) * circumference;
        var memRing = document.getElementById('memRing');
        if (memRing) {
            memRing.style.strokeDashoffset = memOffset;
        }

        var memValue = document.getElementById('memValue');
        if (memValue) memValue.textContent = memPct.toFixed(1);

        var memUsed = document.getElementById('memUsed');
        if (memUsed) memUsed.textContent = formatBytes(mem.used);

        var memTotal = document.getElementById('memTotal');
        if (memTotal) memTotal.textContent = formatBytes(mem.total);

        if (memPct > 90) {
            setBadge('memBadge', 'CRITICAL', 'danger');
        } else if (memPct > 75) {
            setBadge('memBadge', 'HIGH', 'warning');
        } else {
            setBadge('memBadge', 'NORMAL', '');
        }

        // --- System Info ---
        var uptime = document.getElementById('sysUptime');
        if (uptime && metrics.uptime) {
            uptime.textContent = formatDuration(metrics.uptime);
        }

        var sysHost = document.getElementById('sysHost');
        if (sysHost && metrics.hostname) {
            sysHost.textContent = metrics.hostname;
        }

        var sysPlatform = document.getElementById('sysPlatform');
        if (sysPlatform && metrics.os) {
            sysPlatform.textContent = metrics.os;
        }

        var cpuCores = document.getElementById('cpuCores');
        if (cpuCores && metrics.num_cpu) {
            cpuCores.textContent = metrics.num_cpu;
        }

        // --- Ticker ---
        var tickerUptime = document.getElementById('tickerUptime');
        if (tickerUptime && metrics.uptime) {
            tickerUptime.textContent = formatDuration(metrics.uptime);
        }
        var tickerCpu = document.getElementById('tickerCpu');
        if (tickerCpu) tickerCpu.textContent = cpuPct.toFixed(1) + '%';
        var tickerMem = document.getElementById('tickerMem');
        if (tickerMem) tickerMem.textContent = memPct.toFixed(1) + '%';

        // --- Disks ---
        renderDisks(metrics.disks);

        // --- Networks ---
        renderNetworks(metrics.networks);
    }

    function setBadge(id, text, type) {
        var el = document.getElementById(id);
        if (!el) return;
        el.textContent = text;
        el.className = 'gauge-badge';
        if (type) el.classList.add(type);
    }

    function renderDisks(disks) {
        var container = document.getElementById('diskList');
        var count = document.getElementById('diskCount');
        if (!container) return;

        if (!disks || disks.length === 0) {
            container.innerHTML = '<div class="disk-item" style="color:var(--text-muted);font-size:11px;">No disk data</div>';
            if (count) count.textContent = '0 volumes';
            return;
        }

        if (count) count.textContent = disks.length + ' volume' + (disks.length !== 1 ? 's' : '');

        container.innerHTML = disks.map(function (d) {
            var pct = d.percent || 0;
            var cls = 'normal';
            if (pct > 90) cls = 'danger';
            else if (pct > 75) cls = 'warning';

            return '<div class="disk-item">' +
                '<div class="disk-header">' +
                '<span class="disk-name">' + escapeHtml(d.mount_point) + '</span>' +
                '<span class="disk-percent ' + cls + '">' + pct.toFixed(1) + '%</span>' +
                '</div>' +
                '<div class="disk-bar-track">' +
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
        var container = document.getElementById('netList');
        var count = document.getElementById('netCount');
        if (!container) return;

        if (!networks || networks.length === 0) {
            container.innerHTML = '<div class="net-item" style="color:var(--text-muted);font-size:11px;">No network data</div>';
            if (count) count.textContent = '0 interfaces';
            return;
        }

        if (count) count.textContent = networks.length + ' interface' + (networks.length !== 1 ? 's' : '');

        container.innerHTML = networks.map(function (n) {
            return '<div class="net-item">' +
                '<div class="net-header">' +
                '<span class="net-name">' + escapeHtml(n.name) + '</span>' +
                '</div>' +
                '<div class="net-details">' +
                '<span>⬆ ' + formatBytes(n.bytes_sent) + '</span>' +
                '<span>⬇ ' + formatBytes(n.bytes_recv) + '</span>' +
                '</div>' +
                '</div>';
        }).join('');
    }

    function renderServices(services) {
        if (!services) return;

        var known = [];
        var custom = [];

        services.forEach(function (s) {
            if (s.is_custom) custom.push(s);
            else known.push(s);
        });

        renderKnownServices(known);
        renderCustomServices(custom);
    }

    function renderKnownServices(services) {
        var tbody = document.getElementById('servicesBody');
        var count = document.getElementById('svcCount');
        if (!tbody) return;

        if (count) count.textContent = services.length + ' service' + (services.length !== 1 ? 's' : '');

        if (services.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" style="color:var(--text-muted);padding:20px;text-align:center;">No services detected</td></tr>';
            return;
        }

        tbody.innerHTML = services.map(function (s) {
            var isUp = s.status === 'UP';
            return '<tr>' +
                '<td style="font-weight:600;color:var(--text-primary)">' + escapeHtml(s.name) + '</td>' +
                '<td style="font-family:var(--font-mono);color:var(--accent)">' + (s.port || '-') + '</td>' +
                '<td><span class="badge ' + (isUp ? 'badge-up' : 'badge-down') + '">' + s.status + '</span></td>' +
                '<td>' + formatResponseTime(s.response_time) + '</td>' +
                '</tr>';
        }).join('');
    }

    function renderCustomServices(services) {
        var tbody = document.getElementById('customBody');
        var count = document.getElementById('customCount');
        if (!tbody) return;

        if (count) count.textContent = services.length + ' endpoint' + (services.length !== 1 ? 's' : '');

        if (services.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" style="color:var(--text-muted);padding:20px;text-align:center;">No custom endpoints configured</td></tr>';
            return;
        }

        tbody.innerHTML = services.map(function (s) {
            var isUp = s.status === 'UP';
            var displayUrl = s.url || s.port || '-';
            if (!s.url && s.port) displayUrl = 'localhost:' + s.port;
            var endpointId = s.endpoint_id || s.id || 0;
            return '<tr>' +
                '<td style="font-weight:600;color:var(--text-primary)">' + escapeHtml(s.name) + '</td>' +
                '<td style="font-family:var(--font-mono);color:var(--accent)">' + escapeHtml(displayUrl) + '</td>' +
                '<td><span class="badge ' + (isUp ? 'badge-up' : 'badge-down') + '">' + s.status + '</span></td>' +
                '<td>' + formatResponseTime(s.response_time) + '</td>' +
                '<td><button class="btn-delete" onclick="deleteEndpoint(' + endpointId + ')" title="Delete endpoint">' +
                '<svg viewBox="0 0 24 24" width="14" height="14"><polyline points="3 6 5 6 21 6" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>' +
                '</button></td>' +
                '</tr>';
        }).join('');
    }

    window.deleteEndpoint = function (id) {
        if (!id) return;
        if (!confirm('Delete this endpoint?')) return;
        fetch('/api/endpoints/' + id, { method: 'DELETE' })
            .then(function (r) {
                if (r.ok) {
                    showToast('Endpoint deleted', 'warning');
                } else {
                    showToast('Failed to delete endpoint', 'danger');
                }
            })
            .catch(function () {
                showToast('Failed to delete endpoint', 'danger');
            });
    };

    // --- Alerts ---
    function checkAlerts(data) {
        if (!data || !data.metrics || !data.services) return;

        if (data.metrics.disks) {
            data.metrics.disks.forEach(function (d) {
                if (d.percent > diskThreshold) {
                    showToast('DISK ALERT: ' + d.mount_point + ' at ' + d.percent.toFixed(1) + '%', 'warning');
                }
            });
        }

        if (data.metrics.cpu && data.metrics.cpu.percent > cpuThreshold) {
            showToast('CPU ALERT: ' + data.metrics.cpu.percent.toFixed(1) + '%', 'warning');
        }

        data.services.forEach(function (s) {
            if (s.status === 'DOWN') {
                showToast(s.name + ' on port ' + s.port + ' is DOWN', 'danger');
            }
        });
    }

    function showToast(message, type) {
        // Debounce per alert type to prevent notification floods
        var now = Date.now();
        if (lastAlertTimes[type] && (now - lastAlertTimes[type]) < 15000) return;
        lastAlertTimes[type] = now;

        var key = message;
        if (alertTimers[key]) return;
        alertTimers[key] = true;
        setTimeout(function () { delete alertTimers[key]; }, 60000);

        if ('Notification' in window && Notification.permission === 'granted') {
            new Notification('Velkron Pulse', { body: message });
        }

        var container = document.getElementById('toastContainer');
        if (!container) return;

        var toast = document.createElement('div');
        toast.className = 'toast toast-' + type;
        toast.textContent = message;
        container.appendChild(toast);

        setTimeout(function () {
            toast.style.opacity = '0';
            toast.style.transition = 'opacity 0.3s';
            setTimeout(function () { toast.remove(); }, 300);
        }, 5000);
    }

    // --- Tab Navigation ---
    function initTabs() {
        var tabs = document.querySelectorAll('.nav-item');
        var pageTitle = document.getElementById('pageTitle');
        var breadcrumb = document.getElementById('breadcrumbCurrent');

        var titles = {
            overview: 'SYSTEM OVERVIEW',
            services: 'SERVICE MONITOR',
            settings: 'SYSTEM SETTINGS'
        };

        tabs.forEach(function (tab) {
            tab.addEventListener('click', function () {
                var target = this.getAttribute('data-tab');

                tabs.forEach(function (t) { t.classList.remove('active'); });
                this.classList.add('active');

                document.querySelectorAll('.tab-content').forEach(function (tc) {
                    tc.classList.remove('active');
                });
                var targetTab = document.getElementById('tab-' + target);
                if (targetTab) targetTab.classList.add('active');

                if (pageTitle) pageTitle.textContent = titles[target] || 'SYSTEM OVERVIEW';
                if (breadcrumb) breadcrumb.textContent = (target || 'OVERVIEW').toUpperCase();
            });
        });
    }

    // --- System Clock ---
    function initClock() {
        function update() {
            var el = document.getElementById('sysTime');
            if (!el) return;
            var now = new Date();
            el.textContent = now.toTimeString().split(' ')[0];
        }
        update();
        setInterval(update, 1000);
    }

    // --- Settings ---
    function loadSettings() {
        fetch('/api/settings')
            .then(function (r) { return r.json(); })
            .then(function (settings) {
                if (settings.disk_threshold) {
                    diskThreshold = parseInt(settings.disk_threshold, 10);
                    var el = document.getElementById('diskThreshold');
                    if (el) el.value = diskThreshold;
                    localStorage.setItem('diskThreshold', diskThreshold);
                }
                if (settings.cpu_threshold) {
                    cpuThreshold = parseInt(settings.cpu_threshold, 10);
                    var el = document.getElementById('cpuThreshold');
                    if (el) el.value = cpuThreshold;
                    localStorage.setItem('cpuThreshold', cpuThreshold);
                }
            })
            .catch(function () {});
    }

    window.saveThresholds = function () {
        var diskVal = parseInt((document.getElementById('diskThreshold') || {}).value, 10) || 90;
        var cpuVal = parseInt((document.getElementById('cpuThreshold') || {}).value, 10) || 90;

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

        showToast('Thresholds saved', 'warning');
    };

    window.setEndpointType = function (type) {
        endpointType = type;
        document.querySelectorAll('.toggle-btn').forEach(function (btn) {
            btn.classList.toggle('active', btn.getAttribute('data-type') === type);
        });
    };

    window.addEndpoint = function () {
        var name = (document.getElementById('epName') || {}).value || '';
        var url = (document.getElementById('epUrl') || {}).value || '';

        if (!name.trim() || !url.trim()) {
            showToast('Name and URL are required', 'danger');
            return;
        }

        fetch('/api/endpoints', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: name.trim(), url: url.trim(), type: endpointType })
        })
            .then(function (r) {
                if (r.ok) {
                    showToast('Endpoint added', 'warning');
                    var epName = document.getElementById('epName');
                    var epUrl = document.getElementById('epUrl');
                    if (epName) epName.value = '';
                    if (epUrl) epUrl.value = '';
                } else {
                    showToast('Failed to add endpoint', 'danger');
                }
            })
            .catch(function () {
                showToast('Failed to add endpoint', 'danger');
            });
    };

    // --- Init ---
    function init() {
        if ('Notification' in window && Notification.permission === 'default') {
            Notification.requestPermission();
        }

        initParticles();
        initTabs();
        initClock();
        fetchInitial();
        connectWebSocket();
        loadSettings();

        // Refresh custom endpoints list
        setInterval(function () {
            fetch('/api/endpoints')
                .then(function (r) { return r.json(); })
                .then(function (endpoints) {
                    if (currentData && currentData.services) {
                        var known = currentData.services.filter(function (s) { return !s.is_custom; });
                        var custom = endpoints.map(function (ep) {
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
        }, CONFIG.customRefreshInterval);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
