package app

import (
    "fmt"
    "net/http"
    "strings"
)

// GET /overlay/:roomId -> HTML + JS overlay (transparent canvas)
func (s *Server) handleOverlay(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    roomID := strings.TrimPrefix(r.URL.Path, "/overlay/")
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
  <title>SlideFlow Overlay - %s</title>
  <style>
    html, body { margin:0; padding:0; background:transparent; height:100%%; overflow:hidden; }
    canvas { display:block; width:100vw; height:100vh; background:transparent; pointer-events:none; }
  </style>
</head>
<body>
  <canvas id="overlay"></canvas>
  <script>
  (function(){
    const roomId = %q;
    const canvas = document.getElementById('overlay');
    const ctx = canvas.getContext('2d');
    let dpr = window.devicePixelRatio || 1;
    let width = 0, height = 0;
    let lastTime = performance.now();
    const fontSize = 36; // px
    const lineHeight = Math.round(fontSize * 1.2);
    const speed = 160; // px per second
    const maxMessages = 200;

    function resize(){
      dpr = window.devicePixelRatio || 1;
      width = Math.floor(window.innerWidth);
      height = Math.floor(window.innerHeight);
      canvas.width = Math.floor(width * dpr);
      canvas.height = Math.floor(height * dpr);
      canvas.style.width = width + 'px';
      canvas.style.height = height + 'px';
      ctx.setTransform(dpr,0,0,dpr,0,0);
      ctx.font = 'bold ' + fontSize + "px -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans JP', 'Hiragino Kaku Gothic ProN', Meiryo, Arial, sans-serif";
      ctx.textBaseline = 'top';
    }
    window.addEventListener('resize', resize);
    resize();

    function laneCount(){ return Math.max(1, Math.floor(height / lineHeight)); }
    function laneY(l){ return Math.round(l * lineHeight + (lineHeight - fontSize) / 2); }
    const bullets = [];
    const inbox = [];
    function rightmostXOfLane(l){
      let maxX = -Infinity;
      for (const b of bullets){ if (b.lane === l) { maxX = Math.max(maxX, b.x + b.w); } }
      return maxX === -Infinity ? -1 : maxX;
    }
    function tryPlace(text, color){
      if (!text) return false;
      const w = Math.ceil(ctx.measureText(text).width);
      const L = laneCount();
      let bestLane = -1;
      let bestRight = Infinity;
      for (let l=0; l<L; l++){
        const r = rightmostXOfLane(l);
        if (r < bestRight) { bestRight = r; bestLane = l; }
      }
      if (bestLane === -1) return false;
      if (bestRight < width - 140){
        bullets.push({text, x: width, y: laneY(bestLane), w, lane:bestLane, speed, color});
        return true;
      }
      return false;
    }
    function draw(){
      const now = performance.now();
      const dt = Math.min(0.05, (now - lastTime) / 1000);
      lastTime = now;
      ctx.clearRect(0,0,width,height);
      for (let i=bullets.length-1; i>=0; i--){
        const b = bullets[i];
        b.x -= b.speed * dt;
        if (b.x + b.w < 0){ bullets.splice(i,1); continue; }
        ctx.save();
        ctx.shadowColor = 'rgba(0,0,0,0.7)';
        ctx.shadowBlur = 4; ctx.shadowOffsetX = 2; ctx.shadowOffsetY = 2;
        ctx.fillStyle = b.color || '#fff';
        ctx.fillText(b.text, Math.round(b.x), b.y);
        ctx.restore();
      }
      for (let i=0; i<inbox.length && bullets.length < maxMessages; ){
        if (tryPlace(inbox[i].text, inbox[i].color)){
          inbox.splice(i,1);
        } else {
          i++;
        }
      }
      requestAnimationFrame(draw);
    }
    requestAnimationFrame(draw);

    const wsProto = (location.protocol === 'https:') ? 'wss' : 'ws';
    const wsUrl = wsProto + '://' + location.host + '/ws/' + roomId;
    let ws;
    function connect(){
      ws = new WebSocket(wsUrl);
      ws.addEventListener('message', (ev)=>{
        try {
          const msg = JSON.parse(ev.data);
          if (msg && msg.type === 'chat'){
            const txt = String(msg.text || '').slice(0, 200);
            const handle = (msg.handle ? String(msg.handle) : '').trim();
            const text = handle ? '【' + handle + '】 ' + txt : txt;
            inbox.push({ text, color: '#ffffff' });
          } else if (msg && msg.type === 'clear'){
            bullets.length = 0; inbox.length = 0;
          }
        } catch(e){
          const t = String(ev.data || '');
          inbox.push({ text: t, color: '#ffffff' });
        }
      });
      ws.addEventListener('close', ()=> setTimeout(connect, 1000));
      ws.addEventListener('error', ()=> { try{ ws.close(); }catch{} });
    }
    connect();
  })();
  </script>
</body>
</html>`, roomID, roomID)
}

