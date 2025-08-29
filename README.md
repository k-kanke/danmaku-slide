danmaku-slide

ニコニコ動画風の弾幕コメントをスライドや配信画面に重ねて流せる Web アプリです。登壇者はワンクリックでルーム作成し、オーバーレイURLとQRコードを共有。参加者はQRからアクセスしてコメント投稿するだけで、右→左へコメントが流れます。

機能概要
- シンプル体験: インストール不要、QRで即参加。
- 低レイテンシ: Go製 WebSocket ハブで高速配信。
- 安全設計: NGワードフィルタ、レート制限。
- 拡張性: 将来的にAIによる検知・要約・感情分析なども追加可能。

想定利用シーン
- 勉強会・LT大会
- 学生イベント・サークル
- カジュアルなカンファレンス
- 配信（OBSのブラウザソース対応）

リポジトリ構成（予定）
- `backend/`: Go サーバ（API, WebSocket, テンプレート配信）
- `frontend/`: 将来のフロントエンド資産（必要に応じて整備）

進め方（ステップ）
0. Goプロジェクト初期化と /health 実装
1. ルーム生成とQR返却 API
2. WebSocket Hub（ルーム単位で配信）
3. 投稿API（レート制限/NGワード/ブロードキャスト）
4. オーバーレイ（Canvas, rAF, レーン制御）
5. 投稿フォーム（参加者用）
6. モデレーション（Pause/Clear/SlowMode）
7. Dockerfile 作成

---

セットアップ済みの主なエンドポイント
- `POST /rooms` ルーム作成（`roomId`, `overlayUrl`, `postUrl`, `qrPngBase64`）
- `GET /ws/:roomId` WebSocket（ルーム単位のHub）
- `POST /rooms/:roomId/messages` 投稿（レート制限/NGワード/スローモード/一時停止）
- `GET /overlay/:roomId` 透明Canvasオーバーレイ
- `GET /post/:roomId` 参加者用フォーム
- `GET /admin/:roomId` 管理パネル（Pause/Resume/Clear/SlowMode）

ローカル起動
```
cd backend
go run .
```

Docker ビルド/起動
```
docker build -t slideflow .
docker run --rm -p 8080:8080 -e PORT=8080 slideflow
```

NGワードの上書き
```
export NG_WORDS="word1,word2,死ね"
```
