# AUTH-001 実装計画: OAuth 2.0 認可サーバー / OpenID Provider 基盤サンプル(app/auth)

> **起点 Spec/Issue なし。`app/api` と同じくアーキテクチャ提示目的の基盤サンプル。**
> 本計画は特定の機能要求ではなく「公式ドキュメント(OAuth 2.0 / OIDC Core / RFC 7636 PKCE / RFC 7517 JWK / OIDC Discovery)に沿った、そのまま拡張可能な認可サーバー基盤」を Go 標準ライブラリのみで提示することを目的とする。要件は「サポートするフロー」節のエンドポイント仕様と「公式仕様の確定事実」節の遵守事項をもって代替する(下記トレーサビリティ表で実装/テストに対応付ける)。

- 対象 stack: `app/auth`(Go・新規モジュール)。他 stack(`app/api` / `app/web` / `app/iac`)のコードは変更しない
- 成果物: `app/auth` 配下の Go ソース一式 + `go.mod` + `Makefile` + `README.md`(`app/api` と同一のレイヤ構成・実装スタイル)
- 参照モジュール: `app/api`(module `github.com/srrrs-7/cc-orchestrator/app/api`, go 1.24)の DDD レイヤ構成・実装スタイルを厳密に踏襲する
- 新モジュール: `github.com/srrrs-7/cc-orchestrator/app/auth`, go 1.24, **外部依存ゼロ**

---

## 方針

### 採用アプローチ

`app/api` と**完全に同一の Eric Evans DDD レイヤ構成**を踏襲する。依存方向は `route → service → domain`、`infra → domain`(DIP)。`cmd/authz/main.go` はコンポジションルート(配線のみ)。標準ライブラリのみで OAuth 2.0 Authorization Code Grant + PKCE(S256)による OIDC 認証基盤を実装する。

1. **認可フローは Authorization Code Grant + PKCE(S256 必須)に限定**する。エンドポイントは 5 つ:
   - `GET /authorize`(認可)/ `POST /token`(トークン)/ `GET /userinfo`(UserInfo)/ `GET /.well-known/openid-configuration`(Discovery)/ `GET /.well-known/jwks.json`(JWK Set)。
2. **JWT(access token / ID token)は RS256 を stdlib で自作**する。署名 = RSASSA-PKCS1-v1_5 + SHA-256(`crypto/rsa.SignPKCS1v15` / `VerifyPKCS1v15`, `crypto/sha256`)。コンパクト JWT は `base64url(header).base64url(payload).base64url(signature)`(`encoding/base64` の `RawURLEncoding`, `encoding/json`)。PKCE S256 = `RawURLEncoding(SHA256(ASCII(code_verifier)))`。RSA 鍵は**起動時に `crypto/rsa.GenerateKey`(2048bit)で生成**し、秘密鍵はプロセス内メモリのみに保持(ディスク・コードに書かない)。
3. **エンドユーザ認証は基盤簡略化として seed デモユーザへ自動割り当て**る。`/authorize` は `login_hint` があれば seed ユーザ名(`Username`)と突合し、無ければ既定ユーザを resource owner とする。**本来ログイン画面・同意画面を差し込む箇所**であることを、`authorize_handler.go` のコメントと `README.md` に明示する。
4. **デモ用の client / user / RSA 鍵は起動時に生成・seed** する。秘密情報(RSA 秘密鍵・クライアントシークレット・パスワード)はコード/ドキュメント/リポジトリに直書きしない(`project.md`)。スコープは基盤として `openid`(必須)+ `profile` + `email` に限定する。主軸は **public client + PKCE**。confidential client のシークレット検証は基盤では実装しない(将来拡張点として README とリスク欄に記載)。
5. **`issuer` は環境変数 `ISSUER`(既定 `http://localhost:8080`)から取得**する。OIDC 上 issuer は https 必須だが、ローカル基盤の都合で http 既定とし、本番は https を注入する前提であることを Discovery ハンドラのコメントと README に明記する。ポートは `app/api` と同様 `PORT`(既定 `8080`)で受ける。

### 退けた代替案

| 案 | 退けた理由 |
|---|---|
| Resource Owner Password Credentials (ROPC) grant | OAuth 2.1 / OIDC のベストプラクティスで非推奨。生パスワードを client に晒す。基盤サンプルとして誤った手本になるため Auth Code + PKCE を採用 |
| Implicit grant(`response_type=token`/`id_token`) | トークンが URL フラグメントに露出し非推奨。`response_type=code` のみ対応 |
| 外部 JWT ライブラリ(`golang-jwt` 等)の利用 | `app/api` の「stdlib のみ・外部依存ゼロ」方針に反する。RS256 の署名/検証・JWK 生成は `crypto/rsa`・`crypto/sha256`・`encoding/base64`・`encoding/json` で完結できるため自作する |
| 内蔵ログイン/同意画面(HTML フォーム + セッション) | 認証・セッション管理は本基盤のスコープ外で、DDD レイヤ提示という目的をぼかす。seed ユーザ自動認証に簡略化し、差し込み位置をコメント/README で明示する方が意図が伝わる |
| PKCE `plain` メソッドの許可 | RFC 7636 / OAuth 2.1 は S256 を要求。`CodeChallengeMethod` 型には `plain` を値として持たせ検証ロジックも実装するが(型の完全性のため)、`/authorize` は S256 のみ受理し `plain` 指定は `invalid_request` で拒否する。Discovery の `code_challenge_methods_supported` は `["S256"]` |
| RSA 鍵をファイル/tfvars から読み込み | 秘密情報の永続化・混入リスク。基盤サンプルでは起動時にメモリ上で生成する方が安全かつ自己完結。鍵ローテーション/永続化は将来拡張点(リスク欄) |
| 認可コードを JWT(自己記述)で表現 | 認可コードは**不透明・単回使用・サーバ側で失効管理**すべき(RFC 6749)。ランダム不透明値 + in-memory リポジトリで consumed / expiresAt を管理する |
| 時刻を関数引数で注入(`Verify(now, ...)`) | `app/api` の集約は内部で `time.Now()` を用いる。踏襲し、`expiresAt` は生成時に `time.Now()+TTL` で確定させ、`IsExpired()` は集約内で `time.Now()` と比較する(テストは TTL 経過・未経過を境界で検証) |
| Scope を独立パッケージ or client パッケージに配置 | 集約間パッケージ依存を避ける。admin 設計どおり `Scope` VO は `domain/authcode` に置き、`client` は「許可スコープ」を素の文字列集合で保持する。requested と allowed の突合はアプリ層(`authorization_service`)で行う(下記「集約間の関係」参照) |

### 集約間の関係(パッケージ依存を作らない設計)

- 4 集約(`client` / `user` / `authcode` / `token`)は**相互にパッケージ依存しない**。集約をまたぐ参照は ID(文字列)と値で表現する(例: `AuthorizationCode` は `clientID` / `userID` を文字列相当の VO で保持し、`client.Client` 型は参照しない)。
- スコープの「要求 vs 許可」突合、client・user・authcode・token の協調はすべて**アプリケーション層(`service`)が担う**。ドメイン層は自集約の不変条件のみを守る。
- `token`(Signer/Verifier/JWK)は**ポート(interface をドメイン層で宣言)**であり、`infra/jwt` が RSA 実装を提供する(`var _ token.Signer = (*Signer)(nil)` で契約を静的検証、`app/api` の `var _ task.Repository = (*TaskRepository)(nil)` と同型)。

---

## 変更ファイル(すべて新規作成、`app/auth` 配下)

`app/api` のスタイル(値オブジェクト = 非公開フィールド + コンストラクタ検証、集約ルート = 非公開フィールド + 振る舞いメソッド + `New`/`Reconstruct` ファクトリ、sentinel error + カスタム型、`context.Context` 第一引数、`fmt.Errorf("...: %w", err)` ラップ)をすべて踏襲する。

### ルート
- `go.mod` — `module github.com/srrrs-7/cc-orchestrator/app/auth` / `go 1.24`(依存なし)
- `Makefile` — `app/api/Makefile` と**同一構成**(`help`/`fmt`/`fmt-check`/`lint`/`vet`/`build`/`test`/`test-race`/`check`/`run`)。`run` のみ `go run ./cmd/authz`
- `README.md` — レイヤ構成図・DDD 戦術パターン対応表・エンドポイント一覧・OIDC/PKCE フロー図・seed client/user・**「認証/同意画面を差し込む箇所」「issuer は本番 https 注入」「秘密鍵は起動時メモリ生成」の明示**・curl でのフロー実行例(authorize → token → userinfo)。日本語

### domain/(ドメイン層。他層に非依存)

**domain/client/**(集約ルート `Client` = 登録済み OAuth クライアント)
- `client.go` — 集約ルート `Client`(非公開フィールド: `id ClientID`、`redirectURIs []RedirectURI`、許可スコープ・許可 grant/response type)。振る舞い: `ValidateRedirectURI(RedirectURI) error`、`SupportsResponseType(string) bool`、`SupportsGrantType(string) bool`、`AllowsScope(string) bool`。ファクトリ `New` / `Reconstruct`
- `client_id.go` — VO `ClientID`(非空検証 + `String()`)
- `redirect_uri.go` — VO `RedirectURI`(絶対 URI・スキーム検証、`String()`、等価比較)
- `errors.go` — sentinel: `ErrNotFound` / `ErrInvalidClientID` / `ErrRedirectURIMismatch` / `ErrUnsupportedResponseType` / `ErrUnsupportedGrantType`
- `repository.go` — `Repository` interface: `FindByID(ctx, ClientID) (*Client, error)`(未存在は `ErrNotFound`)

**domain/user/**(集約ルート `User` = リソースオーナー / OIDC subject)
- `user.go` — 集約ルート `User`(非公開: `id UserID`(=sub)、`username Username`、`profile Profile`)。振る舞い: `VerifyPassword(string) bool`(デモ簡略)、getters。ファクトリ `New` / `Reconstruct`
- `user_id.go` — VO `UserID`(= `sub`。非空検証)
- `username.go` — VO `Username`(非空検証)
- `profile.go` — VO `Profile`(`name` / `email`。email 形式の簡易検証)
- `errors.go` — sentinel: `ErrNotFound` / `ErrInvalidUserID` / `ErrInvalidUsername` / `ErrInvalidEmail`
- `repository.go` — `Repository`: `FindByID(ctx, UserID)` / `FindByUsername(ctx, Username)`(未存在は `ErrNotFound`)

**domain/authcode/**(集約ルート `AuthorizationCode`)
- `authorization_code.go` — 集約ルート。非公開フィールド: `code Code`、`clientID`、`userID`、`redirectURI`、`scope Scope`、`nonce Nonce`、`challenge CodeChallenge`、`expiresAt time.Time`、`consumed bool`。ファクトリ `New(...)`(内部で `time.Now()+TTL` により `expiresAt` を確定、`consumed=false`)/ `Reconstruct(...)`(永続化からの再構築)。振る舞い:
  - `Verify(codeVerifier string, redirectURI RedirectURI相当, clientID string) error` — consumed/期限/redirect_uri 一致/client 一致 + **PKCE 検証**をまとめて行い、失敗は種別ごとに sentinel を返す
  - `Consume() error` — 単回使用(既 consumed は `ErrAlreadyConsumed`)
  - `IsExpired() bool` — `time.Now()` 比較
  - getters(userID / scope / nonce / clientID など、トークン発行に必要な値)
- `code.go` — VO `Code`(不透明ランダム値。`NewCode()` が `crypto/rand` で生成、`String()`)
- `code_challenge.go` — VO `CodeChallenge`(`challenge` + `method CodeChallengeMethod`)と `CodeChallengeMethod`(`plain` / `S256`)。`Verify(codeVerifier string) error`: method に応じ変換して一致比較(S256 = `RawURLEncoding(SHA256(ASCII(verifier)))`)。`code_verifier` の文字種(unreserved)・長さ(43–128)検証を含む
- `scope.go` — VO `Scope`(スペース区切りスコープ集合。`ParseScope(string)` で `openid` 必須検証、`Has(string) bool`、`String()`)
- `nonce.go` — VO `Nonce`(任意。空許容)
- `errors.go` — sentinel: `ErrNotFound` / `ErrAlreadyConsumed` / `ErrExpired` / `ErrRedirectURIMismatch` / `ErrClientMismatch` / `ErrPKCEVerificationFailed` / `ErrInvalidCodeVerifier` / `ErrUnsupportedChallengeMethod` / `ErrMissingOpenIDScope`
- `repository.go` — `Repository`: `Save(ctx, *AuthorizationCode)` / `FindByCode(ctx, Code)` / `Save`(consumed 反映の永続化)(未存在は `ErrNotFound`)

**domain/token/**(JWT 発行/検証の抽象 = ポート)
- `claims.go` — VO `Claims`(登録済み: `iss`/`sub`/`aud`/`exp`/`iat`/`nonce`/`auth_time`/`scope` + 追加クレーム `name`/`email` を保持できる構造)。access token 用・ID token 用の**ビルダ関数**(`NewAccessTokenClaims(...)` / `NewIDTokenClaims(...)`)を提供。JSON タグは登録済みクレーム名に一致
- `signer.go` — **ポート** `Signer` interface: `Sign(Claims) (string, error)`(Claims → 署名済みコンパクト JWT)
- `verifier.go` — **ポート** `Verifier` interface: `Verify(token string) (Claims, error)`(署名・`exp` 検証済み Claims を返す。`iss`/`aud` 検証は呼び出し側=service かここで実施。方針: `Verifier` は署名+`exp`まで、`iss`/`aud`の突合は `userinfo_service` が行う)
- `jwk.go` — **ポート** `KeyProvider` interface: `PublicJWK() JWK` / `JWKS() JWKSet`(公開鍵配布用)。`JWK`/`JWKSet` 型(`kty`/`use`/`alg`/`kid`/`n`/`e`)を定義
- `errors.go` — sentinel: `ErrInvalidToken` / `ErrTokenExpired` / `ErrSignatureInvalid` / `ErrUnexpectedAlg`

### service/(アプリケーション層。ドメインを協調させ DTO を返す)
- `dto.go` — `AuthorizeResult`(redirect 先 URL / code / state 構築用)、`TokenResponse`(`access_token`/`token_type`/`expires_in`/`id_token`/`scope`)、`UserInfoDTO`(`sub` 必須 + `name`/`email`)、`ProviderMetadata`(Discovery)、`JWKSet`(= token.JWKSet の再エクスポート or 変換)と各変換関数
- `authorization_service.go` — `AuthorizationService`:
  - `Authorize(ctx, req)` — client 検証(存在・`response_type=code`・redirect_uri 一致)→ scope 検証(`openid` 必須・client 許可内)→ `code_challenge_method=S256` 検証 → resource owner 決定(login_hint→seed user / 既定)→ `AuthorizationCode` 生成・保存 → `AuthorizeResult`(code + state)
  - `Token(ctx, req)` — `grant_type=authorization_code` 検証 → 認可コード取得 → `code.Verify(code_verifier, redirect_uri, client_id)` → `Consume()` → user 取得 → **access token(RS256 JWT)+ ID token(RS256 JWT)**を `token.Signer` で発行 → `TokenResponse`
- `userinfo_service.go` — `UserInfoService.UserInfo(ctx, bearerToken)` — `token.Verifier` で署名/`exp` 検証 + `iss`/`aud` 突合 → `sub` から user 取得 → scope に応じ `name`/`email` を含む `UserInfoDTO`(`sub` は常に必須)
- `discovery_service.go` — `DiscoveryService`: `Metadata(ctx) ProviderMetadata`(issuer 起点で各エンドポイント URL 構築)/ `JWKS(ctx) JWKSet`(`token.KeyProvider` から取得)

### infra/(ドメインが宣言したポートの実装)
- `infra/memory/client_repository.go` — `client.Repository` の in-memory 実装(`sync.RWMutex` + clone 方針、`var _ client.Repository = (*ClientRepository)(nil)`)。`Seed(...)` でデモ client 登録
- `infra/memory/user_repository.go` — `user.Repository` の in-memory 実装(同上)。`Seed(...)` でデモ user 登録
- `infra/memory/authcode_repository.go` — `authcode.Repository` の in-memory 実装(Save/FindByCode、consumed 反映、期限切れ掃除は任意)
- `infra/jwt/signer.go` — `token.Signer` の RSA 実装。header `{"alg":"RS256","typ":"JWT","kid":...}`、署名入力 `base64url(header).base64url(payload)`、署名 = `rsa.SignPKCS1v15(rand, key, crypto.SHA256, sum)`。`var _ token.Signer = (*Signer)(nil)`
- `infra/jwt/verifier.go` — `token.Verifier` の RSA 実装。3 分割 → header の `alg=RS256` 確認 → `rsa.VerifyPKCS1v15` → payload デコード → `exp` 検証 → `token.Claims` 復元。`var _ token.Verifier = (*Verifier)(nil)`
- `infra/jwt/jwk.go` — `token.KeyProvider` の実装。RSA 公開鍵 → JWK(`kty=RSA`/`use=sig`/`alg=RS256`/`kid`/`n`=base64url(modulus)/`e`=base64url(exponent))。`kid` は RFC 7638 JWK サムプリント(SHA-256)または生成 ID

### route/(プレゼンテーション層)
- `router.go` — Go 1.22+ `http.ServeMux` メソッドパターンで 5 エンドポイントを配線(`GET /authorize` / `POST /token` / `GET /userinfo` / `GET /.well-known/openid-configuration` / `GET /.well-known/jwks.json`)
- `authorize_handler.go` — クエリパラメータ解析 → `AuthorizationService.Authorize` → **302 リダイレクト**(`redirect_uri?code=...&state=...`)。**seed ユーザ自動認証の差し込み位置をコメントで明示**
- `token_handler.go` — `application/x-www-form-urlencoded` 解析 → `AuthorizationService.Token` → JSON レスポンス + **`Cache-Control: no-store`**
- `userinfo_handler.go` — `Authorization: Bearer` 抽出 → `UserInfoService.UserInfo` → JSON(`sub` 必須)
- `discovery_handler.go` — `metadata`(Discovery JSON)/ `jwks`(JWK Set JSON)の 2 ハンドラ
- `response.go` — ドメインエラー → **OAuth エラーコード**(`invalid_request`/`invalid_client`/`invalid_grant`/`unauthorized_client`/`unsupported_grant_type`/`invalid_scope`)+ HTTP ステータス変換を集約(`errors.Is`/`errors.As`)。`error`/`error_description` の JSON ボディ。`/token` は 400/401 + JSON、`/authorize` は**検証可能な場合 redirect_uri へエラーリダイレクト**(`?error=...&error_description=...&state=...`)、redirect_uri/client が未検証の場合のみ直接エラー表示。`app/api` の `response.go` と同型で内部エラーは slog + 汎用 500

### cmd/
- `cmd/authz/main.go` — コンポジションルート(配線のみ)。`signal.NotifyContext` による graceful shutdown(`app/api/cmd/api/main.go` と同型)。処理: `rsa.GenerateKey(2048)` → `infra/jwt` の Signer/Verifier/KeyProvider 構築 → `infra/memory` の 3 リポジトリ構築 + **デモ client/user を seed** → service 構築 → `route.NewRouter(...)` → HTTP サーバ起動。`ISSUER`/`PORT` を env から取得。**注意: 既存スキャフォールドの空 `cmd/main.go` は使わず `cmd/authz/main.go` を新設**(admin 設計の配置に合わせる)

---

## 手順

依存関係と `app/api` の前例(層が相互に型整合を取り合うため単一 impl agent に集約)を踏まえて進める。

1. **impl-api(単一 agent、逐次)— `app/auth` の実装**
   - モジュール横断で型・interface の整合を取り合う(domain のポート → infra 実装、service が 4 集約を協調、route が service を呼ぶ)ため、**1 つの impl-api に一括委譲**する(集約ごとに別 agent へ割ると interface 不整合が起きやすい)。
   - 実装順(下流の依存から上流へ): `go.mod` → `domain/{client,user,authcode,token}` → `infra/{memory,jwt}`(ポート実装、`var _` で契約検証)→ `service`(DTO + 3 サービス)→ `route`(handler + response)→ `cmd/authz/main.go`(seed + 配線)→ `Makefile` → `README.md`。
   - コードのコメント・識別子は英語、README は日本語(`project.md`)。
   - **本フェーズではテストは書かない**(tests-after。下記テスト戦略に従い次フェーズで tester が担当)。impl-api は自身のスコープ(`app/auth`)のみ変更し、`app/api`/`app/web`/`app/iac` には触れない。
2. **tester(逐次)— テスト作成・実行**(テスト戦略の分類に従う)。table-driven の純ロジック(PKCE 検証・JWT ラウンドトリップ・認可コード単回使用/失効・Claims 構築・VO バリデーション)+ route の httptest e2e(authorize→token→userinfo を 1 本)。`make test` / `make test-race` が緑になるまで。ロジック不備を見つけたら impl-api に差し戻す。
3. **checker(逐次)— 静的検証**。`app/auth` で `make check`(= `fmt-check` + `lint` + `vet` + `build` + `test`)。失敗は impl-api / tester に差し戻し 1↔3 を反復。**checker 通過前にレビューへ進まない**(フェーズ飛ばし禁止)。
4. **review-security / review-spec / review-performance(3 者並列)— レビュー**
   - **review-security(主眼)**: PKCE S256 強制(plain 拒否)、認可コードの不透明性・単回使用・失効、redirect_uri/client の厳密一致、JWT の `alg` 混同(`alg=none`/HS 降格)防止、`Cache-Control: no-store`、秘密鍵/シークレット/パスワードの非露出、UserInfo の Bearer 検証(iss/aud/exp)、エラーレスポンスの情報漏洩。
   - **review-spec(主眼)**: 下記「公式仕様トレーサビリティ表」で OIDC/OAuth/PKCE/JWK/Discovery の確定事実の充足を突き合わせ。REQUIRED クレーム/フィールドの欠落、nonce 反映、Discovery の REQUIRED メタデータ。
   - **review-performance(副次)**: RSA 鍵生成の起動時 1 回化、`sync.RWMutex` の粒度、認可コード検索の計算量、JWT verify のアロケーション。
   - **前提**: checker(3)通過後に開始。
5. **指摘対応(逐次)**: Blocker / Major は impl-api に差し戻し、2→4 を再実行。今回対応しない指摘は issue-creator が Issue 起票。
6. **完了処理**: 本基盤は起点 Spec/Issue を持たないため Spec/Issue の status 更新は不要。admin が成果を検収し、必要なら「基盤サンプル追加」を記録する Spec/Issue の起票要否をユーザーに確認(planner のスコープ外。手順として記載)。

### 並列可否の要約
- 実装(1)は単一 impl-api に集約(並列化しない — interface 不整合回避)。
- テスト(2)→ checker(3)は逐次。
- レビュー(4)の 3 agent は**並列**。

---

## テスト戦略

**方針: `app/api` と同様 tests-after(実装後にテスト追加)。** 純ロジックは table-driven で 正常系/異常系/境界値 を厚く、route は httptest で end-to-end を最低 1 本。実時間 sleep・実行順序依存のテストは書かない(`testing.md`)。時刻依存(失効)は TTL を 0 / 十分小 / 十分大の値で境界検証する(sleep しない)。

| レベル | 対象 | 観点(正常/異常/境界) |
|---|---|---|
| 純ロジック(domain, table-driven) | `authcode.CodeChallenge.Verify` / `code_verifier` 長さ・文字種検証 | S256 一致(正)/ 不一致・未対応 method・不正 verifier(異)/ 43・128 文字境界(境界) |
| 純ロジック(domain) | `AuthorizationCode.Verify` / `Consume` / `IsExpired` | 正当な verify(正)/ consumed 済み・redirect/client 不一致・失効(異)/ 期限ちょうど(境界) |
| 純ロジック(domain) | `Scope.ParseScope` / `Has` | openid 含む(正)/ openid 欠落(異)/ 空・重複(境界) |
| 純ロジック(domain) | VO(`ClientID`/`RedirectURI`/`UserID`/`Username`/`Profile`/`Code`/`Nonce`) | 妥当値(正)/ 空・不正形式(異)/ 空白・境界長(境界) |
| 純ロジック(domain+infra) | `token.Claims` ビルド + `jwt.Signer`/`jwt.Verifier` **ラウンドトリップ** | sign→verify で Claims 復元(正)/ 改竄署名・`alg` 不一致・`exp` 超過(異)/ `exp` ちょうど(境界)。RS256 の JWK が公開鍵と整合すること |
| infra(table-driven) | `infra/memory` 3 リポジトリ | Save/Find(正)/ 未存在=ErrNotFound(異)/ clone による分離(データ独立性) |
| 統合(route, httptest) | **authorize → token → userinfo の一気通貫**(テスト用 RSA 鍵 + in-memory repo + seed) | code 発行→302、token 発行(access+id、`Cache-Control: no-store`)、userinfo で `sub` 取得(正)。異常系: PKCE 不一致で `invalid_grant`、code 再利用で `invalid_grant`、未知 client で `invalid_client`、openid 欠落で `invalid_scope`、無効 Bearer で 401 |
| 統合(route) | Discovery / JWKS | 必須メタデータの存在、`code_challenge_methods_supported=["S256"]`、JWKS が 1 鍵を返し verify に使えること |

テスト用ヘルパは `app/api/route/task_handler_test.go` に倣い、`*_test`(外部テストパッケージ)+ `httptest` + fresh in-memory repo + テスト用に生成した RSA 鍵で構成する。

---

## 公式仕様トレーサビリティ(確定事実 → 実装 → 検証)

| # | 確定事実(公式) | 実装箇所 | 検証 |
|---|---|---|---|
| 1 | 認可リクエスト REQUIRED: `scope`(openid 必須)/`response_type=code`/`client_id`/`redirect_uri`。`state` RECOMMENDED、`nonce` OPTIONAL | `authorize_handler.go` + `authorization_service.Authorize` + `client.Client` + `authcode.Scope` | route e2e(異常系: openid 欠落/未知 client/redirect 不一致)+ review-spec |
| 2 | 認可コードは不透明・単回使用・失効・redirect_uri/client 束縛 | `authcode.AuthorizationCode`(`Verify`/`Consume`/`IsExpired`)+ `infra/memory/authcode_repository` | domain table-driven + route e2e(code 再利用=invalid_grant)+ review-security |
| 3 | PKCE(RFC 7636): S256 = `BASE64URL(SHA256(ASCII(verifier)))`、verifier 43–128 文字 unreserved、不一致/失効=`invalid_grant`、欠落/未対応 method=`invalid_request` | `authcode.CodeChallenge` + `authorization_service.Token` + `response.go` | domain table-driven(境界含む)+ route e2e + review-security |
| 4 | Token 成功: `access_token`/`token_type=Bearer`/`id_token`(+`expires_in`)、`Cache-Control: no-store` | `service.TokenResponse` + `token_handler.go` | route e2e(ヘッダ/フィールド検証)+ review-spec |
| 5 | access token / id token いずれも RS256 JWT | `token.Claims` + `infra/jwt/{signer,verifier}` | JWT ラウンドトリップ test + JWKS 整合 test |
| 6 | ID Token REQUIRED: `iss`/`sub`/`aud`(client_id 含む)/`exp`/`iat`。`nonce` はリクエストにあれば必須反映。`auth_time` は max_age 等の時 | `token.NewIDTokenClaims` + `authorization_service.Token` | 統合 test(nonce 反映)+ review-spec |
| 7 | UserInfo: Bearer 認証、`sub` 必須 | `userinfo_service` + `userinfo_handler.go` | route e2e(sub 取得 / 無効トークン 401)+ review-security |
| 8 | Discovery REQUIRED: `issuer`/`authorization_endpoint`/`token_endpoint`/`jwks_uri`/`response_types_supported`/`subject_types_supported`/`id_token_signing_alg_values_supported`。+ `userinfo_endpoint`/`scopes_supported`/`claims_supported`/`code_challenge_methods_supported=["S256"]`/`grant_types_supported`/`token_endpoint_auth_methods_supported` | `discovery_service.Metadata` + `discovery_handler.go` | 統合 test(必須キー存在)+ review-spec |
| 9 | JWK Set: `{keys:[{kty:"RSA",use:"sig",alg:"RS256",kid,n,e}]}` | `token.JWK`/`JWKSet` + `infra/jwt/jwk.go` | 統合 test(JWKS で verify 成立)+ review-security |
| 10 | issuer は本番 https 必須(ローカルは env `ISSUER` 既定 `http://localhost:8080`) | `cmd/authz/main.go`(env)+ `discovery_service` + README/コメント明示 | review-spec(注記の存在) |

---

## リスク / 未確定事項

- **エンドユーザ認証の簡略化**: seed ユーザ自動割り当て(login_hint 突合 or 既定)は基盤簡略化であり、本番のログイン/同意フローではない。差し込み位置は `authorize_handler.go` のコメントと README に明示するが、**この簡略化を「本番同等」と誤読しない**ことをレビューでも確認する。
- **RSA 鍵の起動時生成 = 再起動で kid 変更・既存トークン検証不能**: 鍵は永続化せず起動ごとに生成する(秘密情報非混入のため)。稼働中に発行したトークンはプロセス再起動で無効化される。鍵の永続化・ローテーション・複数鍵 JWKS は将来拡張点(README に明記)。
- **access token の `aud` 値の設計**: JWT access token(RFC 9068 相当)の `aud` はリソースサーバ識別子。本基盤ではリソースサーバ = 自 UserInfo とみなし、access token の `aud` を**issuer(または `ISSUER` から導出する固定 audience)**とし、`userinfo_service` がこの `aud` を検証する。ID token の `aud` は `client_id`。この割り当ては設計判断であり、外部リソースサーバを足す場合は audience 設計の見直しが必要(README/コメントに明記)。→ 実装時に impl-api が採用値を確定し、tester が verify で突き合わせる。
- **confidential client 非対応**: 基盤は public client + PKCE を主軸とし、`token_endpoint_auth_methods_supported=["none"]`。client_secret 検証(`client_secret_post`/`basic`)は未実装。confidential 対応は将来拡張点。
- **スコープ限定**: `openid`/`profile`/`email` のみ。`offline_access`(refresh token)は基盤スコープ外 — refresh token 発行/`/token` の `grant_type=refresh_token` は未実装(将来拡張点)。
- **in-memory ストア**: `app/api` と同じく永続化しない。認可コードの掃除(期限切れ削除)は任意実装で、無くても単回使用+失効検証で安全性は担保される。
- **PKCE `plain` の扱い**: 型としては `plain` を実装するが `/authorize` は S256 のみ受理。`plain` を受理範囲に含めるべきかは基盤方針として S256 限定で確定済み(退けた代替案参照)。
- **既存スキャフォールドの空 `cmd/main.go`**: 現状 `app/auth/cmd/` 配下に空の `main.go`(ディレクトリ)が存在する。admin 設計の配置 `cmd/authz/main.go` を新設する方針。impl-api は既存スキャフォールドの空ディレクトリ整理を合わせて行う(不要なら削除)。
- **lint 導入状況**: `make check` は `golangci-lint` を要する。未導入環境では checker がその旨を報告する(`app/api` と同条件)。fmt/vet/build/test は Go 本体で実行可能。
- **起点 Spec/Issue の不在**: 本計画は Spec/Issue を持たない基盤サンプル。将来この基盤に機能追加する場合は、その時点で `spec`/`issue` skill により正式な起点ドキュメントを起こす前提。
