# app/auth — OAuth 2.0 認可サーバー / OpenID Provider サンプル実装

`app/api`(タスク管理サンプル)と同一の DDD レイヤ構成を、OAuth 2.0 Authorization Code Grant +
PKCE(RFC 7636, S256)による OpenID Connect 認可サーバーを題材に実装したサンプル。
Go 標準ライブラリのみで構成しており(`crypto/rsa`・`crypto/sha256`・`encoding/base64`・
`encoding/json` 等)、外部依存は一切持たない(`go.mod` に `require` なし)。

**これは DDD レイヤ構成とプロトコル実装を提示するための基盤サンプルであり、本番の IdP として
そのまま使うことは想定していない。** 特にログイン/同意画面が存在しない点(後述)に注意。

## レイヤ構成

```
                 ┌───────────┐
                 │  route    │  プレゼンテーション層 (HTTP ハンドラ / ルーティング / エラー変換)
                 └─────┬─────┘
                       │ 依存
                 ┌─────▼─────┐
                 │  service  │  アプリケーション層 (ユースケース: authorize / token / userinfo / discovery)
                 └─────┬─────┘
                       │ 依存
                 ┌─────▼─────┐
                 │  domain   │  ドメイン層 (client / user / authcode / token の 4 集約)
                 └─────▲─────┘
                       │ 実装 (依存性逆転)
                 ┌─────┴─────┐
                 │  infra    │  インフラ層 (in-memory リポジトリ、RSA 実装の JWT 署名/検証/JWK)
                 └───────────┘
```

- 依存の向きは `route → service → domain` の一方向。`domain` は他のどの層にも依存しない。
- `domain/token` は JWT の署名・検証・JWK 公開を **ポート(`Signer` / `Verifier` /
  `KeyProvider` インターフェース)** として宣言するのみで、RSA による実装は持たない。
  `infra/jwt` がその実装を提供する(**依存性逆転の原則 / DIP**。`app/api` の
  `domain/task.Repository` ⇔ `infra/memory.TaskRepository` と同型)。
- `cmd/authz` はコンポジションルート。RSA 鍵生成・リポジトリ構築・デモ client/user の
  seed・各層の配線を行うのみで、ビジネスロジックを一切持たない。

### 集約間の関係(パッケージ依存を作らない設計)

`client` / `user` / `authcode` / `token` の 4 集約は**互いにパッケージ依存しない**。
たとえば `authcode.AuthorizationCode` は発行先クライアントやリソースオーナーを
`client.ClientID` / `user.UserID` 型ではなく、`authcode` パッケージ自身が定義する
文字列相当の値オブジェクト(`authcode.ClientID` / `authcode.UserID` /
`authcode.RedirectURI`)で保持する。スコープの「要求 vs 許可」の突合や、
client・user・authcode・token の協調はすべて **アプリケーション層(`service`)** が担う。

## DDD 戦術的パターンと本実装の対応

| パターン | 説明 | 本実装での対応 |
|---|---|---|
| 集約ルート (Aggregate Root) | 不変条件を守る単位。外部からは集約ルート経由でのみ操作 | `client.Client` / `user.User` / `authcode.AuthorizationCode` / (ポートのみの)`token` |
| 値オブジェクト (Value Object) | 属性の値そのもので同一性が決まる不変オブジェクト | `client.ClientID` / `client.RedirectURI`、`user.UserID` / `user.Username` / `user.Profile`、`authcode.Code` / `authcode.CodeChallenge` / `authcode.Scope` / `authcode.Nonce`、`token.Claims` |
| ファクトリ (Factory) | 不変条件の充足をカプセル化する生成手段 | 各集約の `New(...)`(新規生成)/ `Reconstruct(...)`(永続化層からの再構築専用) |
| リポジトリ (Repository) | 集約の永続化をコレクションのように抽象化する境界 | `client.Repository` / `user.Repository` / `authcode.Repository`(定義はドメイン層、実装は `infra/memory`) |
| ポート (Port) / ヘキサゴナル | ドメインが必要とする外部機能をインターフェースとして宣言し、実装は外側に置く | `token.Signer` / `token.Verifier` / `token.KeyProvider`(定義は `domain/token`、RSA 実装は `infra/jwt`) |
| アプリケーションサービス (Application Service) | ユースケースを実現するためドメインを協調させる薄い層。ビジネスルールは持たない | `service.AuthorizationService`(authorize/token)、`service.UserInfoService`、`service.DiscoveryService` |
| ドメインエラー | sentinel error による分岐可能なエラー表現 | 各 `domain/<aggregate>/errors.go`。`errors.Is` で判定 |

その他の設計判断:

- アプリケーション層はドメインオブジェクトを直接返さず、DTO(`AuthorizeResult` /
  `TokenResponse` / `UserInfoDTO` / `ProviderMetadata` / `JWKSet`)に変換してから返す
  (`service/dto.go`)。
- `route/response.go` にドメインエラー → OAuth エラーコード(`invalid_request` /
  `invalid_client` / `invalid_grant` / `unsupported_response_type` /
  `unsupported_grant_type` / `invalid_scope`)+ HTTP ステータスの変換を集約している。

## ディレクトリ

```
app/auth/
├── cmd/authz/main.go          コンポジションルート(RSA 鍵生成・seed・配線)
├── domain/
│   ├── client/                 集約: 登録済み OAuth クライアント
│   ├── user/                   集約: リソースオーナー(OIDC subject)
│   ├── authcode/                集約: 認可コード(単回使用・短命・PKCE)
│   └── token/                   ポート: JWT 署名/検証/JWK(実装なし)
├── service/                   アプリケーション層(4 ユースケース)
│   ├── dto.go
│   ├── authorization_service.go   authorize / token
│   ├── userinfo_service.go
│   └── discovery_service.go
├── infra/
│   ├── memory/                  in-memory Repository 実装 (client/user/authcode)
│   └── jwt/                     RS256 の Signer/Verifier/KeyProvider 実装
└── route/                     プレゼンテーション層
    ├── router.go
    ├── authorize_handler.go
    ├── token_handler.go
    ├── userinfo_handler.go
    ├── discovery_handler.go
    └── response.go
```

## 開発コマンド

汎用コマンドは `Makefile` にまとめている(`make help` で一覧表示)。`app/api/Makefile` と
完全に同一構成(`run` のみ対象バイナリが異なる)。

| ターゲット | 内容 |
|---|---|
| `make fmt` | フォーマット自動修正(gofmt + goimports) |
| `make fmt-check` | フォーマット差分チェック |
| `make lint` | golangci-lint |
| `make vet` / `make build` | go vet / go build |
| `make test` / `make test-race` | テスト実行(race detector 付きは test-race) |
| `make check` | 上記チェックを一括実行(fmt-check + lint + vet + build + test) |
| `make run` | 認可サーバー起動 |

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
| response_type / grant_type | `code` / `authorization_code`(public client。`token_endpoint_auth_methods_supported=["none"]`) |
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
| GET | `/.well-known/openid-configuration` | Discovery メタデータ(OIDC Discovery 1.0) |
| GET | `/.well-known/jwks.json` | 公開鍵の JWK Set(RFC 7517) |

### `/token` のエラー例

| ステータス | error | 条件 |
|---|---|---|
| 400 | `invalid_request` | 必須パラメータ欠落・client_id 不正 |
| 400 | `invalid_client` | client_id が未登録 |
| 400 | `invalid_grant` | 認可コードが不正・失効・再利用・redirect_uri/client 不一致・PKCE 不一致 |
| 400 | `unsupported_grant_type` | `grant_type` が `authorization_code` 以外 |
| 500 | `server_error` | 予期しない内部エラー(詳細はレスポンスに含めずログへ出力) |

`/token` のレスポンス(成功・失敗いずれも)には `Cache-Control: no-store` と `Pragma: no-cache`
を必ず付与する(RFC 6749 5.1)。

**`code_verifier` / redirect_uri / client の不一致など、`/token` での検証に失敗した場合、
認可コードは消費されずそのまま残り、正しい値での再試行が可能。** これは意図的な設計であり、
`code` の値だけを盗み見た(あるいは推測した)攻撃者が誤ったリクエストを 1 回送るだけで、
正規クライアントによる正常な交換を妨害できてしまう DoS を避けるため。消費(ストアからの削除
による単回使用の確定)は交換が**成功した場合のみ**、リポジトリの単一ロック区間内でアトミックに
行われる(`infra/memory/authcode_repository.go` の `Consume` を参照)。

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

## 将来拡張点(未実装)

- ログイン/同意 UI(現状は自動割り当て。上記「重要な運用上の注意」参照)
- confidential client(`client_secret` 認証)。現状は public client + PKCE のみ
  (`token_endpoint_auth_methods_supported=["none"]`)
- refresh token(`grant_type=refresh_token`)。`offline_access` スコープ未対応
- RSA 鍵の永続化・ローテーション・複数鍵での JWKS 配布
- 認可コードの期限切れエントリの自動掃除(単回使用+失効検証で安全性自体は担保済み)
