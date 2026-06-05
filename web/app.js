var autoscroll = true;
var hasScrolled = false;
var reconnecting = false;
var pendingEnv = [];
var currentRevealed = {};

document.addEventListener('DOMContentLoaded', function() {
  connectSSE();
  loadBuildCmd();
  pollStatus();
  setInterval(pollStatus, 2000);
  fetchEnv();

  document.getElementById('autoscroll').addEventListener('change', function() {
    autoscroll = this.checked;
  });

  document.getElementById('logfilter').addEventListener('input', function() {
    var q = this.value.toLowerCase();
    var lines = document.querySelectorAll('#loglines .line');
    for (var i = 0; i < lines.length; i++) {
      var line = lines[i];
      if (q === '' || line.textContent.toLowerCase().indexOf(q) !== -1) {
        line.style.display = '';
      } else {
        line.style.display = 'none';
      }
    }
  });

  document.getElementById('loglines').addEventListener('scroll', function() {
    var el = this;
    var atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 30;
    if (!atBottom) hasScrolled = true;
    if (atBottom) hasScrolled = false;
  });
});

function connectSSE() {
  reconnecting = false;
  var sse = new EventSource('/api/events');

  sse.addEventListener('log', function(e) {
    var data = JSON.parse(e.data);
    addLogLine(data.time, data.line, false);
  });

  sse.addEventListener('build-start', function(e) {
    var data = JSON.parse(e.data);
    addLogLine('', '>> BUILD: ' + data.cmd, true);
  });

  sse.addEventListener('build-end', function(e) {
    var data = JSON.parse(e.data);
    var msg = data.success ? '>> BUILD SUCCEEDED' : '>> BUILD FAILED: ' + (data.output || '');
    addLogLine('', msg, true);
  });

  sse.onerror = function() {
    if (reconnecting) return;
    reconnecting = true;
    sse.close();
    setTimeout(connectSSE, 2000);
  };
}

function addLogLine(time, text, isBuild) {
  var el = document.getElementById('loglines');
  var div = document.createElement('div');
  div.className = 'line' + (isBuild ? ' build' : '');
  if (time) text = time + '  ' + text;

  var filter = document.getElementById('logfilter').value.toLowerCase();
  if (filter !== '' && text.toLowerCase().indexOf(filter) === -1) {
    div.style.display = 'none';
  }

  div.textContent = text;
  el.appendChild(div);

  while (el.children.length > 5000) {
    el.removeChild(el.firstChild);
  }

  if (autoscroll && !hasScrolled) {
    el.scrollTop = el.scrollHeight;
  }
}

function pollStatus() {
  fetch('/api/status')
    .then(function(r) { return r.json(); })
    .then(function(s) {
      document.getElementById('dot').className = 'dot ' + (s.running ? 'on' : 'off');
      document.getElementById('st-running').textContent = s.running ? 'Running' : 'Stopped';
      document.getElementById('st-pid').textContent = s.pid || '-';
      document.getElementById('st-uptime').textContent = s.uptime || '-';
      document.getElementById('st-restarts').textContent = s.restarts;
      document.getElementById('st-watching').textContent = s.watching ? 'ON' : 'OFF';
      document.getElementById('st-program').textContent = s.program + ' ' + (s.args || []).join(' ');
      var pauseBtn = document.getElementById('btn-pause');
      pauseBtn.textContent = s.watching ? 'Pause' : 'Resume';
    })
    .catch(function() {});
}

// --- Environment ---

var sensitivePatterns = ['KEY', 'PASSWORD', 'PASSWD', 'SECRET', 'TOKEN',
  'AWS_ACCESS', 'AWS_SECRET', 'CREDENTIAL', 'PRIVATE',
  'AUTH', 'SIGNING', 'CERT', 'CERTIFICATE',
  'API_KEY', 'ACCESS_KEY', 'ENCRYPT'];

function isSensitive(key) {
  var upper = key.toUpperCase();
  for (var i = 0; i < sensitivePatterns.length; i++) {
    if (upper.indexOf(sensitivePatterns[i]) !== -1) return true;
  }
  return false;
}

function fetchEnv() {
  fetch('/api/env')
    .then(function(r) { return r.json(); })
    .then(function(data) {
      pendingEnv = (data.pending || []).map(function(e) {
        return {key: e.key, value: e.value, masked: e.masked};
      });
      renderEnvList('env-current', data.current || [], false);
      renderEnvList('env-pending', data.pending || [], true);
      updatePendingCount();
    })
    .catch(function() {});
}

function updatePendingCount() {
  var countEl = document.getElementById('pending-count');
  if (countEl) countEl.textContent = pendingEnv.length + ' var(s)';
}

function renderEnvList(id, entries, editable) {
  var el = document.getElementById(id);
  if (!entries || entries.length === 0) {
    el.innerHTML = '<div style="color:#999;font-size:12px">(empty)</div>';
    return;
  }
  var html = '<table class="env-table">';
  for (var i = 0; i < entries.length; i++) {
    var e = entries[i];
    html += '<tr data-key="' + escAttr(e.key) + '" data-idx="' + i + '">';
    html += '<td style="max-width:120px">' + esc(e.key) + '</td>';
    html += '<td style="max-width:140px">' + renderValueCell(e, i, editable) + '</td>';
    if (editable) {
      html += '<td><button class="rm" onclick="removeEnv(' + i + ')">&times;</button></td>';
    }
    html += '</tr>';
  }
  html += '</table>';
  el.innerHTML = html;
}

function renderValueCell(e, idx, editable) {
  var val = e.value;
  var key = e.key;
  if (editable) {
    // Pending column
    var display = e.masked && !(e._revealed) ? '&#8226;&#8226;&#8226;&#8226;&#8226;&#8226;&#8226;&#8226;' : esc(val);
    if (e._editing) {
      return '<input type="text" id="edit-' + idx + '" value="' + escAttr(val) + '" ' +
        'onblur="commitEdit(' + idx + ', this.value)" ' +
        'onkeydown="if(event.key===\'Enter\')commitEdit(' + idx + ', this.value)" autofocus>';
    }
    var toggle = e.masked ? ' onclick="toggleReveal(' + idx + ')"' : '';
    var editClick = ' onclick="startEdit(' + idx + ')"';
    return '<span class="env-val"' + (e.masked ? toggle : editClick) + '>' + display + '</span>';
  } else {
    // Current column
    var revealed = currentRevealed[key];
    var display = e.masked && !revealed ? '&#8226;&#8226;&#8226;&#8226;&#8226;&#8226;&#8226;&#8226;' : esc(val);
    var cls = e.masked ? ' masked' : '';
    return '<span class="env-val' + cls + '" onclick="toggleCurrentReveal(\'' + escAttrKey(key) + '\')">' + display + '</span>';
  }
}

function escAttrKey(s) {
  return s.replace(/\\/g, '\\\\').replace(/'/g, "\\'");
}

function toggleCurrentReveal(key) {
  if (currentRevealed[key]) {
    delete currentRevealed[key];
  } else {
    currentRevealed[key] = true;
  }
  fetchEnvCurrentOnly();
}

function fetchEnvCurrentOnly() {
  fetch('/api/env')
    .then(function(r) { return r.json(); })
    .then(function(data) {
      renderEnvList('env-current', data.current || [], false);
    })
    .catch(function() {});
}

function toggleReveal(idx) {
  pendingEnv[idx]._revealed = !pendingEnv[idx]._revealed;
  renderEnvList('env-pending', pendingEnv, true);
}

function startEdit(idx) {
  if (pendingEnv[idx].masked) {
    pendingEnv[idx]._revealed = true;
  }
  pendingEnv[idx]._editing = true;
  renderEnvList('env-pending', pendingEnv, true);
  setTimeout(function() {
    var input = document.getElementById('edit-' + idx);
    if (input) {
      input.focus();
      input.select();
    }
  }, 50);
}

function commitEdit(idx, newValue) {
  if (!pendingEnv[idx]) return;
  pendingEnv[idx].value = newValue;
  pendingEnv[idx]._editing = false;
  // Recompute masked flag (value might no longer be sensitive if key changed)
  renderEnvList('env-pending', pendingEnv, true);
}

function removeEnv(idx) {
  pendingEnv.splice(idx, 1);
  renderEnvList('env-pending', pendingEnv, true);
  updatePendingCount();
}

function addEnv() {
  var key = document.getElementById('add-key').value.trim();
  var val = document.getElementById('add-val').value.trim();
  if (!key) return;
  pendingEnv.push({
    key: key,
    value: val,
    masked: isSensitive(key)
  });
  document.getElementById('add-key').value = '';
  document.getElementById('add-val').value = '';
  renderEnvList('env-pending', pendingEnv, true);
  updatePendingCount();
}

function loadEnvFile() {
  var path = document.getElementById('env-file').value.trim();
  if (!path) return;
  fetch('/api/env/load', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: path })
  })
    .then(function(r) { return r.json(); })
    .then(function(data) {
      pendingEnv = (data.pending || []).map(function(e) {
        return {key: e.key, value: e.value, masked: e.masked};
      });
      renderEnvList('env-pending', pendingEnv, true);
      renderEnvList('env-current', data.current || [], false);
      updatePendingCount();
    })
    .catch(function() {});
}

function applyEnv() {
  var vars = pendingEnv.map(function(e) { return e.key + '=' + e.value; });
  fetch('/api/env', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ vars: vars })
  })
    .then(function() {
      fetch('/api/env/apply', { method: 'POST' })
        .then(function() {
          pollStatus();
          // Re-fetch env after restart
          setTimeout(fetchEnv, 500);
        })
        .catch(function() {});
    })
    .catch(function() {});
}

// --- Actions ---

function api(action) {
  fetch('/api/' + action, { method: 'POST' })
    .then(function() { pollStatus(); })
    .catch(function() {});
}

function signal(name) {
  fetch('/api/signal/' + name, { method: 'POST' }).catch(function() {});
}

function build() {
  var cmd = document.getElementById('build-cmd').value.trim();
  if (!cmd) return;
  saveBuildCmd();
  fetch('/api/build', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ cmd: cmd })
  }).catch(function() {});
}

// --- Cookie helpers ---

function loadBuildCmd() {
  var cmd = getCookie('devil-buildcmd');
  if (cmd) {
    document.getElementById('build-cmd').value = decodeURIComponent(cmd);
  }
}

function saveBuildCmd() {
  var cmd = document.getElementById('build-cmd').value.trim();
  setCookie('devil-buildcmd', encodeURIComponent(cmd), 365);
}

function getCookie(name) {
  var match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? match[2] : null;
}

function setCookie(name, value, days) {
  var d = new Date();
  d.setTime(d.getTime() + days * 86400000);
  document.cookie = name + '=' + value + ';expires=' + d.toUTCString() + ';path=/;SameSite=Lax';
}

// --- Utilities ---

function esc(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function escAttr(s) {
  return s.replace(/'/g, "\\'").replace(/"/g, '&quot;');
}
