import { api, ball, fmt } from './common.js';
import { parseCombination } from './parse.js';

const loginCard = document.getElementById('login-card');
const panel = document.getElementById('panel');
const loginError = document.getElementById('login-error');
const formError = document.getElementById('form-error');
const drawNoInput = document.getElementById('draw-no');
const comboInput = document.getElementById('combo');
const btnLogin = document.getElementById('btn-login');
const btnSave = document.getElementById('btn-save');
const btnCancel = document.getElementById('btn-cancel');
const list = document.getElementById('admin-draws');
const btnSync = document.getElementById('btn-sync');
const syncStatus = document.getElementById('sync-status');

let editNo = null; // null — добавление, число — редактирование существующего

init();

async function init() {
  try {
    const me = await api('/api/me');
    show(me.authenticated);
    if (me.authenticated) {
      refreshList();
    }
  } catch {
    show(false);
  }
}

function show(authed) {
  loginCard.hidden = authed;
  panel.hidden = !authed;
}

// Вход / выход
async function login() {
  loginError.hidden = true;
  btnLogin.disabled = true;
  try {
    await api('/api/login', {
      method: 'POST',
      body: { password: document.getElementById('password').value },
    });
    document.getElementById('password').value = '';
    show(true);
    refreshList();
  } catch (e) {
    loginError.textContent = e.message;
    loginError.hidden = false;
  } finally {
    btnLogin.disabled = false;
  }
}

document.getElementById('btn-login').addEventListener('click', login);
document.getElementById('password').addEventListener('keydown', (e) => {
  if (e.key === 'Enter') login();
});

document.getElementById('btn-logout').addEventListener('click', async () => {
  try {
    await api('/api/logout', { method: 'POST', body: {} });
  } catch (e) {
    alert(e.message);
  } finally {
    setEditMode(null);
    show(false);
  }
});

// Добавление / редактирование
btnSave.addEventListener('click', save);
btnCancel.addEventListener('click', () => setEditMode(null));

// Синхронизация с архивом timelottery.ru: добавляем только новые розыгрыши.
btnSync.addEventListener('click', sync);

async function sync() {
  syncStatus.hidden = true;
  btnSync.disabled = true;
  btnSync.textContent = 'Синхронизация…';
  try {
    const res = await api('/api/sync', { method: 'POST', body: {} });
    syncStatus.textContent = syncSummary(res);
    syncStatus.className = 'hint';
  } catch (e) {
    syncStatus.textContent = e.message;
    syncStatus.className = 'error';
  } finally {
    btnSync.disabled = false;
    btnSync.textContent = 'Синхронизировать';
    syncStatus.hidden = false;
    refreshList();
  }
}

// syncSummary — краткий итог синхронизации одной строкой.
function syncSummary(res) {
  const issues = res.issues ?? [];
  if (res.added === 0 && issues.length === 0) {
    return 'Новых розыгрышей нет';
  }
  const parts = [`Добавлено ${res.added}, пропущено ${res.skipped}`];
  if (issues.length > 0) {
    const names = issues.map((i) => (i.drawNo > 0 ? `№ ${i.drawNo}` : 'строка без номера'));
    parts.push(`не удалось разобрать: ${names.join(', ')}`);
  }
  return parts.join('; ');
}

async function save() {
  formError.hidden = true;
  const parsed = parseCombination(comboInput.value);
  if (parsed.error) {
    formError.textContent = parsed.error;
    formError.hidden = false;
    return;
  }
  btnSave.disabled = true;
  try {
    if (editNo === null) {
      await api('/api/draws', {
        method: 'POST',
        body: { drawNo: Number(drawNoInput.value), numbers: parsed.numbers, bonus: parsed.bonus },
      });
    } else {
      await api(`/api/draws/${editNo}`, {
        method: 'PUT',
        body: { numbers: parsed.numbers, bonus: parsed.bonus },
      });
    }
    setEditMode(null);
    refreshList();
  } catch (e) {
    formError.textContent = e.message;
    formError.hidden = false;
  } finally {
    btnSave.disabled = false;
  }
}

function setEditMode(no) {
  editNo = no;
  drawNoInput.disabled = no !== null; // номер розыгрыша при редактировании заблокирован
  btnSave.textContent = no === null ? 'Добавить' : 'Сохранить';
  btnCancel.hidden = no === null;
  if (no === null) {
    drawNoInput.value = '';
    comboInput.value = '';
    formError.hidden = true;
  }
}

// Список
async function refreshList() {
  try {
    const data = await api('/api/draws');
    list.replaceChildren();
    for (const d of data.draws) {
      list.append(drawRow(d));
    }
  } catch (e) {
    list.replaceChildren(e.message);
  }
}

function drawRow(d) {
  const row = document.createElement('div');
  row.className = 'draw-row';
  const no = document.createElement('span');
  no.className = 'draw-no';
  no.textContent = `№ ${d.drawNo}`;
  row.append(no);
  for (const n of d.numbers) {
    row.append(ball(n, 'small'));
  }
  row.append(ball(d.bonus, 'small bonus'));

  const actions = document.createElement('span');
  actions.className = 'row-actions';

  const edit = document.createElement('button');
  edit.className = 'btn-secondary';
  edit.textContent = 'Изменить';
  edit.addEventListener('click', () => {
    setEditMode(d.drawNo);
    drawNoInput.value = d.drawNo;
    comboInput.value = [...d.numbers, d.bonus].map(fmt).join(', ');
    comboInput.focus();
    window.scrollTo({ top: 0, behavior: 'smooth' });
  });

  const del = document.createElement('button');
  del.className = 'btn-danger';
  del.textContent = 'Удалить';
  del.addEventListener('click', async () => {
    if (!confirm(`Удалить розыгрыш № ${d.drawNo}?`)) {
      return;
    }
    try {
      await api(`/api/draws/${d.drawNo}`, { method: 'DELETE' });
      if (editNo === d.drawNo) {
        setEditMode(null);
      }
      refreshList();
    } catch (e) {
      alert(e.message);
    }
  });

  actions.append(edit, del);
  row.append(actions);
  return row;
}
