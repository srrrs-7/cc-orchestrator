---
id: SPEC-006
title: auth refresh_token グラント(RFC 6749 §6)
status: done  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-10
updated: 2026-07-10  # 実装・レビュー完了、done に更新
issues: [ISSUE-019]       # 関連Issue ID (例: [ISSUE-003])
supersedes: null # 置き換える旧Spec ID
---

# SPEC-006: auth refresh_token グラント(RFC 6749 §6)

## 1. ユーザー価値(なぜ作るか)

> **OAuth クライアント(app/web などの SPA / 一般の RP)** が **アクセストークン失効後にユーザーを再ログインさせずに新しいアクセストークンを取得できるようになり**、**ログインセッションの継続性(UX)とアクセストークン短命化(セキュリティ)を両立できる** 価値を得る。

- **対象ユーザー**: `app/auth` を利用する OAuth 2.0 クライアント(認可コード + PKCE フローを使う public client。第一に app/web の SPA)
- **解決する課題**: 現状 `app/auth` は `grant_type=authorization_code` のみ対応で refresh token を発行しない。アクセストークン(TTL 1h)が切れるたびにユーザーを `/authorize` へ送り再認証させる必要がある。アクセストークンを長命化して回避すると漏洩時のリスクが増す
- **得られる価値**: アクセストークンを短命に保ったまま、バックグラウンドで静かにトークンを更新できる。RFC 9700 のベストプラクティス(public client のローテーション + 再利用検知)に沿った安全な更新
- **価値の検証方法**: 認可コード交換で得た refresh token を `POST /token` (`grant_type=refresh_token`) に提示すると新しい access token / ID token / refresh token が返り、旧 refresh token は以後拒否される。ローテ済みトークンの再提示で同一 family の全 refresh token が失効する。`make check` が green で、準拠チェックリスト(§3)を review-security が確認できたら成功とみなす

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- OAuth クライアント開発者として、保持している refresh token を `/token` に送るだけで新しい access token を得たい。なぜなら、アクセストークンが切れるたびにユーザーをリダイレクト再認証させると体験が途切れるから。
- セキュリティ担当として、public client の refresh token がローテーションされ、盗まれて再利用されたら自動で失効してほしい。なぜなら、public client は client secret で保護できず、長寿命 refresh token が最大の攻撃対象になるから。

### 利用フロー

1. クライアントが認可コード + PKCE で `POST /token` (`grant_type=authorization_code`) を実行する
2. サーバーが access token / ID token に加えて **refresh token(平文・一度きり)** を返す
3. アクセストークン失効後、クライアントが `POST /token` に `grant_type=refresh_token` と `refresh_token`(任意で `scope`)を送る
4. サーバーが refresh token を検証し、新しい access token / ID token / **ローテートされた新しい refresh token** を返す。旧 refresh token は無効化される
5. 万一ローテ済み(無効化済み)の refresh token が再提示されたら、サーバーは盗用とみなし同一 family の refresh token をすべて失効させ、`invalid_grant` を返す(クライアントは再認証が必要になる)

## 3. 要件(何を満たすべきか)

準拠する公式仕様: RFC 6749 §1.5 / §6、RFC 9700(OAuth 2.0 Security BCP)§2.2.2 / §4.14、OpenID Connect Core 1.0 §12。各 R は §6 の準拠チェックリストと 1:1 対応し、テストで検証する。

### 機能要件

- [x] R1: `/token` が `grant_type=refresh_token` / `refresh_token`(必須)/ `scope`(任意)を受理し、有効な refresh token を新しい access token に交換する(RFC 6749 §6)。`refresh_token` が空/未指定の場合は `invalid_grant` を返す(RFC 6749 §6 の refresh 意味論に寄せた設計判断。未発行トークンと同じ扱い)
- [x] R2: 認可コード交換時、client が `refresh_token` グラント対応なら refresh token を発行し、トークンレスポンスの `refresh_token` に含める(offline_access ゲートはしない)
- [x] R3: リフレッシュ時に access token と ID token を再発行する。ID token は新 `iat`、`iss`/`sub`/`aud` は元と同一、`nonce` なし(OIDC §12.2 不変条件)
- [x] R4: refresh token をローテーションする(リフレッシュのたびに新規発行し、旧トークンを無効化する)(RFC 9700 §4.14、public client は §2.2.2 で必須)
- [x] R5: ローテ済み(consumed)refresh token の再提示を検知し、同一グラント(family)の全 refresh token を失効する(再利用/盗用検知、RFC 9700 §4.14)
- [x] R6: refresh token は発行先 client にバインドされ、別 client からの提示は `invalid_grant` で拒否する(RFC 6749 §6)
- [x] R7: 要求 `scope` は元の付与 scope の**部分集合のみ**許可する。拡大は `invalid_scope` で拒否、省略時は元の scope を用いる(RFC 6749 §6、scope を絞った場合は新 refresh token の scope も絞られる)
- [x] R8: refresh token は SHA-256 ハッシュのみを DB に保存する。平文は発行/ローテ時にレスポンスで一度だけ返し、サーバーは保持しない
- [x] R9: OIDC discovery の `grant_types_supported` に `refresh_token` を追加する
- [x] R10: 成功/エラーとも応答は JSON + `Cache-Control: no-store` / `Pragma: no-cache`。エラーは標準 OAuth error 応答(`invalid_grant` / `invalid_scope` / `invalid_request` 等)で内部情報を漏らさない

### 非機能要件

- 標準ライブラリのみで実装する(トークン生成は `crypto/rand`、ハッシュは `crypto/sha256`)。新規 runtime 依存を増やさない(永続化は既存 `pgx` のみ)
- DDD の依存性逆転を維持する。`domain/refreshtoken/repository.go` の `Repository` ポートをドメインが宣言し、`infra/postgres` と `infra/memory` が同格で実装する(切替可能)
- 単回使用の正しさ: 同一 refresh token に対する並行リフレッシュでちょうど 1 つが成功し、他は再利用として family 失効に至る(atomic Rotate)
- スキーマは goose、クエリ→型安全 Go は sqlc で単一ソースから生成しコミットする(drift なし)。既存 `sqlc-drift.yml` / `auth-integration` CI でカバーする
- refresh token TTL は 30 日。ローテーションごとに TTL をリセット(スライディング)。family の絶対上限は設けない

### スコープ外(やらないこと)

- `offline_access` スコープによる発行ゲート・同意画面(OIDC §11)。本 Spec は refresh_token グラント対応 client に常に発行する
- 送信者制約 refresh token(mTLS RFC 8705 / DPoP RFC 9449)。public client 保護はローテーション + 再利用検知で満たす
- confidential client の `client_secret` 認証(AUTH-001 から継続して未実装。`token_endpoint_auth_methods_supported: ["none"]`)
- トークン失効エンドポイント(RFC 7009 `/revoke`)。失効は再利用検知による自動 family 失効のみ
- app/web(SPA)側の refresh token 保存・自動更新ロジックの実装(本 Spec は認可サーバー側のみ)

## 4. 設計(どう実現するか)

### 方針

既存の `authorization_code` の縦切り(domain 集約 + ドメイン宣言ポート + `infra/memory` 実装 + `infra/postgres` 実装 + sqlc + goose + 共有契約テスト)を踏襲し、新しい永続化集約 `refreshtoken` を追加する。これにより impl-auth(domain / service / route)と impl-db(migration / sqlc / infra/postgres)の担当分割が既存パターンと一致する。詳細な実装手順・変更ファイル・テスト戦略は `docs/plans/SPEC-006-plan.md`(planner が作成)に置く。

### アーキテクチャ / データ / インターフェース

- **ドメイン `domain/refreshtoken/`**: 集約 `RefreshToken`(非公開フィールド `tokenHash` / `familyID` / `clientID` / `userID` / `scope` / `expiresAt` / `consumed`)。VO `Token`(平文 opaque、`Hash() TokenHash`)/ `TokenHash`(SHA-256 hex、検索キー)/ `FamilyID` / `Scope`(部分集合チェック `Narrow`)。振る舞い `Issue` / `Rotate` / `MatchesClient` / `IsExpired`。sentinel エラー `ErrNotFound` / `ErrExpired` / `ErrReused` / `ErrClientMismatch` / `ErrInvalidScope` / `ErrInvalidToken`
- **ポート `Repository`**: `Save` / `FindByTokenHash`(consumed でも返す)/ `Rotate`(1トランザクションで旧 consume + 新 insert、競合負けは `ErrReused`)/ `RevokeFamily`
- **データ(新テーブル `refresh_tokens`)**: `token_hash`(PK)/ `family_id`(index)/ `client_id` / `user_id` / `scope` / `expires_at`(timestamptz)/ `consumed`(default false)/ `created_at`。schema は `search_path`(auth)で分離、DDL は unqualified
- **HTTP インターフェース**: `POST /token` に `grant_type=refresh_token` を追加。リクエスト `grant_type` / `refresh_token` / `client_id` / 任意 `scope`。レスポンスは既存 `TokenResponse` に `refresh_token`(omitempty)を追加。`/.well-known/openid-configuration` の `grant_types_supported` に `refresh_token`

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| ローテーションなし(1本の refresh token を再利用) | public client では RFC 9700 §2.2.2 がローテーションか送信者制約を必須とするため不可 |
| 再利用検知なし(ローテーションのみ) | 盗用された旧トークンの検知・失効ができず §4.14 の推奨を満たさない |
| refresh token を平文で DB 保存(authorization_codes と同型) | 長寿命クレデンシャルは DB 漏洩時のリスクが大きい。SHA-256 ハッシュ保存を採用 |
| `offline_access` スコープでゲート(OIDC §11 準拠) | scope / 同意処理が増えサンプルとして冗長。refresh_token グラント対応 client に常に発行する方針を採用 |
| ID token を再発行しない(access token のみ) | SPA がユーザー情報を更新しづらい。OIDC §12.2 準拠で再発行を採用 |
| refresh token を JWT で自己完結(ステートレス) | 個別失効・再利用検知・family 失効ができない。opaque + 永続化 + ローテーションを採用 |

## 5. 実装計画

詳細は `docs/plans/SPEC-006-plan.md`(planner 作成済み)。orchestration.md のパイプラインに従い admin が各フェーズを subagent に委譲する。既存の authorization_code 縦切り(domain 集約 + ドメイン宣言ポート + `infra/memory`/`infra/postgres` + sqlc + goose + `infra/repotest` 共有契約テスト)を雛形に、永続化集約 `refreshtoken` を追加する。

計画で確定した並列実行の 2 前提(計画 §付録 A / B):

- **ポート署名(A)**: `domain/refreshtoken/repository.go` は `Save` / `FindByTokenHash`(consumed でも未期限なら返す・期限切れは lazy evict で `ErrNotFound`)/ `Rotate`(旧 consume + 新 insert を atomic。既 consumed=`ErrReused`→family 失効、不在/期限切れ=`ErrNotFound`)/ `RevokeFamily` の 4 メソッド。impl-auth が domain で宣言、impl-db が memory/postgres で実装。
- **main.go 分担・順(B)**: 関数境界で分割し **impl-auth → impl-db** に直列化。impl-auth = `buildDemoClient` の grant に `"refresh_token"` 追加、impl-db = `setupPersistence`(署名+両arm)と `run()` 配線(戻り値受け取り + `NewAuthorizationService` 引数追加)。

- [x] T1: planner が `docs/plans/SPEC-006-plan.md` を作成し、`domain/refreshtoken/repository.go` のポート署名と `cmd/authz/main.go` の編集分担(impl-auth / impl-db)を確定する
- [x] T2: tester が R1–R10 に対応する失敗テストを先行作成(domain 単体 + 契約テスト + route 統合)
- [x] T3: impl-auth が domain/refreshtoken + service(grant switch・発行・リフレッシュ)+ route + discovery + demo client grant を実装(R1–R7, R9, R10)
- [x] T4: impl-db が migration + queries + sqlc 再生成 + infra/postgres + infra/memory + 共有契約テストを実装(R4, R5, R8)
- [x] T5: tester がテストを green にし、checker が `make check` を通す
- [x] T6: review-security / review-performance / review-spec が §3 準拠チェックリストと R マッピングを検証。Blocker/Major は差し戻し、今回対応しない指摘は issue-creator が Issue 化
- [x] T7: 価値の検証方法を満たしたことを経緯に記録し status を `done` に更新

## 6. 経緯(時系列・追記のみ)

### 2026-07-10

- 初版作成。AUTH-001 実装計画で将来拡張点(スコープ外)とされていた `grant_type=refresh_token` の実装を起点とする。RFC 6749 §6 / RFC 9700 / OIDC Core §12 を参照
- 設計判断をユーザーと確定し `approved` とした: (1) ローテーション + 再利用検知(family 一括失効、public client なので RFC 9700 §2.2.2 で必須)、(2) refresh_token グラント対応 client に常に発行(offline_access ゲートなし)、(3) SHA-256 ハッシュのみ DB 保存、(4) リフレッシュ時に access + ID token を再発行(OIDC §12.2)
- plan-mode 計画を `/Users/s.ryousuke1/.claude/plans/velvety-mapping-cocoa.md` に作成しユーザー承認済み。以降 planner が `docs/plans/SPEC-006-plan.md` に詳細化する
- planner が `docs/plans/SPEC-006-plan.md` を作成(T1 完了)。並列実行の 2 前提を確定: (A) `domain/refreshtoken/repository.go` のポート署名(`Save`/`FindByTokenHash`/`Rotate`/`RevokeFamily`)と各メソッドの sentinel 契約(特に `Rotate` の `ErrReused`(既 consumed=再利用→family 失効)と `ErrNotFound`(不在/期限切れ)の区別)、(B) `cmd/authz/main.go` の関数単位の編集境界と直列適用順 **impl-auth → impl-db**(impl-auth の service 署名変更が先に入らないと `run()` 配線がコンパイルできないため)。§5 に要約を反映
- TDD で実装を完了(T2–T5)。tester が R1–R10 の失敗テスト(domain 単体 + `infra/repotest` 共有契約 + route 統合)を先行作成 → impl-auth が `domain/refreshtoken`・service(grant switch / `authorizationCodeGrant` での発行 / `refreshTokenGrant`)・route・discovery・demo client grant を実装 → impl-db が migration `000002` + `refresh_tokens.sql` + sqlc 再生成 + `infra/postgres`・`infra/memory`・契約バインディング + `main.go` 配線を実装。付録 A のポート署名・付録 B の直列順を逸脱なく適用。checker が `make check`(fmt-check/lint/vet/build/test)+ `make test-race` green、sqlc drift ゼロを確認(生成物は working tree に反映済み・コミットは未実施)。impl-db が実 Postgres で `make test-integration`(20 並行レーサーの `Rotate` 契約含む)と migration up→down→up の健全性も green を確認
- レビュー 3 種を並列実施(T6)、**Blocker/Major なし**: review-security = §3 準拠チェックリスト 8 項目すべて OK(Minor 1: 再利用検知パスのタイミングサイドチャネル、理論上・実害限定)、review-performance = Blocker/Major なし(Minor 2: consumed 行の定期 GC 欠如 / memory `RevokeFamily` の O(n)、いずれもサンプル規模で許容)、review-spec = R1–R10 全充足・done 可(Minor 1: 空 `refresh_token` の境界テスト欠如)
- 指摘対応(T7): 空 `refresh_token` の境界テスト(`TestRefreshToken_EmptyToken_InvalidGrant`、`invalid_grant` + no-cache ヘッダ)を tester が追加し `make test` green。deferred hardening(consumed 行の定期 GC 欠如 + タイミングサイドチャネル)を **[[ISSUE-019]]** に起票(frontmatter 相互リンク済み)。R1 に空トークン時の `invalid_grant` 挙動を明記
- **価値の検証方法を充足**(`make check` green + review-security が §3 準拠チェックリストを確認)につき `status: done` に更新して完了。残: git コミットはユーザー判断(未実施)、`.claude/rules/db.md` の「api/auth 同一 database / search_path」記述と working tree の migrator リファクタ(別 database + `app/migrator`)の drift は本 Spec のスコープ外・別途要確認
