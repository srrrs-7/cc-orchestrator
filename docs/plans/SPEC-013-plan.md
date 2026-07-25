# SPEC-013 実装計画: テストの実 DB 一本化と専用テスト DB 分離(手書きダブル廃止)

起点: `docs/specs/20260711-013-unify-tests-real-db-test-databases.md`(R1–R7・スコープ外・確定事項)
関連: SPEC-005 / SPEC-009(R3→R6 改訂)/ SPEC-010(2 プール)/ SPEC-011(Postgres 一本化)

この計画は **テスト戦略のみ** を変更する。DB スキーマ・sqlc 生成・ドメインポート・HTTP/OpenAPI 契約・
実行時 env 契約・本番 runtime の挙動は一切変えない(SPEC-013 非機能要件「公開契約不変」)。

---

## 1. 方針

### 1.1 データ隔離方式(R4): 既存 truncate ベースの延長を採用

**採用: テスト毎 TRUNCATE + seed による隔離(既存 `testsupport.Truncate*` を延長)。並列競合は DB 到達テストを含む `go test` 実行を `-p 1`(パッケージ間シリアライズ)で回すことで防ぐ。**

決め手は「**既存の `//go:build integration` テストがすでにこの方式で、かつ最も難しい atomic / 複数コネクション操作を緑にしている**」という実証的事実:

- `app/auth/route/helpers_integration_test.go` の `newTestHandlerWithDB` は毎テストで 4 テーブル truncate + 再 seed し、その上で `authcode.Consume` / `refreshtoken.Rotate` / `RevokeFamily` / `token_concurrency`(同一テスト内の並行リクエスト)を検証している。
- `app/{api,auth}/infra/postgres/reader_writer_pool_integration_test.go` は `OpenPair` が開く **2 本の独立プール** に対し `testsupport.TruncateTasks` で隔離している。
- つまり truncate は、SPEC が「単一トランザクションロールバックだと検証できない可能性」と指摘した操作群と **既に共存できている**。方式を変えず tag を外すだけでこれらがそのまま `make test` に載る。

**退けた代替案:**

| 案 | 退けた理由 |
|---|---|
| 単一トランザクションロールバック | (a) `OpenPair` は writer/reader で**別プール**を開くため 1 トランザクションで跨げない。(b) `authcode.Consume` / `refreshtoken.Rotate` / `RevokeFamily` は**実コミット**とコネクション跨ぎの atomic 性そのものを検証対象にしており、外側 tx で包むとコミット挙動を隠蔽する上、リポジトリポート(`ctx` を取り tx ハンドルを取らない)にロールバック用 tx を通す術がない。**最も実 DB 検証したい操作が検証不能**になり本末転倒。 |
| 一意 ID ネームスペース(truncate しない) | `ListPage`/`CountTasks` は**全件 total** を返すため、残存行があると `Total==N` 系アサーションが壊れる。auth フローはハンドラ配線が固定 demo client/user ID を seed・消費するため、per-test ランダム化は共有ヘルパ(`newTestHandler`)の全面書き換えを要する。truncate の「空→seed の確定的ベースライン」に依存する既存アサーションと相性が悪い。 |
| テスト毎に別 DB を作成 | 作成/マイグレーションのコストが per-test で発生し実行時間が非現実的。 |

**決定性の担保(rules/testing.md 準拠):**
- **実行順非依存**: 各テストは冒頭で truncate + seed するので、直前に何が走ったかに依存しない。
- **並列安全**: api/auth 全体を grep しても `t.Parallel` は 0 件(パッケージ内並行なし)。残る競合源は `go test ./...` の**パッケージ間並行**(service / route / infra/postgres が同じ `api_test.tasks` を同時 truncate)のみ。これを **`-p 1`** で封じる(既存 `test-integration-native` が既に `go test -tags=integration -p 1` としているのと同型。今回は無タグの `test-native` に `-p 1` を移す)。api と auth は別 DB(`api_test` / `auth_test`)なので stack 間競合は無い。

### 1.2 R2 の異常系ダブル線引き(核心)

原則: 「**DB を代替して DB の振る舞いを検証しているダブル**」は実 test DB に置換する。「**DB が表現できない別の軸(ルーティング観測 / 到達不能な fallback 分岐 / 非到達の証明)を検証しているダブル**」は理由付きで最小限残す。異常系カバレッジは後退させない。

判定の技術的根拠(調査で確定):
- **api の実リポジトリ(`task_reader.go`/`task_repository.go`)は非 NotFound の DB エラーを必ず `task.NewDBError(err)` にラップする。** 汎用の「非カテゴリエラー」は実リポジトリからは**出得ない**。よって `DBError→500` は実 DB 誘発可能だが、`writeError` の **default(未分類)分岐**は実 DB では到達不能。
- auth の untagged ダブル(`stubClientOnlyRepo`/`alwaysNotFoundAuthCodeRepo`)は「client あり + code 無し」という**実 DB がそのまま再現できる状態**を作っているだけなので全て置換可能。
- `panicRefreshTokenRepo` は**定義のみで未使用**(grep 済み)。

判定表は §4。

### 1.3 test DB 導線(R3): migrator 本体は無改変

`app/migrator/cmd/migrator/env.go` の `Env.validate` は `DB_NAME` を読み、未設定なら `-target` 名を既定にする。`domain/migration.ParseDatabaseName` の識別子検証 `^[a-z_][a-z0-9_]*$` は underscore を許すため `api_test` / `auth_test` はそのまま通る。したがって **migrator のコード変更は不要**。`DB_NAME=api_test`(`-target api`)/ `DB_NAME=auth_test`(`-target auth`)を注入する薄い導線(ルート `make migrate-test`)を足すだけでよい(role provisioning の `APP_DB_USER`/`APP_DB_PASSWORD` はテストでは未設定=スキップ)。

### 1.4 make check / pre-commit / CI 統合(R5)+ SPEC-009 R6 network

- 各 stack の `test`(=`check` の一部)は **test DB 到達可・インターネット非到達**の実行経路で回す。`fmt-check`/`lint`/`vet`/`build` は従来どおり offline(`tools-offline`)を維持する(R6:「テストフェーズのみ緩和」)。よって host 側 `check` は **offline フェーズ + DB フェーズの 2 経路**に分解する。
- SPEC-009 の network 二層(`tools`=full / `tools-offline`=none)に **第 3 層 `tools-db`(postgres 到達可・internet egress なし=`internal: true` network 上)** を追加する(compose.tools.yml)。テストフェーズはこれを使う。
- CI は `api-integration`/`auth-integration` の provisioning(db-up + migrator + up→down→up health check)を **`api`/`auth` の `check` ジョブへ統合**し、統合済みの旧 job を削除する。`migrator-integration`(ISSUE-016 の権限境界テスト)は **本 SPEC のスコープ外**(api/auth の DB リポジトリではなく migrator 自身の role provisioning を検証する別関心)なので、その `//go:build integration` タグと job は**そのまま残す**。
- pre-commit(`.githooks`)は Go DB stack がステージされたとき、check の前に db-up + migrate-test を行い、test フェーズを DB 到達経路で回す。

### 1.5 t.Skip / REQUIRE_DB 安全弁(確定仕様・admin 裁定)

`OpenTestDB` は **`REQUIRE_DB` の有無で分岐**する(admin 裁定で確定):

- **`DB_HOST` 未設定 かつ `REQUIRE_DB=1` → `t.Fatal`**(正規経路。DB テストが黙って skip されて緑に見える事態を封じる)
- **`DB_HOST` 未設定 かつ `REQUIRE_DB` 未設定 → 従来どおり `t.Skip`**(素の ad-hoc `go test ./...` は graceful skip 維持)
- `DB_HOST` 設定時は通常どおり接続

**正規経路(CI `check` / pre-commit の DB フェーズ / ルート経由の test)は `REQUIRE_DB=1` を注入する**(担当: impl-ci が Makefile/CI/hook 側で、impl-db が `OpenTestDB` の分岐実装を)。理由: SPEC-013 の「全テスト実 DB」の意図を守り、DB_HOST 取り違え等で DB テストが黙って skip され緑に見える事態を正規経路で防ぐため。ad-hoc ローカル実行(DB を用意しない素の `go test`)は従来どおり skip して退避する二段構えとする。

---

## 2. 変更ファイル(stack 別・担当 agent 明記)

### impl-db(`infra/postgres` / `testsupport` / migrator 導線 / `infra/repotest` コメント)

| ファイル | 変更 |
|---|---|
| `app/api/infra/postgres/testsupport/db.go` | `//go:build integration` 削除。パッケージ doc から「never compiled in default make test」「SPEC-011 Phase 2」等の記述を削除・更新。`TestConfig` の `DB_NAME` 既定を `api` → **`api_test`**。`Truncate*` 隔離ヘルパをここへ集約(必要なら汎用 truncate を追加)。`OpenTestDB` の DB_HOST 未設定時挙動を **`REQUIRE_DB=1`→`t.Fatal` / 未設定→`t.Skip`** の分岐へ(§1.5・admin 裁定)。 |
| `app/auth/infra/postgres/testsupport/db.go` | 同上(`OpenTestDB` の REQUIRE_DB 分岐含む)。`DB_NAME` 既定 `auth` → **`auth_test`**。`TruncateTable`/`SeedClient`/`SeedUser` は維持。 |
| `app/api/infra/postgres/task_repository_integration_test.go` | `//go:build integration` 削除。「integration」を指す in-file コメント整理。ファイル名の `_integration` 除去は任意(churn 最小なら据え置き可・load-bearing はタグ削除)。 |
| `app/api/infra/postgres/reader_writer_pool_integration_test.go` | 同上(「TDD red phase / 未実装」等の陳腐化コメントも整理)。 |
| `app/auth/infra/postgres/{authcode,client,refreshtoken,user}_repository_integration_test.go` | 各 `//go:build integration` 削除・コメント整理。 |
| `app/auth/infra/postgres/reader_writer_pool_integration_test.go` | 同上。 |
| `app/api/infra/repotest/task_contract.go` | 実態と乖離した「integration のみで実行」等のコメントを「default `make test` の一部として実 DB(`api_test`)で実行」へ更新(R7)。 |
| `app/auth/infra/repotest/{authcode,user,client,refreshtoken}_contract.go` | 「SPEC-011 完了: infra/memory 削除済み … Postgres(integration)のみで実行」の **死んだ「infra/memory」/「(integration)」記述**を実態へ更新(R7)。 |

> 注: **migrator 本体(`cmd`/`domain`/`infra`/`config`)は無改変**(§1.3)。migrator の `//go:build integration`(権限境界テスト)は**スコープ外・据え置き**。

### impl-api(`app/api` の service/route テスト + api Makefile)

| ファイル | 変更 |
|---|---|
| `app/api/service/task_service_test.go` | **`fakeRepository` を削除**(機能系・spy の backing 双方から撤去。in-memory fake backing は禁止)。`fakeRepository` / `stubListPageRepository` の機能系 CRUD/遷移/ページングは実 `postgres.New*Repository`(`api_test`)へ置換。ページング echo は挿入行数ベースへアサーション調整。invalid limit/offset(バリデーション先行=DB 非依存)は実リポジトリ配線+エラー検証へ。**`readerSpy`/`writerSpy` は実 `postgres.NewTaskReader(db)`/`NewTaskWriter(db)`(`api_test`)をラップする薄い計数デコレータへ作り替えて残す**(狭いメソッド集合のみ露出→カウンタ++→実リポジトリへ委譲。`reader==writer==api_test`)。`TestTaskService_RoutesReaderAndWriter` はこのデコレータでルーティング観測 + narrow-port 型証明を維持(§4・例外2)。 |
| `app/api/route/task_handler_test.go` | `notFoundRepository` → 実 DB 空テーブルの 404 へ置換・削除。`dbErrorRepository` → 実 DB 誘発(プール close / ctx cancel)で `*task.DBError`→500 へ置換・削除。バリデーション先行テスト群(invalid priority / empty title / malformed JSON / invalid query)は実 DB backed ハンドラへ配線替え。**`failingRepository` は「未分類エラー→writeError default 分岐」用の最小スタブ 1 つだけ残す(§4)**。整理後に無タグのまま default test に載る。**冒頭・末尾の「untagged (offline) / `//go:build integration` counterpart / SPEC-011 build-tag split」等の陳腐化 package doc コメントを実態(統合済み・実 DB)へ更新(R7)。** |
| `app/api/route/task_handler_integration_test.go` | `//go:build integration` 削除。offline 側と統合し重複ヘルパ(`doRequest` 等)を整理。 |
| `app/api/Makefile` | `test-native`: `go test ./...` → **`go test -p 1 ./...`**。`test-integration`/`test-integration-native` 削除(test に統合)。`DB_NAME ?= api` → **`api_test`**。host 側 `check`/`test` を「offline フェーズ(fmt-check+lint+vet+build)+ DB フェーズ(test、`tools-db` 経由・`-e DB_*` 注入)」の 2 経路へ分解。 |

### impl-auth(`app/auth` の route テスト + auth Makefile)

| ファイル | 変更 |
|---|---|
| `app/auth/route/helpers_test.go` | `stubClientOnlyRepo` / `alwaysNotFoundAuthCodeRepo` を削除(→ 実 DB seed + 空テーブルで再現)。`newTokenErrorTestHandler` を実 DB backed(`newTestHandler` ベース)へ差し替え。**`panicRefreshTokenRepo` は未使用のため削除(R7)**。**`newDiscoveryTestHandler`(nil-repo)は理由付きで残す(§4)**。**冒頭の「SPEC-011 build-tag split / untagged / helpers_integration_test.go(`//go:build integration`)」を説明する package doc コメントを統合後の実態へ更新(R7)。** |
| `app/auth/route/security_test.go` | `TestToken_ErrorResponse_HasNoCacheHeaders` を実 DB backed ハンドラ(client seed 済み + code 不在→invalid_grant)へ。wrong-aud/iss 401 は `newDiscoveryTestHandler` のまま。**冒頭の「offline (untagged) / 実 DB 不要」旨の陳腐化コメントを更新(R7)。** ヘッダ/comment 更新。 |
| `app/auth/route/discovery_test.go` | `newDiscoveryTestHandler` 継続(nil-repo。§4)。**冒頭の offline/untagged 前提コメントを実態へ更新(R7)。** |
| `app/auth/route/helpers_integration_test.go` | `//go:build integration` 削除。offline 側 helpers と統合(名前衝突なし)。「build tag keeps them out of default test」等の陳腐化コメント削除。 |
| `app/auth/route/{authorize_flow,token_concurrency,token_user_not_found,refresh_token,discovery_integration,authorize_open_redirect}_test.go` | 各 `//go:build integration` 削除・コメント整理。 |
| `app/auth/Makefile` | api と対称: `test-native` に `-p 1`、`test-integration*` 削除、`DB_NAME ?= auth` → **`auth_test`**、`check`/`test` の 2 経路分解。 |

### impl-ci(横断ツーリング: ルート Makefile / compose / CI / hooks)

| ファイル | 変更 |
|---|---|
| `Makefile`(ルート) | **`migrate-test`** 追加(`db-up` 前提 + migrator を `DB_NAME=api_test`(-target api)/ `DB_NAME=auth_test`(-target auth)で 2 回実行。`APP_DB_*` は渡さない)。必要なら `test-db-up` 等の便宜集約。 |
| `.devcontainer/compose.tools.yml` | **`tools-db` サービス追加**(postgres 到達可・internet egress なし)。`internal: true` の専用ネットワークを定義し tools-db を接続(postgres 側の接続は compose.yml で調整)。ヘッダの network 二層説明を三層へ更新(SPEC-009 R6)。 |
| `compose.yml` | 上記 `internal` network に `postgres` を参加させる(tools-db から service 名解決するため)。既存の api/auth/web 配線・公開ポートは不変。 |
| `.github/workflows/cicd.yml` | `api`/`auth` の `check` ジョブに **db-up → migrate-test(api_test/auth_test)→ up→down→up health check → `make check`(DB 到達経路)** を統合。`api-integration`/`auth-integration` job を削除。`changes` フィルタの「api/auth は offline / DB 不要」旨コメントを更新。**`migrator-integration` は不変**。 |
| `.githooks/lib/run-checks.sh`(＋必要なら `common.sh`/`detect-stacks.sh`) | `NEED_API`/`NEED_AUTH` 時に stack check の前段で db-up + migrate-test(該当 DB)を実行し、test フェーズを DB 到達経路へ。ヘッダ/`Makefile` の「Integration jobs are excluded (Postgres required)」旨を更新。 |

### admin(`.claude/rules` / SPEC-009 / CLAUDE.md。T6)

| ファイル | 変更 |
|---|---|
| `.claude/rules/testing.md` | 「2 層構成(untagged offline + integration)」節を **「DB テストは default `make test` で `api_test`/`auth_test` に対して実行。純ロジック/env 写像は DB 非依存で対象外」** の一本化モデルへ書換。契約テストの「integration タグ付き実 DB」記述も更新。 |
| `.claude/rules/db.md` | コマンド表から `test-integration` 行を削除・`migrate-test` を追記。CI 節(`api-integration`/`auth-integration`→`check` 統合)。DI 差し替えの「integration ビルドで通す」記述更新。 |
| `docs/specs/20260710-009-...` (SPEC-009) | §4 + 経緯に R3→R6 改訂(テストフェーズのみ DB 到達可・internet 非到達)。**spec skill 経由・admin**。 |
| `CLAUDE.md` | 「永続化」「コマンド早見表」注記(test-integration 廃止・test DB・test フェーズ network)。 |

---

## 3. 手順(実行主体と並列可否)

TDD/レビュー順序(`workflow.md`)を守る: tester(設計/棚卸し)→ impl(移行)→ tester(検証)→ checker → review。

1. **T1 planner(本書)** — 完了。
2. **T2 tester** — 現行 DB 関連テストの**異常系アサーション棚卸し**(移行後に落とさない受け入れ基準)と、§4 判定表に沿った移行後テスト設計を確定。ファイル編集は行わず、impl が満たすべき「移行後に各テストが何をアサートすべきか」の一覧を提示(実質「テスト先行」の成果物)。
3. 以下を **並列起動**(scope 独立)。ただし **T4a/T4b は T3 の testsupport(タグ削除 + `DB_NAME` 既定 `*_test`)を前提**に compile するため、T3 の testsupport 変更を先行着地させる:
   - **T3 impl-db** — testsupport のタグ削除・`DB_NAME` 既定変更・隔離ヘルパ集約、`infra/postgres/*` テストのタグ削除、`infra/repotest` の死んだコメント整理。migrator は無改変。
   - **T5 impl-ci** — ルート `migrate-test`、compose.tools.yml `tools-db` + internal network、compose.yml network、cicd.yml 統合、`.githooks` provisioning。
   - **T4a impl-api / T4b impl-auth**(相互に並列。T3 testsupport 後) — 各 stack の service/route テスト移行(ダブル置換/削除、実 DB 誘発化、残す KEEP ダブルの明文化)、route テストのタグ削除、各 stack Makefile の `test`/`check`/`DB_NAME`/`-p 1` 調整。
4. **T6 admin** — `.claude/rules/testing.md` / `db.md`、SPEC-009(§4+経緯 R3→R6)、CLAUDE.md 更新。
5. **T7 checker + tester** — 両 stack で `make check`(test DB 経路)緑を確認。**実行順非依存**(2 回連続実行 / 別順序)と **`-p 1` 並列安全**、`api_test`/`auth_test` に閉じ **開発用 `api`/`auth` 非汚染**、`app/api`・`app/auth` に `//go:build integration` が残っていない(grep)こと、CI 緑を検証。
6. **T8 review-security / review-performance / review-spec**(並列) — §5 の観点。今回対応しない指摘は issue-creator が起票。

---

## 4. R2 異常系ダブル判定表(移行後カバレッジ非後退の中核)

R2 例外は SPEC-013 改訂(2026-07-11 裁定)で 2 類型化された: **例外1**=実 DB では現実的に誘発できない障害系(汎用 DB エラー→HTTP マッピング等)、**例外2**=実 DB が観測できない実装 seam(service→ポート振り分け・narrow-port 型証明)を検証する計装。さらに **「in-memory fake を backing に残すことは禁止」** が明文化された(データ経路は必ず実 test DB)。

| stack/場所 | ダブル | 現在の検証対象 | 判定 | 移行後の担保 |
|---|---|---|---|---|
| api service | `fakeRepository` | CRUD/遷移/重複の機能系 + spy の backing store | **削除** | 機能系は実 `postgres.NewTaskRepository`(`api_test`)へ置換(truncate + seed)。spy の backing も実 DB backed の計装デコレータ(下行)へ移すため、in-memory fake は完全撤去(例外2 でも fake backing は禁止)。 |
| api service | `stubListPageRepository` | `Total`≠`len(items)` の echo・Page passthrough | **置換** | `api_test` に既知行数を挿入し、返却ページ+echo された `Total` を検証。white-box「渡された Page」→結果ベースへ。 |
| api service | `readerSpy` / `writerSpy` | **どのポート(reader/writer)を service が呼ぶか**(SPEC-010 R2 ルーティング証明)+ narrow-port の compile-time 型証明 | **残す(実 DB backed の計装デコレータ・例外2)** | 実 DB は「どのプールにクエリが行ったか」を表現できない固有カバレッジ。`readerSpy` を **実 `postgres.NewTaskReader(db)`(`api_test`)をラップし呼び出しを数える薄い計装デコレータ**へ、`writerSpy` を **実 `postgres.NewTaskWriter(db)` をラップする計数デコレータ**へ作り替える。狭いメソッド集合のみ露出(reader=`Find*`/`ListPage`、writer=`Save`)し、カウンタ++ 後に実リポジトリへ委譲。両方 `reader==writer==api_test` を指す。**in-memory fake は backing に残さない**。`TestTaskService_RoutesReaderAndWriter` はこのデコレータで維持し、ルーティング観測 + narrow-port の compile-time 型証明を保全。担当 impl-api。 |
| api route | `notFoundRepository` | 404(事前データ不要) | **置換** | 実リポジトリ + 空 `api_test` で存在しない ID → `ErrNotFound`→404。ダブル削除。 |
| api route | `dbErrorRepository` | `*task.DBError` カテゴリ→500 | **置換(実 DB 誘発)** | 実リポジトリはドライバエラーを必ず `task.DBError` にラップ(`task_reader.go` 確認済み)。プール close / ctx cancel で実 `*task.DBError`→500。ダブル削除。 |
| api route | `failingRepository` | **未分類エラー→`writeError` default(fallback)分岐→500** | **残す(意図的・最小 1 個)** | 実リポジトリは未分類エラーを**出し得ない**(常に DBError/NotFound 分類)。fallback 分岐は実 DB では到達不能。`errors.New` を返す最小スタブ 1 つで当該 wire-contract のみ検証。バリデーション先行テスト群はこのスタブから外し実 DB backed ハンドラへ移す。 |
| auth route | `stubClientOnlyRepo` | client あり→後続の検証 | **置換** | `testsupport.SeedClient` で `auth_test` に seed。実 `NewClientRepository` は未知 ID に `ErrNotFound`。ダブル削除。 |
| auth route | `alwaysNotFoundAuthCodeRepo` | code 不在→`invalid_grant` | **置換** | 実 `NewAuthCodeRepository` + 空 `authorization_codes` → `FindByCode` が `ErrNotFound`→invalid_grant。ダブル削除。 |
| auth route | `panicRefreshTokenRepo` | (なし) | **削除(R7)** | 定義のみで未使用(grep 済み)。カバレッジ損失なし。 |
| auth route | `newDiscoveryTestHandler`(nil repos) | discovery / JWT の iss・aud 検証(**repo 到達前に失敗**) | **残す(例外2 相当・nil repos)** | DB 代替ではなく「これらの経路は永続化に触れない」証明(触れれば nil-panic で顕在化)。in-memory fake ではなく nil なので backing 禁止規定に抵触しない。実 DB を要さない。コメントで意図を明記。 |

**異常系サマリ**: `DBError→500` は実 DB 誘発へ、`fallback→500` は最小スタブ、`404` は実空テーブル、`invalid_grant` は実空 authcode、`wrong-aud/iss→401` は nil-repo(不変)。**いずれの異常系も等価以上で保持**。
**残すダブル(全て backing に in-memory fake を持たない)**: api `readerSpy`/`writerSpy`(実 DB backed 計装デコレータ・例外2)、api `failingRepository`(未分類エラーを返す最小スタブ 1 個・例外1)、auth `newDiscoveryTestHandler`(nil repos・例外2 相当)。

---

## 5. テスト戦略

- **方式**: 既存テストの**移行**(挙動保存)。T2 tester が「移行後アサーション一覧 + §4 判定表」を先に確定し(TDD 相当の受け入れ基準)、impl が満たす。新規機能テストではないため、既存アサーションの意味を落とさないことが合格条件。
- **レベル別**:
  - infra/postgres リポジトリ・`OpenPair` 2 プール・`infra/repotest` 契約: 実 `api_test`/`auth_test` に対する統合テスト(タグ削除で default 化)。
  - service: 実リポジトリ配線での機能系 + ルーティング証明(spy 残置)。
  - route: 実 DB backed の正常系/全フロー + 異常系(実誘発 or 最小スタブ)。
  - 純ロジック(domain 状態遷移・VO)・`cmd/*/env.go` の env→Config 写像(`t.Setenv`)・`persistence_selection_test.go`(`Config.DSN`/`Validate`)は **DB 非依存=対象外**(スコープ外。実 DB 化しない)。
- **隔離**: 毎テスト truncate + seed。DB 到達テストを含む run は `-p 1`。`t.Parallel` を新たに足さない。
- **観点**: 正常系/異常系/境界値を維持。特に異常系(§4)を非後退で保つ。
- **要件トレーサビリティ**:
  - R1(タグ廃止)→ T3/T4a/T4b の全 `_integration_test.go` タグ削除、T7 の grep 検証。
  - R2(ダブル置換)→ §4 判定表、T4a/T4b。
  - R3(専用 test DB)→ T3(testsupport 既定 `*_test`)、T5(`migrate-test`)、T7(dev DB 非汚染検証)。
  - R4(隔離)→ §1.1、T7(順序非依存・並列安全)。
  - R5(check/CI/hook 統合)→ T4a/T4b(Makefile)・T5(root/CI/hook)、T7(CI 緑)。
  - R6(SPEC-009 network)→ T5(`tools-db`+internal net)、T6(SPEC-009 更新)、T8 review-security。
  - R7(不要コード/コメント削除)→ T3(repotest/testsupport)、T4a/T4b(`panicRefreshTokenRepo` 削除 + route の「offline/integration 2 層構成」を説明する陳腐化 package doc コメント更新: `task_handler_test.go` 冒頭・末尾、auth `helpers_test.go`/`security_test.go`/`discovery_test.go` 冒頭)、T6(rules)。

---

## 6. リスク / 未確定事項

> 裁定記録: readerSpy/writerSpy を実 DB backed 計装デコレータで残す方針(R2 例外2)は SPEC-013 §6(2026-07-11 追記)で確定済み。詳細はそちらを参照。

- **[確定前提] `-p 1` + 非 `t.Parallel()` を前提に truncate 隔離が成立している**: 本計画の truncate 隔離(§1.1)は「DB 到達 run を `-p 1` でパッケージ間シリアライズし、かつ DB テストに `t.Parallel()` を足さない」ことを**確定前提**とする(現状 api/auth に `t.Parallel` は 0 件)。この範囲では truncate + seed が実行順非依存・並列安全を満たし最適。
- **[将来リスク] `t.Parallel()` を導入すると truncate + 共有 DB は破綻する**: 同一テーブルを触る並行テストが互いの行を実行途中で truncate し合い flaky 化するため、将来 `t.Parallel()` を採るなら truncate 方式は捨てる。移行先は **「並列単位ごとの別 DB」**: worker 数ぶんの `api_test_<n>` / `auth_test_<n>`、または migrate 済みテンプレートからの `CREATE DATABASE api_test_xxx TEMPLATE <migrated>`(高速複製)。いずれも本計画で追加する `migrate-test`(migrator の `DB_NAME` 上書き)導線をそのまま流用できる。スキーマ分離(`CREATE SCHEMA` + `search_path`)は db.md の「別データベース分離・`search_path` 廃止」決定に反するため**再導入しない**。一意 ID ネームスペースは全件 total(`ListPage`/`CountTasks`)と固定 seed に反するため不採(§1.1)。
- **[確定仕様・admin 裁定] `make check` の DB 前提化と REQUIRE_DB 二段構え**: `DB_HOST` 未設定の素の `make -C app/api check` は DB テストを `t.Skip` するが、正規経路(CI `check` / pre-commit の DB フェーズ / ルート経由 test)は **`REQUIRE_DB=1` を注入し `OpenTestDB` を `t.Fatal` 化**する(§1.5)。これにより DB_HOST 取り違え等で DB テストが黙って skip され緑に見える事態を正規経路で封じつつ、ad-hoc ローカル実行は graceful skip を維持する。担当: `OpenTestDB` の分岐は impl-db、`REQUIRE_DB=1` の注入は impl-ci(Makefile/CI/hook)。
- **SPEC-009 R6 の network 実装機構(未確定・impl-ci 確定)**: 「postgres 到達可・internet 非到達」を `internal: true` network + `tools-db` サービスで表現する方針までは確定。compose.yml の `postgres` を internal network に参加させる具体配線(既存 project default network との共存・ポート公開への影響)は impl-ci が最終決定。`internal: true` で service 名解決が期待どおり効くかは実装時に要検証。
- **実行時間の増加(許容だが監視)**: `-p 1` によるパッケージ間シリアライズ + 実 DB I/O でテスト時間が増える。SPEC は許容。接続再利用(既存 `OpenTestDB` の `t.Cleanup` close は per-call。プール共有の余地は review-performance が評価)。過剰なら「DB 到達パッケージのみ列挙して `-p 1`、純ロジックは並列」の分割を後日検討(今回は単純さ優先で `go test -p 1 ./...`)。
- **pre-commit のコスト/前提変化**: これまで hook は「integration 除外(Postgres 不要)」だったが、本 SPEC 後は Go DB stack のコミットで db-up + migrate-test が走り Docker/Postgres 起動時間が加わる。docs-only 等は従来どおりスキップ。開発者体験の後退が大きい場合は hook で DB フェーズを任意化するフラグを検討(未確定・impl-ci/レビュー)。
- **`_integration` ファイル名リネームの要否**: タグ削除が load-bearing。ファイル名の `_integration` 除去は可読性向上だが git 履歴 churn を伴う。今回はタグ削除を必須・リネームは任意とし、impl の裁量(据え置き可)。
- **auth `newTestHandler` 経由の重複 helper 統合**: helpers_test.go(offline)と helpers_integration_test.go(integration)を 1 パッケージに統合する際、`doToken` 等の共有ヘルパと `newTestHandler`/`newDiscoveryTestHandler` の併存を整理する必要がある(名前衝突は現状なし。impl-auth が確認)。
- **migrator スコープ誤解の防止**: `app/migrator` 自身の `//go:build integration`(権限境界テスト・`migrator-integration` job)は**本 SPEC 対象外**。impl-db/impl-ci が誤って外さないこと(§1.4 / §2 注記)。
- **[review 申告 (a)] service 層に純オフライン unit が残らない**: R2 の「in-memory fake backing 禁止」を字義適用した結果、`app/api/service/task_service_test.go` は spy(計装デコレータ)も含め全ケースが実 `postgres.New*`(`api_test`)配線となり、**ファイル全体が DB 必須**化する。service 層に純オフラインのユニットテストは残らない(SPEC-013 の意図どおりだが、テスト時間・オフライン検証性の観点で **review-spec / review-performance が妥当性を確認**)。
- **[review 申告 (b)] 正常系の white-box 後退**: `TestTaskService_List_PagingAppliedAndEchoed` は現在「repo に渡された `Page`(`gotPage`)を直接検証する white-box」だが、実 DB 化により **black-box(`Total`/`Limit`/`Offset`/`len(Items)` からの間接証明)へ後退**する(渡した Page を直接覗けなくなるため)。R1〜R3 の clamp/default/echo は間接的に担保されるが、検証の直接性は下がる。許容範囲だが **review-spec が後退の可否を判断**(不足なら repo をラップした計装で Page を観測する案もあるが、§4 の例外2 拡大に当たるため今回は間接証明を既定とする)。
