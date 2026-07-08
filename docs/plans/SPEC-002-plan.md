# SPEC-002 実装計画: Task に優先度(priority)を追加する

- 起点: `docs/specs/20260708-002-task-priority.md`(status: approved)
- 関連: `docs/issues/20260708-008-api-task-list-pagination-and-single-fetch-contract.md`(単一取得 `GET /tasks/{id}` の DTO と priority を同期)
- 後続依存: **SPEC-003(Go⇄TS 型共有基盤 / OpenAPI B2 方式、approved)が本 SPEC-002 を先行前提(drift D1)としている**(着手順 SPEC-002 → SPEC-003)。SPEC-003 は「**Go を契約の正とし、web を Go に合わせる**」方針で、SPEC-002 追加後の DTO / エンドポイントを swag で OpenAPI 化する。本計画はこの前提に合わせて **(a) 新設エンドポイントを app/api 既存の action-style に統一**し、**(b) DTO を素直な形(余計な変換を挟まない)に保つ**(swag アノテーション対象になるため)。SPEC-002 時点では swag 対応そのものは不要。
- 対象 stack: app/api のみ(app/web の enrichment 撤去は本 Spec 完了後の別作業。スコープ外)

## 方針

app/api の既存 DDD レイヤ構成(`domain → service → route`、`infra` は依存性逆転)と、Task 集約の「非公開フィールド + 値オブジェクト + 振る舞いメソッド」方式を厳密に踏襲して priority を追加する。

- **domain**: `Status` と同型の値オブジェクト `Priority`(`struct { value string }`)を新設する。既存の `Status` / `Title` / `ID` がすべてこの struct ラッパ方式で、`ParseStatus` が sentinel(`ErrInvalidStatus`)で不正値を弾く形になっているため、`Priority` / `ParsePriority` / `ErrInvalidPriority` を完全に対称に作る。Go の string 定義型(`type Priority string`)ではなくこの struct ラッパを採用する理由は、コードベースの既存3値オブジェクトと一貫させ、ゼロ値(`Priority{}`)が「未検証状態」として型で表現できるため。
- **既定値(medium)の適用位置**: `ParsePriority` は `ParseStatus` と対称に **空文字を不正として弾く(strict)**。「作成時に未指定なら medium」という *フィールド不在* の既定化は application 層(`service.Create`)の責務とする(空文字 → `task.PriorityMedium`、非空 → `ParsePriority`)。domain に `ParsePriorityOrDefault` を置く代替案は退けた: 「フィールドが無い/空」は wire フォーマット由来の境界事情であり domain の語彙に持ち込みたくないこと、`ParseStatus` の strict な既存契約と非対称になることが理由。変更(`ChangePriority`)には既定概念が無く、常に明示指定を strict にパースする。
- **priority と状態遷移の直交**: `ChangePriority` は `Rename` と同型 — 既に検証済みの値オブジェクトを受け取り、フィールドを差し替えて `updatedAt` を更新するだけ(エラーを返さない)。不正値の排除はコンストラクション時(`ParsePriority`)に済んでいるため、集約の状態機械(todo→doing→done)には一切触れない。これで非機能要件「priority は状態遷移と直交」を型と構造で保証する。
- **priority 変更エンドポイント(R3)**: `POST /tasks/{id}/priority`(body `{"priority":"high"}`、成功時 200 + 完全な `taskResponse`)を新設する。app/api の既存変更系エンドポイント(`POST /tasks/{id}/start` / `POST /tasks/{id}/complete`)の **action-style を踏襲**する。web 側の変更系は `PATCH /api/tasks/:id/status` だが、**SPEC-003 が「Go を契約の正とし web を Go に合わせる」方針**であり、後続の swag による OpenAPI 化で Go 側の規約一貫性を最優先すべきため、web の PATCH に寄せず app/api 既存の POST action-style に統一する。start/complete が body を取らないのに対し priority は値の代入で body を必須とする点だけが異なるが、パス・メソッドの規約(`POST /tasks/{id}/{action}`)は揃える(代替案の検討は「リスク / 未確定事項」参照)。

## 変更ファイル

### domain(`app/api/domain/task/`)— impl-api

| ファイル | 種別 | 変更内容 |
|---|---|---|
| `priority.go` | 追加 | `Priority` 値オブジェクト、`PriorityLow/Medium/High`、`String()`、`ParsePriority(s)` |
| `errors.go` | 変更 | sentinel `ErrInvalidPriority` を追加(`ErrInvalidStatus` と対称) |
| `task.go` | 変更 | `Task.priority` フィールド追加 / `New` と `Reconstruct` のシグネチャ変更 / `Priority()` getter / `ChangePriority(Priority)` 追加 |

### service(`app/api/service/`)— impl-api

| ファイル | 種別 | 変更内容 |
|---|---|---|
| `dto.go` | 変更 | `TaskDTO.Priority string \`json:"priority"\`` 追加、`newTaskDTO` で反映 |
| `task_service.go` | 変更 | `Create` シグネチャ変更(priority 受領・空→medium)、`ChangePriority` メソッド追加 |

### infra(`app/api/infra/memory/`)— impl-api

| ファイル | 種別 | 変更内容 |
|---|---|---|
| `task_repository.go` | 変更 | `clone()` が `Reconstruct` に priority を渡すよう修正(Repository interface は不変) |

### route(`app/api/route/`)— impl-api

| ファイル | 種別 | 変更内容 |
|---|---|---|
| `task_handler.go` | 変更 | `createTaskRequest.Priority` / `taskResponse.Priority`(snake_case)/ `newTaskResponse` / `changePriorityRequest` + `changePriority` ハンドラ |
| `router.go` | 変更 | `mux.HandleFunc("POST /tasks/{id}/priority", h.changePriority)` を登録 |
| `response.go` | 変更 | `writeError` に `errors.Is(err, task.ErrInvalidPriority)` → 400 のケース追加 |

### cmd — 変更なし

`cmd/api/main.go` は配線のみで `NewTaskService` / `NewRouter` のシグネチャは不変のため **変更不要**。

### テスト(`_test.go`)— tester

| ファイル | 種別 | 変更内容 |
|---|---|---|
| `domain/task/priority_test.go` | 追加 | `ParsePriority` の table-driven(low/medium/high/unknown/empty) |
| `domain/task/task_test.go` | 変更 | 既存 `task.New(title)` / `Reconstruct(...)` 呼び出しをシグネチャ変更に追従。`New` が priority を保持すること、`ChangePriority`、priority×状態遷移の直交を追加 |
| `domain/task/duplicate_checker_test.go` | 変更 | 60 行目の `task.New(title)` をシグネチャ変更に追従(機械的修正のみ) |
| `infra/memory/task_repository_test.go` | 変更 | 18 行目の `task.New(tt)` をシグネチャ変更に追従。clone 経由で priority が保持されることを追加 |
| `service/task_service_test.go` | 変更 | 既存 `svc.Create(ctx, title)` 全呼び出しをシグネチャ変更に追従。既定値・明示指定・`ChangePriority`(正常/不正)を追加 |
| `route/task_handler_test.go` | 変更 | `taskResponseBody` に priority 追加。作成の既定/明示、`POST /tasks/{id}/priority`(正常/不正/未存在)、全レスポンス DTO に priority が乗ることを追加 |

## 正確な契約(tester と impl-api が食い違わないための固定仕様)

以下の名前・シグネチャ・enum・既定値・エンドポイントを **確定** とする。tester・impl-api は双方これに従い、相互に前提を推測しない。

### domain: `Priority` 値オブジェクト(`priority.go`)

```go
// Priority is a value object representing a Task's priority.
type Priority struct {
	value string
}

var (
	PriorityLow    = Priority{value: "low"}
	PriorityMedium = Priority{value: "medium"}
	PriorityHigh   = Priority{value: "high"}
)

// String returns the underlying string representation.
func (p Priority) String() string { return p.value }

// ParsePriority validates and converts s into a Priority. It returns
// ErrInvalidPriority if s does not match any known priority value.
// The empty string is invalid (defaulting to medium on task creation
// is an application-layer concern, not a domain one).
func ParsePriority(s string) (Priority, error) {
	switch s {
	case PriorityLow.value:
		return PriorityLow, nil
	case PriorityMedium.value:
		return PriorityMedium, nil
	case PriorityHigh.value:
		return PriorityHigh, nil
	default:
		return Priority{}, fmt.Errorf("task: parse priority %q: %w", s, ErrInvalidPriority)
	}
}
```

### domain: sentinel(`errors.go`)

```go
// ErrInvalidPriority is returned when a Priority cannot be parsed from a string.
ErrInvalidPriority = errors.New("task: invalid priority")
```

### domain: `Task`(`task.go`)

- フィールド追加: `priority Priority`(非公開)
- コンストラクタ変更(**破壊的**): `func New(title Title, priority Priority) *Task` — priority を必須引数として受ける(空既定化は service 側)
- 再構築変更(**破壊的**): `func Reconstruct(id ID, title Title, status Status, priority Priority, createdAt, updatedAt time.Time) *Task` — priority を status の直後に挿入
- getter 追加: `func (t *Task) Priority() Priority`
- 振る舞い追加: `func (t *Task) ChangePriority(priority Priority)` — `Rename` と同型(検証済みの値を代入し `updatedAt` を更新。エラーなし。status は不変)

### service(`task_service.go` / `dto.go`)

- `TaskDTO` に `Priority string \`json:"priority"\`` を追加、`newTaskDTO` で `Priority: t.Priority().String()`
- 作成(**破壊的**シグネチャ変更): `func (s *TaskService) Create(ctx context.Context, title, priority string) (TaskDTO, error)`
  - priority の既定化: `if priority == "" { p = task.PriorityMedium } else { p, err = task.ParsePriority(priority); if err != nil { return TaskDTO{}, fmt.Errorf("service: create task: %w", err) } }`
  - `task.New(t, p)` を呼ぶ
- 変更(新規): `func (s *TaskService) ChangePriority(ctx context.Context, id, priority string) (TaskDTO, error)`
  - `Start`/`Complete` と同型: `ParseID` → `ParsePriority`(strict。空も不正)→ `FindByID` → `t.ChangePriority(p)` → `Save` → `newTaskDTO(t)`。ラップは `"service: change priority: %w"`

### infra/memory(`task_repository.go`)

- `clone()` のみ変更: `task.Reconstruct(t.ID(), t.Title(), t.Status(), t.Priority(), t.CreatedAt(), t.UpdatedAt())`
- Repository interface(`Save`/`FindByID`/`FindByTitle`/`FindAll`)は不変。permanent storage 表現は `*task.Task` のままで、priority は集約に内包されるため追加のマップやカラム相当は不要

### route / DTO(`task_handler.go` / `router.go` / `response.go`)

- 作成リクエスト: `createTaskRequest` に `Priority string \`json:"priority"\`` を追加(**optional**。未指定/空 → service で medium 既定)。`create` ハンドラは `h.svc.Create(r.Context(), req.Title, req.Priority)` を呼ぶ
- レスポンス DTO(**すべての** Task レスポンス = list / get / create / start / complete で共有される `taskResponse`): `Priority string \`json:"priority"\`` を追加、`newTaskResponse` で `Priority: dto.Priority`。`taskResponse` は全ハンドラが `newTaskResponse` 経由で使うため、1 箇所の追加で list / get / create / 状態遷移すべてに priority が乗る
- priority 変更エンドポイント: `POST /tasks/{id}/priority`(action-style。start/complete と同系)
  - リクエスト型 `changePriorityRequest { Priority string \`json:"priority"\` }`
  - ハンドラ `changePriority`: body を decode(失敗は `writeBadRequest`)→ `h.svc.ChangePriority(r.Context(), r.PathValue("id"), req.Priority)` → 成功は `writeJSON(w, http.StatusOK, newTaskResponse(dto))`
  - `router.go` に `mux.HandleFunc("POST /tasks/{id}/priority", h.changePriority)` を追加
- エラー写像: `response.go` の `writeError` に `case errors.Is(err, task.ErrInvalidPriority): writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid priority"})` を追加(`ErrInvalidStatus` 等と同じ 400 系の並び)

### 既存呼び出し側への影響(シグネチャ変更の波及・すべて列挙)

`New` / `Reconstruct` / `Create` の破壊的変更で、以下が同一 module 内でコンパイル対象になる。production は impl-api、`_test.go` は tester が担当(下の「テスト戦略」で分担を厳密化):

- `task.New` の呼び出し: production = `service/task_service.go:40`。test = `domain/task/task_test.go`(7 箇所)/ `domain/task/duplicate_checker_test.go:60` / `infra/memory/task_repository_test.go:18`
- `task.Reconstruct` の呼び出し: production = `infra/memory/task_repository.go:111`。test = `domain/task/task_test.go:147`
- `svc.Create` の呼び出し: production = `route/task_handler.go:51`。test = `service/task_service_test.go`(11 箇所)
- `route/task_handler_test.go` は JSON body ベースで Go シグネチャに非依存のため、シグネチャ変更では **壊れない**(既存の `taskResponseBody` は priority 欠落でも余剰 JSON フィールドを無視して decode 可能)。priority を検証するために tester が拡張する

## 手順

Go は 1 ディレクトリ = 1 パッケージが丸ごとコンパイルされ、tester(`_test.go`)と impl-api(production `*.go`)で編集ファイルが分離する。破壊的シグネチャ変更を含むため、**計画で固定した上記「正確な契約」を唯一の真実として TDD 先行**で進める(理由はテスト戦略参照)。

1. **tester(TDD 先行)** — 上記契約に従い全 `_test.go` を作成/更新する:
   - 新規: `domain/task/priority_test.go`
   - 既存の機械的追従: `domain/task/task_test.go` / `domain/task/duplicate_checker_test.go` / `infra/memory/task_repository_test.go` / `service/task_service_test.go`(シグネチャ変更に合わせて `New`/`Reconstruct`/`Create` 呼び出しを更新)
   - 観点追加: `route/task_handler_test.go`(priority の入出力・`POST /tasks/{id}/priority` エンドポイント)
   - この時点で `make build` / `make test` は **コンパイルエラーで赤**(production 未実装)。これは TDD の想定内で、契約が固定されているため手戻りにはならない
2. **impl-api** — 上記契約に従い production を実装する(domain → service → infra → route の依存順で編集。cmd は変更不要):
   - `domain/task/priority.go`(新規)/ `errors.go` / `task.go`
   - `service/dto.go` / `task_service.go`
   - `infra/memory/task_repository.go`
   - `route/task_handler.go` / `router.go` / `response.go`
   - impl-api は `_test.go` を編集しない(tester の成果物)。この段階で全パッケージが緑になる想定
3. **tester** — `make test`(必要に応じ `make test-race`)を実行し、全テストが緑であることを確認。不足観点があれば `_test.go` にのみ追加
4. **checker** — `make check`(fmt-check + lint + vet + build + test)を app/api で実行し合否を判定
5. **review-security / review-performance / review-spec(並列)** — checker 通過後に起動。review-spec は R1–R5 と本計画の対応表(下記)を突き合わせる
6. **指摘対応** — Blocker/Major は impl-api(production)/ tester(test)へ差し戻し、手順 3→5 を再実行。今回対応しない指摘は issue-creator が起票
7. **admin** が SPEC-002 の「5. 実装計画」チェックボックスと経緯・status を `spec` skill 手順で更新して完了

- **並列可能箇所**: 手順 1(tester)と 2(impl-api)は編集ファイルが完全分離(`_test.go` ↔ production `*.go`)のため物理的な衝突は無い。ただし 2 は 1 が固定する契約に依存するため、**論理的には 1 → 2 の順**を推奨(計画で契約が固定されているので厳密な待ち合わせは不要だが、赤→緑の確認を段階化するため順次実行が安全)。手順 5 のレビュー3種は並列。

## テスト戦略

**推奨: 計画で正確なシグネチャを固定した上での TDD 先行(手順 1 → 2)。**

理由:
- **編集ファイルが完全分離できる**: tester は全 `_test.go`、impl-api は全 production `*.go` を担当し、重複所有が発生しない。破壊的シグネチャ変更に伴う既存テストの機械的追従(`New`/`Reconstruct`/`Create` 呼び出し)も `_test.go` 側なので tester に一元化でき、impl-api が production を触る時点で test 側は既に新シグネチャ前提になっている。
- **impl 先行(後付け)を退けた理由**: 破壊的シグネチャ変更のため、impl-api が production の `New`/`Reconstruct`/`Create` を変えた瞬間、既存 `_test.go`(task_test / service_test / duplicate_checker_test / memory_test)が **コンパイル不能**になる。この復旧には `_test.go` の編集が必要で、それは tester の担当領域。結局どちらの順でも「test を新シグネチャへ追従させる」作業は不可避であり、後付けにしても中間の赤(コンパイル不能)は解消されない。ならば TDD 先行の方が (a) workflow.md の既定(tester → impl)に沿い、(b) 契約が固定済みで手戻りが無く、(c) ファイル所有が綺麗に割れる、の3点で優位。
- **許容するトレードオフ**: 手順 1 完了〜2 完了の間、module 全体が「production 未実装によるコンパイルエラー」で赤になる。これは TDD の red フェーズそのもので、契約を計画で固定しているため throwaway なやり直しは発生しない。checker(手順 4)は必ず 2 の完了後に実行する。

### テスト観点(要件との対応)

| 要件 | 観点 | レベル / ファイル |
|---|---|---|
| R1(集約が priority を保持) | `New(title, PriorityHigh)` の `Priority()` が high。`Reconstruct` で渡した priority が保持。clone(memory)往復で保持 | domain `task_test.go` / infra `task_repository_test.go` |
| R2(作成時指定・未指定は medium) | service: `Create(ctx, title, "")` → medium(**既定・境界**)、`Create(ctx, title, "high")` → high(**明示**)。route: body に priority 無し → medium、有り → 指定値 | service `task_service_test.go` / route `task_handler_test.go` |
| R3(変更・不変条件を壊さない) | domain: `ChangePriority` で値が変わり status 不変・`updatedAt` 更新。service: `ChangePriority` 正常。route: `POST /tasks/{id}/priority` 200 で priority 反映、未存在 id は 404 | domain / service / route |
| R4(DTO に priority・web と命名/enum 一致) | list / get / create / start / complete の全レスポンスに snake_case `priority` が乗る。作成 priority がそのまま往復。enum 文字列が `low`/`medium`/`high` | route `task_handler_test.go`(**入出力往復**) |
| R5(不正 enum を弾く) | domain: `ParsePriority("urgent")` / `ParsePriority("")` → `ErrInvalidPriority`(`errors.Is`)。service: `Create`/`ChangePriority` に不正 priority → ErrInvalidPriority 伝播。route: 不正 priority の作成 / `POST …/priority` → 400 | domain `priority_test.go` / service / route(**異常系**) |
| 非機能(状態遷移と直交) | priority を high にしても `Start`/`Complete` が従来どおり成功/失敗する(遷移表不変)。既存の遷移テストが全緑のまま | domain `task_test.go` / route `task_handler_test.go` |

- table-driven を基本形にする(`ParsePriority` は `ParseStatus` テストと同じ table 形式、境界に empty を必ず含める)。
- 既存の状態遷移テスト(`TestStatus_CanTransitionTo` 等)は挙動不変で緑を維持することを回帰として確認する。

## リスク / 未確定事項

- **priority 変更エンドポイントの method/path**(本計画では `POST /tasks/{id}/priority` に確定)。**SPEC-003(Go を契約の正とする)を後続前提とする調整の結果、当初検討した `PATCH …/priority` は撤回し、app/api 既存の action-style(`POST /tasks/{id}/start|complete`)へ統一した。** 退けた代替案と理由:
  - `PATCH /tasks/{id}/priority`(web の変更系 `PATCH /api/tasks/:id/status` のスタイルに合わせる): SPEC-003 が Go を契約の正として swag で OpenAPI 化するため、web の PATCH に寄せるのではなく **Go 側の既存規約(POST action-style)への一貫性を優先**すべきと判断し撤回。
  - `PATCH /tasks/{id}`(部分更新の汎用エンドポイント): 現状 title/status/priority を個別エンドポイントで扱う設計(Rename は未公開、status は start/complete)と非対称になり影響範囲が広がる。本 Spec のスコープ(priority のみ)に対し過剰。
  - 採用した `POST /tasks/{id}/priority` は start/complete と同じ `POST /tasks/{id}/{action}` 規約に揃う(相違は body を取る点のみ)。**app/web には現状 priority 変更の client/mock が無い**(status のみ)。web 側の priority 変更 UI/契約追加は本 Spec スコープ外(enrichment 撤去 / SPEC-003 の web 追従作業)で行うため、その時に web を **この `POST /tasks/{id}/priority`(Go 契約)へ合わせる** 前提を後続計画へ引き継ぐ。
- **ISSUE-008 との交差**: ISSUE-008 は `GET /tasks/{id}` の DTO 契約確定を含む。本 Spec で `taskResponse` に priority を追加すると、単一取得を含む全レスポンスに priority が乗るため契約は自動的に同期する。一方 ISSUE-008 の一覧ページネーション追加は **本 Spec では扱わない**(別作業)。着手順によっては `List`/`taskResponse` を双方が触るため、先行した側の変更を尊重してマージする。
- **app/web との最終整合**: web の zod スキーマ(`taskSchema.priority: z.enum(["low","medium","high"])`、snake_case、`createTaskRequestSchema` は priority 必須)は本計画の DTO と一致済み。ただし web の `createTaskRequestSchema` は priority を **必須** とする一方、本 API は **optional(未指定 medium)**。API を寛容(optional)側に倒すのは複数クライアント互換のため妥当だが、web が常に送る前提とのギャップは enrichment 撤去作業時に再確認する。
- **既定値のパース境界**: JSON で priority フィールドを完全省略した場合も、明示的に `""` を送った場合も、`createTaskRequest.Priority` は `""` になり同じく medium 既定となる(`*string` で「省略」と「空」を区別しない設計)。これは意図した挙動(未指定=medium)。`null` を明示送信した場合も Go の `string` へは `""`(実際には decode エラーにならず zero 値)相当となる点は tester が確認する。
- **SPEC-003 への引き継ぎ(順序依存)**: SPEC-003 は本 SPEC-002 を先行前提(drift D1)とし、SPEC-002 が確定させる DTO(`taskResponse` の priority 含む全フィールド)とエンドポイント(`POST /tasks/{id}/priority` 含む)を swag で OpenAPI 化する。したがって本計画の DTO は素直な形(`Priority string`・snake_case・余計な変換なし)を維持し、後で swag アノテーションを付けやすくしておく。SPEC-002 の完了(checker 緑 + レビュー通過)が SPEC-003 着手の解除条件になるため、着手順は SPEC-002 → SPEC-003 を厳守する。
