# SPEC-006 実装計画: auth refresh_token グラント(RFC 6749 §6)

- 起点: `docs/specs/20260710-006-auth-refresh-token-grant.md`(SPEC-006、status: approved、要件 R1–R10)
- 承認済み設計: `/Users/s.ryousuke1/.claude/plans/velvety-mapping-cocoa.md`(plan-mode 計画。本計画はこれを docs/plans 形式に詳細化したもの)
- 対象 stack: `app/auth`(Go / OAuth 2.0 + OIDC)。担当は impl-auth(domain / service / route / discovery)と impl-db(migration / sqlc / infra)。

この計画は着手前に **2 つの合意事項**を確定させる:
- **(A)** `domain/refreshtoken/repository.go` のポート interface の最終署名と各メソッドの契約(§手順 0・§付録 A)
- **(B)** `cmd/authz/main.go` の編集分担と適用順(§手順 0・§付録 B)

---

## 方針

### 採用アプローチ: authorization_code 縦切りの踏襲

既存の `authorization_code` の永続化スライス(domain 集約 + ドメイン宣言ポート + `infra/memory` 実装 + `infra/postgres` 実装 + sqlc + goose + `infra/repotest` 共有契約テスト)を**そのまま雛形**にして、新しい永続化集約 `refreshtoken` を追加する。これにより:

- impl-auth(domain / service / route)と impl-db(migration / sqlc / infra)の担当分割が既存パターン(SPEC-005)と一致する
- 単回使用の atomic 契約を `authcode.Repository.Consume`(DELETE ... RETURNING でちょうど 1 つが勝つ)と同型に組み立てられる。refresh token では「旧を consume + 新を insert」を 1 トランザクションにまとめた `Rotate` が単回使用の権威になる
- テスト方式(`repotest` 共有契約 + memory/postgres 双方から実行 + `//go:build integration` の実 DB テスト + sleep 非依存の `Reconstruct` ベース TTL 検証)を authcode からコピーできる

### リフレッシュフローのローテーション + 再利用検知

demo client は public client(`token_endpoint_auth_methods_supported: ["none"]`、PKCE のみ)。RFC 9700 §2.2.2 によりローテーションまたは送信者制約が必須のため、**ローテーション + 再利用検知(family 一括失効)**を採る。ローテ済み(consumed)トークンの再提示を検知し、同一 `family_id` の全 refresh token を失効させる。

### SHA-256 ハッシュのみ保存

refresh token は長寿命クレデンシャルなので DB には **SHA-256 hex ハッシュのみ**を保存(検索キー = PK)。平文はトークン生成時(初回発行・ローテ時)にレスポンスで一度だけ返し、サーバーは保持しない。標準ライブラリ(`crypto/rand` / `crypto/sha256`)のみで実装し、新規 runtime 依存は増やさない(永続化は既存 `pgx` のまま)。

### 退けた代替案(SPEC-006 §4 の代替案表を要約)

| 案 | 不採用理由 |
|---|---|
| ローテーションなし(1本を再利用) | public client では RFC 9700 §2.2.2 が必須要件を満たさない |
| 再利用検知なし(ローテーションのみ) | 盗用された旧トークンの検知・失効ができず §4.14 を満たさない |
| 平文で DB 保存(authcode と同型) | 長寿命クレデンシャルは DB 漏洩時のリスクが大。SHA-256 ハッシュ保存を採用 |
| `offline_access` でゲート | scope / 同意処理が増えサンプルとして冗長。grant 対応 client に常に発行 |
| ID token を再発行しない | SPA がユーザー情報を更新しづらい。OIDC §12.2 準拠で再発行 |
| refresh token を JWT で自己完結 | 個別失効・再利用検知・family 失効ができない。opaque + 永続化 + ローテーションを採用 |

---

## 手順

TDD を先行(tester → impl → tester → checker → review)。orchestration.md のパイプラインに従い admin が各フェーズを委譲する。

### 手順 0(planner・本計画で確定 = このセクションが成果)

impl-auth と impl-db を衝突なく進めるための 2 つの合意を確定する。**変更不可の契約**として付録に置く。

- **(A) ポート署名**: `domain/refreshtoken/repository.go` の interface(4 メソッド Save / FindByTokenHash / Rotate / RevokeFamily)と、各メソッドがどの sentinel をいつ返すかの契約 → **付録 A(この署名どおりに impl-auth が domain で宣言し、service が呼び、impl-db が memory / postgres で実装する)**
- **(B) main.go 編集分担・適用順** → **付録 B(関数単位の編集境界と直列化順)**

### 手順 1(tester・TDD 先行、red)

要件 R1–R10 から失敗テストを先に作成する。付録 A のポート署名・VO シグネチャに対して書くので、impl が着地した時点でコンパイル・green になる(未実装のうちはコンパイルエラー = red。authcode スライスと同じ TDD 運用)。

- **domain 単体**(`app/auth/domain/refreshtoken/*_test.go`): `Issue` / `Rotate` / `MatchesClient` / `IsExpired`、VO(`Token`/`Hash` の決定性、`FamilyID`、`Scope.Narrow` の部分集合許可・拡大拒否)。期限切れは `Reconstruct` + 計算済み `expiresAt`(実 sleep 禁止、testing.md)。
- **共有契約テスト**(`app/auth/infra/repotest/refreshtoken_contract.go` + memory / postgres 双方のバインディング): §テスト戦略の契約シナリオ。**この repotest ランナーは infra テスト基盤なので tester が作成し、impl-db が memory/postgres 側の呼び出し(`*_contract_test.go` / `*_integration_test.go`)を配線する**(authcode と同じ役割分担)。
- **route 統合**(`app/auth/route/*_test.go`、`helpers_test.go` ハーネス拡張): §テスト戦略の route ケース。demo client seed に `"refresh_token"` grant 追加、`tokenResponseBody` に `refresh_token` フィールド追加、discovery テスト更新。

### 手順 2(impl-auth・impl-db を **一部並列 / main.go は直列**)

**適用順は付録 B のとおり impl-auth → impl-db に直列化する(main.go の配線競合を避けるため)。** domain/service/route と infra/db の**パッケージ本体は独立**しており、admin は両者を並列起動してよいが、`cmd/authz/main.go` の配線は付録 B の境界・順序を厳守する。

- **impl-auth**(R1–R7, R9, R10):
  1. `domain/refreshtoken/` を新規作成(付録 A の署名で `repository.go` を宣言。集約 + VO + errors)
  2. `service/authorization_service.go`: grant switch 化 + `authorizationCodeGrant` 抽出(末尾に refresh token 発行を追加)+ `refreshTokenGrant` 新規。`service/dto.go`: `TokenRequest`/`TokenResponse`/`newTokenResponse` 拡張
  3. `route/token_handler.go`: form から `refresh_token` / `scope` 取得。`route/response.go`: `tokenErrorCode` に refreshtoken sentinel マッピング追加
  4. `service/discovery_service.go`: `GrantTypesSupported` に `"refresh_token"` 追加
  5. `domain/client/client_test.go`: `SupportsGrantType("refresh_token")` の否定アサーションを肯定に反転
  6. `cmd/authz/main.go`: **`buildDemoClient` の grant slice にのみ** `"refresh_token"` を追加(付録 B のとおり `setupPersistence` / `run()` 配線には触れない)
- **impl-db**(R4, R5, R8):
  1. `db/migrations/000002_create_refresh_tokens.sql`(`make migrate-create name=create_refresh_tokens` で雛形生成)
  2. `db/queries/refresh_tokens.sql`
  3. `make sqlc` で `infra/postgres/sqlcgen/*` 再生成(**同一コミットでコミット**、drift なし)
  4. `infra/postgres/refreshtoken_repository.go` / `infra/memory/refreshtoken_repository.go`(付録 A を満たす)
  5. `infra/repotest/refreshtoken_contract.go` の memory/postgres バインディング配線(ランナー本体は手順 1 で tester が作成)
  6. `cmd/authz/main.go`: **`setupPersistence`(署名 + 両 arm + import)と `run()` 配線(戻り値の受け取り + `NewAuthorizationService` への引数追加)**(付録 B のとおり impl-auth の後に適用)

### 手順 3(tester・green)

`cd app/auth && make test`(+ `make test-race`)を green にする。DB が利用可能なら `make migrate-up && make test-integration`。不足テストを追加する。

### 手順 4(checker)

`cd app/auth && make check`(fmt-check + lint + vet + build + test)。加えて `make sqlc` で差分ゼロ(sqlc-drift 相当)を確認。

### 手順 5(review-security / review-performance / review-spec・並列)

SPEC-006 §3 準拠チェックリストと R1–R10 → テスト対応表(§テスト戦略)を検証。Blocker / Major は impl agent に差し戻し(手順 3→5 を再実行)、今回対応しない指摘は issue-creator が Issue 化。

### 手順 6(admin + spec skill)

価値の検証方法を満たしたことを SPEC-006 経緯に記録し `status: done` に更新。

---

## 変更ファイル

### impl-auth 担当

**新規(`app/auth/domain/refreshtoken/`)**

| ファイル | 内容 |
|---|---|
| `refresh_token.go` | 集約 `RefreshToken`(非公開フィールド)+ VO `ClientID`/`UserID` + `RefreshTokenTTL` + `Issue`/`Rotate`/`Reconstruct`/`MatchesClient`/`IsExpired`/getter |
| `token.go` | VO `Token`(平文 opaque、`NewToken`/`Hash`/`String`)+ `TokenHash`(hex、comparable、`HashToken`/`ParseTokenHash`/`String`) |
| `family_id.go` | VO `FamilyID`(`NewFamilyID`/`ParseFamilyID`/`String`) |
| `scope.go` | VO `Scope`(`ParseScope`/`Narrow`/`Has`/`Values`/`String`) |
| `repository.go` | ポート `Repository`(**付録 A の署名どおり**) |
| `errors.go` | sentinel(`ErrNotFound`/`ErrExpired`/`ErrReused`/`ErrClientMismatch`/`ErrInvalidScope`/`ErrInvalidToken`) |

**変更**

| ファイル | 内容 |
|---|---|
| `service/authorization_service.go` | フィールド `refreshTokens` 追加、`NewAuthorizationService` 引数追加、`grantTypeRefreshToken` 定数、`Token` を grant switch 化(`authorizationCodeGrant` 抽出 + refresh token 発行 + `refreshTokenGrant` 新規) |
| `service/dto.go` | `TokenRequest` に `RefreshToken`/`Scope`、`TokenResponse` に `RefreshToken string json:"refresh_token,omitempty"`、`newTokenResponse` に refresh token 引数 |
| `route/token_handler.go` | form から `refresh_token`/`scope` を `TokenRequest` に詰める |
| `route/response.go` | `tokenErrorCode` に refreshtoken sentinel → `invalid_grant`/`invalid_scope` マッピング、`unsupported_grant_type` 説明更新 |
| `service/discovery_service.go` | `GrantTypesSupported: []string{"authorization_code", "refresh_token"}` |
| `domain/client/client_test.go` | `SupportsGrantType("refresh_token")` の否定アサーションを肯定へ反転 |
| `cmd/authz/main.go` | **`buildDemoClient` の grant slice にのみ** `"refresh_token"` 追加(付録 B) |

### impl-db 担当

**新規**

| ファイル | 内容 |
|---|---|
| `db/migrations/000002_create_refresh_tokens.sql` | `refresh_tokens` テーブル + `family_id` index(非修飾 DDL、up/down 対) |
| `db/queries/refresh_tokens.sql` | `InsertRefreshToken` / `GetRefreshToken` / `DeleteExpiredRefreshToken` / `ConsumeRefreshToken` / `RevokeFamilyRefreshTokens` |
| `infra/postgres/refreshtoken_repository.go` | ポート実装(`Rotate` は `WithTx` でトランザクション) |
| `infra/postgres/refreshtoken_repository_integration_test.go` | `//go:build integration`、repotest 契約を実 DB で実行(バインディング) |
| `infra/memory/refreshtoken_repository.go` | `sync.Mutex` + `map[refreshtoken.TokenHash]*refreshtoken.RefreshToken` |
| `infra/memory/refreshtoken_repository_contract_test.go` | repotest 契約を memory で実行(バインディング) |
| `infra/memory/refreshtoken_repository_test.go` | memory 固有テスト(clone 独立性など、authcode に倣う) |

**生成(コミット対象)**

| ファイル | 内容 |
|---|---|
| `infra/postgres/sqlcgen/refresh_tokens.sql.go` ほか | `make sqlc` 再生成物(手で編集しない) |

**変更**

| ファイル | 内容 |
|---|---|
| `cmd/authz/main.go` | **`setupPersistence`(署名 + 両 arm + `refreshtoken` import)と `run()` 配線(戻り値受け取り + `NewAuthorizationService` 引数追加)**(付録 B、impl-auth の後) |

### tester 担当

| ファイル | 内容 |
|---|---|
| `infra/repotest/refreshtoken_contract.go` | 共有契約ランナー `RunRefreshTokenRepositoryContract`(memory/postgres 双方から呼ぶ) |
| `domain/refreshtoken/*_test.go` | domain 単体(集約・VO) |
| `route/*_test.go`(新規 `refresh_token_test.go` 等)+ `route/helpers_test.go` 拡張 + `route/discovery_test.go` 更新 | route 統合 |

> CI: 既存 `.github/workflows/sqlc-drift.yml`(`make sqlc` 再実行で drift 検出)と `auth-integration` job(postgres service に `make migrate-up` で 000002 を自動適用 → `make test-integration`)で新テーブルはカバーされる。**新規 env / secret / workflow は不要のため impl-ci の関与は不要**(§リスクで最終確認事項として明記)。

---

## テスト戦略

TDD 先行(手順 1)。table-driven・sleep 非依存(TTL 境界は `Reconstruct` + 計算済み `expiresAt`)。observability は「新旧 refresh token の平文が異なる」「旧提示で family が失効する」を軸に検証する。

### レベル別

- **domain 単体**(`domain/refreshtoken`): `Issue`(新 Token / 新 FamilyID / expiresAt=now+TTL / consumed=false)、`Rotate(scope)`(同一 family・新 Token・新 hash・新 expiresAt・consumed=false・scope=引数)、`MatchesClient`(不一致→`ErrClientMismatch`)、`IsExpired`(`Reconstruct` で過去 expiresAt)、VO(`HashToken` 決定性 = 同一平文→同一 hash・別平文→別 hash、`Token.Hash()` と `HashToken(Token.String())` 一致、`FamilyID` 生成の一意性、`Scope.Narrow` の部分集合許可 / 拡大 →`ErrInvalidScope` / 空要求)。
- **共有契約テスト**(`repotest.RunRefreshTokenRepositoryContract`、memory + postgres 双方):
  - `Save` → `FindByTokenHash` 往復で全フィールド一致・`Consumed()=false`
  - 未保存 hash の `FindByTokenHash` → `ErrNotFound`
  - 期限切れ行の `FindByTokenHash` → `ErrNotFound` かつ lazy evict(2 回目も `ErrNotFound`)
  - **consumed でも(未期限なら)`FindByTokenHash` は返す**(再利用検知の前提)
  - `Rotate` 成功: 旧は consumed(以後 `Rotate` は `ErrReused`)、新は active(`FindByTokenHash` で取得可)
  - `Rotate` 競合(20 racer で **ちょうど 1 成功・他はすべて `ErrReused`**、`otherErr=0`)
  - 消費済みを `Rotate` → `ErrReused` / 不在・期限切れを `Rotate` → `ErrNotFound`(付録 A の区別)
  - `RevokeFamily` で同一 family の全行が消え、以後 `FindByTokenHash` は `ErrNotFound`(他 family は残る)
  - TTL 境界: `+2s` は find/rotate 可、`-2s` は `ErrNotFound`(実 sleep なし)
- **route 統合**(`route_test`):
  - `TestToken_AuthorizationCode_IssuesRefreshToken` — コード交換応答に非空 `refresh_token`(R2)
  - `TestRefreshToken_Success_IssuesNewAccessAndIDToken` — 新 access/id token、`refresh_token` がローテートされ**旧と異なる**(R1/R3/R4)。ID token の `iss`/`sub`/`aud` は元と同一、`iat` は進む、`nonce` は空(R3)
  - `TestRefreshToken_Rotation_OldTokenRejected` — ローテ後に旧提示 → `invalid_grant`、直近の新トークンも失効(family revocation)(R4/R5)
  - `TestRefreshToken_Reuse_RevokesFamily` — consumed 再提示 → `invalid_grant`、family の active も失効(R5)
  - `TestRefreshToken_ClientMismatch_InvalidGrant` — 別 client 提示 → `invalid_grant`(R6)
  - `TestRefreshToken_ScopeNarrower_OK` / `TestRefreshToken_ScopeWiden_InvalidScope` — 部分集合許可 / 拡大 → `invalid_scope`(R7)。narrow 時は新 refresh token の scope も絞られることを次リフレッシュで確認
  - `TestRefreshToken_Unknown_InvalidGrant` — 未知トークン → `invalid_grant`(R1 異常系)
  - `TestRefreshToken_ErrorResponse_HasNoCacheHeaders` — エラーも `Cache-Control: no-store` / `Pragma: no-cache`・JSON(R10)
  - `TestRefreshToken_ConcurrentUse_ExactlyOneSucceeds` — 同一 refresh token の並行リフレッシュでちょうど 1 成功・他は `invalid_grant`(R4/R5 の atomic)
  - discovery テスト更新 — `grant_types_supported` に `refresh_token`(R9)
  - TTL 期限切れの route ケースは実 sleep を要するため route では扱わず、contract / domain の `Reconstruct` ベースで検証する(必要なら harness がテスト用リポジトリハンドルに期限切れ RT を直接 seed する形に留める)
- **postgres 統合**(`//go:build integration`): `refreshtoken_repository_integration_test.go` が上記契約を実 DB で実行(`make test-integration`、`auth-integration` CI)。

### R1–R10 × テスト対応表

| 要件 | 検証テスト |
|---|---|
| R1 受理・交換 | route `Success` / `Unknown_InvalidGrant`、service |
| R2 コード交換で発行 | route `AuthorizationCode_IssuesRefreshToken` |
| R3 access+ID token 再発行(iat 新・iss/sub/aud 同一・nonce なし) | route `Success`(JWT payload 検証) |
| R4 ローテーション | route `Success` / `Rotation_OldTokenRejected` / `ConcurrentUse`、contract `Rotate` 成功・競合、domain `Rotate` |
| R5 再利用検知 + family 失効 | route `Reuse_RevokesFamily` / `Rotation_OldTokenRejected`、contract `Rotate`=`ErrReused` + `RevokeFamily` |
| R6 client バインド | route `ClientMismatch_InvalidGrant`、domain `MatchesClient`、contract |
| R7 scope 部分集合 | route `ScopeNarrower_OK` / `ScopeWiden_InvalidScope`、domain `Scope.Narrow` |
| R8 SHA-256 ハッシュのみ保存 | domain `HashToken`/`Token.Hash`、contract(保存キー = hash)、migration の `token_hash` PK、review でコード確認(平文が DB 引数・ログに出ない) |
| R9 discovery | route `discovery` 更新 |
| R10 JSON + no-store/no-cache・標準 error | route `ErrorResponse_HasNoCacheHeaders`、各 error code アサーション |

---

## 付録 A(確定): `domain/refreshtoken/repository.go` ポート署名と契約

**この署名は変更不可。** impl-auth が domain で宣言し service が呼ぶ / impl-db が memory・postgres で実装する。`Rotate` の `ErrReused` と `ErrNotFound` の区別が単回使用と再利用検知の権威。authcode の `Consume` の atomic 契約コメントに倣って各メソッドの契約を明文化する。

```go
package refreshtoken

import "context"

// Repository is the persistence boundary for the RefreshToken
// aggregate. It is declared in the domain layer (dependency
// inversion): infra/memory and infra/postgres provide interchangeable
// implementations whose observable behavior MUST be identical (proven
// by repotest.RunRefreshTokenRepositoryContract).
//
// Rotate is the sole atomic single-use + rotation mechanism (the
// RefreshToken analogue of authcode.Repository.Consume): it flips the
// old token to consumed and inserts the new one in one critical
// section, so that when two callers race to rotate the same refresh
// token, exactly one wins (nil) and every loser observes ErrReused --
// the signal service.AuthorizationService uses to revoke the whole
// family (RFC 9700 4.14 reuse detection).
type Repository interface {
	// Save inserts rt as a new row (the initial refresh token minted at
	// authorization_code exchange, RefreshToken.Issue). It is a plain
	// insert, not an upsert: a token_hash collision would indicate a
	// broken random generator, not a legitimate re-save.
	Save(ctx context.Context, rt *RefreshToken) error

	// FindByTokenHash looks up a refresh token by the SHA-256 hash of
	// its opaque value. It returns:
	//   - the RefreshToken (which MAY be Consumed()==true) when a row
	//     exists AND is not expired -- consumed-but-unexpired rows are
	//     returned on purpose, so the service can detect a reuse of an
	//     already-rotated token before it even reaches Rotate;
	//   - a wrapped ErrNotFound when no row exists, OR the row has
	//     expired. Expired rows are lazily evicted as a side effect
	//     (same lazy-eviction contract as authcode.FindByCode), so
	//     expired == absent from the caller's point of view.
	FindByTokenHash(ctx context.Context, hash TokenHash) (*RefreshToken, error)

	// Rotate atomically consumes the token identified by oldHash and
	// inserts newRT, in a single transaction/critical section. It
	// returns:
	//   - nil when the old token existed, was NOT consumed and NOT
	//     expired: it is marked consumed and newRT is inserted (the
	//     caller may return the new refresh token to the client);
	//   - a wrapped ErrReused when a non-expired row for oldHash exists
	//     but could not be consumed because it was ALREADY consumed --
	//     this is both a genuine reuse (a replay after rotation) and the
	//     losing side of a concurrent Rotate race for the same token.
	//     The caller MUST revoke the whole family;
	//   - a wrapped ErrNotFound when no row for oldHash exists, or it
	//     has expired (there is no live family to protect: the caller
	//     returns invalid_grant WITHOUT revoking a family).
	//
	// Precedence when the atomic consume affects zero rows: if a
	// non-expired row for oldHash still exists it is necessarily
	// consumed -> ErrReused (reuse detection takes precedence);
	// otherwise (absent or expired) -> ErrNotFound. newRT is inserted
	// only on the nil path; ErrReused/ErrNotFound leave the store
	// unchanged (no partial write).
	Rotate(ctx context.Context, oldHash TokenHash, newRT *RefreshToken) error

	// RevokeFamily deletes every refresh token whose familyID matches
	// (the whole rotation chain). It is the reuse-detection response
	// (RFC 9700 4.14): after a reuse is detected, the entire family is
	// invalidated so a stolen token cannot yield further tokens. It is
	// idempotent -- deleting zero rows is not an error.
	RevokeFamily(ctx context.Context, familyID FamilyID) error
}
```

### 実装ノート(impl-db 向け・contract を満たすための骨子)

- **postgres `Rotate`**: `sqlcgen.New(db).WithTx(tx)` で
  1. `ConsumeRefreshToken`(`UPDATE ... SET consumed=true WHERE token_hash=$1 AND consumed=false AND expires_at > now() RETURNING token_hash`)を実行。
  2. 1 行 → `InsertRefreshToken(newRT)` → commit → nil。
  3. `sql.ErrNoRows` → 同 tx 内で `GetRefreshToken`(`WHERE token_hash=$1 AND expires_at > now()`、consumed 非依存)。行あり(= 未期限なのに consume できなかった = 既に consumed)→ `ErrReused`。`sql.ErrNoRows`(不在 or 期限切れ)→ `ErrNotFound`。
- **postgres `FindByTokenHash`**: `GetRefreshToken` が `sql.ErrNoRows` → `DeleteExpiredRefreshToken`(lazy evict)→ `ErrNotFound`。行あり → 各 VO コンストラクタで再検証してから `Reconstruct`。
- **memory `Rotate`**(単一 `sync.Mutex` 区間): `old, ok := m[oldHash]`;`!ok || old.IsExpired()` → `ErrNotFound`(期限切れは delete);`old.Consumed()` → `ErrReused`;それ以外 → 旧を consumed 状態で保存 + `m[newHash]=clone(newRT)` → nil。postgres と observable 一致。
- **memory `FindByTokenHash`**: 不在 → `ErrNotFound`;`IsExpired()` → delete + `ErrNotFound`;それ以外(consumed 含む)→ clone を返す。
- `RevokeFamily`: postgres は `DELETE ... WHERE family_id=$1`;memory は family_id 一致行をスキャン削除。
- `SelectMode` / fail-closed / `sql_package: database/sql` は SPEC-005 既存パターンを踏襲(`sqlc.yaml` 変更不要)。

### domain 集約メソッドのシグネチャ(impl-auth が宣言、tester/service が使用)

```go
const RefreshTokenTTL = 30 * 24 * time.Hour // sliding: Rotate resets expiresAt

// Issue: 認可コード交換時の初回発行。新 Token・新 FamilyID・expiresAt=now+TTL・consumed=false。
func Issue(clientID ClientID, userID UserID, scope Scope) (*RefreshToken, Token, error)

// Rotate: 同一 family の後続トークンを生成(新 Token/hash・新 expiresAt・consumed=false・scope=引数=実効scope)。
func (rt *RefreshToken) Rotate(scope Scope) (*RefreshToken, Token, error)

// Reconstruct: infra からの再構築(検証済み状態・エラーなし)。
func Reconstruct(hash TokenHash, familyID FamilyID, clientID ClientID, userID UserID, scope Scope, expiresAt time.Time, consumed bool) *RefreshToken

func (rt *RefreshToken) MatchesClient(clientID ClientID) error // 不一致→ErrClientMismatch
func (rt *RefreshToken) IsExpired() bool
// getter: TokenHash() / FamilyID() / ClientID() / UserID() / Scope() / ExpiresAt() / Consumed()
```

VO: `Token`(`NewToken() (Token, error)` = 32 byte crypto/rand base64url、`Hash() TokenHash`、`String() string`)/ `TokenHash`(comparable string、`HashToken(plaintext string) TokenHash` = SHA-256 hex、`ParseTokenHash`、`String`)/ `FamilyID`(`NewFamilyID`/`ParseFamilyID`/`String`)/ `Scope`(`ParseScope`/`Narrow(requested string) (Scope, error)`/`Has`/`Values`/`String`)。他 domain パッケージを import しない(ローカル VO)。

### service リフレッシュフロー(付録 A を使う `refreshTokenGrant` の順序)

1. `req.RefreshToken` 空 → `invalid_grant`
2. `client.ParseClientID`/`FindByID`、`SupportsGrantType("refresh_token")` 確認
3. `hash := refreshtoken.HashToken(req.RefreshToken)`;`FindByTokenHash` → `ErrNotFound` は `invalid_grant`
4. `rt.Consumed()` → **再利用検知** → `RevokeFamily(rt.FamilyID())` → `invalid_grant`
5. `rt.MatchesClient(...)` → 不一致は `invalid_grant`
6. scope narrowing: `req.Scope` 空なら `rt.Scope()`、非空なら `rt.Scope().Narrow(req.Scope)`(拡大→`invalid_scope`)
7. `users.FindByID(rt.UserID())`
8. access token(sub=userID, aud=issuer, 実効 scope)+ ID token(新 iat・nonce なし・aud=client_id)を `signer.Sign`
9. `newRT, plaintext := rt.Rotate(実効scope)`;`refreshTokens.Rotate(hash, newRT)`:
   - `ErrReused` → `RevokeFamily` → `invalid_grant`
   - `ErrNotFound` → `invalid_grant`(family 失効なし)
   - nil → 応答(access_token + id_token + **新** refresh_token + scope + expires_in)

`authorizationCodeGrant` の末尾は、`c.SupportsGrantType("refresh_token")` の場合に `refreshtoken.Issue(...)` → `refreshTokens.Save` → 平文を `newTokenResponse` に渡す。

---

## 付録 B(確定): `cmd/authz/main.go` の編集分担と適用順

`cmd/authz/main.go` は composition root。`setupPersistence`(impl-db)と `NewAuthorizationService` の引数変更(impl-auth の service 側 + main.go 配線)が同一ファイルに同居する。Go は「戻り値の受け取り変数の未使用」「引数不足」を**コンパイルエラー**にするため、変更をどう割っても片方の中間状態は単体では build できない。よって **関数単位で境界を切り、直列化する**。

### 編集境界(関数単位)

| 関数 | 担当 | 変更内容 |
|---|---|---|
| `buildDemoClient` | **impl-auth** | grant slice `[]string{"authorization_code"}` に `"refresh_token"` を追加。他を参照しない自己完結の leaf 変更 |
| `setupPersistence` | **impl-db** | 戻り値タプルに `refreshtoken.Repository` 追加、Memory/Postgres 両 arm で `memory.NewRefreshTokenRepository()` / `postgres.NewRefreshTokenRepository(db)` を構築して返す、`refreshtoken` の import と doc コメント更新 |
| `run()` の配線 | **impl-db** | `setupPersistence` 呼び出しを `clientRepo, userRepo, authCodeRepo, refreshTokenRepo, closePersistence, err := setupPersistence(ctx)` に、`service.NewAuthorizationService(...)` 呼び出しに `refreshTokenRepo` 引数を追加 |

> `run()` 配線は「setupPersistence が `refreshTokenRepo` を供給する(impl-db)」かつ「`NewAuthorizationService` が新引数を受ける(impl-auth の service 変更)」の**両方が揃って初めてコンパイルできる**。よって `run()` 配線は最後に一括で行い、その所有を impl-db に置く(impl-auth の service 署名が先に tree に入っている前提)。

### 適用順(直列): **impl-auth → impl-db**

1. **impl-auth を先に適用**: domain/refreshtoken・service(`NewAuthorizationService` 署名変更を含む)・route・discovery・client_test・main.go の `buildDemoClient` を実装。
   - この時点で `cmd/authz/main.go` は**まだコンパイルしない**(`NewAuthorizationService` が新引数を要求するが `run()` 未更新、`setupPersistence` も未対応)。**これは想定どおり**で手順 2 で解消する。
   - impl-auth の自己検証は自パッケージに限定する: `go build ./domain/refreshtoken/... ./service/... ./route/...`(または該当パッケージの `go test`)。`cmd/authz` の build gap は impl-db が閉じる。
2. **impl-db を後に適用**: migration・queries・sqlc・infra(postgres/memory/repotest バインディング)、そして main.go の `setupPersistence` + `run()` 配線を実装。impl-auth の service 署名が既に入っているので、`run()` 配線で `refreshTokenRepo` を受け取り `NewAuthorizationService` に渡すと**モジュール全体が green** になる。impl-db は `make build` / `make vet` / `make check` を全体で検証できる。

**なぜ impl-auth → impl-db か**: impl-auth の中間 build 破綻は `cmd/authz`(1 leaf パッケージ)に限定され、impl-auth の成果物(domain/service/route)は単体で build・test 可能。逆順(impl-db 先)だと impl-db が `refreshTokenRepo` を「宣言したが未使用」または「未更新の署名へ引数超過」で渡すことになり、破綻が impl-db 自身の配線編集の内側に入って自己検証しづらい。したがって **impl-auth を先**にする。

> 実運用: admin は両者の**パッケージ本体**(main.go を除く部分)を並列起動してよいが、main.go の適用は上記直列順を厳守する。安全側に倒すなら impl-auth 完了報告 → impl-db 起動の完全直列でもよい(main.go 競合ゼロを保証)。

---

## リスク / 未確定事項

- **main.go 同時編集**: 付録 B で関数境界 + 直列順(impl-auth → impl-db)を確定済み。admin は main.go の適用順を厳守すること(並列起動する場合も main.go 配線は impl-db の最終ステップ)。
- **並行リフレッシュの厳格性**: 再利用検知は正規クライアントの同時リフレッシュ(ネットワークリトライ等)でも family を失効させ得る。RFC 9700 §4.14 準拠として**意図的**。SPEC-006 §「非機能要件」に明記済み(クライアントは再認証で復帰)。
- **期限切れ consumed トークンの再利用は検知しない**: `FindByTokenHash`/`Rotate` とも expired を `ErrNotFound` として扱い family 失効しない(lazy evict 方針)。sliding TTL では旧 consumed トークンが自身の 30 日を過ぎると検知対象から外れる。30 日窓のごく限定的な穴で、hash 保存 + ローテーションのサンプルとして許容。恒久強化(consumed 行の保持期間延長等)は将来 Issue 候補。
- **TTL 方針**: 30 日・ローテーションごとにリセット(スライディング)、family の絶対上限なし(サンプル簡潔性)。可変化が必要なら `RefreshTokenTTL` 定数を調整。
- **scope narrowing の恒久化**: リフレッシュで scope を絞ると新 refresh token の scope も絞られ、以後元の広さには戻せない(RFC 6749 §6 準拠の想定挙動)。narrow が `openid` を落とし得る点も含め、ID token は常に再発行する(refreshtoken.Scope は openid 必須にしない = 他 domain 非結合)。SPEC-006 に明記。
- **CI カバレッジ**: 既存 `sqlc-drift.yml`(auth ジョブ)と `auth-integration`(`make migrate-up` で 000002 自動適用 → `make test-integration`)で新テーブルはカバーされる見込み。**新規 env / secret / workflow なし → impl-ci 不要**と判断。手順 4 の checker と手順 5 の review で、000002 が integration migrate に確実に乗るか(migrate up→down→up)を最終確認する。
- **`Rotate` の二重命名**: 集約メソッド `RefreshToken.Rotate(scope)`(新集約 + 平文生成)と `Repository.Rotate(oldHash, newRT)`(永続化遷移)が同名。authcode の `AuthorizationCode.Consume()` / `Repository.Consume()` と同型の意図的踏襲。doc コメントで層の違いを明記する。
