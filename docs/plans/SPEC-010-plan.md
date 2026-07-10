# SPEC-010 実装計画: DB infra 層の CQRS read/write 分離(Reader/Writer + writer/reader 2 プール)

- 起点: `docs/specs/20260710-010-db-cqrs-read-write-separation.md`(status: **approved**)
- 対象 stack: `app/api`(domain/task・service・infra/memory・cmd/api)、`app/auth`(domain/{authcode,refreshtoken,user,client}・service・infra/memory・cmd/authz)、両 stack の `infra/postgres`(impl-db)、`.claude/rules/db.md`(impl-db)。**app/migrator・app/web・app/iac は対象外**(Spec スコープ外)
- 成果物: 各集約ポートの Reader/Writer 分割(合成 `Repository` を互換維持)、`infra/postgres` に writer/reader 2 プールを開く `OpenPair`(フォールバック時は単一プール共有=二重に開かない)、`cmd/<bin>/env.go` の `DB_READER_*`(未設定は writer 値へフォールバック)、auth の単一利用トークン read を writer プールに固定

---

## 方針

### 採用アプローチ(全体像)

CQRS の command/query 分離を **(1) domain のポート分割** と **(2) infra の 2 プール + プール振り分け** の 2 層で実現する。物理分離(read replica)は `DB_READER_HOST` 注入で有効化できる seam を用意するだけで、リソースは作らない(Spec スコープ)。

1. **domain: `Reader` / `Writer` を additive に追加し、合成 `Repository = interface { Reader; Writer }` を残す**(互換)。既存の `var _ Repository = ...` やポートを受ける全コード・共有 contract test がそのままコンパイル/緑を保つ(R6)。書き込みを持つ集約(`task` / `authcode` / `refreshtoken`)にのみ Reader/Writer を切る。read-only 集約(`user` / `client`)は command メソッドを持たないため **Reader-only ポート**として据え置く(空の Writer を作らない。§リスク・review-spec で確認)。

2. **service: 読み取り負荷を逃がしたい経路が reader プールを通るよう、必要な側だけを型で受け取る**:
   - api `DuplicateChecker`(`FindByTitle` のみ)→ `task.Reader`
   - api `TaskService` → `task.Reader` + `task.Writer`(2 依存)。**理由**: `List`(閲覧=読み取りスケールの主対象)が reader プールへ、`Save`(command)が writer プールへ流れるためには、TaskService が reader プール実装と writer プール実装を別々に保持する必要がある。単一の合成 `Repository`(=単一プール)では `List` も writer プールに固定され R2 の価値が出ない
   - auth `UserInfoService` / `AuthorizationService` は **署名不変**。理由は下記「auth はプール境界が集約境界に一致」を参照

3. **infra/postgres: `OpenPair(ctx, writerCfg, readerCfg)` で 2 プールを開く**。`readerCfg == writerCfg`(Config は全 string フィールドの comparable 構造体)なら **reader プールを開かず writer の `*sql.DB` を共有**する(非機能要件「二重に開かない」)。別ホストのときのみ 2 本目を開く。プール上限(`maxOpenConns` 等)・ping タイムアウトは writer と同一値を reader にも適用する。

4. **プール振り分けはコンポジションルート(cmd/<bin>/main.go)の配線で決める**(domain は関与しない)。infra 実装は「渡された `*sql.DB` を使う」だけ。どの集約のどのメソッドがどちらのプールを使うかは §「auth の correctness-critical read 配置」の表で確定。

5. **env: `cmd/<bin>/env.go` に `DB_READER_*` を追加、各項目は未設定時に対応する writer 値へフォールバック**。全項目未設定なら readerCfg は writerCfg と完全一致し、`OpenPair` が単一プールを共有する。`SelectMode`(Postgres/memory 選択)は **writer の `DB_HOST` のみ**を見る(不変)。env 読み取りは従来どおり `cmd/<bin>` に一本化(`infra/postgres` は env を読まない)。

### api と auth の非対称性(意図的・要 review-spec 確認)

- **api**: task の読み(`List`/`FindByID`/`FindByTitle`)と書き(`Save`)が同一 service(TaskService)を通り、**別プールへ振り分けるため service を Reader+Writer に分割**する。infra も `TaskReader`(readerDB)/ `TaskWriter`(writerDB)の**別構造体に分割**する(Spec §4 のスケッチに一致)。
- **auth**: プール境界が**集約境界に一致**する。read replica に逃がしてよい lookup は seed 済みで実行時に書かれない `user` / `client`(古い読みが安全)、強整合が要る `authcode` / `refreshtoken` は read も write も writer プール。**AuthorizationService は既に集約ごとに別リポジトリを保持している**(clients / users / authCodes / refreshTokens の 4 フィールド)ため、配線で「clients・users ← reader プール、authCodes・refreshTokens ← writer プール」と渡すだけで振り分けが成立し、**service 署名も infra 構造体分割も不要**。authcode/refreshtoken の postgres 実装は単一 `*sql.DB`(writer)のまま(reads も writes も writer)で、domain の Reader/Writer 分割は R1 準拠の型ドキュメントとして additive に足す。

この非対称は「**2 つのロールが別プールに束ねられる集約でのみ実装を物理分割する**」という一貫した規則の帰結(api task だけが該当)。

### 検討した代替案と不採用理由(本計画レベル)

| 案 | 不採用理由 |
|---|---|
| 合成 `Repository` を廃し全 service を Reader/Writer に付け替え | user/client は Writer が空になり無意味。auth service は集約単位でプールが割れるため付け替え不要。churn だけ増え価値が無い。合成 interface は共有 contract test と read+write 消費者(AuthorizationService)に load-bearing |
| api task も 1 構造体に readerQ/writerQ を保持し内部で振り分け(struct 非分割) | 動作はするが、Spec §4 が `TaskReader`/`TaskWriter` の別構造体を明示。別構造体の方が「読みが別物理プールへ」を統合テストで独立に検証しやすく(プール close で振り分けを可視化)、TaskService も Reader/Writer を素直に受け取れる。合成が要る箇所は互換 `NewTaskRepository(db)` を残して吸収 |
| authcode/refreshtoken の read も reader(replica)へ流す | 単一利用トークンの発行直後引き換え・rotation 検証が replica lag で可用性劣化(有効コードが未検出=invalid_grant)。単一利用/reuse 検出の正しさ自体は writer 上の atomic write が最終権威なので**破綻はしない**が、Spec の安全側既定(writer 側固定)に従い驚きを避ける。§リスク参照 |
| authcode/refreshtoken の Reader/Writer 分割を省略(R1 部分未達) | R1 は 4 集約すべての分割を要求。additive で churn ゼロなので満たす。ただし user/client は Writer 無し(Reader-only)とする解釈を明示 |

---

## auth の correctness-critical read の配置(確定)

各 lookup をどちらのプールに固定するかの表(review-security / review-spec で確認)。「役割」は domain の Reader/Writer 分類、「プール」は配線で渡す `*sql.DB`。

| 集約 | メソッド | 役割 | プール | 根拠 |
|---|---|---|---|---|
| client | `FindByID` | read | **reader** | seed 済み・実行時未書込。古い読みが安全=読み取りスケール対象 |
| user | `FindByID` / `FindByUsername` | read | **reader** | 同上(seed 済み) |
| authcode | `FindByCode` | read | **writer** | 発行直後引き換えの read-after-write。replica lag 回避の安全側既定 |
| authcode | `Save` / `Consume` | write | writer | 単一利用の atomic 権威 |
| refreshtoken | `FindByTokenHash` | read | **writer** | reuse 検出の pre-check。lag 回避の安全側既定 |
| refreshtoken | `Save` / `Rotate` / `RevokeFamily` | write | writer | rotation/atomic 権威(`Rotate` は writer 上 `BeginTx`) |

api:

| 集約 | メソッド | 役割 | プール |
|---|---|---|---|
| task | `FindByID` / `FindByTitle` / `ListPage` | read | **reader** |
| task | `Save` | write | writer |

- **正しさの担保**: authcode の単一利用は `Consume`(writer 上 atomic DELETE ... RETURNING)、refreshtoken の reuse 検出は `Rotate`(writer 上 tx)が最終権威。read を writer に固定するのは可用性(lag による未検出)を避ける安全側の措置で、既定フォールバック(reader=writer)では差が出ない。
- **api の read-after-write lag(既知・許容)**: TaskService の `Start`/`Complete`/`ChangePriority` は `FindByID`(reader)→ `Save`(writer)の read-modify-write。別ホスト reader 導入時、作成直後に開始すると replica lag で一時的に not-found になりうる。Spec が「replica lag のアプリ側強整合対処はスコープ外・seam と既知リスクの明示に留める」と定めるため **Spec の Reader 分類(FindByID=Reader)に従い reader プールへ流し、lag は §リスクに明記**。既定フォールバックでは無影響。

---

## 変更ファイル

### app/api — impl-api(domain / service / infra/memory / cmd/api)

| ファイル | 変更 |
|---|---|
| `domain/task/repository.go` | 単一 `Repository` を `Reader`(`FindByID`/`FindByTitle`/`ListPage`)+ `Writer`(`Save`)+ `Repository interface { Reader; Writer }` に分割(additive・メソッド増減なし) |
| `domain/task/service.go` | `DuplicateChecker` の依存を `Repository` → `Reader`(`NewDuplicateChecker(repo Reader)`)。`FindByTitle` のみ使用のため縮小 |
| `service/task_service.go` | `TaskService{repo}` → `TaskService{reader task.Reader; writer task.Writer}`。`NewTaskService(reader task.Reader, writer task.Writer, dupChk)`。読み経路(`Get`/`List`/`Start`等の `FindByID`)は `reader`、`Save` は `writer`。振る舞い不変 |
| `infra/memory/task_repository.go` | 単一構造体のまま。`var _ task.Reader` / `var _ task.Writer` の compile-time 表明を追加(behavior 変更なし。R5) |
| `cmd/api/env.go` | `Env` に `DBReader{Host,Port,Name,User,Password,SSLMode}` を追加。`NewEnv` で `DB_READER_*` を読み各項目を writer 値へフォールバック。`dbConfig()` を `writerConfig()` / `readerConfig()` に分割。`validate()` は Postgres モード時に writer/reader 双方の `Config.Validate()`(reader は fallback 済で writer 妥当なら妥当) |
| `cmd/api/main.go` | 配線を `OpenPair(ctx, writerCfg, readerCfg)` へ。`newTaskRepository` を `postgres.NewTaskReader(readerDB)` / `postgres.NewTaskWriter(writerDB)` の 2 構築へ。`dupChk := task.NewDuplicateChecker(taskReader)`、`svc := service.NewTaskService(taskReader, taskWriter, dupChk)`。memory 経路は同一 struct を reader/writer 両方に渡す。close は `OpenPair` が返す単一 close 関数を `defer` |

### app/api — impl-db(infra/postgres)

| ファイル | 変更 |
|---|---|
| `infra/postgres/db.go` | `OpenPair(ctx, writerCfg, readerCfg Config) (writer, reader *sql.DB, closeFn func() error, err error)` 追加。`readerCfg == writerCfg` → writer を1本開き reader=writer(同一ポインタ)・close は1回。別値 → reader を2本目に開く(失敗時 writer を close)・closeFn は両方 close(reader!=writer のときのみ reader も)。既存 `Open` は内部で再利用。プール上限/ping は現行定数を共用 |
| `infra/postgres/task_reader.go` | **新規**。`TaskReader{ q *sqlcgen.Queries }` + `NewTaskReader(db *sql.DB)`。`task_repository.go` の `FindByID`/`FindByTitle`/`ListPage` を移設。`var _ task.Reader = (*TaskReader)(nil)` |
| `infra/postgres/task_writer.go` | **新規**。`TaskWriter{ q *sqlcgen.Queries }` + `NewTaskWriter(db *sql.DB)`。`Save`(unique violation → `task.ErrDuplicateTitle`/`ConflictError`)を移設。`var _ task.Writer = (*TaskWriter)(nil)` |
| `infra/postgres/task_repository.go` | 共有ヘルパ(`taskFromRow`/`isUniqueViolation`/定数)を残す。互換合成型 `TaskRepository struct { *TaskReader; *TaskWriter }` + `NewTaskRepository(db *sql.DB) *TaskRepository`(reader=writer=db の単一プール)を提供。`var _ task.Repository = (*TaskRepository)(nil)`。**既存の統合 contract test は `NewTaskRepository(db)` のまま無改変で通る** |

### app/auth — impl-auth(domain / service / infra/memory / cmd/authz)

| ファイル | 変更 |
|---|---|
| `domain/authcode/repository.go` | `Reader`(`FindByCode`)+ `Writer`(`Save`/`Consume`)+ `Repository interface { Reader; Writer }` に additive 分割 |
| `domain/refreshtoken/repository.go` | `Reader`(`FindByTokenHash`)+ `Writer`(`Save`/`Rotate`/`RevokeFamily`)+ `Repository interface { Reader; Writer }` |
| `domain/user/repository.go` / `domain/client/repository.go` | **変更なし**(command メソッド無しの Reader-only ポート)。doc コメントに「query-only port(Reader 相当・Writer 無し)」の一文を追記(任意) |
| `service/userinfo_service.go` / `service/authorization_service.go` | **署名変更なし**。AuthorizationService は集約別リポジトリ保持のまま(プール振り分けは配線側)。UserInfoService は `user.Repository`(read-only)継続 |
| `infra/memory/{authcode,refreshtoken}_repository.go` | 単一構造体のまま。`var _ <agg>.Reader` / `var _ <agg>.Writer` の compile-time 表明を追加(R5) |
| `cmd/authz/env.go` | api と同型に `DB_READER_*` 追加・fallback、`writerConfig()`/`readerConfig()`、`validate()` で双方検証 |
| `cmd/authz/main.go` | `setupPersistence` を `OpenPair` ベースへ。Postgres 経路で `clientRepo := postgres.NewClientRepository(readerDB)`・`userRepo := postgres.NewUserRepository(readerDB)`・`authCodeRepo := postgres.NewAuthCodeRepository(writerDB)`・`refreshTokenRepo := postgres.NewRefreshTokenRepository(writerDB)`。seed は writer(`seedPostgres(ctx, writerDB)`)。close は `OpenPair` の closeFn。memory 経路は不変 |

### app/auth — impl-db(infra/postgres)

| ファイル | 変更 |
|---|---|
| `infra/postgres/db.go` | api と同一の `OpenPair` を追加(対称)。既存 `Open`/`Config`/`SelectMode` は不変 |
| `infra/postgres/{authcode,refreshtoken,user,client}_repository.go` | **メソッド変更なし**。additive interface を満たすことの `var _ <agg>.Reader`/`Writer`(authcode/refreshtoken)や `var _ <agg>.Repository`(既存)を必要に応じ追加。単一 `*sql.DB` 構築のまま |

### 横断規約 — impl-db

| ファイル | 変更 |
|---|---|
| `.claude/rules/db.md` | 「接続 env 契約」節に `DB_READER_{HOST,PORT,NAME,USER,PASSWORD,SSLMODE}` と per-項目 writer フォールバック、`SelectMode` は writer `DB_HOST` 基準(不変)を追記。`infra/postgres` の `OpenPair`(2 プール・reader==writer で単一プール共有=二重に開かない)と Reader/Writer seam、プール振り分け(api=task reads→reader、auth=user/client→reader・authcode/refreshtoken→writer 固定)を追記。「ポートは impl-api/auth・実装は impl-db」の既存 seam 記述は維持 |

### 変更なし(確認のみ)

- `app/migrator/**`(writer のみ・従来どおり)、`app/web/**`、`app/iac/**`(Spec スコープ外)
- sqlc 生成物(`infra/postgres/sqlcgen/**`)・`db/queries`・`db/migrations`(read/write で共通・スキーマ不変。`make sqlc` 差分ゼロを維持)

---

## 手順(担当 agent・順序・並列可否)

> `workflow.md` の TDD パイプライン。**infra 側シグネチャ(`OpenPair` / `NewTaskReader` / `NewTaskWriter`)は本計画で固定済み**のため、impl-api / impl-auth / impl-db は並列で着手し、cmd/<bin>/main.go の合流(コンパイル)で結線を検証する。

### フェーズ 1 — テスト先行(tester、TDD)
実装前に赤にする。要件×レベルは §テスト戦略の表。
- **api / auth `cmd/<bin>/env_test.go`(unit)**: `DB_READER_*` fallback。(a) 全 reader 未設定 → readerCfg == writerCfg(全フィールド一致)、(b) `DB_READER_HOST` のみ設定 → host=replica・他は writer fallback、(c) 各 `DB_READER_X` 個別上書き、(d) `SelectMode` は reader 有無に非依存(writer `DB_HOST` 基準)。table-driven・`t.Setenv`。
- **`infra/postgres` unit(既存 `persistence_selection_test.go` 系)**: `OpenPair` の「共有判定」を DB 非依存に検証できるよう、`readerCfg == writerCfg` の等価述語(Config 比較)を対象にした純ユニットを追加(実接続の pointer 一致は下記 integration)。
- **`infra/postgres` integration(`-tags=integration`)**:
  - `OpenPair` 共有: 等価 cfg → `writer == reader`(同一ポインタ)= 単一プール。別ホスト cfg → `writer != reader`。
  - **プール振り分け(close で可視化)**: 単一実 DB に 2 本の `*sql.DB` を開き `TaskWriter(writerDB)`/`TaskReader(readerDB)` を構築。readerDB を `Close` → `FindByID` は失敗・`Save` は成功(=読みは reader プール)。writerDB を `Close` → `Save` 失敗・`FindByID` 成功(=書きは writer プール)。第2 DB 不要で決定的。
  - auth 側(任意・軽め): authcode の `FindByCode`/`Consume` が writer プール固定であること(reader プール close 後も authcode 経路が影響を受けないこと)を同技法で 1 ケース。
- **既存 contract / integration の緑維持(R6)**: `repotest.RunTaskRepositoryContract` 等は合成 `Repository`(`NewTaskRepository(db)` / memory 単一構造体)に対して**無改変で通る**ことを確認。fake 追随のみ(下記)。
- **fake 追随(コンパイル維持)**: `service/task_service_test.go`・`domain/task/duplicate_checker_test.go` の fake は全メソッド実装済みで Reader/Writer/Repository を満たす。`NewTaskService(fake, dupChk)` → `NewTaskService(fake, fake, dupChk)` の呼び出し更新のみ。

### フェーズ 2 — 実装(impl-api ∥ impl-auth ∥ impl-db、並列)
- **impl-api**: `domain/task`(Reader/Writer/Repository)・`domain/task/service.go`(DuplicateChecker→Reader)・`service/task_service.go`(reader+writer)・`infra/memory`(assertions)・`cmd/api/env.go`(DB_READER_*)・`cmd/api/main.go`(OpenPair + NewTaskReader/Writer 配線)。
- **impl-auth**: `domain/{authcode,refreshtoken}`(Reader/Writer/Repository)・`domain/{user,client}`(doc のみ)・`infra/memory`(assertions)・`cmd/authz/env.go`(DB_READER_*)・`cmd/authz/main.go`(OpenPair + user/client←reader・authcode/refresh←writer 配線・seed=writer)。
- **impl-db**: 両 stack の `infra/postgres/db.go`(`OpenPair`)・api の `task_reader.go`/`task_writer.go`/`task_repository.go`(合成)・`.claude/rules/db.md`。
- 合流点: `var _ task.Reader = (*postgres.TaskReader)(nil)` 等の compile 表明と `cmd/<bin>/main.go` のビルドで、ポート(impl-api/auth)を実装(impl-db)が満たすことを担保。

### フェーズ 3 — テスト実行・チェック(tester → checker)
- tester: `cd app/api && make test`・`cd app/auth && make test`。実 DB 前提の `make test-integration` は postgres を用意して(またはローカル `make migrate` 後に)`-tags=integration` で実行し、`OpenPair` 共有/振り分けを確認。不足テストを補う。
- checker: 両 stack で `make check`(fmt-check + lint + vet + build + test)。`make sqlc` 差分ゼロ(スキーマ/クエリ不変)を確認。**web/iac 変更なしのため contract-drift は無関係**。

### フェーズ 4 — レビュー(review-security ∥ review-performance ∥ review-spec、並列)
- security: authcode/refreshtoken の read が writer 固定であること(§配置表)、`DB_READER_PASSWORD` 等が error/log に漏れないこと(既存 `Config.Validate` の name-only 方針を reader にも適用)、reader 資格情報の平文非記載。
- performance: reader==writer 時に**二重に開かない**こと(単一プール共有)、別ホスト時のプール上限が writer と同一で共有 RDS を枯渇させないこと、reader プールのライフタイム設定。
- spec: R1〜R6 と手順・テストの対応(§テスト戦略の表)、user/client を Reader-only とした R1 解釈、api の read-modify-write lag の扱い、auth service 署名不変の妥当性。
- Blocker/Major は impl agent へ差し戻し、フェーズ 3→4 を再実行。今回対応しない指摘は issue-creator が起票。

### フェーズ 5 — 記録
- admin/所定手順で Spec §5・経緯・frontmatter(status/updated)を更新。**本計画では docs/plans 以外は変更しない**(planner 制約)。

---

## テスト戦略

**TDD 先行**(フェーズ 1)。観点は 正常系 / 異常系 / 境界値。外部依存(DB)は interface 越し fake か `-tags=integration` の実 postgres(testing.md)。実時間 sleep・順序依存は使わない。

| 要件 | 検証 | レベル / 場所 |
|---|---|---|
| R1(Reader/Writer 分割) | 各集約に Reader/Writer/Repository が存在し既存実装が満たす | compile-time `var _`(memory/postgres)+ 既存 contract test が緑 |
| R2(2 プール振り分け) | 読み=reader プール・書き=writer プール | `infra/postgres` integration(プール close で可視化) |
| R3(未設定フォールバック) | reader 未設定 → 全クエリ従来どおり単一ホスト | env unit(readerCfg==writerCfg)+ `OpenPair` integration(同一ポインタ)+ 既存 contract/integration 緑 |
| R4(env 契約 + 個別フォールバック) | `DB_READER_*` 各項目未設定→writer 値、`SelectMode` は writer `DB_HOST` 基準 | `cmd/<bin>/env_test.go`(table-driven) |
| R5(memory 単一実装が両 interface) | 同一ストアで Reader/Writer を満たす | compile-time `var _` + 既存 memory/contract test |
| R6(service/route 振る舞い不変) | 戻り値・エラー契約・`errors.Is/As` 不変 | 既存 service/route/contract/integration test を無改変(fake 追随のみ) |
| 非機能(二重に開かない) | reader==writer で 1 プール共有 | `OpenPair` integration(pointer 一致)+ env unit(cfg 等価) |
| 配置(auth 単一利用 read=writer) | authcode/refreshtoken read が reader プール close の影響を受けない | `infra/postgres` integration(auth、1 ケース) |

- **別ホスト reader の再現**: 第2 データベースを用意せず、**単一実 DB に 2 本の `*sql.DB` を開き、片方を `Close` して「どちらのプールが使われたか」を成否で判定**する(reader を閉じれば読みだけ失敗、writer を閉じれば書きだけ失敗)。CI は既存 `api-integration`/`auth-integration`(postgres service + migrator)でそのまま走る。
- **フォールバック時の単一プール共有**は unit(cfg 等価)+ integration(pointer 一致)の二段で担保。
- **落ちるテストを skip/削除しない**。ポート分割で影響する fake(service/duplicate_checker)を漏れなく更新(フェーズ 1 に列挙)。

---

## リスク / 未確定事項

- **R-1 replica lag(既知・Spec スコープ外)**: 別ホスト reader 導入後、api の read-modify-write(`FindByID`→`Save`)や作成直後の閲覧が lag で一時 not-found になりうる。既定フォールバック(reader=writer)では無影響。強整合対処は Spec で明示的にスコープ外。**別 Spec(iac の replica プロビジョニング)着手時に、api の read-modify-write 経路を writer に寄せるか否かを再検討する**ことを review-spec で申し送り。
- **R-2 auth read の writer 固定は「可用性」目的で「正しさ」担保ではない**: 単一利用/reuse 検出の権威は writer 上の atomic write(`Consume`/`Rotate`)であり、read を reader に流しても破綻はしない。それでも Spec の安全側既定に従い writer 固定とする。この解釈を review-security で確認。
- **R-3 user/client を Reader-only とする R1 解釈(要 review-spec 合意)**: command メソッドが無いため空の Writer を作らず既存ポートを Reader 相当として据え置く。R1 の「Find/List→Reader、Save/Delete→Writer」を「Save/Delete が無ければ Writer 無し」と読む。合意が得られなければ空 Writer を足す軽微変更で対応可。
- **R-4 合成 `Repository` interface の要否 → 要る**: 共有 contract test(`RunTaskRepositoryContract` 等)と read+write 消費者(AuthorizationService の authcode/refreshtoken、postgres 合成型・memory 単一構造体)が依存するため互換維持する。将来 read model 分離(full CQRS)へ進める際に段階廃止できる seam として残す。
- **R-5 `OpenPair` の close 二重呼び回避**: `readerCfg == writerCfg` のとき reader=writer(同一ポインタ)。closeFn は `reader != writer` のときのみ reader も close する(`sql.DB.Close` は多重呼び安全だが、意図を明示して 1 回に保つ)。別ホストで reader open が失敗したら writer を close して error を返す(リーク防止)。
- **R-6 統合テストの実 DB 依存**: プール振り分けの検証は `-tags=integration` + 実 postgres 前提(既定 `make test` は緑のまま)。CI の `api-integration`/`auth-integration` に依存。第2 DB を作らない close 技法で単一インスタンスに収める。
- **R-7 reader 資格情報の権限設計はスコープ外**: ISSUE-016 の最小権限ロール(`api_app`/`auth_app`)と整合。reader が別資格情報を使う場合の権限(read-only ロール等)は iac スコープ。本 Spec は env 契約(`DB_READER_USER`/`PASSWORD`)を用意するに留める。
- **R-8 sqlc 生成物は不変**: read/write で共通の生成物を流用(Reader/Writer は Go の interface 分割のみ)。`make sqlc` 差分ゼロを checker で確認。スキーマ/クエリを触らないため `sqlc-drift` CI は無関係。
