---
paths:
  - "app/auth/**"
---

# app/auth — 認証・認可 API(Go / OAuth 2.0 + OIDC)規約

`app/auth` は OAuth 2.0 認可サーバー兼 OpenID Provider。`app/api` と同一の
エリック・エヴァンス DDD レイヤ構成(`domain` / `service` / `infra` / `route` / `cmd`)を踏襲し、
プロトコル・暗号は標準ライブラリで完結する(JWT の RS256 署名・JWKS・PKCE S256 はすべて
`crypto/*`・`encoding/*`、refresh token は `crypto/rand` / `crypto/sha256`。唯一の例外は
パスワードハッシュの `golang.org/x/crypto/bcrypt`)。**永続化層
`infra/postgres` のみ `pgx` に依存する**(SPEC-005。DB 規約の正は `.claude/rules/db.md`)。

実装済みの機能(基盤の経緯は `docs/plans/AUTH-001-plan.md`、機能拡張の傘 Spec は SPEC-015):
authorize / token / userinfo / discovery(openid-configuration / jwks.json)に加え、IdP の
login / consent UI(cookie セッション + 永続化する scope 同意)、RP-Initiated Logout(`GET /logout`)、
revoke(RFC 7009)、introspect(RFC 7662)、admin API `/admin/{clients,users}`(API key 未設定なら
経路ごと未登録 = fail-closed)。グラントは Authorization Code(PKCE S256)+ refresh_token(SPEC-006)。
confidential client(client secret 認証)・API リソースサーバー向け audience 分離・
`prompt`(none / login)/ `max_age` に対応。

## コマンド

実行はすべて `app/auth` ディレクトリで行う。checker / tester はこれを実行する。各ターゲットの実体は `app/auth/Makefile` が単一の情報源(`app/api/Makefile` と同じ構成)。SPEC-009 により全コマンドは toolchain コンテナ内で実行される(ホストで go を直接実行しない)。`make test` は実 test DB `auth_test` を要する(SPEC-013。正規経路はルート `make migrate-test` で用意 → `REQUIRE_DB=1`。意味論の正は `.claude/rules/testing.md`)。

| 目的 | コマンド |
|---|---|
| format(チェック) | `make fmt-check` |
| format(自動修正) | `make fmt` |
| lint | `make lint` |
| type check 相当 | `make vet` && `make build` |
| test | `make test`(race 検査は `make test-race`) |
| 上記すべて | `make check` |
| 署名鍵リング生成 | `make auth-signing-keys`(`.secrets/auth-signing-keys.json` を生成。ISSUE-036。生成であり検査ではないため `make check` には含めない) |
| 署名鍵ローテーション | `make rotate-signing-keys`(旧 active 鍵を verify-only に降格し新 active 鍵を追加 = JWKS overlap 維持) |

## レイアウト

`app/api` と同一のレイヤ別トップディレクトリ構成を採る(`internal/` は用いない)。

- `cmd/authz/main.go` — エントリポイント(配線のみ。署名鍵リングのロード(`SIGNING_KEYS_FILE`。未設定時は ephemeral 生成)・リポジトリ seed・サーバ起動)。ほかに `cmd/keygen`(鍵リング生成 / `-rotate`)・`cmd/healthcheck`
- `domain/<aggregate>/` — ドメイン層(他層に非依存)。集約ごとにパッケージを切る(現状 `client` / `user` / `authcode` / `refreshtoken` / `consent` / `idpsession` の 6 集約 +(JWT 署名ポートのみで永続化しない)`token`)
- `service/` — アプリケーション層(ユースケース。ドメインを協調させる薄い層。authorization / authentication / consent / discovery / introspection / userinfo / admin)
- `infra/` — インフラ層(ドメインが宣言したポートの実装)。`postgres`(client / user / authcode / refreshtoken / consent の 5 リポジトリ。`pgx`)・`memory`(idpsession store。意図的に in-memory)・`jwt`(RSA 署名器 / 検証器 + multi-key リング。`file_loader` / `ephemeral_loader`)・`repotest`(共有ふるまい契約テスト)
- `route/` — プレゼンテーション層(HTTP ハンドラ・ルーティング・エラー変換。`templates/` に login / consent UI、cookie セッションは `session_cookie.go`)

## コーディング

`app/api` と同一(`api.md` のコーディング規約に準拠)。加えて:

- `context.Context` は第一引数で受け渡す。struct フィールドに保持しない
- エラーは `fmt.Errorf("...: %w", err)` でラップする。分岐したいエラーは sentinel / カスタム型 + `errors.Is` / `errors.As`
- 外部依存(署名鍵・リポジトリ)は interface(ドメインが宣言するポート)越しにし、テストで差し替え可能にする
- goroutine は終了条件(context cancel 等)を必ず持つ

## セキュリティ規約(認証基盤ゆえ厳守)

- 秘密鍵・クライアントシークレット・パスワードをコード・ドキュメントに直接埋め込まない。デモ用の値は起動時に生成 / seed し、本番は環境変数・外部から注入する前提にする
- ユーザーパスワードは平文で保持・比較しない。`bcrypt` ハッシュで保存し、`VerifyPassword` は `golang.org/x/crypto/bcrypt` の定数時間比較を用いる
- 認可コードは単回使用(consume 後は再利用不可)かつ短命。PKCE(S256)を必須とする
- リフレッシュトークンは平文を保存せず **SHA-256 ハッシュのみ**を永続化し、リフレッシュのたびにローテーション(旧トークン無効化)+ ローテ済みトークン再提示時の同一 family 一括失効(再利用/盗用検知)で public client を保護する(SPEC-006 / RFC 9700 §4.14)
- JWT は RS256 で署名し、`kid` 付きヘッダ + JWKS で公開鍵を配布する。`alg: none` を受理しない
- 署名鍵リング `.secrets/auth-signing-keys.json` は秘密鍵を含むため**絶対にコミットしない**(gitignore 済み)。ローテーションは旧 active 鍵を verify-only(公開鍵のみ)に降格して JWKS に重複期間を残す(ISSUE-036)
- IdP セッション cookie は常に HttpOnly + SameSite=Lax。Secure は issuer のスキームに追従する(https なら必須。ローカル http compose のみ false)
- introspect / confidential client の revoke はクライアント認証必須。admin API は API key 必須で、未設定なら経路自体を登録しない(fail-closed)
- ID Token / アクセストークンの検証では iss / aud / exp / 署名をすべて検証する
- 標準に沿ったエラー応答(OAuth: `error` / `error_description`、HTTP ステータス)を返し、内部情報を漏らさない

## 準拠する公式仕様

- OAuth 2.0 Authorization Framework(RFC 6749。認可コードグラント §4.1 / リフレッシュトークングラント §6)、Bearer Token 利用(RFC 6750)
- OAuth 2.0 Security Best Current Practice(RFC 9700。refresh token のローテーション + 再利用検知)
- PKCE(RFC 7636、`code_challenge_method=S256`)
- Token Revocation(RFC 7009)、Token Introspection(RFC 7662)
- JWT(RFC 7519)、JWS(RFC 7515、RS256)、JWK / JWK Set(RFC 7517)
- OpenID Connect Core 1.0(`prompt` / `max_age` / `auth_time` / `at_hash` を含む)、OpenID Connect Discovery 1.0、RP-Initiated Logout 1.0
