---
id: ISSUE-018
title: route のエラーハンドリングをカテゴリ型 + 型switch にリファクタ
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-10
specs: []  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-018: route のエラーハンドリングをカテゴリ型 + 型switch にリファクタ

種別: 課題 / リファクタ(技術的改善。バグではない)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/api の開発者(保守者)** の **エラーハンドリングの拡張安全性** が **sentinel を追加するたびに route の switch を手で編集する必要があり、分岐漏れ(default 500 への意図しない転落)を招きやすい構造になっている** ことで損なわれている。

- **影響を受けるユーザー**: 主に app/api の開発者・保守者(間接的にはエンドユーザー)。本 Issue はリファクタであり、現時点でエンドユーザー向けの API 挙動は成立している
- **損なわれる価値**: エラー分類の拡張安全性・保守性。エラーの「種類(バリデーション / 未検出 / 競合 / DB・インフラ障害)」が型として表現されていないため、新しい sentinel を足すたびに `route/response.go` の `writeError` を編集する必要があり、編集漏れが静かに 500 へ転落する
- **影響範囲・頻度**: 常時(構造的な課題)。ただし現行のエンドユーザー向けレスポンスは、下記の未処理 sentinel が現在の route からは到達不能なため、実害としては顕在化していない(「3. 原因」参照)
- **回避策**: 現状の実装でも API は機能しているため、エンドユーザー視点の回避策は不要。開発者視点では sentinel 追加のたびに switch を手当てすることで回避しているが、それ自体が本 Issue の対象

## 2. 現象(何が起きているか)

### 期待する動作

- service 層から返るエラーを **カテゴリ型(ValidationError / NotFoundError / ConflictError / DBError)** で表現し、`route/response.go` の `writeError` は `errors.As` による **型 switch** で HTTP レスポンスを分岐する
- 新しいエラー要因を追加しても、既存カテゴリのいずれかに属する限り route 側の switch を編集不要にする(分岐漏れによる意図しない 500 転落を構造的に防ぐ)
- DB・インフラ由来のエラーは DBError として型で区別でき、内部情報を client に漏らさず slog にのみ出力して 500 を返す

### 実際の動作

- `route/response.go` の `writeError` は、個々の sentinel を `errors.Is` / `errors.As` で **1つずつ列挙** して分岐している(`app/api/route/response.go:37-59`):
  - `task.ErrNotFound` → 404("task not found")
  - `task.ErrDuplicateTitle` → 409("task title already exists")
  - `*task.TransitionError` → 409(`transitionErr.Error()`)
  - `task.ErrEmptyTitle` / `task.ErrTitleTooLong` / `task.ErrInvalidID` / `task.ErrInvalidPriority` → 400
  - default → 500("internal server error"、slog に出力)
- エラーの「種類」が型として表現されていない。sentinel を追加するたびに route の switch を編集する必要があり、拡張時に分岐漏れが起きやすい
  - 具体例: `task.ErrInvalidStatus`(`app/api/domain/task/errors.go:33` で定義済み)は `writeError` の switch に列挙されておらず、到達すれば default の 500 に落ちる(`app/api/route/response.go:40-58` に該当 case なし)
- infra/postgres の not-found 以外の生 DB エラー(接続断・制約違反など)は、`fmt.Errorf("postgres: ...: %w", err)` で無名の wrapped error として route まで届き(例: `app/api/infra/postgres/task_repository.go:72,85,89,102,106,116` 等)、型として区別されないまま default 500 に落ちている

### 再現手順

現時点では「構造上の課題」であり、エンドユーザー操作で誤ったステータスコードを直接引き出す再現手順は確認できていない(下記「環境・条件」と「3. 原因」の調査結果を参照)。コード上の現象は以下で確認できる:

1. `app/api/route/response.go` を開き、`writeError` の `switch` が sentinel を1件ずつ列挙し、`task.ErrInvalidStatus` の case が存在しないことを確認する
2. `app/api/domain/task/errors.go:33` に `ErrInvalidStatus` が定義されていることを確認する
3. `app/api/infra/postgres/task_repository.go` で、not-found 以外の DB エラーが `fmt.Errorf("postgres: ...: %w", err)` の無名 wrap で返り、カテゴリ型を持たないことを確認する

### 環境・条件

- 対象: `app/api`(Go)。route(presentation)/ domain/task / infra/postgres
- 参照コミット: `af2e2b2`(Add Postgres persistence to app/api and app/auth (SPEC-005))時点の main 系列。ブランチ `feat/auth-oidc-foundation`

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `writeError` は sentinel を1件ずつ `errors.Is` / `errors.As` で列挙している(`app/api/route/response.go:40-58`)。エラーの種類を表すカテゴリ型は存在しない
- 事実: `ErrInvalidStatus` は `domain/task/errors.go:33` に定義されているが、`writeError` の switch に case がない。到達すれば default 500 になる
- 事実: `ErrInvalidStatus` を返す `task.ParseStatus`(`app/api/domain/task/status.go:47`)の呼び出し元は、`grep` の結果 `app/api/infra/postgres/task_repository.go:150`(DB 行のデコード時)のみ。route のハンドラ(`app/api/route/task_handler.go`)はステータス遷移を専用エンドポイント(`start` / `complete`)で行い、ユーザー入力のステータス文字列を `ParseStatus` に渡す経路が現状ない
  - したがって `ErrInvalidStatus` は「現状の route からはユーザー操作で到達不能」で、実際に発生し得るのは DB 行が不正値を持つケースの読み取り時に限られる。これは実質 DB データ起因であり、500 でも大きな誤りではない
  - 仮説: 将来ユーザー入力でステータスを受け付けるエンドポイントを追加した場合、この分岐漏れが 400 であるべき応答を 500 にする顕在バグへ転じる。現状は「潜在的な分岐漏れ」に留まる
- 事実: infra/postgres は not-found のみ `task.ErrNotFound` に、重複制約違反のみ `task.ErrDuplicateTitle` にマップし(`app/api/infra/postgres/task_repository.go:70,83,100`、`*pgconn.PgError` の判定は同ファイル `167` 付近)、それ以外の DB エラーは無名 wrap のまま返す。route はこれらを型で区別できず default 500 に集約している

### 根本原因

- エラーの「カテゴリ(バリデーション / 未検出 / 競合 / DB・インフラ障害)」がドメインの型として表現されておらず、HTTP へのマッピングを route が sentinel の列挙に依存していること。このため分岐の網羅性がコンパイル時にも構造的にも保証されず、拡張時に分岐漏れ(意図しない 500)を招く

## 4. 対応(どう解決するか)

### 対応方針

「カテゴリ型 + route 型switch へのリファクタ」を行う。**詳細な実装計画は [`docs/plans/ISSUE-018-plan.md`](../plans/ISSUE-018-plan.md) を参照**(方針 / 変更ファイル / 手順 / テスト戦略 / リスクは plan が正)。要点:

- service 層から返るエラーを **カテゴリ型(ValidationError / NotFoundError / ConflictError / DBError)** で表現し、`writeError` は `errors.As` による **型 switch** に置き換える
- 各カテゴリ型は client 向けメッセージ(`Msg`)や元エラー(`Err`)を保持する。DBError の内部情報は client に漏らさず slog にのみ出力して 500 を返す
- 後方互換: 既存の sentinel error は残し、カテゴリ型が `Unwrap()` で sentinel を包むことで `errors.Is(err, ErrXxx)` と `errors.As(err, &CategoryErr)` の双方を成立させる。既存の HTTP ステータス・レスポンス形状(`{"error": msg}`)・メッセージ文言は不変とする
- DDD のレイヤ制約を維持: カテゴリ型は domain 層(`domain/task`)に定義し、infra/postgres が生 DB エラーを DBError にラップする(`db.md` の「not-found は sentinel を返す」契約は維持)。route は `infra/pgconn` に依存しない
- `ErrInvalidStatus`(`ParseStatus` の唯一の生成元)はユーザー入力から到達せず `infra/postgres` の `taskFromRow`(DB 行復元)経由でのみ呼ばれるため、**`ValidationError`(400)ではなく `DBError`(500)に分類する**(当初案の「ValidationError カテゴリに含める / 400 に回収する」から最終実装で是正。詳細は下記 2026-07-10 の完了エントリを参照)。`taskFromRow` は内側エラーを `%v` で文字列化して Unwrap 連鎖から切断し、corrupt-row が 400 に化けないようにする。これにより「`ErrInvalidStatus` が switch の分岐漏れで default 500 に落ちる」という本 Issue の構造的課題は、**明示的な `DBError`→500 経路**として解消される(挙動としての 500 は変わらないが、型として明示化される)

スコープ / 対象外:

- 対象: `app/api` のみ
- 対象外: `app/auth`。OAuth のエンドポイント別セマンティクス(同一エラーでも `/authorize` と `/token` で error コードが変わる、redirect / WWW-Authenticate / no-store 制御)があり、単純な「型 → レスポンス」に収まらないため今回は対象外(将来別 Issue で検討)
- 文字列ベースの switch は採用しない(`.claude/rules/api.md` の「分岐したいエラーは sentinel / カスタム型 + `errors.Is` / `errors.As`」に反するため)

### 実施内容

- [x] planner が実装計画を `docs/plans/ISSUE-018-plan.md` に作成し、本 Issue の「対応方針」から参照する
- [x] カテゴリ型(ValidationError / NotFoundError / ConflictError / DBError)を `domain/task` に定義(`Unwrap()` で既存 sentinel を包む)
- [x] infra/postgres で not-found 以外の生 DB エラーを DBError にラップ(not-found は sentinel を維持)
- [x] `route/response.go` の `writeError` を `errors.As` の型 switch に置換(ステータス・レスポンス形状・文言は不変を維持、`ErrInvalidStatus` は `taskFromRow` の `%v` 切断により `DBError`→500 として明示化。当初案の「400 に回収」は最終実装で `DBError`→500 に是正)
- [x] テスト: 各カテゴリ → HTTP ステータス・レスポンス body の対応、`errors.Is` / `errors.As` の双方成立、DBError の内部情報非漏洩(body には出さず slog のみ)を検証

### 再発防止

- エラーの種類をカテゴリ型で表現することで、route の分岐が有限のカテゴリに閉じ、sentinel 追加時の switch 編集漏れによる意図しない 500 転落を構造的に防ぐ
- 型 switch の網羅性をテストで固定し、新カテゴリ追加時にテストで気付ける状態にする

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。app/api のエラーハンドリング(`route/response.go` の `writeError`)が sentinel を1件ずつ列挙する構造で、エラーの種類が型として表現されておらず拡張時に分岐漏れ(意図しない 500 転落)を招きやすい、という技術的課題としてリファクタを提起。
- コード確認: `writeError` の switch が sentinel 列挙であること(`app/api/route/response.go:40-58`)、`ErrInvalidStatus`(`domain/task/errors.go:33`)が switch 未収録で default 500 に落ちること、infra/postgres の not-found 以外の DB エラーが無名 wrap のまま route に届くこと(`app/api/infra/postgres/task_repository.go` 各所)を確認。
- 到達性の事実確認: `ErrInvalidStatus` を返す `ParseStatus` の呼び出し元は infra/postgres の DB 行デコードのみで、現状の route からはユーザー操作で到達不能。よって現時点では顕在バグではなく潜在的な分岐漏れ(構造的課題)と判断し、種別をリファクタ、severity を low とした。
- 関連 Spec: DBError カテゴリは SPEC-005(Postgres 永続化)で導入された infra/postgres の生 DB エラーが route に届く経路に関係するが、本 Issue は route のエラーマッピング全体を対象とする横断リファクタで特定 Spec の要件充足ではないため、frontmatter の `specs` は空のままとした(相互リンクの要否は要判断・下記「不明点」参照)。
- 次のアクション: planner が `docs/plans/ISSUE-018-plan.md` に実装計画を作成する。

### 2026-07-10(planner: 実装計画を作成)

- 実装計画を [`docs/plans/ISSUE-018-plan.md`](../plans/ISSUE-018-plan.md) に作成し、「対応方針」から参照するようにした。対象は app/api のみ。
- 現物調査で設計と実コードの整合を確認: `route/response.go` の sentinel 列挙 switch、`domain/task/errors.go` の既存 sentinel + `TransitionError(From/To)`、value object(`title`/`priority`/`id`/`status`)/ `service/task_service.go` / `infra/{memory,postgres}/task_repository.go`、および全既存テスト(`domain/task/*_test.go` / `route/task_handler_test.go` / `service/task_service_test.go` / `infra/**` / `infra/repotest`)を精査。既存の `errors.Is`/`errors.As` アサートと route のステータス+`{error}` 形状アサートは、カテゴリ型が `Unwrap()` で sentinel を包む設計で**無改修 green を維持できる**ことを確認した。
- 計画の骨子: (1) domain に 4 カテゴリ型(`ValidationError`/`NotFoundError`/`ConflictError`/`DBError`)+ コンストラクタを追加し各 sentinel を `Unwrap()` で公開、(2) `TransitionError` は型・`From`/`To` を残し `Unwrap()→&ConflictError` を追加、(3) value object / service / infra の producer を各カテゴリ型に置換(`ParseStatus` は無改修=`taskFromRow` で DBError に畳み込み 500・`ErrInvalidStatus` の分岐漏れを解消)、(4) `route/response.go` の `writeError` を `errors.As` 型 switch に置換(ステータス・body 文言・形状は不変)。`taskFromRow` は内側 ValidationError を `%v` で切断し corrupt-row の 400 化を防止。
- agent 割り振り: `domain/task/errors.go` の型定義を **impl-api が先行確定**(依存の起点)→ その後 impl-api 残作業(title/priority/id/service/route/infra-memory)と **impl-db**(infra/postgres)を並列。`infra/memory` は非 postgres 実装のため impl-api、`infra/postgres` は impl-db が担当。TDD は型スケルトン確定後に tester が red テストを追加 → producer 実装で green。checker=`make check`、review は security/performance/spec を並列。
- ステータスを `open` → `fixing` に更新(修正方針が確定し実装フェーズへ移行)。
- 未確定 / リスク(plan 詳細): `taskFromRow` の `%v` 切断徹底(corrupt-row の 400 回帰防止)、`DuplicateChecker` の `errors.Is(ErrNotFound)` 依存(`NotFoundError.Unwrap` で担保)、swagger `@Failure` はステータス/スキーマ不変で `make openapi` 差分なし(注釈編集不要)、`make test-integration` は稼働中 Postgres 前提でローカル skip 時は CI の `api-integration` job に委ねる。

### 2026-07-10(実装 / テスト / チェック / レビュー完了 → resolved)

- **実装完了**: impl-api が `domain/task` に 4 カテゴリ型(`ValidationError` / `NotFoundError` / `ConflictError` / `DBError`)を追加し、value object・service・route・`infra/memory` の producer を各カテゴリ型に置換。impl-db が `infra/postgres` の not-found 以外の生 DB エラーを `DBError` にラップし、`taskFromRow` は内側エラーを `%v` で文字列化して Unwrap 連鎖から切断(corrupt-row の 400 化を防止)。後方互換のため既存 sentinel は残し、各カテゴリ型が `Unwrap()` で sentinel を包むことで `errors.Is` と `errors.As` の双方が成立。HTTP ステータス・body 文言・`{"error": msg}` 形状は不変。
- **`ErrInvalidStatus` の分類を最終確定(当初案から是正)**: `ErrInvalidStatus`(`ParseStatus` の唯一の生成元)はユーザー入力から到達せず `infra/postgres` の `taskFromRow`(DB 行復元)経由でのみ呼ばれるため、**`ValidationError`(400)ではなく `DBError`(500)に分類**した。当初「対応方針」に記していた「ValidationError カテゴリに含める / 400 に回収」は最終実装と不一致のため、本更新で「対応方針」「実施内容」の該当記述を `DBError`→500 明示化に是正済み(過去の経緯エントリは編集していない)。本 Issue が指摘した構造的課題(`ErrInvalidStatus` が switch の分岐漏れで default 500 に落ちる)は、**明示的な `DBError`→500 経路**として解消された(挙動としての 500 は同じだが、型として明示化された)。この判断は review-spec の Minor 指摘に基づく。
- **テスト**: tester が `domain/task/errors_test.go`(新規)ほかを追加。`make test` / `make test-race` が全 green。既存テストは無改修で green(カテゴリ型の `Unwrap()` により `errors.Is` / route のステータス+`{error}` 形状アサートが維持され、後方互換を確認)。integration は使い捨て Postgres で実機 green を確認し、恒常検査は CI の `api-integration` job に委譲。
- **チェック**: checker の `make check`(fmt-check + lint + vet + build + test)が全通過。
- **レビュー(security / performance / spec を並列)**:
  - review-security = 指摘なし(内部情報の非漏洩・corrupt-row の 500 固定を確認)。Info 2 件のみ。
  - review-performance = Blocker / Major なし。Minor 1 件(`TransitionError.Unwrap()` がエラー応答時に `*ConflictError` を最大 3 回アロケートする点)。
  - review-spec = 要件充足。上記 `ErrInvalidStatus` の記述是正のみを Minor 指摘(本更新で反映済み)。
- **Minor 指摘のトリアージ(admin 判断)**:
  - performance Minor(`TransitionError.Unwrap()` のアロケーション)は **今回は対応せず受容**。理由: 不正な状態遷移というエラー系(cold path)専用で O(1)・スケール非連動であり、レビュアーも「対応必須ではない / 過剰最適化は不要」と評価。可読性のある switch 順序(404 → 400 → 409 → 500)を優先する。**備考**: 将来 profiling で問題化した場合のみ、switch 順序変更または `TransitionError` へのキャッシュフィールド追加で対応可能。
  - security Info(`taskFromRow` の `DBError` メッセージに行の生値=ユーザーのタスクタイトル等が `%v` で埋め込まれ slog に出力される点)は、client body には非漏洩で既存 slog 方針に従うため **受容**。ログ基盤側のアクセス制御・保持期間は組織方針に委ねる。
  - spec Minor(`ErrInvalidStatus` の記述是正)は本更新で反映済み。
- **結論**: 実装・テスト・チェック・レビューが完了し、修正を検証済み。残 Minor は上記のとおり受容(新規 Issue 起票は不要と判断)。status を `fixing` → **resolved** に更新。
