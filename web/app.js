import { api, ball } from './common.js';

// Вкладки
const tabs = document.querySelectorAll('.tab');
tabs.forEach((tab) => {
  tab.addEventListener('click', () => {
    tabs.forEach((t) => {
      const active = t === tab;
      t.classList.toggle('active', active);
      t.setAttribute('aria-selected', String(active));
    });
    document.querySelectorAll('.panel').forEach((p) => {
      p.classList.toggle('active', p.id === `tab-${tab.dataset.tab}`);
    });
    if (tab.dataset.tab === 'archive') {
      loadDraws();
    }
  });
});

// Генерация
const genError = document.getElementById('gen-error');
const ticketsBox = document.getElementById('tickets');

document.getElementById('btn-generate').addEventListener('click', async () => {
  genError.hidden = true;
  const count = Number(document.getElementById('count').value);
  try {
    const data = await api('/api/generate', { method: 'POST', body: { count } });
    ticketsBox.replaceChildren();
    for (const t of data.tickets) {
      const row = document.createElement('div');
      row.className = 'ticket';
      for (const n of t.numbers) {
        row.append(ball(n));
      }
      row.append(ball(t.bonus, 'bonus'));
      ticketsBox.append(row);
    }
  } catch (e) {
    genError.textContent = e.message;
    genError.hidden = false;
  }
});

// Архив
async function loadDraws() {
  const box = document.getElementById('draws-list');
  try {
    const data = await api('/api/draws');
    box.replaceChildren();
    if (data.draws.length === 0) {
      box.textContent = 'Розыгрышей пока нет — добавьте их через редактирование.';
      return;
    }
    for (const d of data.draws) {
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
      box.append(row);
    }
  } catch (e) {
    box.textContent = e.message;
  }
}
