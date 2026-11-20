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
// Avisos: pila #toasts (NPerfCore.pushToast, definido al final de este archivo).
let toastLast = '', toastAt = 0;
function toast(msg) {
  const txt = String(msg && msg.message ? msg.message : msg), now = Date.now();
  if (txt === toastLast && now - toastAt < 6000) return; // un fallo persistente no apila avisos
  toastLast = txt; toastAt = now;
  window.NPerfCore.pushToast(txt, 'error');
}
function act(p) { return Promise.resolve(p).then(poll, function (e) { toast(e); return poll(); }); }

const KINDS = { eth: 'Cable (Ethernet)', wifi: 'WiFi', other: 'Otro tipo de enlace', unknown: 'Tipo de enlace desconocido' };
function selectedDevice(s) {
  const all = (s.peers || []).concat(s.no_agent || []);
  return all.filter(function (p) { return p.id === s.selected_id; })[0];
}

function renderBanners(s) {
  $('config-error').hidden = !s.config_error;
  $('config-error-text').textContent = s.config_error || '';
  window.NPerfCore.renderIf('firewall', s.firewall_hint || '', function (h) { $('firewall').hidden = !h; $('firewall').textContent = h; });
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
// ---- gráficas por fase (las usa history.js hasta T16; el Panel dibuja las suyas en panel.js) ----
const openData = {};        // clave de gráfica -> "Valores" abierto (se conserva al redibujar)

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

function render(s) {
  last = s;
  renderBanners(s);
  ['NPerfSidebar', 'NPerfPanel', 'NPerfDrawers', 'NPerfModals'].forEach(function (n) {
    if (window[n] && typeof window[n].render === 'function') window[n].render(s);
  });
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

document.addEventListener('DOMContentLoaded', function () {
  // Un único sondeo de state() (NPerfCore.startPolling); poll() solo se usa tras una acción.
  window.NPerfCore.startPolling(function () { return nperf.state(); }, render, toast);
});

// Núcleo reutilizable por las vistas (spec 008): window.NPerfCore. Convive con el código anterior.
// Estado local de UI (nunca lo toca el polling), render con firma, protección de foco, busy, toasts y formateadores.
(function () {
  const DASH = '—';
  const ui = { activeView: 'panel', activeDrawer: null, isAddHostOpen: false, isDetailedMode: false, isInactiveAdaptersOpen: false, activeModal: null, pendingActions: new Set() };
  const sigs = {};
  const POLL_MS = 500;

  // Llama a fn solo si la firma (JSON) de value cambió desde la última vez para esa clave.
  function renderIf(key, value, fn) {
    const s = JSON.stringify(value === undefined ? null : value);
    if (sigs[key] === s) return false;
    sigs[key] = s;
    fn(value);
    return true;
  }
  function resetSig(key) { if (key === undefined) { Object.keys(sigs).forEach(function (k) { delete sigs[k]; }); } else delete sigs[key]; }

  // No pisa lo que el usuario escribe: ignora el input con foco.
  function setValueIfIdle(input, value) {
    if (!input || document.activeElement === input) return false;
    const v = value === null || value === undefined ? '' : String(value);
    if (input.value !== v) input.value = v;
    return true;
  }

  // Deshabilita el botón mientras dura la promesa; devuelve la misma promesa (con el mismo resultado o error).
  function busy(btn, promise, busyText) {
    const prev = btn ? btn.textContent : '';
    if (btn) {
      ui.pendingActions.add(btn);
      btn.disabled = true;
      if (busyText) btn.textContent = busyText;
    }
    const done = function () {
      if (!btn) return;
      ui.pendingActions.delete(btn);
      btn.disabled = false;
      if (busyText) btn.textContent = prev;
    };
    return Promise.resolve(promise).then(function (v) { done(); return v; }, function (e) { done(); throw e; });
  }

  // Toasts: contenedor #toasts si existe; texto siempre con textContent.
  function toastText(m) { return String(m && m.message ? m.message : m); }
  function pushToast(msg, kind) {
    const box = document.getElementById('toasts');
    if (!box) return null;
    const t = document.createElement('div');
    t.className = 'toast' + (kind ? ' ' + kind : '');
    t.textContent = toastText(msg);
    box.appendChild(t);
    setTimeout(function () { if (t.parentNode) t.parentNode.removeChild(t); }, kind === 'error' ? 8000 : 4000);
    return t;
  }

  // Formateadores: "—" para null, no medido o no disponible.
  function missing(v) {
    if (v === null || v === undefined || (typeof v === 'number' && !isFinite(v))) return true;
    return typeof v === 'object' && (v.status === 'not_measured' || v.status === 'unavailable');
  }
  function num(v, decimals, unit) {
    if (missing(v)) return DASH;
    const n = Number(v);
    if (!isFinite(n)) return DASH;
    return n.toFixed(decimals) + (unit ? ' ' + unit : '');
  }
  function fmtMbps(v) { return num(v, 1, 'Mbps'); }
  function fmtMs(v) { return num(v, 1, 'ms'); }
  function fmtPct(v) { return num(v, 1, '%'); }
  function fmtCount(v) {
    if (missing(v)) return DASH;
    return typeof v === 'string' ? v : Number(v).toLocaleString('es');
  }
  // Métrica de un objeto con status: valor solo si status = measured.
  function metric(obj, field, fmtFn) {
    if (!obj || missing(obj)) return DASH;
    return fmtFn(obj[field]);
  }

  // Sondeo periódico: una petición en vuelo, siguiente a los 500 ms de acabar la anterior.
  function startPolling(fetchState, render, onError) {
    let running = true, inflight = false, timer = null;
    function tick() {
      if (!running) return;
      inflight = true;
      Promise.resolve().then(fetchState).then(render, function (e) { if (onError) onError(e); })
        .then(function () { inflight = false; if (running) timer = setTimeout(tick, POLL_MS); },
          function () { inflight = false; if (running) timer = setTimeout(tick, POLL_MS); });
    }
    tick();
    return { stop: function () { running = false; clearTimeout(timer); }, isBusy: function () { return inflight; } };
  }

  window.NPerfCore = { ui: ui, POLL_MS: POLL_MS, DASH: DASH, renderIf: renderIf, resetSig: resetSig, setValueIfIdle: setValueIfIdle, busy: busy, pushToast: pushToast,
    fmt: { mbps: fmtMbps, ms: fmtMs, pct: fmtPct, count: fmtCount, num: num, metric: metric, missing: missing }, startPolling: startPolling };
})();
