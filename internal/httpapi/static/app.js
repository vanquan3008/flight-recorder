const keyInput = document.querySelector('#api-key');
const status = document.querySelector('#status');
const results = document.querySelector('#results');
const filters = document.querySelector('#filters');
let view = 'traces';

keyInput.value = sessionStorage.getItem('flight-api-key') || '';
document.querySelector('#save-key').addEventListener('click', () => {
  sessionStorage.setItem('flight-api-key', keyInput.value);
  load();
});
document.querySelectorAll('nav button').forEach(button => button.addEventListener('click', () => { view = button.dataset.view; load(); }));

async function api(path) {
  const response = await fetch(path, { headers: { 'X-API-Key': sessionStorage.getItem('flight-api-key') || '' } });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error?.message || `request failed (${response.status})`);
  return body;
}

function escape(value) {
  return String(value ?? '').replace(/[&<>"']/g, character => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;' })[character]);
}

function table(columns, rows) {
  return `<table><thead><tr>${columns.map(column => `<th>${escape(column)}</th>`).join('')}</tr></thead><tbody>${rows.join('')}</tbody></table>`;
}

async function load() {
  status.textContent = 'Loading…'; results.innerHTML = ''; filters.innerHTML = '';
  try {
    if (view === 'traces') {
      const body = await api('/api/v1/traces');
      results.innerHTML = table(['Trace', 'Service', 'Started', 'Duration', 'Spans', 'Errors'], body.data.map(trace => `<tr><td><button data-trace="${escape(trace.trace_id)}"><code>${escape(trace.trace_id)}</code></button></td><td>${escape(trace.root_service)}</td><td>${escape(trace.started_at)}</td><td>${escape(trace.duration_nanos)}</td><td>${escape(trace.span_count)}</td><td>${escape(trace.error_count)}</td></tr>`));
      document.querySelectorAll('[data-trace]').forEach(button => button.addEventListener('click', () => showTrace(button.dataset.trace)));
    } else if (view === 'logs') {
      const body = await api('/api/v1/logs');
      results.innerHTML = table(['Time', 'Severity', 'Service', 'Body'], body.data.map(log => `<tr><td>${escape(log.occurred_at)}</td><td>${escape(log.severity_text)}</td><td>${escape(log.service_name)}</td><td><button data-log="${escape(log.event_id)}">View</button></td></tr>`));
      document.querySelectorAll('[data-log]').forEach(button => button.addEventListener('click', () => showLog(button.dataset.log)));
    } else if (view === 'services') {
      const body = await api('/api/v1/services');
      results.innerHTML = table(['Service', 'Traces', 'Requests', 'Errors', 'Error rate'], body.data.map(service => `<tr><td>${escape(service.name)}</td><td>${escape(service.trace_count)}</td><td>${escape(service.request_count)}</td><td>${escape(service.error_count)}</td><td>${escape(service.error_rate)}</td></tr>`));
    } else {
      const body = await api('/api/v1/incidents');
      results.innerHTML = table(['Title', 'Service', 'Severity', 'Occurrences', 'Last seen'], body.data.map(incident => `<tr><td>${escape(incident.title)}</td><td>${escape(incident.primary_service)}</td><td>${escape(incident.severity)}</td><td>${escape(incident.occurrence_count)}</td><td>${escape(incident.last_seen_at)}</td></tr>`));
    }
    status.textContent = '';
  } catch (error) { status.innerHTML = `<span class="error">${escape(error.message)}</span>`; }
}

async function showTrace(traceID) {
  try { const body = await api(`/api/v1/traces/${encodeURIComponent(traceID)}`); results.innerHTML = `<pre class="detail">${escape(JSON.stringify(body, null, 2))}</pre>`; } catch (error) { status.textContent = error.message; }
}
async function showLog(eventID) {
  try { const body = await api(`/api/v1/logs/${encodeURIComponent(eventID)}`); results.innerHTML = `<pre class="detail">${escape(JSON.stringify(body, null, 2))}</pre>`; } catch (error) { status.textContent = error.message; }
}

load();
