// The overview refreshes its list in place; the log page streams a
// container's log over server-sent events.
(() => {
  const localize = root => {
    for (const t of root.querySelectorAll('time[data-local]')) {
      const d = new Date(t.dateTime);
      if (!Number.isNaN(d.getTime())) t.textContent = d.toLocaleTimeString([], { hour12: false });
    }
  };

  const overview = document.getElementById('overview');
  if (overview) {
    localize(overview);
    const refresh = async () => {
      try {
        const res = await fetch(overview.dataset.refresh, { cache: 'no-store' });
        if (!res.ok) return;
        overview.innerHTML = await res.text();
        localize(overview);
      } catch {
        // Offline for a moment; the next tick tries again.
      }
    };
    setInterval(() => { if (!document.hidden) refresh(); }, 10000);
    document.addEventListener('visibilitychange', () => { if (!document.hidden) refresh(); });
  }

  const log = document.getElementById('log');
  if (!log) return;

  // Beyond this many lines the oldest are dropped, to keep the page quick.
  const MAX_LINES = 20000;
  const tail = document.getElementById('tail');
  const filter = document.getElementById('filter');
  const follow = document.getElementById('follow');
  const wrap = document.getElementById('wrap');
  const status = document.getElementById('status');
  const reconnect = document.getElementById('reconnect');
  let source = null;
  let pending = [];
  let queued = false;
  let needle = '';

  const setStatus = (text, canReload = false) => {
    status.textContent = text;
    reconnect.hidden = !canReload;
  };

  const pad = n => String(n).padStart(2, '0');
  const stamp = iso => {
    const d = new Date(iso);
    if (!iso || Number.isNaN(d.getTime())) return '';
    return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  };

  const matches = text => !needle || text.toLowerCase().includes(needle);
  const toBottom = () => { log.scrollTop = log.scrollHeight; };

  // Lines are added once per frame rather than one by one.
  const flush = () => {
    queued = false;
    const lines = pending.length > MAX_LINES ? pending.slice(-MAX_LINES) : pending;
    pending = [];
    const frag = document.createDocumentFragment();
    for (const l of lines) {
      const row = document.createElement('div');
      row.className = l.s === 'stderr' ? 'line err' : 'line';
      const time = document.createElement('time');
      if (l.t) {
        time.dateTime = l.t;
        time.title = l.t;
        time.textContent = stamp(l.t);
      }
      const text = document.createElement('span');
      text.textContent = l.m;
      row.append(time, text);
      row.hidden = !matches(l.m);
      frag.append(row);
    }
    log.append(frag);
    for (let excess = log.childElementCount - MAX_LINES; excess > 0; excess--) log.firstElementChild.remove();
    if (follow.checked) toBottom();
  };

  const queue = line => {
    pending.push(line);
    if (pending.length > MAX_LINES * 2) pending = pending.slice(-MAX_LINES);
    if (!queued) {
      queued = true;
      requestAnimationFrame(flush);
    }
  };

  const connect = () => {
    if (source) source.close();
    log.replaceChildren();
    pending = [];
    setStatus('Connecting…');
    const es = new EventSource(`${log.dataset.stream}?tail=${encodeURIComponent(tail.value)}`);
    source = es;
    es.addEventListener('open', () => setStatus('Following'));
    es.addEventListener('line', e => queue(JSON.parse(e.data)));
    es.addEventListener('end', e => {
      es.close();
      setStatus(JSON.parse(e.data) || 'The log ended.', true);
    });
    // EventSource would reconnect by itself and replay the tail, duplicating
    // lines; stop instead and offer a reload.
    es.addEventListener('error', () => {
      if (es.readyState === EventSource.CLOSED) return;
      es.close();
      setStatus('Disconnected.', true);
    });
  };

  tail.addEventListener('change', connect);
  reconnect.addEventListener('click', connect);
  wrap.addEventListener('change', () => log.classList.toggle('wrap', wrap.checked));
  follow.addEventListener('change', () => { if (follow.checked) toBottom(); });
  log.addEventListener('scroll', () => {
    follow.checked = log.scrollHeight - log.scrollTop - log.clientHeight < 24;
  });

  let filterTimer;
  filter.addEventListener('input', () => {
    clearTimeout(filterTimer);
    filterTimer = setTimeout(() => {
      needle = filter.value.trim().toLowerCase();
      for (const row of log.children) row.hidden = !matches(row.lastElementChild.textContent);
      if (follow.checked) toBottom();
    }, 120);
  });

  connect();
})();
