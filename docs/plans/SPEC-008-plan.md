# SPEC-008 実装計画: app/api Task 一覧のページネーション(offset/limit + 封筒レスポンス)

- 起点: `docs/specs/20260710-008-api-task-list-pagination.md`(status: **draft**)/ 関連 `docs/issues/20260708-008-...md`(ISSUE-008)
- 対象 stack: `app/api`(domain / service / route / infra/memory)、`app/api` DB 層(db/queries / sqlc / db/migrations / infra/postgres)、`app/web`(api / hooks / components / mocks / router)、契約 = `app/api/docs/openapi.yaml` → web 生成物。app/auth は対象外
- 成果物: `GET /tasks` を `{items,total,limit,offset}` 封筒 + `limit`/`offset` クエリに変更(破壊的)、両永続化実装の範囲取得、`tasks(created_at, id)` インデックス、OpenAPI 再生成 + web 追従 + 最小ページャ

---

## ⚠️ 冒頭: 着手前に必要な admin / ユーザー判断

1. **Spec の承認ゲート**: SPEC-008 は現在 `status: draft`。`workflow.md` は「機能開発は status: approved にしてから着手」と定める。**本計画(T1)は draft でも作成できるが、T2 以降(テスト先行〜実装)は Spec を approved にしてから**着手する。承認は admin/ユーザーの操作。
2. **app/migrator リファクタの settle 確認**(最重要・後述リスク R-1): 起票時点で別セッションが `app/migrator`(DDL 適用ツール)を DDD レイヤへリファクタ中。**R7 のマイグレーション追加(`app/api/db/migrations` への新規ファイル + goose 適用経路)だけ**は当該リファクタが settle してから着手する。調査時点(2026-07-10)では `app/migrator` は既に DDD レイアウト(`cmd/domain/infra/service`)へ移行済みで作業ツリーは clean だが、**parent からの明示警告に従い R7 を最後段に隔離**し、他タスクをブロックしないよう手順を分離する。
3. **`limit=0` の扱い**(後述リスク R-4): 本計画のベースラインは「`limit < 1`(0 と負値を含む)は 400」。「0 は既定 20 にクランプ」を望む場合は着手前に確定を(小さな分岐なので後戻り可)。

上記が確定するまで、planner が本計画で固定した設計(port/DTO/検証仕様)に沿って準備(テスト設計)は進められるが、実装コミットは 1・2 のゲートを尊重する。

---

## 方針

### 採用アプローチ

1. **offset/limit + 封筒レスポンス**(Spec §4 の決定を踏襲)。ソートは既存の `created_at, id` 昇順(安定)を維持し、ページ境界で重複/欠落を出さない。
2. **ページング引数を domain の小さな値オブジェクト `task.Page` に閉じ込める**。既定(limit=20 / offset=0)・上限クランプ(limit>100→100)・下限検証(limit<1・offset<0→`*task.ValidationError`)という**業務ルールを domain の純ロジックとして一箇所に集約**する。route はワイヤ形式のパース(文字列→int、非整数は 400)だけを担い、business rule を持たない。これで「domain が最下層・他層非依存」を保ちつつ、既存エラー封筒(`{error}` 400)にそのまま乗る。
3. **Repository port は `FindAll` を範囲取得 + 総件数の 1 メソッドに置換する**:
   ```go
   // domain/task/repository.go
   ListPage(ctx context.Context, page Page) (items []*Task, total int, err error)
   ```
   封筒 `{items,total,...}` に 1:1 対応し、service を薄いオーケストレータに保つ。postgres 実装が内部で LIST + COUNT の 2 クエリを撃つ「詳細」を infra に閉じ込められる。
4. **service は薄いまま**: `List(ctx, limit, offset *int)`(nil = 未指定)を受け、`task.NewPage` で VO を構築(検証・クランプ・既定を適用)し `repo.ListPage` を呼び、封筒 DTO `service.TaskListDTO{Items, Total, Limit, Offset}` を返す。`Limit`/`Offset` は VO が確定した**適用後の値**をエコーする。
5. **契約は Go を正とした一方向生成**(SPEC-003): route の swag 注釈 → `make openapi` → `app/web` の `bun run generate`。web はこれを消費し、封筒対応 + 最小の次/前ページャを追加する。`contract-drift` / `sqlc-drift` CI が green になる導線を手順に組み込む。
6. **R7 のインデックスは正しさに不要な P1 最適化**なので、他の全変更(封筒・両 repo・契約・web)から**切り離して最後段**に置き、app/migrator リファクタの settle を待って追加する。

### 検証 / クランプ仕様(確定)

| 入力 | 挙動 | 実装位置 |
|---|---|---|
| `limit`/`offset` 省略(クエリキー無し or 空文字) | 既定 `limit=20` / `offset=0` を適用 | `task.NewPage`(nil 引数) |
| `limit` が非整数(例 `abc`) | **400**(`{error}`。`writeBadRequest`) | route(`strconv.Atoi` 失敗) |
| `offset` が非整数 | **400** | route |
| `limit < 1`(`0` / 負値) | **400**(`*task.ValidationError` → `writeError` で 400) | `task.NewPage` |
| `offset < 0` | **400**(同上) | `task.NewPage` |
| `limit > 100` | **100 にクランプ**(エコー値も 100。エラーにしない) | `task.NewPage` |
| 正常(例 `limit=20&offset=40`) | そのまま適用しエコー | `task.NewPage` |
| `offset` が total 以上 | `items=[]`(空)、`total`=総件数。エラーにしない | 両 repo |

`task.NewPage` の想定シグネチャ(impl-api / impl-db が並列着手できるよう固定):
```go
// domain/task/page.go
const (DefaultLimit = 20; MaxLimit = 100)
type Page struct { limit, offset int } // 非公開フィールド
func NewPage(limit, offset *int) (Page, error) // nil=未指定→既定 / 検証 / クランプ
func (p Page) Limit() int
func (p Page) Offset() int
```

### 封筒 DTO / レスポンス形状(確定)

```go
// service/dto.go
type TaskListDTO struct {
    Items  []TaskDTO
    Total  int
    Limit  int
    Offset int
}

// route/task_handler.go(swag 注釈対象・生成契約の正)
type taskListResponse struct {
    Items  []taskResponse `json:"items"  validate:"required"`
    Total  int            `json:"total"  validate:"required"`
    Limit  int            `json:"limit"  validate:"required"`
    Offset int            `json:"offset" validate:"required"`
}
```
- `validate:"required"` は swag に OpenAPI の `required`(キー存在)を出させるためで、値 0 は許容(int は常にキーが出力される)。既存 `taskResponse` と同じ手法。
- OpenAPI 追加: schema `route.taskListResponse`、`GET /tasks` に `limit`/`offset`(`query`, `int`, `false`)パラメータと `200 = taskListResponse` / `400 = errorResponse`。

### 退けた代替案(本計画レベル)

| 案 | 退けた理由 |
|---|---|
| `FindAll` を残し `ListPage` + `Count` を**追加**(interface 拡張) | fake 実装(route/service/duplicate_checker の各テスト・repotest)は新メソッドを足すため**どのみち全て編集が要る**。得られる差分は「FindAll 本体を消さない/repotest の FindAll サブテストを書き換えない」だけで、代償に**本番未使用の port メソッド(dead code)**が残りレビュー指摘になりやすい。よって置換を採用 |
| 範囲取得と件数を 2 メソッド(`ListRange` + `Count`) | 封筒は常に両方を要するので 1 メソッド(items+total)が use case に素直。service の呼び出しも 1 回で済む。`Count` 単独の再利用需要は現時点で無い |
| クランプ/検証を route に置く | business rule(既定・上限・下限)が presentation に散る。domain VO 集約で単体テスト可能・両 repo と route で同一ルールを共有 |
| web で全件取得のままクライアントページング | §1 の O(n) 全件取得が残り価値を満たさない(Spec §4) |

---

## 変更ファイル

### app/api(impl-api: domain / service / route / infra/memory)

| ファイル | 変更 |
|---|---|
| `domain/task/page.go` | **新規**。`Page` VO + `NewPage` + `Limit/Offset` + 既定/上限/下限定数。business rule の単一ソース |
| `domain/task/repository.go` | `FindAll(ctx) ([]*Task, error)` を **`ListPage(ctx, Page) ([]*Task, int, error)` に置換** |
| `service/dto.go` | `TaskListDTO`(封筒)追加 |
| `service/task_service.go` | `List(ctx)` → `List(ctx, limit, offset *int) (TaskListDTO, error)`。`NewPage` 構築 + `repo.ListPage` + DTO 組み立て(適用値エコー) |
| `route/task_handler.go` | `list` ハンドラ: `limit`/`offset` パース(非整数→400)→ `svc.List` → 封筒 `taskListResponse` を返す。`taskListResponse` 型追加。swag 注釈を query パラメータ + `200 {object} taskListResponse` + `400` に更新 |
| `infra/memory/task_repository.go` | `FindAll` → `ListPage`。**`created_at, id` で決定的にソート**(現状は map 走査で無順序)してから `[offset:offset+limit]` を切り出し、`total = len(map)` |
| `route/response.go` | 変更不要見込み(既存 `writeError`/`writeBadRequest` が 400/`{error}` を提供)。必要時のみ微修正 |

`make openapi` を impl-api が実行し、`app/api/docs/openapi.yaml` の差分をコミット(生成であり検査ではない)。

### app/api DB 層(impl-db: db/queries / sqlc / infra/postgres / db/migrations)

| ファイル | 変更 |
|---|---|
| `db/queries/tasks.sql` | `ListTasks`(既存の無制限一覧)を**削除または残置**し、`ListTasksPage :many`(`... ORDER BY created_at, id LIMIT $1 OFFSET $2`)と `CountTasks :one`(`SELECT COUNT(*) FROM tasks`)を追加 |
| `infra/postgres/sqlcgen/*`(生成物) | `make sqlc` で再生成しコミット(手編集しない)。`sqlc-drift` CI 対象 |
| `infra/postgres/task_repository.go` | `FindAll` → `ListPage`(`CountTasks` + `ListTasksPage` を撃ち、`taskFromRow` で復元、`int64`→`int` 変換、`total` を返す)。DBError ラップは既存踏襲 |
| `db/migrations/000002_add_tasks_created_at_index.sql` | **新規(R7・最後段・migrator settle 後)**。up: `CREATE INDEX ... ON tasks (created_at, id)` / down: `DROP INDEX ...`。連番は着手時に `db/migrations` を再確認して確定 |

**注意(port 契約の分担、`.claude/rules/db.md`)**: `ListPage` の interface 定義(port)は impl-api、`infra/postgres` 実装は impl-db。両者が並列で進められるよう、本計画でシグネチャを固定済み。

### app/web(impl-web: 契約再生成後)

| ファイル | 変更 |
|---|---|
| `src/features/tasks/api/generated/**` | `bun run generate` で再生成しコミット(封筒型 / zod / `getTasks` の `query` 引数)。`contract-drift` CI 対象 |
| `src/features/tasks/api/schema.ts` | `taskListSchema = z.array(taskSchema)` を**生成封筒スキーマ `zRouteTaskListResponse` に差し替え**。`taskSchema`(単体)は温存。封筒→ドメイン `{items:Task[],total,limit,offset}` へのマッパ追加 |
| `src/features/tasks/api/client.ts` | `fetchTasks(params)` を `getTasks({ query:{limit,offset}, throwOnError:true })` → 封筒パース → `items.map(toDomain)` → `TaskPage` 返却に変更 |
| `src/features/tasks/hooks/useTasks.ts` | `useTasksQuery({limit,offset})`(封筒 `TaskPage` を返す)。`queryKey = ["tasks","list",{limit,offset}]`(mutations の `["tasks"]` 無効化は prefix 一致で継続) |
| `src/features/tasks/components/TaskList.tsx` | `data.items` に既存 `filterByStatus`/`sortByPriority` を適用。`TaskPager` を描画 |
| `src/features/tasks/components/TaskPager.tsx` | **新規**。最小の次/前。`offset<=0` で前を無効、`offset+limit>=total` で次を無効。押下で URL search の `offset` を更新。`web.md` 準拠(タップ 44px・`flex-wrap`・狭幅で崩れない) |
| `src/features/tasks/components/TaskSummary.tsx` | 現状ページ内 items でのカウントになる旨のみ(挙動は据え置き。リスク R-5 参照) |
| `src/features/tasks/index.ts` | `TaskPager` の export 追加・`useTasksQuery` 型更新 |
| `src/app/router.tsx` | `taskListSearchSchema` に `limit`(既定 20)/`offset`(既定 0)を `z.coerce.number()` + `.catch` で追加(既存 `status` と同じ zod 検証パターン) |
| `src/mocks/handlers.ts` | `GET /api/tasks` を `searchParams` の limit/offset に応じ「fixture を `created_at,id` ソート → クランプ/既定 → slice → `{items,total,limit,offset}`」に変更 |

### 変更なし(確認のみ)

- `app/auth/**`(対象外)、`app/iac/**`、`app/api/cmd/api/main.go`(配線は不変。`ListPage` 追加で interface を満たす)。

---

## 手順(担当 agent・順序・並列可否)

> フェーズ間の順序は `workflow.md` の TDD パイプライン。並列可能な箇所を明記。**契約(web)生成は Go 側 DTO 確定後**という一点だけが厳格な依存。

### フェーズ 0(前提ゲート)
- admin/ユーザー: Spec を `approved` に(冒頭 1)。app/migrator の settle 確認(冒頭 2)。`limit=0` 方針の確定(冒頭 3)。

### フェーズ 1 — テスト先行(tester、TDD)※ Go 側と web ロジック骨子
tester が要件からテストを先に作成(実装前に赤)。web の生成型に依存する部分は生成後(フェーズ 4)に確定させる二段構え。
- **Go(即時に書ける)**:
  - `domain/task/page_test.go`(新規): 既定/クランプ/下限検証/正常(R1・R3 の境界)
  - `service/task_service_test.go`: `List` のページング(fake の `ListPage` に置換)+ 適用値エコー(R1・R2)
  - `route/task_handler_test.go`: list の `?limit=&offset=` 反映・封筒 JSON 形状・`?limit=abc`→400・`?offset=-5`→400・`?limit=1000`→200 かつ echo limit=100(R1・R2・R3)。既存 fake(`failingRepository`/`dbErrorRepository`)を新 interface に追随
  - `infra/repotest/task_contract.go`: `FindAll` サブテスト群を `ListPage`(+ `total`)へ書換(R5)。空・全件・upsert 後件数=1 を `ListPage(全件相当ページ)` で表現
  - `infra/memory/task_repository_test.go`: `ListPage` の順序(`created_at,id`)・範囲・offset 超過→空(R5)
  - `infra/postgres/task_repository_integration_test.go`: `ListPage`/`CountTasks` の境界 + 閉接続 DBError(R5)。`-tags=integration`
  - `service/duplicate_checker_test.go`: fake の interface 追随(コンパイル維持)
- **web(骨子。生成型待ちの箇所はスタブ/後追い)**: `TaskPager.test.tsx`(境界の活性/非活性・offset 更新)、`TaskList.test.tsx`(封筒 items 描画)、`useTasks.test.tsx`(queryKey に limit/offset)、`client.test.ts`(封筒パース + query 送出)、`schema.test.ts`(封筒スキーマ)。MSW は封筒返却へ更新。

### フェーズ 2 — Go 実装(impl-api ∥ impl-db、並列)
シグネチャは本計画で固定済みのため並列可。
- **impl-api**: `domain/task/page.go` / `repository.go`(port 置換)/ `service` / `route`(ハンドラ + swag)/ `infra/memory`。
- **impl-db**: `db/queries/tasks.sql`(`ListTasksPage`/`CountTasks`)→ `make sqlc` 再生成 → `infra/postgres/task_repository.go` の `ListPage`。**R7 マイグレーションはこのフェーズでは着手しない**(フェーズ 6 へ隔離)。
- 合流点: port(impl-api)を実装(impl-db)が満たすことを `var _ task.Repository = ...` のコンパイル確認で担保。

### フェーズ 3 — 契約生成(impl-api)
- impl-api が `cd app/api && make openapi` を実行、`docs/openapi.yaml`(新 schema・query・400)をコミット。**ここで Go 側 wire 契約を確定**。

### フェーズ 4 — web 実装(impl-web)※ フェーズ 3 の後(契約依存)
- `cd app/web && bun run generate` で生成物更新・コミット。
- `schema.ts` / `client.ts` / `useTasks.ts` / `TaskList.tsx` / `TaskPager.tsx`(新規)/ `router.tsx` / `mocks/handlers.ts` / `index.ts` を封筒 + ページャ対応(R2・R6)。
- tester がフェーズ 1 で置いた web テストを生成型に合わせて確定。

### フェーズ 5 — テスト実行・チェック(tester → checker)
- tester: `cd app/api && make test`(+ 必要に応じ `make test-integration` は postgres 前提)、`cd app/web && bun run test`。不足テスト追加。
- checker: `app/api` `make check` / `app/web` `format:check`・`lint`・`typecheck`・`build`。**contract-drift 相当**の整合(`make openapi` 再実行差分ゼロ・`bun run generate` 差分ゼロ・`make sqlc` 差分ゼロ)を確認(R4)。CI では `contract-drift.yml` / `sqlc-drift.yml` が最終ゲート。

### フェーズ 6 — R7 インデックス(impl-db)※ app/migrator settle 後・最後段
- impl-db: `db/migrations` の連番を再確認 → `make migrate-create name=add_tasks_created_at_index` → up/down を記述(`(created_at, id)`)。
- 健全性は CI の `api-integration`(postgres service で app/migrator 経由 up→down→up)が検査(R7)。ローカル確認はルート `make migrate`(`-target api`)。**`terraform`/本番適用は行わない**。

### フェーズ 7 — レビュー(review-security ∥ review-performance ∥ review-spec、並列)
- security: クエリのパラメータ化(sqlc)・offset/limit の数値化で injection 面が無いこと、深い offset の DoS 懸念(上限 100 クランプで緩和)。
- performance: `ORDER BY created_at,id` + `(created_at,id)` インデックスの効き、LIST/COUNT 2 クエリの妥当性、web の再フェッチ/queryKey 設計。
- spec: R1〜R7 と手順・テストの対応(下表)を確認。
- Blocker/Major は impl agent へ差し戻し、フェーズ 5→7 を再実行。今回対応しない指摘は issue-creator が起票。

### フェーズ 8 — 記録
- admin/所定手順で Spec の §5・経緯・frontmatter(status/updated)を更新、ISSUE-008 相互リンク更新。**本計画では docs/plans 以外は変更しない**(parent 制約)。

---

## テスト戦略

**TDD 先行**(フェーズ 1)。観点は 正常系 / 異常系 / 境界値。要件との対応:

| 要件 | 検証 | レベル / 場所 |
|---|---|---|
| R1(query + 既定) | 省略時 20/0、指定時反映、echo | domain `page_test`(既定)、route handler test、service test |
| R2(封筒形状) | `{items,total,limit,offset}` JSON、`TaskListDTO` | route handler test、service test、web `schema.test`/`client.test` |
| R3(クランプ + 400) | >100→100、<1/負→400、非整数→400 | domain `page_test`(クランプ/下限)、route handler test(非整数/負/超過) |
| R4(契約単一ソース) | 再生成差分ゼロ | checker + CI `contract-drift` / `sqlc-drift`(単体テストではなくゲート) |
| R5(両 repo 同一契約) | `ListPage`+`total` の空/全件/範囲/offset 超過/順序 | `repotest` 共有契約(memory=default build / postgres=integration build)+ memory 単体 + postgres integration |
| R6(web ページャ) | 次/前の活性・境界・offset 更新、封筒消費 | `TaskPager.test`、`TaskList.test`、`useTasks.test`、`client.test`、MSW |
| R7(index) | up→down→up 健全性 | CI `api-integration`(実 DB)。ローカル `make migrate` |

- **外部依存は interface 越し**(testing.md): service/route は fake `task.Repository`、postgres は integration タグ + 実 postgres。
- **実時間/順序依存を避ける**: 時刻は固定値・`created_at,id` の決定的ソートで安定化(memory も postgres と同順序に揃える)。
- **落ちるテストを skip/削除しない**。`FindAll`→`ListPage` 置換で影響する全 fake(route/service/duplicate_checker/repotest/memory/postgres)を漏れなく更新(フェーズ 1 のチェックリストに列挙済み)。

---

## リスク / 未確定事項

- **R-1(最重要)app/migrator リファクタとの衝突**: 別セッションの app/migrator リファクタ中は R7(新規マイグレーション + goose 適用経路)が衝突しうる。調査時点で app/migrator は DDD 化済み・作業ツリー clean だが parent 警告に従い**R7 をフェーズ 6 に隔離**し、封筒/両 repo/契約/web(正しさの本体)を先行させる。impl-db は着手時に (a) `db/migrations` の空き連番、(b) migrator のマイグレーション探索パス(`-migrations-dir` 既定 `/migrations/<target>` の COPY レイアウト)が変わっていないか、(c) `api-integration` の up→down→up が green か、を再確認する。settle 未確認なら R7 を保留し他フェーズのみ完了させる運用も可(index は P1 最適化で封筒機能の前提ではない)。
- **R-2 破壊的変更**: `GET /tasks` 配列→封筒。web・MSW・全テストを同一変更内で更新する(未更新は typecheck/build/test と `contract-drift` CI が検出)。Spec §4 で「サンプルアプリ・外部クライアント無し・drift を CI 検出」を根拠にバージョニング無し一括切替を承認済み。
- **R-3 `FindAll` 置換の波及**: port 置換で 7+ 箇所の実装/fake/テストが連動(本計画で列挙)。漏れはコンパイルエラーで露見するが、tester のチェックリストで先回りする。
- **R-4 `limit=0` の扱い(要確認)**: ベースラインは「<1 は 400」。API 慣習として「0→既定にクランプ」も一般的。`task.NewPage` の 1 分岐で切替可能なので、確定が付くまではベースラインで実装し、変更要望が出ても影響は局所。
- **R-5 web のサマリ/フィルタの意味論変化(既知の制約)**: 現状 `TaskSummary`(件数)と `filterByStatus`(client 側絞り込み)は全件前提。ページネーション後は**現在ページ内**の items にしか作用しない(他ページに該当タスクがあってもフィルタ結果が空になり得る)。Spec のスコープ外(ステータス絞り込み/検索は将来の別 Spec、サマリの全件集計には別エンドポイントが要る)。今回は「ページ内での据え置き挙動 + 最小の次/前」に留め、**この制約を review-spec で明示・必要なら後続 ISSUE 化**する。
- **R-6 LIST/COUNT の非トランザクション整合**: `ListPage` は `CountTasks` と `ListTasksPage` を別クエリで撃つため、同時書き込みで `total` と `items` に僅かな不整合が生じ得る。サンプル規模では許容(単一トランザクション/`COUNT(*) OVER()` は将来最適化)。
- **R-7 memory の順序保証追加**: `infra/memory` はこれまで map 走査で無順序。ページ境界の重複/欠落を防ぐため `created_at,id` の決定的ソートを追加する(postgres と一致)。同 `created_at` の並びは `id` 昇順で確定。
- **R-8 web ページ状態の置き場所**: URL search params(`offset`/`limit`、既存 `status` と同じ zod 検証)に置く方針。`page` 番号方式ではなく `offset` 直置きにするのは API 引数と 1:1 で写像が単純なため。UX 上の妥当性(リンク共有・戻る操作)も満たす。
