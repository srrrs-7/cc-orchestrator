---
id: ISSUE-022
title: app/migrator の並行 goose up に排他制御(advisory lock)が無く、desired_count>1 のローリングデプロイで goose_db_version 書き込みが競合し得る
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-12
specs: [SPEC-005]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-022: app/migrator の並行 goose up に排他制御(advisory lock)が無く、desired_count>1 のローリングデプロイで goose_db_version 書き込みが競合し得る

## 1. ユーザー価値への影響(なぜ対応するか)

> **将来 ECS サービスを `desired_count>1` にスケールする運用者** の **「ローリングデプロイ時にマイグレーションが安全に一度だけ適用される」という期待** が、**複数の migrate init コンテナが同一 DB へ並行実行する `goose up` に排他制御(advisory lock)が無いことで、条件付きで損なわれ得る**。

- **影響を受けるユーザー**: app/api(または鍵外部化後の app/auth)の ECS サービスを `desired_count>1` に増やして可用性確保 / スケールアウトする運用者
- **損なわれる価値**: マイグレーションの「安全に一度だけ適用される」保証。複数 init コンテナが同一 DB へ同時に `goose up` を実行すると `goose_db_version` への書き込みが競合し得る
- **影響範囲・頻度**: **現状は発生しない**。`app/iac/envs/dev` の既定は `desired_count = 1`(auth は JWKS 単一化制約からも 1 固定)で、複数 init コンテナが同時に `goose up` を走らせることが無いため。`desired_count>1` かつ未適用マイグレーションがある状態のローリングデプロイという **特定条件下(将来の構成変更の前提条件)** でのみ顕在化し得る
- **回避策**: あり(`desired_count = 1` を維持する / 一回限りの ECS タスク方式に切り替えて実行主体を 1 回に限定する / app/migrator に goose の advisory lock を導入する)

## 2. 現象(何が起きているか)

### 期待する動作

`desired_count>1` のローリングデプロイで複数の新タスクがほぼ同時に起動しても、各タスクの migrate init コンテナが実行する `goose up` が同一 DB に対して直列化(排他)され、マイグレーションが安全に一度だけ適用される。

### 実際の動作

`app/migrator` の goose 実行(`app/migrator/main.go:103` の `goose.RunContext(ctx, command, db, migrationsDir)`)には advisory lock 等の排他制御が無い(goose ライブラリはデフォルトで advisory lock を取らない)。`CREATE DATABASE` の冪等化(`42P04` / `23505` + 存在再確認、`app/migrator/database.go` の `ensureDatabase`)は実装済みだが、これは DB 作成のレースのみをカバーし、**`goose up` の版適用(`goose_db_version` テーブルへの書き込みを含む)は保護されていない**。複数の init コンテナが同時に `goose up` を実行すると、`goose_db_version` の書き込みや版適用が競合し得る。

### 再現手順

> 注: 本リポジトリの既定構成(`desired_count = 1`)では発生しない。以下は第三者が競合条件を作る手順(agent は `terraform apply` を実行しない方針のため、apply は手動実行前提)。

1. 対象サービス(例: api)の `desired_count` を 2 以上に設定する(`app/iac/envs/dev` の該当変数)。
2. 未適用の goose マイグレーションが存在する状態で、新しいリビジョンをデプロイ(ローリング更新)する。
3. ECS が複数の新タスクをほぼ同時に起動し、各タスクの migrate init コンテナ(`app/migrator -target api`)が同一の `api` データベースに対して `goose up` を並行実行する。
4. goose の版適用・`goose_db_version` 書き込みが排他されていないため、同一マイグレーションの二重適用や `goose_db_version` 書き込み競合が起こり得る(具体的な失敗モードは実測未確認。仮説)。

### 環境・条件

- 対象: AWS(ECS)のローリングデプロイ経路のみ。ローカル(ルート `Makefile` の `migrate` は `app/migrator` を api → auth の順に **逐次 2 回** `go run`)・CI(単発実行)では並行 `goose up` は発生しない。
- api と auth は **別データベース**(`DB_NAME` api=`api` / auth=`auth`)を触るため、**api の migrate コンテナと auth の migrate コンテナ同士は競合しない**(別 DB への `goose up` は独立)。競合し得るのは **同一サービス内の複数タスク間**のみ。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/migrator/main.go:103` は `goose.RunContext(ctx, command, db, migrationsDir)` を呼ぶのみで、goose の `SessionLocker`(Postgres advisory lock)等の排他オプションを指定していない。goose ライブラリは明示設定が無い限り advisory lock を取らない。
- 事実: DB 作成のレースは冪等化済み(`app/migrator/database.go` の `ensureDatabase` が `CREATE DATABASE` の `42P04` / `23505` を成功扱い + `pg_database` 再確認)。ただしこれは **DB 作成のレースのみ**をカバーし、`goose up` の版適用書き込みは対象外。
- 事実: `app/iac/envs/dev` の既定 `desired_count = 1`(auth は JWKS 単一化制約からも 1 固定。`app/iac/modules/service/README.md`「auth を `desired_count = 1` に固定する理由」)のため、現構成では複数 init コンテナが同時に `goose up` を実行することはなく実害なし。
- 事実: この並行実行の競合は **既知事項として文書化済み** — `app/iac/modules/service/README.md`「イメージ・並行実行・代替案」節が「`desired_count` を 2 以上にするとローリングデプロイの瞬間に複数の新タスクの migrate コンテナが同じデータベースに `CREATE DATABASE` / `goose up` を並行実行し得る。`desired_count` を増やす場合は goose のアドバイザリロック(app/migrator 側で有効化するか)か、一回限り ECS タスク方式への切り替えを検討すること」と明記している(SPEC-005 plan RF.6.1 RF-f 系譜)。
- 事実: api と auth の migrate コンテナは別データベースを触るため相互に競合しない(上記 README 同節に明記)。
- 仮説: 具体的な失敗モード(片方のタスクの `goose up` がエラー終了 → `dependsOn: SUCCESS` を満たせずロールアウト失敗、あるいはマイグレーションの部分適用 / `goose_db_version` の不整合)は実測未確認。

### 根本原因

`app/migrator` は DB 作成のレースを冪等化で解いた一方、`goose up` の適用(`goose_db_version` 書き込み)の排他制御は「現構成(`desired_count = 1`)では並行実行が発生しない」ため実装を見送っている。これは構造的な欠陥というより、現状スコープに合わせた意図的な未実装であり、`desired_count>1` 化がこの排他制御の欠如を **前提条件として顕在化させ得る**。

## 4. 対応(どう解決するか)

### 対応方針

`desired_count>1` を採用する前に、以下のいずれかで並行 `goose up` の排他を担保する(SPEC-005 本体スコープ外の後続作業として追跡):

- **goose の advisory lock(`SessionLocker`)を `app/migrator` で有効化する**(担当: impl-db)。`app/migrator` は goose を **ライブラリ**として実行しているため(`app/migrator/main.go`)、`goose.WithSessionLocker` 等で Postgres advisory lock を取り、同一 DB への `goose up` を直列化できる。goose は `app/migrator/go.mod` に閉じるため、app/api・app/auth の runtime require は pgx のみを維持できる(SPEC-005 価値検証 #4)。
- **一回限りの ECS タスク方式へ切り替える**(担当: impl-iac、plan まで)。`aws_ecs_service` を持たない `aws_ecs_task_definition` を用意しデプロイパイプライン / 手動で `RunTask` する方式で、実行主体を 1 回に限定して構造的に競合を避ける(`app/iac/modules/service/README.md`「イメージ・並行実行・代替案」の代替案として記録済み)。
- `desired_count>1` を設定する際のチェック項目として「並行 migrate の排他が担保されているか」を明記する(ISSUE-002 の SPOF 解消項目 7・14 で `desired_count>=2` を挙げているため、それらと併せて確認する)。

### 実施内容

- [x] `desired_count>1` を採用する前提として、advisory lock 導入(app/migrator)と一回限り task 方式(iac)のいずれを採るか決める → advisory lock 導入を採用
- [x] 採用方式を実装する(app/migrator の `SessionLocker` 有効化)
- [ ] iac の README(`modules/service`)の `desired_count>=2` に関する記述に「並行 migrate の排他」をチェック項目として追記する
- [ ] ISSUE-002(SPOF 解消で `desired_count>=2` を挙げる項目 7・14)と本 Issue の依存関係を相互に確認する

### 再発防止

マイグレーション実行主体を複数化し得る構成変更(`desired_count` の増加・並列デプロイ)を入れる際は、マイグレーションの排他(単一実行主体 or advisory lock)をセットで確認することを、デプロイ設計チェックに加える。

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。SPEC-005 リファクタリング(別データベース + `app/migrator` 集約)のレビュー(review-performance、E3/RD3)で挙がった「並行 `goose up` の排他制御(advisory lock)欠如」を記録。SPEC-005 と相互リンク(frontmatter `specs: [SPEC-005]`、Spec 側 `issues` に ISSUE-022 を追記)。
- 事実確認: `app/migrator/main.go:103` の `goose.RunContext` に advisory lock 等の排他オプションが無い。`CREATE DATABASE` の冪等化(`app/migrator/database.go` の `ensureDatabase`、`42P04` / `23505` + 存在再確認)は実装済みだが、`goose up`(`goose_db_version` 書き込み含む)は保護外。既定 `desired_count = 1`(auth は JWKS 単一化制約からも 1 固定)のため現構成では並行実行が発生せず実害なし。`app/iac/modules/service/README.md`「イメージ・並行実行・代替案」節に既知事項として言及済み(対応候補 = advisory lock 導入 or 一回限り task 方式)。api と auth は別 DB のため両者の migrate コンテナ同士は競合せず、競合し得るのは同一サービスの複数タスク間のみ。
- 重複確認: 既存 Issue に advisory lock / 並行 `goose up` / `goose_db_version` 書き込み競合を扱うものは無い(`docs/issues` を横断確認)。`desired_count` の既存言及(ISSUE-001 のデータ一貫性、ISSUE-002 の項目 7・14 = ECS の SPOF 解消)はいずれも **可用性 / 単一障害点**の文脈であり、マイグレーションの排他とは別テーマ。SPEC-005 起票済みの ISSUE-005(平文パスワード)/ ISSUE-015(authcode 無制限増加)/ ISSUE-016(DB 最小権限・TLS)/ ISSUE-017(migrate イメージの ECR push 経路)とも重複しない独立テーマ。ISSUE-018(route エラー型)/ ISSUE-020(bun.lock)/ ISSUE-021(healthcheck SSRF)とも無関係。
- severity は **low** と判定(依頼の「low〜medium」の下限を採用)。判定根拠: 現構成(`desired_count = 1` 固定)では **一切発生せず実害が無い** ため、現時点の性質は軽微(low)。ただし `desired_count>1` へ構成変更した場合は `goose_db_version` の書き込み競合というデータ整合性リスク(実質 medium 相当)に上がるため、**`desired_count>1` 化の前提条件**として追跡する。回避策(`desired_count = 1` 維持 / 一回限り task / advisory lock)が複数存在する。
- 次にやること: `desired_count>1` を採用する意思決定が生じた時点で、後続の impl-db(app/migrator の advisory lock)または impl-iac(一回限り ECS タスク)がいずれかを実装し、iac README のチェック項目に反映する。

### 2026-07-12

- advisory lock 導入を採用し実装した。`app/migrator/infra/goose/runner.go` を goose のグローバル API(`goose.RunContext`)から Provider API(`goose.NewProvider` + `goose.WithSessionLocker`)へ移行。`lock.NewPostgresSessionLocker()` で Postgres session-level advisory lock(`pg_try_advisory_lock`)を生成し、`goose.WithSessionLocker` で Provider に渡すことで、複数の init コンテナが同一 DB へ並行して `goose up` を実行した場合でも直列化される。既存の `r.timeout` によるコンテキストキャンセルは引き続き advisory lock の取得待ちにも適用されるため、ハング時のフェイルファスト挙動は維持されている。`make check`(fmt-check + lint + vet + build + test)全項目パス確認済み。残タスク: iac README の `desired_count>=2` チェック項目追記・ISSUE-002 との依存確認は別途対応。
