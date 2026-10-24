// ---------------------------------------------------------------- new skin slots (Repack in "New skin slot" mode)
const radio = name => (document.querySelector('input[name="' + name + '"]:checked') || {}).value;
const pad3 = n => String(n).padStart(3, '0');

function selectedSkin() { return LIB.find(sk => sk.dir === $('#rpSkin').value); }
function folderNumberOf(sk) { return sk && /^\d{3}$/.test(sk.folder) ? +sk.folder : 0; }

function templateHint() {
  const sk = selectedSkin();
  const n = folderNumberOf(sk);
  const box = $('#rpTemplate');
  box.placeholder = n ? pad3(n) : 'number';
  $('#rpTemplateHint').textContent = sk
    ? (n ? 'The new skin starts as a copy of ' + sk.aircraft + ' skin ' + pad3(n) + ': its material, finish and normal map, and where emblems may go. Type another number to start from a different skin of this aircraft.'
         : 'This folder is not named after a skin number, so type the number of the game skin to start from.')
    : 'Choose the skin first.';
}

function fillModSelect() {
  const sel = $('#slMod'), keep = sel.value;
  const own = (SLOTS && SLOTS.mods || []).filter(m => m.own);
  sel.replaceChildren(...(own.length ? own.map(m => el('option', { value: m.key, text: m.title + ' (' + modContentsText(m) + ')' })) : [el('option', { value: '', text: 'No skin mods made here yet' })]));
  if (own.some(m => m.key === keep)) sel.value = keep;
  const addRadio = document.querySelector('input[name="slModMode"][value="add"]');
  addRadio.disabled = !own.length;
  if (!own.length && addRadio.checked) document.querySelector('input[name="slModMode"][value="new"]').checked = true;
  syncModMode();
}
function syncModMode() {
  const add = radio('slModMode') === 'add';
  $('#slNewModBox').hidden = add; $('#slAddModBox').hidden = !add;
}
$$('input[name="slModMode"]').forEach(r => r.addEventListener('change', () => { syncModMode(); updateButtons(); }));
$('#slMod').addEventListener('change', updateButtons);

function renderIconPreview() {
  const mode = radio('slIcon');
  $('#slColourBox').hidden = mode !== 'recolour';
  $('#slPicBox').hidden = mode !== 'picture';
  $('#slColour').disabled = $('#slColourAuto').checked;
  $('#slColourText').textContent = $('#slColourAuto').checked ? 'worked out from your paint when building' : $('#slColour').value;
  const f = $('#slIconPreview');
  const ph = text => el('span', { class: 'ph', text });
  if (mode === 'picture') {
    if (!repack.iconPicture) { f.replaceChildren(ph('Drop a picture here')); return; }
    f.replaceChildren(el('img', { src: '/api/slot/iconpreview?t=' + encodeURIComponent(TOKEN) + '&mode=picture&picture=' + encodeURIComponent(repack.iconPicture), alt: 'Icon', onerror: () => f.replaceChildren(ph('That picture could not be read')) }));
    return;
  }
  const skin = $('#rpSkin').value;
  if (!skin) { f.replaceChildren(ph('Choose the skin to start from')); return; }
  const auto = mode === 'recolour' && $('#slColourAuto').checked;
  const url = '/api/slot/iconpreview?t=' + encodeURIComponent(TOKEN) + '&skin=' + encodeURIComponent(skin) + '&template=' + encodeURIComponent($('#rpTemplate').value) +
    '&mode=' + mode + '&colour=' + encodeURIComponent(mode === 'recolour' && !auto ? $('#slColour').value : '') + '&v=' + Date.now();
  const img = el('img', { src: url, alt: 'Icon' });
  img.onerror = () => f.replaceChildren(ph('The original icon is not fetched yet'),
    el('button', { class: 'btn small', type: 'button', style: 'position:absolute;bottom:26px', onclick: async () => {
      try { await runJob('/api/slot/fetchicon', { skin, template: $('#rpTemplate').value }); } catch (e) { alertLine(e.message); }
      renderIconPreview();
    } }, 'Show it'));
  f.replaceChildren(...[img, auto ? el('span', { class: 'ph', style: 'position:absolute;bottom:0;left:0;right:0;background:rgba(0,0,0,.55);color:#fff', text: 'recoloured from your paint' }) : null].filter(Boolean));
}
$$('input[name="slIcon"]').forEach(r => r.addEventListener('change', () => { renderIconPreview(); updateButtons(); }));
$('#slColourAuto').addEventListener('change', () => { renderIconPreview(); updateButtons(); });
$('#slColour').addEventListener('input', renderIconPreview);
['#slName', '#slModTitle'].forEach(s => $(s).addEventListener('input', updateButtons));
$('#rpTemplate').addEventListener('input', () => { updateButtons(); clearTimeout(repack.tplTimer); repack.tplTimer = setTimeout(renderIconPreview, 400); });
wireDrop($('#slIconPreview'), async files => {
  const pic = files.find(f => /\.(png|jpe?g|tga)$/i.test(f.name));
  if (!pic) { alertLine('Drop a PNG, JPEG or TGA picture for the icon.'); return; }
  try {
    repack.iconBatch = repack.iconBatch || newBatch();
    repack.iconPicture = await upload(pic, repack.iconBatch);
    document.querySelector('input[name="slIcon"][value="picture"]').checked = true;
    renderIconPreview(); updateButtons();
  } catch (e) { alertLine(e.message); }
});
$('#slPicPick').addEventListener('click', async () => {
  try {
    const r = await api('/api/pick', { kind: 'file', filter: 'pictures', title: 'Choose a picture for the icon' });
    if (r.paths && r.paths.length) { repack.iconPicture = r.paths[0]; renderIconPreview(); updateButtons(); }
  } catch (e) { alertLine(e.message); }
});

function slotNotReady() {
  if (!STATE) return '';
  const c = STATE.checks;
  for (const k of ['workspace', 'retoc', 'paks', 'aes', 'mods']) if (!c[k].ok) return 'Finish the setup in Settings first.';
  if (!$('#rpSkin').value) return 'Choose the skin to start from.';
  if (!folderNumberOf(selectedSkin()) && !+$('#rpTemplate').value) return 'Type the number of the game skin to start from.';
  if (!$('#slName').value.trim()) return 'Give the skin a name.';
  if (radio('slModMode') === 'add' && !$('#slMod').value) return 'Choose the skin mod to add it to.';
  if (radio('slIcon') === 'picture' && !repack.iconPicture) return 'Choose the picture for the icon.';
  if (radio('slIcon') === 'recolour' && $('#slColourAuto').checked && !repack.rows.some(x => x.kind === 'colour' && repack.picked.has(x.name)))
    return 'To colour the icon from your paint, tick the colour texture, or pick a colour.';
  return '';
}

// ", BC7, no mipmaps" when a skin's colour texture is not stored the game's way
const texWords = t => (t && t.bc7 ? ', BC7' : '') + (t && t.noMips ? ', no mipmaps' : '');
async function buildSlot() {
  const out = $('#rpResult'); out.replaceChildren();
  const add = radio('slModMode') === 'add';
  const body = {
    skinDir: $('#rpSkin').value, paintDir: repack.paintDir, files: repack.files, textures: Array.from(repack.picked), choices: repack.choices, cleanup: [],
    template: 0, name: $('#slName').value, description: $('#slDesc').value, unlock: 'mission',
    icon: radio('slIcon'), iconColour: $('#slColourAuto').checked ? '' : $('#slColour').value, iconPicture: repack.iconPicture,
    mod: add ? $('#slMod').value : '', modTitle: $('#slModTitle').value, author: $('#slAuthor').value,
    slot: repack.rebuild && add && repack.rebuild.mod === $('#slMod').value ? repack.rebuild.slot : '',
    tex: texOptions(),
  };
  const sk = selectedSkin();
  if (+$('#rpTemplate').value && sk) body.template = aircraftNumber(sk.aircraft) * 1000000 + (+$('#rpTemplate').value) * 1000 + 1;
  try {
    const res = await runJob('/api/slot/build', body);
    out.replaceChildren(slotResultCard(res));
    await loadSlots(true);
    loadMods(true);
    // pressing Build again now updates this skin instead of making a second one
    repack.rebuild = { mod: res.mod, slot: res.slot };
    document.querySelector('input[name="slModMode"][value="add"]').checked = true;
    fillModSelect(); $('#slMod').value = res.mod; syncModMode(); updateButtons();
  } catch (e) { notice(out, 'error', e.message); }
}
function startAnotherSkin() {
  repack.rebuild = null;
  $('#slName').value = ''; $('#slDesc').value = '';
  $('#rpResult').replaceChildren();
  $('#slName').focus();
  updateButtons();
}
function aircraftNumber(name) { const m = /^[A-Za-z]+0*(\d+)_/.exec(name || ''); return m ? +m[1] : 0; }

function slotResultCard(res) {
  const box = el('div', { class: 'notice good results' });
  const head = el('div', { style: 'display:flex;gap:14px;align-items:center' },
    res.icon ? el('img', { src: thumbURL(res.icon) + '&w=180', alt: '', style: 'width:72px;height:72px;border:1px solid var(--line-strong)' }) : null,
    el('div', {}, el('p', {}, el('strong', { text: res.name + ' is skin ' + pad3(res.number) + ' of the ' + res.plane + '. ' })),
      el('p', { text: 'It is in the skin mod "' + res.modTitle + '" (' + res.folder + '), installed in your mods folder, and loads the next time the game starts.' })));
  box.append(head);
  box.append(el('p', {}, el('strong', { text: 'To see it in the hangar, finish a campaign mission first: ' }), 'the game unlocks new skins when a mission ends, the same way it unlocks its own first skins. After that it stays in the list.'));
  for (const n of res.notes || []) box.append(el('p', { class: 'hint', text: 'Note: ' + n }));
  const moved = ((res.list && res.list.slots) || []).filter(p => p.moved);
  for (const p of moved) box.append(el('p', { class: 'hint', text: p.name + ' (' + p.modTitle + ') wanted skin ' + pad3(p.wanted) + ', which is taken, so it is skin ' + pad3(p.number) + '.' }));
  box.append(el('p', { class: 'hint', text: 'Building again now updates this skin. To make another skin, press "Start another skin".' }));
  box.append(el('div', { class: 'row', style: 'margin-top:8px' },
    el('button', { class: 'btn small', onclick: () => showPage('slots') }, 'Open Skin slots'),
    el('button', { class: 'btn small', onclick: () => exportMod(res.mod, $('#rpResult')) }, 'Share as a .zip'),
    el('button', { class: 'btn small', onclick: startAnotherSkin }, 'Start another skin')));
  const wrap = el('div', { style: 'display:flex;flex-direction:column;gap:10px' }, box);
  for (const p of (res.setup || []).concat((res.list && res.list.problems) || [])) wrap.append(el('div', { class: 'notice warn', text: p }));
  return wrap;
}

// ---------------------------------------------------------------- skin slots screen
let SLOTS = null;
async function loadSlots(quiet) {
  try { SLOTS = await api('/api/slots'); } catch (e) { if (quiet !== true) notice($('#slList'), 'error', e.message); return; }
  HUD.loader = SLOTS.ok ? (SLOTS.loader || '') : null;
  HUD.slots = SLOTS.mods.reduce((a, m) => a + m.slots.length, 0);
  setHud();
  const b = $('#navSlotsBadge');
  if (SLOTS.stale) { b.hidden = false; b.textContent = '!'; b.classList.remove('blue'); b.title = 'The skin list needs rebuilding'; }
  else { b.hidden = !HUD.slots; b.textContent = HUD.slots; b.classList.add('blue'); b.title = ''; }
  fillModSelect();
  fillEmblemModSelect();
  if (quiet !== true) renderSlots();
}

function stat(label, val, cls, hint) {
  return el('div', { class: 'stat' }, el('span', { class: 'mono-label', text: label }), el('span', { class: 'val ' + (cls || ''), text: val }), hint ? el('span', { class: 'hint', text: hint }) : null);
}

function renderSlots() {
  const S = SLOTS;
  const setup = $('#slSetupNote');
  setup.hidden = S.ok;
  if (!S.ok) setup.replaceChildren(el('strong', { text: 'No mods folder yet. ' }), S.note, ' ', el('button', { class: 'btn small', onclick: () => showPage('settings') }, 'Open Settings'));
  const sst = S.sst || {};
  const count = S.mods.reduce((a, m) => a + m.slots.length, 0);
  const emblems = S.mods.reduce((a, m) => a + (m.emblems || []).length, 0);
  $('#slStats').replaceChildren(
    stat('Mod loader', S.loader ? 'v' + S.loader : 'not found', S.loader ? (versionLess(S.loader, '1.1.0') ? 'bad' : 'good') : 'bad', S.loader && versionLess(S.loader, '1.1.0') ? 'New skins need 1.1.0 or newer' : (S.loader ? 'Loads new skin files' : 'Needed: AC8 Mod Loader 1.1.0+')),
    stat('Names (SkinSlotText)', sst.installed ? (sst.disabled ? 'switched off' : 'installed') : (sst.dir ? 'not yet' : 'UE4SS not found'), sst.installed && !sst.disabled ? 'good' : (sst.dir ? 'warn' : 'bad'), sst.installed ? '' : 'Installed with the first new skin'),
    stat('Skin list', S.stale ? 'out of date' : S.listBuilt ? 'up to date' : (count ? 'not built' : 'not needed yet'), S.stale ? 'bad' : S.listBuilt ? 'good' : (count ? 'bad' : ''), S.listBuilt ? 'Built ' + new Date(S.listBuilt).toLocaleString() : 'Built when skins are added'),
    stat('New skins', String(count), count ? 'good' : '', S.mods.length + (S.mods.length === 1 ? ' skin mod' : ' skin mods') + (emblems ? ', and ' + emblems + (emblems === 1 ? ' new emblem' : ' new emblems') : '')));
  const probs = $('#slProblems');
  probs.replaceChildren(...(S.setup || []).concat(S.problems || []).map(p => el('div', { class: 'notice warn', style: 'margin-bottom:8px', text: p })));
  if (S.stale) probs.prepend(el('div', { class: 'notice error', style: 'margin-bottom:8px' }, el('strong', { text: 'The skin list needs rebuilding. ' }), S.stale, ' ',
    el('button', { class: 'btn small', onclick: () => slotJob('/api/skinlist/rebuild', {}, 'The skin list is up to date.') }, 'Rebuild the skin list')));

  $('#slModsHead').textContent = S.mods.length ? 'Installed skin mods' : 'No skin mods installed yet';
  const list = $('#slList');
  if (!S.mods.length) {
    list.replaceChildren(el('div', { class: 'notice', text: 'Make a new skin with "New skin slot" in the menu, a new emblem with "New emblem", or import a skin mod above.' }));
  } else {
    list.replaceChildren(...S.mods.map(modCard));
  }
  $('#slTablesBlock').hidden = !S.tableMods.length;
  $('#slTables').replaceChildren(...S.tableMods.map(n => el('span', { class: 'chip orange', text: n })));
  $('#slRemovedBlock').hidden = !S.removed.length;
  $('#slRemoved').replaceChildren(...S.removed.map(m => el('div', { class: 'smod' },
    el('div', { class: 'mhead' }, el('div', { class: 'mtitle' }, m.title, el('small', { text: modContentsText(m) + ', not in the mods folder' })),
      el('button', { class: 'btn small', onclick: () => slotJob('/api/slotmod/reinstall', { mod: m.key }, 'Put back in the mods folder.') }, 'Put it back')))));
}

function modCard(m) {
  const acts = el('div', { class: 'row' });
  const normal = () => acts.replaceChildren(
    el('button', { class: 'btn small', onclick: () => exportMod(m.key, $('#slResult')) }, 'Share as .zip'),
    el('button', { class: 'btn small', onclick: renameForm }, 'Rename'),
    openFolderBtn(m.dir, 'Open'),
    el('button', { class: 'btn small danger', onclick: confirmRemove }, 'Remove'));
  const confirmRemove = () => acts.replaceChildren(el('span', { class: 'hint', text: 'Take ' + m.title + ' out of the mods folder?' }),
    el('button', { class: 'btn small danger', onclick: () => slotJob('/api/slotmod/remove', { mod: m.key }, m.title + ' was taken out of the mods folder (it is kept in removed-mods).') }, 'Yes, remove'),
    el('button', { class: 'btn small', onclick: normal }, 'Keep it'));
  const renameForm = () => {
    const t = el('input', { type: 'text', value: m.title, maxlength: '60', 'aria-label': 'Mod name' });
    const a = el('input', { type: 'text', value: m.author || '', maxlength: '60', placeholder: 'Made by', 'aria-label': 'Made by' });
    acts.replaceChildren(t, a, el('button', { class: 'btn small', onclick: async () => {
      try { await api('/api/slotmod/rename', { mod: m.key, title: t.value, author: a.value }); loadSlots(); } catch (e) { alertLine(e.message); }
    } }, 'Save'), el('button', { class: 'btn small', onclick: normal }, 'Cancel'));
    t.focus();
  };
  normal();
  return el('div', { class: 'smod' },
    el('div', { class: 'mhead' },
      el('div', { class: 'mtitle' }, m.title,
        el('small', { text: [m.author ? 'by ' + m.author : '', m.folder, m.own ? 'made on this computer' : 'imported'].filter(Boolean).join('  ·  ') })),
      acts),
    el('div', { class: 'sgrid' }, m.slots.map(sl => slotCard(m, sl)), (m.emblems || []).map(e => emblemCard(m, e))));
}

function slotCard(m, sl) {
  const card = el('div', { class: 'scard' });
  const show = () => {
    card.replaceChildren(
      sl.iconPng ? el('img', { class: 'ic', src: thumbURL(sl.iconPng) + '&w=160', alt: '' }) : el('div', { class: 'ic' }),
      el('div', {},
        el('div', { class: 'nm', text: sl.name }),
        el('div', { class: 'meta' }, sl.plane + '  ·  ', sl.number ? el('b', { text: 'skin ' + pad3(sl.number) }) : 'not placed yet', sl.moved ? '  (wanted ' + pad3(sl.wanted) + ', taken)' : ''),
        el('div', { class: 'meta', text: (sl.textures || []).length ? 'own paint' + texWords(sl.tex) : 'look of skin ' + pad3(sl.template % 1000000 / 1000 | 0) }),
        sl.description ? el('div', { class: 'desc', text: sl.description }) : null),
      el('div', { class: 'acts' },
        el('button', { class: 'btn small', onclick: edit }, 'Edit name'),
        m.own ? el('button', { class: 'btn small', onclick: () => rebuildSlot(m, sl) }, 'Change paint or icon') : null,
        m.own && m.slots.length + (m.emblems || []).length > 1 ? el('button', { class: 'btn small danger', onclick: confirmRemove }, 'Remove') : null));
  };
  const confirmRemove = () => {
    const a = card.querySelector('.acts');
    a.replaceChildren(el('span', { class: 'hint', text: 'Remove ' + sl.name + ' from ' + m.title + '?' }),
      el('button', { class: 'btn small danger', onclick: () => slotJob('/api/slot/remove', { mod: m.key, slot: sl.key }, sl.name + ' was removed.') }, 'Yes, remove'),
      el('button', { class: 'btn small', onclick: show }, 'Keep it'));
  };
  const edit = () => {
    const nm = el('input', { type: 'text', value: sl.name, maxlength: '80', 'aria-label': 'Name' });
    const ds = el('textarea', { maxlength: '1000', 'aria-label': 'Description' }); ds.value = sl.description || '';
    show();
    card.querySelector('.acts').replaceChildren();
    card.append(el('div', { class: 'edit' },
      el('label', { class: 'field' }, 'Name', nm), el('label', { class: 'field' }, 'Description', ds),
      el('div', { class: 'row' },
        el('button', { class: 'btn small', onclick: () => slotJob('/api/slot/edit', { mod: m.key, slot: sl.key, name: nm.value, description: ds.value }, 'Saved. The new text shows the next time the game starts.') }, 'Save'),
        el('button', { class: 'btn small', onclick: show }, 'Cancel'))));
    nm.focus();
  };
  show();
  return card;
}

// "Change paint or icon": the New skin slot screen, set up to rebuild this skin.
function rebuildSlot(m, sl) {
  showPage('repack', 'slot');
  repack.rebuild = { mod: m.key, slot: sl.key };
  document.querySelector('input[name="slModMode"][value="add"]').checked = true;
  fillModSelect(); $('#slMod').value = m.key; syncModMode();
  $('#slName').value = sl.name; $('#slDesc').value = sl.description || '';
  const n = Math.floor(sl.template % 1000000 / 1000);
  const sk = LIB.find(s => s.aircraft === sl.vehicle && s.folder === pad3(n) && s.textures.some(t => t.supported));
  if (sk) { chooseSkin(sk.dir); skinChanged(); } else $('#rpTemplate').value = n;
  notice($('#rpResult'), 'info', el('strong', { text: 'Changing ' + sl.name + '. ' }), sk ? 'Drop new pictures if the paint changes (its own textures are kept otherwise), choose the icon and press Update this skin. It keeps its number.' : 'Extract ' + sl.vehicle + ' skin ' + pad3(n) + ' first (or add its raw files), then drop the new pictures.');
  updateButtons();
}

async function slotJob(path, body, okText) {
  const out = $('#slResult'); out.replaceChildren();
  try {
    const res = await runJob(path, body);
    const box = el('div', { class: 'notice good', role: 'status' }, el('strong', { text: okText + ' ' }));
    if (res && res.movedTo) box.append(el('span', { class: 'path', text: res.movedTo }), ' ', openFolderBtn(res.movedTo));
    out.replaceChildren(box);
    const list = res && (res.list || (res.slots ? res : null));
    for (const p of (list && list.problems) || []) out.append(el('div', { class: 'notice warn', style: 'margin-top:8px', text: p }));
  } catch (e) { notice(out, 'error', e.message); }
  await loadSlots();
  loadMods(true);
}

async function exportMod(key, out) {
  out.replaceChildren();
  try {
    const res = await runJob('/api/slotmod/export', { mod: key });
    notice(out, 'good', el('strong', { text: 'Ready to share: ' }), el('span', { class: 'path', text: res.zip }), ' (' + fmtSize(res.size) + '). ', openFolderBtn(res.zip, 'Show the file'),
      el('p', { class: 'hint', text: 'Anyone with ACE8 Skin Studio can drop this .zip on their Skin slots screen. It also works without the program, but then only one new-slot skin mod at a time (its README explains).' }));
  } catch (e) { notice(out, 'error', e.message); }
}

// ---------------------------------------------------------------- importing
const imp = { path: '', batch: '' };
wireDrop($('#imDrop'), files => importFiles(files));
// A skin mod's .zip can also be dropped anywhere in the window, or pasted (Ctrl+V after copying it in Explorer).
window.addEventListener('drop', async e => {
  if (e.target.closest && e.target.closest('.drop, .iconframe')) return;
  const files = await droppedFiles(e.dataTransfer);
  if (files.some(f => /\.zip$/i.test(f.name))) { showPage('slots'); importFiles(files); }
});
document.addEventListener('paste', e => {
  if (e.target.closest && e.target.closest('input, textarea')) return;
  const files = Array.from((e.clipboardData && e.clipboardData.files) || []);
  if (files.some(f => /\.zip$/i.test(f.name))) { e.preventDefault(); showPage('slots'); importFiles(files); }
});
async function importFiles(files) {
  const zip = files.find(f => /\.zip$/i.test(f.name));
  if (!zip) { notice($('#imPlan'), 'error', 'Drop a .zip file. A .7z or .rar file needs unpacking first; then choose its folder.'); return; }
  if (imp.batch && !activeJob) api('/api/inbox/clean', { batches: [imp.batch] }).catch(() => {});
  imp.batch = newBatch();
  const up = $('#imUp'), bar = $('.upbar i', up); up.hidden = false;
  $('#imUpText').textContent = 'Copying ' + zip.name;
  try { imp.path = await upload(zip, imp.batch, p => { bar.style.width = Math.round(p * 100) + '%'; }); }
  catch (e) { notice($('#imPlan'), 'error', e.message); up.hidden = true; return; }
  up.hidden = true; bar.style.width = '0';
  $('#imPlan').scrollIntoView({ block: 'center' });
  planImportUI();
}
$('#imPick').addEventListener('click', async () => {
  try {
    const r = await api('/api/pick', { kind: 'file', filter: 'zip', title: 'Choose a skin mod (.zip)' });
    if (r.paths && r.paths.length) { imp.path = r.paths[0]; imp.batch = ''; planImportUI(); }
  } catch (e) { notice($('#imPlan'), 'error', e.message); }
});
async function planImportUI() {
  const box = $('#imPlan'); box.replaceChildren();
  let plan;
  try { plan = await runJob('/api/import/plan', { path: imp.path }); } catch (e) { notice(box, 'error', e.message); return; }
  const card = el('div', { class: 'smod' });
  const m = plan.manifest;
  if (plan.studio) {
    card.append(el('div', { class: 'mhead' }, el('div', { class: 'mtitle' }, m.title,
      el('small', { text: [m.author ? 'by ' + m.author : '', plan.update ? 'replaces the copy you have installed' : 'goes into ' + plan.folder, 'made with ' + (m.madeWith || 'ACE8 Skin Studio')].filter(Boolean).join('  ·  ') }))));
    const rows = el('div', { class: 'list', style: 'border:0' });
    for (const sk of plan.skins) {
      const num = el('div', { class: 'num' });
      if (sk.wanted && sk.number !== sk.wanted) num.append(el('span', { class: 'was', text: pad3(sk.wanted) }), el('span', { class: 'now', text: pad3(sk.number) }));
      else num.append(el('span', { text: sk.number ? pad3(sk.number) : '—' }));
      rows.append(el('div', { class: 'plancard' },
        sk.icon ? el('img', { src: thumbURL(sk.icon) + '&w=120', alt: '' }) : el('div', { class: 'noimg' }),
        el('div', {}, el('div', { class: 'nm', style: 'font-family:var(--font-head);font-weight:600;letter-spacing:.04em', text: sk.name }),
          el('div', { class: 'hint', text: sk.vehicle + (sk.taken ? '  ·  skin ' + pad3(sk.wanted) + ' is taken by ' + sk.taken + ', so it gets ' + pad3(sk.number) : '') })),
        num));
    }
    for (const em of plan.emblems || []) {
      const num = el('div', { class: 'num' });
      if (em.wanted && em.number !== em.wanted) num.append(el('span', { class: 'was', text: String(em.wanted) }), el('span', { class: 'now', text: String(em.number) }));
      else num.append(el('span', { text: em.number ? String(em.number) : '—' }));
      rows.append(el('div', { class: 'plancard' },
        em.icon ? el('img', { class: 'embthumb', src: thumbURL(em.icon) + '&w=120&alpha=1', alt: '' }) : el('div', { class: 'noimg' }),
        el('div', {}, el('div', { class: 'nm', style: 'font-family:var(--font-head);font-weight:600;letter-spacing:.04em', text: em.name }),
          el('div', { class: 'hint', text: 'Emblem' + (em.wanted && em.number !== em.wanted ? '  ·  number ' + em.wanted + ' is taken, so it gets ' + em.number : '') })),
        num));
    }
    card.append(rows);
  } else {
    card.append(el('div', { class: 'mhead' }, el('div', { class: 'mtitle' }, 'Mod files',
      el('small', { text: 'Not made with ACE8 Skin Studio; installed as they are.' }))));
    const rows = el('div', { class: 'list', style: 'border:0' });
    for (const c of plan.containers) rows.append(el('div', { class: 'skin' }, el('div', { class: 'line' },
      el('div', { class: 'id' }, c.folder, el('small', { text: c.files.map(shortFile).join(', ') })), c.skinList ? el('span', { class: 'chip orange', text: 'own skin list: merged' }) : null)));
    card.append(rows);
  }
  const go = el('button', { class: 'btn primary', onclick: async () => {
    const out = $('#imPlan');
    try {
      const res = await runJob('/api/import', { path: imp.path, cleanup: imp.batch ? [imp.batch] : [] });
      imp.batch = '';
      const done = el('div', { class: 'notice good' }, el('strong', { text: plan.studio ? m.title + ' is installed. ' : 'Installed. ' }), 'It loads the next time the game starts.');
      const moved = ((res.list && res.list.slots) || []).filter(p => p.moved && (!m || p.mod === m.key));
      for (const p of moved) done.append(el('p', { class: 'hint', text: p.name + ' is skin ' + pad3(p.number) + ' of the ' + p.plane + ' (it wanted ' + pad3(p.wanted) + ').' }));
      if (plan.studio && (plan.skins.length || (plan.emblems || []).length)) done.append(el('p', { class: 'hint' }, 'New skins and emblems unlock when you finish a campaign mission, or straight away with ', el('button', { class: 'linkbtn', type: 'button', onclick: () => showPage('save') }, 'Unlock now'), ' on the Save tools screen.'));
      out.replaceChildren(done);
      for (const p of (res.setup || []).concat((res.list && res.list.problems) || [])) out.append(el('div', { class: 'notice warn', style: 'margin-top:8px', text: p }));
    } catch (e) { notice(out, 'error', e.message); }
    loadSlots(); loadMods(true);
  } }, plan.update ? 'Update it' : 'Install');
  box.replaceChildren(card, ...plan.notes.map(n => el('div', { class: 'notice info', style: 'margin-top:8px', text: n })), el('div', { class: 'row', style: 'margin-top:12px' }, go, el('button', { class: 'btn quiet', onclick: () => box.replaceChildren() }, 'Cancel')));
}

$('#slRefresh').addEventListener('click', () => { $('#slResult').replaceChildren(); loadSlots(); });
$('#slNew').addEventListener('click', () => showPage('repack', 'slot'));
$('#slRebuild').addEventListener('click', () => slotJob('/api/skinlist/rebuild', {}, 'The skin list is up to date.'));
$('#slOpenShared').addEventListener('click', () => api('/api/open', { path: STATE.folders.shared }).catch(() => notice($('#slResult'), 'warn', 'Nothing has been shared yet. "Share as .zip" on a skin mod makes one.')));
