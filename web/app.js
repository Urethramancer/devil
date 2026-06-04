var autoscroll = true;
var hasScrolled = false;

document.addEventListener('DOMContentLoaded', function() {
  connectSSE();
  loadBuildCmd();
  pollStatus();
  setInterval(pollStatus, 2000);
  setInterval(fetchEnv, 3000);

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

var reconnecting = false;

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
      var dot = document.getElementById('dot');
      dot.className = 'dot ' + (s.running ? 'on' : 'off');
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

function fetchEnv() {
  fetch('/api/env')
    .then(function(r) { return r.json(); })
    .then(function(data) {
      renderEnvList('env-current', data.current || []);
      renderEnvList('env-pending', data.pending || [], true);
    })
    .catch(function() {});
}

function renderEnvList(id, vars, editable) {
  var el = document.getElementById(id);
  if (vars.length === 0) {
    el.innerHTML = '<div style="color:#999;font-size:12px">(empty)</div>';
    return;
  }
  var html = '<table class="env-table">';
  for (var i = 0; i < vars.length; i++) {
    var eq = vars[i].indexOf('=');
    var key = vars[i].substring(0, eq);
    var val = vars[i].substring(eq + 1);
    html += '<tr><td style="max-width:120px">' + esc(key) + '</td><td style="max-width:140px">' + esc(val) + '</td>';
    if (editable) {
      html += '<td><button class="rm" onclick="removeEnv(\'' + escAttr(key) + '\')">&times;</button></td>';
    }
    html += '</tr>';
  }
  html += '</table>';
  el.innerHTML = html;
}

function esc(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function escAttr(s) {
  return s.replace(/'/g, "\\'").replace(/"/g, '&quot;');
}

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

function addEnv() {
  var key = document.getElementById('add-key').value.trim();
  var val = document.getElementById('add-val').value.trim();
  if (!key) return;
  fetch('/api/env/add', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key: key, value: val })
  })
    .then(function() {
      document.getElementById('add-key').value = '';
      document.getElementById('add-val').value = '';
      fetchEnv();
    })
    .catch(function() {});
}

function removeEnv(key) {
  fetch('/api/env/remove/' + encodeURIComponent(key), { method: 'DELETE' })
    .then(function() { fetchEnv(); })
    .catch(function() {});
}

function loadEnvFile() {
  var path = document.getElementById('env-file').value.trim();
  if (!path) return;
  fetch('/api/env/load', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: path })
  })
    .then(function() { fetchEnv(); })
    .catch(function() {});
}

function applyEnv() {
  fetch('/api/env/apply', { method: 'POST' })
    .then(function() { pollStatus(); })
    .catch(function() {});
}

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
