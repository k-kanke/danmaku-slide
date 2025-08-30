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

クイックスタート（発表者向け・おすすめ）
- `cd backend && go run .` を起動
- ブラウザで `http://localhost:8080/present` を開く
- 画面下のバーで資料ファイルを選択
  - ファイル選択: PNG/JPGを複数同時選択可、PDF（単一）も可
  - フォルダ選択: 画像フォルダを丸ごと選択可（サブパス順／自然順で並びます）
  - `F` で全画面、`H` でオーバーレイ表示/非表示、`←/→` で前後スライド
- 右下「QR表示」から参加者に投稿ページを共有できます（同時に `投稿ページを開く` も可）

備考（ファイル形式）
- 画像: PNG/JPG を複数可。`←/→` でページ送り。
- PDF: 単一ファイルに対応（埋め込みビューアで表示）。ページ操作はPDFビューア側のUIまたは `PageUp/PageDown/矢印キー` を使用してください。
- PPTXなど: いったんPDFに書き出して利用してください（ネイティブ対応は検討中）。

セットアップ済みの主なエンドポイント
- `POST /rooms` ルーム作成（`roomId`, `overlayUrl`, `postUrl`, `qrPngBase64`）
- `GET /ws/:roomId` WebSocket（ルーム単位のHub）
- `POST /rooms/:roomId/messages` 投稿（レート制限/NGワード/スローモード/一時停止）
- `GET /overlay/:roomId` 透明Canvasオーバーレイ
- `GET /post/:roomId` 参加者用フォーム
- `GET /admin/:roomId` 管理パネル（Pause/Resume/Clear/SlowMode）
- `GET /present` 発表者UI（画像スライド選択 + オーバーレイ）

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
