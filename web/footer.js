// Общий футер: версия приложения и кнопка «наверх».
import { fetchVersion } from './common.js';

// «Наверх»: прокрутка к началу страницы. Регистрация — до запроса версии:
// зависший fetch не должен оставлять кнопку мёртвой. При
// prefers-reduced-motion прокрутка мгновенная (вестибулярные нарушения).
document.getElementById('btn-top')?.addEventListener('click', () => {
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  window.scrollTo({ top: 0, behavior: reduced ? 'auto' : 'smooth' });
});

// Версия: элемент скрыт, пока ответа нет; при сбое остаётся скрытым.
// Промис /api/version шарится с app.js (fetchVersion кэширует).
const versionEl = document.getElementById('app-version');
if (versionEl) {
  const data = await fetchVersion();
  // 200 без поля version или с пустой — не показываем «undefined».
  if (data && typeof data.version === 'string' && data.version) {
    versionEl.textContent = data.version;
    versionEl.hidden = false;
  }
}
