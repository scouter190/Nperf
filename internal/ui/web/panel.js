'use strict';
// Vista Panel (T9-T11 la completan): barra TEST!, diagnósticos, popover ⓘ (lo abre shell.js),
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
    render: function (s) { renderBar(s); renderStrip(s); },
    phasesFor: phasesFor, barModel: barModel, stripModel: stripModel, fmtElapsed: fmtElapsed
  };
})();
