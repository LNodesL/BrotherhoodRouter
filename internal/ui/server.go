package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"

	"bhrouter/internal/hosts"
)

type Server struct {
	Manager *hosts.Manager
}

type apiResponse struct {
	OK       bool            `json:"ok"`
	Error    string          `json:"error,omitempty"`
	Snapshot *hosts.Snapshot `json:"snapshot,omitempty"`
}

type setRequest struct {
	Host string `json:"host"`
	IP   string `json:"ip"`
}

type removeRequest struct {
	Host string `json:"host"`
}

func NewServer(manager *hosts.Manager) *Server {
	return &Server{Manager: manager}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/list", s.handleList)
	mux.HandleFunc("/api/set", s.handleSet)
	mux.HandleFunc("/api/remove", s.handleRemove)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = indexTemplate.Execute(w, map[string]string{"Title": "BHRouter v0.0.1"})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	snapshot, err := s.Manager.List()
	if err != nil {
		s.writeError(w, statusFromError(err), err)
		return
	}
	s.writeJSON(w, http.StatusOK, apiResponse{OK: true, Snapshot: snapshot})
}

func (s *Server) handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var req setRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if err := s.Manager.Set(req.Host, req.IP); err != nil {
		s.writeError(w, statusFromError(err), err)
		return
	}
	snapshot, err := s.Manager.List()
	if err != nil {
		s.writeError(w, statusFromError(err), err)
		return
	}
	s.writeJSON(w, http.StatusOK, apiResponse{OK: true, Snapshot: snapshot})
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var req removeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if _, err := s.Manager.Remove(req.Host); err != nil {
		s.writeError(w, statusFromError(err), err)
		return
	}
	snapshot, err := s.Manager.List()
	if err != nil {
		s.writeError(w, statusFromError(err), err)
		return
	}
	s.writeJSON(w, http.StatusOK, apiResponse{OK: true, Snapshot: snapshot})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload apiResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	s.writeJSON(w, status, apiResponse{OK: false, Error: err.Error()})
}

func statusFromError(err error) int {
	if os.IsPermission(err) {
		return http.StatusForbidden
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "permission denied") || strings.Contains(msg, "access is denied") || strings.Contains(msg, "insufficient privileges") {
		return http.StatusForbidden
	}
	if strings.Contains(msg, "invalid") || strings.Contains(msg, "cannot") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>{{.Title}}</title>
  <style>
    :root {
      --bg: #f4f6f8;
      --card: #ffffff;
      --line: #d7dce1;
      --text: #182026;
      --muted: #5f6b76;
      --accent: #0b6c80;
      --accent-hover: #095665;
      --danger: #c93a2f;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Avenir Next", "Segoe UI", sans-serif;
      color: var(--text);
      background: radial-gradient(circle at 20% 0%, #fefbf4 0%, var(--bg) 55%);
    }
    .wrap {
      max-width: 920px;
      margin: 28px auto;
      padding: 0 16px;
    }
    .card {
      background: var(--card);
      border: 1px solid var(--line);
      border-radius: 12px;
      box-shadow: 0 10px 30px rgba(20, 31, 42, 0.06);
      padding: 18px;
      margin-bottom: 14px;
    }
    h1 {
      margin: 2px 0 4px;
      font-size: 1.6rem;
      letter-spacing: 0.02em;
    }
    .muted { color: var(--muted); }
    .row {
      display: grid;
      grid-template-columns: 1fr 1fr auto;
      gap: 10px;
      margin-top: 12px;
    }
    input {
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 10px;
      font-size: 14px;
    }
    button {
      border: 0;
      border-radius: 8px;
      padding: 10px 14px;
      cursor: pointer;
      font-weight: 600;
      transition: transform .12s ease, background .12s ease;
    }
    button:hover { transform: translateY(-1px); }
    .btn-primary { background: var(--accent); color: #fff; }
    .btn-primary:hover { background: var(--accent-hover); }
    .btn-danger { background: var(--danger); color: #fff; }
    table { width: 100%; border-collapse: collapse; }
    th, td {
      border-bottom: 1px solid var(--line);
      text-align: left;
      padding: 10px 6px;
      font-size: 14px;
    }
    .flash {
      padding: 10px;
      border-radius: 8px;
      display: none;
      margin-top: 10px;
      font-size: 14px;
    }
    .ok { display: block; background: #e9f8ec; color: #17512b; }
    .err { display: block; background: #ffecec; color: #772222; }
    @media (max-width: 720px) {
      .row { grid-template-columns: 1fr; }
      th:nth-child(3), td:nth-child(3) { width: 88px; }
    }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1>BHRouter</h1>
      <div class="muted">Brotherhood Router v0.0.1</div>
      <div class="muted" id="hostsPath"></div>
      <div class="row">
        <input id="host" placeholder="example.com" />
        <input id="ip" placeholder="127.0.0.1" />
        <button class="btn-primary" id="saveBtn">Set Override</button>
      </div>
      <div id="flash" class="flash"></div>
    </div>

    <div class="card">
      <h2>Managed Overrides</h2>
      <table>
        <thead>
          <tr><th>Host</th><th>IP</th><th>Action</th></tr>
        </thead>
        <tbody id="rows"></tbody>
      </table>
    </div>

    <div class="card" id="conflictCard" style="display:none;">
      <h2>Potential Conflicts</h2>
      <div class="muted">An existing unmanaged mapping may shadow this override.</div>
      <table>
        <thead>
          <tr><th>Host</th><th>Existing IP</th></tr>
        </thead>
        <tbody id="conflicts"></tbody>
      </table>
    </div>
  </div>

<script>
async function api(url, opts = {}) {
  const res = await fetch(url, {
    headers: {'Content-Type': 'application/json'},
    ...opts
  });
  const data = await res.json();
  if (!res.ok || !data.ok) {
    throw new Error(data.error || 'request failed');
  }
  return data;
}

function flash(msg, isErr = false) {
  const el = document.getElementById('flash');
  el.textContent = msg;
  el.className = 'flash ' + (isErr ? 'err' : 'ok');
}

function render(snapshot) {
  document.getElementById('hostsPath').textContent = 'Hosts file: ' + snapshot.path;
  const body = document.getElementById('rows');
  body.innerHTML = '';

  if (!snapshot.managed.length) {
    body.innerHTML = '<tr><td colspan="3" class="muted">No managed overrides yet.</td></tr>';
  }

  for (const e of snapshot.managed) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td>' + e.host + '</td><td>' + e.ip + '</td><td><button class=\"btn-danger\" data-host=\"' + e.host + '\">Remove</button></td>';
    body.appendChild(tr);
  }

  body.querySelectorAll('button[data-host]').forEach(btn => {
    btn.addEventListener('click', async () => {
      try {
        const data = await api('/api/remove', {method: 'POST', body: JSON.stringify({host: btn.dataset.host})});
        render(data.snapshot);
        flash('Removed ' + btn.dataset.host);
      } catch (err) {
        flash(err.message, true);
      }
    });
  });

  const conflictCard = document.getElementById('conflictCard');
  const conflictsBody = document.getElementById('conflicts');
  conflictsBody.innerHTML = '';
  if (snapshot.conflicts.length) {
    conflictCard.style.display = 'block';
    for (const c of snapshot.conflicts) {
      const tr = document.createElement('tr');
      tr.innerHTML = '<td>' + c.host + '</td><td>' + c.ip + '</td>';
      conflictsBody.appendChild(tr);
    }
  } else {
    conflictCard.style.display = 'none';
  }
}

async function refresh() {
  try {
    const data = await api('/api/list');
    render(data.snapshot);
  } catch (err) {
    flash(err.message, true);
  }
}

document.getElementById('saveBtn').addEventListener('click', async () => {
  const host = document.getElementById('host').value.trim();
  const ip = document.getElementById('ip').value.trim();
  try {
    const data = await api('/api/set', {method: 'POST', body: JSON.stringify({host, ip})});
    render(data.snapshot);
    flash('Saved ' + host + ' -> ' + ip);
  } catch (err) {
    flash(err.message, true);
  }
});

refresh();
</script>
</body>
</html>`))
