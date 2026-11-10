'use strict';
// UI de NPerf. Todo dato del backend se inserta como texto (textContent / createTextNode).

// Puente con Go: T15 define window.nperfCall(secreto, nombre, ...args) -> Promise.
// El secreto de sesión lo entrega Go dentro del HTML servido (meta nperf-session).
// Para previsualizar, mock.js define window.nperf antes de cargar este archivo.
const NPERF_METHODS = ['state', 'selectPeer', 'setDirection', 'setDuration', 'setAlias', 'setKey', 'addSubnet', 'removeSubnet', 'startTest', 'cancelTest', 'adapters', 'openLocationSettings'];
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

function deviceRow(d, selectable, selected) {
  const row = el('div', 'dev' + (selectable ? '' : ' self') + (selected ? ' sel' : '') + (d.status === 'unreachable' ? ' unreachable' : ''));
  const kind = KINDS[d.link] ? d.link : 'unknown';
  const ico = el('span', 'ico ' + kind); ico.title = KINDS[kind];
  row.appendChild(ico);
  const info = el('div', 'info');
  info.appendChild(el('span', 'name', d.alias));
  info.appendChild(el('span', 'ip', (d.ips || []).join(', ')));
  const ln = linkLine(d);
  if (ln) info.appendChild(el('span', 'ip', ln));
  row.appendChild(info);
  row.appendChild(el('span', 'badge ' + d.status, d.status_label));
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
  $('empty').hidden = s.peers.length > 0;
}

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
  $('test').hidden = running;
  $('cancel').hidden = !running;
  $('cancel').disabled = s.test.status === 'cancelling';
  $('test').disabled = !s.can_test;
  $('test').title = s.can_test ? '' : s.test_blocked_reason;
  $('blocked').textContent = s.can_test ? '' : s.test_blocked_reason;
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
  const peer = s.peers.filter(function (p) { return p.id === s.selected_id; })[0];
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
  const rtt = r.rtt;
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
      const d = r[proto][dir];
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
  if (detailed && r.notes && r.notes.length) {
    const ul = el('ul', 'notes');
    r.notes.forEach(function (n) { ul.appendChild(el('li', '', n)); });
    box.appendChild(ul);
  }
  return box;
}

function renderDetail(s) {
  const detailed = $('detailed').checked;
  document.body.classList.toggle('compact', !detailed);
  if (!changed('detail', [s.test, s.directions, detailed, s.selected_id])) return;
  const c = $('detail'); clear(c);
  const t = s.test;
  const peer = s.peers.filter(function (p) { return p.id === s.selected_id; })[0];
  c.appendChild(el('p', 'status-line', peer ? 'Destino: ' + peer.alias + ' (' + (peer.ips || []).join(', ') + ')' : 'Sin destino seleccionado'));
  if (t.status === 'queued' || t.status === 'running' || t.status === 'cancelling') c.appendChild(progress(s));
  else if (t.status === 'error') c.appendChild(el('p', 'err-text', 'Error: ' + t.error));
  else if (t.status === 'cancelled') c.appendChild(el('p', 'muted', 'Prueba cancelada.'));
  else if (t.status === 'done' && t.report) c.appendChild(reportView(s, detailed));
  else c.appendChild(el('p', 'muted', 'Elija un equipo y pulse TEST!.'));
}

function adapterCard(a, detailed) {
  const card = el('div', 'card' + (a.connected ? '' : ' off'));
  const head = el('div', 'card-head');
  const kind = KINDS[a.kind] ? a.kind : 'unknown';
  const ico = el('span', 'ico ' + kind); ico.title = a.kind_label || KINDS[kind];
  head.appendChild(ico);
  head.appendChild(el('span', 'name', a.name));
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

function render(s) {
  last = s;
  renderBar(s); renderList(s); renderSettings(s); renderDetail(s);
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
  $('test').addEventListener('click', function () { act(nperf.startTest()); });
  $('cancel').addEventListener('click', function () { act(nperf.cancelTest()); });
  $('detailed').addEventListener('change', function () { delete sig.detail; if (last) renderDetail(last); refreshAdapters(); });
  $('adapters-box').addEventListener('toggle', refreshAdapters);
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
