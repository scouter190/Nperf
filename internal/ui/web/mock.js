'use strict';
// Simulación de la API para previsualizar y probar. NO se carga en producción.
(function () {
  const dir = function (mbps, extra) { return Object.assign({ status: 'measured', throughput_mbps: mbps, bytes: Math.round(mbps * 125000 * 10), duration_s: 10, retransmissions: null, udp: null }, extra); };
  const nm = { status: 'not_measured', throughput_mbps: null, bytes: null, duration_s: null, retransmissions: null, udp: null };
  const report = {
    format_version: 1, params: { host: '192.168.1.20', port: 5300, proto: 'both', direction: 'both', duration_s: 10, streams: 4, udp_rate_bps: null, version: 'test' },
    rtt: { status: 'measured', min_ms: 0.4, avg_ms: 0.7, max_ms: 1.9, samples: 20 },
    tcp: {
      fwd: dir(931.4, { retransmissions: { status: 'available', count: 12, estimated: true } }),
      rev: dir(905.2, { retransmissions: { status: 'unavailable', count: null, estimated: false } })
    },
    udp: {
      fwd: dir(838.1, { udp: { sent_mbps: 840, sent_bytes: 1050000000, target_rate_bps: 838e6, jitter_ms: 0.021, loss_pct: 0.03, datagrams_sent: 750000, datagrams_expected: 750000, datagrams_lost: 225 } }),
      rev: nm
    },
    notes: ['La retransmisión TCP en Windows es estimada.', 'UDP enviado al 98 % de la tasa objetivo.']
  };
  const diagnosis = {
    fwd: { color: 'green', percent: 93, text: 'Sin limitación detectada', partial: false },
    rev: { color: 'amber', percent: 61, text: 'Limitado por el WiFi de Portátil Ana (↑ 300 Mbps)', partial: true },
    local: { kind: 'eth', tx_bps: 1e9, rx_bps: 1e9 },
    remote: { kind: 'wifi', tx_bps: 3e8, rx_bps: 4.33e8, ssid: 'Oficina-5G' }
  };
  const view = {
    adapters: [
      { name: 'Ethernet', kind: 'eth', kind_label: 'Ethernet', connected: true, speed: '1 Gbps', description: 'Intel(R) Ethernet I219-V', mac: 'AA:BB:CC:00:11:22', ipv4: ['192.168.1.10'], tx_rate: '1 Gbps', rx_rate: '1 Gbps' },
      { name: 'Wi-Fi', kind: 'wifi', kind_label: 'WiFi', connected: true, speed: '↑ 300 Mbps / ↓ 433 Mbps', wifi: { available: true, ssid: 'Oficina-5G', band: '5 GHz', channel: '36', signal: '72 %', bssid: '11:22:33:44:55:66', rssi: '-55 dBm', standard: '802.11ac', tx_rate: '300 Mbps', rx_rate: '433 Mbps' }, description: 'Intel(R) Wi-Fi 6 AX200', mac: 'AA:BB:CC:00:11:33', ipv4: ['192.168.1.11'] },
      { name: 'Ethernet 2', kind: 'eth', kind_label: 'Ethernet', connected: false, speed: 'desconectada', description: 'USB GbE', mac: 'AA:BB:CC:00:11:44' }
    ],
    location_denied: false, location_notice: ''
  };
  const st = {
    self: { id: 'me', alias: 'PC-Oficina', ips: ['192.168.1.10'], status: 'available', status_label: 'disponible', queue: 0, link: 'eth', link_speed: '1 Gbps', ssid: '' },
    peers: [
      { id: 'p1', alias: 'Servidor', ips: ['192.168.1.20'], status: 'available', status_label: 'disponible', queue: 0, port: 5300, link: 'eth', link_speed: '1 Gbps' },
      { id: 'p2', alias: 'Portátil Ana', ips: ['192.168.1.31'], status: 'busy', status_label: 'ocupado', queue: 0, port: 5300, link: 'wifi', link_speed: '↑ 300 Mbps / ↓ 433 Mbps', ssid: 'Oficina-5G' },
      { id: 'p3', alias: 'Taller', ips: ['192.168.2.5'], status: 'queued', status_label: 'en cola 2', queue: 2, port: 5300, link: 'other', link_speed: '100 Mbps' },
      { id: 'p4', alias: 'Impresora vieja', ips: ['192.168.1.99'], status: 'unreachable', status_label: 'inalcanzable', queue: 0, port: 5300, link: 'unknown', link_speed: '' }
    ],
    selected_id: '', direction: 'both', directions: [], duration_seconds: 10, total_seconds: 20, total_label: '',
    alias: 'PC-Oficina', extra_subnets: ['192.168.2.0/24'], firewall_hint: '', has_key: false,
    test: { running: false, status: 'idle' }, can_test: false, test_blocked_reason: 'elija un equipo de destino', config_error: ''
  };
  let timers = [];
  function sync() {
    const p = st.peers.filter(function (x) { return x.id === st.selected_id; })[0];
    const pa = p ? p.alias : 'destino', me = st.alias;
    st.directions = [{ value: 'both', label: me + ' ⇄ ' + pa }, { value: 'fwd', label: me + ' → ' + pa }, { value: 'rev', label: pa + ' → ' + me }];
    st.total_seconds = st.direction === 'both' ? 2 * st.duration_seconds : st.duration_seconds;
    st.total_label = st.direction === 'both' ? st.duration_seconds + ' s por sentido, ' + st.total_seconds + ' s en total' : st.duration_seconds + ' s';
    st.self.alias = st.alias;
    if (st.test.running) st.can_test = false, st.test_blocked_reason = 'hay una prueba en curso';
    else if (!st.selected_id) st.can_test = false, st.test_blocked_reason = 'elija un equipo de destino';
    else st.can_test = true, st.test_blocked_reason = '';
  }
  function later(ms, fn) { timers.push(setTimeout(fn, ms)); }
  const api = {
    state: function () { sync(); return Promise.resolve(JSON.parse(JSON.stringify(st))); },
    selectPeer: function (id) {
      const p = st.peers.filter(function (x) { return x.id === id; })[0];
      if (!p || p.status === 'unreachable') return Promise.reject(new Error('equipo no seleccionable'));
      st.selected_id = id; return Promise.resolve();
    },
    setDirection: function (d) { st.direction = d; return Promise.resolve(); },
    setDuration: function (s) {
      if (s < 3 || s > 300) return Promise.reject(new Error('la duración debe estar entre 3 y 300 segundos'));
      st.duration_seconds = s; return Promise.resolve();
    },
    setAlias: function (a) { st.alias = a.trim() || 'PC-Oficina'; return Promise.resolve(); },
    setKey: function (k) { if (!k.trim()) return Promise.reject(new Error('la clave no puede estar vacía')); st.has_key = true; return Promise.resolve(); },
    addSubnet: function (s) { if (st.extra_subnets.indexOf(s) < 0) st.extra_subnets.push(s); return Promise.resolve(); },
    removeSubnet: function (s) { st.extra_subnets = st.extra_subnets.filter(function (x) { return x !== s; }); return Promise.resolve(); },
    startTest: function () {
      st.test = { running: true, status: 'queued', queue_position: 2 };
      later(1500, function () { st.test = { running: true, status: 'queued', queue_position: 1 }; });
      later(3000, function () { st.test = { running: true, status: 'running', phase: 'rtt' }; });
      later(4000, function () { st.test = { running: true, status: 'running', phase: 'tcp_fwd' }; });
      later(6000, function () { st.test = { status: 'done', running: false, report: report, diagnosis: diagnosis }; });
      return Promise.resolve();
    },
    adapters: function (detailed) {
      const v = JSON.parse(JSON.stringify(view));
      if (!detailed) v.adapters.forEach(function (a) { ['description', 'mac', 'ipv4', 'tx_rate', 'rx_rate'].forEach(function (k) { delete a[k]; }); if (a.wifi) ['bssid', 'rssi', 'standard', 'tx_rate', 'rx_rate'].forEach(function (k) { delete a.wifi[k]; }); });
      return Promise.resolve(v);
    },
    openLocationSettings: function () { view.location_denied = false; view.location_notice = ''; return Promise.resolve(); },
    cancelTest: function () { timers.forEach(clearTimeout); timers = []; st.test = { running: false, status: 'cancelled' }; return Promise.resolve(); }
  };
  window.nperf = api;
  // Control para pruebas: __mock.state es el estado mutable.
  window.__mock = { state: st, view: view, report: report, sync: sync };
})();
