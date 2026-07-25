# SPEC-011 実装計画: 永続化を Postgres 一本化し infra/memory を完全削除

- 起点: `docs/specs/20260710-011-persistence-postgres-only-drop-memory.md`(SPEC-011, status: approved)
- 種別: リファクタリング(挙動不変が絶対条件)+ テスト再編
- 関連: SPEC-005(Postgres 永続化)/ SPEC-010(CQRS reader/writer, `OpenPair`)/ SPEC-009(オフライン `make check` / toolchain コンテナ)/ ISSUE-018(route error category)/ ISSUE-019(auth user-not-found → invalid_grant)
- 関連 rules: `.claude/rules/db.md` / `.claude/rules/testing.md` / `.claude/rules/workflow.md`

---

## 方針

`infra/memory` が担っていた 2 役割(①実行時フォールバック / ②テストダブル)を両方 Postgres に寄せて廃止する。調査の結果、削除の波及は「実行時合成の単純化」より「テスト配線」に集中している(service 層は既に store 非依存で DI は達成済み)。

### 論点 1〜6 の結論(Spec §4 の planner 委任事項)

**論点 1: テスト用 Postgres コンテナの立て方(SPEC-009 を壊さない)**
- 新しいコンテナ基盤は作らない。既存の `compose.yml` の `postgres` サービス + 各スタック `Makefile` の `test-integration`(`DB_ONLINE` = `compose.yml` を層に重ね `tools` サービスから `postgres` をサービス名解決)経路をそのまま使う。これは既に SPEC-009 準拠(検査系は `--network none`、DB 到達フェーズのみ `tools` でネットワーク有効)。
- DB 依存テスト(リポジトリ contract / pool / **新規に移す route 統合テスト**)は `//go:build integration` に集約し、ネットワーク有効な `make test-integration` = integration ジョブでのみ実行する。オフラインの `make check`(`make test` = `go test ./...`、`--network none`)は build tag により integration ファイルを一切コンパイルしないため不変。

**論点 2: build tag 方針**
- **build-tag split** を採用する。route テストを以下 2 層に再編する:
  - **untagged(オフライン unit、`make check` に載る)**: 機能ストアを要しない検証系・エラー注入系。domain ポートを満たす **最小のテスト専用スタブ**(canned error を返す fake、`route_test` パッケージ内)で回す。これは `infra/memory` の復活ではない(汎用ストアではなく、`service_test` が既に採用する test-local fake と同じ思想。testing.md の「外部依存は interface 越しに fake へ」に合致)。
  - **`//go:build integration`(実 DB、`make test-integration` に載る)**: 機能ストアを要する正常系・全フロー(api の CRUD/一覧、auth の authorize→token→refresh→userinfo 等)。共有 test-DB ヘルパで実 DB に接続し、テーブル TRUNCATE + 実 `postgres.New*Repository` を router に配線して回す。
- 振る舞い契約テスト `Run<集約>RepositoryContract` は Postgres 実装のみで回す(memory バインディング削除)。contract test 自体は既に integration-tagged で存在するため追加の tag 作業は不要。

**論点 3: CI ジョブ再編**
- **新規ジョブは作らない**。既存の 2 層構造を維持する:
  - `api` / `auth` unit ジョブ(`make check`、オフライン): memory-backed unit test が消えるだけで構造は不変。offline route テスト(エラー注入系)は引き続きここで回る。
  - `api-integration` / `auth-integration` ジョブ: 既に `app/migrator` で DB 作成 + マイグレーション済み。`make test-integration` の対象を `./infra/postgres/...` から **`./infra/postgres/... ./route/...` へ拡張**することで、route 統合テストが同ジョブで実 DB に対して回る。ジョブ定義(`cicd.yml`)自体は `make test-integration` を呼ぶだけなので**原則変更不要**(impl-ci は path-filter と実行結果の妥当性のみ確認)。
- 「unit job も Postgres 化して全テストを実 DB で回す」案は SPEC-009 のオフライン不変条件を崩すため不採用(Spec §4 代替案表どおり)。

**論点 4: error injection の扱い**
- 「リポジトリが DB エラーを返す→ハンドラが 500」等、実 DB で再現しにくいパスは **test-local 最小スタブ** で対応する:
  - api: 既存の `failingRepository` / `dbErrorRepository`(`route/task_handler_test.go` 内、canned error を返すだけの `task.Repository` 実装)を **untagged のまま維持**。DB 不要。
  - auth: ISSUE-019 の `removableUserRepository`(`token_user_not_found_test.go`)は「user 削除後の invalid_grant」を要し、これは全フロー(client/authcode/refreshtoken の機能ストア)を伴う。実 DB では user 行を SQL で `DELETE` すれば同じ状況を作れるため、**このテストは integration に移し、スタブは削除**する。純粋なエラー注入のみで機能ストア不要な auth ケースがあれば untagged 最小スタブで残す(impl-auth が個別に判定)。
- いずれも汎用ストアの再実装は禁止(削除した `infra/memory` の実質再導入をしない)。

**論点 5: 削除対象の確定**(下記「変更ファイル」に詳細)
- `infra/memory` パッケージ全体(api 3 ファイル / auth 12 ファイル)、`Mode` / `ModeMemory` / `ModePostgres` / `SelectMode` / `ErrPersistenceNotConfigured`(auth)、`cmd/*/main.go` の memory 分岐・`seedMemory`、`Env.validate` の Mode 戻り値、memory を参照する test / doc コメント。
- 併せて、memory 選択のためだけに存在する **`APP_ENV` プラミング**(`env.go` の読み取り・`env_test.go` の関連ケース)を除去する。`APP_ENV` は `DB_*`/`DB_READER_*`/`ISSUER` の env 契約に含まれず(不変対象外)、memory フォールバック廃止で完全に dead になるため。fail-closed は `Config.Validate`(`DB_HOST`/`DB_NAME`/`DB_USER`/`DB_PASSWORD` 必須)が既に担うので、`SelectMode` 廃止後も本番必須は維持される。

**論点 6: seam 確認**
- Spec §4 の調査どおり `service` / `route` / `domain` は `pgx` / `database/sql` / `pgconn` / `sqlcgen` を import していない。impl-api / impl-auth が grep で再確認し、漏れがあれば `infra/postgres` 内へ封じ直す。
- store 固有 seam(`sql.ErrNoRows` 翻訳 / unique 違反判定 / DSN スキーム / `*sql.Tx`)が `infra/postgres` に閉じていることを再確認し、`.claude/rules/db.md` / 各 README に「mysql 差し替えは `infra/postgres` を新実装で置換し同じ contract を回す」旨を一文追記(実装はしない)。
- (推奨・任意)golangci-lint `depguard` で `domain`/`service`/`route` からの driver 系 import を禁止し、seam を将来にわたり機械的に守る。Spec R4 の必須は「確認 + 文書化」なので必須化はしない(impl-ci の enhancement 候補として記録)。

### 退けた代替案

| 案 | 不採用理由 |
|---|---|
| route テストを untagged のまま `route_test` 内の汎用 in-memory ストア fake で回す | happy-path 用の汎用ストアは「`infra/memory` の実質再導入」。Spec R3(route テストは実 DB へ)に反する |
| route テストを untagged にし `openTestDB` の `t.Skip`(DB_HOST 未設定時)だけで制御 | 既存 DB テストは build tag で「オフラインではコンパイルもしない」規約。skip だけだと `make check` が DB 依存ファイルを常にコンパイル/実行(skip)することになり規約と不整合 |
| unit ジョブを Postgres 化して全テストを実 DB で | SPEC-009 のオフライン `make check` を崩す |
| `APP_ENV` を残す(将来のロギング用) | memory 廃止後に唯一の消費者(`SelectMode`)が消え dead code 化する。残すと「使われない env」を誤読させる。必要になれば別途追加する |

---

## 変更ファイル(stack ごと)

### app/api

**削除**
- `infra/memory/task_repository.go`
- `infra/memory/task_repository_test.go`
- `infra/memory/task_repository_contract_test.go`
- パッケージごと `infra/memory/` を消す

**変更**
- `infra/postgres/db.go` — `Mode` / `ModeMemory` / `ModePostgres` / `SelectMode` を削除。永続化は Postgres 単一。`Config` / `Validate` / `DSN` / `Open` / `OpenPair` は不変(SPEC-010 維持)。
- `cmd/api/main.go` — `infra/memory` import 削除。`newTaskRepository` から `mode` 分岐と memory ブランチを除去し、常に `postgres.OpenPair` → `NewTaskReader`/`NewTaskWriter` を配線。`run()` の `mode` 受け渡しを撤去。
- `cmd/api/env.go` — `AppEnv` フィールドと `os.Getenv("APP_ENV")` を削除。`validate()` は `postgres.Mode` を返さず `error` のみ(writer/reader `Config.Validate` を実行)。`SelectMode` 依存を除去。
- `cmd/api/env_test.go` — `APP_ENV` / mode 関連ケースを削除し、`validate()` の新シグネチャ(fail-closed = `DB_*` 欠落でエラー)に合わせて更新。
- `infra/postgres/persistence_selection_test.go` — `TestSelectMode*` を削除。`Config` の `DSN`/`Validate`/`Equality`(SPEC-010 の `==` 保証)テストは残す。ファイル名/doc コメントの「selection」文言を実態に合わせる。
- `route/task_handler_test.go` — **分割**:
  - untagged 側: `failingRepository` / `dbErrorRepository` を用いるエラー注入ケース + ストア不要な検証系(malformed JSON、invalid priority、not-found の一部など repo 到達前に決まるもの)。`infra/memory` import を削除。
  - `//go:build integration` 側(例: `route/task_handler_integration_test.go`): 機能ストアを要する正常系(create→get、list、start/complete 遷移、wire-shape の成功系)。共有 test-DB ヘルパ経由で実 `postgres.NewTaskRepository` を配線。
- `Makefile` — `test-integration-native` の対象を `./infra/postgres/...` → `./infra/postgres/... ./route/...` に拡張。
- `README.md` — memory 記述の削除 + 「Postgres 一本化 / mysql 差し替えは infra/postgres 差し替え + 同 contract」の一文。

**追加**
- 共有 test-DB ヘルパ(impl-db 担当。下記「テスト戦略」参照)。`infra/postgres` の integration test が持つ `openTestDB`/`testDSN`/`truncate*` を、`route` の integration test からも使える形へ集約(`//go:build integration` の専用テストサポート。例: `app/api/infra/postgres/testsupport`)。repotest とは別物(repotest は stdlib + domain のみ依存の縛りを維持)。

### app/auth

**削除**
- `infra/memory/{client,user,authcode,refreshtoken}_repository.go` とそれぞれの `_test.go` / `_contract_test.go`(計 12 ファイル)。パッケージごと `infra/memory/` を消す。
- `route/token_user_not_found_test.go` の `removableUserRepository`(integration 化に伴い実 DB の `DELETE` で代替)。

**変更**
- `infra/postgres/db.go` — `Mode` / `ModeMemory` / `ModePostgres` / `SelectMode` / `ErrPersistenceNotConfigured` を削除。`Config`/`Open`/`OpenPair` は不変。
- `cmd/authz/main.go` — `infra/memory` import 削除。`setupPersistence` から `mode` 分岐・memory ブランチ・`seedMemory` を除去し、常に `OpenPair` → 各 `postgres.New*Repository`(reader/writer 振り分けは SPEC-010 表どおり)を配線。`seedPostgres` / `buildDemoClient` / `buildDemoUser` は維持(seedMemory 専用でないため)。
- `cmd/authz/env.go` — `AppEnv` / `APP_ENV` 読み取り削除。`validate()` は `error` のみを返す。
- `cmd/authz/env_test.go` — `APP_ENV` / mode 関連ケース削除、新シグネチャに更新。
- `infra/postgres/persistence_selection_test.go` — `TestSelectMode*`(および `ErrPersistenceNotConfigured` 参照)を削除。`Config` の `DSN`/`Validate`/`Equality` テストは残す。
- `route/helpers_test.go` — memory-backed `newTestHandler` を **実 DB backed へ**(`//go:build integration`)。共有 test-DB ヘルパで接続・TRUNCATE・`postgres.SeedClient`/`SeedUser` で demo client(2 種)+ user を seed し、実 `postgres.New*Repository` を配線。RSA 鍵生成等の非 DB 部分は維持。
- `route/token_user_not_found_test.go` / `route/token_concurrency_test.go` — `infra/memory` 依存を除去。全フローを要するため `//go:build integration` 化し、共有ヘルパ経由に統一(`token_user_not_found` は user 行 `DELETE` で「owner 消失」を再現)。
- `route/*_test.go`(authorize_flow / refresh_token / security / discovery / userinfo / authorize_open_redirect 等) — 大半は `newTestHandler` 経由なので、`newTestHandler` が integration になることで**同ファイルも integration 化が必要**。DB 不要のもの(例: discovery は issuer + keyProvider のみで repo 状態不要)は、DB 非依存のハンドラ構築ヘルパを別途用意し untagged で残す余地を impl-auth が判定する。
- `Makefile` — `test-integration-native` の対象を `./infra/postgres/... ./route/...` に拡張。
- `README.md` — memory 記述削除 + mysql 差し替え一文。

**追加**
- 共有 test-DB ヘルパ(impl-db。api と同型、`app/auth/infra/postgres/testsupport` 等)。demo データ seed ユーティリティを含める。

### 横断

- `.claude/rules/db.md` — `SelectMode` の選択規則表(memory 分岐)を「Postgres 必須・fail-closed」に更新。`infra/memory` を同格実装として挙げている記述を削除。「別 store 差し替えは `infra/postgres` 置換 + 同 contract」を追記。
- `.claude/rules/testing.md` — 「ふるまい契約テストを memory / postgres 双方で回す」節を「Postgres(integration)のみで回す」へ更新。route テストの 2 層(untagged エラー注入 / integration 実 DB)方針を追記。
- `.github/workflows/cicd.yml` — 原則不変(`make test-integration` 拡張で route も回る)。impl-ci が path-filter・ジョブ実行を確認。ジョブコメント内の「memory-backed unit job」等の文言を実態に更新。
- `CLAUDE.md` / `.claude/agents/impl-db.md` 等の `infra/memory` 言及 — 必要に応じ admin が別途更新(planner はスコープ外として指摘に留める)。

---

## 手順(担当 agent・順序・並列可否)

> フェーズ間は原則直列、フェーズ内の `[並列]` は同時実行可。

- **T2 / Phase 0(ベースライン確認)** — **tester**
  - 現状の `make check`(api/auth、オフライン)と integration(`make test-integration`、DB 起動下)が緑であることを特性化確認。以降の「挙動不変」の安全網を固定する。

- **T3 / Phase 1(共有基盤)** — **impl-db**(直列・後続の前提)
  1. seam 再確認: `service`/`route`/`domain` に driver 系 import が無いことを grep 確認、`sql.ErrNoRows` 翻訳 / unique 判定 / DSN / `*sql.Tx` が `infra/postgres` 内に閉じていることを再確認。漏れがあれば閉じ直す。
  2. 共有 test-DB ヘルパ(`//go:build integration`)を api / auth 双方に整備: `OpenTestDB(t)`(DB_HOST 未設定時 `t.Skip`)/ `Truncate...` / auth 用 demo データ seed。既存 `infra/postgres/*_integration_test.go` の `openTestDB`/`testDSN`/`truncate*` を集約し、route integration からも import 可能にする(repotest の stdlib+domain 縛りは崩さない別パッケージ)。
  3. contract test の memory バインディング前提(doc コメント等)を Postgres 単一へ整理。`db.md` の該当規約更新。

- **T4 / Phase 2(各スタック本体)** — `[並列]` **impl-api** と **impl-auth**(Phase 1 完了後)
  - impl-api:
    - `infra/memory/` 削除、`db.go` の `Mode`/`SelectMode` 削除、`cmd/api/main.go`・`env.go`・`env_test.go` の Postgres 一本化 + `APP_ENV` 除去。
    - `persistence_selection_test.go` の `SelectMode` テスト削除(Config テストは残す)。
    - `route/task_handler_test.go` を untagged(エラー注入 + 検証系)/ integration(正常系・共有ヘルパ + 実 repo)に分割。`Makefile` の `test-integration-native` 拡張。README 更新。
  - impl-auth:
    - `infra/memory/` 削除、`db.go` の `Mode`/`SelectMode`/`ErrPersistenceNotConfigured` 削除、`cmd/authz/main.go`(memory 分岐・`seedMemory` 除去)・`env.go`・`env_test.go`(`APP_ENV` 除去)。
    - `persistence_selection_test.go` の `SelectMode` テスト削除。
    - `route/helpers_test.go` の `newTestHandler` を integration 実 DB backed へ。`token_user_not_found_test.go`(DELETE で owner 消失を再現)・`token_concurrency_test.go` を integration 化。DB 非依存で残せる route テストがあれば untagged 用ヘルパで分離。`Makefile` 拡張。README 更新。
  - 両者とも: seam 再確認結果を README/該当箇所に反映(mysql 差し替え一文)。

- **T5 / Phase 3(CI 整合)** — **impl-ci**(Phase 2 後、Phase 4 と一部並行可)
  - `cicd.yml` の path-filter・`api-integration`/`auth-integration` が route integration まで回すことを確認。ジョブコメントの memory 文言を更新。`.claude/rules/testing.md` の CI 記述整合。
  - (任意)`depguard` で driver 系 import を禁止する lint ルールの追加提案。

- **T6 / Phase 4(検証・レビュー)**
  1. **tester**: オフライン(`make test`/`make check` 相当)+ integration(`make test-integration`、DB 起動下)を api/auth 双方で実行。memory 削除後もカバレッジ観点(正常/異常/境界)が保たれているか確認、不足を追加。
  2. **checker**: 各スタック `make check`(fmt-check + lint + vet + build + test)。**通るまでレビューに進まない。**
  3. `[並列]` **review-spec** / **review-security** / **review-performance**: 特に review-spec が「HTTP/DTO/OpenAPI・ドメインポート・env(`DB_*`/`DB_READER_*`/`ISSUER`)・スキーマ/sqlc 不変」を確認。review-security は fail-closed(DB 必須)維持と error injection スタブが情報漏洩経路を作っていないこと。
  4. 指摘対応: Blocker/Major は該当 impl へ差し戻し、Phase 4 を再実行。今回対応しない指摘は issue-creator が起票。

- **T7 / Phase 5(ドキュメント)** — 上記各 impl の担当範囲で `.claude/rules/{db,testing}.md` / 各 README を更新済みにする(Phase 2〜3 に内包)。admin は Spec §5 と経緯を spec skill 経由で更新(下記「Spec 反映」)。

- **T8(完了判定)** — admin
  - Spec §1「価値の検証方法」3 条件を確認して done 化。

---

## テスト戦略

- **方式**: 挙動不変リファクタリングなので原則 **既存テストを安全網として維持**。ただし本 Spec は「テスト配線の Postgres 化」自体が要件のため、memory 依存テストは削除ではなく **実 DB 経路へ移設**する(落として通したことにしない)。
- **レベル別**:
  - domain / service: 変更なし(既に test-local fake / 純関数。`infra/memory` 非依存)。
  - repository ふるまい契約(`Run<集約>RepositoryContract`): Postgres(integration)のみで実行。memory バインディング削除。
  - route(HTTP): 2 層。
    - untagged(オフライン): エラー注入(500 / invalid_grant のうちストア不要なもの)・入力検証・wire-shape のエラー系。test-local 最小スタブ。
    - integration(実 DB): 正常系・全フロー・wire-shape 成功系。共有ヘルパ + 実 repo。
  - env / main: `validate()` 新シグネチャ(fail-closed = `DB_*` 欠落でエラー)を untagged で検証。`APP_ENV` ケースは削除。
  - pool(`OpenPair`、SPEC-010): 既存 integration テスト維持。
- **要件 → カバレッジ対応**:
  - R1(memory 完全削除): ビルドが通ること(`infra/memory` import 消滅)+ 全 integration/contract が緑。
  - R2(Postgres 一本化・fail-closed): `env_test`(`DB_*` 欠落でエラー)+ 起動が常に Postgres 配線であること(main のコンパイル/配線)。
  - R3(テスト Postgres 化): route/contract テストが実 DB で緑(integration ジョブ)。
  - R4(DI 差し替え耐性): seam grep 確認 + `service`/`route`/`domain` が driver 非依存であることの文書化(+任意 depguard)。
  - R5(不要コード削除): `Mode`/`SelectMode`/`seedMemory`/`APP_ENV`/compile assertion の消滅。
- **非機能(挙動不変)の安全網**: wire-contract テスト(field set / casing / status code)と OpenAPI/DTO 不変を review-spec が確認。sqlc/スキーマ/マイグレーションは触らない。

---

## リスク / 未確定事項

1. **オフライン route カバレッジの縮小(要合意)**: 正常系 route テストが integration(実 DB)へ移ることで、`make check`(オフライン)での HTTP レイヤ正常系カバレッジが減る。これは Spec §4 の明示的トレードオフ(DB 依存テストは integration に集約)だが、特に auth は `newTestHandler` 経由の全フローが一括で integration 化するため offline 側が薄くなる。→ 許容する前提で進めるが、review 時に「offline に残すべき最小の HTTP 検証系(入力バリデーション等)」の線引きを impl-auth/tester が確定する。
2. **`APP_ENV` 除去の是非**: memory 廃止で dead になるため除去を推奨。ただし `compose.yml` / iac が `APP_ENV` を設定していないか impl-api/impl-auth が最終確認する(設定していても読み捨てで無害だが、意図の明確化のため確認)。もし将来用途で残す判断ならユーザー確認。→ 現時点では除去方針。
3. **`make test-integration` 拡張の副作用**: 対象に `./route/...` を足すと、route integration テストの demo データ seed / TRUNCATE の順序依存に注意(testing.md「実行順序依存を書かない」)。共有ヘルパで各テスト冒頭に TRUNCATE + seed し独立性を担保する。並列実行(`t.Parallel`)は同一 DB を共有するため原則使わない/テーブル分離できる範囲に留める。
4. **共有 test-DB ヘルパの置き場所**: repotest は「stdlib + domain のみ依存」の縛りがあるため、DB 接続ヘルパはそこへ入れられない。`infra/postgres` 配下の integration-tagged サポートパッケージに置く案を採るが、`route_test` から import 可能なパッケージ構成(循環や可視性)を impl-db が実装時に確定する。
5. **auth の read-after-write 配置(SPEC-010)維持**: integration route テストで authcode/refreshtoken を writer 固定・client/user を reader という配線を崩さないこと。テストは単一プール(`DB_READER_*` 未設定 = writer/reader 同一)で回すため差は出にくいが、配線コード自体は本番同型を維持する。
6. **depguard 導入の扱い**: seam の機械的保証として有用だが Spec R4 の必須要件ではない。導入するなら既存 golangci-lint 設定への追加が必要で、誤検知調整コストがある。→ enhancement として impl-ci が可否判断(必須化しない)。

---

## Spec 反映(admin へ)

- `docs/specs/20260710-011-...md` §5 の T1 にチェックを入れ、`docs/plans/SPEC-011-plan.md` への参照を記す。
- §6 経緯に「planner が実装計画作成。build-tag split で route テストを untagged(エラー注入)/ integration(実 DB)に再編、CI は既存 integration ジョブへ集約(新規ジョブなし)、`SelectMode`/`Mode`/`APP_ENV` を除去して `Config.Validate` の fail-closed に一本化する方針を確定」を追記。
- 更新は spec skill の手順(経緯追記・frontmatter `updated` 更新・過去エントリ不編集)に従う。planner は本 plan 作成に留め、Spec 本体編集は admin が spec skill 経由で実施。
