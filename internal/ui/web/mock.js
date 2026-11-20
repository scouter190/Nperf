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
    history_enabled: true, internet_targets: ['1.1.1.1', '8.8.8.8'], history_warning: '', probe: { running: false, status: 'idle' }, can_probe: false, probe_blocked_reason: '',
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

  // ---- gráficas de ejemplo (forma de State.test.chart) ----
  // Muestras [t_s, dur_s, mbps, rtt_ms, loss_pct, jitter_ms]; rtt null = sonda sin respuesta.
  function mkSamples(phase, bloat) {
    const udp = phase.indexOf('udp') === 0, out = [];
    for (let i = 0; i < 10; i++) {
      const base = { tcp_fwd: 931, tcp_rev: 905, udp_fwd: 838 }[phase] || 800;
      const mbps = base + Math.round(Math.sin(i * 1.7 + phase.length) * 25 * 10) / 10 - (i === 0 && !udp ? 120 : 0);
      let rtt = bloat ? (phase.indexOf('fwd') > 0 ? 6 + i * 26 + (i % 3) * 7 : 8 + i * 3) : 4 + (i % 3);
      out.push([i, 1, mbps, i === 6 ? null : rtt, udp ? (i % 4 === 3 ? 0.8 : 0.05) : null, udp ? 0.4 + (i % 3) * 0.3 : null]);
    }
    return out;
  }
  function loaded(ss) {
    const v = ss.map(function (s) { return s[3]; }).filter(function (x) { return x !== null; }).sort(function (a, b) { return a - b; });
    const sum = v.reduce(function (a, b) { return a + b; }, 0);
    return { min_ms: v[0], avg_ms: sum / v.length, median_ms: v[Math.floor(v.length / 2)], max_ms: v[v.length - 1], samples: v.length, sent: ss.length };
  }
  const INIT = { min_ms: 2.1, avg_ms: 3.4, max_ms: 6.2 };
  function phaseOf(name, label, old, n) {
    if (old) return { phase: name, label: label, nominal_mbps: null, rtt_initial: null, samples: [], loaded_rtt: null, note: '', available: false };
    const ss = mkSamples(name, true).slice(0, n === undefined ? 10 : n);
    const l = ss.length ? loaded(ss) : null;
    const note = l && l.median_ms - INIT.avg_ms >= 100 && n === undefined && name === 'tcp_fwd' ? 'la latencia sube ' + Math.round(l.median_ms - INIT.avg_ms) + ' ms con carga en TCP ida: colas llenas en el enlace (bufferbloat)' : '';
    return { phase: name, label: label, nominal_mbps: 1000, rtt_initial: INIT, samples: ss, loaded_rtt: l, note: note, available: true };
  }
  function mkChart(peer, old, n) {
    const me = st.alias, ph = [['tcp_fwd', 'TCP ' + me + ' → ' + peer], ['tcp_rev', 'TCP ' + peer + ' → ' + me], ['udp_fwd', 'UDP ' + me + ' → ' + peer]];
    if (n !== undefined) ph.length = 1;
    return { phases: ph.map(function (x) { return phaseOf(x[0], x[1], old, n); }) };
  }
  // Monitor: 300 puntos (1 por segundo) por tarjeta física, hasta ahora.
  let monOn = false;
  function monitorView() {
    const now = Date.now(), mk = function (guid, name, kind, status, base) {
      const pts = [];
      if (status === 'up') for (let i = 299; i >= 0; i--) {
        const t = now - i * 1000, k = 300 - i, gap = k === 120;
        pts.push({ at_ms: t, in_bps: gap ? 0 : base * (0.15 + Math.max(0, Math.sin(k / 18)) * 0.8) + (k % 7) * base * 0.01, out_bps: gap ? 0 : base * 0.05 * (1 + Math.sin(k / 5)), gap: gap });
      }
      return { guid: guid, name: name, kind: kind, status: status, points: pts };
    };
    return { available: true, adapters: [mk('{1}', 'Ethernet', 'eth', 'up', 4e8), mk('{2}', 'Wi-Fi', 'wifi', 'up', 6e7), mk('{3}', 'Ethernet 2', 'eth', 'disconnected', 0)] };
  }

  // ---- historial de ejemplo (forma de internal/history.Record) ----
  const me = { alias: 'PC-Oficina', ip: '192.168.1.10', link: 'eth', nominal_mbps: 1000 };

  const stt = function (avg, loss, recv) { return { sent: 10, received: recv === undefined ? 10 : recv, loss_pct: loss || 0, min_ms: avg * 0.8, avg_ms: avg, max_ms: avg * 1.6, rtt_status: recv === 0 ? 'unavailable' : 'ok', jitter_ms: 1.2, jitter_status: recv === 0 ? 'unavailable' : 'ok' }; };
  const inetResult = { format_version: 1, command: 'internet', gateway: Object.assign({ ip: '192.168.1.1', status: 'ok' }, stt(1.1)),
    targets: [Object.assign({ ip: '1.1.1.1', status: 'ok', tcp443: true, tcp443_status: 'ok' }, stt(14.2)), Object.assign({ ip: '8.8.8.8', status: 'ok', tcp443: null, tcp443_status: 'unavailable' }, stt(52.8, 10, 9))],
    verdict: { kind: 'provider', message: 'El problema está después de su router: la red local responde bien pero internet es lenta.', notes: ['La pérdida hacia 8.8.8.8 es del 10 %.'] },
    trace: { target: '1.1.1.1', reached: true, hops: [{ ttl: 1, from: '192.168.1.1', status: 'ok' }, { ttl: 2, from: null, status: 'unavailable' }, { ttl: 3, from: '1.1.1.1', status: 'ok' }].map(function (h) { return Object.assign(h, stt(h.ttl * 5, h.from ? 0 : 100, h.from ? 10 : 0)); }) },
    trace_notes: ['El salto 2 no responde, pero los siguientes sí.'] };
  const speedResult = { format_version: 1, command: 'speed', time: new Date().toISOString(), provider: 'M-Lab NDT7', attribution: 'Velocidad medida con NDT7 de M-Lab (Measurement Lab), https://www.measurementlab.net/', tests_today: 3,
    server: { machine: 'mlab1-mad01', city: 'Madrid', country: 'ES' },
    download: { mbps: 487.2, bytes: 600000000, duration_s: 10, min_rtt_ms: 9.4, incomplete: false }, download_status: 'ok',
    upload: { mbps: 96.4, bytes: 70000000, duration_s: 7.5, min_rtt_ms: 9.1, incomplete: true, reason: 'terminó antes de 9 s', notes: ['Subida acortada por el servidor.'] }, upload_status: 'incomplete', upload_reason: 'terminó antes de 9 s' };
  const mkRec = function (id, iso, kind, status, dest, extra) { return Object.assign({ format_version: 1, id: id, time: iso, kind: kind, status: status, version: '0.6.0', source: me, dest: dest }, extra); };
  const srv = { alias: 'Servidor', ip: '192.168.1.20', link: 'eth', nominal_mbps: 1000 };
  const ana = { alias: 'Portátil Ana', ip: '192.168.1.31', link: 'wifi', ssid: 'Oficina-5G', nominal_mbps: 300 };
  const gw = { alias: 'Puerta de enlace', ip: '192.168.1.1' };
  const day = function (n, h) { const d = new Date(); d.setDate(d.getDate() - n); d.setHours(h, 12, 30, 0); return d.toISOString(); };
  let records = [
    mkRec('20260120-101230-aaaa0001', day(0, 10), 'test', 'done', srv, { params: report.params, result: report, diagnosis: diagnosis, chart: mkChart('Servidor', false), notes: report.notes }),
    mkRec('20260120-091230-aaaa0002', day(0, 9), 'ping', 'done', gw, { result: probes.ping, diagnosis: combined.ping }),
    mkRec('20260119-171230-aaaa0003', day(1, 17), 'test', 'cancelled', ana, { chart: Object.assign(mkChart('Portátil Ana', false, 4), { cancelled: true }) }),
    mkRec('20260119-161230-aaaa0004', day(1, 16), 'trace', 'done', { alias: 'dns.google', ip: '8.8.8.8' }, { result: probes.trace }),
    mkRec('20260118-101230-aaaa0005', day(2, 10), 'ports', 'done', gw, { result: probes.ports }),
    mkRec('20260118-091230-aaaa0006', day(2, 9), 'mtu', 'error', { alias: 'dns.google', ip: '8.8.8.8' }, { error: 'no se pudo abrir el socket ICMP' }),
    mkRec('20260117-101230-aaaa0007', day(3, 10), 'speed', 'done', { alias: 'M-Lab NDT7' }, { result: speedResult }),
    mkRec('20260117-091230-aaaa0008', day(3, 9), 'internet', 'done', { alias: 'Internet' }, { result: inetResult, diagnosis: { healthy: false, messages: [inetResult.verdict.message].concat(inetResult.verdict.notes, inetResult.trace_notes) } }),
    mkRec('20260116-101230-aaaa0009', day(4, 10), 'test', 'cancelled', srv, { result: { format_version: 1, params: report.params, tcp: { fwd: { status: 'measured', throughput_mbps: 900, bytes: 1, duration_s: 3, retransmissions: null, udp: null } }, notes: [] } })
  ];
  let timers = [];
  const internetCombined = { healthy: false, messages: [inetResult.verdict.message].concat(inetResult.verdict.notes, inetResult.trace_notes) };
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
      const p = st.peers.filter(function (x) { return x.id === st.selected_id; })[0];
      const peer = p ? p.alias : 'Servidor', old = !!p && p.id === 'p3'; // p3 simula un par antiguo
      st.test = { running: true, status: 'queued', queue_position: 2 };
      later(1500, function () { st.test = { running: true, status: 'queued', queue_position: 1 }; });
      later(3000, function () { st.test = { running: true, status: 'running', phase: 'rtt' }; });
      later(4000, function () { st.test = { running: true, status: 'running', phase: 'tcp_fwd', chart: mkChart(peer, old, 0) }; });
      for (let i = 1; i <= 10; i++) later(4000 + i * 400, function () { st.test = { running: true, status: 'running', phase: 'tcp_fwd', chart: mkChart(peer, old, i) }; });
      later(8600, function () { st.test = { status: 'done', running: false, report: report, diagnosis: diagnosis, chart: mkChart(peer, old) }; });
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
    monitor: function (active) { monOn = !!active; return Promise.resolve(active ? monitorView() : { available: true, adapters: [] }); },
    history: function (f) {
      const since = f.since ? Date.parse(f.since) : null, until = f.until ? Date.parse(f.until) : null, h = String(f.host || '').toLowerCase();
      const out = records.filter(function (r) {
        const t = Date.parse(r.time);
        if (f.kind && r.kind !== f.kind) return false;
        if (since !== null && t < since) return false;
        if (until !== null && t > until) return false;
        if (h && ![r.source, r.dest].some(function (e) { return e && ((e.alias || '').toLowerCase() === h || e.ip === h); })) return false;
        return true;
      });
      return Promise.resolve(JSON.parse(JSON.stringify({ records: out, skipped: 0 })));
    },
    historyRecord: function (id) {
      const r = records.filter(function (x) { return x.id === id; })[0];
      return r ? Promise.resolve(JSON.parse(JSON.stringify(r))) : Promise.reject(new Error('registro no encontrado'));
    },
    deleteHistory: function (ids) { const n = records.length; records = records.filter(function (r) { return ids.indexOf(r.id) < 0; }); return Promise.resolve(n - records.length); },
    deleteAllHistory: function () { const n = records.length; records = []; return Promise.resolve(n); },
    exportHistory: function (ids, format, opts) {
      if (window.__mockExport === 'cancel') return Promise.resolve('cancelado');
      if (window.__mockExport === 'error') return Promise.reject(new Error('no se pudo escribir el archivo'));
      const p = 'C:\\Users\\Ana\\Documents\\nperf-historial.' + (format === 'html' ? 'html' : format === 'json' ? 'json' : 'csv');
      return Promise.resolve(opts.intervals ? [p, p.replace('.csv', '-intervalos.csv')] : [p]);
    },
    setHistoryEnabled: function (on) { st.history_enabled = !!on; return Promise.resolve(); },
    startInternet: function () {
      if (st.test.running || st.probe.running) return Promise.reject(new Error('hay otra prueba en curso'));
      st.probe = { running: true, status: 'running', kind: 'internet' };
      later(1500, function () { st.probe = { running: false, status: 'done', kind: 'internet', result: inetResult, combined: internetCombined }; });
      return Promise.resolve();
    },
    startSpeed: function (consent) {
      if (consent !== true) return Promise.reject(new Error('hace falta aceptar el aviso de M-Lab en cada prueba de velocidad'));
      if (st.test.running || st.probe.running) return Promise.reject(new Error('hay otra prueba en curso'));
      window.__speedCalls = (window.__speedCalls || 0) + 1;
      st.probe = { running: true, status: 'running', kind: 'speed' };
      later(2000, function () { st.probe = { running: false, status: 'done', kind: 'speed', result: speedResult }; });
      return Promise.resolve();
    },
    openPrivacy: function (which) { if (which !== 'mlab' && which !== 'nperf') return Promise.reject(new Error('política no permitida')); window.__privacy = which; return Promise.resolve(); },
    setInternetTargets: function (list) {
      for (const t of list) {
        const m = /^(\d+)\.(\d+)\.(\d+)\.(\d+)$/.exec(t);
        if (!m) return Promise.reject(new Error('destino no válido: use una dirección IPv4 pública'));
        const a = +m[1], b = +m[2];
        if (a === 10 || a === 127 || (a === 192 && b === 168) || (a === 172 && b >= 16 && b <= 31)) return Promise.reject(new Error('destino no válido: ' + t + ' no es una dirección pública'));
      }
      if (list.length > 4) return Promise.reject(new Error('máximo 4 destinos'));
      st.internet_targets = list.length ? list.slice() : ['1.1.1.1', '8.8.8.8'];
      return Promise.resolve();
    },
    cancelTest: function () { timers.forEach(clearTimeout); timers = []; const ch = st.test.chart; if (ch) ch.cancelled = true; st.test = { running: false, status: 'cancelled', chart: ch }; return Promise.resolve(); }
  };
  window.nperf = api;
  // Control para pruebas: __mock.state es el estado mutable.
  window.__mock = { monitorOn: function () { return monOn; }, mkChart: mkChart, probes: probes, state: st, view: view, report: report, sync: sync };
})();
