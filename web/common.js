// Общие помощники фронтенда.

// api выполняет запрос к JSON API; 204 → null; не-2xx → Error с сообщением сервера.
export async function api(path, { method = 'GET', body } = {}) {
  let res;
  try {
    res = await fetch(path, {
      method,
      headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    // офлайн / DNS-сбой: fetch кидает TypeError с англоязычным текстом браузера
    throw new Error('Нет соединения с сервером');
  }
  if (res.status === 204) {
    return null;
  }
  let data = null;
  try {
    data = await res.json();
  } catch {
    // тело пустое или не JSON — оставляем null
  }
  if (!res.ok) {
    throw new Error((data && data.error) || `Ошибка ${res.status}`);
  }
  return data;
}

// fmt приводит число к виду «02» для шаров.
export function fmt(n) {
  return String(n).padStart(2, '0');
}

// ball создаёт элемент-шар.
export function ball(n, extra = '') {
  const el = document.createElement('span');
  el.className = `ball${extra ? ' ' + extra : ''}`;
  el.textContent = fmt(n);
  return el;
}
