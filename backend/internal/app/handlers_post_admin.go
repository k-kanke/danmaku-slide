package app

import (
    "fmt"
    "net/http"
    "strings"
    "time"
)

// GET /post/:roomId -> simple HTML form to submit messages
func (s *Server) handlePostForm(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    roomID := strings.TrimPrefix(r.URL.Path, "/post/")
    s.mu.Lock()
    _, ok := s.rooms[roomID]
    s.mu.Unlock()
    if !ok {
        http.Error(w, "room not found", http.StatusNotFound)
        return
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprintf(w, `<!doctype html>
<html lang="ja">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>SlideFlow Post - %s</title>
  <style>
    :root { color-scheme: light dark; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans JP', 'Hiragino Kaku Gothic ProN', Meiryo, Arial, sans-serif; margin: 24px; }
    .wrap { max-width: 640px; margin: 0 auto; }
    label { display:block; margin: 12px 0 6px; font-weight: 600; }
    input, textarea, button { width:100%%; font-size:16px; padding:10px; box-sizing:border-box; }
    textarea { height: 120px; resize: vertical; }
    .row { display:flex; gap:12px; align-items:center; }
    .row > * { flex: 1; }
    .hint { color: #888; font-size: 12px; }
    .status { margin-top: 12px; min-height: 1.4em; }
    button { cursor: pointer; }
  </style>
</head>
<body>
  <div class="wrap">
    <h1>コメント投稿</h1>
    <p class="hint">ルームID: <code>%s</code></p>
    <form id="msgForm">
      <label for="handle">ハンドルネーム（任意・32文字まで）</label>
      <input id="handle" name="handle" maxlength="32" placeholder="例: alice" />

      <label for="text">コメント（必須・200文字まで）</label>
      <textarea id="text" name="text" maxlength="200" placeholder="今のスライドに一言！"></textarea>
      <div class="row">
        <div class="hint" id="counter">0 / 200</div>
        <div class="hint">送信後、すぐにスクリーンへ流れます。</div>
      </div>
      <button id="submitBtn" type="submit">送信</button>
      <div class="status" id="status"></div>
    </form>
  </div>
  <script>
  (function(){
    const roomId = %q;
    const form = document.getElementById('msgForm');
    const text = document.getElementById('text');
    const handle = document.getElementById('handle');
    const counter = document.getElementById('counter');
    const status = document.getElementById('status');
    const submitBtn = document.getElementById('submitBtn');

    text.addEventListener('input', ()=>{
      const n = (text.value||'').length; counter.textContent = n + ' / 200';
    });
    form.addEventListener('submit', async (e)=>{
      e.preventDefault();
      const payload = { text: (text.value||'').trim(), handle: (handle.value||'').trim() };
      if (!payload.text){ status.textContent = 'テキストは必須です'; return; }
      submitBtn.disabled = true;
      try {
        const res = await fetch('/rooms/' + roomId + '/messages', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload)
        });
        if (res.ok) { status.textContent = '送信しました'; text.value = ''; counter.textContent='0 / 200'; }
        else { status.textContent = 'エラー: ' + await res.text(); }
      } catch(e){ status.textContent = 'ネットワークエラー'; }
      finally { submitBtn.disabled = false; }
    });
  })();
  </script>
</body>
</html>`, roomID, roomID, roomID)
}

// GET /admin/:roomId -> simple admin controls
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    roomID := strings.TrimPrefix(r.URL.Path, "/admin/")
    s.mu.Lock()
    rm, ok := s.rooms[roomID]
    s.mu.Unlock()
    if !ok {
        http.Error(w, "room not found", http.StatusNotFound)
        return
    }
    paused := rm.Paused
    slowMs := int(rm.SlowMode / time.Millisecond)
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprintf(w, `<!doctype html>
<html lang="ja">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>SlideFlow Admin - %s</title>
  <style>
    :root { color-scheme: light dark; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans JP', 'Hiragino Kaku Gothic ProN', Meiryo, Arial, sans-serif; margin: 24px; }
    .wrap { max-width: 640px; margin: 0 auto; }
    h1 { margin-bottom: 4px; }
    .hint { color: #888; font-size: 12px; margin-bottom: 16px; }
    label { display:block; margin: 12px 0 6px; font-weight: 600; }
    input, button { font-size:16px; padding:10px; }
    .row { display:flex; gap:12px; align-items:center; }
    .status { margin-top: 12px; min-height: 1.4em; }
    button { cursor: pointer; }
  </style>
</head>
<body>
  <div class="wrap">
    <h1>管理パネル</h1>
    <p class="hint">ルームID: <code>%s</code></p>
    <div class="row">
      <button id="pauseBtn">一時停止</button>
      <button id="resumeBtn">再開</button>
      <button id="clearBtn">全消去</button>
    </div>
    <label for="slow">スローモード（ミリ秒）</label>
    <div class="row">
      <input id="slow" type="number" min="0" step="100" value="%d" />
      <button id="applySlow">適用</button>
    </div>
    <div class="status" id="status"></div>
  </div>
  <script>
  (function(){
    const roomId = %q;
    const status = document.getElementById('status');
    const pauseBtn = document.getElementById('pauseBtn');
    const resumeBtn = document.getElementById('resumeBtn');
    const clearBtn = document.getElementById('clearBtn');
    const slow = document.getElementById('slow');
    const applySlow = document.getElementById('applySlow');
    let paused = %t;

    function setStatus(t){ status.textContent = t; }
    function post(path, body){
      return fetch('/rooms/' + roomId + '/' + path, {
        method:'POST', headers:{'Content-Type':'application/json'},
        body: body ? JSON.stringify(body) : null
      });
    }

    pauseBtn.addEventListener('click', async ()=>{
      const res = await post('pause');
      if (res.ok){ paused = true; setStatus('一時停止しました'); } else setStatus('エラー: ' + await res.text());
    });
    resumeBtn.addEventListener('click', async ()=>{
      const res = await post('resume');
      if (res.ok){ paused = false; setStatus('再開しました'); } else setStatus('エラー: ' + await res.text());
    });
    clearBtn.addEventListener('click', async ()=>{
      const res = await post('clear');
      if (res.ok){ setStatus('全消去を送信しました'); } else setStatus('エラー: ' + await res.text());
    });
    applySlow.addEventListener('click', async ()=>{
      const ms = parseInt(slow.value||'0', 10) || 0;
      const res = await post('slowmode', {ms});
      if (res.ok){ setStatus('スローモード: ' + ms + 'ms'); } else setStatus('エラー: ' + await res.text());
    });
  })();
  </script>
</body>
</html>`, roomID, roomID, slowMs, roomID, paused)
}

