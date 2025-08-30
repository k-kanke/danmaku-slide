package app

import (
    "fmt"
    "net/http"
)

// GET /present -> Presenter UI: select slide images, full-screen viewer + overlay canvas
func (s *Server) handlePresent(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprintf(w, `<!doctype html>
<html lang="ja">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>SlideFlow Present</title>
  <style>
    :root { color-scheme: light dark; }
    html, body { height:100%%; margin:0; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans JP', 'Hiragino Kaku Gothic ProN', Meiryo, Arial, sans-serif; }
    .bar { position: fixed; inset: auto 0 0 0; display:flex; gap:12px; align-items:center; padding:10px 12px; background: rgba(0,0,0,.5); color:#fff; z-index: 10000; }
    .bar button, .bar input[type="file"] { font-size:14px; }
    .wrap { position:fixed; inset:0; background:#000; }
    .stage { position:absolute; inset:0; display:grid; place-items:center; }
    #slide { max-width:100vw; max-height:100vh; display:none; }
    #pdf { position:absolute; inset:0; width:100%%; height:100%%; border:0; display:none; background:#111; }
    #overlay { position:absolute; inset:0; width:100%%; height:100%%; background:transparent; pointer-events:none; }
    .spacer { flex:1; }
    .qr { display:none; position: fixed; right: 16px; top: 16px; padding: 8px; background: rgba(0,0,0,.6); border-radius: 8px; z-index: 10001; }
    .qr img { width: 200px; height: 200px; display:block; }
    .hint { font-size:12px; opacity: .85; }
  </style>
  </head>
  <body>
    <div class="wrap">
      <div class="stage">
        <img id="slide" alt="slide" />
        <iframe id="pdf" title="PDF viewer"></iframe>
        <canvas id="overlay"></canvas>
      </div>
      <div class="qr" id="qrBox"><img id="qrImg" alt="QR" /><div id="qrTxt" class="hint"></div></div>
      <div class="bar">
        <input type="file" id="files" accept="image/*,.pdf,application/pdf" multiple />
        <label class="hint">または</label>
        <input type="file" id="dirpick" accept="image/*" webkitdirectory directory />
        <button id="prev">前へ ⬅︎</button>
        <button id="next">次へ ➡︎</button>
        <button id="toggleOverlay">オーバーレイ表示/非表示 (H)</button>
        <button id="fullscreen">全画面 (F)</button>
        <div class="spacer"></div>
        <span id="idx" class="hint">0 / 0</span>
        <button id="qr">QR表示</button>
        <a id="postLink" href="#" target="_blank">投稿ページを開く</a>
        <a id="adminLink" href="#" target="_blank" style="margin-left:8px;">管理パネル</a>
      </div>
    </div>

    <script>
    (function(){
      const files = document.getElementById('files');
      const slide = document.getElementById('slide');
      const canvas = document.getElementById('overlay');
      const ctx = canvas.getContext('2d');
      const prevBtn = document.getElementById('prev');
      const nextBtn = document.getElementById('next');
      const fsBtn = document.getElementById('fullscreen');
      const toggleBtn = document.getElementById('toggleOverlay');
      const idxTxt = document.getElementById('idx');
      const qrBtn = document.getElementById('qr');
      const qrBox = document.getElementById('qrBox');
      const qrImg = document.getElementById('qrImg');
      const qrTxt = document.getElementById('qrTxt');
      const postLink = document.getElementById('postLink');
      const adminLink = document.getElementById('adminLink');
      const dirpick = document.getElementById('dirpick');
      const pdfFrame = document.getElementById('pdf');
      let dpr = window.devicePixelRatio || 1;
      let width = 0, height = 0;
      let lastTime = performance.now();
      const fontSize = 36; // px
      const lineHeight = Math.round(fontSize * 1.2);
      const speed = 160; // px/sec
      const maxMessages = 200;
      let overlayVisible = true;

      // Slides state
      let urls = []; // for images
      let pdfUrl = '';
      let i = 0;
      function renderImage(){
        slide.style.display = 'block';
        pdfFrame.style.display = 'none';
        slide.src = urls[i];
        idxTxt.textContent = (i+1) + ' / ' + urls.length;
      }
      function show(){
        if (pdfUrl){
          slide.style.display = 'none';
          pdfFrame.style.display = 'block';
          idxTxt.textContent = 'PDF';
          return;
        }
        if (!urls.length) { slide.removeAttribute('src'); slide.style.display='none'; pdfFrame.style.display='none'; idxTxt.textContent = '0 / 0'; return; }
        i = Math.max(0, Math.min(i, urls.length-1));
        renderImage();
      }
      function next(){ if (pdfUrl){ pdfFrame.focus(); sendKeyToPdf('PageDown'); return; } if (i < urls.length-1) { i++; renderImage(); } }
      function prev(){ if (pdfUrl){ pdfFrame.focus(); sendKeyToPdf('PageUp'); return; } if (i > 0) { i--; renderImage(); } }

      function loadImagesFromFileList(fileList){
        urls.forEach(u=> URL.revokeObjectURL(u));
        urls = Array.from(fileList)
          .filter(f => f.type.startsWith('image/'))
          .sort((a,b)=>{
            const ap = (a.webkitRelativePath||a.name);
            const bp = (b.webkitRelativePath||b.name);
            return ap.localeCompare(bp, undefined, {numeric:true, sensitivity:'base'});
          })
          .map(f => URL.createObjectURL(f));
        i = 0; show();
      }

      files.addEventListener('change', ()=>{
        // revoke previous
        urls.forEach(u=> URL.revokeObjectURL(u));
        if (pdfUrl) { URL.revokeObjectURL(pdfUrl); pdfUrl = ''; }
        const fs = Array.from(files.files || []);
        const hasPdf = fs.find(f => f.type === 'application/pdf' || f.name.toLowerCase().endsWith('.pdf'));
        if (hasPdf){
          pdfUrl = URL.createObjectURL(hasPdf);
          pdfFrame.src = pdfUrl;
          urls = [];
          i = 0; show();
          setTimeout(()=> pdfFrame.focus(), 100);
          return;
        }
        loadImagesFromFileList(fs);
      });

      dirpick.addEventListener('change', ()=>{
        if (!dirpick.files || !dirpick.files.length) return;
        if (pdfUrl) { URL.revokeObjectURL(pdfUrl); pdfUrl = ''; }
        loadImagesFromFileList(dirpick.files);
      });

      function sendKeyToPdf(code){
        try {
          pdfFrame.contentWindow && pdfFrame.contentWindow.focus();
        } catch(e){}
        // The embedded PDF viewer will handle PageUp/PageDown/Arrow keys when focused
      }

      document.addEventListener('keydown', (e)=>{
        if (e.key === 'ArrowRight' || e.key === 'PageDown' || e.key === ' ') { next(); }
        else if (e.key === 'ArrowLeft' || e.key === 'PageUp' || (e.shiftKey && e.key===' ')) { prev(); }
        else if (e.key.toLowerCase() === 'f') { toggleFullscreen(); }
        else if (e.key.toLowerCase() === 'h') { overlayVisible = !overlayVisible; }
      });
      function toggleFullscreen(){
        const el = document.documentElement;
        if (!document.fullscreenElement) { el.requestFullscreen && el.requestFullscreen(); }
        else { document.exitFullscreen && document.exitFullscreen(); }
      }
      fsBtn.addEventListener('click', toggleFullscreen);
      toggleBtn.addEventListener('click', ()=>{ overlayVisible = !overlayVisible; });

      // Overlay rendering (same as /overlay with small tweaks)
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
        if (overlayVisible) {
          for (let i=bullets.length-1; i>=0; i--){
            const b = bullets[i]; b.x -= b.speed * dt;
            if (b.x + b.w < 0){ bullets.splice(i,1); continue; }
            ctx.save();
            ctx.shadowColor = 'rgba(0,0,0,0.7)';
            ctx.shadowBlur = 4; ctx.shadowOffsetX = 2; ctx.shadowOffsetY = 2;
            ctx.fillStyle = b.color || '#fff';
            ctx.fillText(b.text, Math.round(b.x), b.y);
            ctx.restore();
          }
          for (let i=0; i<inbox.length && bullets.length < maxMessages; ){
            if (tryPlace(inbox[i].text, inbox[i].color)) { inbox.splice(i,1); }
            else { i++; }
          }
        }
        requestAnimationFrame(draw);
      }
      requestAnimationFrame(draw);

      // Create room on load and wire QR + WS
      const base = location.origin;
      const create = async () => {
        const res = await fetch('/rooms', { method:'POST' });
        if (!res.ok) throw new Error('room create failed');
        return res.json();
      };
      let roomId = '';
      create().then(info => {
        roomId = info.roomId;
        postLink.href = info.postUrl;
        postLink.textContent = '投稿ページ';
        adminLink.href = base + '/admin/' + info.roomId;
        qrImg.src = 'data:image/png;base64,' + info.qrPngBase64;
        qrTxt.textContent = info.postUrl;
        const wsProto = (location.protocol === 'https:') ? 'wss' : 'ws';
        const wsUrl = wsProto + '://' + location.host + '/ws/' + roomId;
        function connect(){
          const ws = new WebSocket(wsUrl);
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
      }).catch(err => {
        alert('ルーム作成に失敗しました: ' + err.message);
      });

      qrBtn.addEventListener('click', ()=>{
        qrBox.style.display = (qrBox.style.display === 'none' || !qrBox.style.display) ? 'block' : 'none';
      });
    })();
    </script>
  </body>
</html>`)
}

