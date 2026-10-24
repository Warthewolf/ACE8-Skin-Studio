// ---------------------------------------------------------------- pilots: helmets and radio portraits
const pilots = { idx: null, pick: '', files: [], batches: [], plan: null, edits: {}, inMods: [], nameTouched: false, font: null };
const pimg = (kind, name) => '/api/pilots/img?t=' + encodeURIComponent(TOKEN) + '&kind=' + kind + '&name=' + encodeURIComponent(name);
const HELMET_CH = [['base', 'Base colour'], ['mro', 'Finish (_MRO)'], ['metallic', 'Metallic'], ['roughness', 'Roughness'], ['ao', 'Ambient occlusion'], ['normal', 'Normal map']];

async function loadPilots() {
  try { pilots.idx = await api('/api/pilots'); } catch (e) { pilots.idx = null; }
  if (pilots.pick && !pilotNow()) pilots.pick = '';
  await loadSavedPortraits(true);
  renderPilots();
  loadNames();
}

// New portraits are kept in the workspace (my-portraits), so they are still here after a restart.
const fullPic = (path, v) => thumbURL(path) + '&alpha=1&w=1600&v=' + encodeURIComponent(v || '');
function savedEdit(sp) {
  return { picture: sp.picture, url: thumbURL(sp.picture) + '&alpha=1&w=600&v=' + encodeURIComponent(sp.saved), state: sp.state, photo: sp.photo, base: sp.base, from: sp.from, saved: sp.saved };
}
async function loadSavedPortraits(quiet) {
  let r;
  try { r = await api('/api/pilots/portraits'); } catch (e) { return; }
  pilots.edits = Object.fromEntries(r.saved.map(sp => [sp.name, savedEdit(sp)]));
  pilots.inMods = r.inMods || [];
  if (quiet !== true) { renderPortraits(); pilotsButtons(); }
}
// The picture a portrait is made on: the game's own, or one read back out of a mod.
function baseOf(name) {
  const ed = pilots.edits[name];
  const path = ed ? (ed.base || (!ed.state ? ed.picture : '')) : '';
  return { path, url: path ? fullPic(path, ed.saved) : pimg('portrait', name) };
}
function pilotNow() { return pilots.idx && pilots.idx.pilots.find(p => p.id === pilots.pick); }
function pilotTitle(p) { return p.name || 'Pilot ' + p.id.slice(2); }
function partTitle(p, sh) {
  const helmets = new Set(p.sheets.map(x => x.helmet)).size;
  return (helmets > 1 ? 'Helmet ' + sh.helmet + ', part ' : 'Part ') + sh.part;
}
function defaultPilotMod() {
  const p = pilotNow();
  if (p) return ('Pilot_' + (p.name || p.id)).replace(/[^A-Za-z0-9_-]+/g, '_');
  return 'Portraits';
}

function renderPilots() {
  const idx = pilots.idx, has = !!(idx && (idx.pilots.length || idx.portraits.length));
  const c = STATE && STATE.checks;
  const need = c ? ['workspace', 'retoc', 'paks', 'aes'].filter(k => !c[k].ok) : [];
  const su = $('#piSetup');
  su.hidden = !need.length;
  if (need.length) su.replaceChildren(el('strong', { text: 'Pilots needs a little setup first. ' }), need.map(k => c[k].note).join(' '), ' ', el('button', { class: 'btn small', onclick: () => showPage('settings') }, 'Open Settings'));
  $('#piFind').textContent = has ? 'Look again' : 'Find pilots in the game files';
  $('#piFindHint').textContent = !has
    ? 'Reads the game\'s file list, fetches the radio portraits (they are small) and makes a small picture of each helmet. The full-size helmet files are deleted again.'
    : idx.stale ? 'The game has been updated since this list was made. Press Look again to use its current files.'
      : idx.pilots.length + (idx.pilots.length === 1 ? ' pilot' : ' pilots') + ' with a helmet and ' + idx.portraits.length + (idx.portraits.length === 1 ? ' radio portrait' : ' radio portraits') + '. Pick the pilot whose helmet you want to paint.';
  $('#piList').replaceChildren(...(idx ? idx.pilots : []).map(p => {
    const sh = p.sheets.find(s => s.preview);
    const on = p.id === pilots.pick;
    return el('button', { type: 'button', class: 'pilotcard' + (on ? ' on' : ''), 'aria-pressed': String(on), onclick: () => pickPilot(on ? '' : p.id) },
      el('div', { class: 'thumb' }, sh ? el('img', { src: pimg('helmet', sh.stem), alt: '', loading: 'lazy' }) : el('span', { class: 'ph', text: 'No picture' })),
      el('span', { class: 'n', text: pilotTitle(p) }),
      el('span', { class: 's', text: [p.callsign, p.id, p.sheets.length + (p.sheets.length === 1 ? ' helmet part' : ' helmet parts')].filter(Boolean).join(' · ') }));
  }));
  if (!pilots.nameTouched) $('#piName').value = defaultPilotMod();
  renderPilotHelmet();
  renderPortraits();
  pilotsButtons();
}

function pickPilot(id) {
  if (id === pilots.pick) return;
  if (pilots.files.length) { api('/api/inbox/clean', { batches: pilots.batches }).catch(() => {}); pilots.files = []; pilots.batches = []; pilots.plan = null; }
  pilots.pick = id;
  $('#piPaintOut').replaceChildren();
  renderPilots();
  if (id) $('#piHelmetBlock').scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function renderPilotHelmet() {
  const p = pilotNow();
  $('#piHelmetBlock').hidden = !p;
  if (!p) return;
  $('#piHelmetHead').textContent = pilotTitle(p) + '\'s helmet';
  $('#piHelmetHint').textContent = p.sheets.length > 1
    ? 'This helmet has ' + p.sheets.length + ' parts, each with its own pictures. Paint any of them; the parts you leave alone stay as they are.'
    : 'Paint the helmet\'s pictures; whatever you leave out stays as it is.';
  const usesOf = key => { const sp = pilots.plan && pilots.plan.sheets.find(s => s.key === key); return sp ? sp.uses : {}; };
  $('#piParts').replaceChildren(...p.sheets.map(sh => {
    const u = usesOf(sh.key);
    const got = HELMET_CH.filter(([ch]) => u[ch]);
    const card = el('div', { class: 'partcard' },
      el('div', { class: 'thumb' }, sh.preview ? el('img', { src: pimg('helmet', sh.stem), alt: '' }) : el('span', { class: 'ph', text: 'No picture' })),
      el('div', { class: 'meta' },
        el('b', { text: partTitle(p, sh) }),
        el('span', { class: 'mono', text: sh.stem }),
        el('span', { class: 'hint', text: 'In the game: ' + ['C', 'MRO', 'N'].filter(k => sh.textures[k]).map(k => ({ C: 'colour', MRO: 'finish', N: 'normal map' })[k]).join(', ') }),
        got.length ? el('ul', { class: 'uses' }, ...got.map(([ch, label]) => el('li', {}, el('span', { class: 'lab', text: label }), el('span', { class: 'f', text: shortFile(u[ch].path) }))))
          : el('span', { class: 'droptip', text: 'Drop this part\'s pictures here' })));
    wireDrop(card, files => addPilotFiles(files, sh.key));
    return card;
  }));
  const out = $('#piPlan');
  out.replaceChildren();
  const pl = pilots.plan;
  if (pl) {
    const notes = [];
    if (pl.problems.length) notes.push(el('div', { class: 'notice error', role: 'alert' }, ...pl.problems.map(x => el('p', { text: x }))));
    if (pl.unplaced.length) notes.push(el('div', { class: 'notice warn' }, el('strong', { text: 'Which part? ' }), 'These pictures do not say which helmet part they are for. Drop them on the part they belong to: ' + pl.unplaced.map(shortFile).join(', ')));
    if (pl.ignored.length) notes.push(el('div', { class: 'notice info' }, 'Not used, because the names do not say base colour, metallic, roughness, ambient occlusion or normal map: ' + pl.ignored.map(shortFile).join(', ')));
    out.append(...notes);
  }
  $('#piClear').hidden = !pilots.files.length;
}

async function addPilotFiles(list, sheet) {
  const pics = list.filter(f => isPicture(f.name));
  const out = $('#piPlan');
  if (!pics.length) { notice(out, 'warn', 'Drop PNG or TGA pictures.'); return; }
  const batch = newBatch();
  pilots.batches.push(batch);
  const bar = $('#piUp'), text = $('#piUpText');
  bar.hidden = false;
  try {
    for (let i = 0; i < pics.length; i++) {
      text.textContent = 'Copying ' + pics[i].name + ' (' + (i + 1) + ' of ' + pics.length + ')';
      const path = await upload(pics[i], batch, f => { bar.querySelector('i').style.width = Math.round((i + f) / pics.length * 100) + '%'; });
      addPilotPath(path, sheet);
    }
  } catch (e) { notice(out, 'error', e.message); }
  bar.hidden = true;
  replanPilot();
}
// A picture with the same name for the same part replaces the earlier one (painted again).
function addPilotPath(path, sheet) {
  const name = shortFile(path.replace(/\\/g, '/')).toLowerCase();
  pilots.files = pilots.files.filter(f => !(f.sheet === sheet && shortFile(f.path.replace(/\\/g, '/')).toLowerCase() === name));
  pilots.files.push({ path, sheet });
}
async function replanPilot() {
  if (!pilots.files.length) { pilots.plan = null; renderPilotHelmet(); pilotsButtons(); return; }
  try { pilots.plan = await api('/api/pilots/plan', { pilot: pilots.pick, files: pilots.files }); }
  catch (e) { notice($('#piPlan'), 'error', e.message); }
  renderPilotHelmet();
  pilotsButtons();
}
wireDrop($('#piDrop'), files => addPilotFiles(files, ''));
$('#piPick').addEventListener('click', async () => {
  try {
    const r = await api('/api/pick', { kind: 'folder', title: 'Choose the folder with your helmet pictures' });
    if (!r.paths || !r.paths.length) return;
    const l = await api('/api/pilots/listdir', { dir: r.paths[0] });
    if (!l.files.length) { notice($('#piPlan'), 'warn', 'That folder has no PNG or TGA pictures.'); return; }
    l.files.forEach(f => addPilotPath(f, ''));
    replanPilot();
  } catch (e) { notice($('#piPlan'), 'error', e.message); }
});
$('#piClear').addEventListener('click', () => {
  api('/api/inbox/clean', { batches: pilots.batches }).catch(() => {});
  pilots.files = []; pilots.batches = []; pilots.plan = null;
  renderPilotHelmet(); pilotsButtons();
});
$('#piFind').addEventListener('click', async () => {
  const out = $('#piFindOut'); out.replaceChildren();
  try { pilots.idx = await runJob('/api/pilots/find'); pilots.font = null; renderPilots(); loadNames(); }
  catch (e) { notice(out, 'error', e.message); }
});
$('#piPaint').addEventListener('click', async () => {
  const out = $('#piPaintOut'); out.replaceChildren();
  try {
    const r = await runJob('/api/pilots/paint', { pilot: pilots.pick, format: STATE.settings.imageFormat });
    out.replaceChildren(el('div', { class: 'notice good' }, el('p', {}, el('strong', { text: 'Saved ' + r.files.length + ' pictures. ' }), openFolderBtn(r.dir)), el('span', { class: 'path', text: r.dir }),
      el('p', { class: 'hint', text: 'Paint over them and drop them back here with the same names. Each picture says which part it is for, so a whole folder can be dropped at once.' })));
  } catch (e) { notice(out, 'error', e.message); }
});

// ---- radio portraits
function portraitInfo(name) { return pilots.idx && pilots.idx.portraits.find(x => x.name === name); }
function renderPortraits() {
  const idx = pilots.idx;
  $('#piPortraitBlock').hidden = !(idx && idx.portraits.length);
  if (!idx) return;
  const p = pilotNow();
  const mine = new Set(p ? p.portraits : []);
  const list = [...idx.portraits].sort((a, b) => Number(mine.has(b.name)) - Number(mine.has(a.name)));
  $('#piPorts').replaceChildren(...list.map(pt => {
    const ed = pilots.edits[pt.name];
    const on = !!(pc && pc.name === pt.name);
    return el('button', { type: 'button', class: 'portcard' + (on ? ' on' : ''), 'aria-pressed': String(on), title: pt.name, onclick: () => openComposer(pt.name) },
      el('div', { class: 'thumb' }, el('img', { src: ed ? ed.url : pimg('portrait', pt.name), alt: '', loading: 'lazy' })),
      el('span', { class: 'n' }, pt.label, ed ? el('span', { class: 'chip orange', text: 'New' }) : mine.has(pt.name) ? el('span', { class: 'chip blue', text: pilotTitle(p) }) : null));
  }));
  const n = Object.keys(pilots.edits).length;
  $('#piPortCount').textContent = n ? n + (n === 1 ? ' new portrait is' : ' new portraits are') + ' ready for the mod.' : '';
  // portraits made before they were kept, still inside a mod built here
  const label = name => { const pt = portraitInfo(name); return pt ? pt.label : name.replace(portraitPrefixJS, ''); };
  $('#piPortMods').replaceChildren(...pilots.inMods.map(m => el('div', { class: 'notice info', style: 'margin:10px 0' },
    el('strong', { text: 'Your mod ' + m.mod + ' has new portraits that are not in this list: ' }), m.names.map(label).join(', ') + '. ',
    'Bring them back to keep them and add to them; building the mod again without them puts the game\'s own back. ',
    el('button', { class: 'btn small', type: 'button', onclick: () => recoverPortraits(m.mod) }, 'Bring them back'))));
}
const portraitPrefixJS = /^T_HUD_SubtitleSpeakerPortrait_00_/;
async function recoverPortraits(mod) {
  const out = $('#piPortMods');
  try {
    const r = await api('/api/pilots/portraits/recover', { mod });
    await loadSavedPortraits();
    out.prepend(el('div', { class: 'notice good', style: 'margin:10px 0', text: r.names.length + (r.names.length === 1 ? ' portrait is' : ' portraits are') + ' back. Click one to change it; Build puts them all in the mod.' }));
  } catch (e) { notice(out, 'error', e.message); }
}

// The portrait layout, measured on the game's own portraits (428 x 540): the photo sits in a
// soft-edged box, the text below it is drawn at 50% with a faint blurred copy behind it.
const PT = {
  w: 428, h: 540, photo: [24, 24, 380, 380], copy: [20, 20, 408, 408], textTop: 406,
  name: [39, 27, 457], call: [20, 26, 485], rank: [20, 26.5, 512], gap: 15, alpha: 0.5,
  ghost: { dx: -5, dy: 5, blur: 1.4, alpha: 0.18 }, green: 'rgb(107,255,142)',
};
let pc = null; // the portrait being made
const loadImg = src => new Promise((res, rej) => { const i = new Image(); i.onload = () => res(i); i.onerror = () => rej(new Error('That picture could not be read.')); i.src = src; });

async function gameFont() {
  if (pilots.font !== null) return pilots.font;
  pilots.font = false;
  try {
    const r = await fetch('/api/pilots/font', { headers: { 'X-AC8-Token': TOKEN } });
    if (r.ok) { const ff = new FontFace('AcesStudio', await r.arrayBuffer()); await ff.load(); document.fonts.add(ff); pilots.font = true; }
  } catch (e) {}
  return pilots.font;
}
function textFamily() { return pilots.font ? 'AcesStudio' : 'Bahnschrift, "DIN Alternate", "Segoe UI", sans-serif'; }

function drawPortraitText(ctx, f) {
  const up = s => String(s || '').trim().toUpperCase();
  const fam = textFamily();
  const runs = [];
  if (up(f.name)) runs.push([up(f.name), PT.name, '#fff']);
  if (up(f.callsign)) runs.push([up(f.callsign), PT.call, '#fff']);
  let x = PT.rank[1];
  if (up(f.rank)) {
    runs.push([up(f.rank), PT.rank, '#fff']);
    ctx.font = PT.rank[0] + 'px ' + fam;
    x += ctx.measureText(up(f.rank)).width + PT.gap;
  }
  if (up(f.unit)) runs.push([up(f.unit), [PT.rank[0], x, PT.rank[2]], PT.green]);
  for (const ghost of [true, false]) {
    ctx.save();
    if (ghost) { ctx.filter = 'blur(' + PT.ghost.blur + 'px)'; ctx.globalAlpha = PT.ghost.alpha; } else ctx.globalAlpha = PT.alpha;
    for (const [t, [size, tx, ty], colour] of runs) {
      ctx.font = size + 'px ' + fam; ctx.fillStyle = colour;
      ctx.fillText(t, tx + (ghost ? PT.ghost.dx : 0), ty + (ghost ? PT.ghost.dy : 0));
    }
    ctx.restore();
  }
}

function pixelsOf(img, W, H) {
  const c = document.createElement('canvas'); c.width = W; c.height = H;
  const x = c.getContext('2d', { willReadFrequently: true });
  x.drawImage(img, 0, 0, W, H);
  return x.getImageData(0, 0, W, H);
}
// The person's picture in the photo box: it covers the box, zoomed and moved as they chose.
function photoLayer(W, H) {
  const c = document.createElement('canvas'); c.width = W; c.height = H;
  const x = c.getContext('2d', { willReadFrequently: true });
  const sx = W / PT.w, sy = H / PT.h;
  const [bx, by, bw, bh] = PT.photo;
  const im = pc.mine, s = Math.max(bw / im.naturalWidth, bh / im.naturalHeight) * pc.zoom;
  const dw = im.naturalWidth * s, dh = im.naturalHeight * s;
  pc.ox = Math.max(-(dw - bw) / 2, Math.min((dw - bw) / 2, pc.ox));
  pc.oy = Math.max(-(dh - bh) / 2, Math.min((dh - bh) / 2, pc.oy));
  x.scale(sx, sy);
  x.fillStyle = '#808080'; x.fillRect(0, 0, PT.w, PT.h);
  x.drawImage(im, bx + (bw - dw) / 2 + pc.ox, by + (bh - dh) / 2 + pc.oy, dw, dh);
  const d = x.getImageData(0, 0, W, H);
  if (pc.soften) {
    const p = d.data;
    for (let i = 0; i < p.length; i += 4) {
      const l = 0.299 * p[i] + 0.587 * p[i + 1] + 0.114 * p[i + 2];
      for (let k = 0; k < 3; k++) p[i + k] = 14 + 0.9 * (l + (p[i + k] - l) * 0.72);
    }
  }
  return d;
}
// composePortrait makes the finished picture of one portrait, with its transparency.
function composePortrait(orig, other) {
  const W = orig.naturalWidth, H = orig.naturalHeight, sx = W / PT.w, sy = H / PT.h;
  const cv = document.createElement('canvas'); cv.width = W; cv.height = H;
  const ctx = cv.getContext('2d', { willReadFrequently: true });
  const base = pixelsOf(orig, W, H), d = base.data;
  let src = null;
  if (pc.photo === 'mine' && pc.mine) src = photoLayer(W, H);
  if (pc.photo === 'other' && other) src = pixelsOf(other, W, H);
  if (src) {
    const s = src.data, [x0, y0, x1, y1] = [PT.copy[0] * sx, PT.copy[1] * sy, PT.copy[2] * sx, PT.copy[3] * sy].map(Math.round);
    for (let y = y0; y < y1; y++) for (let x = x0; x < x1; x++) {
      const i = (y * W + x) * 4;
      if (d[i + 3]) { d[i] = s[i]; d[i + 1] = s[i + 1]; d[i + 2] = s[i + 2]; }
    }
  }
  const top = Math.round(PT.textTop * sy) * W * 4;
  if (pc.text === 'other' && other) d.set(pixelsOf(other, W, H).data.subarray(top), top);
  if (pc.text === 'new') d.fill(0, top);
  ctx.putImageData(base, 0, 0);
  if (pc.text === 'new') { ctx.save(); ctx.scale(sx, sy); drawPortraitText(ctx, pc.fields); ctx.restore(); }
  return cv;
}

let pcFrame = 0;
function drawComposer() {
  if (pcFrame) return;
  pcFrame = requestAnimationFrame(() => {
    pcFrame = 0;
    if (!pc || !pc.orig) return;
    const out = composePortrait(pc.orig, pc.otherImg);
    const cv = $('#pcCanvas'); cv.width = out.width; cv.height = out.height;
    cv.getContext('2d').drawImage(out, 0, 0);
  });
}
function pcRadio(name) { const r = document.querySelector('input[name=' + name + ']:checked'); return r ? r.value : ''; }
function syncComposer() {
  if (!pc) return;
  pc.photo = pcRadio('pcPhoto'); pc.text = pcRadio('pcText');
  pc.zoom = Number($('#pcZoom').value); pc.soften = $('#pcSoften').checked;
  pc.fields = { name: $('#pcTName').value, callsign: $('#pcTCall').value, rank: $('#pcTRank').value, unit: $('#pcTUnit').value };
  $('#pcMineBox').hidden = pc.photo !== 'mine';
  $('#pcOtherBox').hidden = pc.photo !== 'other';
  $('#pcTextOtherRow').hidden = pc.photo !== 'other';
  if (pc.photo !== 'other' && pc.text === 'other') { pc.text = 'keep'; document.querySelector('input[name=pcText][value=keep]').checked = true; }
  $('#pcNewBox').hidden = pc.text !== 'new';
  $('#pcWrap').classList.toggle('movable', pc.photo === 'mine' && !!pc.mine);
  $('#pcMineName').textContent = pc.mine ? pc.mineLabel : 'No picture chosen yet.';
  const pt = portraitInfo(pc.name);
  const twin = pilots.idx.portraits.find(x => x.name !== pc.name && x.label.split(' ')[0] === pt.label.split(' ')[0]);
  $('#pcTwinRow').hidden = !twin;
  if (twin) $('#pcTwinText').textContent = 'Do the same to ' + twin.label;
  pc.twin = twin && $('#pcTwin').checked ? twin.name : '';
  $('#pcUse').disabled = !!activeJob || (pc.photo === 'keep' && pc.text === 'keep') || (pc.photo === 'mine' && !pc.mine) || (pc.photo === 'other' && !pc.otherImg);
  $('#pcRemove').hidden = !pilots.edits[pc.name];
  drawComposer();
}

async function openComposer(name) {
  const pt = portraitInfo(name);
  if (!pt) return;
  const ed = pilots.edits[name];
  const fresh = { name, photo: 'keep', text: 'keep', mine: null, mineLabel: '', zoom: 1, ox: 0, oy: 0, soften: true, other: '', otherImg: null, fields: pt.text ? { ...pt.text } : { name: '', callsign: '', rank: '', unit: '' } };
  pc = ed && ed.state ? { ...fresh, ...ed.state, name, mine: null, otherImg: null } : fresh;
  pc.photoPath = ed && ed.photo || ''; pc.mineFile = null;
  pc.base = baseOf(name);
  const own = !!pc.base.path;
  $('#pcKeepPhoto').textContent = own ? 'Keep this portrait\'s photo' : 'Keep the game\'s photo';
  $('#pcKeepText').textContent = own ? 'Keep this portrait\'s text' : 'Keep the game\'s text';
  $('#pcBox').hidden = false;
  $('#pcTitle').textContent = pt.label; $('#pcName').textContent = pt.name;
  document.querySelector('input[name=pcPhoto][value=' + pc.photo + ']').checked = true;
  document.querySelector('input[name=pcText][value=' + pc.text + ']').checked = true;
  $('#pcZoom').value = pc.zoom; $('#pcSoften').checked = pc.soften;
  $('#pcTName').value = pc.fields.name; $('#pcTCall').value = pc.fields.callsign; $('#pcTRank').value = pc.fields.rank; $('#pcTUnit').value = pc.fields.unit;
  $('#pcTwin').checked = true;
  const others = pilots.idx.portraits.filter(x => x.name !== name);
  $('#pcOther').replaceChildren(el('option', { value: '', text: 'Choose a portrait' }), ...others.map(x => el('option', { value: x.name, text: x.label })));
  $('#pcOther').value = pc.other || '';
  renderPortraits();
  $('#pcBox').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  try {
    const [orig] = await Promise.all([loadImg(pc.base.url), gameFont()]);
    if (!pc || pc.name !== name) return;
    pc.orig = orig;
    // a kept portrait opens as it was made: its photo and the portrait it took one from
    if (pc.photo === 'mine' && pc.photoPath) { try { pc.mine = await loadImg(fullPic(pc.photoPath, ed && ed.saved)); } catch (e) { pc.photo = 'keep'; } }
    if (pc.other) { try { pc.otherImg = await loadImg(pimg('portrait', pc.other)); } catch (e) {} }
    if (!pc || pc.name !== name) return;
    document.querySelector('input[name=pcPhoto][value=' + pc.photo + ']').checked = true;
    $('#pcFontNote').textContent = pilots.font ? 'Written in the game\'s own font, in capitals like the game\'s portraits.' : 'The game\'s font could not be loaded, so a similar font of this computer is used. Press Look again above to fetch it.';
  } catch (e) { notice($('#pcMsg'), 'error', 'The game\'s portrait could not be loaded. Press Look again above.'); }
  syncComposer();
}

async function setMinePicture(file) {
  if (!pc || !file) return;
  if (!/^image\//.test(file.type) && !/\.(png|jpe?g|webp|bmp|gif)$/i.test(file.name)) { notice($('#pcMsg'), 'warn', 'Choose a PNG, JPEG or WebP picture.'); return; }
  try {
    const url = URL.createObjectURL(file);
    pc.mine = await loadImg(url); pc.mineLabel = file.name; pc.ox = 0; pc.oy = 0; pc.mineFile = file; pc.photoPath = '';
    document.querySelector('input[name=pcPhoto][value=mine]').checked = true;
    $('#pcMsg').replaceChildren();
    syncComposer();
  } catch (e) { notice($('#pcMsg'), 'error', e.message); }
}
$('#pcChoose').addEventListener('click', () => $('#pcFile').click());
$('#pcFile').addEventListener('change', ev => { setMinePicture(ev.target.files[0]); ev.target.value = ''; });
wireDrop($('#pcWrap'), files => setMinePicture(files[0]));
$('#pcOther').addEventListener('change', async () => {
  if (!pc) return;
  pc.other = $('#pcOther').value; pc.otherImg = null;
  if (pc.other) { try { pc.otherImg = await loadImg(pimg('portrait', pc.other)); } catch (e) { notice($('#pcMsg'), 'error', e.message); } }
  syncComposer();
});
$$('input[name=pcPhoto], input[name=pcText], #pcSoften, #pcTwin').forEach(x => x.addEventListener('change', syncComposer));
$$('#pcZoom, #pcTName, #pcTCall, #pcTRank, #pcTUnit').forEach(x => x.addEventListener('input', syncComposer));
// drag the picture to move it; the wheel zooms
(() => {
  const wrap = $('#pcWrap');
  let drag = null;
  wrap.addEventListener('pointerdown', ev => {
    if (!pc || pc.photo !== 'mine' || !pc.mine) return;
    drag = { x: ev.clientX, y: ev.clientY, ox: pc.ox, oy: pc.oy }; wrap.setPointerCapture(ev.pointerId);
  });
  wrap.addEventListener('pointermove', ev => {
    if (!drag) return;
    const k = PT.w / $('#pcCanvas').clientWidth;
    pc.ox = drag.ox + (ev.clientX - drag.x) * k; pc.oy = drag.oy + (ev.clientY - drag.y) * k;
    drawComposer();
  });
  const end = () => { drag = null; };
  wrap.addEventListener('pointerup', end); wrap.addEventListener('pointercancel', end);
  wrap.addEventListener('wheel', ev => {
    if (!pc || pc.photo !== 'mine' || !pc.mine) return;
    ev.preventDefault();
    const z = $('#pcZoom'); z.value = Math.max(1, Math.min(4, Number(z.value) * (ev.deltaY < 0 ? 1.08 : 1 / 1.08)));
    syncComposer();
  }, { passive: false });
})();
$('#pcClose').addEventListener('click', () => { pc = null; $('#pcBox').hidden = true; renderPortraits(); });
$('#pcRemove').addEventListener('click', async () => {
  if (!pc) return;
  const name = pc.name;
  try { await api('/api/pilots/portraits/remove', { name }); } catch (e) { notice($('#pcMsg'), 'error', e.message); return; }
  await loadSavedPortraits();
  openComposer(name); // starts again from the game's own portrait
});
// how a portrait was made, kept with it so it opens the same way next time
const portraitState = () => ({ photo: pc.photo, text: pc.text, zoom: pc.zoom, ox: pc.ox, oy: pc.oy, soften: pc.soften, other: pc.other || '', fields: { ...pc.fields }, mineLabel: pc.mineLabel || '' });
$('#pcUse').addEventListener('click', async () => {
  if (!pc || !pc.orig) return;
  const targets = [[pc.name, pc.orig, pc.base.path]];
  try {
    if (pc.twin) { const b = baseOf(pc.twin); targets.push([pc.twin, await loadImg(b.url), b.path]); }
    const batch = newBatch();
    pilots.batches.push(batch);
    // the photo is kept with the portrait, so it can be moved or zoomed again later
    let photo = pc.photo === 'mine' ? pc.photoPath : '';
    if (pc.photo === 'mine' && pc.mineFile) photo = await upload(pc.mineFile, batch);
    const state = portraitState();
    for (const [name, orig, base] of targets) {
      const cv = composePortrait(orig, pc.otherImg);
      const blob = await new Promise(r => cv.toBlob(r, 'image/png'));
      const path = await upload(new File([blob], name + '.png', { type: 'image/png' }), batch);
      const sp = await api('/api/pilots/portraits/save', { name, picture: path, photo, base, state });
      pilots.edits[name] = savedEdit(sp);
    }
    const mine = pilots.edits[pc.name];
    pc.mineFile = null; pc.photoPath = mine.photo || ''; pc.base = baseOf(pc.name);
    api('/api/inbox/clean', { batches: [batch] }).catch(() => {});
    notice($('#pcMsg'), 'good', (targets.length > 1 ? 'Both portraits are ready for the mod' : 'The portrait is ready for the mod') + ' and kept for next time.');
  } catch (e) { notice($('#pcMsg'), 'error', e.message); }
  syncComposer(); renderPortraits(); pilotsButtons();
});

// ---- build
$('#piName').addEventListener('input', () => { pilots.nameTouched = true; pilotsButtons(); });
function pilotsNotReady() {
  const pics = pilots.plan ? pilots.plan.sheets.reduce((n, s) => n + Object.keys(s.uses).length, 0) : 0;
  const ports = Object.keys(pilots.edits).length;
  if (!STATE || !STATE.checks.retoc.ok) return 'Set up retoc in Settings first.';
  if (!pics && !ports) return 'Add helmet pictures or a new portrait first.';
  if (pilots.plan && pilots.plan.problems.length) return 'Sort out the helmet pictures first (see above).';
  if (pilots.plan && pilots.plan.unplaced.length) return 'Drop each picture on the helmet part it is for.';
  if (!$('#piName').value.trim()) return 'Give the mod a name.';
  return '';
}
function pilotsButtons() {
  if (!$('#piGo')) return;
  const busy = !!activeJob;
  const exOK = STATE && ['workspace', 'retoc', 'paks', 'aes'].every(k => STATE.checks[k].ok);
  $('#piFind').disabled = busy || !exOK;
  $('#piPaint').disabled = busy || !exOK || !pilotNow();
  const why = pilotsNotReady();
  $('#piGo').disabled = busy || !!why;
  $('#piGoHint').textContent = busy ? '' : why;
  if (STATE) {
    $('#piInstall').disabled = !STATE.checks.mods.ok;
    if (!STATE.checks.mods.ok) $('#piInstall').checked = false;
    $('#piInstallHint').textContent = STATE.checks.mods.ok ? STATE.settings.modsDir : 'Set your mods folder in Settings to use this.';
    $('#piPick').hidden = !STATE.canPick;
  }
  if (pc) $('#pcUse').disabled = busy || (pc.photo === 'keep' && pc.text === 'keep') || (pc.photo === 'mine' && !pc.mine) || (pc.photo === 'other' && !pc.otherImg);
}
$('#piGo').addEventListener('click', async () => {
  const out = $('#piResult'); out.replaceChildren();
  const portraits = Object.entries(pilots.edits).map(([name, ed]) => ({ name, picture: ed.picture }));
  try {
    const r = await runJob('/api/pilots/build', { pilot: pilots.pick, files: pilots.files, portraits, modName: $('#piName').value.trim(), install: $('#piInstall').checked });
    const box = el('div', { class: 'notice good results' }, el('p', {}, el('strong', { text: 'Mod ' + r.mod + ' is ready. ' }), openFolderBtn(r.installedDir || r.packedDir)),
      el('p', { class: 'hint', text: 'Changed: ' + r.built.join(', ') }));
    if (r.installedDir) box.append(el('p', { text: 'Copied into your mods folder. It loads the next time the game starts.' }));
    else box.append(el('p', { text: 'Copy the three files in that folder into a folder of their own in your mods folder.' }));
    if (r.conflicts && r.conflicts.length) box.append(el('p', { class: 'warn', text: 'Another installed mod replaces some of the same files: ' + r.conflicts.map(c => c.with).join(', ') + '. The game uses only one of them; the Mods screen can remove one.' }));
    out.replaceChildren(box);
    loadMods(true);
    loadSavedPortraits();
  } catch (e) { notice(out, 'error', e.message); }
});
