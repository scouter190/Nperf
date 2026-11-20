'use strict';
// UI de NPerf. Todo dato del backend se inserta como texto (textContent / createTextNode).

// Puente con Go: T15 define window.nperfCall(secreto, nombre, ...args) -> Promise.
// El secreto de sesión lo entrega Go dentro del HTML servido (meta nperf-session).
// Para previsualizar, mock.js define window.nperf antes de cargar este archivo.
const NPERF_METHODS = ['state', 'selectPeer', 'setDirection', 'setDuration', 'setAlias', 'setKey', 'addSubnet', 'removeSubnet', 'startTest', 'cancelTest', 'adapters', 'openLocationSettings', 'addHost', 'removeHost', 'startProbe', 'cancelProbe', 'monitor', 'history', 'historyRecord', 'deleteHistory', 'deleteAllHistory', 'exportHistory', 'setHistoryEnabled', 'startInternet', 'startSpeed', 'openPrivacy', 'setInternetTargets'];
if (!window.nperf) {
  const api = {};
  NPERF_METHODS.forEach(function (m) {
    api[m] = function () {
      const args = Array.prototype.slice.call(arguments);
      if (typeof window.nperfCall !== 'function') return Promise.reject(new Error('puente no disponible'));
      const meta = document.querySelector('meta[name="nperf-session"]');
      const secret = meta ? meta.getAttribute('content') : '';
      return Promise.resolve(window.nperfCall.apply(null, [secret, m].concat(args)));
    };
  });
  window.nperf = api;
}
const nperf = window.nperf;

const $ = function (id) { return document.getElementById(id); };
const PHASES = [['rtt', 'RTT'], ['tcp_fwd', 'TCP ida'], ['tcp_rev', 'TCP vuelta'], ['udp_fwd', 'UDP ida'], ['udp_rev', 'UDP vuelta']];
let last = null;
let showProbe = false;
const sig = {};

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined && text !== null) e.textContent = String(text);
  return e;
}
function clear(node) { while (node.firstChild) node.removeChild(node.firstChild); }
// Solo vuelve a pintar si el contenido cambió (evita perder clics por repintar cada 500 ms).
function changed(key, value) {
  const s = JSON.stringify(value);
  if (sig[key] === s) return false;
  sig[key] = s;
  return true;
}
let toastTimer = null;
function toast(msg) {
  const t = $('toast');
  t.textContent = String(msg && msg.message ? msg.message : msg);
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(function () { t.hidden = true; }, 6000);
}
function act(p) { return Promise.resolve(p).then(poll, function (e) { toast(e); return poll(); }); }

const KINDS = { eth: 'Cable (Ethernet)', wifi: 'WiFi', other: 'Otro tipo de enlace', unknown: 'Tipo de enlace desconocido' };
function linkLine(d) {
  const parts = [];
  if (d.link_speed && d.link_speed !== 'no disponible') parts.push(d.link_speed);
  if (d.ssid) parts.push(d.ssid);
  return parts.join(' · ');
}

const GATEWAY_ID = 'gateway';
function deviceRow(d, selectable, selected, noAgent) {
  const gw = noAgent && d.id === GATEWAY_ID;
  const row = el('div', 'dev' + (selectable ? '' : ' self') + (selected ? ' sel' : '') + (d.status === 'unreachable' ? ' unreachable' : '') + (noAgent ? ' noagent' : '') + (gw ? ' gw' : ''));
  const kind = noAgent ? 'noagent' : (KINDS[d.link] ? d.link : 'unknown');
  const ico = el('span', 'ico ' + kind); ico.title = noAgent ? (gw ? 'Puerta de enlace (sin NPerf)' : 'Equipo sin NPerf') : KINDS[kind];
  row.appendChild(ico);
  const info = el('div', 'info');
  info.appendChild(el('span', 'name', d.alias));
  info.appendChild(el('span', 'ip', (d.ips || []).join(', ')));
  const ln = noAgent ? '' : linkLine(d);
  if (ln) info.appendChild(el('span', 'ip', ln));
  row.appendChild(info);
  row.appendChild(el('span', 'badge ' + d.status, d.status_label));
  if (noAgent && !gw) {
    const rm = el('button', 'rm', 'Quitar'); rm.type = 'button';
    rm.title = 'Quitar ' + d.alias + ' de la lista';
    rm.addEventListener('click', function (e) { e.stopPropagation(); act(nperf.removeHost(d.id)); });
    rm.addEventListener('keydown', function (e) { e.stopPropagation(); });
    row.appendChild(rm);
  }
  row.tabIndex = selectable && d.status !== 'unreachable' ? 0 : -1;
  if (selectable) {
    row.role = 'button';
    row.ariaDisabled = d.status === 'unreachable' ? 'true' : 'false';
    row.ariaPressed = selected ? 'true' : 'false';
  }
  row.title = d.alias + ' - ' + (d.ips || []).join(', ');
  if (selectable && d.status !== 'unreachable') {
    const pick = function () { act(nperf.selectPeer(d.id)); };
    row.addEventListener('click', pick);
    row.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pick(); }
    });
  }
  return row;
}

function renderList(s) {
  if (changed('self', s.self)) {
    const c = $('self'); clear(c); c.appendChild(deviceRow(s.self, false, false));
  }
  if (changed('peers', [s.peers, s.selected_id])) {
    const c = $('peers'); clear(c);
    s.peers.forEach(function (p) { c.appendChild(deviceRow(p, true, p.id === s.selected_id)); });
  }
  const na = s.no_agent || [];
  if (changed('noagent', [na, s.selected_id])) {
    const c = $('noagent'); clear(c);
    na.forEach(function (p) { c.appendChild(deviceRow(p, true, p.id === s.selected_id, true)); });
  }
  $('empty').hidden = s.peers.length > 0;
}

function selectedDevice(s) {
  const all = (s.peers || []).concat(s.no_agent || []);
  return all.filter(function (p) { return p.id === s.selected_id; })[0];
}
function isNoAgent(s, id) { return (s.no_agent || []).some(function (p) { return p.id === id; }); }

function renderBar(s) {
  if (changed('dirs', [s.directions, s.direction])) {
    const sel = $('direction'); clear(sel);
    s.directions.forEach(function (o) {
      const op = el('option', '', o.label); op.value = o.value; sel.appendChild(op);
    });
    sel.value = s.direction;
  }
  const dur = $('duration');
  if (document.activeElement !== dur) dur.value = s.duration_seconds;
  $('total').textContent = s.direction === 'both' ? s.total_label : '';
  const running = s.test.running;
  $('test').hidden = running || isNoAgent(s, s.selected_id);
  $('cancel').hidden = !running;
  $('cancel').disabled = s.test.status === 'cancelling';
  $('test').disabled = !s.can_test;
  $('test').title = s.can_test ? '' : s.test_blocked_reason;
  $('blocked').textContent = s.can_test || isNoAgent(s, s.selected_id) ? '' : s.test_blocked_reason;
  $('direction').disabled = running;
  dur.disabled = running;
  const ce = $('config-error'); ce.hidden = !s.config_error; ce.textContent = s.config_error || '';
  const fw = $('firewall'); fw.hidden = !s.firewall_hint; fw.textContent = s.firewall_hint || '';
}

function renderSettings(s) {
  if (document.activeElement !== $('alias')) $('alias').value = s.alias;
  $('key-state').textContent = s.has_key ? '(clave configurada: sí)' : '(clave configurada: no)';
  if (changed('subnets', s.extra_subnets)) {
    const ul = $('subnets'); clear(ul);
    s.extra_subnets.forEach(function (n) {
      const li = el('li'); li.appendChild(el('span', '', n));
      const b = el('button', '', 'Quitar'); b.type = 'button';
      b.addEventListener('click', function () { act(nperf.removeSubnet(n)); });
      li.appendChild(b); ul.appendChild(li);
    });
  }
}

function fmtBytes(n) {
  if (n === null || n === undefined) return '-';
  const u = ['B', 'KB', 'MB', 'GB', 'TB']; let v = Number(n), i = 0;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return v.toLocaleString('es', { maximumFractionDigits: i === 0 ? 0 : 2, minimumFractionDigits: i === 0 ? 0 : 2 }) + ' ' + u[i];
}
function fmtInt(n) { return Number(n).toLocaleString('es'); }
function fmt(v, d) { return (v === null || v === undefined) ? '-' : Number(v).toFixed(d === undefined ? 2 : d); }
function dirLabel(s, dir) {
  const o = s.directions.filter(function (x) { return x.value === dir; })[0];
  return o ? o.label : dir;
}

function progress(s) {
  const t = s.test;
  const box = el('div');
  const line = el('div', 'status-line');
  if (t.status === 'queued') line.textContent = t.queue_position > 0 ? 'En cola, posición ' + t.queue_position : 'Conectando…';
  else if (t.status === 'running') line.textContent = 'Midiendo…';
  else if (t.status === 'cancelling') line.textContent = t.label || 'Cancelando…';
  box.appendChild(line);
  const bar = el('div', 'progress');
  const idx = PHASES.map(function (p) { return p[0]; }).indexOf(t.phase);
  PHASES.forEach(function (p, i) {
    bar.appendChild(el('span', 'step' + (idx > i ? ' done' : '') + (idx === i && t.status === 'running' ? ' cur' : ''), p[1]));
  });
  box.appendChild(bar);
  return box;
}

function cell(tr, text, cls) { tr.appendChild(el('td', cls || '', text)); }

function retrans(r) {
  if (!r) return '-';
  if (r.status !== 'available') return 'no disponible';
  return (r.estimated ? '~' : '') + r.count;
}

function fmtBps(n) {
  if (!n || n <= 0) return null;
  const u = ['bps', 'Kbps', 'Mbps', 'Gbps', 'Tbps']; let v = Number(n), i = 0;
  while (v >= 1000 && i < u.length - 1) { v /= 1000; i++; }
  return v.toLocaleString('es', { maximumFractionDigits: 1 }) + ' ' + u[i];
}
function linkText(l) {
  if (!l) return 'enlace desconocido';
  const parts = [KINDS[l.kind] || KINDS.unknown];
  const tx = fmtBps(l.tx_bps), rx = fmtBps(l.rx_bps);
  if (tx && rx && tx !== rx) parts.push('↑ ' + tx + ' / ↓ ' + rx); else if (tx || rx) parts.push(tx || rx);
  if (l.ssid) parts.push(l.ssid);
  return parts.join(' · ');
}
const SEMA = { green: 'Sin limitación detectada', amber: 'Limitación moderada', red: 'Limitación fuerte' };
function diagnosisView(s) {
  const dg = s.test.diagnosis;
  const peer = selectedDevice(s);
  const box = el('div', 'diag');
  box.appendChild(el('h3', '', 'Diagnóstico'));
  [['fwd', dg.fwd], ['rev', dg.rev]].forEach(function (x) {
    if (!x[1]) return;
    const d = x[1];
    const color = SEMA[d.color] ? d.color : 'none';
    const row = el('div', 'dg-row');
    const dot = el('span', 'dot ' + color); dot.title = SEMA[color] || 'Sin datos';
    row.appendChild(dot);
    row.appendChild(el('span', 'dg-dir', dirLabel(s, x[0])));
    row.appendChild(el('span', 'dg-pct', color === 'none' ? 'sin datos' : d.percent + ' %'));
    const txt = el('span', 'dg-text', d.text || (SEMA[color] || ''));
    row.appendChild(txt);
    if (d.partial) { const p = el('span', 'badge partial', 'parcial'); p.title = 'Diagnóstico parcial: falta información de un extremo'; row.appendChild(p); }
    box.appendChild(row);
  });
  box.appendChild(el('p', 'muted dg-links', s.self.alias + ': ' + linkText(dg.local) + ' · ' + (peer ? peer.alias : 'destino') + ': ' + linkText(dg.remote)));
  return box;
}

function reportView(s, detailed) {
  const r = s.test.report;
  const box = el('div');
  const rtt = r.rtt || { status: 'not_measured' };
  const dgn = s.test.diagnosis;
  if (dgn && (dgn.fwd || dgn.rev)) box.appendChild(diagnosisView(s));
  box.appendChild(el('h3', '', 'RTT'));
  if (rtt.status !== 'measured') box.appendChild(el('p', 'muted', rtt.status === 'not_measured' ? 'no medido' : 'no disponible'));
  else {
    box.appendChild(el('p', '', 'mín ' + fmt(rtt.min_ms) + ' ms · media ' + fmt(rtt.avg_ms) + ' ms · máx ' + fmt(rtt.max_ms) + ' ms'));
  }
  ['tcp', 'udp'].forEach(function (proto) {
    box.appendChild(el('h3', '', proto.toUpperCase()));
    const t = el('table');
    const hr = el('tr');
    const cols = ['Sentido', 'Velocidad (Mbps)'];
    if (proto === 'tcp') cols.push('Retransmisiones'); else cols.push('Jitter (ms)', 'Pérdida (%)');
    if (detailed) cols.push(proto === 'tcp' ? 'Bytes' : 'Bytes / datagramas');
    cols.forEach(function (c) { hr.appendChild(el('th', '', c)); });
    t.appendChild(hr);
    ['fwd', 'rev'].forEach(function (dir) {
      const d = (r[proto] || {})[dir] || { status: 'not_measured' };
      const tr = el('tr');
      cell(tr, dirLabel(s, dir));
      if (d.status !== 'measured') {
        const c = el('td', 'muted', d.status === 'not_measured' ? 'no medido' : 'no disponible');
        c.colSpan = cols.length - 1; tr.appendChild(c);
      } else {
        cell(tr, fmt(d.throughput_mbps), 'num');
        if (proto === 'tcp') cell(tr, retrans(d.retransmissions), 'num');
        else { cell(tr, d.udp ? fmt(d.udp.jitter_ms, 3) : '-', 'num'); cell(tr, d.udp ? fmt(d.udp.loss_pct) : '-', 'num'); }
        if (detailed) {
          let txt = fmtBytes(d.bytes);
          if (proto === 'udp' && d.udp) txt += ' / enviados ' + fmtInt(d.udp.datagrams_sent) + ', esperados ' + fmtInt(d.udp.datagrams_expected) + ', perdidos ' + fmtInt(d.udp.datagrams_lost);
          cell(tr, txt, 'num');
        }
      }
      t.appendChild(tr);
    });
    box.appendChild(t);
  });
  if (detailed && Array.isArray(r.notes) && r.notes.length) {
    const ul = el('ul', 'notes');
    r.notes.forEach(function (n) { ul.appendChild(el('li', '', n)); });
    box.appendChild(ul);
  }
  return box;
}

// ---- diagnósticos sin agente ----
const PROBES = [['ping', 'Ping', 'probe-ping'], ['trace', 'Traceroute', 'probe-trace'], ['mtu', 'MTU', 'probe-mtu'], ['ports', 'Puertos', 'probe-ports']];
const PROBE_NAMES = { ping: 'Ping', trace: 'Traceroute', mtu: 'MTU de la ruta', ports: 'Puertos TCP', internet: 'Latencia a internet', speed: 'Velocidad a internet (M-Lab)' };
const PORT_STATES = { open: 'abierto', closed: 'cerrado', filtered: 'filtrado', canceled: 'cancelado', cancelled: 'cancelado' };
const REPLY_STATES = { ok: 'respuesta', timeout: 'tiempo agotado', ttl_expired: 'TTL expirado', host_unreachable: 'host inaccesible', net_unreachable: 'red inaccesible', port_unreachable: 'puerto inaccesible', frag_needed: 'fragmentación necesaria', other: 'sin respuesta' };

function fmtMs(v) {
  if (v === null || v === undefined) return 'no disponible';
  return Number(v) < 1 ? '< 1 ms' : Number(v).toFixed(1) + ' ms';
}
function kvTable(rows) {
  const t = el('table', 'kv-table');
  rows.forEach(function (r) {
    const tr = el('tr'); tr.appendChild(el('th', '', r[0])); tr.appendChild(el('td', 'num', r[1])); t.appendChild(tr);
  });
  return t;
}
function listOf(cls, items) {
  const ul = el('ul', cls);
  (items || []).forEach(function (n) { ul.appendChild(el('li', '', n)); });
  return ul;
}
function pingView(r, detailed) {
  const box = el('div');
  const ok = r.rtt_status === 'ok';
  box.appendChild(kvTable([
    ['Enviados', r.sent], ['Recibidos', r.received], ['Pérdida', fmt(r.loss_pct, 1) + ' %'],
    ['Mín', ok ? fmtMs(r.min_ms) : 'no disponible'], ['Media', ok ? fmtMs(r.avg_ms) : 'no disponible'], ['Máx', ok ? fmtMs(r.max_ms) : 'no disponible'],
    ['Jitter', r.jitter_status === 'ok' ? fmtMs(r.jitter_ms) : 'no disponible']
  ]));
  if (r.last_error) box.appendChild(el('p', 'muted', 'Último error: ' + r.last_error));
  if (detailed && r.replies && r.replies.length) {
    const t = el('table'); const hr = el('tr');
    ['N.º', 'Resultado', 'Desde', 'Tiempo'].forEach(function (c) { hr.appendChild(el('th', '', c)); });
    t.appendChild(hr);
    r.replies.forEach(function (p) {
      const tr = el('tr');
      cell(tr, p.seq, 'num'); cell(tr, REPLY_STATES[p.status] || p.status); cell(tr, p.from || '-');
      cell(tr, p.rtt_ms === null || p.rtt_ms === undefined ? (p.reason || '*') : fmtMs(p.rtt_ms), 'num');
      t.appendChild(tr);
    });
    box.appendChild(t);
  }
  return box;
}
function traceView(r) {
  const box = el('div');
  box.appendChild(el('p', r.reached ? 'status-line' : 'status-line err-text', r.reached ? 'Destino alcanzado.' : 'Destino no alcanzado.'));
  const t = el('table', 'trace');
  const hr = el('tr');
  const n = (r.hops || []).reduce(function (m, h) { return Math.max(m, (h.probes || []).length); }, 0);
  const cols = ['Salto', 'Equipo'];
  for (let i = 1; i <= n; i++) cols.push('Sonda ' + i);
  cols.push('Pérdida');
  cols.forEach(function (c) { hr.appendChild(el('th', '', c)); });
  t.appendChild(hr);
  (r.hops || []).forEach(function (h) {
    const tr = el('tr', h.from ? '' : 'silent');
    cell(tr, h.ttl, 'num'); cell(tr, h.from || '*');
    for (let i = 0; i < n; i++) {
      const p = (h.probes || [])[i];
      cell(tr, p && p.rtt_ms !== null && p.rtt_ms !== undefined ? fmtMs(p.rtt_ms) : '*', 'num');
    }
    cell(tr, fmt(h.loss_pct, 0) + ' %', 'num');
    t.appendChild(tr);
  });
  box.appendChild(t);
  if (r.findings && r.findings.length) { box.appendChild(el('h3', '', 'Análisis')); box.appendChild(listOf('findings', r.findings)); }
  return box;
}
function mtuView(r) {
  const box = el('div');
  if (r.status !== 'ok') box.appendChild(el('p', 'err-text', 'MTU no disponible: ' + (r.reason || 'sin motivo')));
  else box.appendChild(kvTable([['MTU de la ruta', r.mtu_bytes + ' bytes'], ['Carga ICMP máxima', r.max_payload_bytes + ' bytes']]));
  box.appendChild(el('p', 'muted', 'Sondas enviadas: ' + r.probes + (r.interface_mtu_bytes ? ' · MTU de la interfaz local: ' + r.interface_mtu_bytes + ' bytes' : '')));
  if (r.notes && r.notes.length) box.appendChild(listOf('notes warn-notes', r.notes));
  return box;
}
function portsView(r) {
  const t = el('table');
  const hr = el('tr');
  ['Puerto', 'Estado', 'Detalle'].forEach(function (c) { hr.appendChild(el('th', '', c)); });
  t.appendChild(hr);
  (r.ports || []).forEach(function (p) {
    const tr = el('tr');
    cell(tr, p.port, 'num');
    const st = PORT_STATES[p.state] ? p.state : 'error';
    tr.appendChild(el('td', 'port ' + st, PORT_STATES[p.state] || 'error'));
    cell(tr, p.detail || '');
    t.appendChild(tr);
  });
  return t;
}
function combinedView(c) {
  const box = el('div', 'combined ' + (c.healthy ? 'healthy' : 'problem'));
  box.appendChild(el('h3', '', 'Diagnóstico combinado'));
  (c.messages || []).forEach(function (m) { box.appendChild(el('p', 'cmb-msg', m)); });
  return box;
}
function probeDetail(s, c) {
  const p = s.probe;
  const dev = (s.peers || []).concat(s.no_agent || []).filter(function (x) { return x.id === p.target_id; })[0];
  c.appendChild(el('p', 'status-line', (PROBE_NAMES[p.kind] || 'Diagnóstico') + (dev ? ' a ' + dev.alias + ' (' + (dev.ips || []).join(', ') + ')' : '')));
  if (p.status === 'running') c.appendChild(el('p', 'muted', 'Ejecutando…'));
  else if (p.status === 'cancelling') c.appendChild(el('p', 'muted', p.label || 'Cancelando…'));
  else if (p.status === 'cancelled') c.appendChild(el('p', 'muted', 'Diagnóstico cancelado.'));
  else if (p.status === 'error') c.appendChild(el('p', 'err-text', 'Error: ' + p.error));
  else if (p.status === 'done' && p.result) {
    if (p.kind === 'internet') { c.appendChild(internetView(p.result, p.combined)); return; }
    if (p.kind === 'speed') { c.appendChild(speedView(p.result)); return; }
    if (p.combined && p.combined.messages && p.combined.messages.length) c.appendChild(combinedView(p.combined));
    const detailed = $('detailed').checked;
    if (p.kind === 'ping') c.appendChild(pingView(p.result, detailed));
    else if (p.kind === 'trace') c.appendChild(traceView(p.result));
    else if (p.kind === 'mtu') c.appendChild(mtuView(p.result));
    else if (p.kind === 'ports') c.appendChild(portsView(p.result));
  }
}

function renderProbeBar(s) {
  const p = s.probe || { running: false, status: 'idle' };
  const hasSel = !!selectedDevice(s);
  PROBES.forEach(function (x) {
    const b = $(x[2]);
    const own = p.running && p.kind === x[0];
    b.textContent = own ? (p.status === 'cancelling' ? 'Cancelando…' : 'Cancelar') : x[1];
    b.classList.toggle('danger', own);
    b.disabled = own ? p.status === 'cancelling' : (!hasSel || !s.can_probe);
    b.title = own ? '' : (!hasSel ? 'elija un equipo de destino' : (s.can_probe ? '' : s.probe_blocked_reason));
  });
  $('probe-blocked').textContent = !s.can_probe && !p.running ? s.probe_blocked_reason : (p.running ? '' : (hasSel ? '' : 'elija un equipo de destino'));
}

function startProbe(kind) {
  if (!last) return;
  showProbe = true; delete sig.detail;
  act(nperf.startProbe(kind, last.selected_id));
}

function testNoChart(t) {
  const c = Object.assign({}, t); delete c.chart; return c;
}

function renderDetail(s) {
  const detailed = $('detailed').checked;
  document.body.classList.toggle('compact', !detailed);
  if (s.probe && s.probe.running) showProbe = true;
  if (s.test && s.test.running) showProbe = false;
  if (!changed('detail', [testNoChart(s.test), s.probe, showProbe, s.directions, detailed, s.selected_id, s.no_agent])) return;
  const c = $('detail'); clear(c);
  const t = s.test;
  const peer = selectedDevice(s);
  if (showProbe && s.probe && s.probe.status !== 'idle') { probeDetail(s, c); return; }
  c.appendChild(el('p', 'status-line', peer ? 'Destino: ' + peer.alias + ' (' + (peer.ips || []).join(', ') + ')' : 'Sin destino seleccionado'));
  if (t.status === 'queued' || t.status === 'running' || t.status === 'cancelling') c.appendChild(progress(s));
  else if (t.status === 'error') c.appendChild(el('p', 'err-text', 'Error: ' + t.error));
  else if (t.status === 'cancelled') c.appendChild(el('p', 'muted', 'Prueba cancelada.'));
  else if (t.status === 'done' && t.report) c.appendChild(reportView(s, detailed));
  else c.appendChild(el('p', 'muted', peer && isNoAgent(s, peer.id) ? 'Equipo sin NPerf: use Ping, Traceroute, MTU o Puertos.' : 'Elija un equipo y pulse TEST! o un diagnóstico.'));
}

function adapterCard(a, detailed) {
  const card = el('div', 'card' + (a.connected ? '' : ' off') + (a.virtual === true ? ' virtual' : ''));
  const head = el('div', 'card-head');
  const kind = KINDS[a.kind] ? a.kind : 'unknown';
  const ico = el('span', 'ico ' + kind); ico.title = a.kind_label || KINDS[kind];
  head.appendChild(ico);
  head.appendChild(el('span', 'name', a.name));
  if (a.virtual === true) { const tag = el('span', 'tag-virtual', 'virtual'); tag.title = 'Adaptador virtual (VPN, máquina virtual...): su velocidad es nominal'; head.appendChild(tag); }
  head.appendChild(el('span', 'card-speed', a.speed));
  card.appendChild(head);
  const kv = function (k, v) { if (v) card.appendChild(el('div', 'kv', k + ': ' + v)); };
  const w = a.wifi;
  if (w) {
    if (!w.available) kv('WiFi', w.reason || 'no disponible');
    else {
      kv('SSID', w.ssid); kv('Banda', w.band); kv('Canal', w.channel); kv('Señal', w.signal);
      if (detailed) { kv('BSSID', w.bssid); kv('RSSI', w.rssi); kv('Estándar', w.standard); kv('Tasa ↑', w.tx_rate); kv('Tasa ↓', w.rx_rate); }
    }
  }
  if (detailed) {
    kv('Descripción', a.description); kv('MAC', a.mac); kv('IPv4', (a.ipv4 || []).join(', '));
    if (!w) { kv('Tasa ↑', a.tx_rate); kv('Tasa ↓', a.rx_rate); }
  }
  return card;
}

function renderAdapters(v, detailed) {
  if (!changed('adapters', [v, detailed])) return;
  const c = $('adapters'); clear(c);
  const n = $('location-notice');
  n.hidden = !v.location_denied;
  $('location-text').textContent = v.location_notice || '';
  if (!v.adapters || !v.adapters.length) c.appendChild(el('p', 'muted', 'No se detectaron tarjetas de red.'));
  (v.adapters || []).forEach(function (a) { c.appendChild(adapterCard(a, detailed)); });
}

let adaptersBusy = false;
function refreshAdapters() {
  if (!$('adapters-box').open || adaptersBusy) return;
  adaptersBusy = true;
  const detailed = $('detailed').checked;
  Promise.resolve().then(function () { return nperf.adapters(detailed); })
    .then(function (v) { renderAdapters(v, detailed); }, function (e) { $('adapters').textContent = String(e && e.message ? e.message : e); delete sig.adapters; })
    .then(function () { adaptersBusy = false; });
}

// ---- gráficas del TEST! y monitor de tarjetas (NPerfChart) ----
const CHART_MIN_MS = 1000; // redibujo como máximo 1 vez por segundo
const openData = {};        // clave de gráfica -> "Valores" abierto (se conserva al redibujar)
let chartDrawn = 0, chartPending = false;

// Dibuja una gráfica en un contenedor propio y conserva el estado del desplegable de valores.
function drawChart(parent, key, opts) {
  const box = el('div', 'chart-box');
  parent.appendChild(box);
  const w = window.NPerfChart.render(box, opts);
  const det = w.querySelector('.chart-data');
  if (det) {
    det.open = !!openData[key];
    det.addEventListener('toggle', function () { openData[key] = det.open; });
  }
}
function pts(samples, idx, endOfInterval) {
  return (samples || []).map(function (x) {
    const t = x[0] === null ? null : (endOfInterval ? x[0] + (x[1] || 0) : x[0]);
    return [t, x[idx]];
  });
}
function chartBlock(p, cancelled) {
  const box = el('section', 'chart-phase');
  box.appendChild(el('h3', '', p.label));
  if (!p.available) {
    box.appendChild(el('p', 'muted chart-na', 'gráfica no disponible: el otro equipo no envía muestras'));
    return box;
  }
  const udp = p.phase.indexOf('udp') === 0;
  const samples = p.samples || [];
  if (!samples.length) { box.appendChild(el('p', 'muted', cancelled ? 'Cancelado antes de recibir muestras.' : 'Esperando muestras…')); return box; }
  const refs = [];
  if (typeof p.nominal_mbps === 'number') refs.push({ y: p.nominal_mbps, label: 'nominal ' + Math.round(p.nominal_mbps) + ' Mbps' });
  drawChart(box, p.phase + ':v', { title: 'Velocidad · ' + p.label, xLabel: 'Tiempo', xUnit: 's', yUnit: 'Mbps', decimals: 1,
    series: [{ name: 'Velocidad', points: pts(samples, 2, true) }], refs: refs });
  if (udp) {
    drawChart(box, p.phase + ':l', { title: 'Pérdida · ' + p.label, xLabel: 'Tiempo', xUnit: 's', yUnit: '%', decimals: 2,
      series: [{ name: 'Pérdida', points: pts(samples, 4, true) }] });
    drawChart(box, p.phase + ':j', { title: 'Jitter · ' + p.label, xLabel: 'Tiempo', xUnit: 's', yUnit: 'ms', decimals: 3,
      series: [{ name: 'Jitter', points: pts(samples, 5, true) }] });
  }
  const lr = p.loaded_rtt, ri = p.rtt_initial;
  if (samples.some(function (x) { return x[3] !== null && x[3] !== undefined; })) {
    const lrefs = [];
    if (ri && typeof ri.avg_ms === 'number') lrefs.push({ y: ri.avg_ms, label: 'RTT inicial ' + fmt(ri.avg_ms, 1) + ' ms' });
    drawChart(box, p.phase + ':r', { title: 'Latencia bajo carga · ' + p.label, xLabel: 'Tiempo', xUnit: 's', yUnit: 'ms', decimals: 1,
      series: [{ name: 'RTT bajo carga', points: pts(samples, 3, true) }], refs: lrefs });
    if (lr) box.appendChild(el('p', 'muted chart-loaded', 'RTT bajo carga: mín ' + fmt(lr.min_ms, 1) + ' ms · mediana ' + fmt(lr.median_ms, 1) + ' ms · media ' + fmt(lr.avg_ms, 1) + ' ms · máx ' + fmt(lr.max_ms, 1) + ' ms (' + lr.samples + ' de ' + lr.sent + ' sondas)'));
  } else {
    box.appendChild(el('p', 'muted', 'Sin sondas de latencia respondidas.'));
  }
  if (p.note) box.appendChild(el('p', 'chart-note', 'Aviso: ' + p.note));
  return box;
}

function renderCharts(s) {
  const c = $('charts');
  const ch = s.test && s.test.chart;
  if (showProbe && s.probe && s.probe.status !== 'idle') { if (changed('charts', 'probe')) clear(c); return; }
  if (!ch || !ch.phases || !ch.phases.length) { if (changed('charts', 'none')) clear(c); return; }
  if (!changed('charts', [ch, s.test.status])) return;
  const now = Date.now();
  if (now - chartDrawn < CHART_MIN_MS) {
    // Demasiado pronto: se reintenta con el siguiente sondeo (la firma se invalida).
    delete sig.charts;
    if (!chartPending) { chartPending = true; setTimeout(function () { chartPending = false; if (last) renderCharts(last); }, CHART_MIN_MS - (now - chartDrawn)); }
    return;
  }
  chartDrawn = now;
  clear(c);
  const h = el('h2', '', 'Gráficas del TEST!');
  c.appendChild(h);
  if (ch.cancelled || s.test.status === 'cancelled') c.appendChild(el('p', 'muted chart-cancelled', 'Cancelado: se muestran las muestras recibidas antes de cancelar.'));
  ch.phases.forEach(function (p) { c.appendChild(chartBlock(p, ch.cancelled)); });
}

let monitorBusy = false, monitorOn = false;
function stopMonitor() {
  if (!monitorOn) return;
  monitorOn = false;
  Promise.resolve().then(function () { return nperf.monitor(false); }).then(null, function () {});
}
function monitorPoints(a, key, last) {
  const out = [];
  (a.points || []).forEach(function (p, i) {
    const x = (p.at_ms - last) / 1000;
    if (p.gap && i > 0) out.push([x - 0.001, null]);
    out.push([x, p.gap ? null : p[key] / 1e6]);
  });
  return out;
}
function renderMonitor(v) {
  const msg = $('monitor-msg');
  msg.hidden = v.available;
  msg.textContent = v.available ? '' : (v.message || 'el monitor de tarjetas no está disponible');
  if (!changed('monitor', v)) return;
  const c = $('monitor'); clear(c);
  let latest = 0;
  (v.adapters || []).forEach(function (a) { (a.points || []).forEach(function (p) { if (p.at_ms > latest) latest = p.at_ms; }); });
  (v.adapters || []).forEach(function (a) {
    const box = el('div', 'mon-adapter');
    const hd = el('div', 'card-head');
    const kind = KINDS[a.kind] ? a.kind : 'unknown';
    const ico = el('span', 'ico ' + kind); ico.title = KINDS[kind];
    hd.appendChild(ico);
    hd.appendChild(el('span', 'name', a.name));
    hd.appendChild(el('span', 'card-speed', a.status === 'up' ? 'conectada' : 'desconectada'));
    box.appendChild(hd);
    if (!a.points || !a.points.length) box.appendChild(el('p', 'muted', 'Sin datos todavía.'));
    else drawChart(box, 'mon:' + a.guid, { title: 'Tráfico de ' + a.name, xLabel: 'Hace', xUnit: 's', yUnit: 'Mbps', decimals: 2, width: 300, height: 190,
      series: [{ name: 'Entrada', points: monitorPoints(a, 'in_bps', latest) }, { name: 'Salida', points: monitorPoints(a, 'out_bps', latest) }] });
    c.appendChild(box);
  });
  if (!(v.adapters || []).length && v.available) c.appendChild(el('p', 'muted', 'No hay tarjetas físicas que monitorizar.'));
}
// Solo con el panel abierto y la ventana visible; si no, se detiene el muestreo.
function refreshMonitor() {
  if (!$('adapters-box').open || document.hidden) { stopMonitor(); return; }
  if (monitorBusy) return;
  monitorBusy = true; monitorOn = true;
  Promise.resolve().then(function () { return nperf.monitor(true); })
    .then(function (v) { if ($('adapters-box').open && !document.hidden) renderMonitor(v); },
      function (e) { $('monitor-msg').hidden = false; $('monitor-msg').textContent = String(e && e.message ? e.message : e); delete sig.monitor; })
    .then(function () { monitorBusy = false; });
}

function render(s) {
  last = s;
  renderBar(s); renderProbeBar(s); renderList(s); renderSettings(s); renderDetail(s); renderCharts(s);
  if (typeof renderHistoryState === 'function') renderHistoryState(s);
  if (typeof renderInternetState === 'function') renderInternetState(s);
}

let polling = false;
function poll() {
  if (polling) return Promise.resolve();
  polling = true;
  return Promise.resolve().then(function () { return nperf.state(); }).then(render, function (e) { toast(e); })
    .then(function () { polling = false; }, function () { polling = false; });
}
function loop() { poll().then(function () { setTimeout(loop, 500); }); }

function commitDuration() {
  const v = parseInt($('duration').value, 10);
  if (isNaN(v) || v < 3 || v > 300) { toast('La duración debe estar entre 3 y 300 segundos'); if (last) $('duration').value = last.duration_seconds; return; }
  act(nperf.setDuration(v));
}

document.addEventListener('DOMContentLoaded', function () {
  $('direction').addEventListener('change', function () { act(nperf.setDirection($('direction').value)); });
  $('duration').addEventListener('change', commitDuration);
  $('test').addEventListener('click', function () { showProbe = false; delete sig.detail; act(nperf.startTest()); });
  PROBES.forEach(function (x) {
    $(x[2]).addEventListener('click', function () {
      if (last && last.probe && last.probe.running && last.probe.kind === x[0]) act(nperf.cancelProbe());
      else startProbe(x[0]);
    });
  });
  $('host-form').addEventListener('submit', function (e) {
    e.preventDefault();
    const err = $('host-error'); err.hidden = true;
    Promise.resolve().then(function () { return nperf.addHost($('host-target').value, $('host-alias').value); })
      .then(function () { $('host-target').value = ''; $('host-alias').value = ''; return poll(); },
        function (x) { err.textContent = String(x && x.message ? x.message : x); err.hidden = false; });
  });
  $('cancel').addEventListener('click', function () { act(nperf.cancelTest()); });
  $('detailed').addEventListener('change', function () { delete sig.detail; if (last) renderDetail(last); refreshAdapters(); });
  $('adapters-box').addEventListener('toggle', function () { refreshAdapters(); if ($('adapters-box').open) refreshMonitor(); else stopMonitor(); });
  document.addEventListener('visibilitychange', function () { if (document.hidden) stopMonitor(); else refreshMonitor(); });
  window.addEventListener('pagehide', stopMonitor);
  setInterval(refreshMonitor, 1000);
  $('location-open').addEventListener('click', function () { act(nperf.openLocationSettings()); });
  setInterval(refreshAdapters, 5000);
  $('alias-save').addEventListener('click', function () { act(nperf.setAlias($('alias').value)); });
  $('key-save').addEventListener('click', function () {
    const k = $('key').value;
    $('key').value = '';
    act(nperf.setKey(k));
  });
  $('subnet-add').addEventListener('click', function () {
    const v = $('subnet').value; $('subnet').value = '';
    act(nperf.addSubnet(v));
  });
  loop();
});
