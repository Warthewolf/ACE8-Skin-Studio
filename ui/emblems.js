// ---------------------------------------------------------------- new emblems
const emb = { picture: '', batch: '', rebuild: null };

// "2 skins, 1 emblem"
function modContentsText(m) {
  const parts = [];
  const n = m.slots.length, e = (m.emblems || []).length;
  if (n || !e) parts.push(n + (n === 1 ? ' skin' : ' skins'));
  if (e) parts.push(e + (e === 1 ? ' emblem' : ' emblems'));
  return parts.join(', ');
}

function fillEmblemModSelect() {
  const sel = $('#emMod'), keep = sel.value;
  const own = (SLOTS && SLOTS.mods || []).filter(m => m.own);
  sel.replaceChildren(...(own.length ? own.map(m => el('option', { value: m.key, text: m.title + ' (' + modContentsText(m) + ')' })) : [el('option', { value: '', text: 'No skin mods made here yet' })]));
  if (own.some(m => m.key === keep)) sel.value = keep;
  const addRadio = document.querySelector('input[name="emModMode"][value="add"]');
  addRadio.disabled = !own.length;
  if (!own.length && addRadio.checked) document.querySelector('input[name="emModMode"][value="new"]').checked = true;
  syncEmblemMode();
}
function syncEmblemMode() {
  const add = radio('emModMode') === 'add';
  $('#emNewModBox').hidden = add; $('#emAddModBox').hidden = !add;
}
$$('input[name="emModMode"]').forEach(r => r.addEventListener('change', () => { syncEmblemMode(); updateButtons(); }));
['#emMod', '#emName', '#emModTitle'].forEach(s => $(s).addEventListener(s === '#emMod' ? 'change' : 'input', updateButtons));

// Changing an emblem that is already built (and not choosing a new picture) keeps its pictures.
function emblemUpdating() { return !!(emb.rebuild && radio('emModMode') === 'add' && $('#emMod').value === emb.rebuild.mod); }

function renderEmblemPreview() {
  const f = $('#emPreview');
  const ph = text => el('span', { class: 'ph', text });
  $('#emPicName').textContent = emb.picture ? shortFile(emb.picture.replace(/\\/g, '/')) : '';
  if (!emb.picture) {
    if (emblemUpdating() && emb.rebuild.icon) { f.replaceChildren(el('img', { src: thumbURL(emb.rebuild.icon) + '&w=256&alpha=1', alt: 'The emblem as it is' })); return; }
    f.replaceChildren(ph('No picture yet'));
    return;
  }
  f.replaceChildren(el('img', { src: '/api/emblem/preview?t=' + encodeURIComponent(TOKEN) + '&picture=' + encodeURIComponent(emb.picture) + '&v=' + Date.now(), alt: 'The new emblem',
    onerror: () => f.replaceChildren(ph('That picture could not be read')) }));
}

async function useEmblemFiles(files) {
  const pic = files.find(f => /\.(png|jpe?g|tga)$/i.test(f.name));
  if (!pic) { notice($('#emResult'), 'error', 'Drop a PNG, JPEG or TGA picture for the emblem.'); return; }
  const up = $('#emUp'), bar = $('.upbar i', up);
  up.hidden = false;
  try {
    emb.batch = emb.batch || newBatch();
    emb.picture = await upload(pic, emb.batch, p => { bar.style.width = Math.round(p * 100) + '%'; });
    $('#emResult').replaceChildren();
  } catch (e) { notice($('#emResult'), 'error', e.message); }
  up.hidden = true; bar.style.width = '0';
  renderEmblemPreview(); updateButtons();
}
wireDrop($('#emDrop'), useEmblemFiles);
wireDrop($('#emPreview'), useEmblemFiles);
$('#emPick').addEventListener('click', async () => {
  try {
    const r = await api('/api/pick', { kind: 'file', filter: 'pictures', title: 'Choose the emblem\'s picture' });
    if (r.paths && r.paths.length) { emb.picture = r.paths[0]; renderEmblemPreview(); updateButtons(); }
  } catch (e) { notice($('#emResult'), 'error', e.message); }
});

function renderEmblemSetup() {
  if (!STATE) return;
  const c = STATE.checks, box = $('#emSetup');
  const need = ['workspace', 'retoc', 'paks', 'aes', 'mods'].filter(k => !c[k].ok);
  box.hidden = !need.length;
  if (need.length) box.replaceChildren(el('strong', { text: 'New emblems need a little setup first. ' }), need.map(k => c[k].note).join(' '), ' ', el('button', { class: 'btn small', onclick: () => showPage('settings') }, 'Open Settings'));
}

function emblemNotReady() {
  if (!STATE) return '';
  const c = STATE.checks;
  for (const k of ['workspace', 'retoc', 'paks', 'aes', 'mods']) if (!c[k].ok) return 'Finish the setup in Settings first.';
  if (!emb.picture && !emblemUpdating()) return 'Choose the emblem\'s picture.';
  if (!$('#emName').value.trim()) return 'Give the emblem a name.';
  if (radio('emModMode') === 'add' && !$('#emMod').value) return 'Choose the skin mod to add it to.';
  return '';
}
function emblemButtons() {
  const why = emblemNotReady();
  $('#emGo').disabled = !!activeJob || !!why;
  $('#emGo').textContent = emblemUpdating() ? 'Update this emblem' : 'Build new emblem';
  $('#emGoHint').textContent = activeJob ? '' : why;
}

async function buildEmblem() {
  const out = $('#emResult'); out.replaceChildren();
  const add = radio('emModMode') === 'add';
  const body = {
    mod: add ? $('#emMod').value : '', modTitle: $('#emModTitle').value, author: $('#emAuthor').value,
    emblem: emblemUpdating() ? emb.rebuild.emblem : '', name: $('#emName').value, description: $('#emDesc').value,
    picture: emb.picture, cleanup: emb.batch ? [emb.batch] : [],
  };
  try {
    const res = await runJob('/api/emblem/build', body);
    emb.batch = ''; emb.picture = '';
    emb.rebuild = { mod: res.mod, emblem: res.emblem, icon: res.icon };
    await loadSlots(true);
    loadMods(true);
    // pressing Build again now updates this emblem instead of making a second one
    document.querySelector('input[name="emModMode"][value="add"]').checked = true;
    fillEmblemModSelect(); $('#emMod').value = res.mod; syncEmblemMode();
    renderEmblemPreview(); updateButtons();
    out.replaceChildren(emblemResultCard(res));
  } catch (e) { notice(out, 'error', e.message); }
}
$('#emGo').addEventListener('click', buildEmblem);

function emblemResultCard(res) {
  const box = el('div', { class: 'notice good results' });
  box.append(el('div', { style: 'display:flex;gap:14px;align-items:center' },
    res.icon ? el('img', { class: 'embthumb', src: thumbURL(res.icon) + '&w=180&alpha=1', alt: '' }) : null,
    el('div', {}, el('p', {}, el('strong', { text: res.name + ' is emblem number ' + res.number + '. ' })),
      el('p', { text: 'It is in the skin mod "' + res.modTitle + '" (' + res.folder + '), installed in your mods folder, and loads the next time the game starts.' }))));
  box.append(el('p', {}, el('strong', { text: 'To see it in the emblem list, ' }), 'finish a campaign mission (the game unlocks new emblems when a mission ends), or close the game and press Unlock now on the Save tools screen.'));
  for (const p of ((res.list && res.list.emblems) || []).filter(p => p.moved && p.mod === res.mod && p.emblem === res.emblem))
    box.append(el('p', { class: 'hint', text: 'It wanted number ' + p.wanted + ', which is taken, so it is number ' + p.number + '.' }));
  box.append(el('p', { class: 'hint', text: 'Building again now updates this emblem. To make another, press "Start another emblem".' }));
  box.append(el('div', { class: 'row', style: 'margin-top:8px' },
    el('button', { class: 'btn small', onclick: () => showPage('save') }, 'Unlock it now'),
    el('button', { class: 'btn small', onclick: () => showPage('slots') }, 'Open Skin slots'),
    el('button', { class: 'btn small', onclick: () => exportMod(res.mod, $('#emResult')) }, 'Share as a .zip'),
    el('button', { class: 'btn small', onclick: startAnotherEmblem }, 'Start another emblem')));
  const wrap = el('div', { style: 'display:flex;flex-direction:column;gap:10px' }, box);
  for (const p of (res.setup || []).concat((res.list && res.list.problems) || [])) wrap.append(el('div', { class: 'notice warn', text: p }));
  return wrap;
}

function startAnotherEmblem() {
  emb.rebuild = null; emb.picture = '';
  $('#emName').value = ''; $('#emDesc').value = '';
  $('#emResult').replaceChildren();
  renderEmblemPreview(); updateButtons();
  $('#emName').focus();
}

// "Change picture" on the Skin slots screen: this screen, set up to change that emblem.
function changeEmblem(m, e) {
  showPage('emblem');
  emb.rebuild = { mod: m.key, emblem: e.key, icon: e.iconPng || '' };
  emb.picture = '';
  document.querySelector('input[name="emModMode"][value="add"]').checked = true;
  fillEmblemModSelect(); $('#emMod').value = m.key; syncEmblemMode();
  $('#emName').value = e.name; $('#emDesc').value = e.description || '';
  renderEmblemPreview();
  notice($('#emResult'), 'info', el('strong', { text: 'Changing ' + e.name + '. ' }), 'Drop a new picture (or keep the one it has), change the name if you like, and press Update this emblem. It keeps its number.');
  updateButtons();
}

function showEmblemPage() {
  renderEmblemSetup();
  fillEmblemModSelect();
  renderEmblemPreview();
  $('#emPick').hidden = !(STATE && STATE.canPick);
  loadSlots(true).then(() => { fillEmblemModSelect(); updateButtons(); });
}

// An emblem on the Skin slots screen.
function emblemCard(m, e) {
  const card = el('div', { class: 'scard' });
  const total = m.slots.length + (m.emblems || []).length;
  const show = () => {
    card.replaceChildren(
      e.iconPng ? el('img', { class: 'ic emb', src: thumbURL(e.iconPng) + '&w=160&alpha=1', alt: '' }) : el('div', { class: 'ic emb' }),
      el('div', {},
        el('div', { class: 'nm', text: e.name }),
        el('div', { class: 'meta' }, 'Emblem  ·  ', e.number ? el('b', { text: 'number ' + e.number }) : 'not placed yet', e.moved ? '  (wanted ' + e.wanted + ', taken)' : ''),
        e.description ? el('div', { class: 'desc', text: e.description }) : null),
      el('div', { class: 'acts' },
        el('button', { class: 'btn small', onclick: edit }, 'Edit name'),
        m.own ? el('button', { class: 'btn small', onclick: () => changeEmblem(m, e) }, 'Change picture') : null,
        m.own && total > 1 ? el('button', { class: 'btn small danger', onclick: confirmRemove }, 'Remove') : null));
  };
  const confirmRemove = () => {
    card.querySelector('.acts').replaceChildren(el('span', { class: 'hint', text: 'Remove ' + e.name + ' from ' + m.title + '?' }),
      el('button', { class: 'btn small danger', onclick: () => slotJob('/api/slot/remove', { mod: m.key, slot: e.key }, e.name + ' was removed.') }, 'Yes, remove'),
      el('button', { class: 'btn small', onclick: show }, 'Keep it'));
  };
  const edit = () => {
    const nm = el('input', { type: 'text', value: e.name, maxlength: '80', 'aria-label': 'Name' });
    const ds = el('textarea', { maxlength: '1000', 'aria-label': 'Description' }); ds.value = e.description || '';
    show();
    card.querySelector('.acts').replaceChildren();
    card.append(el('div', { class: 'edit' },
      el('label', { class: 'field' }, 'Name', nm), el('label', { class: 'field' }, 'Description', ds),
      el('div', { class: 'row' },
        el('button', { class: 'btn small', onclick: () => slotJob('/api/slot/edit', { mod: m.key, slot: e.key, name: nm.value, description: ds.value }, 'Saved. The new text shows the next time the game starts.') }, 'Save'),
        el('button', { class: 'btn small', onclick: show }, 'Cancel'))));
    nm.focus();
  };
  show();
  return card;
}
