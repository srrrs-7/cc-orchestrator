---
paths:
  - "app/auth/**"
---

# app/auth — 認証・認可 API(Go / OAuth 2.0 + OIDC)規約

`app/auth` は OAuth 2.0 認可サーバー兼 OpenID Provider の基盤実装。`app/api` と同一の
エリック・エヴァンス DDD レイヤ構成(`domain` / `service` / `infra` / `route` / `cmd`)を踏襲し、
標準ライブラリのみで実装する(JWT の RS256 署名・JWKS・PKCE S256 はすべて `crypto/*`・
`encoding/*` で完結する)。

## コマンド

実行はすべて `app/auth` ディレクトリで行う。checker / tester はこれを実行する。各ターゲットの実体は `app/auth/Makefile` が単一の情報源(`app/api/Makefile` と同じ構成)。

| 目的 | コマンド |
|---|---|
| format(チェック) | `make fmt-check` |
| format(自動修正) | `make fmt` |
| lint | `make lint` |
| type check 相当 | `make vet` && `make build` |
| test | `make test`(race 検査は `make test-race`) |
| 上記すべて | `make check` |

## レイアウト

`app/api` と同一のレイヤ別トップディレクトリ構成を採る(`internal/` は用いない)。

- `cmd/authz/main.go` — エントリポイント(配線のみ。RSA 鍵生成・リポジトリ seed・サーバ起動)
- `domain/<aggregate>/` — ドメイン層(他層に非依存)。集約ごとにパッケージを切る
- `service/` — アプリケーション層(ユースケース。ドメインを協調させる薄い層)
- `infra/` — インフラ層(リポジトリ実装・RSA 署名器などドメインが宣言したポートの実装)
- `route/` — プレゼンテーション層(HTTP ハンドラ・ルーティング・エラー変換)

## コーディング

`app/api` と同一(`api.md` のコーディング規約に準拠)。加えて:

- `context.Context` は第一引数で受け渡す。struct フィールドに保持しない
- エラーは `fmt.Errorf("...: %w", err)` でラップする。分岐したいエラーは sentinel / カスタム型 + `errors.Is` / `errors.As`
- 外部依存(署名鍵・リポジトリ)は interface(ドメインが宣言するポート)越しにし、テストで差し替え可能にする
- goroutine は終了条件(context cancel 等)を必ず持つ

## セキュリティ規約(認証基盤ゆえ厳守)

- 秘密鍵・クライアントシークレット・パスワードをコード・ドキュメントに直接埋め込まない。デモ用の値は起動時に生成 / seed し、本番は環境変数・外部から注入する前提にする
- 認可コードは単回使用(consume 後は再利用不可)かつ短命。PKCE(S256)を必須とする
- JWT は RS256 で署名し、`kid` 付きヘッダ + JWKS で公開鍵を配布する。`alg: none` を受理しない
- ID Token / アクセストークンの検証では iss / aud / exp / 署名をすべて検証する
- 標準に沿ったエラー応答(OAuth: `error` / `error_description`、HTTP ステータス)を返し、内部情報を漏らさない

## 準拠する公式仕様

- OAuth 2.0 Authorization Framework(RFC 6749)、Bearer Token 利用(RFC 6750)
- PKCE(RFC 7636、`code_challenge_method=S256`)
- JWT(RFC 7519)、JWS(RFC 7515、RS256)、JWK / JWK Set(RFC 7517)
- OpenID Connect Core 1.0、OpenID Connect Discovery 1.0
