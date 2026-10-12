const send = (message) => chrome.runtime.sendMessage(message);
const status = document.querySelector('#status');
async function refresh() {
  const { candidates = [] } = await send({ type: 'diagnostic-list' });
  const list = document.querySelector('#candidates');
  list.replaceChildren();
  for (const c of candidates) {
    const button = document.createElement('button');
    button.textContent = `${c.host} · tab ${c.tabId} · frame ${c.frameId}: наблюдать один fragment`;
    button.onclick = async () => {
      const r = await send({ type: 'diagnostic-arm', id: c.id });
      status.textContent = r.ok
        ? 'Ожидание 30 секунд. Вернитесь к воспроизводящемуся видео'
        : 'Не удалось начать диагностику';
      document.querySelector('#report').textContent = '';
    };
    list.append(button);
  }
}
document.querySelector('#refresh').onclick = refresh;
document.querySelector('#stop').onclick = async () => {
  await send({ type: 'diagnostic-stop' });
  document.querySelector('#report').textContent = '';
  status.textContent = 'Остановлено, transient context освобождён';
};
const timer = setInterval(async () => {
  const { report } = await send({ type: 'diagnostic-report' });
  if (report)
    document.querySelector('#report').textContent = JSON.stringify(
      report,
      null,
      2,
    );
}, 1000);
window.addEventListener('pagehide', () => {
  clearInterval(timer);
  send({ type: 'diagnostic-stop' });
});
refresh();
