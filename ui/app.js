<script>
'use strict';
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => Array.from(r.querySelectorAll(s));
const el = (tag, attrs = {}, ...kids) => {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === false || v == null) continue;
    if (k === 'class') n.className = v;
    else if (k === 'text') n.textContent = v;
    else if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
    else if (v === true) n.setAttribute(k, '');
    else n.setAttribute(k, v);
  }
  for (const k of kids.flat()) if (k != null && k !== false) n.append(k.nodeType ? k : document.createTextNode(String(k)));
  return n;
};

// ---------------------------------------------------------------- talking to the program
const params = new URLSearchParams(location.search);
let TOKEN = params.get('t') || sessionStorage.getItem('ace8-token') || '';
try { sessionStorage.setItem('ace8-token', TOKEN); history.replaceState(null, '', '/'); } catch (e) {}

// Every open window gives itself a name, so the program can tell when its last window has closed.
const CLIENT = 'w' + Math.random().toString(36).slice(2, 10) + Date.now().toString(36);

async function api(path, body) {
  let res;
  try {
    res = await fetch(path, {
      method: body === undefined ? 'GET' : 'POST',
      headers: { 'X-AC8-Token': TOKEN, 'X-AC8-Client': CLIENT, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (e) {
    throw new Error('The program is not answering. It may have been closed; start it again.');
  }
  let data = null;
  try { data = await res.json(); } catch (e) {}
  if (!res.ok) throw new Error((data && data.error) || (res.status === 403 ? 'This window is out of date. Close it and start the program again.' : 'Request failed (' + res.status + ').'));
  return data;
}
const thumbURL = p => '/api/thumb?t=' + encodeURIComponent(TOKEN) + '&path=' + encodeURIComponent(p);
const newBatch = () => 'b' + Math.random().toString(36).slice(2, 10) + Date.now().toString(36);

function upload(file, batch, onProgress) {
  return new Promise((resolve, reject) => {
    const x = new XMLHttpRequest();
    x.open('POST', '/api/upload?batch=' + batch + '&name=' + encodeURIComponent(file.name));
    x.setRequestHeader('X-AC8-Token', TOKEN);
    x.upload.onprogress = e => { if (e.lengthComputable && onProgress) onProgress(e.loaded / e.total); };
    x.onload = () => {
      let d = null; try { d = JSON.parse(x.responseText); } catch (e) {}
      if (x.status === 200 && d && d.path) resolve(d.path); else reject(new Error((d && d.error) || 'Could not copy ' + file.name + ' into the program.'));
    };
    x.onerror = () => reject(new Error('Could not copy ' + file.name + ' into the program.'));
    x.send(file);
  });
}

// Collect files from a drop, looking one level into dropped folders.
async function droppedFiles(dt) {
  const out = [];
  const entries = [];
  if (dt.items) for (const it of dt.items) { const en = it.webkitGetAsEntry && it.webkitGetAsEntry(); if (en) entries.push(en); }
  if (!entries.length) return Array.from(dt.files || []);
  const fileOf = en => new Promise(res => en.file(res, () => res(null)));
  const readAll = dir => new Promise(res => { const r = dir.createReader(); const all = []; const step = () => r.readEntries(b => { if (!b.length) res(all); else { all.push(...b); step(); } }, () => res(all)); step(); });
  for (const en of entries) {
    if (en.isFile) { const f = await fileOf(en); if (f) out.push(f); }
    else if (en.isDirectory) {
      for (const sub of await readAll(en)) {
        if (sub.isFile) { const f = await fileOf(sub); if (f) out.push(f); }
        else if (sub.isDirectory) for (const s2 of await readAll(sub)) if (s2.isFile) { const f = await fileOf(s2); if (f) out.push(f); }
      }
    }
  }
  return out;
}
const isPicture = name => /\.(png|tga)$/i.test(name);

function notice(target, kind, ...content) {
  target.replaceChildren(el('div', { class: 'notice ' + kind, role: kind === 'error' ? 'alert' : 'status' }, ...content));
}
const openFolderBtn = (path, label = 'Open folder') => el('button', { class: 'btn small', onclick: () => api('/api/open', { path }).catch(e => alertLine(e.message)) }, label);
function alertLine(msg) { const d = $('#dock'); showDockMessage(msg); d.hidden = false; }

// ---------------------------------------------------------------- state
let STATE = null;
async function refreshState() {
  STATE = await api('/api/state');
  const c = STATE.checks;
  const missingExtract = ['workspace', 'retoc', 'paks', 'aes'].filter(k => !c[k].ok);
  const st = $('#setupStatus');
  st.classList.toggle('bad', missingExtract.length > 0);
  $('#setupText').textContent = missingExtract.length ? 'Setup: ' + missingExtract.length + (missingExtract.length === 1 ? ' thing' : ' things') + ' missing. Open Settings.' : 'Setup is complete';
  const ex = $('#exSetup');
  ex.hidden = missingExtract.length === 0;
  if (missingExtract.length) ex.replaceChildren(el('strong', { text: 'Extracting needs a little setup first. ' }), missingExtract.map(k => c[k].note).join(' '), ' ', el('button', { class: 'btn small', onclick: () => showPage('settings') }, 'Open Settings'));
  renderRepackSetup();
  renderEmblemSetup();
  $('#rpInstallHint').textContent = c.mods.ok ? STATE.settings.modsDir : 'Set your mods folder in Settings to use this.';
  $('#rpInstall').disabled = !c.mods.ok;
  if (!c.mods.ok) $('#rpInstall').checked = false;
  if (!$('#spOut').value) $('#spOut').value = STATE.folders.split;
  $('#seAbout').textContent = STATE.app + ' ' + STATE.version + '. Program folder: ' + STATE.folders.data;
  $$('[data-pick-folder],[data-pick-file]').forEach(b => { b.hidden = !STATE.canPick; });
  $('#spPick').hidden = !STATE.canPick; $('#rpPick').hidden = !STATE.canPick; $('#mdPick').hidden = !STATE.canPick;
  $('#imPick').hidden = !STATE.canPick; $('#slPicPick').hidden = !STATE.canPick; $('#seImportOld').disabled = !STATE.canPick;
  setHud();
  return STATE;
}

// The strip along the top: setup, the task that is running, the loader and the new skins.
const HUD = { op: '', loader: null, slots: null };
function setHud() {
  if (!STATE) return;
  const c = STATE.checks;
  const missing = ['workspace', 'retoc', 'paks', 'aes', 'mods'].filter(k => !c[k].ok);
  const s = $('#hudSession');
  s.textContent = missing.length ? 'Setup needed' : 'Ready';
  s.className = 'val ' + (missing.length ? 'bad' : 'good');
  const op = $('#hudOp');
  op.textContent = HUD.op || 'No data'; op.title = HUD.op || '';
  op.className = 'val' + (HUD.op ? ' warn' : '');
  const lo = $('#hudLoader');
  if (HUD.loader == null) { lo.textContent = '—'; lo.className = 'val'; }
  else if (!HUD.loader) { lo.textContent = 'Not found'; lo.className = 'val bad'; }
  else { lo.textContent = 'v' + HUD.loader; lo.className = 'val ' + (versionLess(HUD.loader, '1.1.0') ? 'bad' : 'good'); lo.title = versionLess(HUD.loader, '1.1.0') ? 'New skin slots need AC8 Mod Loader 1.1.0 or newer' : ''; }
  const sl = $('#hudSlots');
  sl.textContent = HUD.slots == null ? '—' : HUD.slots + (HUD.slots === 1 ? ' skin' : ' skins');
  sl.className = 'val' + (HUD.slots ? ' good' : '');
}
function versionLess(a, b) {
  const pa = String(a).split('.').map(Number), pb = String(b).split('.').map(Number);
  for (let i = 0; i < 3; i++) if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) < (pb[i] || 0);
  return false;
}
function renderRepackSetup() {
  if (!STATE) return;
  const c = STATE.checks, rp = $('#rpSetup');
  const need = repack.mode === 'slot' ? ['workspace', 'retoc', 'paks', 'aes', 'mods'].filter(k => !c[k].ok) : ['workspace', 'retoc'].filter(k => !c[k].ok);
  rp.hidden = !need.length;
  if (need.length) rp.replaceChildren(el('strong', { text: repack.mode === 'slot' ? 'New skin slots need a little setup first. ' : 'Packing needs retoc. ' }), need.map(k => c[k].note).join(' '), ' ', el('button', { class: 'btn small', onclick: () => showPage('settings') }, 'Open Settings'));
}

// ---------------------------------------------------------------- pages and appearance
function showPage(name, mode) {
  if (name === 'repack' && mode) setRepackMode(mode);
  $$('.page').forEach(p => { p.hidden = p.id !== 'page-' + name; });
  $$('.nav button[data-page]').forEach(b => {
    const on = b.dataset.page === name && (!b.dataset.mode || b.dataset.mode === repack.mode);
    if (on) b.setAttribute('aria-current', 'page'); else b.removeAttribute('aria-current');
  });
  $('#scroll').scrollTop = 0;
  try { sessionStorage.setItem('ace8-page', name); } catch (e) {}
  if (!activeJob && $('#dock').classList.contains('done')) $('#dock').hidden = true;
  if (name === 'extract') { loadLibrary(); loadVehicles(); }
  if (name === 'repack') { loadLibrary(); loadSlots(true); }
  if (name === 'mods') loadMods();
  if (name === 'slots') loadSlots();
  if (name === 'pilots') loadPilots();
  if (name === 'emblem') showEmblemPage();
  if (name === 'save') loadSave();
  if (name === 'settings') fillSettings();
  if (name === 'links') showLinks();
}
$$('.nav button[data-page]').forEach(b => b.addEventListener('click', () => showPage(b.dataset.page, b.dataset.mode)));
$('#setupStatus').addEventListener('click', () => showPage('settings'));

function applyTheme(t) {
  if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t; else delete document.documentElement.dataset.theme;
  $$('[data-theme-set]').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.themeSet === (t || 'dark'))));
  try { localStorage.setItem('ace8-theme', t || 'dark'); } catch (e) {}
}
$$('[data-theme-set]').forEach(b => b.addEventListener('click', async () => {
  applyTheme(b.dataset.themeSet);
  try { await api('/api/settings', { theme: b.dataset.themeSet }); } catch (e) {}
}));

async function pickInto(inputId, kind, filter, title) {
  const input = $('#' + inputId);
  try {
    const r = await api('/api/pick', { kind, filter: filter || '', title: title || 'Choose', start: input.value });
    if (r.paths && r.paths.length) { input.value = r.paths[0]; input.dispatchEvent(new Event('change', { bubbles: true })); }
  } catch (e) { alertLine(e.message); }
}
$$('[data-pick-folder]').forEach(b => b.addEventListener('click', ev => { ev.preventDefault(); pickInto(b.dataset.pickFolder, 'folder', '', 'Choose a folder'); }));
$$('[data-pick-file]').forEach(b => b.addEventListener('click', ev => { ev.preventDefault(); pickInto(b.dataset.pickFile, 'file', b.dataset.filter, 'Choose a file'); }));

function wireDrop(zone, onFiles) {
  let depth = 0;
  zone.addEventListener('dragenter', e => { e.preventDefault(); depth++; zone.classList.add('over'); });
  zone.addEventListener('dragover', e => { e.preventDefault(); e.dataTransfer.dropEffect = 'copy'; });
  zone.addEventListener('dragleave', () => { depth = Math.max(0, depth - 1); if (!depth) zone.classList.remove('over'); });
  zone.addEventListener('drop', async e => { e.preventDefault(); depth = 0; zone.classList.remove('over'); onFiles(await droppedFiles(e.dataTransfer)); });
}
// A picture dropped outside a drop zone must not replace the page.
window.addEventListener('dragover', e => e.preventDefault());
window.addEventListener('drop', e => e.preventDefault());

// ---------------------------------------------------------------- tasks (dock)
let activeJob = null;
function showDockMessage(msg) {
  $('#dock').className = 'dock failed';
  $('#dockTitle').textContent = 'Problem';
  $('#dockStep').textContent = msg; $('#dockStep').title = msg;
  $('#dockBar').className = 'bar'; $('#dockBar').firstElementChild.style.width = '100%';
  $('#dockCancel').hidden = true; $('#dockClose').hidden = false; $('#dockLogBtn').hidden = true; $('#dockLog').hidden = true;
  $('#dockReport').hidden = true;
}

// The support report: copied to the clipboard, and shown so the person can see what they are sharing.
function legacyCopy(text) {
  const ta = el('textarea', { style: 'position:fixed;left:-9999px;top:0', 'aria-hidden': 'true' });
  ta.value = text; document.body.append(ta); ta.select();
  let ok = false; try { ok = document.execCommand('copy'); } catch (e) {}
  ta.remove(); return ok;
}
async function copyText(text) {
  try { await navigator.clipboard.writeText(text); return true; } catch (e) { return legacyCopy(text); }
}
async function copyReport(out) {
  let text;
  try { text = (await api('/api/report')).text; } catch (e) { notice(out, 'error', e.message); return false; }
  const ok = await copyText(text);
  out.replaceChildren(el('div', { class: 'notice ' + (ok ? 'good' : 'warn'), role: 'status' },
    el('strong', { text: ok ? 'Copied. ' : 'Could not copy it for you. ' }),
    ok ? 'Paste it into your message. This is exactly what was copied:' : 'Click the text below to select it, then press Ctrl+C:',
    el('pre', { class: 'report', tabindex: '0', text })));
  return ok;
}
$('#dockReport').addEventListener('click', async () => {
  const b = $('#dockReport');
  showPage('settings');
  const ok = await copyReport($('#seReportOut'));
  $('#seReportOut').scrollIntoView({ block: 'nearest' });
  b.textContent = ok ? 'Copied' : 'Copy report'; setTimeout(() => { b.textContent = 'Copy report'; }, 2500);
});
$('#dockClose').addEventListener('click', () => { $('#dock').hidden = true; });
$('#dockLogBtn').addEventListener('click', () => {
  const log = $('#dockLog'); log.hidden = !log.hidden;
  $('#dockLogBtn').setAttribute('aria-expanded', String(!log.hidden));
  $('#dockLogBtn').textContent = log.hidden ? 'Show details' : 'Hide details';
  log.scrollTop = log.scrollHeight;
});
$('#dockCancel').addEventListener('click', () => { if (activeJob) fetch('/api/job?id=' + activeJob + '&cancel=1', { headers: { 'X-AC8-Token': TOKEN } }); });

// Starts watching a task; resolves with its result or rejects with its error.
function watchJob(id) {
  activeJob = id;
  const dock = $('#dock'), log = $('#dockLog');
  dock.hidden = false; dock.className = 'dock';
  log.textContent = ''; $('#dockLogBtn').hidden = false; $('#dockReport').hidden = true;
  $('#dockCancel').hidden = false; $('#dockClose').hidden = true;
  let since = 0;
  setBusy(true);
  return new Promise((resolve, reject) => {
    const tick = async () => {
      let v;
      try { v = await api('/api/job?id=' + id + '&since=' + since); }
      catch (e) { setBusy(false); showDockMessage(e.message); reject(e); return; }
      since = v.next;
      v.lines = v.lines || [];
      if (v.lines.length) { const atEnd = log.scrollTop + log.clientHeight >= log.scrollHeight - 8; log.append(v.lines.join('\n') + '\n'); if (atEnd) log.scrollTop = log.scrollHeight; }
      $('#dockTitle').textContent = v.title;
      HUD.op = v.status === 'running' ? v.title : ''; setHud();
      const bar = $('#dockBar');
      if (v.status === 'running') {
        $('#dockStep').textContent = v.step || 'Working'; $('#dockStep').title = v.step || '';
        bar.className = 'bar' + (v.progress < 0 ? ' unknown' : '');
        bar.firstElementChild.style.width = v.progress < 0 ? '' : Math.round(v.progress * 100) + '%';
        setTimeout(tick, 350);
        return;
      }
      activeJob = null; setBusy(false);
      bar.className = 'bar'; bar.firstElementChild.style.width = '100%';
      $('#dockCancel').hidden = true; $('#dockClose').hidden = false;
      dock.className = 'dock ' + (v.status === 'done' ? 'done' : 'failed');
      $('#dockReport').hidden = v.status !== 'failed';
      const secs = Math.max(1, Math.round(v.seconds));
      if (v.status === 'done') { $('#dockStep').textContent = 'Finished in ' + secs + ' s'; resolve(v.result); }
      else { const msg = v.error || 'Stopped.'; $('#dockStep').textContent = msg; $('#dockStep').title = msg; const err = new Error(msg); err.result = v.result; reject(err); }
    };
    tick();
  });
}
function setBusy(b) { $$('.btn.primary, #exFind').forEach(x => { x.dataset.busy = b ? '1' : ''; }); if (!b) { HUD.op = ''; setHud(); } updateButtons(); }
async function runJob(path, body) { const r = await api(path, body === undefined ? {} : body); return watchJob(r.job); }

// ---------------------------------------------------------------- split
const split = { files: [], batches: [] };
function stripFor(n, imgPath, url) {
  n = Math.max(1, Math.min(n || 5, 20));
  const wrap = el('div', { class: 'strip-wrap', style: '--n:' + n });
  url = url || (imgPath ? thumbURL(imgPath) : '');
  const strip = el('div', { class: 'strip' + (url ? ' has-img' : ''), style: '--n:' + n });
  if (url) strip.append(el('img', { src: url, alt: '', loading: 'lazy' }));
  const tags = el('div', { class: 'tags', 'aria-hidden': 'true' });
  for (let i = 0; i < n; i++) { strip.append(el('div', { class: 'cell' })); tags.append(el('span', { text: String(1001 + i) })); }
  wrap.append(strip, tags);
  return wrap;
}
function renderSplit() {
  const box = $('#spFiles');
  const piecesSel = +$('#spPieces').value;
  box.replaceChildren(...split.files.map((f, i) => {
    const n = piecesSel || f.pieces;
    const card = el('div', { class: 'filecard' });
    const meta = f.uploading != null ? 'Copying into the program…' : f.error ? '' : f.width + ' × ' + f.height + ' pixels' + (n ? ', ' + n + (n === 1 ? ' piece of ' : ' pieces of ') + Math.round(f.width / n) + ' × ' + f.height : '');
    card.append(el('div', { class: 'top' }, el('span', { class: 'name', text: f.name }), el('span', { class: 'meta', text: meta }), el('span', { class: 'spacer' }),
      el('button', { class: 'btn small', onclick: () => { split.files.splice(i, 1); renderSplit(); }, 'aria-label': 'Remove ' + f.name }, 'Remove')));
    if (f.uploading != null) { const bar = el('div', { class: 'upbar' }, el('i', { style: 'width:' + Math.round(f.uploading * 100) + '%' })); card.append(bar); }
    else if (f.error) card.append(el('div', { class: 'notice error', text: f.error }));
    else if (!n) card.append(el('div', { class: 'notice warn', text: 'The width is not a whole number of squares, so choose how many pieces to cut under "Pieces".' }));
    else if (f.width % n) card.append(el('div', { class: 'notice error', text: 'A picture ' + f.width + ' pixels wide cannot be cut into ' + n + ' equal pieces.' }));
    else card.append(stripFor(n, f.path));
    return card;
  }));
  $('#spEmptyStrip').hidden = split.files.length > 0;
  $('#spClear').hidden = split.files.length === 0;
  updateButtons();
}
function splitReady() {
  const piecesSel = +$('#spPieces').value;
  return split.files.filter(f => f.uploading == null && !f.error && (piecesSel || f.pieces) && f.width % (piecesSel || f.pieces) === 0);
}
async function addSplitPaths(paths) {
  if (!paths.length) return;
  const r = await api('/api/inspect', { paths });
  for (const f of r.files) if (!split.files.some(x => x.path === f.path)) split.files.push(f);
  renderSplit();
}
wireDrop($('#spDrop'), async files => {
  const pics = files.filter(f => isPicture(f.name));
  if (!pics.length) { notice($('#spResult'), 'error', 'Only PNG and TGA pictures can be split.'); return; }
  $('#spResult').replaceChildren();
  const batch = newBatch(); split.batches.push(batch);
  for (const file of pics) {
    const entry = { name: file.name, uploading: 0, batch };
    split.files.push(entry); renderSplit();
    try {
      let last = 0;
      const path = await upload(file, batch, p => { entry.uploading = p; if (p - last > 0.04) { last = p; renderSplit(); } });
      const r = await api('/api/inspect', { paths: [path] });
      Object.assign(entry, r.files[0], { uploading: null, name: file.name });
    } catch (e) { entry.uploading = null; entry.error = e.message; }
    renderSplit();
  }
});
$('#spPick').addEventListener('click', async () => {
  try { const r = await api('/api/pick', { kind: 'files', filter: 'images', title: 'Choose skin pictures' }); await addSplitPaths(r.paths || []); }
  catch (e) { notice($('#spResult'), 'error', e.message); }
});
$('#spPieces').addEventListener('change', renderSplit);
// Uploaded copies are deleted once no listed picture still needs them.
function dropSplitBatches() {
  const inUse = new Set(split.files.map(f => f.batch).filter(Boolean));
  const gone = split.batches.filter(b => !inUse.has(b));
  split.batches = split.batches.filter(b => inUse.has(b));
  if (gone.length) api('/api/inbox/clean', { batches: gone }).catch(() => {});
}
$('#spClear').addEventListener('click', () => { split.files = []; $('#spResult').replaceChildren(); renderSplit(); dropSplitBatches(); });
$('#spGo').addEventListener('click', async () => {
  const ready = splitReady();
  const out = $('#spResult'); out.replaceChildren();
  try {
    const res = await runJob('/api/split', { paths: ready.map(f => f.path), pieces: +$('#spPieces').value, painter: $('#spPainter').checked, format: $('#spFormat').value, outDir: $('#spOut').value, group: $('#spGroup').checked, cleanup: [] });
    showSplitResult(res, null);
    split.files = split.files.filter(f => !ready.includes(f)); renderSplit();
    dropSplitBatches();
  } catch (e) { showSplitResult(e.result, e.message); }
});
function showSplitResult(res, errMsg) {
  const out = $('#spResult');
  if (!res || !res.results) { notice(out, 'error', errMsg || 'Splitting failed.'); return; }
  const ok = res.results.filter(r => !r.error), bad = res.results.filter(r => r.error);
  const total = ok.reduce((a, r) => a + r.files.length, 0);
  const box = el('div', { class: 'notice ' + (bad.length ? (ok.length ? 'warn' : 'error') : 'good') + ' results' });
  if (ok.length) {
    box.append(el('p', {}, el('strong', { text: 'Wrote ' + total + ' files from ' + ok.length + (ok.length === 1 ? ' picture' : ' pictures') + '.' })));
    const dirs = Array.from(new Set(ok.map(r => r.dir || res.outDir)));
    for (const d of dirs) box.append(el('div', { class: 'row', style: 'margin-top:6px' }, el('span', { class: 'path grow', style: 'display:inline', text: d }), openFolderBtn(d), el('button', { class: 'btn small', onclick: () => useInRepack(d) }, 'Use in Repack')));
  }
  for (const b of bad) box.append(el('p', {}, el('strong', { text: b.source + ': ' }), b.error));
  if (ok.length) box.append(el('details', { class: 'files' }, el('summary', { text: 'Show the file names' }), el('ul', {}, ok.flatMap(r => r.files).map(f => el('li', { class: 'path', text: f.path.split(/[\\/]/).pop() })))));
  out.replaceChildren(box);
}

function useInRepack(dir) {
  repack.paintDir = dir; repack.files = []; repack.choices = {}; repack.touched = false;
  showPage('repack'); replan();
}

// ---------------------------------------------------------------- library and aircraft list
// ---------------------------------------------------------------- model
const model = { paths: [], names: {}, mats: [], keep: new Set(), batches: [] };
const isModel = name => /\.glb$/i.test(name);
async function addModels(paths, names) {
  const err = $('#mdError'); err.replaceChildren();
  for (const p of paths) if (!model.paths.includes(p)) model.paths.push(p);
  if (!model.paths.length) return;
  try {
    const r = await api('/api/model/inspect', { paths: model.paths });
    const first = !model.mats.length;
    const known = new Set(model.mats.map(m => m.file + '|' + m.name));
    for (const m of r.materials) if (m.suggested && !known.has(m.file + '|' + m.name)) model.keep.add(m.file + '|' + m.name);
    model.mats = r.materials;
    if (first || !$('#mdName').value) {
      const body = r.materials.slice().sort((a, b) => b.vertices - a.vertices).find(m => m.suggested) || r.materials[0];
      if (body) $('#mdName').value = body.file.replace(/\.glb$/i, '').replace(/[^A-Za-z0-9_-]+/g, '_') + '_paint';
    }
  } catch (e) { model.paths = model.paths.filter(p => !paths.includes(p)); notice(err, 'error', e.message); }
  renderModel();
}
function renderModel() {
  $('#mdBlock').hidden = !model.mats.length;
  const order = model.mats.map((m, i) => [m, i]).sort((a, b) => (b[0].suggested - a[0].suggested) || (a[1] - b[1])).map(x => x[0]);
  const rows = order.map(m => {
    const key = m.file + '|' + m.name;
    return el('tr', {},
      el('td', { class: 'tick' }, el('input', { type: 'checkbox', checked: model.keep.has(key), 'aria-label': 'Keep ' + m.name + ' from ' + m.file, onchange: ev => { if (ev.target.checked) model.keep.add(key); else model.keep.delete(key); updateButtons(); } })),
      el('td', {}, el('div', { class: 'tname', text: m.name }), el('div', { class: 'role', text: m.file })),
      el('td', { text: m.triangles.toLocaleString() + ' triangles' }),
      el('td', { text: m.tiles > 0 ? m.tiles + (m.tiles === 1 ? ' tile' : ' tiles') : 'none' }));
  });
  $('#mdTable').replaceChildren(el('thead', {}, el('tr', {}, el('th', { text: '' }), el('th', { text: 'Material and file' }), el('th', { text: 'Size' }), el('th', { text: 'Texture area' }))), el('tbody', {}, rows));
  updateButtons();
}
wireDrop($('#mdDrop'), async files => {
  const glbs = files.filter(f => isModel(f.name));
  if (!glbs.length) { notice($('#mdError'), 'error', 'Only .glb models can be used here. In FModel, export the model as glTF (.glb).'); return; }
  const batch = newBatch(); model.batches.push(batch);
  const up = $('#mdUp'), bar = $('.upbar i', up); up.hidden = false;
  const paths = [];
  try { for (let i = 0; i < glbs.length; i++) paths.push(await upload(glbs[i], batch, p => { bar.style.width = Math.round((i + p) / glbs.length * 100) + '%'; })); }
  catch (e) { notice($('#mdError'), 'error', e.message); }
  up.hidden = true; bar.style.width = '0';
  await addModels(paths);
});
$('#mdPick').addEventListener('click', async () => {
  try { const r = await api('/api/pick', { kind: 'files', filter: 'glb', title: 'Choose .glb models' }); await addModels(r.paths || []); }
  catch (e) { notice($('#mdError'), 'error', e.message); }
});
$('#mdClear').addEventListener('click', () => { model.paths = []; model.mats = []; model.keep = new Set(); $('#mdResult').replaceChildren(); $('#mdError').replaceChildren(); renderModel(); });
$('#mdName').addEventListener('input', () => updateButtons());
$('#mdGo').addEventListener('click', async () => {
  const out = $('#mdResult'); out.replaceChildren();
  try {
    const res = await api('/api/model/build', { paths: model.paths, keep: Array.from(model.keep), name: $('#mdName').value, moveLoose: $('#mdLoose').checked, cleanup: [] });
    const box = el('div', { class: 'notice good results' }, el('p', {}, el('strong', { text: 'Saved. The skin covers ' + res.tiles + (res.tiles === 1 ? ' tile. ' : ' tiles. ') }), openFolderBtn(res.path, 'Show the file')), el('span', { class: 'path', text: res.path }),
      el('p', { class: 'hint', text: 'In Substance Painter, create the project from this file with "Use UV Tile workflow" switched on.' }));
    for (const n of res.notes) box.append(el('p', { class: 'hint', text: 'Note: ' + n + '.' }));
    out.replaceChildren(box);
  } catch (e) { notice(out, 'error', e.message); }
});

let LIB = [];
const KIND_LABEL = { colour: 'colour', finish: 'finish (metal and roughness)', normal: 'normal map', other: 'other' };
async function loadLibrary(fresh) {
  try { LIB = (await api('/api/library' + (fresh === true ? '?fresh=1' : ''))).skins; } catch (e) { LIB = []; }
  repack.guess = null;
  renderLibrary(); fillSkinSelect();
}
function renderLibrary() {
  const box = $('#libList');
  if (!LIB.length) { box.replaceChildren(el('div', { class: 'notice', text: 'Nothing extracted yet. Extract an aircraft above, or add a folder of raw files you already have.' })); return; }
  box.replaceChildren(el('div', { class: 'list' }, LIB.map(sk => {
    const usable = sk.textures.filter(t => t.width > 0);
    const face = usable.find(t => t.kind === 'colour') || usable[0];
    return el('div', { class: 'skin' },
      el('div', { class: 'line' },
        el('div', { class: 'id' }, sk.aircraft + ' / ' + sk.folder, el('small', { text: usable.length + (usable.length === 1 ? ' skin texture' : ' skin textures') })),
        el('div', { class: 'chips' }, sk.textures.map(t => el('span', { class: 'chip' + (t.width > 0 ? '' : ' off'), title: t.width > 0 ? t.format.replace('PF_', '') + ', ' + t.width + ' × ' + t.height + ', ' + (t.plain ? 'one picture' : (t.blocksW * t.blocksH) + ' tiles') : (t.reason || ''), text: t.name.replace(/^T_/, '') }))),
        el('div', { class: 'row' },
          el('button', { class: 'btn small', disabled: !usable.length, onclick: () => makeTiles(usable) }, 'Make tiles'),
          el('button', { class: 'btn small', disabled: !sk.textures.some(t => t.supported), onclick: () => { showPage('repack', 'replace'); chooseSkin(sk.dir); skinChanged(); } }, 'Repack this skin'),
          el('button', { class: 'btn small', disabled: !sk.textures.some(t => t.supported), onclick: () => { showPage('repack', 'slot'); chooseSkin(sk.dir); skinChanged(); } }, 'New skin from it'),
          openFolderBtn(sk.dir, 'Open'))),
      face && face.blocksH === 1 ? stripFor(face.blocksW, null, '/api/texthumb?t=' + encodeURIComponent(TOKEN) + '&uexp=' + encodeURIComponent(face.uexp)) : null);
  })));
}
async function makeTiles(texs) {
  const out = $('#exResult'); out.replaceChildren();
  try {
    const res = await runJob('/api/export', { uexps: texs.map(t => t.uexp), painter: true, format: $('#exFormat').value });
    notice(out, 'good', el('strong', { text: 'Wrote ' + res.files + ' tile files. ' }), openFolderBtn(res.paintDir), el('span', { class: 'path', style: 'display:block', text: res.paintDir }));
  } catch (e) { notice(out, 'error', e.message); }
}
const samePath = (a, b) => a.replace(/[\\/]+$/, '').toLowerCase() === b.replace(/[\\/]+$/, '').toLowerCase();
const underPath = (p, dir) => { p = p.toLowerCase().replace(/\\/g, '/'); dir = dir.toLowerCase().replace(/\\/g, '/').replace(/\/+$/, ''); return p === dir || p.startsWith(dir + '/'); };
async function addRawFolder(ev) {
  const fromRepack = ev && ev.currentTarget && ev.currentTarget.id === 'rpAddRaw';
  const out = fromRepack ? $('#rpSkinAuto') : $('#exResult');
  try {
    const r = await api('/api/pick', { kind: 'folder', title: 'Choose a folder that holds raw .uasset, .uexp and .ubulk files' });
    if (!r.paths || !r.paths.length) return;
    const dir = r.paths[0];
    const before = STATE.settings.extraRawDirs || [];
    const list = before.filter(p => !samePath(p, dir)).concat(dir);
    await api('/api/settings', { extraRawDirs: list }); await refreshState(); await loadLibrary();
    const found = LIB.filter(sk => underPath(sk.dir, dir) && sk.textures.some(t => t.supported));
    if (!found.length) {
      // nothing usable there: most likely the painted pictures, which belong in the next step
      await api('/api/settings', { extraRawDirs: before }); await refreshState(); await loadLibrary();
      const msg = 'No raw game textures were found in that folder (a .uexp with its .uasset and .ubulk). ' +
        'Your painted pictures do not go here: choose them under “Your painted files” below.';
      if (fromRepack) skinNote('warn', el('span', { text: msg }));
      else notice(out, 'warn', msg);
      return;
    }
    if (fromRepack) {
      const g = (repack.paintDir || repack.files.length) ? await pictureGuess() : { found: false };
      const pick = (g.found && found.find(sk => sk.dir === g.dir)) || (found.length === 1 ? found[0] : null);
      if (pick && chooseSkin(pick.dir)) skinChanged();
      else skinNote('ok', el('span', { text: found.length + ' skins were found there. Choose one in the list above.' }));
    }
  } catch (e) { alertLine(e.message); }
}
$('#libAdd').addEventListener('click', addRawFolder);
$('#rpAddRaw').addEventListener('click', addRawFolder);
$('#libRefresh').addEventListener('click', () => loadLibrary(true));
$('#libOpenPaint').addEventListener('click', () => api('/api/open', { path: STATE.folders.paint }).catch(() => notice($('#exResult'), 'warn', 'There is no paint-ready folder yet. It appears after the first tiles are made.')));

let VEHICLES = [];
const exSkins = new Set();
const exPrev = { vehicle: '', previews: [], missing: [], stamp: 0 };
function currentVehicle() {
  const name = $('#exFilter').value.trim().toLowerCase();
  return VEHICLES.find(x => x.name.toLowerCase() === name || x.path.toLowerCase() === name);
}
function setPreviews(idx) {
  exPrev.vehicle = idx.vehicle || ''; exPrev.previews = idx.previews || []; exPrev.missing = idx.missing || []; exPrev.stamp = Date.now();
}
function toggleSkin(k) { if (exSkins.has(k)) exSkins.delete(k); else exSkins.add(k); renderSkinPicker(); }
function renderSkinPicker() {
  const v = currentVehicle();
  const all = document.querySelector('input[name=exScope]:checked').value === 'all';
  $('#exSkinsBox').hidden = !v || all;
  if (!v) { exSkins.clear(); return; }
  for (const k of Array.from(exSkins)) if (!v.skins.includes(k)) exSkins.delete(k);
  // previews saved earlier for this aircraft are loaded the first time it is picked
  if (exPrev.vehicle !== v.name) {
    setPreviews({ vehicle: v.name });
    api('/api/previews?vehicle=' + encodeURIComponent(v.name)).then(idx => { if (idx.vehicle === v.name && currentVehicle() === v) { setPreviews(idx); renderSkinPicker(); } }).catch(() => {});
  }
  const shown = exPrev.previews.filter(p => v.skins.includes(p.skin));
  const withPicture = new Set(shown.map(p => p.skin));
  const grid = $('#exPreviews');
  grid.hidden = !shown.length;
  grid.replaceChildren(...shown.map(p => {
    const on = exSkins.has(p.skin);
    const url = '/api/previewimg?t=' + encodeURIComponent(TOKEN) + '&vehicle=' + encodeURIComponent(v.name) + '&skin=' + encodeURIComponent(p.skin) + '&v=' + exPrev.stamp;
    return el('div', { class: 'pv' + (on ? ' on' : ''), onclick: () => toggleSkin(p.skin) },
      el('div', { class: 'top' },
        el('button', { type: 'button', class: 'chip', 'aria-pressed': String(on), 'aria-label': 'Skin folder ' + p.skin + (on ? ', picked' : ''), onclick: ev => { ev.stopPropagation(); toggleSkin(p.skin); } }, p.skin),
        el('span', { class: 'tex', text: p.texture + (p.tiles ? ', ' + (p.tiles * (p.rows || 1)) + ' tiles' : '') })),
      (p.rows || 1) === 1 ? stripFor(p.tiles, null, url) : el('img', { src: url, alt: '', style: 'width:100%;display:block' }));
  }));
  const rest = v.skins.filter(k => !withPicture.has(k));
  $('#exSkins').replaceChildren(...rest.map(k => el('button', { type: 'button', class: 'chip', 'aria-pressed': String(exSkins.has(k)), title: shown.length ? 'No picture could be made for this folder' : null, onclick: () => toggleSkin(k) }, k)));
  $('#exPreview').textContent = shown.length ? 'Refresh the previews' : 'Preview skins';
  $('#exPreviewClear').hidden = !shown.length;
  $('#exPreviewNote').textContent = shown.length
    ? shown.length + (shown.length === 1 ? ' preview' : ' previews') + '. Click a skin to pick it. The full-size files fetched to make these were deleted again; only these small pictures are kept.' + (rest.length ? ' Folders without a picture are listed below.' : '')
    : 'Preview skins shows what each folder looks like before you extract. It fetches only each skin\'s colour texture, makes a small picture of it, and deletes the full-size file again.';
}
$('#exPreview').addEventListener('click', async () => {
  const v = currentVehicle(); if (!v) return;
  const out = $('#exResult'); out.replaceChildren();
  try { const idx = await runJob('/api/preview', { vehicle: v.name }); setPreviews(idx); }
  catch (e) { if (e.result && e.result.previews) setPreviews(e.result); notice(out, 'error', e.message); }
  renderSkinPicker();
});
$('#exPreviewClear').addEventListener('click', async () => {
  const v = currentVehicle(); if (!v) return;
  try { await api('/api/previews/clear', { vehicle: v.name }); } catch (e) {}
  setPreviews({ vehicle: v.name }); renderSkinPicker();
});
$$('input[name=exScope]').forEach(r => r.addEventListener('change', renderSkinPicker));
async function loadVehicles() { try { VEHICLES = (await api('/api/vehicles')).vehicles; } catch (e) { VEHICLES = []; } renderVehicles(); }
function renderVehicles() {
  renderSkinPicker();
  const box = $('#exVehiclesBox');
  if (!VEHICLES.length) { box.hidden = true; $('#exFind').textContent = 'Find aircraft in the game files'; return; }
  $('#exFind').textContent = 'Refresh the aircraft list';
  box.hidden = false;
  const q = $('#exFilter').value.trim().toLowerCase();
  const hits = VEHICLES.filter(v => !q || v.name.toLowerCase().includes(q) || v.path.toLowerCase().includes(q));
  $('#exVehiclesHint').textContent = hits.length + ' of ' + VEHICLES.length + ' vehicles' + (q ? ' match what you typed' : ' in the game. Type to narrow the list, or pick one') + '.';
  $('#exVehicles').replaceChildren(...hits.slice(0, 300).map(v => el('button', { type: 'button', onclick: () => { $('#exFilter').value = v.name; renderVehicles(); updateButtons(); } },
    el('span', { class: 'n', text: v.name }), el('span', { class: 's', text: v.skins.length + (v.skins.length === 1 ? ' texture folder' : ' texture folders') }))));
}
$('#exFilter').addEventListener('input', () => { renderVehicles(); updateButtons(); });
$('#exFind').addEventListener('click', async () => {
  const out = $('#exResult'); out.replaceChildren();
  try { const res = await runJob('/api/vehicles/find'); VEHICLES = res.vehicles; renderVehicles(); }
  catch (e) { notice(out, 'error', e.message); }
});
$('#exGo').addEventListener('click', async () => {
  const out = $('#exResult'); out.replaceChildren();
  try {
    const res = await runJob('/api/extract', { filter: $('#exFilter').value, skins: Array.from(exSkins), scope: $('input[name=exScope]:checked').value, convert: $('#exConvert').checked, painter: true, format: $('#exFormat').value });
    const box = el('div', { class: 'notice good results' }, el('p', {}, el('strong', { text: 'Extracted ' + res.textures + ' textures in ' + res.skins + (res.skins === 1 ? ' skin folder' : ' skin folders') + '. ' }), openFolderBtn(res.rawDir, 'Open raw files')));
    if (res.paintDir) box.append(el('p', {}, 'Wrote ' + res.files + ' paint-ready tile files. ', openFolderBtn(res.paintDir, 'Open tiles')), el('span', { class: 'path', text: res.paintDir }));
    out.replaceChildren(box);
  } catch (e) { notice(out, 'error', e.message); }
  loadLibrary();
});

// ---------------------------------------------------------------- repack
const repack = { mode: 'replace', paintDir: '', files: [], batches: [], rows: [], picked: new Set(), touched: false, choices: {}, iconPicture: '', iconBatch: '', rebuild: null, guess: null, autoSkin: '' };
const REPACK_TEXT = {
  replace: { eyebrow: 'Build // 04', title: 'Repack into a mod', lead: 'Turns the pictures you exported from Substance Painter or Blender back into the game\'s format, puts them in the right folders and packs them into a mod that replaces one of the game\'s skins.', skin: 'Skin to replace', build: 'Build the mod', step: '04' },
  slot: { eyebrow: 'Build // 04', title: 'New skin slot', lead: 'Adds your paint to the game as a skin of its own, next to the game\'s skins, with its own name, description and icon. Nothing of the game is replaced, and any number of new skins can be installed together.', skin: 'Skin to start from', build: 'Build the new skin', step: '05' },
};
function setRepackMode(mode) {
  repack.mode = mode === 'slot' ? 'slot' : 'replace';
  const t = REPACK_TEXT[repack.mode];
  $('#rpEyebrow').textContent = t.eyebrow; $('#rpTitle').textContent = t.title; $('#rpLead').textContent = t.lead;
  $('#rpSkinHead').textContent = t.skin; $('#rpBuildHead').textContent = t.build; $('#rpBuildStep').textContent = t.step;
  $$('[data-rpmode]').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.rpmode === repack.mode)));
  $$('#page-repack .slot-only').forEach(n => { n.hidden = repack.mode !== 'slot'; });
  $$('#page-repack .replace-only').forEach(n => { n.hidden = repack.mode !== 'replace'; });
  $$('.nav button[data-page="repack"]').forEach(b => { if (!$('#page-repack').hidden && b.dataset.mode === repack.mode) b.setAttribute('aria-current', 'page'); else b.removeAttribute('aria-current'); });
  try { sessionStorage.setItem('ace8-rpmode', repack.mode); } catch (e) {}
  renderRepackSetup(); templateHint(); fillModSelect(); renderIconPreview(); updateButtons();
}
$$('[data-rpmode]').forEach(b => b.addEventListener('click', () => setRepackMode(b.dataset.rpmode)));
// The skins step 01 offers: one entry per game skin, grouped by aircraft. Copies of the same skin
// found in several raw folders are shown once (the workspace's own copy first, then the most
// complete one), unless another copy is the one chosen.
const supportedCount = sk => sk.textures.filter(t => t.supported).length;
function betterCopy(a, b) {
  if (a.root !== b.root) return a.root < b.root;
  if (supportedCount(a) !== supportedCount(b)) return supportedCount(a) > supportedCount(b);
  if (a.dir.length !== b.dir.length) return a.dir.length < b.dir.length;
  return a.dir < b.dir;
}
function skinChoices(keep) {
  const best = new Map();
  for (const sk of LIB) {
    if (!sk.textures.some(t => t.supported)) continue;
    const id = (sk.package || sk.dir).toLowerCase();
    const have = best.get(id);
    if (!have || sk.dir === keep || (have.dir !== keep && betterCopy(sk, have))) best.set(id, sk);
  }
  return Array.from(best.values());
}
function fillSkinSelect(keep) {
  const sel = $('#rpSkin');
  keep = keep || sel.value;
  const skins = skinChoices(keep);
  const groups = new Map();
  for (const sk of skins) { if (!groups.has(sk.aircraft)) groups.set(sk.aircraft, []); groups.get(sk.aircraft).push(sk); }
  sel.replaceChildren(el('option', { value: '', text: skins.length ? 'Choose a skin' : 'No extracted skins yet' }),
    ...Array.from(groups, ([aircraft, list]) => el('optgroup', { label: aircraft }, list.map(sk => el('option', { value: sk.dir, title: sk.dir, text: sk.aircraft + ' / ' + sk.folder })))));
  sel.value = skins.some(sk => sk.dir === keep) ? keep : '';
  $('#rpSkinHint').hidden = skins.length > 0;
}
// chooseSkin sets step 01 from the program's side (a guess, a button elsewhere), as if picked by hand
function chooseSkin(dir) {
  if (!LIB.some(sk => sk.dir === dir)) return false;
  fillSkinSelect(dir);
  return $('#rpSkin').value === dir;
}
function skinChanged() { repack.touched = false; repack.autoSkin = ''; $('#rpSkinAuto').hidden = true; templateHint(); if (repack.mode === 'slot') renderIconPreview(); replan(); }
// One small picture per tile, side by side, so it is plain which pictures go into a texture.
function sourceStrip(src, plain) {
  // a texture that is one picture (no tiles): just the picture
  if (plain) return el('div', { class: 'srcstrip', style: '--cols:1;max-width:260px' }, el('img', { src: thumbURL(src.files[0]) + '&w=900', alt: '', loading: 'lazy' }));
  // one wide picture: show it with the lines where the tiles fall, like the Split screen does
  if (src.wide && (src.rows || 1) === 1) { const w = stripFor(src.cols, null, thumbURL(src.files[0]) + '&w=900'); w.classList.add('srcwide'); return w; }
  const box = el('div', { class: 'srcstrip' + (src.wide ? ' wide' : ''), style: '--cols:' + Math.max(1, Math.min(src.cols || src.files.length, 20)) });
  for (const f of src.files) box.append(el('img', { src: thumbURL(f) + '&w=' + (src.wide ? 900 : 180), alt: '', loading: 'lazy' }));
  return box;
}
function planCell(x) {
  const rec = x.recipe, out = [];
  const waiting = (rec.choices || []).some(c => !c.picked); // a set still has to be chosen: the box below says so
  if (!rec.ready && !waiting) out.push(el('div', { class: 'no', text: 'Not available: ' + (rec.missing || 'no matching pictures') }));
  for (const c of rec.choices || []) {
    const sel = el('select', { 'aria-label': 'Which ' + c.label + ' pictures to use for ' + x.name, onchange: ev => { repack.choices[c.key] = ev.target.value; replan(); } },
      c.picked ? null : el('option', { value: '', text: 'Choose a set' }),
      c.options.map(o => el('option', { value: o.name, selected: o.name === c.picked, text: o.name + (o.tiles ? ' (' + o.tiles + (o.tiles === 1 ? ' tile)' : ' tiles)') : ' (one wide picture)') })));
    out.push(el('label', { class: 'choice' + (c.picked ? ' settled' : '') }, el('span', {}, el('strong', { text: c.options.length + ' sets of ' + (/^(tex|kind):/.test(c.key) ? '' : c.label + ' ') + 'pictures were found. ' }), c.picked ? 'Using' : 'Choose which to use:'), sel));
  }
  for (const s of rec.ready ? rec.sources || [] : []) {
    const head = el('div', { class: 'srchead' }, el('span', { class: 'lab', text: s.label }));
    if (s.kept) { head.append(el('span', { class: 'meta', text: 'kept from the original' })); out.push(el('div', { class: 'src' }, head)); continue; }
    head.append(el('span', { class: 'path', text: s.name }), el('span', { class: 'meta', text: x.plain ? 'one picture' : s.wide ? 'one wide picture' : s.files.length + (s.files.length === 1 ? ' tile' : ' tiles') }));
    out.push(el('div', { class: 'src' }, head, sourceStrip(s, x.plain)));
  }
  for (const n of rec.ready ? rec.notes || [] : []) out.push(el('div', { class: 'note', text: n }));
  if (x.warning) out.push(el('div', { class: 'warn', text: x.warning }));
  return out;
}
$('#rpSkin').addEventListener('change', skinChanged);
// Which skin the pictures were painted on, from their names (asked once per set of pictures).
async function pictureGuess() {
  const key = repack.paintDir + '|' + repack.files.join('|');
  if (repack.guess && repack.guess.key === key) return repack.guess;
  let g = { found: false };
  try { g = await api('/api/guessskin', { paintDir: repack.paintDir, files: repack.files }); } catch (e) {}
  g.key = key; repack.guess = g;
  return g;
}
function skinNote(kind, ...content) {
  const n = $('#rpSkinAuto');
  if (!kind) { n.hidden = true; n.replaceChildren(); return; }
  n.className = 'skinauto' + (kind === 'warn' ? ' warn' : '');
  n.replaceChildren(...content); n.hidden = false;
}
// Step 01 follows the pictures: when nothing is chosen yet it is set from their names, and when
// the names point at another skin than the one chosen, that is said, with a way to switch.
async function settleSkinFromPictures() {
  const g = await pictureGuess();
  const cur = selectedSkin();
  const label = sk => sk.aircraft + ' / ' + sk.folder;
  if (!g.found) { if (!cur || cur.dir !== repack.autoSkin) skinNote(''); return; }
  if (!cur) {
    if (!chooseSkin(g.dir)) return;
    repack.autoSkin = g.dir; repack.touched = false;
    templateHint(); if (repack.mode === 'slot') renderIconPreview();
    skinNote('ok', el('span', {}, 'Chosen from your pictures’ names: ', el('b', { text: g.label }), '. Pick another skin here if you painted on a different one.'));
    return;
  }
  if (label(cur) === g.label) { if (cur.dir !== repack.autoSkin) skinNote(''); return; }
  const other = (cur.aircraft || '').toLowerCase() !== g.label.split(' / ')[0].toLowerCase();
  skinNote('warn', el('span', {}, 'Your pictures’ names say ', el('b', { text: g.label }), ', but ', el('b', { text: label(cur) }), ' is chosen here.' +
    (other ? ' Pictures painted for another aircraft do not fit this one.' : repack.mode === 'slot' ? ' That is fine if you want the new skin to start from ' + label(cur) + '.' : '')),
    el('button', { class: 'btn small', type: 'button', onclick: () => { if (chooseSkin(g.dir)) skinChanged(); } }, 'Use ' + g.label));
}
async function replan() {
  const box = $('#rpPlan');
  const haveFiles = repack.paintDir || repack.files.length;
  $('#rpSource').hidden = !haveFiles;
  $('#rpSource').textContent = repack.paintDir ? 'Using pictures in ' + repack.paintDir : repack.files.length ? 'Using ' + repack.files.length + ' dropped pictures' : '';
  if (haveFiles) await settleSkinFromPictures(); else if (!repack.autoSkin) skinNote('');
  const skinDir = $('#rpSkin').value;
  if (!skinDir || !haveFiles) { repack.rows = []; box.replaceChildren(); texOptsShown(); updateButtons(); return; }
  try {
    const r = await api('/api/plan', { skinDir, paintDir: repack.paintDir, files: repack.files, choices: repack.choices });
    repack.rows = r.rows;
    if (!repack.touched) repack.picked = new Set(r.rows.filter(x => x.default).map(x => x.name));
    else repack.picked = new Set(r.rows.filter(x => x.recipe.ready && repack.picked.has(x.name)).map(x => x.name));
    if (!r.pictures) { notice(box, 'error', 'No PNG or TGA pictures were found there.'); updateButtons(); return; }
    const anyReady = r.rows.some(x => x.recipe.ready);
    const table = el('table', { class: 'plan' }, el('thead', {}, el('tr', {}, el('th', { text: '' }), el('th', { text: 'Game texture' }), el('th', { text: 'Rebuilt from' }))),
      el('tbody', {}, r.rows.map(x => el('tr', {},
        el('td', { class: 'tick' }, el('input', { type: 'checkbox', disabled: !x.recipe.ready, checked: repack.picked.has(x.name), 'aria-label': 'Rebuild ' + x.name, onchange: ev => { repack.touched = true; if (ev.target.checked) repack.picked.add(x.name); else repack.picked.delete(x.name); texOptsShown(); updateButtons(); } })),
        el('td', {}, el('div', { class: 'tname', text: x.name }), el('div', { class: 'role', text: (KIND_LABEL[x.kind] || x.kind) + (x.plain ? ', one picture of ' + x.width + ' × ' + x.height : x.blocksW ? ', ' + (x.blocksW * x.blocksH) + ' tiles' + (x.blocksH === 1 ? ' (1001 to ' + (1000 + x.blocksW) + ')' : '') : '') })),
        el('td', {}, planCell(x))))));
    box.replaceChildren(el('div', { class: 'tablewrap' }, table));
    texOptsShown();
    const choosing = r.rows.some(x => (x.recipe.choices || []).some(c => !c.picked));
    const tiled = r.rows.some(x => !x.plain && x.blocksW * x.blocksH > 1);
    if (!anyReady && !choosing) box.append(el('div', { class: 'notice error', style: 'margin-top:10px', text: 'None of these pictures could be matched to a texture of this skin. Check that the file names say what the picture is, for example Base_color or _C.' +
      (tiled ? ' Tiles also need their tile number (1001 to ' + (1000 + Math.max(...r.rows.map(x => x.blocksW || 0), 1)) + '); a picture without a number is taken as one wide picture of the whole skin.' : '') +
      (r.rows.some(x => x.plain) ? ' This aircraft has a picture per part (such as Body and Wing): each file name must also say the part.' : '') }));
  } catch (e) { repack.rows = []; notice(box, 'error', e.message); }
  texOptsShown();
  updateButtons();
}
// The boxes for how the colour texture is stored show while a colour texture is ticked; they keep
// the last choice (in settings.json).
function texOptsShown() {
  const on = (repack.rows || []).some(x => x.kind === 'colour' && repack.picked.has(x.name));
  $('#rpTexOpts').hidden = !on;
  const t = (STATE && STATE.settings.tex) || {};
  if (!texOptsShown.touched) { $('#rpBC7').checked = !!t.bc7; $('#rpNoMips').checked = !!t.noMips; }
}
const texOptions = () => ({ bc7: $('#rpBC7').checked, noMips: $('#rpNoMips').checked });
['#rpBC7', '#rpNoMips'].forEach(s => $(s).addEventListener('change', () => {
  texOptsShown.touched = true;
  api('/api/settings', { tex: texOptions() }).then(st => { STATE = st; }).catch(() => {});
}));
wireDrop($('#rpDrop'), async files => {
  const pics = files.filter(f => isPicture(f.name));
  if (!pics.length) { notice($('#rpPlan'), 'error', 'Drop PNG or TGA pictures, or the folder that holds them.'); return; }
  if (repack.batches.length && !activeJob) api('/api/inbox/clean', { batches: repack.batches }).catch(() => {});
  const batch = newBatch(); repack.batches = [batch];
  repack.paintDir = ''; repack.files = []; repack.touched = false; repack.choices = {};
  const up = $('#rpUp'), bar = $('.upbar i', up); up.hidden = false;
  try {
    for (let i = 0; i < pics.length; i++) {
      $('#rpUpText').textContent = 'Copying ' + pics[i].name + ' (' + (i + 1) + ' of ' + pics.length + ')';
      const path = await upload(pics[i], batch, p => { bar.style.width = Math.round((i + p) / pics.length * 100) + '%'; });
      repack.files.push(path);
    }
  } catch (e) { notice($('#rpPlan'), 'error', e.message); }
  up.hidden = true; bar.style.width = '0';
  replan();
});
$('#rpPick').addEventListener('click', async () => {
  try {
    const r = await api('/api/pick', { kind: 'folder', title: 'Choose the folder with your exported pictures', start: repack.paintDir });
    if (r.paths && r.paths.length) { repack.paintDir = r.paths[0]; repack.files = []; repack.touched = false; repack.choices = {}; replan(); }
  } catch (e) { notice($('#rpPlan'), 'error', e.message); }
});
$('#rpName').addEventListener('input', updateButtons);
function syncExtras() {
  $('#rxBox').hidden = !$('#rxOn').checked;
  const mode = radio('rxIcon');
  $('#rxColourBox').hidden = mode !== 'recolour'; $('#rxPicBox').hidden = mode !== 'picture';
  $('#rxColour').disabled = $('#rxColourAuto').checked;
  $('#rxPicName').textContent = repack.rxPicture || '';
  updateButtons();
}
['#rxOn', '#rxColourAuto'].forEach(s => $(s).addEventListener('change', syncExtras));
$$('input[name="rxIcon"]').forEach(r => r.addEventListener('change', syncExtras));
$('#rxPicPick').addEventListener('click', async () => {
  try {
    const r = await api('/api/pick', { kind: 'file', filter: 'pictures', title: 'Choose a picture for the icon' });
    if (r.paths && r.paths.length) { repack.rxPicture = r.paths[0]; syncExtras(); }
  } catch (e) { alertLine(e.message); }
});
$('#rpGo').addEventListener('click', async () => {
  if (repack.mode === 'slot') { buildSlot(); return; }
  const out = $('#rpResult'); out.replaceChildren();
  try {
    const extras = $('#rxOn').checked ? { name: $('#rxName').value, description: $('#rxDesc').value, icon: radio('rxIcon'), iconColour: $('#rxColourAuto').checked ? '' : $('#rxColour').value, iconPicture: repack.rxPicture || '', template: +$('#rxNumber').value || 0 } : null;
    const res = await runJob('/api/build', { skinDir: $('#rpSkin').value, paintDir: repack.paintDir, files: repack.files, modName: $('#rpName').value, textures: Array.from(repack.picked), install: $('#rpInstall').checked, choices: repack.choices, cleanup: [], extras, tex: texOptions() });
    const box = el('div', { class: 'notice good results' }, el('p', {}, el('strong', { text: 'Mod ' + res.mod + ' is built. ' }), openFolderBtn(res.installedDir || res.packedDir, res.installedDir ? 'Open mods folder' : 'Open mod files')));
    box.append(el('p', { text: res.installedDir ? 'It was copied into your mods folder, so it loads the next time the game starts:' : 'Copy these files into their own folder inside your mods folder:' }));
    for (const f of (res.installedFiles || res.files)) box.append(el('span', { class: 'path', text: f }));
    for (const n of res.notes || []) box.append(el('p', { class: 'hint', text: 'Note: ' + n + '.' }));
    out.replaceChildren(box);
    if (res.conflicts && res.conflicts.length) out.append(el('div', { class: 'notice warn', style: 'margin-top:10px', role: 'status' },
      el('strong', { text: 'Another installed mod replaces the same game files. ' }),
      res.mod + ' and ' + res.conflicts.map(c => c.with).join(', ') + ' both change ' + res.conflicts.flatMap(c => c.files).filter((f, i, a) => a.indexOf(f) === i).map(shortFile).join(', ') + '. The game shows only one of them. ',
      el('button', { class: 'btn small', onclick: () => showPage('mods') }, 'Open Mods')));
    if (res.installedDir) loadMods(true);
  } catch (e) { notice(out, 'error', e.message); }
});

// ---------------------------------------------------------------- installed mods
const shortFile = p => String(p).split('/').pop();
const fmtSize = n => n >= 1 << 30 ? (n / (1 << 30)).toFixed(2) + ' GB' : n >= 1 << 20 ? (n / (1 << 20)).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB';
let MODS = null;
const KIND_CHIP = {
  slots: ['new skin slots', 'blue', 'A skin mod with skins in slots of their own. Manage it on the Skin slots screen.'],
  skinlist: ['shared skin list', 'blue', 'Made by ACE8 Skin Studio: the skin list and unlock list for every new skin, rebuilt when skins are added or removed.'],
  tables: ['own skin list: merged', 'orange', 'This mod carries its own copy of the skin list; its skins are copied into the shared skin list.'],
};
// quiet = true only refreshes the count next to "Mods" in the menu
async function loadMods(quiet) {
  let r;
  try { r = await api('/api/mods'); } catch (e) { if (quiet !== true) notice($('#moList'), 'error', e.message); return; }
  MODS = r;
  const clashing = r.mods.filter(m => m.conflicts.length).length;
  const badge = $('#navModsBadge');
  badge.hidden = !clashing; badge.textContent = clashing; badge.title = clashing ? clashing + ' mods replace the same game files as another mod' : '';
  if (quiet === true) return;
  const setup = $('#moSetup');
  setup.hidden = r.ok; $('#moBlock').hidden = !r.ok;
  if (!r.ok) { setup.replaceChildren(el('strong', { text: 'No mods folder yet. ' }), r.note, ' ', el('button', { class: 'btn small', onclick: () => showPage('settings') }, 'Open Settings')); return; }
  $('#moDir').textContent = r.dir;
  $('#moOpenRemoved').hidden = !r.haveRemoved;
  $('#moSummary').textContent = r.mods.length ? r.mods.length + (r.mods.length === 1 ? ' mod' : ' mods') + (clashing ? ', ' + clashing + ' in conflict' : ', no conflicts') : 'No mods yet';
  const box = $('#moList');
  if (!r.mods.length) { box.replaceChildren(el('div', { class: 'notice', text: 'No mods were found in this folder. A mod is a folder holding .utoc and .ucas files; mods built on the Repack screen with "Copy the finished mod into my mods folder" ticked appear here.' })); return; }
  box.replaceChildren(el('div', { class: 'list' }, r.mods.map(m => {
    const when = m.modified ? new Date(m.modified * 1000).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : '';
    const actions = el('div', { class: 'row' });
    const normal = () => actions.replaceChildren(openFolderBtn(m.dir, 'Open'), m.kind === 'slots' || m.kind === 'skinlist'
      ? el('button', { class: 'btn small', onclick: () => showPage('slots') }, 'Skin slots')
      : el('button', { class: 'btn small', onclick: confirmRemove }, 'Remove'));
    const confirmRemove = () => actions.replaceChildren(el('span', { class: 'hint', text: 'Remove ' + m.name + '?' }),
      el('button', { class: 'btn small', onclick: () => removeMod(m) }, 'Yes, remove'), el('button', { class: 'btn small', onclick: normal }, 'Keep it'));
    normal();
    const card = el('div', { class: 'mod' },
      el('div', { class: 'line' },
        el('div', { class: 'id' }, m.name, el('small', { text: m.files.length + (m.files.length === 1 ? ' file, ' : ' files, ') + fmtSize(m.size) + (when ? ', changed ' + when : '') + (m.loose ? ', lying directly in the mods folder' : '') })),
        KIND_CHIP[m.kind] ? el('span', { class: 'chip ' + KIND_CHIP[m.kind][1], title: KIND_CHIP[m.kind][2], text: KIND_CHIP[m.kind][0] }) : null,
        actions));
    if (m.replaces.length && m.kind === 'skinlist') {
      // the shared skin list replaces the game's two lists and adds the new skins' material slots
      const tables = m.replaces.filter(f => /\/(DT_Skin|DA_Unlock_Skin)$/.test(f));
      const added = (m.replaced || m.replaces.length) - tables.length;
      card.append(el('div', { class: 'replaces' }, el('span', { class: 'lab', text: 'Replaces' }), tables.map(f => el('span', { class: 'chip', title: f, text: shortFile(f) }))));
      if (added > 0) card.append(el('div', { class: 'replaces' }, el('span', { class: 'lab', text: 'Adds' }),
        el('span', { class: 'lab', title: m.replaces.filter(f => !tables.includes(f)).slice(0, 60).join('\n'), text: added + ' files: the material slots and icons of the new skins' })));
    } else if (m.replaces.length) {
      const shown = m.replaces.slice(0, 8);
      card.append(el('div', { class: 'replaces' }, el('span', { class: 'lab', text: m.kind === 'slots' ? 'Adds' : 'Replaces' }), shown.map(f => el('span', { class: 'chip', title: f, text: shortFile(f) })),
        (m.replaced || m.replaces.length) > shown.length ? el('span', { class: 'lab', title: m.replaces.slice(8, 60).join('\n'), text: 'and ' + ((m.replaced || m.replaces.length) - shown.length) + ' more' }) : null));
    }
    if (m.note) card.append(el('p', { class: 'hint', text: m.note }));
    for (const c of m.conflicts) {
      const files = c.files.map(shortFile);
      card.append(el('div', { class: 'notice warn' },
        el('strong', { text: 'Conflicts with ' + c.with + '. ' }),
        c.files.length ? 'Both replace ' + files.slice(0, 6).join(', ') + (files.length > 6 ? ' and ' + (files.length - 6) + ' more' : '') + '. The game shows only one of them, so remove the one you do not want.'
                       : 'Both are the same container inside, so the game can load only one of them.'));
    }
    return card;
  })));
}
async function removeMod(m) {
  const out = $('#moResult'); out.replaceChildren();
  try {
    const r = await api('/api/mods/remove', { name: m.name, loose: m.loose });
    await loadMods();
    notice(out, 'good', el('strong', { text: m.name + ' was taken out of the mods folder. ' }), 'Nothing was deleted; it is now in ', el('span', { class: 'path', text: r.movedTo }), ' ', openFolderBtn(r.movedTo));
  } catch (e) { notice(out, 'error', e.message); }
}
$('#moRefresh').addEventListener('click', () => { $('#moResult').replaceChildren(); loadMods(); });
$('#moOpen').addEventListener('click', () => { if (MODS) api('/api/open', { path: MODS.dir }).catch(e => notice($('#moResult'), 'error', e.message)); });
$('#moOpenRemoved').addEventListener('click', () => { if (MODS) api('/api/open', { path: MODS.removedDir }).catch(e => notice($('#moResult'), 'error', e.message)); });

// Why the build button cannot be pressed yet, for Replace a game skin (New skin slot has slotNotReady).
function replaceNotReady() {
  if (!STATE) return '';
  if (!STATE.checks.retoc.ok) return 'Finish the setup in Settings first.';
  if (!$('#rpSkin').value) return 'Choose the skin to replace.';
  if (!repack.paintDir && !repack.files.length) return 'Choose your painted pictures.';
  if (!repack.picked.size) return 'Tick the textures to rebuild.';
  if (!$('#rpName').value.trim()) return 'Give the mod a name.';
  if ($('#rxOn').checked && radio('rxIcon') === 'picture' && !repack.rxPicture) return 'Choose the picture for the icon.';
  return '';
}
// The reason beside the build button takes you to the step it is about.
const HINT_TARGETS = [
  [/Settings/, () => showPage('settings')],
  [/skin to (start from|replace)/, '#rpSkin'],
  [/number of the game skin/, '#rpTemplate'],
  [/painted pictures/, '#rpPick'],
  [/Tick the textures|colour the icon/, '#rpPlan'],
  [/Give the mod a name/, '#rpName'],
  [/Give the skin a name/, '#slName'],
  [/skin mod to add/, '#slMod'],
  [/picture for the icon/, () => repack.mode === 'slot' ? '#slPicPick' : '#rxPicPick'],
];
function goToStep(target) {
  if (typeof target === 'function') { target = target(); if (!target) return; }
  const n = $(target);
  if (!n) return;
  const block = n.closest('.block') || n;
  block.scrollIntoView({ behavior: 'smooth', block: 'center' });
  block.classList.remove('flash'); void block.offsetWidth; block.classList.add('flash');
  setTimeout(() => { try { n.focus({ preventScroll: true }); } catch (e) {} }, 400);
}
function setGoHint(text) {
  const h = $('#rpGoHint');
  if (!text) { h.replaceChildren(); return; }
  const t = HINT_TARGETS.find(([re]) => re.test(text));
  h.replaceChildren(t ? el('button', { type: 'button', class: 'linkbtn', title: 'Show me', onclick: () => goToStep(t[1]) }, text) : text);
}
function updateButtons() {
  const busy = !!activeJob;
  const ready = splitReady().length;
  $('#spGo').disabled = busy || !ready;
  $('#spGo').textContent = ready > 1 ? 'Split ' + ready + ' pictures' : 'Split';
  const exOK = STATE && ['workspace', 'retoc', 'paks', 'aes'].every(k => STATE.checks[k].ok);
  $('#exGo').disabled = busy || !exOK || $('#exFilter').value.trim().length < 3;
  $('#exFind').disabled = busy || !exOK;
  $('#exPreview').disabled = busy || !exOK;
  if (repack.mode === 'slot') {
    const why = slotNotReady();
    $('#rpGo').disabled = busy || !!why;
    const updating = repack.rebuild && radio('slModMode') === 'add' && $('#slMod').value === repack.rebuild.mod;
    $('#rpGo').textContent = updating ? 'Update this skin' : 'Build new skin';
    setGoHint(busy ? '' : why);
  } else {
    const why = replaceNotReady();
    $('#rpGo').disabled = busy || !!why;
    $('#rpGo').textContent = repack.picked.size > 1 ? 'Build mod with ' + repack.picked.size + ' textures' : 'Build mod';
    setGoHint(busy ? '' : why);
  }
  $('#mdGo').disabled = busy || !model.keep.size || !$('#mdName').value.trim();
  pilotsButtons();
  if (pn.view) namesButtons();
  emblemButtons();
  if (SAVE) saveButtons();
}

// ---------------------------------------------------------------- settings
function fillSettings() {
  if (!STATE) return;
  const s = STATE.settings;
  $('#sePaks').value = s.paksDir; $('#seAes').value = s.aesKey; $('#seUsmap').value = s.usmap; $('#seRetoc').value = s.retocPath; $('#seUe4ss').value = s.ue4ssMods || ''; $('#seSaveDir').value = s.saveDir || '';
  $('#seMods').value = s.modsDir; $('#seWork').value = s.workspace; $('#seFormat').value = s.imageFormat; $('#seBrowser').checked = s.defaultBrowser;
  showChecks();
  const raws = s.extraRawDirs || [];
  $('#seRawBox').hidden = !raws.length;
  $('#seRaw').replaceChildren(...raws.map(p => el('div', { class: 'skin' }, el('span', { class: 'path grow', text: p }), el('button', { class: 'btn small', onclick: async () => { await api('/api/settings', { extraRawDirs: raws.filter(x => x !== p) }); await refreshState(); fillSettings(); } }, 'Remove'))));
}
function showChecks() {
  $$('[data-check]').forEach(n => { const c = STATE.checks[n.dataset.check]; n.textContent = c.note; n.classList.toggle('bad', !c.ok); });
  $('#seWorkDefault').hidden = STATE.checks.workspace.ok || STATE.settings.workspace === STATE.folders.defaultWorkspace;
}
$('#seWorkDefault').addEventListener('click', ev => { ev.preventDefault(); $('#seWorkDefault').hidden = true; $('#seWork').value = STATE.folders.defaultWorkspace; saveSettings(); });
async function saveSettings() {
  const body = { paksDir: $('#sePaks').value, aesKey: $('#seAes').value, usmap: $('#seUsmap').value, retocPath: $('#seRetoc').value, modsDir: $('#seMods').value, ue4ssMods: $('#seUe4ss').value, saveDir: $('#seSaveDir').value, workspace: $('#seWork').value, imageFormat: $('#seFormat').value, defaultBrowser: $('#seBrowser').checked };
  try {
    await api('/api/settings', body); await refreshState(); fillSettings();
    $('#exFormat').value = STATE.settings.imageFormat;
    $('#seSaved').textContent = 'Saved.'; setTimeout(() => { $('#seSaved').textContent = ''; }, 2500);
    updateButtons();
  } catch (e) { $('#seSaved').textContent = e.message; }
}
$('#seSave').addEventListener('click', saveSettings);
$$('#page-settings input[type=text], #page-settings input[type=password]').forEach(i => i.addEventListener('change', saveSettings));
$('#seAesShow').addEventListener('click', () => { const show = $('#seAes').classList.toggle('masked') === false; $('#seAesShow').textContent = show ? 'Hide' : 'Show'; });
$('#seReport').addEventListener('click', () => copyReport($('#seReportOut')));
$('#seImportOld').addEventListener('click', async () => {
  const note = $('#seImportNote');
  try {
    const r = await api('/api/pick', { kind: 'folder', title: 'Choose the folder of AC8 Skin Studio 1.x (where AC8SkinStudio.exe is)' });
    if (!r.paths || !r.paths.length) return;
    await api('/api/settings/import', { path: r.paths[0] });
    await refreshState(); fillSettings(); loadLibrary(true);
    note.textContent = 'Copied. Check the settings below.';
  } catch (e) { note.textContent = e.message; }
});
$('#seOpenWork').addEventListener('click', () => api('/api/open', { path: STATE.settings.workspace }).catch(() => { $('#seSaved').textContent = 'The workspace folder is created the first time something is saved in it.'; }));
$('#seQuit').addEventListener('click', async () => { try { await api('/api/quit', {}); } catch (e) {} document.body.replaceChildren(el('p', { style: 'padding:40px;font-size:17px', text: 'The program has stopped. You can close this window.' })); });

// ---------------------------------------------------------------- tutorial
// One short page per screen. Every path and file name here is a made-up example.
// "page" names the screen the "Open the ... screen" button goes to; "eg" is an example shown under a step.
const TUTORIAL = [
  { topic: 'Start here', title: 'How a skin gets into the game',
    why: `This program does the file handling around your painting program. It gets a skin's pictures out of the game, and puts your painted ones back in: either in place of one of the game's skins, or as a new skin of its own.`,
    flow: [['Extract', 0], ['Model', 0], ['Paint in your own program', 1], ['Repack or New skin slot', 0], ['Skin slots', 0], ['Play', 1]],
    steps: [
      { t: `Fill in Settings once: the game's Paks folder, the AES key and your mods folder. Coming from AC8 Skin Studio 1.x? One button there copies its settings.` },
      { t: `Extract an aircraft's skin. You get one picture per UDIM tile, ready to paint on.` },
      { t: `Prepare the aircraft's model, so your painting program shows only the skin.` },
      { t: `Paint in Substance Painter or Blender, then export your pictures.` },
      { t: `Repack turns those pictures into a mod that replaces a game skin. New skin slot adds them as an extra skin with its own name, description and icon. New emblem adds a picture of yours to the emblem list.` },
      { t: `Skin slots shows every new skin and emblem, imports skin mods other people share, and makes a .zip of yours to share. Save tools unlocks them straight away and gets your save ready for the game without mods.` } ],
    tip: `Split is a side tool. It cuts a wide skin picture you already have into tiles, and is only needed for pictures that did not come from Extract.` },
  { topic: 'Extract', page: 'extract', title: 'Extract: get a skin out of the game',
    why: `Pulls an aircraft's skin textures out of the game files and saves them as tiles you can paint on. The game files are never changed.`,
    steps: [
      { t: `Press "Find aircraft in the game files" once. It reads the game's file list and shows every aircraft.` },
      { t: `Type part of a name to narrow the list, then pick the aircraft.`, eg: `Typing f15 leaves PP0005_f15c in the list.` },
      { t: `Press "Preview skins" to see what each skin folder looks like. It fetches only the colour texture of each skin, makes a small picture and deletes the full-size file again.` },
      { t: `Click the skins you want. Leave them all unpicked to take every skin.`, eg: `001   002   003   101` },
      { t: `Press Extract. With "Make paint-ready tiles" ticked you get base colour, metallic, roughness, normal map and ambient occlusion, one picture per tile.`, eg: `workspace\\paint-ready\\PP0005_f15c\\001\\T_PP0005_f15c_001_C_BaseColor.1001.png` },
      { t: `Look under "Extracted skins". Each skin shows a preview, and "Open" takes you to its files.` } ],
    tip: `Folder numbers are not the order of skins in the game's menu. Use the previews to find the one you want.` },
  { topic: 'Split', page: 'split', title: 'Split: cut a wide picture into tiles',
    why: `A skin picture exported with FModel is one wide strip. Painting programs want one picture per tile, numbered 1001, 1002 and so on.`,
    steps: [
      { t: `Drop PNG or TGA pictures onto the page, or press "Choose pictures".`, eg: `T_PP0005_f15c_001_C.png` },
      { t: `Check the preview. It shows where each cut falls and the number each tile gets. The count comes from the picture itself: five tiles, seven, or however many fit across it.` },
      { t: `Tick "Separate the packed channels" if a picture's name ends in _MRX or _NO and you want metallic, roughness, normal map and ambient occlusion as separate files.` },
      { t: `Press Split. Each skin's tiles go into a folder of their own, so a skin's colour and finish tiles end up together and two skins never get mixed.`, eg: `workspace\\split\\T_PP0005_f15c_001\\T_PP0005_f15c_001_C.1001.png` },
      { t: `Press "Use in Repack" next to a folder to carry on with those tiles, or "Open folder" to see them.` } ] },
  { topic: 'Model', page: 'model', title: 'Model: prepare the aircraft for painting',
    why: `A model from the game also carries the cockpit, glass and weapons, each with its own textures. This keeps only the parts that wear the skin.`,
    steps: [
      { t: `In FModel, export the aircraft's model as glTF (.glb). To paint the doors, refuelling probe and pylons too, also export the models whose names end in GearOff, RefuelOff, DPTK or DPP. Pylon names often carry a number, such as DPP1, DPP2 and DPP3; add as many as you like.`, eg: `SM_PP0005_f15c.glb   SM_PP0005_f15c_GearOff.glb   SM_PP0005_f15c_DPP1.glb` },
      { t: `Drop the .glb files onto the page. Parts that wear the skin are ticked for you.` },
      { t: `Press "Save paint model".`, eg: `workspace\\models\\SM_PP0005_f15c_paint.glb` },
      { t: `In Substance Painter, create a new project from the saved file with "Use UV Tile workflow" switched on, then import the tiles from Extract.` } ] },
  { topic: 'Repack', page: 'repack', title: 'Repack: turn your paint into a mod',
    why: `Converts the pictures you exported after painting back into the game's format, puts them in the right folders and packs them into a mod.`,
    steps: [
      { t: `Choose the skin to replace. It must be extracted first, because the program rebuilds the game's original files.` },
      { t: `Drop your exported pictures, or the folder they are in. Use one picture per tile, with the tile number in each name, or one wide picture of the whole skin with no number; a wide picture does not need splitting first.`, eg: `MySkin_Base_color_1001.png  to  MySkin_Base_color_1005.png   or just   MySkin_Base_color.png` },
      { t: `The names must say what each picture is: Substance Painter's channel names work, and so do the game's own endings (_C, _MRX, _NO), whichever skin they were named after.`, eg: `MySkin_Roughness_1001.png   T_PP0005_f15c_003_C.1001.png   T_PP0005_f15c_003_C.png` },
      { t: `The table shows what each game texture will be built from: the file names and a small picture of every tile. Check it is the skin you mean. If the folder holds more than one set of pictures, choose which to use.` },
      { t: `Tick the textures to rebuild. Colour is ticked for you. Below the list you can store the colour texture as BC7 for sharper markings, or with no mipmaps.` },
      { t: `Name the mod and press "Build mod". With a mods folder set in Settings it is copied there, and loads the next time the game starts.`, eg: `Mods\\MySkin\\MySkin_P.utoc   MySkin_P.ucas   MySkin_P.pak` } ],
    tip: `Two mods that replace the same skin conflict, and the game shows only one. Repack warns you when that happens, and the Mods screen can remove one. Use modded files offline and in single player only.` },
  { topic: 'New skin slot', page: 'repack', mode: 'slot', title: 'New skin slot: a skin of your own',
    why: `Instead of painting over one of the game's skins, add your paint as an extra skin. It gets its own place in the skin list, its own name, description and icon, and every skin of the game stays as it was.`,
    steps: [
      { t: `In the menu, choose New skin slot. Choose the skin to start from: the one you painted on. The new skin copies its material, finish and normal map, and where emblems may go.`, eg: `PP0021_su57 / 001` },
      { t: `Drop your painted pictures, as for Repack, and tick the textures to rebuild. Without pictures, the new skin keeps the original's look; only its name, description and icon are new.` },
      { t: `Give it a name and a description. These are what the game shows in the hangar.`, eg: `Navy Feather   /   Dark navy with white star roundels.` },
      { t: `It unlocks the way the game unlocks its own first skins: when you finish a campaign mission. After that it stays in the hangar.` },
      { t: `Choose the icon: keep the original, recolour the original in your paint's main colour or a colour you pick, or use a picture of your own (its middle square is used).` },
      { t: `Choose a new skin mod or add it to one you made before, then press Build new skin. It is installed straight away and gets the first free skin number from 601 up.`, eg: `Mods\\NavyFeather\\NavyFeather_xkqdm_P.utoc` } ],
    tip: `New skins need AC8 Mod Loader 1.1.0 or newer, which loads files that are new to the game, and SkinSlotText, the small UE4SS mod that shows their names. The program installs SkinSlotText for you.` },
  { topic: 'New emblem', page: 'emblem', title: 'New emblem: an emblem of your own',
    why: `Add your own picture to the game's emblem list, with its own name and description. It goes on any aircraft like the game's emblems, and every emblem of the game stays as it was.`,
    steps: [
      { t: `Drop the picture on the page. A PNG with a see-through background works best: the see-through parts show the aircraft's paint. The whole picture is used, fitted into a square.`, eg: `NightOwl.png   (1024 x 1024, see-through around the owl)` },
      { t: `Check the preview. The checks show where it is see-through.` },
      { t: `Give it a name and a description. These are what the game shows in the emblem list.`, eg: `Night Owl   /   A red moon over the sea.` },
      { t: `Choose a new skin mod or add it to one you made before. A skin mod can hold new skins, emblems or both.` },
      { t: `Press Build new emblem. It is installed straight away and gets the first free emblem number from 601 up.` },
      { t: `It unlocks when you finish a campaign mission, like the game's emblems. To have it straight away, close the game and press Unlock now on the Save tools screen.` } ],
    tip: `"Change picture" and "Edit name" on the Skin slots screen change an emblem later; it keeps its number, so aircraft wearing it keep it.` },
  { topic: 'Pilots', page: 'pilots', title: 'Pilots: helmets, portraits and names',
    why: `Paint a pilot's helmet, change the portrait shown next to their radio messages (a photo of your own, another portrait's photo, new text), and give the people on the radio new names and callsigns.`,
    steps: [
      { t: `Press "Find pilots in the game files" once. It finds every pilot with a helmet and every radio portrait, and makes a small picture of each helmet.` },
      { t: `Pick a pilot. The helmet is shown in parts, each with pictures of its own: Tasha's has two, the shell and the mask.`, eg: `Part 1  T_PI003_Helmet01_01      Part 2  T_PI003_Helmet01_02` },
      { t: `Press "Get paint-ready pictures" for full-size base colour, metallic, roughness, ambient occlusion and normal map pictures of every part, and paint over them.`, eg: `workspace\\paint-ready\\Pilots\\PI003_Tasha\\T_PI003_Helmet01_01_BaseColor.png` },
      { t: `Drop your painted pictures on the part they are for, or all of them at once when their names say the part. Whatever you leave out stays as the game's.` },
      { t: `Click a radio portrait to change it. Use a picture of your own (drag it into place, scroll to zoom) or another portrait's photo, and keep the text or write new text in the game's own font. New portraits are kept when you close the program, so you can come back and add more.`, eg: `R. IVANOVA   /   ROSA   /   ENS   FCU NAVY` },
      { t: `Under "Names on the radio", type a new name for anyone who talks on the radio, and a new callsign for the wingmen. Press "Save names": they need no building and show the next time the game starts.`, eg: `Tasha   name Rosa   callsign Rosebud` },
      { t: `Baxter, Tasha, Coster and Kate also have a "Hangar name" box for the hangar's pilot list. Leave it empty and the hangar gets the new name in capitals. After playing, each saved name says whether the game showed it in your last game.`, eg: `Name Rosa   Hangar name (empty)   → the hangar shows ROSA` },
      { t: `Name the mod and press "Build mod". The helmet and the portraits go into one mod.` } ],
    tip: `Helmets and portraits replace the game's own files, so two mods that change the same helmet or portrait conflict; the Mods screen shows it. Use modded files offline and in single player only.` },
  { topic: 'Skin slots', page: 'slots', title: 'Skin slots: share, import and keep them together',
    why: `The game has one list of skins. This screen keeps one shared copy of it for every new skin you have, yours and other people's, so any number of skin mods work together.`,
    steps: [
      { t: `Each skin mod shows its skins with their icon, name and skin number. "Edit name" changes the name and description without building anything.` },
      { t: `"Share as .zip" makes a file you can upload, for example to Nexus Mods. It carries the skins, their names and icons, and a README.`, eg: `workspace\\shared\\NavyFeather.zip` },
      { t: `To install someone's skin mod, drop its .zip on "Import a skin mod". You see its skins and the numbers they will get before anything is installed.` },
      { t: `When a skin wants a number another skin already has, it gets the next free one. Its name, description and icon stay with it, and skins that were there first keep their numbers.` },
      { t: `Mods made with other tools that bring their own skin list are found too: their skins are copied into the shared list instead of hiding everyone else's.` } ],
    tip: `Before taking out a skin mod, choose another skin for the aircraft in the game, so the game does not look for a skin that is gone.` },
  { topic: 'Mods', page: 'mods', title: 'Mods: see what is installed',
    why: `Lists the mods in your mods folder and the game files each one replaces. "My skin does not show up" is very often two mods replacing the same file.`,
    steps: [
      { t: `Each mod shows its size, its date and the game files it replaces.`, eg: `MySkin   3 files, 60.3 MB   Replaces T_PP0005_f15c_001_C` },
      { t: `A yellow box means another mod replaces the same files. The game shows only one of them, so remove the one you do not want.` },
      { t: `"Remove" takes a mod out of the mods folder without deleting it. It is moved into the workspace, in the removed-mods folder. Close the game first.`, eg: `workspace\\removed-mods\\MySkin_20260101-120000` },
      { t: `To bring a removed mod back, move its folder into your mods folder again.` } ],
    tip: `The number next to "Mods" in the menu counts the mods that are in conflict. Mods made by other tools are listed too.` },
  { topic: 'Save tools', page: 'save', title: 'Save tools: unlock, go back to vanilla, restore',
    why: `Changes to your campaign save that the game itself does not offer: unlock new skins and emblems straight away, and take them out again before playing without mods. The save is always backed up first.`,
    steps: [
      { t: `Close the game first. It writes its save when it closes, which would undo the change; the buttons wait until it is closed.` },
      { t: `Unlock now puts every new skin and emblem in the game's lists into your save as unlocked, marked new. No mission needed.` },
      { t: `Before playing the game without mods (or online), press "Prepare my save for the game without mods". The new skins and emblems come out of the unlocked lists, and any aircraft wearing one goes back to the game's own skin or no emblem. Nothing else in the save changes.` },
      { t: `Every change first copies both save files into a dated backup. "Put this back" next to a backup restores it; the save as it is then is backed up too, so a restore can be undone.`, eg: `…\\ACE COMBAT 8\\Saved\\SkinStudioSaveBackups\\2026-10-15 18-30-00` },
      { t: `Turn on the backup at game start and your save is also copied each time the game starts, keeping the last 20. It is a small UE4SS mod that only reads the save.` } ],
    tip: `The save's checksum is worked out again after every change, and the new save is read back before it replaces the old one. If Steam reports a cloud conflict the next time the game starts, keep the files on this computer.` },
  { topic: 'Settings', page: 'settings', title: 'Settings: set up once',
    why: `Everything the program needs to reach the game. It is saved in settings.json next to the program.`,
    steps: [
      { t: `Game Paks folder: inside the game's own folder.`, eg: `C:\\Program Files (x86)\\Steam\\steamapps\\common\\ACE COMBAT 8\\Game\\Content\\Paks` },
      { t: `AES key: the game's key, 64 characters, optionally starting with 0x. The program does not include it, and Extract cannot read the game files without it.`, eg: `0x0123456789ABCDEF…  (64 characters in all)` },
      { t: `retoc program: the free tool that unpacks and packs the game's files. It comes with this program; leave the box empty.` },
      { t: `Mods folder: the folder your mod loader reads, with one sub-folder per mod. New skins go there, and finished Repack mods can be copied there for you.`, eg: `…\\ACE COMBAT 8\\Game\\Binaries\\Win64\\ue4ss\\Mods\\IoStoreLoaderMod` },
      { t: `UE4SS Mods folder: found by itself when the mods folder is inside it. It is where SkinSlotText goes, which shows the names of new skins.` } ],
    tip: `Each setting shows a green dot when it is right. If you need help, "Copy a support report" under About copies your version, settings and the last task's log, ready to paste into a message. The AES key is never included.` },
];
let tutAt = 0;
function renderTutorial() {
  const t = TUTORIAL[tutAt];
  $('#tutTopics').replaceChildren(...TUTORIAL.map((x, i) => el('button', { type: 'button', 'aria-current': i === tutAt ? 'step' : null, onclick: () => { tutAt = i; renderTutorial(); $('#tutBody').focus(); } }, x.topic)));
  const body = $('#tutBody');
  body.replaceChildren(...[
    el('h2', { id: 'tutTitle', text: t.title }),
    el('p', { class: 'why', text: t.why }),
    t.flow ? el('div', { class: 'flow', 'aria-hidden': 'true' }, t.flow.flatMap((f, i) => [i ? el('i', { text: 'then' }) : null, el('span', { class: f[1] ? 'out' : '', text: f[0] })])) : null,
    el('ol', {}, t.steps.map(s => el('li', {}, el('span', {}, s.t, s.eg ? el('span', { class: 'eg', text: 'Example: ' + s.eg }) : null)))),
    t.tip ? el('p', { class: 'tip', text: t.tip }) : null,
    t.page ? el('div', {}, el('button', { class: 'btn small', type: 'button', onclick: () => { $('#tut').close(); showPage(t.page, t.mode); } }, 'Open the ' + t.topic + ' screen')) : null].filter(Boolean));
  body.scrollTop = 0;
  $('#tutCount').textContent = (tutAt + 1) + ' of ' + TUTORIAL.length;
  $('#tutBack').disabled = tutAt === 0;
  $('#tutNext').textContent = tutAt === TUTORIAL.length - 1 ? 'Done' : 'Next';
}
function openTutorial(at) {
  tutAt = at || 0;
  $('#tutHide').checked = !!(STATE && STATE.settings.hideTutorial);
  renderTutorial();
  const d = $('#tut');
  if (!d.open) d.showModal();
  $('#tutNext').focus();
}
$('#tutBack').addEventListener('click', () => { if (tutAt > 0) { tutAt--; renderTutorial(); } });
$('#tutNext').addEventListener('click', () => { if (tutAt < TUTORIAL.length - 1) { tutAt++; renderTutorial(); } else $('#tut').close(); });
$('#tutHide').addEventListener('change', async ev => {
  const want = ev.target.checked;
  try { await api('/api/settings', { hideTutorial: want }); if (STATE) STATE.settings.hideTutorial = want; }
  catch (e) { ev.target.checked = !want; }
});
$('#tut').addEventListener('click', ev => { if (ev.target === $('#tut')) $('#tut').close(); }); // a click on the dimmed area closes it
$('#navTutorial').addEventListener('click', () => openTutorial(0));

// ---------------------------------------------------------------- useful links
async function showLinks() {
  const box = $('#lkList');
  try {
    const r = await api('/api/links');
    box.replaceChildren(...r.links.map(l => el('div', { class: 'lk-card' },
      el('h2', { text: l.title }),
      el('div', { class: 'by', text: 'By ' + l.author + ' \u00b7 ' + l.site }),
      el('p', { style: 'margin:0', text: l.about }),
      l.points && l.points.length ? el('ul', {}, ...l.points.map(p => el('li', { text: p }))) : null,
      el('div', { class: 'url', text: l.url }),
      el('div', { class: 'row' },
        el('button', { class: 'btn primary', type: 'button', onclick: async () => {
          try { await api('/api/link/open', { url: l.url }); $('#lkResult').replaceChildren(); } catch (e) { notice($('#lkResult'), 'error', e.message); }
        } }, 'Open in my browser'),
        el('button', { class: 'btn', type: 'button', onclick: async ev => {
          const b = ev.currentTarget;
          try { await navigator.clipboard.writeText(l.url); b.textContent = 'Copied'; } catch (e) { b.textContent = 'Select the link above to copy it'; }
          setTimeout(() => { b.textContent = 'Copy link'; }, 2000);
        } }, 'Copy link')))));
  } catch (e) { notice(box, 'error', e.message); }
}

// ---------------------------------------------------------------- start
setInterval(() => { api('/api/ping').catch(() => {}); }, 15000);
// Tell the program this window is closing; it stops a few seconds after its last window has gone.
window.addEventListener('pagehide', () => {
  const url = '/api/bye?t=' + encodeURIComponent(TOKEN) + '&c=' + encodeURIComponent(CLIENT);
  try { if (navigator.sendBeacon && navigator.sendBeacon(url)) return; } catch (e) {}
  try { fetch(url, { method: 'POST', keepalive: true }); } catch (e) {}
});
window.addEventListener('pageshow', ev => { if (ev.persisted) api('/api/ping').catch(() => {}); }); // came back from the browser's page cache
(async () => {
  try {
    await refreshState();
    applyTheme(STATE.settings.theme);
    $('#exFormat').value = STATE.settings.imageFormat;
    let page = 'extract', mode = 'replace';
    try { page = sessionStorage.getItem('ace8-page') || page; mode = sessionStorage.getItem('ace8-rpmode') || mode; } catch (e) {}
    setRepackMode(mode);
    showPage(page, page === 'repack' ? mode : undefined);
    updateButtons();
    loadMods(true);
    loadSlots(true);
    if (STATE.busy) watchJob(STATE.busy).catch(() => {});
    // Show the tutorial when the program starts, until "Don't show this" is ticked (once per window, not on every reload).
    let shown = false;
    try { shown = sessionStorage.getItem('ace8-tut') === '1'; sessionStorage.setItem('ace8-tut', '1'); } catch (e) {}
    if (!STATE.settings.hideTutorial && !shown) openTutorial(0);
  } catch (e) {
    document.body.replaceChildren(el('p', { style: 'padding:40px;font-size:17px;max-width:60ch', text: e.message }));
  }
})();
</script>
