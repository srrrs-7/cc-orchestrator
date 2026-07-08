---
id: ISSUE-009
title: Task の状態遷移エンドポイント契約が app/web(PATCH /tasks/:id/status)と app/api(POST /tasks/{id}/start|complete)で乖離し、実 web↔api 結合時に状態遷移(Start/Complete)が機能しない(cross-stack)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-08
updated: 2026-07-08
specs: [SPEC-003]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-009: Task の状態遷移エンドポイント契約が app/web(PATCH /tasks/:id/status)と app/api(POST /tasks/{id}/start|complete)で乖離し、実 web↔api 結合時に状態遷移(Start/Complete)が機能しない(cross-stack)

## 1. ユーザー価値への影響(なぜ対応するか)

> **タスク管理の利用者(実 web↔api 結合後)** の **タスクの状態を進める操作(着手 / 完了)** が **web と api で状態遷移の HTTP 契約が一致しておらず、web の遷移リクエストに一致する api エンドポイントが存在しないため、結合後は機能しなくなることで損なわれる(結合条件下)**。

- **影響を受けるユーザー**: 将来 app/web を実 app/api に結合して使う利用者(および同じ状態遷移契約に依存する他クライアント)。**現時点では app/web は MSW モックで自己完結して動作しており、実 app/api には結合していない。**
- **損なわれる価値(結合後)**: タスクのライフサイクル操作(todo→doing の着手 / doing→done の完了)。タスク管理の中核操作であり、これが呼べないと表示以外の主要機能が使えない。
- **影響範囲・頻度**: **現時点では実害ゼロ。** web は MSW でモック済み・実 api と未結合のため、乖離は「実 web↔api 結合時」または「SPEC-003(OpenAPI 型共有)で Go を正として契約を生成した時点」にのみ、状態遷移が全面的に機能しない構造的乖離として顕在化する。
- **回避策**: 現状は不要(MSW が web 側契約を再現して両立させているだけ)。結合時は「契約をどちらかに寄せる」設計判断=修正そのものが必要で、回避策ではない。

## 2. 現象(何が起きているか)

本 Issue は app/web レビューおよび ISSUE-008 調査で「別課題」として記録された、Task 状態遷移エンドポイントの cross-stack 契約乖離である。SPEC-003(OpenAPI 型共有)の実装計画でも同一乖離が drift **D2** として特定されている。

### 期待する動作

web が状態遷移を要求したとき、その HTTP メソッド・パス・粒度・ボディが app/api の実装するエンドポイント契約と一致し、リクエストが api に到達して遷移が実行される。

### 実際の動作

web と api で状態遷移の HTTP 表現が異なり、web のリクエストに一致するエンドポイントが api に存在しない。

- **web(単一の汎用エンドポイント + body で目標状態を表現)**: `PATCH /tasks/:id/status` に body `{status}` を送る。
  - `app/web/src/features/tasks/api/client.ts:28-32` の `updateTaskStatus(id, status)` → `httpPatch(\`/tasks/${id}/status\`, { status })`
  - MSW: `app/web/src/mocks/handlers.ts:97-118` の `http.patch("/api/tasks/:id/status", ...)`。body を `updateTaskStatusRequestSchema`(`status` = todo|doing|done、`:14-16`)で検証、`canTransition`(todo→doing / doing→done、`:22-26`)を確認、不正遷移は 409、未存在は 404 を返す。
- **api(遷移種別ごとに別エンドポイント + body 無し)**: `POST /tasks/{id}/start`(着手)/ `POST /tasks/{id}/complete`(完了)。
  - `app/api/route/router.go:19-20` の登録 `POST /tasks/{id}/start` → `h.start` / `POST /tasks/{id}/complete` → `h.complete`
  - `app/api/route/task_handler.go:89-100` の `start` → `svc.Start(ctx, id)` / `:102-113` の `complete` → `svc.Complete(ctx, id)`。いずれも body を読まず path の `{id}` のみで遷移する。
  - api に `PATCH /tasks/{id}/status` は**存在しない**(router 登録は create / list / get / start / complete のみ)。
- **したがって乖離は次の3軸**:
  1. **HTTP メソッド**: web `PATCH` ↔ api `POST`
  2. **パス / 粒度**: web は 1 本の `/status`(目標状態を body で表現)↔ api は遷移ごとに `/start`・`/complete`(遷移種別を URL で表現、body 無し)
  3. **リクエストボディ**: web は `{status}` を送る ↔ api は body を取らない
- **状態値の語彙と許可遷移は両者一致**している(api: `app/api/domain/task/task.go` の `StatusTodo`/`StatusDoing`/`StatusDone`、`Start`=todo→doing・`Complete`=doing→done。web: todo/doing/done、canTransition も todo→doing / doing→done)。**乖離は「遷移の HTTP 表現」であって状態値そのものではない。**

### 再現手順

第三者がコード観察で確認できる(実行時の再現は実 web↔api 結合が必要で現状は不能。以下は契約不一致の構造の再現)。

1. **web の遷移契約**: `app/web/src/features/tasks/api/client.ts:27-32` が `PATCH /tasks/:id/status` + body `{status}` を送ることを確認。MSW `app/web/src/mocks/handlers.ts:97` が `http.patch("/api/tasks/:id/status", ...)` を提供していることを確認。
2. **api の遷移契約**: `app/api/route/router.go:16-20` の登録が `POST /tasks`・`GET /tasks`・`GET /tasks/{id}`・`POST /tasks/{id}/start`・`POST /tasks/{id}/complete` のみで、`PATCH /tasks/{id}/status` が無いことを確認。`app/api/route/task_handler.go:89-113` の `start`/`complete` が body を読まず path の `{id}` のみで遷移することを確認。
3. **突き合わせ**: web が送る `PATCH /api/tasks/:id/status {status}` に一致する api ハンドラが存在しない(メソッド・パスとも不一致)ことを確認。

### 環境・条件

- 対象 stack: app/web(TypeScript / React サンプル)と app/api(Go / DDD サンプル)の cross-stack。
- 現在 app/web は MSW モックで自己完結して動作しており、実 app/api に結合していない。乖離は結合時にのみ顕在化する。
- 発見文脈: app/web レビュー、および ISSUE-008 の調査で「別の cross-stack 契約乖離であり別課題」として記録された乖離(ISSUE-008 本文 `:61` / 経緯 `:99`)。SPEC-003(OpenAPI 型共有)の実装計画でも drift **D2** として同一乖離が特定されている(`docs/plans/SPEC-003-plan.md` の D2 行、推奨 Q1)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実(web): `features/tasks/api/client.ts:28-32` の `updateTaskStatus` は `PATCH /tasks/:id/status` + body `{status}`。MSW `mocks/handlers.ts:97-118` が同契約を再現(status enum 検証 `:14-16`、canTransition todo→doing / doing→done `:22-26`、404 / 409)。
- 事実(api): 遷移は `POST /tasks/{id}/start`・`POST /tasks/{id}/complete`(`route/router.go:19-20`、`route/task_handler.go:89-113`)で body 無し。`PATCH /tasks/{id}/status` は未実装。
- 事実: 状態語彙・許可遷移(todo→doing→done)は両者一致(`app/api/domain/task/task.go` の Start/Complete)。乖離は遷移の HTTP 表現(メソッド / パス / 粒度 / ボディ)に限定される。
- 事実: 現状 web は MSW モックで動作し実 api と未結合のため、実行時の実害はゼロ。MSW は「web が期待する契約」を再現しているだけで、「api の実契約」を検証してはいない。
- 事実(関連): SPEC-003 の実装計画は本乖離を drift **D2** として明記し、着手前ユーザー判断ゲート(D2 の解消方向を確定するまで T3/T4 着手不可)に据えている。推奨は **Q1「web を Go の start/complete に合わせる」**(`docs/plans/SPEC-003-plan.md`)。ただし SPEC-003 R1 は `PATCH /tasks/{id}/status`(= web 側契約)を名指しており Go 実コードと食い違うため、spec-owner による §3 再整合が申し送られている。
- 仮説: MSW が web 独自契約を「正しく」再現するため、web の CI / テストが緑でも api との不一致は検出されない(だから表面化していない)。SPEC-003 R6 のドリフト検査が導入されるまでは自動検出されない見込み。

### 根本原因

**現行のバグではない。** app/api / app/web はともにサンプル実装で、状態遷移を独立に設計した結果、遷移の HTTP 表現が食い違ったまま MSW で両立している。api は DDD ドメインの振る舞い(Start / Complete)を素直に遷移ごとのエンドポイントへ写像し、web は REST 的に単一サブリソース(status)の PATCH としてモデル化した。両者を接続する単一の契約(正)が存在しないことが根本原因。契約の正を1つに定める設計判断(SPEC-003 の D2 ゲート)がなされ、片側または両側を寄せない限り、実結合では解消しない。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: 現状(MSW モック・実 api と未結合)では実害ゼロ。**今回のスコープでは即時対応必須ではなく**、実 web↔api 結合 / SPEC-003 着手時に解消すべき構造的乖離として記録・追跡する。着手時は planner + spec-owner が方針を確定し、impl-web / impl-api / tester / checker / review-spec のパイプラインで実施する。
- 以下はいずれも**提案(候補)**であり確定仕様ではない。契約を1つに統一する設計判断が前提:
  - **(a) web を app/api の `POST /tasks/{id}/start|complete` に合わせる**(SPEC-003 計画の推奨 Q1)。web の `updateTaskStatus` を start / complete の2コール、または「次状態 → 対応エンドポイント」の薄いディスパッチへ移行する(domain の `startTask`/`completeTask` で遷移種別は既に判別可能)。B2(Go を契約の正)方針と整合し、ISSUE-008 が別課題とした乖離もこれで解消する。
  - **(b) app/api に `PATCH /tasks/{id}/status` を新設して web を維持する**。Go の業務 API 追加であり、SPEC-003(型共有基盤)のスコープを超えるため、採るなら別途 api 側の設計と SPEC-003 R1 の位置づけ整理が必要。
  - **(c) 統一的な遷移 API を再設計する**(両 stack を新契約へ寄せる)。
- いずれも web↔api 両 stack にまたがるため、結合 / SPEC-003 着手時に planner が方針確定する。
- 仮説: (a) が最小変更かつ B2 方針整合で有力。ただし SPEC-003 R1 の `PATCH` 記述との整合は spec-owner の §3 再整合が必要(要確認)。

### 実施内容

- [ ] SPEC-003 の D2 ゲートで契約統一方針(a / b / c)を確定する(planner + spec-owner + ユーザー判断)
- [ ] 確定方針に沿って web / api の遷移契約を一致させる(impl-web / impl-api)
- [ ] 遷移契約のテストを両 stack で整合させる(tester。api: start/complete または新 PATCH の正常 / 異常 / 遷移境界。web: 遷移 mutation の成功 / 不正遷移 / 未存在)
- [ ] SPEC-003 R6 のドリフト検査で再発を機械的に検出できる状態にする

### 再発防止

- cross-stack の状態遷移 / エンドポイント契約(メソッド・パス・粒度・ボディ)は、web の client / mock と api の route を突き合わせてレビュー観点にする。
- MSW は「api の実契約」ではなく「web が期待する契約」を再現するに過ぎない前提を明示し、契約の正を1つ(SPEC-003 の OpenAPI + R6 ドリフト検査)に集約して、モックがそこから外れないようにする。

## 5. 経緯(時系列・追記のみ)

### 2026-07-08

- 起票。app/web レビューおよび ISSUE-008 調査で「別課題」として記録された、Task 状態遷移エンドポイントの cross-stack 契約乖離を独立 Issue として起票した。
- 事実確認(web): `features/tasks/api/client.ts:28-32` は `PATCH /tasks/:id/status` + body `{status}`。MSW `mocks/handlers.ts:97-118` が同契約を再現(status enum・canTransition todo→doing / doing→done・404 / 409)。
- 事実確認(api): 遷移は `POST /tasks/{id}/start`・`POST /tasks/{id}/complete`(`route/router.go:19-20`、`route/task_handler.go:89-113`、body 無し)。`PATCH /tasks/{id}/status` は未実装。状態語彙・許可遷移(todo→doing→done)は両者一致(`domain/task/task.go` の Start/Complete)で、乖離は遷移の HTTP 表現(メソッド / パス / 粒度 / ボディ)に限定。
- 現状影響: 実害ゼロ。app/web は MSW モックで自己完結し実 app/api と未結合のため、乖離は結合時、または SPEC-003 で Go を正に契約生成した時点で顕在化する。
- severity は **medium** と判定。判定根拠: 現在の実行時影響はゼロ(MSW でマスク)である一方、ISSUE-008(perf の緩やかな劣化 = low)と異なり、本件は結合時に状態遷移(タスク管理の中核操作)が**全面的に機能しなくなる hard break** であり、かつ **approved 済みの SPEC-003** の実装計画で drift D2 として「解消方向を確定するまで T3/T4 着手不可」の能動的ゲートになっている。純粋な現行ランタイム基準なら low 相当だが、結合時の重大度(中核機能の破綻)と approved Spec を実際にブロックしている点を踏まえ medium とし、SPEC-003 スコープ内では blocker 級として扱う。実結合が具体化した時点で high への再評価を検討する。
- 相互リンク: frontmatter `specs` に **SPEC-003** を追加し、SPEC-003 側 `issues` にも本 Issue を追記した。判断根拠: SPEC-003(OpenAPI 型共有基盤)の実装計画が本乖離を drift D2 として特定し、生成契約の整合方向を確定する着手前ゲートに据えている。本 Issue はその D2 を追跡・可視化する記録であり、直接の相互参照が有益。
- SPEC-002 は frontmatter リンクを**見送った**。判断根拠: SPEC-002(priority)は SPEC-003 の drift D1 に相当し、本 Issue の D2(状態遷移)とは**直交**(SPEC-002 自身が「priority は状態遷移と直交」と明記)。両者の調整点は umbrella の SPEC-003 側で既に D1 / D2 として束ねられており、SPEC-002 への直接リンクは弱い辺になるため本文参照に留めた。
- 関連: ISSUE-008(同じ web↔api Task 契約整備の領域。本乖離を「別課題」として記録済み)を本文で相互参照した。
- 次にやること: SPEC-003 の D2 ゲート判断(planner + spec-owner + ユーザー)で契約統一方針を確定し、確定後に impl-web / impl-api / tester のパイプラインで両 stack の遷移契約を一致させる。
</content>
</invoke>
