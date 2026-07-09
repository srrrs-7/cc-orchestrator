---
id: SPEC-005
title: app/api・app/auth の Postgres 永続化基盤(goose + sqlc)
status: done  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-09
updated: 2026-07-09
issues: [ISSUE-005, ISSUE-015, ISSUE-016, ISSUE-017]       # 関連Issue ID (例: [ISSUE-003])
supersedes: null # 置き換える旧Spec ID
---

# SPEC-005: app/api・app/auth の Postgres 永続化基盤(goose + sqlc)

## 1. ユーザー価値(なぜ作るか)

> **cc-orchestrator の開発者・運用者(および multi-agent ワークフロー)** が **app/api・app/auth のデータを Postgres に永続化できるようになり**、**プロセス再起動やスケールアウトでデータ・認可状態が失われるリスク** を無くす。

- **対象ユーザー**: cc-orchestrator の開発者(impl-db / impl-api / impl-iac / impl-ci を含む各 agent とレビュアー)、および app を運用する人
- **解決する課題**: 現状、app/api・app/auth の永続化は `infra/memory`(インメモリ)のみ。プロセスを再起動するとタスク・登録クライアント・ユーザー・認可コードがすべて消える。ECS で複数タスク(インスタンス)に水平スケールしても状態を共有できず、**認可コードの単回使用保証がインスタンス間で成立しない**(同じコードが別インスタンスで再利用され得る)など、実運用に耐えない。
- **得られる価値**:
  - 再起動・スケールアウトを跨いでデータと認可状態が保持される(共有ストア化)
  - スキーマは goose の SQL、クエリは sqlc の型安全 Go として **単一ソースから生成**され(SPEC-003 の OpenAPI 契約と同じ思想)、手書きの型ズレ・SQL 文字列連結を排除できる
  - DDD の依存性逆転(`domain` の `Repository` interface)を保ったまま `infra/memory` ↔ `infra/postgres` を差し替えられ、既存のテスト構造・レイヤ依存を崩さない
- **価値の検証方法**: 以下がすべて満たされたら成功とみなす。
  1. api・auth を Postgres 接続で起動し、データを作成 → プロセス再起動後も同じデータ・認可状態が残ることを確認できる
  2. `infra/postgres` の各リポジトリが対応する `domain/<集約>/Repository` interface を満たし、`infra/memory` と**同じ振る舞い**(`FindByX` の `ErrNotFound`、`Save` の重複扱い、認可コードの単回使用/TTL)を示すテストがグリーン
  3. スキーマ or クエリを変えて sqlc を再生成し忘れた状態を作ると、CI の sqlc drift 検査が **fail** する
  4. api・auth のランタイムバイナリに増える**新規 runtime 依存は Postgres ドライバ(pgx)のみ**であり、goose / sqlc は `go.mod` の require に載らない

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- **永続化実装者(impl-db)** として、スキーマ変更は goose マイグレーション(up/down)を 1 枚足し、必要なら `db/queries` の SQL を追記して sqlc を再生成するだけでよい。なぜなら型安全な Go アクセスコードが生成され、`domain` の `Repository` を満たす実装に流し込めるから。
- **API/認可 実装者(impl-api / auth)** として、`Repository` interface(ポート)だけ見ていればよく、SQL・ドライバ・生成コードを意識しなくてよい。なぜなら永続化の詳細は `infra/postgres` に閉じているから。
- **運用者** として、デプロイ時にマイグレーションを流せば、アプリ本体は接続情報を環境から受け取るだけで動く。なぜなら接続情報は Secrets Manager / 環境変数から注入され、コード・tfvars に平文が無いから。
- **開発者(ローカル)** として、`make up` で Postgres 込みの全スタックが立ち上がる。なぜなら compose に postgres サービスが含まれるから。

### 利用フロー

**スキーマ / クエリを変更するとき(開発フロー):**

1. impl-db が `db/migrations/` に goose マイグレーション(`NNNNNN_*.sql`、up/down)を追加する
2. 必要なら `db/queries/` の SQL を追記し、`make sqlc`(= `go run` の sqlc CLI)で生成コードを更新する
3. impl-db が生成コードを使って `infra/postgres` のリポジトリ実装を更新する
4. マイグレーション・クエリ・生成コード・実装を一緒にコミットする
5. CI が sqlc drift 検査(再生成して `git diff --exit-code`)でコミット漏れが無いことを保証する

**マイグレーションを適用するとき(実行フロー):**

- **ローカル**: `make up`(compose の postgres 起動)→ `make migrate-*`(goose CLI で up)
- **本番(AWS)**: RDS(既存 `app/iac/modules/db`)に対し、**一回限りの ECS タスク or init コンテナ**として goose CLI を実行してから api/auth 本体を起動する(iac 側で用意)

## 3. 要件(何を満たすべきか)

### 機能要件

- [x] R1: **app/api(task)** の `domain/task/Repository`(`Save` / `FindByID` / `FindByTitle` / `FindAll`)を満たす `infra/postgres` 実装を追加する。振る舞い(特に `FindByID`/`FindByTitle` が該当なしで `ErrNotFound`、`Save` の重複・更新の扱い)は既存 `infra/memory` と一致させる
- [x] R2: **app/auth** の 3 集約 `client`(`FindByID`)/ `user`(`FindByID` / `FindByUsername`)/ `authcode`(`Save` / `FindByCode`)の `Repository` を満たす `infra/postgres` 実装を追加する。**認可コードは単回使用と TTL(短命)のセマンティクスを SQL 上で保持**する(consume 済み・期限切れは `FindByCode` で有効なコードとして返さない)。`token` は JWT のステートレス設計で `Repository` を持たないため対象外
- [x] R3: goose マイグレーション(up/down 対)で api・auth のスキーマを定義する。**api と auth は同一 RDS の単一データベース内で別スキーマ(`search_path` で分離)に分ける**(バウンデッドコンテキストの分離。ユーザー確定)
- [x] R4: sqlc で `db/queries/**` の SQL から型安全 Go を生成し、**生成コードをコミット**する。生成コードは各スタックの `make build` / `make vet` / `make test` を通す
- [x] R5: マイグレーション実行手段を提供する。ローカルは Makefile ターゲット(compose の postgres 相手)、本番は一回限りの ECS タスク / init コンテナ(iac)。**goose は `go run <pkg>@<pinned>` の CLI として実行し、`go.mod` の require に載せない**
- [x] R6: リポジトリ実装を環境で切り替えられる。**本番は Postgres 必須**(接続情報が無ければ起動を失敗させる)で、`infra/memory` フォールバックは local / test に限定する(ユーザー確定)。**接続情報は Secrets Manager / 環境変数から注入し、コード・tfvars に平文で書かない**
- [x] R7: ローカル開発用に `compose.yml` とルート `Makefile` に `postgres` サービスを追加し、api・auth コンテナがそこへ接続する(host は既存方針どおり `127.0.0.1` バインド)
- [x] R8: CI に **sqlc drift 検査**(`sqlc generate` → `git diff --exit-code`)を追加する(SPEC-003 の contract-drift と同型、impl-ci)。マイグレーション適用の健全性(up → down → up が通る等)も検査対象に含めるか planner が判断する
- [x] R9: `.claude/rules/db.md` の「コマンド」表(`make sqlc` / マイグレーション系ターゲット名)と、CLAUDE.md のコマンド早見表を確定して反映する

### 非機能要件

- **std-lib 方針の緩和(明文化)**: app/api・app/auth はこれまで標準ライブラリのみ。本 Spec で **`infra/postgres` 層に限り** 外部 runtime 依存を許容する。増やしてよいのは **Postgres ドライバ(`github.com/jackc/pgx/v5`、`database/sql` 経由の `stdlib` ドライバ)のみ**。sqlc 生成コードは `database/sql`(標準)にのみ依存させる。goose / sqlc はビルド時 CLI(`go run <pkg>@<pinned>`)に隔離し require に載せない。この緩和は `domain` / `service` / `route` には及ばない(それらは標準ライブラリ維持)。
- **サプライチェーン**: pgx のバージョンを固定する。sqlc / goose の `go run` 版もピンする。
- **セキュリティ**: SQL は sqlc のパラメータ化クエリのみ(文字列連結禁止 = SQL インジェクション防止)。接続情報は Secrets Manager / 環境変数から注入。認可コード・ユーザー資格情報など機微データの扱いは `app/auth` のセキュリティ規約(`.claude/rules/auth.md`)を維持する。
- **マイグレーション安全性**: マイグレーションは前進的かつ可逆(up/down)に書く。破壊的変更(列削除・型変更・NOT NULL 化)はレビューで必ず明示・報告する。
- **既存テストの維持**: `infra/memory` 経路の既存テスト(api の `go test`、auth の各 `*_repository_test.go`)はグリーンのまま残す。`infra/memory` は削除せず、テスト・ローカル簡易起動のフォールバックとして温存する。
- **std-lib 検証の維持**: 既存の「ランタイム依存が最小」という性質を CI/レビューで確認できるようにする(pgx 以外の runtime 依存が増えていないこと)。

### スコープ外(やらないこと)

- ORM(GORM / ent 等)の導入(sqlc は ORM ではない。SQL を正とする方針)
- Postgres 以外のストア(Redis 等)の導入。認可コード/セッションの専用キャッシュ層は将来別 Spec
- `infra/memory` の削除(切替可能な実装として残す)
- 既存データの移行(現状インメモリで永続データが無いため移行対象なし)
- コネクションプーラ製品(pgbouncer 等)・リードレプリカ・マルチAZ 化などの可用性強化(iac 側は既存 `modules/db` の範囲。拡張は別途)
- `terraform apply` の実行(plan までを報告し apply はユーザー判断。既存方針どおり)

## 4. 設計(どう実現するか)

### 方針

**DDD の依存性逆転を維持し、`infra/postgres` を `infra/memory` と同格の `Repository` 実装として追加する。** 契約の正は `domain/<集約>/repository.go` の interface(ポート)。スキーマは goose(SQL)、クエリ→型安全アクセスは sqlc で単一ソースから生成する(SPEC-003 と同じ「生成物をコミットし CI で drift 検出」方式を DB クエリへ展開)。

```
domain/<集約>/repository.go  (Repository interface = ポート / 契約の正)
        ▲ 実装(依存性逆転)
infra/postgres/<集約>_repository.go ──uses──▶ sqlc 生成コード ──▶ database/sql + pgx stdlib ドライバ
        ▲                                        ▲
db/queries/*.sql ──(sqlc generate)──────────────┘
db/migrations/*.sql ──(goose up/down)──▶ Postgres
                                          ├─ local: compose の postgres サービス
                                          └─ prod : RDS(app/iac/modules/db、Secrets Manager 管理)
cmd/*/main.go : 環境(接続情報の有無)で memory / postgres を選択して配線
CI(impl-ci): sqlc generate → git diff --exit-code(drift 検査)
```

### アーキテクチャ / データ / インターフェース

- **確定した技術選定**:
  - ドライバ = `database/sql` + **pgx v5 stdlib**(`github.com/jackc/pgx/v5/stdlib`)。sqlc の出力を `database/sql` にすることで生成コードは標準ライブラリのみに依存し、ドライバ import だけが外部依存になる
  - マイグレーション = **goose を `go run github.com/pressly/goose/v3/cmd/goose@<pinned>` の CLI として実行**(require に載せない)
  - api ⇔ auth = **同一 RDS の単一データベース内で別スキーマ(`search_path` 分離)**(ユーザー確定)。ローカルも同一 database + 2 スキーマで用意する
  - sqlc 設定 = **スタックごとに `sqlc.yaml`**(api / auth は別モジュール・別ドメインのため)
- **ディレクトリ(planner が最終確定)**: 各 Go スタック配下に `db/migrations/`(goose)・`db/queries/`(sqlc 入力)・sqlc 生成コード(コミット対象。配置は `infra/postgres` 近傍 or `db/gen`)・`sqlc.yaml` を置く
- **app/api**: `infra/postgres/task_repository.go` が `task.Repository` を実装。`Title` の一意制約・`ID` 主キーなどドメイン不変条件を DB 制約にも反映
- **app/auth**: `infra/postgres/{client,user,authcode}_repository.go`。authcode は `expires_at` と consume 状態(used フラグ or consume 時 DELETE)で単回使用・TTL を表現し、`FindByCode` は有効なコードのみ返す。client / user はデモ seed を維持できるよう seed 手段(マイグレーション or 起動時 seed)を planner が決める
- **配線(cmd/*/main.go)**: 接続情報(例 `DATABASE_URL`)があれば `postgres.NewXxxRepository(db)`、無ければ従来の `memory.NewXxxRepository()`。DB プールの寿命は context / graceful shutdown に合わせて解放。**この永続化配線は impl-db が担当**(HTTP/サーバ配線は impl-api / auth)
- **iac(impl-iac)**: 既存 `modules/db`(Postgres RDS・Secrets Manager 管理)を前提に、(a) api/auth 用データベースの作成、(b) アプリへの接続情報注入(Secrets 参照 → タスク定義の環境変数)、(c) マイグレーション実行用の一回限り ECS タスク / init コンテナ定義、を追加。plan まで(apply しない)
- **CI(impl-ci)**: sqlc drift 検査ジョブ(Go セットアップ → `sqlc generate` → `git diff --exit-code`)。Go 単一 stack で完結し、既存 contract-drift とは別ジョブ

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| ORM(GORM / ent) | 永続化を強く隠蔽し、SQL の明示性と DDD レイヤの見通しを損なう。sqlc は SQL を正にでき、生成物レビューで変更が可視化される方針と合う |
| golang-migrate(migrate) | goose と同格だが、`go run` 隔離のしやすさ・SQL/Go 両対応・埋め込み実装の柔軟性で goose を採用済み(agent 設計時に決定) |
| pgx-native + pgxpool | 高性能だが生成コードが pgx に直接依存する。std-lib leaning を優先し `database/sql` + pgx stdlib を採用(pgxpool は将来の最適化余地として保持) |
| goose を library として起動時に埋め込み実行 | goose が runtime 依存(go.mod)化する。新規 runtime 依存を pgx のみに保つため不採用。運用簡易さは init コンテナ / 一回限りタスクで担保 |
| authcode / token も含め全状態を即 Postgres 化(無条件) | token は JWT でステートレスのため対象外。authcode は単回使用・TTL を SQL で正しく表現する必要があり、設計を R2 とゲートで明示して段階的に固める |
| api/auth で DB を共有(単一 database・スキーマ共有) | バウンデッドコンテキストが混ざり、権限分離・スキーマ進化が絡む。別データベース/スキーマで分離する |

## 5. 実装計画

詳細計画は planner が `docs/plans/SPEC-005-plan.md` に作成する(方針・変更ファイル・手順・テスト戦略・リスクは同ファイルが正)。概要タスク:

> **着手前ゲート:** ユーザー確定分は反映済み。残りは planner が確定する。
> 1. ~~auth の永続化スコープ~~ → **確定: client / user / authcode の 3 集約を同時に Postgres 化**(認可コードの単回使用・TTL の穴を残さない)
> 2. **リポジトリ切替方式(一部確定)**: **本番は Postgres 必須**(接続情報が無ければ起動失敗、`infra/memory` フォールバックは local / test 限定)。環境変数の具体名・接続文字列の形式は planner が確定する
> 3. ~~api/auth の分離単位~~ → **確定: 同一 database の別スキーマ(`search_path` 分離)**
> 4. **seed とテスト(planner)**: client / user のデモ seed の置き場所(マイグレーション or 起動時 seed)と、`infra/postgres` 統合テストの実行方式(CI の postgres service container + build tag で分離 等)を確定する

- [x] T1: (planner) ディレクトリ/命名・`sqlc.yaml` 構成・goose/sqlc の版固定・マイグレーション実行方式・切替方式・テスト方式・iac 変更範囲の確定と、上記ゲートの解決案提示 → **完了: `docs/plans/SPEC-005-plan.md`(方針・変更ファイル・手順・テスト戦略・リスクは同ファイルが正)**。残ゲートの結論: 版 = pgx `v5.7.2`(唯一の runtime require)/ goose `v3.24.1` / sqlc `v1.28.0`(`go run` CLI)、配置 = `db/migrations`・`db/queries`・`sqlc.yaml`・生成コードは `infra/postgres/sqlcgen`、スキーマ分離 = 非修飾 DDL + `search_path`(スキーマ作成は goose 外のブートストラップ)、切替 = 離散 `DB_*` env + `DB_HOST`/`APP_ENV` による fail-closed(本番は Postgres 必須)、seed = client/user とも Postgres 経路の起動時 idempotent seed、authcode = `Consume` を `DELETE ... RETURNING` の delete-based で単回使用/TTL、統合テスト = build tag `integration` + CI の postgres service container、compose/ルート Makefile/`Dockerfile.migrate` = impl-db 担当
- [x] T2: (tester) memory/postgres 共通の振る舞い契約スイート(`infra/repotest`)を先行作成(TDD)。authcode の単回使用/TTL/並行 Consume、build tag `integration` 分離を含む
- [x] T3: (impl-db) api を実装(`db/migrations`・`db/queries`・sqlc 生成・`infra/postgres/task_repository.go`・`db.go`・配線)。実 DB で統合テスト green
- [x] T4: (impl-db) auth 3 集約を実装(client/user/authcode、単回使用は `DELETE ... RETURNING`、client は `jsonb`、seed)。A1 の `CodeChallenge.Challenge()` を利用。実 DB で green
- [x] T5: (impl-iac) DB 接続 env 注入(Secrets JSON key)+ migrate init コンテナ(`dependsOn: SUCCESS`)を追加(`terraform validate` green、**apply 未実行**)。(impl-db) compose の postgres サービス + `docker/postgres/initdb` + ルート `Makefile`(`db-up`/`migrate`)+ `Dockerfile.migrate` を追加
- [x] T6: (impl-ci) `.github/workflows/sqlc-drift.yml` + `api-integration`/`auth-integration` job(postgres service + goose up→down→up + `make test-integration`)を追加
- [x] T7: (tester) タグ無し + 統合テスト実行(全 green)→ (checker) 全スタック `make check` green → (review-security / performance / spec) 実施
- [x] T8: 指摘対応完了(下記経緯)。`.claude/rules/db.md` コマンド表・`CLAUDE.md` 早見表を反映。本 Spec を `done` に更新

> 注: T3(api)と T4(auth)は scope 独立で並列可。T5(iac)・T6(ci)は T3/T4 のディレクトリ・コマンド確定に依存する部分がある。`infra/memory` は削除せず残す。

## 6. 経緯(時系列・追記のみ)

### 2026-07-09

- 初版作成。app/api・app/auth の永続化がインメモリのみで、再起動・スケールアウトでデータと認可状態が失われる(特に認可コードの単回使用がマルチインスタンスで保証できない)課題に対し、**Postgres 永続化基盤**を導入する方針を起票。
- admin(orchestrator)とユーザーの合意で技術選定を確定:
  - マイグレーション = **goose**、クエリ→型安全 Go 生成 = **sqlc**(SPEC-003 と同じ「単一ソースから生成 + 生成物コミット + CI drift 検査」思想を DB へ展開)
  - ドライバ = **`database/sql` + pgx v5 stdlib**(生成コードは標準 `database/sql` のみ依存、新規 runtime 依存は pgx のみ)
  - マイグレーション実行 = **goose を `go run <pkg>@<pinned>` の CLI**(local は make、prod は一回限り ECS タスク/init コンテナ。goose は go.mod に載せない)
  - api ⇔ auth = **同一 RDS の別データベース/別スキーマ**で分離
  - ローカル = `compose.yml` + ルート `Makefile` に postgres サービス追加
- あわせて、この永続化縦割りを横断で担う専用 agent **impl-db**(sonnet)を新設し、`.claude/rules/db.md`(goose/sqlc の契約・`Repository` ポート seam・セキュリティ)を追加。orchestration.md の割り振り表・モデル方針、CLAUDE.md/impl-ci のルール参照を更新済み。担当分担は **ポート(`Repository` interface)= impl-api / auth、実装(`infra/postgres`)= impl-db、接続/マイグレーション基盤 = impl-iac、drift 検査 = impl-ci**。
- 現状把握: app/api は task(Save/FindByID/FindByTitle/FindAll)、app/auth は client(FindByID)/ user(FindByID/FindByUsername)/ authcode(Save/FindByCode)の 3 集約が `infra/memory` で実装済み。token は JWT ステートレスで Repository を持たず対象外。既存 `app/iac/modules/db` は Postgres RDS + Secrets Manager 管理を実装済み。
- 未確定(着手前ゲート・§5 参照): auth の永続化スコープ(3 集約同時か段階か。推奨: 同時)、リポジトリ切替方式、別 database/別スキーマの別、seed・統合テストの実行方式。status は draft。**ユーザー承認(approved)後に planner へ実装計画作成を委譲する。**
- ユーザー承認により status を `approved` に更新。着手前ゲートをユーザー確定: **(1) auth スコープ = client / user / authcode の 3 集約を同時に永続化 / (2) api・auth = 同一 database の別スキーマ(`search_path` 分離)/ (3) 本番は Postgres 必須(`infra/memory` フォールバックは local / test 限定)**。残る seed・統合テスト方式・環境変数名は planner に委譲。R3 / R6 と §4 設計・§5 ゲートを確定内容へ更新した。planner に実装計画(`docs/plans/SPEC-005-plan.md`)の作成を委譲する。
- planner が実装計画 `docs/plans/SPEC-005-plan.md` を作成し、残ゲートを確定(§5 T1 参照)。要点: 版ピン(pgx `v5.7.2` のみ runtime require / goose `v3.24.1` / sqlc `v1.28.0` は `go run` CLI)、スキーマ分離は非修飾 DDL + 接続 `search_path`(スキーマ作成は goose 外のブートストラップ = local は postgres init script / prod は iac)、切替は離散 `DB_*` env による fail-closed(`DB_HOST` があれば Postgres、無ければ `APP_ENV∈{local,test}` のみ memory、既定は起動失敗)、client の多値属性は `jsonb`、authcode の単回使用は `DELETE ... RETURNING`。実装調査で **`domain/authcode` の `CodeChallenge` に raw challenge の accessor が無く永続化不可**が判明 → 唯一のポート補助として impl-auth が accessor を 1 つ追加する(interface / `Reconstruct` は不変)。repo ルートの `compose.yml` / `Makefile` / `Dockerfile.migrate` は横断の永続化ツーリングとして impl-db が担当と確定。tester は共通振る舞い契約スイートを先行(TDD)、`infra/postgres` 実 DB 統合テストはスキーマ確定に依存するため近接後追いを推奨。status を `in-progress` に更新。次フェーズ(tester / impl-db / impl-auth / impl-iac / impl-ci)へ委譲する。
- レビュー(review-security / review-performance、E3)で「今回は対応せず追跡する」と判断された指摘を issue-creator が起票し、本 Spec と相互リンクした(frontmatter `issues` に ISSUE-005 / ISSUE-015 / ISSUE-016 を追加):
  - **ISSUE-015**(perf / medium): Postgres 化した認可コードが lazy eviction のみで、`/token` に到達しない離脱フローの未消費・期限切れコードが恒久残存し無制限増加する(テーブル全体の定期 bulk purge の実行主体が未存在)。周期 purge は新しいデプロイ / 運用要素を要し、本 Spec のスコープ(plan まで / 永続化実装、runtime 依存は pgx のみ)を超えるためスコープ外。
  - **ISSUE-016**(security / medium): DB 接続の最小権限・TLS ハードニング 2 項目 — api/auth が同一 RDS マスターユーザで接続し `search_path` 分離は権限境界でない(R-c)/ アプリ側 `DB_SSLMODE` 既定が `"disable"` で注入漏れ時に平文接続へ fail-open する(m-2)。いずれも本 Spec のスコープ(R3 スキーマ分離 / R6 接続情報注入)は満たしており意図的な現状踏襲(plan §6.1 で評価済み)。
  - **ISSUE-005**(security / low、既存): auth user パスワードの平文保存を本 Spec が `infra/memory` から Postgres(`users.password text`)へ引き継いだ facet を既存 Issue に追記し、本 Spec を相互リンク(plan §6.1 R-b「ハッシュ化は将来 Issue」の追跡先)。
- レビュー(review-spec、E3)で挙がった **R-h**(migrate イメージの ECR push 経路が未配線)を issue-creator が起票し、本 Spec と相互リンクした(frontmatter `issues` に ISSUE-017 を追加):
  - **ISSUE-017**(infra / medium): prod マイグレーション用の `:migrate` イメージ(`app/{api,auth}/Dockerfile.migrate`)を ECR に build & push する経路が存在せず(ルート `Makefile` の `push-images` はアプリイメージのみ)、init コンテナ(`app/iac/modules/service/main.tf:35` の `:migrate` 参照 + `dependsOn: SUCCESS`)がイメージを pull できないため、このままでは `terraform apply`(api/auth の新規 / 更新デプロイ)が成立しない。plan §6.2 R-h で「実配線は後続に委ねる」と意図的にスコープ外化した項目の追跡先。実運用デプロイの前提条件だが、ローカル / CI 経路・既存 running タスクには影響せず、手動 build & push の回避策があるため medium。
- **実装・レビュー完了、status を `done` に更新。** パイプライン(TDD → 実装 → checker → review → 指摘対応)を完走した:
  - 実装: impl-auth(`CodeChallenge.Challenge()` accessor)/ impl-db(api・auth の `infra/postgres` + goose + sqlc、compose・ルート Makefile・`Dockerfile.migrate`)/ impl-iac(DB env 注入 + migrate init コンテナ、plan まで)/ impl-ci(`sqlc-drift.yml` + integration job)。tester が memory/postgres 共通契約スイートを先行(TDD)。
  - **価値の検証方法 1〜4 をすべて確認**: #1 実 Postgres でタスク作成 → プロセス再起動後もデータ保持を確認 / #2 `infra/postgres` が `Repository` を満たし `infra/memory` と同値の契約テストが green(api 統合 15・auth 統合 26 サブテスト) / #3 生成コードへ drift を注入 → `sqlc-drift` 検査が fail することを再現 / #4 `go.mod` の新規 runtime require は `github.com/jackc/pgx/v5` のみ。
  - checker が全スタック `make check` green。review-security / review-performance / review-spec を実施。
  - **指摘対応(Blocker/Major)**: api の DB プール上限未設定(perf Blocker)/ auth の初回 ping タイムアウト欠如(perf Major)/ prod migrate エントリポイントのスキーマ未作成(security・spec Major = R5 の穴)を impl-db が修正(api/auth の `db.go` を対称化、entrypoint が `CREATE SCHEMA` + libpq `PG*` 接続)。R9(`.claude/rules/db.md` コマンド表 + `CLAUDE.md` 早見表)を反映。
  - **今回対応せず Issue 化**: ISSUE-005(平文パスワード)/ ISSUE-015(authcode 無制限増加)/ ISSUE-016(DB 最小権限・TLS)/ ISSUE-017(migrate イメージ ECR push 経路)。
  - **計画差分**: sqlc は macOS SDK での C パーサビルド不能のため、計画ベースライン `v1.28.0` → **`v1.31.1`** に両スタック統一(生成コードの形は不変。理由は各 Makefile に記載)。
  - **残(ユーザー判断)**: `terraform apply` は未実行(方針どおりユーザーに委ねる)。prod デプロイは **ISSUE-017**(`:migrate` イメージの ECR push)が前提。作業ツリーは未コミット(commit はユーザー指示時)。
