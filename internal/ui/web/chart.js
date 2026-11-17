'use strict';
// Gráfica SVG genérica de líneas. Sin API de HTML en texto: solo createElementNS, textContent y clases.
// Los atributos SVG pasan por attr(): nombres de lista fija y valores numéricos finitos o
// cadenas de lista fija; ningún dato de la red llega a un atributo salvo el aria-label,
// que se sanea (sin control, longitud acotada) y se pone como texto de atributo inerte.
(function () {
  const NS = 'http://www.w3.org/2000/svg';
  const ATTRS = ['x', 'y', 'x1', 'y1', 'x2', 'y2', 'width', 'height', 'viewBox', 'points', 'd', 'r',
    'cx', 'cy', 'text-anchor', 'dominant-baseline', 'role', 'aria-label', 'class', 'preserveAspectRatio'];
  const FIXED = /^[A-Za-z0-9 .,\-]*$/;
  const MARKERS = ['circle', 'square', 'triangle', 'diamond'];

  // Único punto de escritura de atributos.
  function attr(el, name, v) {
    if (ATTRS.indexOf(name) < 0) throw new Error('atributo SVG no permitido');
    let s;
    if (typeof v === 'number') {
      if (!isFinite(v)) throw new Error('valor SVG no finito');
      s = String(Math.round(v * 100) / 100);
    } else if (name === 'aria-label') {
      s = String(v).replace(/[\u0000-\u001f\u007f]/g, ' ').slice(0, 600);
    } else {
      s = String(v);
      if (!FIXED.test(s)) throw new Error('valor SVG no permitido');
    }
    el.setAttribute(name, s);
  }
  function svg(tag, attrs, text) {
    const e = document.createElementNS(NS, tag);
    for (const k in attrs || {}) attr(e, k, attrs[k]);
    if (text !== undefined) e.textContent = text;
    return e;
  }
  function html(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }
  const num = v => typeof v === 'number' && isFinite(v);
  const fmt = (v, d) => num(v) ? v.toFixed(d === undefined ? 1 : d) : '—';

  function niceStep(span, n) {
    const raw = span / n, p = Math.pow(10, Math.floor(Math.log10(raw))), f = raw / p;
    return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * p;
  }

  function marker(kind, x, y, cls) {
    const c = 'mk ' + cls, r = 3.5;
    if (kind === 'circle') return svg('circle', { cx: x, cy: y, r: r, class: c });
    if (kind === 'square') return svg('rect', { x: x - r, y: y - r, width: 2 * r, height: 2 * r, class: c });
    const pts = kind === 'triangle'
      ? [x, y - r - 1, x + r + 1, y + r, x - r - 1, y + r]
      : [x, y - r - 1, x + r + 1, y, x, y + r + 1, x - r - 1, y];
    return svg('polygon', { points: pts.map(n => Math.round(n * 100) / 100).join(' '), class: c });
  }

  // opts: {title, xLabel, xUnit, yUnit, decimals, yMin, yMax, width, height,
  //   series:[{name, points:[[x, y|null]]}], refs:[{y, label}]}
  function render(container, opts) {
    while (container.firstChild) container.removeChild(container.firstChild);
    const W = opts.width || 640, H = opts.height || 220;
    const m = { l: 52, r: 12, t: 10, b: 34 };
    const series = (opts.series || []).filter(s => s && Array.isArray(s.points));
    const refs = (opts.refs || []).filter(r => r && num(r.y));
    let xs = [], ys = [];
    series.forEach(s => s.points.forEach(p => {
      if (num(p[0])) xs.push(p[0]);
      if (num(p[1])) ys.push(p[1]);
    }));
    refs.forEach(r => ys.push(r.y));
    let x0 = Math.min.apply(null, xs), x1 = Math.max.apply(null, xs);
    if (!xs.length) { x0 = 0; x1 = 1; }
    if (x1 <= x0) x1 = x0 + 1;
    let y0 = opts.yMin !== undefined ? opts.yMin : 0;
    let y1 = opts.yMax !== undefined ? opts.yMax : (ys.length ? Math.max.apply(null, ys) * 1.1 : 1);
    if (y1 <= y0) y1 = y0 + 1;
    const px = x => m.l + (x - x0) / (x1 - x0) * (W - m.l - m.r);
    const py = y => H - m.b - (Math.min(Math.max(y, y0), y1) - y0) / (y1 - y0) * (H - m.t - m.b);

    const wrap = html('figure', 'chart');
    wrap.appendChild(html('figcaption', 'chart-title', opts.title || ''));
    const names = series.map(s => s.name).join(', ');
    const label = (opts.title || 'Gráfica') + '. Eje Y en ' + (opts.yUnit || '') + ', eje X en ' + (opts.xUnit || '') +
      '. Series: ' + names + '. Los valores están en la tabla siguiente.';
    const root = svg('svg', { viewBox: '0 0 ' + W + ' ' + H, preserveAspectRatio: 'xMidYMid meet', role: 'img', class: 'chart-svg' });
    attr(root, 'aria-label', label);

    const ystep = niceStep(y1 - y0, 4);
    for (let v = Math.ceil(y0 / ystep) * ystep; v <= y1 + 1e-9; v += ystep) {
      root.appendChild(svg('line', { x1: m.l, y1: py(v), x2: W - m.r, y2: py(v), class: 'grid' }));
      root.appendChild(svg('text', { x: m.l - 6, y: py(v), 'text-anchor': 'end', 'dominant-baseline': 'middle', class: 'tick' }, fmt(v, ystep < 1 ? 1 : 0)));
    }
    const xstep = niceStep(x1 - x0, 6);
    for (let v = Math.ceil(x0 / xstep) * xstep; v <= x1 + 1e-9; v += xstep) {
      root.appendChild(svg('text', { x: px(v), y: H - m.b + 14, 'text-anchor': 'middle', class: 'tick' }, fmt(v, xstep < 1 ? 1 : 0)));
    }
    root.appendChild(svg('line', { x1: m.l, y1: H - m.b, x2: W - m.r, y2: H - m.b, class: 'axis' }));
    root.appendChild(svg('line', { x1: m.l, y1: m.t, x2: m.l, y2: H - m.b, class: 'axis' }));
    root.appendChild(svg('text', { x: (m.l + W - m.r) / 2, y: H - 4, 'text-anchor': 'middle', class: 'axis-label' },
      (opts.xLabel || '') + (opts.xUnit ? ' (' + opts.xUnit + ')' : '')));
    root.appendChild(svg('text', { x: 4, y: 10, class: 'axis-label' }, opts.yUnit || ''));

    refs.forEach(r => {
      root.appendChild(svg('line', { x1: m.l, y1: py(r.y), x2: W - m.r, y2: py(r.y), class: 'ref' }));
      root.appendChild(svg('text', { x: W - m.r - 2, y: py(r.y) - 3, 'text-anchor': 'end', class: 'ref-label' }, r.label || ''));
    });

    series.forEach((s, i) => {
      const cls = 's' + (i % 4), kind = MARKERS[i % MARKERS.length];
      let seg = [];
      const flush = () => {
        if (seg.length > 1) root.appendChild(svg('polyline', { points: seg.join(' '), class: 'line ' + cls }));
        seg = [];
      };
      s.points.forEach(p => {
        if (!num(p[1]) || !num(p[0])) { flush(); return; }
        seg.push(Math.round(px(p[0]) * 100) / 100 + ',' + Math.round(py(p[1]) * 100) / 100);
      });
      flush();
      // Con muchos puntos solo se marca el último valor (la serie se distingue por el trazo).
      const valid = s.points.filter(p => num(p[1]) && num(p[0]));
      (valid.length > 60 ? valid.slice(-1) : valid).forEach(p => root.appendChild(marker(kind, px(p[0]), py(p[1]), cls)));
    });
    wrap.appendChild(root);

    const leg = html('ul', 'chart-legend');
    series.forEach((s, i) => {
      const li = html('li');
      const k = svg('svg', { width: 16, height: 12, viewBox: '0 0 16 12', class: 'legend-key' });
      k.appendChild(svg('line', { x1: 0, y1: 6, x2: 16, y2: 6, class: 'line s' + (i % 4) }));
      k.appendChild(marker(MARKERS[i % MARKERS.length], 8, 6, 's' + (i % 4)));
      li.appendChild(k);
      li.appendChild(html('span', '', ' ' + s.name));
      leg.appendChild(li);
    });
    wrap.appendChild(leg);

    // Tabla de texto con los valores y el resumen de cada serie.
    const d = opts.decimals === undefined ? 1 : opts.decimals, unit = opts.yUnit ? ' ' + opts.yUnit : '';
    const sum = html('table', 'chart-summary');
    sum.appendChild(html('caption', '', (opts.title || 'Gráfica') + ': resumen'));
    const hr = html('tr');
    ['Serie', 'Último', 'Mínimo', 'Media', 'Máximo'].forEach(h => hr.appendChild(html('th', '', h)));
    sum.appendChild(hr);
    series.forEach(s => {
      const v = s.points.map(p => p[1]).filter(n => num(n));
      const tr = html('tr');
      tr.appendChild(html('th', '', s.name));
      const vals = v.length ? [v[v.length - 1], Math.min.apply(null, v), v.reduce((a, b) => a + b, 0) / v.length, Math.max.apply(null, v)] : [null, null, null, null];
      vals.forEach(n => tr.appendChild(html('td', '', n === null ? '—' : fmt(n, d) + unit)));
      sum.appendChild(tr);
    });
    const det = html('details', 'chart-data');
    det.appendChild(html('summary', '', 'Valores'));
    det.appendChild(sum);
    const tbl = html('table', 'chart-values');
    tbl.appendChild(html('caption', '', (opts.title || 'Gráfica') + ': valores'));
    const h2 = html('tr');
    h2.appendChild(html('th', '', (opts.xLabel || 'X') + (opts.xUnit ? ' (' + opts.xUnit + ')' : '')));
    series.forEach(s => h2.appendChild(html('th', '', s.name + (opts.yUnit ? ' (' + opts.yUnit + ')' : ''))));
    tbl.appendChild(h2);
    const rows = {};
    series.forEach((s, i) => s.points.forEach(p => { if (num(p[0])) { (rows[p[0]] = rows[p[0]] || {})[i] = p[1]; } }));
    Object.keys(rows).map(Number).sort((a, b) => a - b).forEach(x => {
      const tr = html('tr');
      tr.appendChild(html('th', '', fmt(x, 1)));
      series.forEach((s, i) => tr.appendChild(html('td', '', fmt(rows[x][i], d))));
      tbl.appendChild(tr);
    });
    det.appendChild(tbl);
    wrap.appendChild(det);
    container.appendChild(wrap);
    return wrap;
  }

  window.NPerfChart = { render: render };
})();
