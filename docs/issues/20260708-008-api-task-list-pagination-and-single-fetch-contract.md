---
id: ISSUE-008
title: app/api の Task 一覧にページネーションが無く、web に単一取得(GET /tasks/{id})契約が無いため、本番データ規模で全件転送・線形探索がタスク総数に比例して悪化する(cross-stack)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-08
updated: 2026-07-10
specs: [SPEC-002, SPEC-008]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-008: app/api の Task 一覧にページネーションが無く、web に単一取得(GET /tasks/{id})契約が無いため、本番データ規模で全件転送・線形探索がタスク総数に比例して悪化する(cross-stack)

## 1. ユーザー価値への影響(なぜ対応するか)

> **タスク管理の利用者(特にタスク件数が多い将来の本番データ規模)** の **一覧・タスク詳細表示の応答性能** が **一覧が全件無制限に返り、web の詳細表示が全件取得 + 線形探索でタスクを引くため、タスク総数 n に比例して転送・パース・検証コストが悪化することで損なわれる(将来条件下)**。

- **影響を受けるユーザー**: app/web(および将来の他クライアント)経由でタスク一覧・タスク詳細を閲覧する利用者。とりわけタスク件数が増えた本番データ規模の利用者
- **損なわれる価値(将来条件下)**: 一覧取得およびタスク詳細表示のレイテンシと転送量。詳細1件を見るだけでも全タスクを取得・検証する
- **影響範囲・頻度**: **現時点では実害ゼロ。** web のモックデータは4件のサンプル規模で、パフォーマンスレビューでも「サンプル規模では概ね妥当」と評価済み。タスク件数が本番規模に増えたときにのみ、一覧・詳細のたびの O(n) コストとして顕在化する構造的課題
- **回避策**: 部分的にあり。web 側で単一取得 hook(`fetchTaskById` + queryKey `["tasks", id]`)を導入すれば詳細ページの全件取得は緩和できるが、**根本(一覧の無制限返却・単一取得エンドポイントの契約整備)は app/api 側の変更が必要**で web 単独では解消しない

## 2. 現象(何が起きているか)

本 Issue は app/web のフロントエンドサンプル実装のパフォーマンスレビュー指摘 M1(Major)を起点とする、app/api 側の構造的ギャップ(cross-stack)である。以下の2点。

### 期待する動作

- **(A) 単一タスク取得**: web がタスク詳細1件を表示する際、単一取得(`GET /tasks/{id}`)で当該タスクだけを取得する。web の mock / client / schema と app/api のエンドポイント契約(パス・レスポンス形状)が一致している
- **(B) 一覧のページネーション**: 一覧取得(`GET /tasks`)が limit/offset(または cursor)で返却件数を制御でき、返却件数が無制限にならない

### 実際の動作

- **(A-1) web に単一取得の契約が無い**: web の client(`app/web/src/features/tasks/api/client.ts:7-11`)は `fetchTasks`(`GET /tasks` 全件)のみで、単一取得関数(例: `fetchTaskById`)が無い。mock(`app/web/src/mocks/handlers.ts:65-68`)も一覧 `http.get("/api/tasks")` だけで、`GET /api/tasks/:id` のハンドラが無い
- **(A-2) 詳細ページが全件取得 + 線形探索**: `TaskDetailPage`(`app/web/src/app/router.tsx:37-65`)は一覧 hook `useTasksQuery()` で全件を取得し、`(data ?? []).find((candidate) => candidate.id === taskId)`(`:53`)で `Array.find` の線形探索(O(n))によって1件を引く
- **(A-3) app/api 側は単一取得が既に実在**: app/api には `GET /tasks/{id}` のハンドラが存在する(`app/api/route/task_handler.go:76-87` の `get` → `service.TaskService.Get` `app/api/service/task_service.go:48-61`)。レスポンス形状は `taskResponse`(`app/api/route/task_handler.go:22-38`)で `{id, title, status, created_at, updated_at}`(snake_case)。**すなわち契約不一致の実態は「app/api 側にはあるが web 側に無い」であり、両者を整合させる必要がある**
- **(B-1) 一覧が無制限返却**: `app/api/service/task_service.go:63-75` の `List(ctx)` は `s.repo.FindAll(ctx)` を呼んで全件を DTO 化して返す。limit/offset 等の引数を持たない。`app/api/route/task_handler.go:60-74` の `list` ハンドラもクエリパラメータ(limit/offset)を一切パースせず全件を返す

### 再現手順

第三者が以下を確認できる(コード観察による構造の再現)。

1. **単一取得契約の欠落(web)**: `app/web/src/features/tasks/api/client.ts` を開き、`fetchTasks`(`:7-11`)/ `createTask`(`:14-18`)/ `updateTaskStatus`(`:21-25`)のみで単一取得関数が無いことを確認する。`app/web/src/mocks/handlers.ts` を開き、`handlers` 配列(`:65-110`)に `GET /api/tasks/:id` が無い(list `:66-68` / POST `:70-86` / PATCH status `:88-109` のみ)ことを確認する
2. **詳細ページの線形探索(web)**: `app/web/src/app/router.tsx:37-65` の `TaskDetailPage` が `useTasksQuery()`(全件)を使い、`:53` で `.find((candidate) => candidate.id === taskId)` により線形探索していることを確認する
3. **単一取得の実在(app/api)**: `app/api/route/task_handler.go:76-87` の `get`(`GET /tasks/{id}`)と `app/api/service/task_service.go:48-61` の `Get` が存在し、レスポンス形状が `taskResponse`(`app/api/route/task_handler.go:22-38`)であることを確認する
4. **一覧の無制限返却(app/api)**: `app/api/service/task_service.go:63-75` の `List` が `FindAll` で全件返却し limit/offset を持たないこと、`app/api/route/task_handler.go:60-74` の `list` がクエリパラメータをパースしないことを確認する

### 環境・条件

- 対象 stack: app/api(Go / DDD サンプル)と app/web(TypeScript / React サンプル)の cross-stack
- 発見文脈: app/web のフロントエンドサンプル実装のパフォーマンスレビュー(review-performance)で挙がった Major 指摘 M1。「詳細1件の表示に全件取得 + 線形探索を用いており、web 側の単一取得 hook 化で緩和できるが根本は app/api の契約・ページネーション不在」との観察

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: app/api の `List` は `FindAll` で全件を返し、ページネーション引数を持たない(`app/api/service/task_service.go:63-75`)。route の `list` もクエリパラメータをパースしない(`app/api/route/task_handler.go:60-74`)。
- 事実: app/api の単一取得 `GET /tasks/{id}` は**実在する**(`app/api/route/task_handler.go:76-87`、`app/api/service/task_service.go:48-61`)。レスポンス形状は `{id, title, status, created_at, updated_at}` の snake_case(`app/api/route/task_handler.go:22-38`)。
- 事実: web の client / mock には単一取得の契約が無い(`app/web/src/features/tasks/api/client.ts:7-11`、`app/web/src/mocks/handlers.ts:65-68`)。詳細ページは一覧取得 + `Array.find` 線形探索で1件を引く(`app/web/src/app/router.tsx:39,53`)。
- 事実: 現状 web のモックデータは4件(`app/web/src/mocks/handlers.ts:28-61`)。サンプル規模のため全件転送・線形探索でも実害はない。
- 事実(スコープ外の観察): app/api の `taskResponse`(`app/api/route/task_handler.go:22-38`)には `priority` が無いが、web の `TaskDto` / mock は `priority` を持つ(`app/web/src/mocks/handlers.ts:34,41,49,57`)。これは SPEC-002(Task に優先度を追加)の対象であり、単一取得エンドポイントのレスポンス形状を確定する際は SPEC-002 の priority 反映と同期させる必要がある(本 Issue の直接対象ではないが、DTO 契約整備で交差する)。
- 事実(スコープ外の観察): web はステータス遷移を `PATCH /api/tasks/:id/status`(`app/web/src/features/tasks/api/client.ts:21-24`、`app/web/src/mocks/handlers.ts:88`)で行うが、app/api は `POST /tasks/{id}/start` / `POST /tasks/{id}/complete`(`app/api/route/task_handler.go:89-113`)で遷移する。これは別の cross-stack 契約乖離であり、本 Issue とは別課題(深追いはしない。記録として残す)。

### 根本原因

**現行のバグではない。** app/api / app/web はともにサンプル実装で、一覧のページネーション不在と単一取得契約の未整備は、サンプル規模(モック4件)での簡潔さを優先した設計上の未整備である。タスク件数が本番データ規模に増えたときに、(A) 詳細1件の表示に全件取得 + O(n) 線形探索、(B) 一覧取得の無制限な全件転送・パース・検証、として性能問題が顕在化する。根本の解消には app/api 側のエンドポイント契約(ページネーション追加・単一取得契約の確定)と、それに整合する web 側の client / mock / hook の改修が必要で、いずれか一方の stack だけでは解消しない。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: サンプル規模(モック4件)の現状では実害ゼロ。**今回のスコープでは対応必須ではなく**、本番データ規模でスケールさせる際の構造的課題として記録・追跡する。着手時は planner が計画化し、以下を impl-api / impl-web / tester / checker / review-performance のパイプラインで実施する。
- 以下はいずれも**提案(候補)**であり確定仕様ではない:
  - **(B) 一覧のページネーション追加(app/api)**: `List` と route の `list` に limit/offset(または cursor)を導入し、返却件数を制御する。Repository interface(`FindAll`)への影響、レスポンスへの総件数 / 次カーソルの付与方針を計画で確定する(impl-api)。
  - **(A) 単一取得契約の確定と整合(app/api + app/web)**: app/api に既存の `GET /tasks/{id}`(`route/task_handler.go:76-87`)のパス・レスポンス形状を契約として確定し、web の mock(`GET /api/tasks/:id`)/ client(`fetchTaskById`)/ schema を追加して整合させる(impl-api で契約確定、impl-web で mock/client/hook 追加)。
  - **web の単一取得 hook 化(app/web)**: `TaskDetailPage`(`app/web/src/app/router.tsx`)を、一覧取得 + `.find` から単一取得 hook(`fetchTaskById` + queryKey `["tasks", id]`)へ移行する(impl-web)。
- 仮説: 単一取得エンドポイントのレスポンス形状は、SPEC-002 の priority 反映後は `priority` を含む必要がある。SPEC-002 の完了状況と DTO 契約を同期させると手戻りが少ない(要確認)。

### 実施内容

- [ ] app/api: `List` / route `list` に limit/offset(または cursor)ページネーションを追加する(契約・レスポンス形状は計画で確定)
- [ ] app/api: `GET /tasks/{id}` のパス・レスポンス形状を契約として確定する(SPEC-002 の priority 反映と同期を検討)
- [ ] app/web: `GET /api/tasks/:id` の mock ハンドラを追加する(`app/web/src/mocks/handlers.ts`)
- [ ] app/web: client に `fetchTaskById` を追加し、schema と整合させる(`app/web/src/features/tasks/api/client.ts`)
- [ ] app/web: `TaskDetailPage` を単一取得 hook(queryKey `["tasks", id]`)へ移行する(`app/web/src/app/router.tsx`)
- [ ] tester: app/api はページネーション(件数制御 / 境界 / 不正パラメータ)と単一取得(正常 / 未存在)を table-driven で、app/web は単一取得 hook のローディング / 成功 / 未存在 / エラーを検証する

### 再発防止

- クライアント↔API の一覧系エンドポイントは、返却件数を制御する契約(ページネーション)をデフォルトの設計要件とし、詳細取得は一覧を経由せず単一取得エンドポイントで引く方針をレビュー観点にする。
- cross-stack の DTO / エンドポイント契約(パス・レスポンス形状・命名)は web の mock/client/schema と app/api の route/DTO を突き合わせてレビューする。

## 5. 経緯(時系列・追記のみ)

### 2026-07-08

- 起票。app/web のフロントエンドサンプル実装のパフォーマンスレビュー(review-performance)で挙がった Major 指摘 M1(タスク詳細1件の表示に全件取得 + 線形探索を用いている)を起点に、根本にある app/api 側の構造的ギャップ(cross-stack)を記録した。
- 事実確認(app/api): `service/task_service.go:63-75` の `List` は `FindAll` で全件返却・ページネーション引数なし、`route/task_handler.go:60-74` の `list` もクエリパラメータをパースしない。単一取得 `GET /tasks/{id}` は**実在**(`route/task_handler.go:76-87`、`service/task_service.go:48-61`)、レスポンス形状は snake_case `{id, title, status, created_at, updated_at}`(`route/task_handler.go:22-38`、priority は未含有)。
- 事実確認(app/web): client(`features/tasks/api/client.ts:7-11`)/ mock(`mocks/handlers.ts:65-68`)に単一取得契約なし。`TaskDetailPage`(`app/router.tsx:37-65`)は `useTasksQuery()` 全件取得 + `.find`(`:53`)の線形探索。モックデータは4件(`mocks/handlers.ts:28-61`)でサンプル規模のため現状実害ゼロ。
- スコープ外の観察を記録: (1) app/api の `taskResponse` に priority が無く web は priority を持つ(SPEC-002 対象、単一取得の DTO 確定時に同期が必要)、(2) web はステータス遷移を `PATCH /tasks/:id/status`、api は `POST /tasks/{id}/start`|`/complete` で行う契約乖離(別課題として記録、本 Issue では深追いしない)。
- severity は **low** と判定。判定根拠: サンプル規模(モック4件)の現状では全件転送・線形探索でも実害ゼロで、レビューでも「サンプル規模では概ね妥当」と評価済み。本番データ規模でタスク件数が増えたときにのみ O(n) コストが顕在化する予防的・構造的課題であり、web 側の単一取得 hook 化という部分的回避策も存在するため low(critical/high/medium ではないのは、現に応答性能が損なわれておらず主要機能も使えているため)。**本番データ規模の導入が具体化した時点で medium 以上へ再評価すること。**
- 関連 Spec: SPEC-002(Task に優先度を追加)を frontmatter `specs` に相互リンクした。判断根拠: 本 Issue の単一取得エンドポイントのレスポンス DTO 確定は、SPEC-002 が Task の DTO 契約(web の zod スキーマ / 命名 / enum との整合)に priority を追加する作業と、同じ Task 集約の web↔api 契約整備・同じ web ファイル群(client / schema / mocks)で交差するため。純粋な perf 課題としての関連は薄いが、DTO 契約整備の座標として実務上の同期点があると判断してリンクした。
- 次にやること: 本番データ規模でのスケールを決めた時点で planner に計画化を依頼し、app/api のページネーション追加・単一取得契約確定を impl-api、web の mock/client/hook 整合を impl-web、検証を tester、品質確認を checker / review-performance で実施する。

### 2026-07-10(パフォーマンスレビューの追加詳細: ページネーション対応時に併せて解消すべき 3 点)

- プロジェクト全体のパフォーマンスレビューで、本 Issue(ページネーション / 単一取得契約)に着手する際に **併せて対応すべき周辺の性能項目 3 点**が挙がった。いずれも **現状のサンプル規模(モック 4 件)では実害なし**で、本 Issue の解消(本番データ規模でのスケール対応)に合流させるのが合理的。単独での緊急対応は不要。
  - **(P1 / app/api インデックス欠如)** `app/api/db/migrations/000001_create_tasks.sql` の tasks テーブルに `created_at` のインデックスが無い(`created_at timestamptz NOT NULL`、`:31`。インデックス定義なし)。一方 `ListTasks` は `ORDER BY created_at, id`(`app/api/db/queries/tasks.sql:41`、生成物 `app/api/infra/postgres/sqlcgen/tasks.sql.go:61`)でソートするため、**深いオフセットのページネーションを入れると、索引が無い状態では全件ソート / スキャンが効いて劣化しうる**。ページネーション実装時に `(created_at, id)` の複合インデックス追加を検討する(担当: impl-db。ソートキーと同順の索引)。
  - **(P2 / app/web 再計算)** `app/web/src/features/tasks/components/TaskList.tsx:28` の `const tasks = sortByPriority(filterByStatus(data ?? [], status));` が `useMemo` 未使用で、`TaskList` の再レンダーごとにフィルタ + ソートを再計算する。件数が増えるとレンダーコストがタスク数に比例する。`data` / `status` を依存に `useMemo` で包む(担当: impl-web)。
  - **(P3 / app/web 再フェッチ)** `app/web/src/lib/queryClient.ts` の `QueryClient` は `retry: 1` / `refetchOnWindowFocus: false` のみ設定で **`staleTime` 未設定(既定 0)**。データが即 stale 扱いになり、クエリを使うコンポーネントのマウントごとに再フェッチが走る。一覧が本番規模になるとマウント都度の全件再取得コストが乗る。適切な `staleTime` を設定してマウント毎の再フェッチを抑える(担当: impl-web。値は要件で決定)。
- いずれも **現状サンプル規模では実害なし**であり、本 Issue が扱う「本番データ規模でのスケール」対応(ページネーション追加・単一取得契約整備)に着手するタイミングで **合流して一括対応**するのが妥当。単独では優先度を上げない。severity は **low** を維持(現状実害ゼロの構造的 / 予防的課題という性質は不変)。frontmatter は status=open 維持・updated=2026-07-10。
- 次にやること: 変わらず。ページネーション着手時の planner の計画に P1(created_at 索引)/ P2(TaskList の useMemo)/ P3(queryClient の staleTime)を含める。

### 2026-07-10(修正ラウンド: web 側の軽微改善 P2 / P3 を解消)

- 今回の修正ラウンドで、上記 2026-07-10 追記で挙げた周辺の性能項目のうち、**web 側で単独完結する軽微改善 P2 / P3 を解消**した(いずれも app/api の契約変更を伴わないため先行対応した)。
- 実施内容(impl-web):
  - **(P2 解消)** `app/web/src/features/tasks/components/TaskList.tsx` のフィルタ + ソート(`filterByStatus` / `sortByPriority`)を `useMemo` 化し、`data` / `status` を依存配列にして再レンダーごとの再計算を抑えた。Rules of Hooks を遵守するため、`useMemo` は early-return より前に移動した。
  - **(P3 解消)** `app/web/src/lib/queryClient.ts` の `QueryClient` に `staleTime: 30_000`(30 秒)を設定し、マウント都度の再フェッチを抑制した。
- 検証(checker): web の typecheck / lint / build / test green を確認。
- **status は open を維持。** 理由: 本 Issue の本体である以下が未対応で残るため:
  - **API のページネーション本体**(`GET /tasks` の limit/offset または cursor 対応)。OpenAPI 契約変更を伴うフィーチャーのため、本体は別途 SPEC として起票予定。単一取得契約の整合(A)もこの本体対応に合流させる。
  - **P1(app/api の `created_at` インデックス欠如)** — `app/api/db/migrations` に `(created_at, id)` 複合インデックスを追加(担当: impl-db)。深いオフセットのページネーション実装と同時に対応するのが合理的なため、本体着手時に合流させる。
- severity は **low** を維持(解消したのは現状サンプル規模で実害ゼロの軽微改善で、残る本体 / P1 も本番データ規模で顕在化する予防的・構造的課題という性質は不変)。frontmatter は status=open 維持・updated=2026-07-10。
- 次にやること: ページネーション本体を SPEC として起票(admin / spec skill)し、その計画に P1(created_at 複合インデックス)と単一取得契約の整合(A)を含める。本体 + P1 が解消した時点で本 Issue をクローズ可能。

### 2026-07-10(本体を SPEC-008 として起票・相互リンク)

- ページネーション本体を **SPEC-008**(`docs/specs/20260710-008-api-task-list-pagination.md`、status: draft)として起票した。対象は `GET /tasks` の offset/limit ページネーション + レスポンス封筒 `{items, total, limit, offset}`(既定 limit=20 / 最大 100、超過はサーバ側クランプ)、および本 Issue P1(`tasks.created_at` インデックス追加)。レスポンス形状を「配列 → 封筒」に変える**破壊的レスポンス形状変更**は、サンプルアプリで外部クライアントが無いこと・contract-drift CI で drift を機械検出できることを根拠にバージョニングせず一括切替とする(SPEC-008 §4)。
- 相互リンク: frontmatter `specs` に **SPEC-008** を追記(既存の SPEC-002 は維持)。SPEC-008 側 frontmatter の `issues` には ISSUE-008 が既に記載済み。
- **status は open を維持。** 理由: 本 Issue 本体(API のページネーション・単一取得契約の整合・P1 の created_at インデックス)は SPEC-008 として起票しただけで実装は未着手のため。実装は SPEC-008 / planner 経由で進める。severity は **low** を維持(現状サンプル規模で実害ゼロの予防的・構造的課題という性質は不変)。frontmatter は status=open 維持・updated=2026-07-10。
- 次にやること: SPEC-008 を approved にしたうえで planner に計画化を依頼し、SPEC-008 の実装完了(本体 + P1)と単一取得契約の整合(A)が解消した時点で本 Issue をクローズ可能。
