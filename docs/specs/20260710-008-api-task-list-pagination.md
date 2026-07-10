---
id: SPEC-008
title: app/api Task 一覧のページネーション(offset/limit + レスポンス封筒)
status: done  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-10
updated: 2026-07-10
issues: [ISSUE-008, ISSUE-025]       # 関連Issue ID (例: [ISSUE-003])
supersedes: null # 置き換える旧Spec ID
---

# SPEC-008: app/api Task 一覧のページネーション(offset/limit + レスポンス封筒)

## 1. ユーザー価値(なぜ作るか)

> **app/api を利用するクライアント(app/web / API 直接利用者)と運用者** が **Task 一覧を一定件数ずつ取得できるようになり**、**タスクが本番規模(数万件)に増えても 1 リクエストで全件を読み・転送してレイテンシ/メモリを浪費するリスクが無くなる** 価値を得る。

- **対象ユーザー**: app/web の利用者(一覧画面)、および app/api を直接叩くクライアント/運用者
- **解決する課題**: 現状 `GET /tasks` は `FindAll` で**全件を無条件に返す**(limit/offset なし)。タスク総数 n に比例して DB 取得・DTO 変換・JSON 転送がすべて O(n) で上限がなく、本番データ規模でタイムアウト/メモリ増大の要因になる(ISSUE-008)
- **得られる価値**: 一覧取得のコストとレスポンスサイズが「ページサイズ」で上限を持つ。UI はページ送りができ、大規模データでも一定の応答性を保てる
- **価値の検証方法**: `GET /tasks?limit=L&offset=O` が最大 L 件の items と総件数 total を返し、L 件を超えるデータがあってもレスポンスが limit で頭打ちになること、app/web の一覧が envelope を消費してページ送りできること、契約(OpenAPI)と生成型が一致し `contract-drift` CI が green であることを確認できたら成功とみなす

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- API クライアントとして、`GET /tasks?limit=20&offset=40` のように範囲を指定して Task を取得したい。なぜなら全件を一度に受け取ると重く、UI もページ単位で扱いたいから。
- app/web 利用者として、一覧を一定件数ずつ表示し、次/前ページに移動したい。なぜなら大量のタスクを一画面に詰め込みたくないから。

### 利用フロー

1. クライアントが `GET /tasks?limit=<L>&offset=<O>` を送る(いずれも省略可)
2. システムは `created_at, id` 昇順で安定ソートした Task のうち offset から最大 limit 件を返す。レスポンスは配列ではなく **封筒オブジェクト** `{ items, total, limit, offset }`
3. クライアント(app/web)は `total` と `limit`/`offset` から総ページ数・現在位置を算出し、次/前ページのリンクを出す

## 3. 要件(何を満たすべきか)

### 機能要件

- [ ] R1: `GET /tasks` に query パラメータ `limit`(1 件以上)/ `offset`(0 以上)を追加する。既定は `limit=20` / `offset=0`
- [ ] R2: レスポンスを**封筒オブジェクト** `{ "items": Task[], "total": number, "limit": number, "offset": number }` に変更する。`total` は絞り込み前の総件数(ページ計算用)、`limit`/`offset` はサーバが実際に適用した値をエコーする
- [ ] R3: `limit` が最大値(既定 **100**)を超えたらサーバ側で 100 にクランプして適用する(エコーされる `limit` はクランプ後の値)。`limit`/`offset` が非整数・負値など不正な場合は `400`(既存のエラー封筒 `{error}` 準拠)
- [ ] R4: OpenAPI 契約(`app/api/docs/openapi.yaml`)に query パラメータと封筒スキーマを反映し、app/web の生成物(型/zod/クライアント)を再生成して一致させる(SPEC-003 の単一契約原則。`contract-drift` CI が green)
- [ ] R5: 永続化層の両実装(`infra/postgres` = SQL `LIMIT`/`OFFSET` + `COUNT`、`infra/memory` = スライス + 件数)を同一の `Repository` interface 拡張で満たす(依存性逆転を維持)
- [ ] R6: app/web が封筒レスポンスを消費し、一覧のページ送り(最低限の次/前)ができる
- [ ] R7: `app/api/db/migrations` に `tasks.created_at` のインデックスを追加する(ISSUE-008 P1。`ORDER BY created_at, id` の深い offset での劣化緩和)

### 非機能要件

- **後方互換**: 本変更は `GET /tasks` のレスポンス形状を「配列 → 封筒」に変える**破壊的変更**。本リポジトリはサンプルアプリで外部公開クライアントを持たないため、バージョニングせず一括で切り替えることを許容する(この判断を §4 に明記)
- **一貫性**: ソート順は既存の `created_at, id` 昇順(安定)を維持し、ページ境界で重複/欠落が出ないようにする
- **契約単一ソース**: 型は Go の swag 注釈 → OpenAPI → 生成物、の一方向生成のみ。手書き二重定義しない(SPEC-003)

### スコープ外(やらないこと)

- カーソル(keyset)ページネーション(§4 で不採用理由を記載)
- ステータス/優先度による絞り込み・検索・ソート順の可変化(将来の別 Spec)
- app/web の高度なページ UI(ページ番号ジャンプ・無限スクロール等)。今回は最低限の次/前に留める
- app/auth 側 API(OpenAPI 契約対象外)への波及

## 4. 設計(どう実現するか)

### 方針

offset/limit 方式を採用する。実装が単純で `LIMIT`/`OFFSET` に素直に対応でき、UI も総件数からページ数を出せる。レスポンスは封筒 `{items,total,limit,offset}` に統一し、`total` でクライアントのページ制御を可能にする。ソートは既存の `created_at, id` 昇順を踏襲し、P1 のインデックスで深い offset の劣化を抑える。破壊的なレスポンス形状変更は、サンプルアプリで外部クライアントが無いこと・契約 drift を CI が機械検出することを根拠に、バージョニングせず一括切替とする。

### アーキテクチャ / データ / インターフェース

- **route**(`app/api/route`): list ハンドラで `limit`/`offset` を parse・検証(不正は 400)・クランプ。service へ渡し、封筒 DTO を返す。swag 注釈を query パラメータ + 封筒レスポンスに更新
- **service**(`app/api/service`): `List(ctx, limit, offset)` に拡張し、items と total を返す
- **domain / repository port**(`app/api/domain/task`): `Repository` interface に「範囲取得 + 総件数」を表すメソッドを追加(例: `List(ctx, limit, offset) ([]*Task, error)` + `Count(ctx) (int, error)`、または items/total を返す 1 メソッド)。ページング引数はプリミティブか小さな値オブジェクトで表現し、domain の他層非依存を維持
- **infra/postgres**: sqlc クエリに `LIMIT $1 OFFSET $2` の一覧取得と `COUNT(*)` を追加(sqlc 再生成しコミット)。`db/queries/tasks.sql` 更新
- **infra/memory**: ソート済みスライスの範囲切り出し + 長さ
- **migration**(`app/api/db/migrations`): `CREATE INDEX ... ON tasks (created_at)`(必要に応じ `(created_at, id)`)。goose の新規マイグレーションとして追加(**app/migrator の DDL 適用経路に乗る**。app/migrator リファクタと衝突しないよう、着手タイミングを調整する)
- **OpenAPI / web**: 契約再生成(`make openapi`)→ app/web 再生成(`bun run generate`。ISSUE-023 修正済みの hey-api next 版で動作)。`features/tasks` の hooks/components を封筒対応にし、最小のページャを追加

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| カーソル(keyset)ページネーション | 大規模で offset より効率的だが、実装(カーソル発行/解釈・安定キー)が複雑でサンプルの規模に対し過剰。まず offset/limit で契約を確立する |
| レスポンスを配列のままヘッダ(`X-Total-Count` 等)で total を返す | 形状は非破壊だが、生成型・zod・TanStack Query との相性が悪く、SPEC-003 の型契約単一ソースに乗せにくい。封筒の方が型で表現でき明快 |
| バージョニング(`/v2/tasks`)して両立 | サンプルアプリで外部クライアントが無く、二重メンテのコストに見合わない。破壊的一括切替で足りる |
| クライアント側だけでページング(全件取得のまま) | 根本の O(n) 全件取得が残り価値(§1)を満たさない |

## 5. 実装計画

詳細は `docs/plans/SPEC-008-plan.md`(planner が作成)。着手前に planner が以下を設計する:

- [x] T1: planner が方針(port の拡張形・封筒 DTO・検証/クランプ仕様・app/migrator リファクタとのタイミング調整)と影響範囲・手順・テスト戦略を計画化
- [x] T2: tester が要件からテスト作成(api: domain/service/route/memory + 統合テスト、web: pagination domain/TaskPager/hooks)
- [x] T3: impl-api(route/service/domain port/memory)∥ impl-db(infra/postgres クエリ・sqlc・migration の created_at インデックス)で実装
- [x] T4: OpenAPI 再生成(`make openapi`)→ impl-web が `bun run generate` + hooks/components/pager 追従
- [x] T5: tester がテスト実行・追加、checker が全スタック + contract-drift 相当の整合確認(全 green)
- [x] T6: review-security / review-performance / review-spec(Blocker/Major 0)

## 6. 経緯(時系列・追記のみ)

### 2026-07-10

- 初版作成(status: draft)。プロジェクト全体レビューで再確認した ISSUE-008(Task 一覧の全件取得・ページネーション欠如)の本体対応として起票。ISSUE-008 の web 側軽微改善(P2: 一覧ソートの useMemo 化 / P3: queryClient の staleTime 設定)は既に別途対応済みで、本 Spec は API 契約を含む本体(offset/limit + 封筒)と P1(created_at インデックス)を扱う。
- 設計判断: offset/limit 方式・封筒レスポンス `{items,total,limit,offset}`・既定 limit=20 / 最大 100(超過はクランプ)・破壊的レスポンス形状変更をバージョニングせず一括切替(サンプルアプリ・contract-drift CI で drift 検出可能を根拠)。カーソル方式・ヘッダ total・バージョニングは不採用(§4)。
- 実装は未着手。**着手タイミングの注意**: 起票時点で別セッションが `app/migrator`(DDL 適用ツール)を DDD レイヤへリファクタ中のため、`app/api/db/migrations` への新規マイグレーション追加(R7)は当該リファクタが settle してから行い、goose 適用経路の衝突を避ける。実装計画は planner に委ねる。
- 相互リンク: ISSUE-008 と相互参照(Issue 側 `specs` への SPEC-008 追記は issue-creator に委譲、反映済み)。
- 承認(status: approved)。ユーザーが全体レビューの結果を受けて本体実装を承認。planner が `docs/plans/SPEC-008-plan.md` を作成済み: port は `FindAll` → `ListPage(ctx, task.Page) ([]*Task, int, error)` へ置換、業務ルール(既定 20 / 最大 100 クランプ / `limit<1` は 400)を domain の値オブジェクト `task.Page`(`NewPage`)に集約、route はワイヤのパース(非整数→400)のみ担う。手順は TDD 先行 → impl-api ∥ impl-db → `make openapi` 契約確定 → impl-web(`bun run generate` + pager)→ tester/checker → R7 索引(app/migrator リファクタ settle 後・最後段。**現時点で settle 済み=commit `ca4be4d`**)→ review。`limit<1` は baseline どおり 400 を採用する。
- **実装完了・done。** 計画どおり完走: impl-api(domain `Page` VO + `Repository.ListPage` + service + route + infra/memory)∥ impl-db(`db/queries` LIMIT/OFFSET+COUNT・sqlc 再生成・infra/postgres・R7 `created_at` インデックス `000002`)→ gosec G115(int→int32)を LIMIT/OFFSET の bigint 化(int64 パラメータ)で解消 → `make openapi` 契約再生成 → impl-web(`bun run generate` + 封筒消費 + `domain/pagination.ts` + `TaskPager` + MSW + router search schema)。tester が境界テストを追加(api 89 pass / web 106 pass)、checker が全スタック green(golangci-lint v2.12.2 gosec 0)+ contract-drift/sqlc-drift 相当の再生成 diff 0 を確認。
- レビュー3観点: **review-spec** 全7要件充足・Blocker/Major 0・done 可、**review-security** Blocker/Major 0(SQL パラメータ化・入力検証/クランプ適正・int64 で overflow 解消)、**review-performance** Blocker 0(Major-1 `CountTasks` 無制限 COUNT と offset 上限欠如は別 Issue でスケーリング/DoS フォローアップとして起票・追跡)。指摘対応として web に `placeholderData: keepPreviousData`(ページ遷移の Loading フラッシュ回避)と MSW モックの 400 エラー契約整合を適用。
- 明確化: R3 の下限は「`limit` が 1 未満(0 含む)は 400」。LIST/COUNT は非トランザクションのため同時書き込み下で `total` と `items` がわずかにずれ得る(サンプル規模で許容)。価値の検証方法(封筒返却・web ページ送り・contract green)を満たしたため status を done とした。ISSUE-008 は resolved、残る技術的負債(COUNT スケーリング・offset 上限)は新規 Issue で追跡。
