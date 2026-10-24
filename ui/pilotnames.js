// ---------------------------------------------------------------- pilots: names on the radio
const pn = { view: null, names: {}, saved: {}, open: false };
const pnField = (kind, id) => kind + ':' + id;

async function loadNames() {
  try { pn.view = await api('/api/pilots/names'); } catch (e) { pn.view = null; }
  if (pn.view) { pn.saved = { ...pn.view.names }; pn.names = { ...pn.view.names }; }
  renderNames();
}

function namesDirty() {
  const keys = new Set([...Object.keys(pn.names), ...Object.keys(pn.saved)]);
  for (const k of keys) if ((pn.names[k] || '').trim() !== (pn.saved[k] || '').trim()) return true;
  return false;
}

// "Seen in your last game" / "Not seen in your last game" / "Start the game to see it"
function seenMark(field) {
  const v = pn.view;
  if (!v || !(pn.saved[field] || '').trim()) return null;
  const last = new Date(v.lastGame), saved = new Date(v.saved);
  if (v.seen[field]) return el('span', { class: 'seen yes', text: 'Seen in your last game' });
  if (isNaN(last) || last.getFullYear() < 2000 || last < saved) return el('span', { class: 'seen', text: 'Start the game to see it' });
  return el('span', { class: 'seen', text: 'Not seen in your last game' });
}

function nameInput(field, placeholder, label) {
  const inp = el('input', { type: 'text', maxlength: '40', spellcheck: 'false', placeholder, 'aria-label': label });
  inp.value = pn.names[field] || '';
  inp.addEventListener('input', () => { pn.names[field] = inp.value; namesButtons(); });
  return el('label', { class: 'field' }, el('span', { class: 'mono-label', text: label }), inp, seenMark(field));
}

function speakerRow(sp) {
  const known = sp.portrait && portraitInfo(sp.portrait); // fetched by Find pilots
  const face = known ? el('img', { class: 'face', src: pimg('portrait', sp.portrait), alt: '', loading: 'lazy', onerror: e => e.target.replaceWith(el('div', { class: 'face' })) }) : el('div', { class: 'face' });
  const sub = [sp.flight ? speakerFlightLabel(sp.flight) : '', sp.callsignLabel ? 'callsign ' + sp.callsignLabel : ''].filter(Boolean).join('  ·  ');
  const who = el('div', { class: 'who' }, el('b', { text: sp.label }), sub ? el('small', { text: sub }) : null);
  const name = nameInput(pnField('name', sp.id), sp.label, 'Name');
  const boxes = [name];
  if (sp.callsign) boxes.push(nameInput(pnField('callsign', sp.callsign), sp.callsignLabel, 'Callsign'));
  if (sp.hangar) {
    // the hangar's pilot list writes names in capitals: an empty box gives it the name in capitals
    const hangar = nameInput(pnField('hangar', sp.id), '', 'Hangar name');
    const hInput = $('input', hangar), nInput = $('input', name);
    const hint = () => { hInput.placeholder = (nInput.value.trim() || sp.label).toUpperCase(); };
    nInput.addEventListener('input', hint); hint();
    hInput.title = 'Leave it empty to use the name in capitals, as the hangar writes names.';
    boxes.push(hangar);
  }
  const cls = boxes.length === 3 ? 'namerow three' : boxes.length === 2 ? 'namerow' : 'namerow one';
  return el('div', { class: cls }, face, who, ...boxes);
}
function speakerFlightLabel(id) {
  const f = pn.view && pn.view.flights.find(x => x.id === id);
  return f ? f.label : id;
}

function renderNames() {
  const v = pn.view, block = $('#piNamesBlock');
  const c = STATE && STATE.checks;
  const ready = c && ['workspace', 'retoc', 'paks', 'aes'].every(k => c[k].ok);
  block.hidden = !v || !ready;
  if (block.hidden) return;
  const status = $('#pnStatus');
  status.replaceChildren();
  if (v.problem) status.append(el('div', { class: 'notice warn', style: 'margin-bottom:10px', text: v.problem }));
  for (const f of v.clashes || []) status.append(el('div', { class: 'notice warn', style: 'margin-bottom:10px', text: f + ' (in SkinSlotText\'s folder) also sets some of these names and is read first, so its names would show instead.' + (f === 'PilotNameTest.ini' ? ' Saving takes it out.' : ' Take it out, or take those lines out of it.') }));
  if (v.needLists) {
    status.append(el('div', { class: 'notice info' }, el('strong', { text: 'The game\'s list of people on the radio is not fetched yet. ' }),
      el('button', { class: 'btn small', type: 'button', onclick: fetchNameLists }, 'List the names')));
    $('#pnList').replaceChildren(); $('#pnOthers').replaceChildren(); $('#pnFlights').replaceChildren();
    $('#pnMoreBox').hidden = $('#pnFlightBox').hidden = true;
    namesButtons();
    return;
  }
  const last = new Date(v.lastGame);
  if (Object.keys(pn.saved).length && !isNaN(last) && last.getFullYear() > 2000)
    status.append(el('p', { class: 'hint', text: 'Your last game started ' + last.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) + '. "Seen" means the game showed that name; "Not seen" means it did not come up in that game (for example a pilot who did not fly).' }));
  const wing = v.speakers.filter(s => s.side === 'wingman');
  const others = v.speakers.filter(s => s.side !== 'wingman');
  $('#pnList').replaceChildren(...wing.map(speakerRow));
  $('#pnOthers').replaceChildren(...others.map(speakerRow));
  const changedOthers = others.filter(s => (pn.saved[pnField('name', s.id)] || '').trim()).length;
  $('#pnMoreHead').textContent = 'Everyone else on the radio (' + others.length + (changedOthers ? ', ' + changedOthers + ' changed' : '') + ')';
  $('#pnMoreBox').hidden = !others.length;
  if (changedOthers) $('#pnMoreBox').open = true;
  $('#pnFlights').replaceChildren(...v.flights.map(f => el('div', { class: 'namerow one' }, el('div', { class: 'face' }),
    el('div', { class: 'who' }, el('b', { text: f.label })), nameInput(pnField('flight', f.id), f.label, 'Flight name'))));
  $('#pnFlightBox').hidden = !v.flights.length;
  if (v.flights.some(f => (pn.saved[pnField('flight', f.id)] || '').trim())) $('#pnFlightBox').open = true;
  namesButtons();
}

function namesButtons() {
  const v = pn.view;
  if (!v) return;
  const dirty = namesDirty();
  const any = Object.values(pn.saved).some(x => (x || '').trim());
  $('#pnSave').disabled = !!activeJob || !!v.problem || v.needLists || !dirty;
  $('#pnClear').hidden = !any;
  $('#pnHint').textContent = v.problem ? '' : dirty ? 'Changes not saved yet.' : any ? 'Saved. They show the next time the game starts.' : '';
}

async function saveNames(names, okText) {
  const out = $('#pnResult'); out.replaceChildren();
  try {
    const r = await api('/api/pilots/names', { names });
    pn.view = r.view; pn.saved = { ...r.view.names }; pn.names = { ...r.view.names };
    renderNames();
    notice(out, 'good', el('strong', { text: okText + ' ' }), 'Start the game (or restart it) to see them. ' + (r.notes || []).join(' '));
  } catch (e) { notice(out, 'error', e.message); }
}

async function fetchNameLists() {
  const out = $('#pnResult'); out.replaceChildren();
  try { pn.view = await runJob('/api/pilots/names/fetch'); pn.saved = { ...pn.view.names }; pn.names = { ...pn.view.names }; renderNames(); }
  catch (e) { notice(out, 'error', e.message); }
}

$('#pnSave').addEventListener('click', () => saveNames(pn.names, 'Names saved.'));
$('#pnClear').addEventListener('click', () => {
  const row = $('#pnClear').parentElement;
  const keep = [...row.children];
  row.replaceChildren(el('span', { class: 'hint', text: 'Clear every new name and use the game\'s own again?' }),
    el('button', { class: 'btn small danger', type: 'button', onclick: async () => { row.replaceChildren(...keep); await saveNames({}, 'The game\'s own names are back.'); } }, 'Yes, clear them'),
    el('button', { class: 'btn small', type: 'button', onclick: () => row.replaceChildren(...keep) }, 'Cancel'));
});
