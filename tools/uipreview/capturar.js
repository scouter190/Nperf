// Capturas y comprobaciones de la maqueta (spec 008 T5). Solo desarrollo.
// Uso: NODE_PATH=$(npm root -g) node tools/uipreview/capturar.js [escenario,escenario,...]
// Escribe docs/img/gui-v2/<escenario>-<claro|oscuro>-<ancho>x<alto>.png (después se reducen los colores)
// y imprime las comprobaciones: sin scroll horizontal a 720x480 y KPI + gráfica visibles a 1000x640.
const { chromium } = require('playwright');
const { spawn, execFileSync } = require('child_process');
const fs = require('fs');
const os = require('os');
const path = require('path');

const raiz = path.resolve(__dirname, '..', '..');
const salida = path.join(raiz, 'docs', 'img', 'gui-v2') + path.sep;
const TODOS = ['resultado', 'detallado', 'parcial', 'curso', 'cola', 'diagnostico', 'ping', 'internet', 'velocidad', 'vacio', 'anadir', 'sinhist', 'popover', 'mlab',
  'historial', 'historial-registro', 'historial-desactivado', 'exportar', 'borrar', 'red', 'red-inactivas', 'ajustes', 'ajustes-error', 'error'];

(async () => {
  const bin = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'uipreview-')), 'uipreview');
  execFileSync('go', ['build', '-o', bin, './tools/uipreview'], { cwd: raiz });
  const srv = spawn(bin, [], { cwd: raiz });
  const url = await new Promise(r => { let b = ''; srv.stdout.on('data', d => { b += d; const m = b.match(/Maqueta: (http:\/\/\S+)/); if (m) r(m[1]); }); });
  const browser = await chromium.launch();
  fs.mkdirSync(salida, { recursive: true });
  const escenarios = process.argv[2] ? process.argv[2].split(',') : TODOS;
  const fallos = [];
  for (const [esquema, nombre] of [['light', 'claro'], ['dark', 'oscuro']]) {
    for (const [w, h] of [[1000, 640], [720, 480]]) {
      const ctx = await browser.newContext({ colorScheme: esquema, viewport: { width: w, height: h }, deviceScaleFactor: 2 });
      const page = await ctx.newPage();
      const errs = [];
      page.on('pageerror', e => errs.push(String(e)));
      page.on('console', m => { if (m.type() === 'error') errs.push(m.text()); });
      for (const s of escenarios) {
        await page.goto(url + '#' + s + '&c');
        await page.reload();
        await page.waitForTimeout(300);
        const m = await page.evaluate(() => {
          const r = e => { const x = document.querySelector(e); return x && x.offsetParent ? x.getBoundingClientRect() : null; };
          const k = r('.kpis'), g = r('.gcard');
          return { sw: document.documentElement.scrollWidth, iw: innerWidth, ih: innerHeight, kb: k && k.bottom, gb: g && g.bottom };
        });
        const id = `${s} ${nombre} ${w}x${h}`;
        if (w === 720 && m.sw > m.iw) fallos.push(`${id}: scroll horizontal (${m.sw} > ${m.iw})`);
        if (w === 1000 && ['resultado', 'detallado', 'sinhist', 'popover', 'mlab'].includes(s) && !(m.kb && m.gb && m.kb <= m.ih && m.gb <= m.ih)) fallos.push(`${id}: KPI/gráfica fuera de la vista (${m.kb}, ${m.gb} > ${m.ih})`);
        if (errs.length) fallos.push(`${id}: errores de consola ${errs.join(';')}`);
        if (w === 720 || ['resultado', 'detallado', 'sinhist'].includes(s)) console.log(id.padEnd(44), 'scrollWidth', m.sw, '/', m.iw, m.kb ? `KPI.bottom ${Math.round(m.kb)} gráfica.bottom ${Math.round(m.gb)} / ${m.ih}` : '');
        await page.screenshot({ path: `${salida}${s}-${nombre}-${w}x${h}.png` });
      }
      await ctx.close();
    }
  }
  await browser.close();
  srv.kill();
  console.log(fallos.length ? 'FALLOS:\n' + fallos.join('\n') : 'OK: sin scroll horizontal a 720x480 y KPI + gráfica visibles a 1000x640');
  process.exit(fallos.length ? 1 : 0);
})();
