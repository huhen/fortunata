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
    if (tab.dataset.tab === 'stats') {
      loadStats();
    }
  });
});

// Генерация
const genError = document.getElementById('gen-error');
const ticketsBox = document.getElementById('tickets');
const countInput = document.getElementById('count');
const btnAI = document.getElementById('btn-generate-ai');
const aiLabel = btnAI.textContent;

// renderTickets рисует пачку билетов в #tickets (общий для обеих кнопок).
function renderTickets(tickets) {
  ticketsBox.replaceChildren();
  for (const t of tickets) {
    const row = document.createElement('div');
    row.className = 'ticket';
    for (const n of t.numbers) {
      row.append(ball(n));
    }
    row.append(ball(t.bonus, 'bonus'));
    ticketsBox.append(row);
  }
}

function showGenError(e) {
  genError.textContent = e.message;
  genError.hidden = false;
}

document.getElementById('btn-generate').addEventListener('click', async () => {
  genError.hidden = true;
  try {
    const data = await api('/api/generate', { method: 'POST', body: { count: Number(countInput.value) } });
    renderTickets(data.tickets);
  } catch (e) {
    showGenError(e);
  }
});

btnAI.addEventListener('click', async () => {
  genError.hidden = true;
  btnAI.disabled = true;
  btnAI.textContent = 'AI думает…';
  countInput.disabled = true;
  try {
    const data = await api('/api/generate/ai', { method: 'POST', body: { count: Number(countInput.value) } });
    renderTickets(data.tickets);
  } catch (e) {
    showGenError(e);
  } finally {
    btnAI.disabled = false;
    btnAI.textContent = aiLabel;
    countInput.disabled = false;
  }
});

// AI-кнопка видна, только если сервер настроил LLM. Запрос после
// навешивания обработчиков: top-level await не задержит их.
try {
  const v = await api('/api/version');
  if (v && v.ai) {
    btnAI.hidden = false;
  }
} catch {
  // /api/version недоступен — кнопка остаётся скрытой
}

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

// Статистика
async function loadStats() {
  const box = document.getElementById('stats-list');
  try {
    const data = await api('/api/stats');
    box.replaceChildren();
    if (data.main.length === 0) {
      box.textContent = 'Розыгрышей пока нет — добавьте их через редактирование.';
      return;
    }
    box.append(statsCard('Основные шары', data.main, ''));
    box.append(statsCard('Бонусные шары', data.bonus, 'bonus'));
  } catch (e) {
    box.textContent = e.message;
  }
}

// statsCard — карточка с заголовком и списком «шар + количество выпадений».
function statsCard(title, freqs, extra) {
  const card = document.createElement('div');
  card.className = 'card';
  const h2 = document.createElement('h2');
  h2.className = 'stat-title';
  h2.textContent = title;
  card.append(h2);
  const list = document.createElement('div');
  list.className = 'stat-list';
  for (const f of freqs) {
    const item = document.createElement('span');
    item.className = 'stat-item';
    item.append(ball(f.n, extra));
    const count = document.createElement('span');
    count.className = 'ball-count';
    count.textContent = f.count;
    item.append(count);
    list.append(item);
  }
  card.append(list);
  return card;
}
