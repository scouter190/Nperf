'use strict';
// Vista Panel (T9-T11 la completan; T10: resultados): barra TEST!, diagnósticos, popover ⓘ (lo abre shell.js),
// franja de progreso y resultados. Los textos de bloqueo son siempre los de Go
// (test_blocked_reason, probe_blocked_reason); todo dato del backend va como texto.
(function () {
  const core = window.NPerfCore;
  const $ = function (id) { return document.getElementById(id); };

  const PROBES = [['ping', 'probe-ping'], ['trace', 'probe-trace'], ['mtu', 'probe-mtu'], ['ports', 'probe-ports']];
  const PROBE_NAMES = { ping: 'Ping', trace: 'Traceroute', mtu: 'MTU de la ruta', ports: 'Puertos TCP', internet: 'Latencia a internet', speed: 'Velocidad a internet (M-Lab)' };
  const PHASE_LABELS = { rtt: 'RTT', tcp_fwd: 'TCP Ida', tcp_rev: 'TCP Vuelta', udp_fwd: 'UDP Ida', udp_rev: 'UDP Vuelta' };

  function devices(s) { return (s.peers || []).concat(s.no_agent || []); }
  function selected(s) { return devices(s).filter(function (p) { return p.id === s.selected_id; })[0]; }

  // Fases visibles según el sentido elegido (ida = cliente -> servidor).
  function phasesFor(direction) {
    return ['rtt', 'tcp_fwd', 'tcp_rev', 'udp_fwd', 'udp_rev'].filter(function (k) {
      if (k === 'rtt') return true;
      return k.slice(-3) === 'fwd' ? direction !== 'rev' : direction !== 'fwd';
    });
  }

  // Modelo de la barra: botón TEST!/Cancelar, texto de bloqueo y botones de diagnóstico.
  function barModel(s) {
    const t = s.test || {}, p = s.probe || {};
    const st = t.status || 'idle';
    let mode = 'test';
    if (st === 'cancelling') mode = 'cancelling';
    else if (st === 'queued' || st === 'running') mode = 'cancel';
    const testBlocked = mode === 'test' && !s.can_test ? (s.test_blocked_reason || '') : '';
    // El motivo se muestra una sola vez: el de los diagnósticos solo si el de TEST! no está visible.
    const probeBlocked = !s.can_probe && !p.running && !testBlocked ? (s.probe_blocked_reason || '') : '';
    const hasSel = !!selected(s);
    return {
      mode: mode,
      testDisabled: mode === 'test' && !s.can_test,
      testText: testBlocked,
      probeText: probeBlocked,
      probeOff: !hasSel || !s.can_probe,
      probeTip: s.can_probe ? '' : (s.probe_blocked_reason || ''),
      locked: mode !== 'test'
    };
  }

  function fmtElapsed(ms) {
    const sec = Math.max(0, Math.floor(ms / 1000));
    return Math.floor(sec / 60) + ':' + (sec % 60 < 10 ? '0' : '') + (sec % 60);
  }

  // Modelo de la franja de progreso (null si no hay nada en curso). No incluye el tiempo.
  function stripModel(s) {
    const t = s.test || {}, p = s.probe || {};
    const peer = selected(s);
    const alias = peer ? peer.alias : '';
    if (t.status === 'queued') {
      return { kind: 'queued', text: t.queue_position > 0 ? 'En cola en ' + alias + ' — Posición #' + t.queue_position : 'Conectando con ' + alias + '…', cancel: 'Cancelar espera' };
    }
    if (t.status === 'running') {
      const ph = phasesFor(s.direction), cur = ph.indexOf(t.phase);
      return { kind: 'running', label: t.label || '', chips: ph.map(function (k, i) { return { key: k, text: PHASE_LABELS[k], state: cur > i ? 'done' : (cur === i ? 'cur' : '') }; }) };
    }
    if (t.status === 'cancelling') return { kind: 'cancelling', text: t.label || 'Cancelando…' };
    if (p.status === 'running' || p.status === 'cancelling') {
      return { kind: 'probe', text: PROBE_NAMES[p.kind] || 'Diagnóstico', label: p.label || '', cancelling: p.status === 'cancelling', cancel: 'Cancelar' };
    }
    return null;
  }

  // ---- tiempo transcurrido (solo reloj local, sin ETA) ----
  const clock = { key: '', t0: 0 };
  function elapsedFor(m, now) {
    const key = m ? m.kind : '';
    if (key !== 'running' && key !== 'probe') { clock.key = ''; return null; }
    if (clock.key !== key) { clock.key = key; clock.t0 = now; }
    return now - clock.t0;
  }

  // ---- DOM ----
  function run(btn, fn, text) {
    core.busy(btn, Promise.resolve().then(fn), text).then(poll, function (e) { core.pushToast(e, 'error'); return poll(); });
  }

  function renderBar(s) {
    const m = barModel(s);
    core.renderIf('panel-dirs', s.directions || [], function (list) {
      const sel = $('direction'); clear(sel);
      list.forEach(function (o) { const op = el('option', '', o.label); op.value = o.value; sel.appendChild(op); });
    });
    core.setValueIfIdle($('direction'), s.direction);
    core.setValueIfIdle($('duration'), s.duration_seconds);
    $('direction').disabled = m.locked;
    $('duration').disabled = m.locked;
    $('total').textContent = s.direction === 'both' ? (s.total_label || '') : '';
    core.renderIf('panel-bar', m, function () {
      const t = $('test'), c = $('cancel');
      t.hidden = m.mode !== 'test';
      t.disabled = m.testDisabled;
      c.hidden = m.mode === 'test';
      c.disabled = m.mode === 'cancelling';
      c.textContent = m.mode === 'cancelling' ? 'Cancelando…' : 'Cancelar';
      c.classList.toggle('busy', m.mode === 'cancelling');
      $('blocked').textContent = m.testText;
      $('probe-blocked').textContent = m.probeText;
      PROBES.forEach(function (x) {
        const b = $(x[1]);
        b.disabled = m.probeOff;
        b.title = m.probeOff ? m.probeTip : '';
      });
    });
  }

  function renderStrip(s) {
    const box = $('panel-progress');
    const m = stripModel(s);
    const ms = elapsedFor(m, Date.now());
    core.renderIf('panel-strip', m, function () {
      clear(box);
      if (!m) return;
      const row = el('div', 'surf prog' + (m.kind === 'queued' ? ' q' : ''));
      row.role = 'status';
      row.appendChild(el('span', 'ind' + (m.kind === 'queued' ? ' q' : '')));
      if (m.kind === 'running') {
        const chips = el('div', 'chips');
        m.chips.forEach(function (c) { chips.appendChild(el('span', 'chip' + (c.state ? ' ' + c.state : ''), c.text)); });
        row.appendChild(chips);
        if (m.label) row.appendChild(el('span', 'muted2', m.label));
        row.appendChild(el('span', 'grow'));
        row.appendChild(el('span', 'mono muted2 elapsed'));
      } else {
        row.appendChild(el('span', 'grow', m.text));
        if (m.kind === 'probe') {
          if (m.label) row.appendChild(el('span', 'muted2', m.label));
          row.appendChild(el('span', 'mono muted2 elapsed'));
        }
        if (m.cancel) {
          const b = el('button', 'btn sm', m.cancelling ? 'Cancelando…' : m.cancel); b.type = 'button';
          b.disabled = !!m.cancelling;
          b.addEventListener('click', function () { run(b, m.kind === 'probe' ? nperf.cancelProbe : nperf.cancelTest); });
          row.appendChild(b);
        }
      }
      box.appendChild(row);
    });
    const e = box.querySelector('.elapsed');
    if (e && ms !== null) e.textContent = fmtElapsed(ms);
  }


  // ================= Resultados (T10) =================
  const DASH = '—';
  const F = function () { return core.fmt; };
  const DIR_NAMES = { fwd: 'Ida', rev: 'Vuelta' };
  const STATUS_TAGS = { done: ['ok', 'terminado'], cancelled: ['warn', 'cancelado'], running: ['accent', 'en curso'], queued: ['accent', 'en cola'], cancelling: ['warn', 'cancelando'], error: ['err', 'error'] };
  const KIND_LABELS = { eth: 'Ethernet', wifi: 'Wi-Fi', other: 'Otro enlace', unknown: 'Enlace desconocido' };
  const PORT_STATES = { open: 'abierto', closed: 'cerrado', filtered: 'filtrado', canceled: 'cancelado', cancelled: 'cancelado' };
  const REPLY_STATES = { ok: 'respuesta', timeout: 'tiempo agotado', ttl_expired: 'TTL expirado', host_unreachable: 'host inaccesible', net_unreachable: 'red inaccesible', port_unreachable: 'puerto inaccesible', frag_needed: 'fragmentación necesaria', other: 'sin respuesta' };

  function isNum(v) { return typeof v === 'number' && isFinite(v); }
  function missLabel(d) { return d && d.status === 'unavailable' ? 'No disponible' : 'No medido'; }
  function nf(v, d) { return isNum(v) ? F().num(v, d === undefined ? 1 : d) : DASH; }
  function fmtMbpsNum(v) { return isNum(v) ? F().num(v, v >= 100 ? 0 : 1) : DASH; }
  function fmtMB(b) {
    if (!isNum(b)) return DASH;
    return b >= 1073741824 ? (b / 1073741824).toFixed(2) + ' GB' : (b / 1048576).toFixed(b >= 104857600 ? 0 : 1) + ' MB';
  }
  function fmtBps(n) {
    if (!isNum(n) || n <= 0) return '';
    const u = ['bps', 'Kbps', 'Mbps', 'Gbps', 'Tbps']; let v = n, i = 0;
    while (v >= 1000 && i < u.length - 1) { v /= 1000; i++; }
    return (Math.round(v * 10) / 10) + ' ' + u[i];
  }
  function linkLine(l) {
    if (!l) return 'enlace desconocido';
    const parts = [KIND_LABELS[l.kind] || KIND_LABELS.unknown];
    const tx = fmtBps(l.tx_bps), rx = fmtBps(l.rx_bps);
    if (tx && rx && tx !== rx) parts.push('↑' + tx + ' / ↓' + rx); else if (tx || rx) parts.push(tx || rx);
    if (l.ssid) parts.push(l.ssid);
    return parts.join(' · ');
  }
  function retransText(r) {
    if (!r || r.status !== 'available' || !isNum(r.count)) return 'retr. n/d';
    return (r.estimated ? '~' : '') + r.count + ' retr.';
  }
  function phaseOf(s, name) {
    const ch = s.test && s.test.chart;
    return ((ch && ch.phases) || []).filter(function (p) { return p.phase === name; })[0] || null;
  }
  function liveMbps(p, mean) {
    const v = (p.samples || []).map(function (x) { return x[2]; }).filter(isNum);
    if (!v.length) return null;
    return mean ? v.reduce(function (a, b) { return a + b; }, 0) / v.length : v[v.length - 1];
  }
  // Valor TCP de un sentido: del informe si está medido; si no, de las muestras en vivo (en curso o cancelado).
  function tcpValue(s, dir) {
    const t = s.test || {}, rep = t.report || {};
    const r = (rep.tcp && rep.tcp[dir]) || null;
    if (r && r.status === 'measured' && isNum(r.throughput_mbps)) return { mbps: r.throughput_mbps, d: r, note: '' };
    const p = phaseOf(s, 'tcp_' + dir);
    if (p && p.available !== false && (t.status === 'running' || t.status === 'cancelled')) {
      const v = liveMbps(p, t.status === 'cancelled');
      if (v !== null) return { mbps: v, d: r, note: t.status === 'running' && t.phase === 'tcp_' + dir ? 'en curso…' : 'parcial' };
    }
    return { mbps: null, d: r, note: '' };
  }
  // Mayor subida de latencia bajo carga que Go ya marcó como aviso (null si no hay).
  function bloatMs(s) {
    const ch = s.test && s.test.chart; let best = null;
    ((ch && ch.phases) || []).forEach(function (p) {
      if (p.note && p.loaded_rtt && p.rtt_initial && isNum(p.loaded_rtt.median_ms) && isNum(p.rtt_initial.avg_ms)) {
        const d = Math.round(p.loaded_rtt.median_ms - p.rtt_initial.avg_ms);
        if (best === null || d > best) best = d;
      }
    });
    return best;
  }

  function kpiModels(s) {
    const t = s.test || {}, rep = t.report || {}, dg = t.diagnosis || {};
    const run = t.status === 'running';
    const tv = { fwd: tcpValue(s, 'fwd'), rev: tcpValue(s, 'rev') };
    const top = Math.max(tv.fwd.mbps || 0, tv.rev.mbps || 0);
    const out = [];
    ['fwd', 'rev'].forEach(function (dir) {
      const v = tv[dir], d = dg[dir];
      const k = { id: 'tcp_' + dir, title: 'TCP ' + DIR_NAMES[dir] + (dir === 'fwd' ? ' ↑' : ' ↓'), rev: dir === 'rev', tag: '', value: DASH, unit: '', bar: null, subs: [], foot: '' };
      if (v.mbps === null) { k.foot = missLabel(v.d); k.bar = 0; if (run && t.phase === 'tcp_' + dir) k.foot = 'en curso…'; }
      else {
        k.value = fmtMbpsNum(v.mbps); k.unit = 'Mbps';
        k.bar = top > 0 ? Math.round(v.mbps / top * 100) : 0;
        if (v.d) k.foot = fmtMB(v.d.bytes) + ' · ' + retransText(v.d.retransmissions);
        else k.foot = v.note;
      }
      if (d && d.color && d.color !== 'none' && isNum(d.percent) && v.mbps !== null && !v.note) k.tag = d.percent + ' % PHY';
      out.push(k);
    });
    const r = rep.rtt;
    const rk = { id: 'rtt', title: 'RTT', tag: '', value: DASH, unit: '', bar: null, subs: [], foot: '' };
    if (r && r.status === 'measured' && isNum(r.avg_ms)) {
      rk.value = nf(r.avg_ms); rk.unit = 'ms';
      rk.subs.push({ text: 'mín ' + nf(r.min_ms) + ' / máx ' + nf(r.max_ms), warn: false });
    } else {
      const ini = ['fwd', 'rev'].map(function (d) { return phaseOf(s, 'tcp_' + d); }).filter(function (p) { return p && p.rtt_initial && isNum(p.rtt_initial.avg_ms); })[0];
      if (ini) { rk.value = nf(ini.rtt_initial.avg_ms); rk.unit = 'ms'; rk.subs.push({ text: 'mín ' + nf(ini.rtt_initial.min_ms) + ' / máx ' + nf(ini.rtt_initial.max_ms), warn: false }); }
      else rk.subs.push({ text: missLabel(r), warn: false });
    }
    const bl = bloatMs(s);
    if (bl !== null) rk.subs.push({ text: '+' + bl + ' ms bajo carga', warn: true });
    out.push(rk);
    const uk = { id: 'udp', title: 'UDP pérdida / jitter', tag: '', value: DASH, unit: '', bar: null, subs: [], foot: '' };
    const ud = {}; ['fwd', 'rev'].forEach(function (dir) { const d = rep.udp && rep.udp[dir]; ud[dir] = d && d.status === 'measured' && d.udp ? d : null; });
    const have = ['fwd', 'rev'].filter(function (dir) { return ud[dir]; });
    if (have.length) {
      const loss = Math.max.apply(null, have.map(function (dir) { return ud[dir].udp.loss_pct; }).filter(isNum));
      const jit = Math.max.apply(null, have.map(function (dir) { return ud[dir].udp.jitter_ms; }).filter(isNum));
      uk.value = isFinite(loss) ? nf(loss, 2) : DASH;
      uk.unit = '% · ' + (isFinite(jit) ? nf(jit) : DASH) + ' ms';
      uk.subs.push({ text: ['fwd', 'rev'].map(function (dir) { return (dir === 'fwd' ? '↑' : '↓') + (ud[dir] && isNum(ud[dir].udp.loss_pct) ? nf(ud[dir].udp.loss_pct, 2) + ' %' : DASH); }).join(' · '), warn: false });
    } else {
      const d0 = (rep.udp && (rep.udp.fwd || rep.udp.rev)) || null;
      uk.subs.push({ text: run ? 'Pendiente' : missLabel(d0), warn: false });
    }
    out.push(uk);
    return out;
  }

  // Semáforo: peor color de los sentidos; null si no hay datos (o la prueba no terminó).
  function semModel(s) {
    const t = s.test || {};
    if (t.status !== 'done') return null;
    const dg = t.diagnosis || {}, rank = { green: 0, amber: 1, red: 2 };
    const list = ['fwd', 'rev'].filter(function (k) { return dg[k] && rank[dg[k].color] !== undefined; });
    const bl = bloatMs(s);
    const tags = [];
    if (bl !== null) tags.push('Bufferbloat +' + bl + ' ms');
    if (!list.length) return bl === null ? null : { cls: 'warn', icon: '!', title: 'Latencia alta bajo carga', tags: tags, text: '' };
    let worst = list[0];
    list.forEach(function (k) { if (rank[dg[k].color] > rank[dg[worst].color]) worst = k; });
    const w = dg[worst], c = w.color;
    list.forEach(function (k) { if (rank[dg[k].color] > 0 && isNum(dg[k].percent)) tags.push(DIR_NAMES[k] + ' al ' + dg[k].percent + ' % del PHY'); });
    const text = list.filter(function (k) { return k !== worst || list.length > 1; }).map(function (k) { return DIR_NAMES[k] + ': ' + (dg[k].text || '') + (dg[k].partial ? ' (parcial)' : ''); }).join(' · ');
    return { cls: c === 'green' ? 'ok' : (c === 'red' ? 'err' : 'warn'), icon: c === 'green' ? '✓' : (c === 'red' ? '✕' : '!'), title: w.text || 'Sin limitación detectada', tags: tags, text: text };
  }

  function destLabel(dev) { return dev ? dev.alias + ' (' + (dev.ips || []).join(', ') + ')' : ''; }

  // Cabecera común: título, etiqueta de estado, línea PHY y Repetir.
  function headModel(s, mode) {
    const t = s.test || {}, p = s.probe || {}, dev = selected(s);
    if (mode === 'test') {
      const dg = t.diagnosis || {};
      const tag = STATUS_TAGS[t.status] || STATUS_TAGS.done;
      return { title: 'Destino: ' + (dev ? destLabel(dev) : 'sin seleccionar'), tag: tag, sub: dg.local || dg.remote ? ((s.self && s.self.alias) || 'Este equipo') + ': ' + linkLine(dg.local) + '  →  ' + (dev ? dev.alias : 'destino') + ': ' + linkLine(dg.remote) : '',
        repeat: { kind: 'test', enabled: !!s.can_test && !!dev && t.status !== 'queued' && t.status !== 'running' && t.status !== 'cancelling', why: s.can_test ? '' : (s.test_blocked_reason || '') } };
    }
    const target = (devices(s).filter(function (x) { return x.id === p.target_id; })[0]);
    const net = p.kind === 'internet' || p.kind === 'speed';
    const tag = STATUS_TAGS[p.status] || STATUS_TAGS.done;
    return { title: (PROBE_NAMES[p.kind] || 'Diagnóstico') + (!net && target ? ' · ' + destLabel(target) : ''), tag: tag, sub: '',
      repeat: { kind: p.kind, enabled: !!s.can_probe && p.status !== 'running' && p.status !== 'cancelling' && (net || !!target), why: s.can_probe ? '' : (s.probe_blocked_reason || '') } };
  }

  // Qué se muestra en el Panel: 'test', 'probe' o 'empty'.
  function viewMode(s, probeFirst) {
    const t = s.test || {}, p = s.probe || {};
    const tAct = t.status === 'queued' || t.status === 'running' || t.status === 'cancelling';
    const pAct = p.status === 'running' || p.status === 'cancelling';
    if (tAct) return 'test';
    if (pAct) return 'probe';
    if (probeFirst && p.status && p.status !== 'idle') return 'probe';
    if (t.status && t.status !== 'idle') return 'test';
    if (p.status && p.status !== 'idle') return 'probe';
    return 'empty';
  }

  // ---- DOM de resultados ----
  const openVals = {};
  let chartDrawn = 0, chartPending = false;

  function head(s, hm) {
    const box = el('div', 'surf rhead');
    const info = el('div', 'info');
    const t = el('div', 't');
    t.appendChild(el('span', '', hm.title));
    t.appendChild(el('span', 'tag ' + hm.tag[0], hm.tag[1]));
    info.appendChild(t);
    if (hm.sub) info.appendChild(el('div', 'phy', hm.sub));
    box.appendChild(info);
    const acts = el('div', 'acts');
    const b = el('button', 'btn sm', '↻ Repetir'); b.type = 'button';
    b.disabled = !hm.repeat.enabled; b.title = hm.repeat.enabled ? '' : hm.repeat.why;
    b.addEventListener('click', function () { repeat(b, hm.repeat.kind); });
    acts.appendChild(b);
    box.appendChild(acts);
    return box;
  }
  // Repetir: TEST! y diagnósticos arrancan directos; la velocidad reabre el consentimiento (nunca mide sola).
  function repeat(btn, kind) {
    if (!last) return;
    if (kind === 'test') { showProbe = false; delete sig.detail; run(btn, function () { return nperf.startTest(); }); return; }
    if (kind === 'speed') { if (window.NPerfInternet) window.NPerfInternet.openSpeedConsent(); return; }
    showProbe = true; delete sig.detail;
    if (kind === 'internet') { run(btn, function () { return nperf.startInternet(); }); return; }
    const id = last.probe && last.probe.target_id;
    run(btn, function () { return nperf.startProbe(kind, id); });
  }
  function semBox(m) {
    const box = el('div', 'sem' + (m.cls === 'ok' ? ' ok' : (m.cls === 'err' ? ' err' : '')));
    box.role = 'status';
    box.appendChild(el('span', 'sem-ico', m.icon));
    box.lastChild.ariaHidden = 'true';
    const b = el('div', 'sem-b');
    const t = el('div', 'sem-t', m.title);
    (m.tags || []).forEach(function (x) { t.appendChild(el('span', 'tag warn', x)); });
    b.appendChild(t);
    if (m.text) b.appendChild(el('p', 'sem-x', m.text));
    box.appendChild(b);
    return box;
  }
  function kpiBox(k) {
    const box = el('div', 'surf kpi');
    const h = el('div', 'kpi-h'); h.appendChild(el('span', '', k.title));
    if (k.tag) h.appendChild(el('span', 'tag accent', k.tag));
    box.appendChild(h);
    const v = el('div', 'kpi-v' + (k.value === DASH ? ' na' : ''), k.value);
    if (k.unit) { v.appendChild(document.createTextNode(' ')); v.appendChild(el('small', '', k.unit)); }
    box.appendChild(v);
    if (k.bar !== null) {
      const pr = el('progress', 'pb' + (k.rev ? ' rev' : '')); pr.max = 100; pr.value = k.bar;
      pr.ariaLabel = k.title + ' respecto al máximo'; box.appendChild(pr);
    }
    k.subs.forEach(function (x) { box.appendChild(el('div', 'kpi-s' + (x.warn ? ' warn' : ''), x.text)); });
    if (k.foot) box.appendChild(el('div', 'kpi-f', k.foot));
    return box;
  }
  function pts(samples, idx) {
    return (samples || []).map(function (x) { return [x[0] === null ? null : x[0] + (x[1] || 0), x[idx]]; });
  }
  function chartIn(parent, key, opts) {
    if (!window.NPerfChart) return;
    const box = el('div', 'chart-box');
    parent.appendChild(box);
    const w = window.NPerfChart.render(box, opts);
    const det = w.querySelector('.chart-data');
    if (det) { det.open = !!openVals[key]; det.addEventListener('toggle', function () { openVals[key] = det.open; }); }
  }
  function mainChart(s) {
    const ch = s.test && s.test.chart;
    if (!ch || !ch.phases || !ch.phases.length) return null;
    const t = s.test;
    const card = el('div', 'surf gcard');
    const gh = el('div', 'gh'); gh.appendChild(el('span', 't', 'Rendimiento TCP'));
    const lg = el('span', 'legend');
    const tcp = ch.phases.filter(function (p) { return p.phase.indexOf('tcp') === 0; });
    tcp.forEach(function (p) { const sp = el('span', '', ''); sp.appendChild(el('i', p.phase === 'tcp_fwd' ? 'f' : 'r')); sp.appendChild(document.createTextNode('TCP ' + (p.phase === 'tcp_fwd' ? 'Ida' : 'Vuelta'))); lg.appendChild(sp); });
    gh.appendChild(lg); card.appendChild(gh);
    const ok = tcp.filter(function (p) { return p.available && (p.samples || []).length; });
    if (!ok.length) {
      const na = tcp.some(function (p) { return !p.available; });
      card.appendChild(el('p', 'muted chart-na', na ? 'gráfica no disponible: el otro equipo no envía muestras' : (t.status === 'cancelled' ? 'Cancelado antes de recibir muestras.' : 'Esperando muestras…')));
      return card;
    }
    chartIn(card, 'main', { title: 'Rendimiento TCP', xLabel: 'Tiempo', xUnit: 's', yUnit: 'Mbps', decimals: 1, width: 720, height: 118,
      series: ok.map(function (p) { return { name: 'TCP ' + (p.phase === 'tcp_fwd' ? 'Ida' : 'Vuelta'), points: pts(p.samples, 2) }; }) });
    if (ch.cancelled || t.status === 'cancelled') card.appendChild(el('p', 'muted chart-cancelled', 'Cancelado: se muestran las muestras recibidas antes de cancelar.'));
    return card;
  }
  function tbl(cols, rows) {
    const w = el('div', 'tw'), t = el('table', 'tbl'), thead = el('thead'), hr = el('tr');
    cols.forEach(function (c) { hr.appendChild(el('th', c[1] ? 'r' : '', c[0])); });
    thead.appendChild(hr); t.appendChild(thead);
    const tb = el('tbody');
    rows.forEach(function (r) {
      const tr = el('tr');
      r.forEach(function (c, i) { tr.appendChild(el('td', cols[i][1] ? 'r' : '', c)); });
      tb.appendChild(tr);
    });
    t.appendChild(tb); w.appendChild(t);
    return w;
  }
  function card(title, body) {
    const c = el('div', 'surf');
    if (title) c.appendChild(el('div', 'sech', title));
    c.appendChild(body);
    return c;
  }
  function metricCell(d, f) { return d && d.status === 'measured' ? f(d) : missLabel(d); }
  // Tablas del Modo detallado (TCP, UDP, RTT bajo carga, avisos y gráficas por fase).
  function detailBlocks(s) {
    const t = s.test || {}, rep = t.report || {}, dg = t.diagnosis || {}, out = [];
    const rows = ['fwd', 'rev'].map(function (dir) {
      const d = rep.tcp && rep.tcp[dir];
      const m = d && d.status === 'measured';
      const pct = dg[dir] && dg[dir].color !== 'none' && isNum(dg[dir].percent) ? dg[dir].percent + ' %' : DASH;
      return [DIR_NAMES[dir], m ? fmtMbpsNum(d.throughput_mbps) : missLabel(d), m ? fmtMB(d.bytes) : DASH, m ? retransText(d.retransmissions).replace(' retr.', '').replace('retr. n/d', 'no disponible') : DASH, pct];
    });
    out.push(card('TCP', tbl([['Sentido', 0], ['Mbps', 1], ['MB', 1], ['Retrans.', 1], ['% PHY', 1]], rows)));
    const urows = ['fwd', 'rev'].map(function (dir) {
      const d = rep.udp && rep.udp[dir];
      const m = d && d.status === 'measured' && d.udp;
      return [DIR_NAMES[dir], m ? fmtMbpsNum(d.throughput_mbps) : missLabel(d), m ? (nf(d.udp.loss_pct, 2) + ' %') : DASH, m ? nf(d.udp.jitter_ms, 3) : DASH,
        m ? F().count(d.udp.datagrams_sent) + ' / ' + F().count(d.udp.datagrams_lost) : DASH];
    });
    out.push(card('UDP', tbl([['Sentido', 0], ['Mbps', 1], ['Pérdida', 1], ['Jitter (ms)', 1], ['Datagramas env. / perd.', 1]], urows)));
    const ch = (t.chart && t.chart.phases) || [];
    const lrows = ch.filter(function (p) { return p.loaded_rtt; }).map(function (p) {
      const l = p.loaded_rtt, i = p.rtt_initial;
      return [p.label || p.phase, i ? nf(i.avg_ms) : DASH, nf(l.min_ms), nf(l.median_ms), nf(l.avg_ms), nf(l.max_ms), F().count(l.samples) + ' de ' + F().count(l.sent)];
    });
    if (lrows.length) out.push(card('RTT bajo carga (ms)', tbl([['Fase', 0], ['Inicial', 1], ['Mín', 1], ['Mediana', 1], ['Media', 1], ['Máx', 1], ['Sondas', 1]], lrows)));
    if (Array.isArray(rep.notes) && rep.notes.length) { const ul = el('ul', 'notes'); rep.notes.forEach(function (n) { ul.appendChild(el('li', '', n)); }); out.push(card('Avisos', ul)); }
    ch.filter(function (p) { return p.note; }).forEach(function (p) { out.push(el('p', 'chart-note', 'Aviso: ' + p.note)); });
    const withRtt = ch.filter(function (p) { return p.available && (p.samples || []).some(function (x) { return isNum(x[3]); }); });
    const udp = ch.filter(function (p) { return p.available && p.phase.indexOf('udp') === 0 && (p.samples || []).length; });
    if (withRtt.length || udp.length) {
      const c = el('div', 'surf gcard'); c.appendChild(el('div', 'gh')).appendChild(el('span', 't', 'Gráficas por fase'));
      if (withRtt.length) chartIn(c, 'rtt', { title: 'Latencia bajo carga', xLabel: 'Tiempo', xUnit: 's', yUnit: 'ms', decimals: 1, width: 720, height: 150,
        series: withRtt.map(function (p) { return { name: p.label || p.phase, points: pts(p.samples, 3) }; }) });
      if (udp.length) {
        chartIn(c, 'loss', { title: 'Pérdida UDP', xLabel: 'Tiempo', xUnit: 's', yUnit: '%', decimals: 2, width: 720, height: 150, series: udp.map(function (p) { return { name: p.label || p.phase, points: pts(p.samples, 4) }; }) });
        chartIn(c, 'jit', { title: 'Jitter UDP', xLabel: 'Tiempo', xUnit: 's', yUnit: 'ms', decimals: 3, width: 720, height: 150, series: udp.map(function (p) { return { name: p.label || p.phase, points: pts(p.samples, 5) }; }) });
      }
      out.push(c);
    }
    return out;
  }
  function detailToggle() {
    const row = el('div', 'dtoggle');
    row.appendChild(el('span', 'hl', 'Detalle técnico'));
    const lab = el('label', 'chk', 'Modo detallado ');
    const sw = el('button', 'sw'); sw.type = 'button'; sw.id = 'sw-detail'; sw.role = 'switch';
    sw.ariaChecked = core.ui.isDetailedMode ? 'true' : 'false';
    sw.addEventListener('click', function () {
      core.ui.isDetailedMode = !core.ui.isDetailedMode;
      const cb = $('detailed'); if (cb) cb.checked = core.ui.isDetailedMode; // history.js lo lee hasta T16
      delete sig.detail; if (last) renderResult(last);
    });
    lab.appendChild(sw); row.appendChild(lab);
    return row;
  }

  function testResult(s, box) {
    const t = s.test || {};
    box.appendChild(head(s, headModel(s, 'test')));
    if (t.status === 'error') {
      box.appendChild(semBox({ cls: 'err', icon: '✕', title: 'No se pudo completar la prueba', tags: [], text: t.error || '' }));
      return;
    }
    if (t.status === 'queued' || t.status === 'cancelling' && !t.chart) return;
    const sm = semModel(s); if (sm) box.appendChild(semBox(sm));
    const kp = el('div', 'kpis'); kpiModels(s).forEach(function (k) { kp.appendChild(kpiBox(k)); }); box.appendChild(kp);
    const g = mainChart(s); if (g) box.appendChild(g);
    box.appendChild(detailToggle());
    if (core.ui.isDetailedMode) detailBlocks(s).forEach(function (b) { box.appendChild(b); });
  }

  // ---- diagnósticos sin agente ----
  function combinedSem(c) {
    const m = (c && c.messages) || [];
    if (!m.length && !(c && c.healthy)) return null;
    if (c.healthy) return { cls: 'ok', icon: '✓', title: m[0] || 'Sin problemas detectados', tags: [], text: m.slice(1).join(' · ') };
    return { cls: 'warn', icon: '!', title: m[0] || 'Se detectaron problemas', tags: [], text: m.slice(1).join(' · ') };
  }
  function pingTables(r, detailed, box) {
    const ok = r.rtt_status === 'ok';
    box.appendChild(card('', tbl([['Enviados', 1], ['Recibidos', 1], ['Pérdida', 1], ['Mín (ms)', 1], ['Media (ms)', 1], ['Máx (ms)', 1], ['Jitter (ms)', 1]],
      [[F().count(r.sent), F().count(r.received), nf(r.loss_pct, 1) + ' %', ok ? nf(r.min_ms) : DASH, ok ? nf(r.avg_ms) : DASH, ok ? nf(r.max_ms) : DASH, r.jitter_status === 'ok' ? nf(r.jitter_ms) : DASH]])));
    if (!ok) box.appendChild(el('p', 'muted2', 'RTT: no disponible (ninguna respuesta).'));
    if (r.last_error) box.appendChild(el('p', 'muted2', 'Último error: ' + r.last_error));
    if (detailed && r.replies && r.replies.length) {
      box.appendChild(card('Respuestas', tbl([['N.º', 1], ['Resultado', 0], ['Desde', 0], ['Tiempo', 1]], r.replies.map(function (p) {
        return [String(p.seq), REPLY_STATES[p.status] || p.status, p.from || DASH, isNum(p.rtt_ms) ? (p.rtt_ms < 1 ? '< 1 ms' : nf(p.rtt_ms) + ' ms') : (p.reason || '*')];
      }))));
    }
  }
  function traceTables(r, box) {
    const n = (r.hops || []).reduce(function (m, h) { return Math.max(m, (h.probes || []).length); }, 0);
    const cols = [['Salto', 1], ['Equipo', 0]];
    for (let i = 1; i <= n; i++) cols.push(['Sonda ' + i, 1]);
    cols.push(['Pérdida', 1]);
    const rows = (r.hops || []).map(function (h) {
      const row = [String(h.ttl), h.from || '*'];
      for (let i = 0; i < n; i++) { const p = (h.probes || [])[i]; row.push(p && isNum(p.rtt_ms) ? (p.rtt_ms < 1 ? '< 1 ms' : nf(p.rtt_ms) + ' ms') : '*'); }
      row.push(isNum(h.loss_pct) ? nf(h.loss_pct, 0) + ' %' : DASH);
      return row;
    });
    box.appendChild(el('p', 'status-line' + (r.reached ? '' : ' err-text'), r.reached ? 'Destino alcanzado.' : 'Destino no alcanzado.'));
    box.appendChild(card('', tbl(cols, rows)));
    if (r.findings && r.findings.length) { const ul = el('ul', 'notes'); r.findings.forEach(function (f) { ul.appendChild(el('li', '', f)); }); box.appendChild(card('Análisis', ul)); }
  }
  function mtuTables(r, box) {
    if (r.status !== 'ok') { box.appendChild(semBox({ cls: 'err', icon: '✕', title: 'MTU no disponible', tags: [], text: r.reason || 'sin motivo' })); return; }
    box.appendChild(card('', tbl([['MTU de la ruta', 1], ['Carga ICMP máx.', 1], ['MTU interfaz local', 1], ['Sondas', 1]],
      [[r.mtu_bytes + ' bytes', r.max_payload_bytes + ' bytes', r.interface_mtu_bytes ? r.interface_mtu_bytes + ' bytes' : DASH, F().count(r.probes)]])));
    if (r.notes && r.notes.length) { const ul = el('ul', 'notes'); r.notes.forEach(function (f) { ul.appendChild(el('li', '', f)); }); box.appendChild(card('Avisos', ul)); }
  }
  function portsTables(r, box) {
    box.appendChild(card('', tbl([['Puerto', 1], ['Estado', 0], ['Detalle', 0]], (r.ports || []).map(function (p) { return [String(p.port), PORT_STATES[p.state] || 'error', p.detail || '']; }))));
  }

  function probeResult(s, box) {
    const p = s.probe || {};
    box.appendChild(head(s, headModel(s, 'probe')));
    if (p.status === 'error') { box.appendChild(semBox({ cls: 'err', icon: '✕', title: 'No se pudo completar el diagnóstico', tags: [], text: p.error || '' })); return; }
    if (p.status === 'cancelled') { box.appendChild(el('div', 'surf hempty', 'Diagnóstico cancelado.')); return; }
    if (p.status !== 'done' || !p.result) { box.appendChild(el('div', 'surf hempty', p.label || 'Ejecutando…')); return; }
    if (p.kind === 'internet' || p.kind === 'speed') {
      const fn = p.kind === 'internet' ? (typeof internetView === 'function' ? internetView : null) : (typeof speedView === 'function' ? speedView : null);
      if (fn) box.appendChild(card('', p.kind === 'internet' ? fn(p.result, p.combined) : fn(p.result)));
      return;
    }
    const cs = combinedSem(p.combined); if (cs) box.appendChild(semBox(cs));
    if (p.kind === 'ping') pingTables(p.result, core.ui.isDetailedMode, box);
    else if (p.kind === 'trace') traceTables(p.result, box);
    else if (p.kind === 'mtu') mtuTables(p.result, box);
    else if (p.kind === 'ports') portsTables(p.result, box);
    if (p.kind === 'ping') box.appendChild(detailToggle());
  }

  function buildResult(s, mode) {
    const box = el('div', 'pw0');
    if (mode === 'test') testResult(s, box);
    else if (mode === 'probe') probeResult(s, box);
    else {
      const dev = selected(s);
      box.appendChild(el('div', 'surf hempty', dev && (s.no_agent || []).some(function (x) { return x.id === dev.id; }) ? 'Equipo sin NPerf: use Ping, Traceroute, MTU o Puertos.' : (dev ? 'Pulse TEST! o un diagnóstico para empezar.' : 'Elija un equipo y pulse TEST! o un diagnóstico.')));
    }
    return box;
  }

  function renderResult(s) {
    if (s.probe && s.probe.running && s.probe.status === 'running') showProbe = true;
    if (s.test && s.test.running) showProbe = false;
    const mode = viewMode(s, showProbe);
    const t = s.test || {};
    const live = mode === 'test' && t.chart && (t.status === 'running' || t.status === 'cancelling');
    const key = [mode, mode === 'probe' ? s.probe : t, s.can_test, s.can_probe, s.test_blocked_reason, s.selected_id, (s.no_agent || []).length, core.ui.isDetailedMode, s.direction];
    if (!changed('detail', key)) return;
    const now = Date.now();
    if (live && now - chartDrawn < 1000) { // las gráficas en vivo se redibujan como máximo 1 vez por segundo
      delete sig.detail;
      if (!chartPending) { chartPending = true; setTimeout(function () { chartPending = false; if (last) renderResult(last); }, 1000 - (now - chartDrawn)); }
      return;
    }
    if (live) chartDrawn = now;
    const c = $('detail'); clear(c);
    c.appendChild(buildResult(s, mode));
  }

  function commitDuration() {
    const v = parseInt($('duration').value, 10);
    if (isNaN(v) || v < 3 || v > 300) {
      core.pushToast('La duración debe estar entre 3 y 300 segundos', 'error');
      if (last) $('duration').value = last.duration_seconds;
      return;
    }
    act(nperf.setDuration(v));
  }

  function init() {
    $('direction').addEventListener('change', function () { act(nperf.setDirection($('direction').value)); });
    $('duration').addEventListener('change', commitDuration);
    $('test').addEventListener('click', function () {
      showProbe = false; delete sig.detail;
      run($('test'), function () { return nperf.startTest(); });
    });
    $('cancel').addEventListener('click', function () { run($('cancel'), function () { return nperf.cancelTest(); }); });
    PROBES.forEach(function (x) {
      $(x[1]).addEventListener('click', function () {
        if (!last) return;
        showProbe = true; delete sig.detail;
        const id = last.selected_id;
        run($(x[1]), function () { return nperf.startProbe(x[0], id); });
      });
    });
  }
  if (typeof document !== 'undefined' && document.getElementById && document.getElementById('test')) init();

  window.NPerfPanel = {
    render: function (s) { renderBar(s); renderStrip(s); renderResult(s); },
    resultModel: function (s) { return { mode: viewMode(s, false), head: headModel(s, 'test'), kpis: kpiModels(s), sem: semModel(s) }; },
    kpiModels: kpiModels, semModel: semModel, viewMode: viewMode, headModel: headModel, buildResult: buildResult, bloatMs: bloatMs,
    phasesFor: phasesFor, barModel: barModel, stripModel: stripModel, fmtElapsed: fmtElapsed
  };
})();
