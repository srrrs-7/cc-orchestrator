# ISSUE-018 実装計画: route のエラーハンドリングをカテゴリ型 + 型switch にリファクタ

- 起点 Issue: `docs/issues/20260710-018-route-error-category-types.md`(種別: リファクタ / severity: low)
- 対象スタック: **app/api のみ**(`app/auth` は対象外)
- 関連ルール: `.claude/rules/api.md`(domain は他層非依存 / 分岐は sentinel・カスタム型 + `errors.Is`/`errors.As`)、`.claude/rules/db.md`(DB 詳細は infra/postgres に閉じ込め・not-found は domain sentinel に変換・memory↔postgres の振る舞い一致)、`.claude/rules/testing.md`(table-driven / 正常・異常・境界)

## 方針

route の `writeError` が sentinel を 1 件ずつ `errors.Is`/`errors.As` で列挙している構造を、**エラーの「カテゴリ型」+ route 側の型 switch** に置き換える。エラーの種類(バリデーション / 未検出 / 競合 / DB・インフラ障害)を domain 層の型で表現し、HTTP マッピングを有限のカテゴリに閉じることで、sentinel 追加時の switch 編集漏れ(意図しない 500 転落。既知の `ErrInvalidStatus` 未収録もこれで解消)を構造的に防ぐ。

### 採用するアプローチ

1. **domain に 4 カテゴリ型を新設**(`domain/task/errors.go`)。各型は該当 sentinel を `Err` に保持し `Unwrap()` で公開する。これが後方互換の要:
   - `ValidationError{Msg, Err}` → HTTP 400。`Error()` は `Msg`(+ `Err`)、`Unwrap()` は `Err`。
   - `NotFoundError{}` → HTTP 404。マーカー型。`Error()`＝`ErrNotFound.Error()`、`Unwrap()`＝`ErrNotFound`。
   - `ConflictError{Msg, Err}` → HTTP 409。`Error()` は `Msg`(+ `Err`)、`Unwrap()` は `Err`(`Err` は nil 可)。
   - `DBError{Err}` → HTTP 500。`Error()`＝`"task: database error: <Err>"`、`Unwrap()`＝`Err`。内部情報は client に漏らさず slog にのみ出す。
   - 型はすべて **ポインタレシーバ**で `Error()`/`Unwrap()` を実装し、producer は `&Category{...}`(コンストラクタは `*Category` を返す)で生成する。route の `errors.As` ターゲット(`*task.ValidationError` 等)と一致させるため。
2. **producer 側で生の sentinel をカテゴリ型に置換**(value object / service / infra)。各生成箇所で該当カテゴリ型に該当 sentinel を包む。
3. **route の `writeError` を型 switch に置換**(`route/response.go`)。`errors.As` で 4 カテゴリを判定。レスポンス形状(`{"error": msg}`)・ステータス・文言は不変。
4. **TransitionError は型・`From`/`To` を残す**。新たに `Unwrap() error { return &ConflictError{Msg: e.Error()} }` を追加し、route の ConflictError case が遷移エラーを 409 として拾う。route は TransitionError を列挙不要になる。`transitionTo`(task.go)は無改修。

### 後方互換が保てる根拠(調査で検証済み)

- 各カテゴリ型が `Unwrap()` で sentinel を返すため、既存の `errors.Is(err, ErrXxx)` アサート(value object / service / repotest / memory / postgres integration の全テスト)がそのまま green を維持する。
- route のステータス + `{error}` 形状アサート(`route/task_handler_test.go` の behavior/wire 両テスト群)は、各カテゴリ型の `Msg` を現行 route の body 文言(`"title must not be empty"` / `"title is too long"` / `"invalid task id"` / `"invalid priority"` / `"task not found"` / `"task title already exists"`)・TransitionError の `Error()` 文字列に一致させることで 400/404/409/500 とも不変に保つ。
- `service.DuplicateChecker.IsDuplicated` は `errors.Is(err, ErrNotFound)` に依存(`domain/task/service.go:31`)。`NotFoundError.Unwrap()==ErrNotFound` で担保される。
- `TransitionError` を参照する既存テスト(`task_test.go` / `service/task_service_test.go` の `errors.As(&transitionErr)` と `From`/`To` 参照)は、型と両フィールドを残すため無改修。

### 退けた代替案

- **文字列ベース switch**: `.claude/rules/api.md` の「分岐は sentinel / カスタム型 + `errors.Is`/`errors.As`」に反する。不採用。
- **既存 sentinel を全廃してカテゴリ型のみにする**: 既存テストの `errors.Is(err, ErrXxx)` が全滅し、value object の API 破壊。sentinel を残しカテゴリ型で包む(Unwrap 公開)方が影響最小。不採用。
- **`ParseStatus` を 400 化(ValidationError 付与)**: `ParseStatus` はユーザー入力から到達せず `taskFromRow`(行復元)経由でのみ呼ばれる(Issue の到達性調査で確認)。400 化すると corrupt-row が 400 に化ける。無改修のまま taskFromRow 側で DBError に畳み込み 500 とする。不採用。
- **route を `service`/`infra` に依存させて型判定**: route が `infra`/`pgconn` に依存し DDD レイヤ違反。カテゴリ型を domain に置くことで route の import を `domain/task` + `service` のみに保つ。不採用。

## 変更ファイル(app/api のみ)

### impl-api 担当(domain / service / route / infra-memory)

| ファイル | 変更内容 |
|---|---|
| `domain/task/errors.go` | 4 カテゴリ型(`ValidationError` / `NotFoundError` / `ConflictError` / `DBError`)+ コンストラクタ(`NewValidationError` / `NewConflictError` / `NewDBError` / `NewNotFoundError` / `NewDuplicateTitleError`)を追加。`TransitionError` に `Unwrap() error { return &ConflictError{Msg: e.Error()} }` を追加(型・`From`/`To`・`Error()` は不変)。既存 sentinel 群は全て残す |
| `domain/task/title.go` | `NewTitle`: empty → `&ValidationError{Msg:"title must not be empty", Err:ErrEmptyTitle}` / too long → `&ValidationError{Msg:"title is too long", Err:ErrTitleTooLong}` |
| `domain/task/priority.go` | `ParsePriority`: 不正値 → `&ValidationError{Msg:"invalid priority", Err:ErrInvalidPriority}` |
| `domain/task/id.go` | `ParseID`: empty → `&ValidationError{Msg:"invalid task id", Err:ErrInvalidID}` |
| `domain/task/service.go` | 無改修(`IsDuplicated` の `errors.Is(err, ErrNotFound)` は NotFoundError.Unwrap で担保) |
| `service/task_service.go` | `Create` の重複判定 `%w` を `task.NewDuplicateTitleError()` に差し替え(他の `fmt.Errorf("...: %w", err)` 行は無改修=カテゴリ透過) |
| `route/response.go` | `writeError` を `errors.As` の型 switch に置換。`errorResponse` / `writeJSON` / `writeBadRequest` は不変。import は `domain/task` のみ(現状維持) |
| `infra/memory/task_repository.go` | not-found(FindByID/FindByTitle)→ `&task.NotFoundError{}`。`ctx.Done()` の 4 箇所(Save/FindByID/FindByTitle/FindAll)→ `task.NewDBError(ctx.Err())` |

`domain/task/status.go`(`ParseStatus`)は**無改修**(理由は方針の「退けた代替案」参照)。

### impl-db 担当(infra/postgres)

| ファイル | 変更内容 |
|---|---|
| `infra/postgres/task_repository.go` | not-found(`sql.ErrNoRows`, FindByID/FindByTitle)→ `&task.NotFoundError{}` を `%w` で wrap。unique violation(`isUniqueViolation`, Save)→ `task.NewDuplicateTitleError()` を `%w` で wrap。その他 driver エラー(Save/FindByID/FindByTitle の else・FindAll のクエリエラー)→ `task.NewDBError(err)` を `%w` で wrap。`taskFromRow` の行デコード失敗(ParseID/NewTitle/ParseStatus/ParsePriority)→ `task.NewDBError(fmt.Errorf("decode task row: %v", err))`。**内側エラーは `%v` で文字列化(`%w` を使わない)** し Unwrap 連鎖から切り離す |

`taskFromRow` の `%v` 切断が要点: corrupt-row の内側 ValidationError を連鎖から外すことで `errors.As(&validationErr)` が false・`errors.As(&dbErr)` が true になり、DB 行不正が 400 に化けず必ず 500 になる。`ErrInvalidStatus` もこの経路で明示的に DBError→500 へ落ちる。

### テスト(tester 担当。詳細は「テスト戦略」)

| ファイル | 変更内容 |
|---|---|
| `domain/task/errors_test.go`(新規) | 4 カテゴリ型の `errors.As`/`errors.Is` 双方成立・非成立、TransitionError の二重 As |
| `domain/task/title_test.go` / `priority_test.go` / `id_test.go`(追記) | 既存 `errors.Is` に加え `errors.As(&ValidationError{})` true と `Msg` 一致を追加 |
| `route/task_handler_test.go`(追記) | カテゴリ→ステータスを直接駆動する fake Repository 経由のケース(特に `DBError`→500 を default 経路の 500 と型カテゴリとして区別)|
| `infra/memory/task_repository_test.go`(追記) | cancel(`context.Canceled`)→ `DBError` に As 可能を検証 |
| `infra/postgres/task_repository_integration_test.go`(追記 / `integration` タグ) | duplicate→`ConflictError`(既存の `ErrDuplicateTitle` As に加え)、強制クエリ失敗 / corrupt-row → `DBError` かつ `ValidationError` に As されないこと |

## 手順(agent 割り振り・依存順序・並列可否)

依存の起点は `domain/task/errors.go` のカテゴリ型定義。全 producer とテストがこれに依存するため、まず型スケルトンを確定させてから並列展開する。Go はカテゴリ型が未定義だと参照するテストがコンパイル不能なので、TDD の red は「型スケルトン確定後」に置く(下記 P0→P1)。

### P0(直列・先行): impl-api が errors.go のカテゴリ型を確定

- **impl-api**: `domain/task/errors.go` に 4 カテゴリ型 + コンストラクタ + `TransitionError.Unwrap()` を実装(振る舞いの本体)。この時点でパッケージがコンパイル可能になり、他 producer / テストが型を参照できる。
- 完了報告をもって P1 を開始する(型シグネチャがフリーズする)。

### P1(並列): TDD の red → producer 実装

P0 完了後、以下を並列で進める。tester は producer 実装前に失敗テストを置く(TDD)。

- **tester**(P0 直後に着手可): 追加テスト(「テスト戦略」の全項目)を書く。カテゴリ型は P0 で定義済みなのでコンパイルは通り、producer 未改修のため**カテゴリ As 系の新規アサートが red** になる(既存 `errors.Is` 系は既に green のまま)。
- **impl-api**(tester と並列可): `title.go` / `priority.go` / `id.go` / `service/task_service.go` / `route/response.go` / `infra/memory/task_repository.go` を実装。
- **impl-db**(tester・impl-api と並列可): `infra/postgres/task_repository.go` を実装。`task.NewDBError` / `&task.NotFoundError{}` / `task.NewDuplicateTitleError` に依存(P0 で確定済み)。

impl-api と impl-db は scope(パッケージ)が独立しているため並列で問題ない(共有するのは P0 で固定済みの `domain/task` の型シグネチャのみ)。

### P2(直列): テスト実行 → チェック → レビュー

1. **tester**: `cd app/api && make test`(+ 必要に応じ `make test-race`)。integration 分は `make test-integration`(postgres 前提。ローカルで DB が無い場合は CI の `api-integration` job に委ね、その旨を報告)。全 green を確認し、不足テストを補う。
2. **checker**: `cd app/api && make check`(fmt-check + lint + vet + build + test)。
3. **review**(checker 通過後に並列): review-security / review-performance / review-spec。
   - review-spec: Issue の期待動作(カテゴリ型表現・型 switch・`ErrInvalidStatus` 回収・DBError の内部情報非漏洩)と各カテゴリの HTTP マッピング不変性を照合。
   - review-security: DBError body に内部情報が出ないこと(slog のみ)を確認。
   - review-performance: エラーパスのみの変更で hot path 影響が無いことを確認。
4. **指摘対応**: Blocker / Major は impl-api / impl-db に差し戻し、P2 の 1→3 を再実行。今回対応しない指摘は issue-creator が別 Issue に起票。

## テスト戦略

TDD(P0 で型を先置き → P1 で red テスト → producer 実装で green)。観点は 正常系 / 異常系 / 境界値 を最低限カバー(table-driven)。

### 無改修で green を維持(検証済み・回帰防止の土台)

- value object: `title_test.go` / `priority_test.go` / `id_test.go` / `status_test.go` の `errors.Is(err, ErrXxx)`。
- service: `task_service_test.go` の `errors.Is`(ErrDuplicateTitle / ErrInvalidPriority / ErrNotFound / ErrInvalidID / ErrEmptyTitle)と `errors.As(&transitionErr)`。
- domain: `task_test.go` の `errors.As(&transitionErr)` + `From`/`To` 参照、`duplicate_checker_test.go`。
- repotest / infra: `repotest/task_contract.go`(`errors.Is(...ErrNotFound)`)、memory テスト、postgres integration(`errors.Is(...ErrDuplicateTitle)`)。
- route: `task_handler_test.go` の behavior 群 + wire-contract 群(400/404/409/500 のステータス・`{error}` 形状・all-string success shape)。**ステータス・body 文言・形状は全カテゴリで不変**。

### 追加テスト(Issue 期待動作との対応)

- **カテゴリ型の同定**(`domain/task/errors_test.go`・新規): 各型で「`errors.As(&Category{})` true」かつ「対応 sentinel の `errors.Is` true」。相互排他: `DBError` は `ValidationError` に As されない。`TransitionError` は `*TransitionError` と `*ConflictError` の**双方**に As 可能で、`From`/`To` を参照できる。→ Issue「カテゴリ型で表現」「型 switch の網羅性を固定」。
- **value object のカテゴリ付与**(title/priority/id テストに追記): 既存 `errors.Is` に `errors.As(&ValidationError{})` true と `Msg` 一致を追加。→ Issue「ValidationError カテゴリ」。
- **route のカテゴリ→ステータス直接駆動**(`route/task_handler_test.go` に追記): カテゴリ型を返す fake Repository を通し、
  - `NotFoundError`→404 / `ValidationError`→400 / `ConflictError`→409 は既存の実リポジトリ経路で既にカバー済み(重複・遷移・空title 等)。
  - **`DBError`→500 を専用ケースで固定**: fake Repository が `task.NewDBError(...)` を返す経路で 500 + `{error}` を検証し、「DBError カテゴリ = 500」を default 経路の 500 とは独立に pin する(誤って 400 等へ回した場合に検知)。body に内部情報が出ないこと(`{error:"internal server error"}` のみ)も確認。→ Issue「DBError を型で区別・内部情報非漏洩」「分岐漏れの 500 転落防止」。
- **infra/memory の cancel→DBError**(`memory` テストに追記): キャンセル済み context で `context.Canceled` を `DBError` に As 可能なことを検証(postgres とカテゴリ挙動を一致=`db.md` の memory↔postgres 挙動一致要件)。
- **infra/postgres(integration)**: duplicate→`ConflictError`(既存 `ErrDuplicateTitle` As に追加)、強制クエリ失敗 / corrupt-row → `DBError` かつ `ValidationError` に As されないこと(`taskFromRow` の `%v` 切断=corrupt-row が 400 に化けない回帰テスト)。→ Issue「生 DB エラーを DBError に」「`ErrInvalidStatus` 回収」。

### テストレベルの分担

- 単体(domain): カテゴリ型の Is/As 相互関係、value object の付与。
- 単体(service): カテゴリ透過(既存 Is 系)。
- 結合(route, in-process handler + fake/memory repo): カテゴリ→HTTP ステータス・body 形状。
- 統合(postgres, `integration` タグ): driver エラー / unique violation / corrupt-row のカテゴリ写像(実 DB 前提。CI job で担保)。

## リスク / 未確定事項

- **corrupt-row の 400 回帰リスク**: `taskFromRow` が内側 ValidationError を `%w` で連鎖に残すと `errors.As(&validationErr)` が true になり 400 に化ける。**`%v` で文字列化して切断**することで防止(integration テストで「`ValidationError` に As されない」を明示検証)。impl-db は必ず `%v` を使うこと。
- **`DuplicateChecker` の Is 依存**: `IsDuplicated` は `errors.Is(err, ErrNotFound)` に依存。`NotFoundError.Unwrap()==ErrNotFound` で担保。カテゴリ型実装時に `Unwrap()` の返り値を取り違えると重複検出が壊れる(既存の memory/postgres/service テストが検知する)。
- **swagger `@Failure` 注釈**: ステータス / スキーマ(`errorResponse`)は不変のため `make openapi` の再生成差分は無い想定。注釈編集・再生成は不要。念のため checker/tester は既存 `docs/openapi.yaml` に触れない(契約 drift の CI は Go DTO 変更が無いため発火しない)。
- **infra/memory を DBError で包む判断**: memory の cancel を 500 相当の `DBError` に統一するのは `db.md` の「memory↔postgres の振る舞い一致」に基づく型統一(両者とも観測上 500 で同値だが型を揃える)。実挙動の後方非互換は無い(既存テストは cancel 経路をアサートしていない)。
- **infra/memory の担当**: `infra/memory` は非 postgres の infra 実装で `db.md` の paths(`infra/postgres/**`)外のため **impl-api** が担当する(impl-db は infra/postgres に限定)。この切り分けを手順で明示済み。
- **route テストで DBError→500 と default→500 の出力が同一**: 両者とも `{error:"internal server error"}` を返すため、レスポンスだけでは経路を区別できない。テストの狙いは「DBError カテゴリが 500 にマップされる」ことの pin(将来 400 等へ誤配線した場合の検知)であり、出力同値は許容する。
- **統合テストのローカル実行可否**: `make test-integration` は稼働中 Postgres(`make migrate-up` 済み)を要する。ローカルに DB が無い環境では skip され、実検証は CI の `api-integration` job(`.github/workflows/cicd.yml`)に委ねられる。tester はローカル skip 時にその旨を報告し、default `make test` の green を完了条件とする。
