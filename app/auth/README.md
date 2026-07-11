# app/auth — OAuth 2.0 認可サーバー / OpenID Provider サンプル実装

`app/api`(タスク管理サンプル)と同一の DDD レイヤ構成を、OAuth 2.0 Authorization Code Grant +
PKCE(RFC 7636, S256)+ refresh_token グラント(RFC 6749 §6、SPEC-006)による OpenID Connect
認可サーバーを題材に実装したサンプル。プロトコル・暗号は Go 標準ライブラリのみで完結しており
(`crypto/rsa`・`crypto/sha256`・`crypto/rand`・`encoding/base64`・`encoding/json` 等)、
**永続化層 `infra/postgres` のみ Postgres ドライバ `pgx`(`github.com/jackc/pgx/v5`)に
依存する**(SPEC-005。DB 規約の正は `.claude/rules/db.md`)。

**これは DDD レイヤ構成とプロトコル実装を提示するための基盤サンプルであり、本番の IdP として
そのまま使うことは想定していない。** 特にログイン/同意画面が存在しない点(後述)に注意。

## レイヤ構成

```
                 ┌───────────┐
                 │  route    │  プレゼンテーション層 (HTTP ハンドラ / ルーティング / エラー変換)
                 └─────┬─────┘
                       │ 依存
                 ┌─────▼─────┐
                 │  service  │  アプリケーション層 (ユースケース: authorize / token(認可コード交換 + refresh_token グラント) / userinfo / discovery)
                 └─────┬─────┘
                       │ 依存
                 ┌─────▼─────┐
                 │  domain   │  ドメイン層 (client / user / authcode / refreshtoken / token の 5 集約)
                 └─────▲─────┘
                       │ 実装 (依存性逆転)
                 ┌─────┴─────┐
                 │  infra    │  インフラ層 (in-memory リポジトリ、Postgres リポジトリ(infra/postgres)、RSA 実装の JWT 署名/検証/JWK、共有ふるまい契約テスト infra/repotest)
                 └───────────┘
```

- 依存の向きは `route → service → domain` の一方向。`domain` は他のどの層にも依存しない。
- `domain/token` は JWT の署名・検証・JWK 公開を **ポート(`Signer` / `Verifier` /
  `KeyProvider` インターフェース)** として宣言するのみで、RSA による実装は持たない。
  `infra/jwt` がその実装を提供する(**依存性逆転の原則 / DIP**)。同様に `client` /
  `user` / `authcode` / `refreshtoken` の各 `Repository`(書き込みを持つ `authcode` /
  `refreshtoken` は SPEC-010 で `Reader`/`Writer` にも additive に分割)は **Postgres 一本化
  (SPEC-011): `infra/postgres` が唯一の実装**。別ストア(例: MySQL)へ差し替える場合は
  `infra/postgres` を同じポート contract を満たす新実装で置き換え、`infra/repotest` の
  共有契約テストを回して動作を保証する。
- `cmd/authz` はコンポジションルート。RSA 鍵生成・Postgres への永続化配線・デモ client/user の
  seed・各層の配線を行うのみで、ビジネスロジックを一切持たない。接続 env・CQRS reader/writer 2
  プールの詳細は `.claude/rules/db.md` が正(担当は impl-db)。

### 集約間の関係(パッケージ依存を作らない設計)

`client` / `user` / `authcode` / `refreshtoken` / `token` の 5 集約は**互いにパッケージ
依存しない**。たとえば `authcode.AuthorizationCode` は発行先クライアントやリソースオーナーを
`client.ClientID` / `user.UserID` 型ではなく、`authcode` パッケージ自身が定義する
文字列相当の値オブジェクト(`authcode.ClientID` / `authcode.UserID` /
`authcode.RedirectURI`)で保持する。`refreshtoken.RefreshToken` も同じ設計を踏襲し、
`client` / `user` パッケージを import せず、自前の値オブジェクト(`refreshtoken.ClientID` /
`refreshtoken.UserID`)で発行先クライアント・リソースオーナーを保持する(SPEC-006)。
スコープの「要求 vs 許可」の突合や、client・user・authcode・refreshtoken・token の協調は
すべて **アプリケーション層(`service`)** が担う。

## DDD 戦術的パターンと本実装の対応

| パターン | 説明 | 本実装での対応 |
|---|---|---|
| 集約ルート (Aggregate Root) | 不変条件を守る単位。外部からは集約ルート経由でのみ操作 | `client.Client` / `user.User` / `authcode.AuthorizationCode` / `refreshtoken.RefreshToken` / (ポートのみの)`token` |
| 値オブジェクト (Value Object) | 属性の値そのもので同一性が決まる不変オブジェクト | `client.ClientID` / `client.RedirectURI`、`user.UserID` / `user.Username` / `user.Profile`、`authcode.Code` / `authcode.CodeChallenge` / `authcode.Scope` / `authcode.Nonce`、`refreshtoken.Token` / `refreshtoken.TokenHash` / `refreshtoken.FamilyID` / `refreshtoken.ClientID` / `refreshtoken.UserID` / `refreshtoken.Scope`、`token.Claims` |
| ファクトリ (Factory) | 不変条件の充足をカプセル化する生成手段 | 各集約の `New(...)`(新規生成)/ `Reconstruct(...)`(永続化層からの再構築専用)。`refreshtoken` はローテーション専用に `Issue(...)`(新規発行。認可コード交換時)/ `Rotate(...)`(リフレッシュ時の次トークン生成)も持つ |
| リポジトリ (Repository) | 集約の永続化をコレクションのように抽象化する境界 | `client.Repository` / `user.Repository` / `authcode.Repository` / `refreshtoken.Repository`(定義はドメイン層、実装は `infra/postgres`。SPEC-010 で `authcode` / `refreshtoken` は additive に `Reader`/`Writer` へも分割) |
| ポート (Port) / ヘキサゴナル | ドメインが必要とする外部機能をインターフェースとして宣言し、実装は外側に置く | `token.Signer` / `token.Verifier` / `token.KeyProvider`(定義は `domain/token`、RSA 実装は `infra/jwt`) |
| アプリケーションサービス (Application Service) | ユースケースを実現するためドメインを協調させる薄い層。ビジネスルールは持たない | `service.AuthorizationService`(authorize / token: 認可コード交換 + refresh_token グラント)、`service.UserInfoService`、`service.DiscoveryService` |
| ドメインエラー | sentinel error による分岐可能なエラー表現 | 各 `domain/<aggregate>/errors.go`。`errors.Is` で判定 |

その他の設計判断:

- アプリケーション層はドメインオブジェクトを直接返さず、DTO(`AuthorizeResult` /
  `TokenResponse` / `UserInfoDTO` / `ProviderMetadata` / `JWKSet`)に変換してから返す
  (`service/dto.go`)。
- `service.AuthorizationService.Token` は `grant_type` で分岐し、`authorization_code`
  (認可コード交換。対応 client には access/ID token に加え refresh token も同時発行する)と
  `refresh_token`(SPEC-006: ローテーション + 同一 family 一括失効による再利用検知)の
  2 グラントを実装する(`service/authorization_service.go` の `authorizationCodeGrant` /
  `refreshTokenGrant`)。
- `route/response.go` にドメインエラー → OAuth エラーコード(`invalid_request` /
  `invalid_client` / `invalid_grant` / `invalid_scope` / `unsupported_response_type` /
  `unsupported_grant_type`)+ HTTP ステータスの変換を集約している。

## ディレクトリ

```
app/auth/
├── cmd/
│   ├── authz/main.go            コンポジションルート(RSA 鍵生成・永続化選択+配線・seed・配線)
│   └── healthcheck/main.go      コンテナ HEALTHCHECK 用プローブ(シェルの無い distroless イメージ向け)
├── domain/
│   ├── client/                   集約: 登録済み OAuth クライアント
│   ├── user/                     集約: リソースオーナー(OIDC subject)
│   ├── authcode/                  集約: 認可コード(単回使用・短命・PKCE)
│   ├── refreshtoken/              集約: refresh token(単回使用ローテーション + family 単位の再利用検知。SPEC-006)
│   └── token/                     ポート: JWT 署名/検証/JWK(実装なし)
├── service/                     アプリケーション層(4 ユースケース)
│   ├── dto.go
│   ├── authorization_service.go   authorize / token(認可コード交換 + refresh_token グラント)
│   ├── userinfo_service.go
│   └── discovery_service.go
├── infra/
│   ├── postgres/                  Postgres Repository 実装(client/user/authcode/refreshtoken。SPEC-005。SPEC-010 で reader/writer 2 プールへ配線)
│   ├── repotest/                  ふるまい契約テスト(`Run<集約>RepositoryContract`。Postgres 実装に対して integration tag で実行)
│   └── jwt/                       RS256 の Signer/Verifier/KeyProvider 実装
├── route/                       プレゼンテーション層
│   ├── router.go
│   ├── authorize_handler.go
│   ├── token_handler.go
│   ├── userinfo_handler.go
│   ├── discovery_handler.go
│   └── response.go
├── db/
│   ├── migrations/                goose マイグレーション SQL(up/down)
│   └── queries/                   sqlc の入力クエリ SQL
├── sqlc.yaml                    sqlc 設定(`db/queries` → `infra/postgres/sqlcgen` を生成)
├── Makefile                     開発コマンド(下記「開発コマンド」参照)
├── Dockerfile                   コンテナイメージ定義
└── .golangci.yml                lint 設定
```

永続化(`infra/postgres` / `db/*` / `sqlc.yaml` / マイグレーション適用)の規約・接続 env・
CQRS reader/writer 分離の詳細は `.claude/rules/db.md` が正で、本 README では重複させない
(担当は impl-db)。

## 開発コマンド

汎用コマンドは `Makefile` にまとめている(`make help` で一覧表示)。`app/api/Makefile` と
同一構成(`run` のみ対象バイナリが異なる)。**SPEC-009 により、下表のコマンドはホストに
go / golangci-lint 等を入れず、単一の toolchain コンテナ内で実行される**(ホストの前提は
Docker のみ)。コマンド名・契約はこの配線変更の前後で不変。

| ターゲット | 内容 |
|---|---|
| `make fmt` | フォーマット自動修正(gofmt + goimports) |
| `make fmt-check` | フォーマット差分チェック |
| `make lint` | golangci-lint |
| `make vet` / `make build` | go vet / go build |
| `make test` / `make test-race` | テスト実行(race detector 付きは test-race) |
| `make check` | 上記チェックを一括実行(fmt-check + lint + vet + build + test) |
| `make run` | 認可サーバー起動 |

上記に加え、永続化(DB)系のターゲット(`make sqlc` / `make migrate-create` /
`make test-integration`)がある。これらは生成 / 実 DB 依存であり検査ではないため
`make check` には含めない。規約・実行契約・接続 env・マイグレーション適用(`app/migrator`)
の詳細は `.claude/rules/db.md` が正(担当は impl-db)。

## 起動方法

```sh
cd app/auth
make run  # または go run ./cmd/authz
```

デフォルトでは `:8080` で待ち受け、issuer は `http://localhost:8080` になる。
`PORT` / `ISSUER` 環境変数で変更できる。

```sh
PORT=9000 ISSUER=https://auth.example.com go run ./cmd/authz
```

`Ctrl+C`(SIGINT)または SIGTERM で graceful shutdown する。

### 重要な運用上の注意

- **issuer は本番では https 必須。** OIDC Discovery 1.0 上、issuer(`iss`)は https の URL
  でなければならない。この基盤はローカル開発の利便性のため `ISSUER` 未設定時に
  `http://localhost:8080` を既定値として使うが、本番相当の環境にデプロイする場合は
  `ISSUER` に https の URL を必ず注入すること(`cmd/authz/main.go` の `run()` 内コメント参照)。
- **Postgres 必須(fail-closed)。** `DB_HOST` / `DB_NAME` / `DB_USER` / `DB_PASSWORD` が
  設定されていない場合は起動時にエラーで終了する(SPEC-011: in-memory フォールバックは
  廃止。`cmd/authz/env.go` の `validate()` が起動前に必須フィールドを検証する)。
  接続 env(writer 用 `DB_*` に加え、CQRS reader/writer 分離(SPEC-010)用の reader 用
  `DB_READER_*`。各項目は未設定時に対応する writer 値へフォールバックする)の一覧・既定値・
  マイグレーション適用(`app/migrator`)は `.claude/rules/db.md` の「接続 env 契約」が正。
- **RSA 秘密鍵はプロセス起動のたびにメモリ上で生成する。** ディスクにもコードにも書き出さない
  代わりに、**再起動すると鍵(と `kid`)が変わり、それ以前に発行したトークンはすべて検証不能
  になる。** 鍵の永続化・ローテーション・複数鍵での JWKS 提供は将来拡張点(未実装)。
- **ログイン/同意画面は実装していない。** `/authorize` は `login_hint` があれば seed 済みの
  ユーザ名と突合し、一致しなければ既定ユーザ(`demo-user`)を自動的にリソースオーナーとして
  扱う(`service/authorization_service.go` の `resolveOwner` を参照)。実際の IdP では、ここで
  ユーザ認証(ログインフォーム)とスコープ同意(同意画面)を挟み、承認が得られてから認可コード
  を発行する必要がある。**差し込み位置は `route/authorize_handler.go` の `handle` 冒頭コメント、
  および `service/authorization_service.go` の `resolveOwner` 関数のコメントに明示している。**

## Seed されるデモデータ

`cmd/authz/main.go` が起動時に以下を自動登録する(client_id・redirect_uri・ユーザ名・sub は
公開情報として固定値。RSA 秘密鍵とデモユーザのパスワードのみ起動時にランダム生成する秘密情報):

| 項目 | 値 |
|---|---|
| client_id | `demo-client` |
| redirect_uri | `http://localhost:3000/callback`(登録済みの 1 件のみ) |
| 許可スコープ | `openid` `profile` `email` |
| response_type / grant_type | `code` / `authorization_code`, `refresh_token`(public client。`token_endpoint_auth_methods_supported=["none"]`) |
| ユーザ名(username) | `demo-user` |
| sub(UserID) | `demo-user-id` |
| name / email | `Demo User` / `demo-user@example.com` |
| パスワード | 起動時にランダム生成(非公開。現状のフローでは未使用) |

## API 一覧

| メソッド | パス | 説明 |
|---|---|---|
| GET | `/authorize` | 認可エンドポイント(RFC 6749 4.1.1)。成功時は `redirect_uri` へ 302 |
| POST | `/token` | トークンエンドポイント(RFC 6749 4.1.3)。`application/x-www-form-urlencoded` |
| GET | `/userinfo` | UserInfo エンドポイント(OIDC Core 5.3)。`Authorization: Bearer <access_token>` |
| GET | `/.well-known/openid-configuration` | Discovery メタデータ(OIDC Discovery 1.0)。`grant_types_supported` は `["authorization_code", "refresh_token"]`(SPEC-006 R9) |
| GET | `/.well-known/jwks.json` | 公開鍵の JWK Set(RFC 7517) |

`POST /token` は `grant_type=authorization_code`(認可コード交換)と
`grant_type=refresh_token`(SPEC-006。リフレッシュ)の 2 グラントに対応する。

### `/token` のエラー例

| ステータス | error | 条件 |
|---|---|---|
| 400 | `invalid_request` | 必須パラメータ欠落・client_id 不正 |
| 400 | `invalid_client` | client_id が未登録 |
| 400 | `invalid_grant` | 認可コードが不正・失効・再利用・redirect_uri/client 不一致・PKCE 不一致。または refresh_token が不正・失効・再利用(ローテ済みトークンの再提示)・発行先 client と不一致。またはコード/トークンに紐づくユーザーが存在しない |
| 400 | `invalid_scope` | `grant_type=refresh_token` で要求 scope が元の付与 scope を超える(拡大) |
| 400 | `unsupported_grant_type` | `grant_type` が `authorization_code` / `refresh_token` 以外 |
| 500 | `server_error` | 予期しない内部エラー(詳細はレスポンスに含めずログへ出力) |

`/token` のレスポンス(成功・失敗いずれも)には `Cache-Control: no-store` と `Pragma: no-cache`
を必ず付与する(RFC 6749 5.1)。

**`code_verifier` / redirect_uri / client の不一致など、`/token` での検証に失敗した場合、
認可コードは消費されずそのまま残り、正しい値での再試行が可能。** これは意図的な設計であり、
`code` の値だけを盗み見た(あるいは推測した)攻撃者が誤ったリクエストを 1 回送るだけで、
正規クライアントによる正常な交換を妨害できてしまう DoS を避けるため。消費(ストアからの削除
による単回使用の確定)は交換が**成功した場合のみ**、リポジトリの単一ロック区間内でアトミックに
行われる(`infra/memory/authcode_repository.go` の `Consume` を参照)。

**ローテ済み(消費済み)の refresh_token が再提示された場合、サーバーは同一 family(発行元の
`Issue` から続くローテーション連鎖)の refresh token をすべて失効させたうえで
`invalid_grant` を返す。** これは RFC 9700 §4.14 の再利用/盗用検知の応答で、盗まれたトークン
が繰り返し新しいトークンを生み出すのを防ぐ(`service/authorization_service.go` の
`refreshTokenGrant` を参照)。

## OIDC / PKCE フロー(curl 例)

PKCE は **S256 のみ受理**(`plain` は `invalid_request`)。`code_verifier` は
RFC 7636 4.1 の unreserved 文字集合・43〜128 文字を満たす必要がある。

```sh
BASE=http://localhost:8080

# 1. PKCE の code_verifier / code_challenge (S256) を生成する
CODE_VERIFIER=$(openssl rand -base64 96 | tr -d '=+/\n' | cut -c1-64)
CODE_CHALLENGE=$(printf '%s' "$CODE_VERIFIER" \
  | openssl dgst -binary -sha256 \
  | openssl base64 \
  | tr -d '=' | tr '+/' '-_')

# 2. /authorize にリクエスト(login_hint なし=既定の demo-user が自動的にリソースオーナーになる)
#    302 Location ヘッダから code を取り出す(ヘッダのみ取得し、リダイレクト先へは追従しない)
LOCATION=$(curl -s -D - -o /dev/null -G "$BASE/authorize" \
  --data-urlencode "response_type=code" \
  --data-urlencode "client_id=demo-client" \
  --data-urlencode "redirect_uri=http://localhost:3000/callback" \
  --data-urlencode "scope=openid profile email" \
  --data-urlencode "state=xyz123" \
  --data-urlencode "nonce=nonce-abc" \
  --data-urlencode "code_challenge=$CODE_CHALLENGE" \
  --data-urlencode "code_challenge_method=S256" \
  | grep -i '^location:' | tr -d '\r')

CODE=$(echo "$LOCATION" | sed -n 's/.*[?&]code=\([^&]*\).*/\1/p')
echo "issued code: $CODE"

# 3. /token でコードをトークンに交換する
TOKEN_RESPONSE=$(curl -s -X POST "$BASE/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=authorization_code" \
  --data-urlencode "code=$CODE" \
  --data-urlencode "redirect_uri=http://localhost:3000/callback" \
  --data-urlencode "client_id=demo-client" \
  --data-urlencode "code_verifier=$CODE_VERIFIER")
echo "$TOKEN_RESPONSE"

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')

# 4. /userinfo をアクセストークンで呼び出す
curl -s "$BASE/userinfo" -H "Authorization: Bearer $ACCESS_TOKEN"

# 5. Discovery / JWKS
curl -s "$BASE/.well-known/openid-configuration"
curl -s "$BASE/.well-known/jwks.json"

# 6. grant_type=refresh_token でアクセストークンを更新する
#    (demo-client は grant_type=refresh_token に対応済み。旧 refresh_token はローテーションに
#    より無効化され、レスポンスの refresh_token には新しいトークンが返る)
REFRESH_TOKEN=$(echo "$TOKEN_RESPONSE" | sed -n 's/.*"refresh_token":"\([^"]*\)".*/\1/p')

REFRESH_RESPONSE=$(curl -s -X POST "$BASE/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=refresh_token" \
  --data-urlencode "refresh_token=$REFRESH_TOKEN" \
  --data-urlencode "client_id=demo-client")
echo "$REFRESH_RESPONSE"
```

(`jq` が利用可能であれば `| jq .` を末尾に付けると読みやすい。)

## トークン設計

- access token / ID token とも **RS256 署名の JWT** で、`base64url(header).base64url(payload).base64url(signature)`
  の compact 形式(`encoding/base64` の `RawURLEncoding`)。ヘッダは
  `{"alg":"RS256","typ":"JWT","kid":"..."}`。`kid` は RFC 7638 の JWK Thumbprint。
- **`Verifier` は `alg` が厳密に `RS256` であることを確認し、それ以外(`none` を含む)は
  すべて拒否する。** これはアルゴリズム混同攻撃(署名検証をすり抜けるために `alg` を
  `none` や対称鍵アルゴリズムへ差し替える攻撃)への対策(`infra/jwt/verifier.go`)。
- ID Token の必須クレーム: `iss` / `sub` / `aud`(= `client_id`)/ `exp` / `iat`。リクエストに
  `nonce` があれば ID Token にもそのまま反映する。
- **access token の `aud` は issuer(自 UserInfo エンドポイント)とする設計。** 本基盤では
  リソースサーバを自身の `/userinfo` のみと見なし、access token の audience を issuer に
  固定している。`UserInfoService` はこの `aud` を検証する。外部リソースサーバを追加する場合は
  audience 設計を見直す必要がある(`service/authorization_service.go` の `Token` 内コメント参照)。
- **refresh token 設計(SPEC-006)**: access/ID token とは異なり自己完結した JWT ではなく、
  `crypto/rand` で生成した 256bit(32byte)を base64url エンコードした**不透明(opaque)な
  ランダム文字列**(`domain/refreshtoken.Token`)。DB には平文を保存せず
  **SHA-256(`crypto/sha256`)ハッシュのみ**を保存し(`Token.Hash()` / `TokenHash`)、平文は
  発行(認可コード交換)時・ローテーション時のレスポンスで一度だけ返す。サーバー自身は平文を
  保持しない(R8)。
  - **ローテーション**: リフレッシュのたびに旧トークンを consumed としてマークし新トークンを
    発行する。旧トークンの consume と新トークンの insert は `Repository.Rotate` の単一の
    atomic 操作内で行われる(R4)。
  - **再利用検知**: 消費済み(ローテ済み)のトークンが再提示されたら、発行元の `Issue` から
    続くローテーション連鎖(`family_id`)の refresh token をすべて `RevokeFamily` で失効させ、
    `invalid_grant` を返す(RFC 9700 §4.14、R5)。
  - **client バインディング**: refresh token は発行先 client に紐付き、別 client からの提示は
    `invalid_grant` で拒否する(R6)。
  - **scope 縮小のみ**: リフレッシュ時に要求できる scope は元の付与 scope の**部分集合のみ**
    (拡大は `invalid_scope`。省略時は元の scope を維持する。R7)。
  - **TTL**: 30 日、ローテーションのたびにスライディングでリセットされる
    (`refreshtoken.RefreshTokenTTL`)。

## 将来拡張点(未実装)

- ログイン/同意 UI(現状は自動割り当て。上記「重要な運用上の注意」参照)
- confidential client(`client_secret` 認証)。現状は public client + PKCE のみ
  (`token_endpoint_auth_methods_supported=["none"]`)
- `offline_access` スコープによる refresh token 発行のゲート・同意画面(OIDC Core 11)。
  現状は `grant_type=refresh_token` に対応した client には常に refresh token を発行する
  (SPEC-006 のスコープ外)
- トークン失効エンドポイント(RFC 7009 `/revoke`)。現状の失効経路は再利用検知による自動
  family 失効のみ
- 送信者制約トークン(mTLS RFC 8705 / DPoP RFC 9449)。public client の refresh token 保護は
  ローテーション + 再利用検知(RFC 9700 §4.14)で満たしている
- RSA 鍵の永続化・ローテーション・複数鍵での JWKS 配布
- 認可コード・consumed 済み refresh token の期限切れエントリの定期一括掃除。単回使用+失効
  検証(参照時の lazy eviction)で安全性自体は担保済みだが、正規のローテーションフローでは
  二度と参照されない consumed 行が Postgres 永続化下では恒久的に残存する
  (deferred hardening として **ISSUE-019**(open / low)に記録済み)
