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
      { name: 'Ethernet', kind: 'eth', kind_label: 'Ethernet', connected: true, virtual: false, speed: '1 Gbps', description: 'Intel(R) Ethernet I219-V', mac: 'AA:BB:CC:00:11:22', ipv4: ['192.168.1.10'], tx_rate: '1 Gbps', rx_rate: '1 Gbps' },
      { name: 'Wi-Fi', kind: 'wifi', kind_label: 'WiFi', connected: true, virtual: false, speed: '↑ 300 Mbps / ↓ 433 Mbps', wifi: { available: true, ssid: 'Oficina-5G', band: '5 GHz', channel: '36', signal: '72 %', bssid: '11:22:33:44:55:66', rssi: '-55 dBm', standard: '802.11ac', tx_rate: '300 Mbps', rx_rate: '433 Mbps' }, description: 'Intel(R) Wi-Fi 6 AX200', mac: 'AA:BB:CC:00:11:33', ipv4: ['192.168.1.11'] },
      { name: 'Ethernet 2', kind: 'eth', kind_label: 'Ethernet', connected: false, virtual: false, speed: 'desconectada', description: 'USB GbE', mac: 'AA:BB:CC:00:11:44' },
      { name: 'Tailscale', kind: 'other', kind_label: 'Otro', connected: true, virtual: true, speed: '(10 Gbps nominal)', description: 'Tailscale Tunnel', ipv4: ['100.64.0.5'], tx_rate: '10 Gbps', rx_rate: '10 Gbps' },
      { name: 'vEthernet (Default Switch)', kind: 'eth', kind_label: 'Ethernet', connected: true, virtual: true, speed: '(10 Gbps nominal)', description: 'Hyper-V Virtual Ethernet Adapter', ipv4: ['172.20.0.1'], tx_rate: '10 Gbps', rx_rate: '10 Gbps' },
      { name: 'Conexión de área local* 10', kind: 'wifi', kind_label: 'WiFi', connected: false, virtual: true, speed: 'desconectada', description: 'Microsoft Wi-Fi Direct Virtual Adapter' }
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
    no_agent: [
      { id: 'gateway', alias: 'Puerta de enlace (sin NPerf)', ips: ['192.168.1.1'], status: 'noagent', status_label: 'sin NPerf', queue: 0, link: 'unknown', link_speed: '' },
      { id: 'h1', alias: 'Router del taller', ips: ['192.168.2.1'], status: 'noagent', status_label: 'sin NPerf', queue: 0, link: 'unknown', link_speed: '' }
    ],
    probe: { running: false, status: 'idle' }, can_probe: false, probe_blocked_reason: '',
    selected_id: '', direction: 'both', directions: [], duration_seconds: 10, total_seconds: 20, total_label: '',
    alias: 'PC-Oficina', extra_subnets: ['192.168.2.0/24'], firewall_hint: '', has_key: false,
    test: { running: false, status: 'idle' }, can_test: false, test_blocked_reason: 'elija un equipo de destino', config_error: ''
  };
  const pr = function (ms) { return ms === null ? { status: 'timeout', rtt_ms: null } : { status: ms === 'ttl' ? 'ttl_expired' : 'ok', rtt_ms: ms }; };
  const hop = function (ttl, from, ms, loss) { return { ttl: ttl, from: from, status: from ? 'ok' : 'unavailable', probes: ms.map(pr), loss_pct: loss }; };
  const probes = {
    ping: { format_version: 1, command: 'ping', host: '192.168.1.1', ip: '192.168.1.1', sent: 10, received: 9, loss_pct: 10, min_ms: 0.6, avg_ms: 2.4, max_ms: 8.1, rtt_status: 'ok', jitter_ms: 1.3, jitter_status: 'ok',
      replies: [1, 2, 3].map(function (i) { return { seq: i, status: 'ok', from: '192.168.1.1', rtt_ms: i === 1 ? 0.6 : 2.2 * i }; }) },
    trace: { format_version: 1, command: 'trace', host: '8.8.8.8', ip: '8.8.8.8', reached: true, findings: ['El salto 3 no responde, pero los siguientes sí: probablemente filtra ICMP.', 'La latencia sube de 9 ms a 48 ms en el salto 4.'],
      hops: [hop(1, '192.168.1.1', [0.5, 0.7, 0.6], 0), hop(2, '10.20.0.1', [4.1, 3.8, 5.2], 0), hop(3, null, [null, null, null], 100), hop(4, '81.46.2.17', [48.3, 49.9, 47.2], 0), hop(5, '8.8.8.8', [51.2, null, 50.6], 33.3)] },
    mtu: { format_version: 1, command: 'mtu', host: '8.8.8.8', ip: '8.8.8.8', status: 'ok', mtu_bytes: 1492, max_payload_bytes: 1464, probes: 9, interface_mtu_bytes: 1500, notes: ['Posible agujero negro de MTU: hubo tiempos agotados sin aviso de fragmentación.'] },
    ports: { format_version: 1, command: 'ports', host: '192.168.1.1', ip: '192.168.1.1', ports: [{ port: 22, state: 'closed' }, { port: 53, state: 'open' }, { port: 80, state: 'open' }, { port: 443, state: 'filtered', detail: 'tiempo agotado' }, { port: 8291, state: 'open' }] }
  };
  const combined = {
    ping: { healthy: false, messages: ['pérdida/latencia en el tramo WiFi de este equipo (señal 48 %)'] },
    trace: { healthy: false, messages: ['MTU reducido en la ruta (1492 bytes): posibles cortes con paquetes grandes'] },
    mtu: { healthy: false, messages: ['MTU reducido en la ruta (1492 bytes): posibles cortes con paquetes grandes'] }
  };
  let timers = [];
  function sync() {
    const p = st.peers.concat(st.no_agent).filter(function (x) { return x.id === st.selected_id; })[0];
    const pa = p ? p.alias : 'destino', me = st.alias;
    st.directions = [{ value: 'both', label: me + ' ⇄ ' + pa }, { value: 'fwd', label: me + ' → ' + pa }, { value: 'rev', label: pa + ' → ' + me }];
    st.total_seconds = st.direction === 'both' ? 2 * st.duration_seconds : st.duration_seconds;
    st.total_label = st.direction === 'both' ? st.duration_seconds + ' s por sentido, ' + st.total_seconds + ' s en total' : st.duration_seconds + ' s';
    st.self.alias = st.alias;
    if (st.test.running) st.can_test = false, st.test_blocked_reason = 'hay una prueba en curso';
    else if (st.probe.running) st.can_test = false, st.test_blocked_reason = 'hay un diagnóstico en curso: espere a que termine para iniciar un TEST!';
    else if (!st.selected_id) st.can_test = false, st.test_blocked_reason = 'elija un equipo de destino';
    else if (st.no_agent.some(function (x) { return x.id === st.selected_id; })) st.can_test = false, st.test_blocked_reason = 'los equipos sin NPerf no admiten TEST!; use Ping, Traceroute, MTU o Puertos';
    else st.can_test = true, st.test_blocked_reason = '';
    st.can_probe = !st.test.running && !st.probe.running;
    st.probe_blocked_reason = st.test.running ? 'hay un TEST! en curso: espere a que termine para lanzar un diagnóstico' : (st.probe.running ? 'ya hay un diagnóstico en curso' : '');
  }
  function later(ms, fn) { timers.push(setTimeout(fn, ms)); }
  const api = {
    state: function () { sync(); return Promise.resolve(JSON.parse(JSON.stringify(st))); },
    selectPeer: function (id) {
      const p = st.peers.filter(function (x) { return x.id === id; })[0];
      if (st.no_agent.some(function (x) { return x.id === id; })) { st.selected_id = id; return Promise.resolve(); }
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
    addHost: function (target, alias) {
      target = String(target).trim();
      if (!target) return Promise.reject(new Error('indique una IP o un nombre de equipo'));
      if (!/^[A-Za-z0-9.-]+$/.test(target)) return Promise.reject(new Error('destino no válido: use una IP o un nombre de equipo'));
      if (st.no_agent.length >= 65) return Promise.reject(new Error('se alcanzó el máximo de 64 equipos añadidos a mano'));
      st.no_agent.push({ id: 'h' + Date.now(), alias: String(alias).trim() || target, ips: [target], status: 'noagent', status_label: 'sin NPerf', queue: 0, link: 'unknown', link_speed: '' });
      return Promise.resolve();
    },
    removeHost: function (id) {
      if (id === 'gateway') return Promise.reject(new Error('la puerta de enlace no se puede quitar'));
      st.no_agent = st.no_agent.filter(function (x) { return x.id !== id; });
      if (st.selected_id === id) st.selected_id = '';
      return Promise.resolve();
    },
    startProbe: function (kind, id) {
      if (st.test.running) return Promise.reject(new Error('hay un TEST! en curso: espere a que termine para lanzar un diagnóstico'));
      if (st.probe.running) return Promise.reject(new Error('ya hay un diagnóstico en curso'));
      if (!id) return Promise.reject(new Error('elija un equipo para el diagnóstico'));
      st.probe = { running: true, status: 'running', kind: kind, target_id: id };
      later(kind === 'ping' ? 1200 : 2500, function () { st.probe = { running: false, status: 'done', kind: kind, target_id: id, result: probes[kind], combined: combined[kind] }; });
      return Promise.resolve();
    },
    cancelProbe: function () {
      if (!st.probe.running) return Promise.resolve();
      timers.forEach(clearTimeout); timers = [];
      st.probe = { running: false, status: 'cancelled', kind: st.probe.kind, target_id: st.probe.target_id };
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
  window.__mock = { probes: probes, state: st, view: view, report: report, sync: sync };
})();
