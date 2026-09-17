// Общий футер: версия приложения и кнопка «наверх».

// Версия: элемент скрыт, пока ответа нет; при сбое остаётся скрытым.
const versionEl = document.getElementById('app-version');
if (versionEl) {
  try {
    const res = await fetch('/api/version');
    if (res.ok) {
      const data = await res.json();
      versionEl.textContent = data.version;
      versionEl.hidden = false;
    }
  } catch {
    // нет соединения — версия просто не показывается
  }
}

// «Наверх»: плавная прокрутка к началу страницы.
document.getElementById('btn-top')?.addEventListener('click', () => {
  window.scrollTo({ top: 0, behavior: 'smooth' });
});
