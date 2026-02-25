package web

// indexHTML is the embedded single-file web UI served at "/".
// It uses vanilla JS + WebSocket to display flows in real time.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>http-proxy</title>
<style>
  :root {
    --bg: #1a1a2e;
    --bg2: #16213e;
    --bg3: #0f3460;
    --fg: #e0e0e0;
    --fg2: #a0a0b0;
    --green: #4caf50;
    --yellow: #ffc107;
    --red: #f44336;
    --cyan: #00bcd4;
    --blue: #2196f3;
    --selected: #1e3a5f;
    --border: #2a2a4a;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: 'Menlo','Monaco','Courier New',monospace; background: var(--bg); color: var(--fg); height: 100vh; display: flex; flex-direction: column; font-size: 13px; }
  #header { background: var(--bg3); padding: 8px 16px; display: flex; align-items: center; gap: 16px; border-bottom: 1px solid var(--border); }
  #header h1 { font-size: 15px; color: var(--cyan); }
  #header .stats { color: var(--fg2); font-size: 12px; }
  #header .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--red); }
  #header .dot.live { background: var(--green); animation: pulse 2s infinite; }
  @keyframes pulse { 0%,100%{opacity:1} 50%{opacity:.4} }
  #toolbar { background: var(--bg2); padding: 6px 16px; display: flex; gap: 8px; border-bottom: 1px solid var(--border); align-items: center; }
  #filter-input { background: var(--bg); border: 1px solid var(--border); color: var(--fg); padding: 4px 8px; font-family: inherit; font-size: 12px; width: 350px; border-radius: 3px; }
  #filter-input:focus { outline: none; border-color: var(--cyan); }
  .btn { background: var(--bg3); border: 1px solid var(--border); color: var(--fg2); padding: 4px 10px; cursor: pointer; font-family: inherit; font-size: 12px; border-radius: 3px; }
  .btn:hover { color: var(--fg); border-color: var(--cyan); }
  #main { display: flex; flex: 1; overflow: hidden; }
  #flow-list { width: 55%; border-right: 1px solid var(--border); display: flex; flex-direction: column; }
  #flow-table-wrap { overflow-y: auto; flex: 1; }
  table { width: 100%; border-collapse: collapse; }
  thead { position: sticky; top: 0; background: var(--bg2); z-index: 1; }
  th { padding: 6px 8px; text-align: left; color: var(--cyan); font-weight: bold; border-bottom: 1px solid var(--border); font-size: 11px; white-space: nowrap; }
  td { padding: 5px 8px; border-bottom: 1px solid var(--border); white-space: nowrap; overflow: hidden; max-width: 0; cursor: pointer; }
  tr:hover { background: var(--bg2); }
  tr.selected { background: var(--selected); }
  .method { font-weight: bold; color: var(--cyan); }
  .status-2xx { color: var(--green); font-weight: bold; }
  .status-3xx { color: var(--cyan); }
  .status-4xx { color: var(--yellow); font-weight: bold; }
  .status-5xx { color: var(--red); font-weight: bold; }
  .status-err { color: var(--red); font-style: italic; }
  .path-col { max-width: 200px; overflow: hidden; text-overflow: ellipsis; }
  .tag { background: var(--bg3); color: var(--cyan); padding: 1px 5px; border-radius: 2px; font-size: 10px; }
  #detail { width: 45%; display: flex; flex-direction: column; overflow: hidden; }
  #detail-header { padding: 8px 16px; background: var(--bg2); border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; }
  #detail-body { flex: 1; overflow-y: auto; display: flex; }
  .pane { flex: 1; padding: 12px; overflow: hidden; border-right: 1px solid var(--border); }
  .pane:last-child { border-right: none; }
  .pane h3 { color: var(--cyan); font-size: 11px; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 1px; }
  .section { margin-bottom: 12px; }
  .section-title { color: var(--fg2); font-size: 10px; text-transform: uppercase; letter-spacing: 1px; margin-bottom: 4px; }
  .headers-table { width: 100%; }
  .headers-table td { padding: 2px 4px; font-size: 11px; border: none; white-space: normal; word-break: break-all; }
  .headers-table td:first-child { color: var(--fg2); white-space: nowrap; width: 40%; }
  pre.body { background: var(--bg); padding: 8px; border-radius: 3px; font-size: 11px; white-space: pre-wrap; word-break: break-all; color: var(--fg); max-height: 400px; overflow-y: auto; }
  .empty { color: var(--fg2); font-style: italic; padding: 16px; text-align: center; }
  .replay-btn { background: var(--blue); border: none; color: white; padding: 3px 8px; cursor: pointer; border-radius: 3px; font-family: inherit; font-size: 11px; }
  .replay-btn:hover { background: #1976d2; }
  .curl-btn { background: var(--bg); border: 1px solid var(--border); color: var(--fg2); padding: 3px 8px; cursor: pointer; border-radius: 3px; font-family: inherit; font-size: 11px; }
  .curl-btn:hover { color: var(--fg); }
  #notice { position: fixed; bottom: 16px; right: 16px; background: var(--bg3); border: 1px solid var(--cyan); color: var(--fg); padding: 8px 16px; border-radius: 4px; font-size: 12px; display: none; z-index: 100; }
</style>
</head>
<body>
<div id="header">
  <div class="dot" id="ws-dot"></div>
  <h1>http-proxy</h1>
  <span class="stats" id="stats">0 flows</span>
</div>
<div id="toolbar">
  <input id="filter-input" type="text" placeholder='filter: ~m POST  ~s 5  ~p /api  ~u ctl-api' />
  <button class="btn" onclick="clearFlows()">Clear</button>
  <button class="btn" onclick="exportHAR()">Export HAR</button>
</div>
<div id="main">
  <div id="flow-list">
    <div id="flow-table-wrap">
      <table>
        <thead>
          <tr>
            <th>#</th>
            <th>Method</th>
            <th>Status</th>
            <th>Upstream</th>
            <th class="path-col">Path</th>
            <th>Time</th>
            <th>Size</th>
          </tr>
        </thead>
        <tbody id="flow-tbody"></tbody>
      </table>
      <div id="empty" class="empty">No traffic yet. Point your client at the proxy.</div>
    </div>
  </div>
  <div id="detail">
    <div id="detail-header">
      <span id="detail-title" style="color:var(--fg2)">Select a flow</span>
      <div>
        <button class="replay-btn" id="replay-btn" onclick="replaySelected()" style="display:none">⟳ Replay</button>
        <button class="curl-btn" id="curl-btn" onclick="copyCURL()" style="display:none">Copy cURL</button>
      </div>
    </div>
    <div id="detail-body">
      <div class="pane" id="req-pane"><div class="empty">Select a flow to inspect</div></div>
      <div class="pane" id="resp-pane"></div>
    </div>
  </div>
</div>
<div id="notice"></div>

<script>
const flows = new Map();  // id -> flow
let filteredIds = [];
let selectedId = null;
let filterExpr = '';

// --- WebSocket ---
let ws;
function connect() {
  ws = new WebSocket('ws://' + location.host + '/ws');
  ws.onopen = () => { document.getElementById('ws-dot').className = 'dot live'; };
  ws.onclose = () => {
    document.getElementById('ws-dot').className = 'dot';
    setTimeout(connect, 2000);
  };
  ws.onmessage = e => {
    const evt = JSON.parse(e.data);
    handleFlowEvent(evt);
  };
}

function handleFlowEvent(evt) {
  if (evt.type === 'new') {
    flows.set(evt.flow.id, evt.flow);
  } else if (evt.flow) {
    flows.set(evt.flow.id, evt.flow);
    if (selectedId === evt.flow.id) renderDetail(evt.flow);
  }
  applyFilter();
  updateStats();
}

// --- Filter ---
document.getElementById('filter-input').addEventListener('input', function() {
  filterExpr = this.value.trim().toLowerCase();
  applyFilter();
});

function applyFilter() {
  filteredIds = [];
  for (const [id, f] of flows) {
    if (matchFilter(f)) filteredIds.push(id);
  }
  renderTable();
}

function matchFilter(f) {
  if (!filterExpr) return true;
  const tokens = filterExpr.split(/\s+/);
  // Simple client-side filter: just substring match on method/path/status/upstream
  for (const tok of tokens) {
    if (!tok) continue;
    const method = (f.request?.method || '').toLowerCase();
    const path = (f.request?.path || '').toLowerCase();
    const status = String(f.response?.statusCode || '');
    const upstream = (f.upstream || '').toLowerCase();
    const body = (typeof f.request?.body === 'string' ? f.request.body : '').toLowerCase();
    const resp_body = (typeof f.response?.body === 'string' ? f.response.body : '').toLowerCase();
    if (tok.startsWith('~m ') || tok.startsWith('~m')) {
      const v = tok.slice(2).trim();
      if (!method.includes(v)) return false;
    } else if (tok.startsWith('~s')) {
      const v = tok.slice(2).trim();
      if (!status.startsWith(v)) return false;
    } else if (tok.startsWith('~p')) {
      const v = tok.slice(2).trim();
      if (!path.includes(v)) return false;
    } else if (tok.startsWith('~u')) {
      const v = tok.slice(2).trim();
      if (!upstream.includes(v)) return false;
    } else if (tok.startsWith('~b')) {
      const v = tok.slice(2).trim();
      if (!body.includes(v) && !resp_body.includes(v)) return false;
    }
  }
  return true;
}

// --- Table rendering ---
function renderTable() {
  const tbody = document.getElementById('flow-tbody');
  const empty = document.getElementById('empty');
  if (filteredIds.length === 0) {
    tbody.innerHTML = '';
    empty.style.display = 'block';
    return;
  }
  empty.style.display = 'none';
  // Render newest-first for easy inspection.
  const ids = [...filteredIds].reverse();
  tbody.innerHTML = ids.map((id, i) => {
    const f = flows.get(id);
    const n = filteredIds.length - i;
    const method = f.request?.method || '-';
    const path = f.request?.path || '/';
    const upstream = f.upstream || '-';
    let statusHtml = '<span class="status-err">ERR</span>';
    if (f.response) {
      const sc = f.response.statusCode;
      const cls = sc >= 500 ? 'status-5xx' : sc >= 400 ? 'status-4xx' : sc >= 300 ? 'status-3xx' : 'status-2xx';
      statusHtml = '<span class="'+cls+'">'+sc+'</span>';
    }
    const dur = fmtDur(durationMs(f));
    const size = f.response ? fmtSize(bodyLen(f.response.body)) : '-';
    const tags = (f.tags || []).map(t => '<span class="tag">'+escHtml(t)+'</span>').join(' ');
    const sel = id === selectedId ? ' selected' : '';
    return '<tr class="flow-row'+sel+'" data-id="'+id+'" onclick="selectFlow(\''+id+'\')">'+
      '<td>'+n+'</td>'+
      '<td class="method">'+escHtml(method)+'</td>'+
      '<td>'+statusHtml+'</td>'+
      '<td>'+escHtml(upstream)+'</td>'+
      '<td class="path-col" title="'+escHtml(path)+'">'+escHtml(path)+'</td>'+
      '<td>'+dur+'</td>'+
      '<td>'+size+' '+tags+'</td>'+
      '</tr>';
  }).join('');
}

function updateStats() {
  document.getElementById('stats').textContent = flows.size + ' flows';
}

// --- Detail ---
function selectFlow(id) {
  selectedId = id;
  renderTable(); // refresh selection highlight
  const f = flows.get(id);
  if (!f) return;
  renderDetail(f);
  document.getElementById('replay-btn').style.display = '';
  document.getElementById('curl-btn').style.display = '';
}

function renderDetail(f) {
  const sc = f.response?.statusCode;
  let statusHtml = '';
  if (sc) {
    const cls = sc>=500?'status-5xx':sc>=400?'status-4xx':sc>=300?'status-3xx':'status-2xx';
    statusHtml = ' → <span class="'+cls+'">'+sc+'</span>';
  }
  document.getElementById('detail-title').innerHTML =
    '<strong>'+escHtml(f.request?.method||'-')+'</strong> '+escHtml(f.request?.path||'/')+statusHtml+
    ' <span style="color:var(--fg2);font-size:11px">['+fmtDur(durationMs(f))+']</span>';

  document.getElementById('req-pane').innerHTML = renderRequestPane(f);
  document.getElementById('resp-pane').innerHTML = renderResponsePane(f);
}

function renderRequestPane(f) {
  if (!f.request) return '<div class="empty">No request data</div>';
  const r = f.request;
  let h = '<h3>Request</h3>';
  h += '<div class="section"><div class="section-title">'+escHtml(r.method)+' '+escHtml(r.url)+'</div></div>';
  h += renderHeaders(r.headers);
  if (r.body) {
    h += '<div class="section"><div class="section-title">Body</div>';
    h += '<pre class="body">'+prettyBody(r.headers?.['Content-Type']?.[0]||'', atob_safe(r.body))+'</pre>';
    if (r.bodyTruncated) h += '<span style="color:var(--red);font-size:11px">… body truncated</span>';
    h += '</div>';
  }
  return h;
}

function renderResponsePane(f) {
  if (!f.response) {
    if (f.error) return '<h3>Response</h3><div style="color:var(--red)">'+escHtml(f.error)+'</div>';
    return '<h3>Response</h3><div class="empty">Pending…</div>';
  }
  const r = f.response;
  const cls = r.statusCode>=500?'status-5xx':r.statusCode>=400?'status-4xx':r.statusCode>=300?'status-3xx':'status-2xx';
  let h = '<h3>Response</h3>';
  h += '<div class="section"><div class="section-title"><span class="'+cls+'">'+r.statusCode+'</span></div></div>';
  h += renderHeaders(r.headers);
  if (r.body) {
    h += '<div class="section"><div class="section-title">Body</div>';
    h += '<pre class="body">'+prettyBody(r.headers?.['Content-Type']?.[0]||'', atob_safe(r.body))+'</pre>';
    if (r.bodyTruncated) h += '<span style="color:var(--red);font-size:11px">… body truncated</span>';
    h += '</div>';
  }
  return h;
}

function renderHeaders(hdrs) {
  if (!hdrs || Object.keys(hdrs).length === 0) return '';
  let h = '<div class="section"><div class="section-title">Headers</div><table class="headers-table">';
  for (const [k, vv] of Object.entries(hdrs)) {
    for (const v of vv) {
      h += '<tr><td>'+escHtml(k)+'</td><td>'+escHtml(v)+'</td></tr>';
    }
  }
  h += '</table></div>';
  return h;
}

function prettyBody(ct, body) {
  if (!body) return '';
  if ((ct||'').includes('json')) {
    try { return JSON.stringify(JSON.parse(body), null, 2); } catch(e) {}
  }
  return body.slice(0, 10000);
}

function atob_safe(b64) {
  if (!b64) return '';
  try { return atob(b64); } catch(e) { return b64; }
}

// --- Actions ---
async function replaySelected() {
  if (!selectedId) return;
  const r = await fetch('/api/flows/'+selectedId+'/replay', {method:'POST'});
  if (r.ok) {
    notify('Replaying…');
  } else {
    notify('Replay failed: ' + await r.text());
  }
}

function copyCURL() {
  if (!selectedId) return;
  const f = flows.get(selectedId);
  if (!f) return;
  const curl = toCURL(f);
  navigator.clipboard?.writeText(curl);
  notify('Copied cURL command');
}

async function clearFlows() {
  await fetch('/api/flows', {method:'DELETE'});
  flows.clear();
  filteredIds = [];
  selectedId = null;
  renderTable();
  updateStats();
  document.getElementById('req-pane').innerHTML = '<div class="empty">Select a flow to inspect</div>';
  document.getElementById('resp-pane').innerHTML = '';
  document.getElementById('detail-title').textContent = 'Select a flow';
  document.getElementById('replay-btn').style.display = 'none';
  document.getElementById('curl-btn').style.display = 'none';
}

async function exportHAR() {
  const all = await fetch('/api/flows').then(r => r.json());
  const har = { log: { version: '1.2', creator: { name: 'http-proxy' }, entries: all.map(flowToHAR) } };
  const blob = new Blob([JSON.stringify(har, null, 2)], {type: 'application/json'});
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = 'http-proxy-' + new Date().toISOString().slice(0,19) + '.har';
  a.click();
}

// --- Helpers ---
function toCURL(f) {
  if (!f.request) return '';
  let cmd = 'curl -X ' + f.request.method + " '" + f.request.url + "'";
  for (const [k, vv] of Object.entries(f.request.headers||{})) {
    const lk = k.toLowerCase();
    if (lk === 'connection' || lk === 'transfer-encoding') continue;
    for (const v of vv) cmd += " \\\n  -H '" + k + ': ' + v + "'";
  }
  if (f.request.body) cmd += " \\\n  -d '" + atob_safe(f.request.body).replace(/'/g, "'\\''") + "'";
  return cmd;
}

function flowToHAR(f) {
  return {
    startedDateTime: f.timestamps?.created || new Date().toISOString(),
    time: durationMs(f),
    request: {
      method: f.request?.method || '',
      url: f.request?.url || '',
      httpVersion: f.request?.proto || 'HTTP/1.1',
      headers: headersToHAR(f.request?.headers),
      queryString: [],
      postData: f.request?.body ? { mimeType: f.request?.headers?.['Content-Type']?.[0]||'', text: atob_safe(f.request.body) } : undefined,
      headersSize: -1,
      bodySize: bodyLen(f.request?.body),
    },
    response: {
      status: f.response?.statusCode || 0,
      statusText: '',
      httpVersion: f.response?.proto || 'HTTP/1.1',
      headers: headersToHAR(f.response?.headers),
      content: { size: bodyLen(f.response?.body), mimeType: f.response?.headers?.['Content-Type']?.[0]||'', text: atob_safe(f.response?.body) },
      redirectURL: '',
      headersSize: -1,
      bodySize: bodyLen(f.response?.body),
    },
    timings: { send: 0, wait: durationMs(f), receive: 0 },
  };
}

function headersToHAR(hdrs) {
  const out = [];
  for (const [k, vv] of Object.entries(hdrs||{})) {
    for (const v of vv) out.push({name: k, value: v});
  }
  return out;
}

function durationMs(f) {
  if (!f.timestamps) return 0;
  const start = new Date(f.timestamps.created).getTime();
  const end = f.timestamps.responseDone ? new Date(f.timestamps.responseDone).getTime() : Date.now();
  return end - start;
}

function bodyLen(b) {
  if (!b) return 0;
  try { return atob(b).length; } catch(e) { return b.length; }
}

function fmtDur(ms) {
  if (ms < 1) return '<1ms';
  if (ms < 1000) return ms + 'ms';
  return (ms/1000).toFixed(1) + 's';
}

function fmtSize(n) {
  if (n === 0) return '0';
  if (n < 1024) return n + 'B';
  if (n < 1024*1024) return (n/1024).toFixed(1) + 'K';
  return (n/1024/1024).toFixed(1) + 'M';
}

function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function notify(msg) {
  const el = document.getElementById('notice');
  el.textContent = msg;
  el.style.display = 'block';
  clearTimeout(el._timer);
  el._timer = setTimeout(() => { el.style.display = 'none'; }, 3000);
}

// Load existing flows on startup.
fetch('/api/flows').then(r => r.json()).then(all => {
  if (!all) return;
  for (const f of all) flows.set(f.id, f);
  applyFilter();
  updateStats();
});

connect();
</script>
</body>
</html>
`
