---
id: SPEC-014
title: app/web Content-Security-Policy ヘッダー導入
status: done
created: 2026-07-11
updated: 2026-07-11
issues: [ISSUE-042]
supersedes: null
---

# SPEC-014: app/web Content-Security-Policy ヘッダー導入

## 1. ユーザー価値(なぜ作るか)

> **Task Manager を利用するユーザー** が **XSS 等のコンテンツ注入攻撃から保護された Web UI** でタスクを操作でき、**開発者・運用者** が **セキュリティベースラインを HTTP ヘッダーで明示的に強制** できる。

- **対象ユーザー**: Task Manager Web UI のエンドユーザー、およびローカル compose / AWS 上で web を配信する開発者・運用者
- **解決する課題**: 現状 `X-Content-Type-Options` と `X-Frame-Options` のみで、コンテンツ読み込み元を制限する CSP が未設定。AWS 本番経路(S3 + CloudFront)ではベースラインのセキュリティヘッダー自体が付与されていない
- **得られる価値**: ブラウザが同一オリジン以外のスクリプト・スタイル・接続先を拒否し、注入された悪意あるコンテンツの実行を抑止する
- **価値の検証方法**: ローカル compose の web 応答と CloudFront の default behavior(S3/web)応答に同一の `Content-Security-Policy` が含まれること。`make -C app/web check` と `make -C app/iac check` が green。review-security が方針を承認すること

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- エンドユーザーとして、見た目や操作感は変わらず Task Manager を使いたい。なぜなら CSP は正常な同一オリジン SPA には影響を与えない設計だから。
- 開発者として、ローカル compose と AWS 本番で同じ CSP 契約を維持したい。なぜなら配信経路が nginx(ローカル)と CloudFront+S3(本番)で異なるため。

### 利用フロー

1. ユーザーが web を開く(ローカル `http://localhost:8080` または CloudFront ドメイン)
2. ブラウザが HTML / JS / CSS を同一オリジンから読み込む(Vite ビルド成果物)
3. アプリが `/api` へ fetch する(同一オリジン。ローカルは nginx リバースプロキシ、本番は CloudFront `/api/*` behavior)
4. 外部オリジンへのスクリプト注入があっても CSP により実行がブロックされる

## 3. 要件(何を満たすべきか)

### 機能要件

- [x] R1: web SPA の HTML 応答に `Content-Security-Policy` を付与する。ポリシー文字列はローカル(nginx)と本番(CloudFront default behavior)で同一とする
- [x] R2: ポリシーは現行アプリの読み込み実態に合わせ、外部 CDN・インライン script/style・`eval` を許可しない(strict same-origin)
- [x] R3: ローカル `app/web/nginx.conf` の既存 `X-Content-Type-Options` / `X-Frame-Options` を維持し、静的アセット location でもヘッダーが欠落しない(nginx の `add_header` 上書き挙動に配慮)
- [x] R4: AWS 本番は `modules/cdn` の CloudFront **default behavior(S3/web)のみ** に Response Headers Policy を関連付ける。`/api/*` `/auth/*` behavior には CSP を付けない(JSON / OIDC 応答への不要な制約を避ける)
- [x] R5: 本番経路では既存の nginx ベースラインと同等の `X-Content-Type-Options` / `X-Frame-Options` も CloudFront Response Headers Policy で付与する

### 非機能要件

- **アプリコード不変**: React / Vite ソースに変更を持ち込まない(配信層のみ)
- **dev サーバー**: Vite dev server(`bun run dev`)は対象外。MSW・HMR は compose / 本番相当構成とは別経路
- **同期**: nginx と Terraform の CSP 文字列は手動で二重管理となる。変更時は両方を同時に更新する(単一ソース化はスコープ外)

### 確定 CSP 文字列

```
default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'
```

根拠: Vite 本番ビルドはハッシュ付き外部 JS/CSS のみ。API は同一オリジン `/api`。外部フォント・画像 CDN なし。`data:` は将来のインライン SVG 等に備え img のみ許可。

### スコープ外(やらないこと)

- CSP 違反レポートエンドポイント(`report-uri` / `report-to`)
- `Content-Security-Policy-Report-Only` 段階的ロールアウト
- api / auth Go サービスへのミドルウェア追加(JSON 応答が主で HTML 表面が小さい)
- Vite dev server 向け CSP
- カスタムドメイン移行時の HSTS `includeSubDomains` 再設計

## 4. 設計(どう実現するか)

### 方針

配信層のみ変更する。ローカルは `app/web/nginx.conf`、AWS は `app/iac/modules/cdn/main.tf` に `aws_cloudfront_response_headers_policy` を追加し default behavior に `response_headers_policy_id` を設定する。

### アーキテクチャ

| 経路 | 変更箇所 | 備考 |
|---|---|---|
| ローカル compose | `app/web/nginx.conf` | server ブロック + 静的アセット location |
| AWS 本番 web | `app/iac/modules/cdn/main.tf` | default_cache_behavior のみ |
| AWS api/auth | 変更なし | 別 behavior。CSP 非付与 |

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| meta タグで CSP | HTTP ヘッダーの方が強制力が高く、既存ヘッダーと同じ層で管理できる |
| `'unsafe-inline'` 許可 | 本番ビルドにインライン script/style が無く不要。XSS 防御が弱まる |
| 全 behavior に同一 CSP | api/auth は JSON / リダイレクト応答が主で HTML CSP の恩恵が薄い |

## 5. 実装計画

- [x] T1: impl-web が `app/web/nginx.conf` に CSP を追加
- [x] T2: impl-iac が `modules/cdn` に Response Headers Policy を追加
- [x] T3: checker が `make -C app/web check` / `make -C app/iac check` を実行
- [x] T4: review-security がポリシー妥当性をレビュー

## 6. 経緯(時系列・追記のみ)

### 2026-07-11

- 初版作成。web SPA 配信層(nginx / CloudFront)へ strict same-origin CSP を導入する方針で `in-progress`。
- 実装完了。impl-web が `app/web/nginx.conf` に CSP + Referrer-Policy を追加。impl-iac が `modules/cdn` に Response Headers Policy を追加(default behavior のみ)。checker green。review-security: Blocker/Major 0、Minor-1(Referrer-Policy nginx 欠落)を是正済み。status を `done` に更新。
