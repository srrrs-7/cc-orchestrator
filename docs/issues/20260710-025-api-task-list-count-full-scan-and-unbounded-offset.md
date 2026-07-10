---
id: ISSUE-025
title: app/api Task 一覧の COUNT(*) 全件カウント・LIST/COUNT の逐次 2 クエリ・offset 無上限(SPEC-008 実装のスケーリング/DoS フォローアップ)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-10
specs: [SPEC-008]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-025: app/api Task 一覧の COUNT(*) 全件カウント・LIST/COUNT の逐次 2 クエリ・offset 無上限(SPEC-008 実装のスケーリング/DoS フォローアップ)

## 1. ユーザー価値への影響(なぜ対応するか)

> **`GET /tasks`(一覧)を利用するクライアント(app/web / API 直接利用者)** の **一覧取得の応答性能** が **本番データ規模でタスク件数 n が増えると、毎回走る `COUNT(*)` の O(n) コストと深い offset の O(offset) スキップコストによって悪化しうる**(将来条件下・予防的課題)。

- **影響を受けるユーザー**: `GET /tasks` を叩くクライアント全般。とりわけタスク件数が本番規模(数万件以上)に増えた環境の利用者
- **損なわれる価値(将来条件下)**: 一覧取得のレイテンシ。加えて、`offset` に上限が無いため深い offset を連打されると応答遅延を積む限定的な DoS 余地がある
- **影響範囲・頻度**: **現時点では実害ゼロ。** SPEC-008 のページネーションは正しく機能しており、サンプル規模(タスク数が少ない状態)では `COUNT(*)`・深い offset いずれもコストは無視できる。タスク件数が増えたときにのみ顕在化する構造的課題
- **回避策**: あり(現状はサンプル規模のため実害なし)。将来の顕在化に対しては本文「4. 対応」の候補(COUNT の 1 クエリ統合・total キャッシュ・近似カウント・offset 上限 / rate limit)で緩和する

## 2. 現象(何が起きているか)

本 Issue は SPEC-008(Task 一覧のページネーション)の実装レビューで検出した、スケーリング / DoS のフォローアップ項目(3 点)である。いずれも SPEC-008 の要件を満たした上で残る性能特性であり、現行のバグではない。

### 期待する動作

- 一覧取得(`GET /tasks?limit=L&offset=O`)のコストが、タスク総数 n や指定 offset の大きさに対して過度に増大しない
- 巨大な `offset` を受理しても、応答遅延を無制限に積み上げる余地を残さない

### 実際の動作

- **(1) `CountTasks` の無制限全件カウント**: `app/api/infra/postgres/task_repository.go:127-149` の `ListPage` が、ページ取得のたびに `r.q.CountTasks(ctx)`(`:128`)を実行する。その実体は `app/api/db/queries/tasks.sql` の `CountTasks` = `SELECT COUNT(*) FROM tasks`(WHERE 句なし・全件対象)。Postgres の `COUNT(*)` は MVCC の可視性判定のためテーブル総件数に比例(O(n))し、これが `GET /tasks` の**全呼び出しで毎回走る**ため、データ増で一覧レイテンシの支配項になり得る
- **(2) LIST / COUNT が逐次 2 ラウンドトリップ**((1) と同系の懸念): `ListPage` は `CountTasks`(`:128`)→ `ListTasksPage`(`:133`)を**逐次**に実行しており、1 リクエストにつき DB へ 2 往復する。単一クエリに統合していないため、往復レイテンシとカウントコストがそのまま積み上がる
- **(3) `offset` に上限が無い**: `app/api/domain/task/page.go` の `NewPage` は `limit` を `MaxLimit`(100)にクランプする一方、`offset` は負値を拒否する(`o < 0` → `ErrInvalidOffset`)だけで**上限が無く、巨大値も受理する**。route(`app/api/route/task_handler.go` の `parseQueryInt` → `service.List` → `NewPage`)も offset の上限検査をしない。深い offset は索引があっても O(offset) のスキップコストを伴い、`?limit=1&offset=999999999` の連打で応答遅延を積む**限定的な DoS 余地**を残す

### 再現手順

第三者がコード観察で構造を確認できる(現状サンプル規模では実測での遅延は再現しない)。

1. **全件 COUNT(*)**: `app/api/db/queries/tasks.sql` の `CountTasks` が `SELECT COUNT(*) FROM tasks`(WHERE なし)であることを確認する。`app/api/infra/postgres/task_repository.go:128` の `ListPage` がページ取得のたびにこれを呼ぶことを確認する
2. **逐次 2 クエリ**: 同 `ListPage`(`:127-149`)が `CountTasks`(`:128`)と `ListTasksPage`(`:133`)を 2 文に分けて逐次実行していることを確認する
3. **offset 無上限**: `app/api/domain/task/page.go` の `NewPage` が `limit > MaxLimit` はクランプする一方、`offset` は `o < 0` のみ拒否し上限を持たないことを確認する。route 側(`app/api/route/task_handler.go` の list ハンドラ / `parseQueryInt`)も offset 上限をチェックしないことを確認する
4. **理論上の DoS 余地**: 本番データ規模を仮定すると、`GET /tasks?limit=1&offset=999999999` は索引があっても offset 件数のスキップコスト(O(offset))を要する。これを連打すると応答遅延を積める

### 環境・条件

- 対象 stack: app/api(Go / DDD サンプル、`infra/postgres`)。永続化が Postgres 経路のときに顕在化する。`infra/memory` はスライス切り出しのため DB 由来の (1)(2) は該当せず、(3) の offset 無上限は同様に成立する
- 発見文脈: SPEC-008(Task 一覧のページネーション)実装のレビューで検出したスケーリング / DoS フォローアップ

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `CountTasks` は `SELECT COUNT(*) FROM tasks`(WHERE なし)で、`ListPage`(`app/api/infra/postgres/task_repository.go:128`)が一覧取得のたびに実行する。Postgres の `COUNT(*)` は MVCC 可視性判定によりテーブル総件数に比例(O(n))する。
- 事実: `ListPage`(`app/api/infra/postgres/task_repository.go:127-149`)は `CountTasks`(`:128`)→ `ListTasksPage`(`:133`)の 2 文を逐次実行する(1 リクエストにつき DB 2 往復)。コード上のコメント(`:122-126`)も両者を別文としており、windowed query に統合していない。
- 事実: `NewPage`(`app/api/domain/task/page.go`)は `limit` を `MaxLimit=100` にクランプするが、`offset` は `o < 0` を拒否するのみで上限が無い(巨大値を受理)。route も offset 上限を検査しない。
- 仮説: 本番データ規模では (1) の `COUNT(*)` が `GET /tasks` レイテンシの支配項になり得る。深い offset の (3) は索引を張っても O(offset) のスキップコストが避けられず、offset 無上限との組み合わせで限定的な DoS 余地になる(実測は未取得)。

### 根本原因

**現行のバグではない。** SPEC-008 のページネーション(offset/limit + 封筒 `{items,total,limit,offset}`)は要件どおり機能している。本 Issue は、その実装が持つ性能特性 — (1) total を都度全件 `COUNT(*)` で得る、(2) LIST と COUNT を統合していない、(3) offset に上限を設けていない — が、**タスク件数が本番規模に増えたときにコストとして顕在化する構造的な技術的負債**である。サンプル規模での簡潔さ・正確な total(絞り込み前総件数)を優先した設計上のトレードオフに起因する。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: サンプル規模の現状では実害ゼロ。**今回のスコープでは対応必須ではなく**、本番データ規模でスケールさせる際の技術的負債として記録・追跡する。着手時は planner が計画化し、impl-api / impl-db / tester / checker / review-performance のパイプラインで実施する。
- 以下はいずれも**候補(将来検討)**であり確定仕様ではない:
  - **(A) COUNT を LIST に統合**: `COUNT(*) OVER()` を用いて 1 クエリで items と total を同時取得し、逐次 2 往復((2))を解消する。**ただし空ページ(0 行)時にウィンドウ関数の行が返らず total が取得できない既知のトレードオフ**があるため、offset が総件数を超えたケースの total の扱いを計画で確定する必要がある
  - **(B) total の短時間キャッシュ**: total を短時間(TTL 数秒〜)キャッシュし、毎回の `COUNT(*)`((1))を間引く。厳密性が緩む点(直近の追加/削除が total に即反映されない)を許容できるかを要件で判断する
  - **(C) 近似カウント**: `pg_class.reltuples` などの統計由来の近似値で total を代替し、O(n) の厳密 `COUNT(*)` を避ける。ページ制御に厳密な total が要らない場合の選択肢
  - **(D) offset 上限 / rate limit**: `offset` に上限を設ける(超過は 400、または keyset/cursor への移行を促す)か、rate limit を導入して (3) の DoS 余地を塞ぐ。SPEC-008 §4 で不採用としたカーソル(keyset)ページネーションへの移行も、深い offset を根本回避する選択肢として再検討し得る
- 仮説: 厳密な total を保ちつつコストを下げたい場合は (A)+(D)、total の厳密性を緩めてよい場合は (B) or (C) が有力(要件で確定)。

### 実施内容

- [ ] (A) `COUNT(*) OVER()` 等での LIST/COUNT 統合を検討し、空ページ時の total 取得トレードオフの扱いを確定する(impl-db / impl-api)
- [ ] (B)/(C) total の短時間キャッシュ or `pg_class.reltuples` 近似カウントの適否を要件で判断する
- [ ] (D) `offset` 上限の導入 or rate limit で深い offset の DoS 余地を塞ぐ(上限超過時の HTTP 応答契約を確定)
- [ ] tester: 大規模データ相当の性能検証(可能なら)と、offset 上限を入れる場合の境界(上限直下 / 上限超過)テストを追加する
- [ ] review-performance: 採用案の効果(レイテンシ・往復数)を確認する

### 再発防止

- 一覧系エンドポイントで total を返す設計では、都度の全件 `COUNT(*)` のコスト・LIST と COUNT の往復数・offset 上限の有無をレビュー観点にする(本番データ規模でのスケール前提の設計チェック)。
- 深い offset を許容するページネーションは、上限 / rate limit / keyset への移行余地をセットで検討する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。SPEC-008(Task 一覧のページネーション)実装のレビューで検出したスケーリング / DoS フォローアップ 3 点を記録した。(1) `ListPage` が毎回 `SELECT COUNT(*) FROM tasks`(WHERE なし)を実行し Postgres の `COUNT(*)` は O(n)(`app/api/infra/postgres/task_repository.go:128`、`app/api/db/queries/tasks.sql` の `CountTasks`)、(2) `CountTasks` → `ListTasksPage` の逐次 2 ラウンドトリップ(`app/api/infra/postgres/task_repository.go:127-149`)、(3) `NewPage` は `limit` を 100 にクランプする一方 `offset` は負値のみ拒否で上限なし(`app/api/domain/task/page.go`)で、深い offset の O(offset) スキップコストと相まって限定的 DoS 余地。
- いずれも **SPEC-008 の要件充足を損なう現行バグではなく**、本番データ規模で顕在化する構造的な技術的負債。サンプル規模の現状では実害ゼロ。
- severity は **low** と判定。判定根拠: 現状サンプル規模で応答性能は損なわれておらず主要機能も正常(critical/high ではない)。(3) の DoS 余地はテーブルに実際に skip 対象の行が多量に存在して初めてコストになるため、本番データ規模・公開環境という条件が付く予防的課題(現時点で medium の「回避策ありの実害」には至らない)。**本番データ規模の導入や公開エンドポイント化が具体化した時点で、特に (3) の offset 無上限 DoS 余地を medium 以上へ再評価すること。**
- 関連: 本 3 点は SPEC-008 の実装が直接の出所のため frontmatter `specs` に SPEC-008 を相互リンクした。また ISSUE-008(Task 一覧のページネーション / 単一取得契約の構造的課題)の本体対応(= SPEC-008)から派生した残課題であり、ISSUE-008 の resolved 化に伴い本 Issue で追跡を引き継ぐ(ISSUE-008 の 2026-07-10 経緯参照)。
- 次にやること: 本番データ規模でのスケールが具体化した時点で planner に計画化を依頼し、対応候補 (A)〜(D) の適否を要件で確定して impl-db / impl-api / tester / checker / review-performance で実施する。
