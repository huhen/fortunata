// Общий футер: версия приложения и кнопка «наверх».

// «Наверх»: прокрутка к началу страницы. Регистрация — до запроса версии:
// зависший fetch не должен оставлять кнопку мёртвой. При
// prefers-reduced-motion прокрутка мгновенная (вестибулярные нарушения).
document.getElementById('btn-top')?.addEventListener('click', () => {
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  window.scrollTo({ top: 0, behavior: reduced ? 'auto' : 'smooth' });
});

// Версия: элемент скрыт, пока ответа нет; при сбое остаётся скрытым.
const versionEl = document.getElementById('app-version');
if (versionEl) {
  try {
    const res = await fetch('/api/version');
    if (res.ok) {
      const data = await res.json();
      // 200 без поля version или с пустой — не показываем «undefined».
      if (typeof data.version === 'string' && data.version) {
        versionEl.textContent = data.version;
        versionEl.hidden = false;
      }
    }
  } catch {
    // нет соединения — версия просто не показывается
  }
}
