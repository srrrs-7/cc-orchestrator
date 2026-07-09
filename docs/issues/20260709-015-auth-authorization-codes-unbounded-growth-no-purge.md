---
id: ISSUE-015
title: SPEC-005 で Postgres 化した認可コードが lazy eviction のみで無制限に増加する(未消費・期限切れコードの恒久残存)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-09
updated: 2026-07-09
specs: [SPEC-005]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-015: SPEC-005 で Postgres 化した認可コードが lazy eviction のみで無制限に増加する(未消費・期限切れコードの恒久残存)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth(認可サーバー)を本番運用する運用者** の **永続化基盤の運用安定性・コスト予測性** が **`authorization_codes` テーブルが時間とともに無制限に肥大化し、ストレージと autovacuum のコストが単調増加することで損なわれる**。

- **影響を受けるユーザー**: app/auth を Postgres モードで本番運用する運用者、および長期運用で RDS のストレージ・autovacuum を管理する人
- **損なわれる価値**: `authorization_codes` テーブルサイズが発行トラフィックに対して有界に保たれない(ストレージ・autovacuum コストの無制限増加)
- **影響範囲・頻度**: 特定条件下・累積的。`/token` まで到達しない離脱フロー(ブラウザのバックボタン、`/authorize` への走査、放棄されたログイン)で発行された**未消費・期限切れコードが恒久的に残る**。1 件あたりは小さいが、消費されないコードが恒久蓄積する
- **回避策**: あり(運用者が手動または定期的に `DELETE FROM authorization_codes WHERE expires_at <= now()` を実行できる)。ただし恒久的な自動 purge は未実装

## 2. 現象(何が起きているか)

### 期待する動作

未消費・期限切れの認可コードは、有限のバックログを超えて蓄積しないよう(定期 purge 等で)回収され、テーブルサイズが認可コードの発行トラフィックに対して有界に保たれる。

### 実際の動作

期限切れコードの削除は、**その同じ code が再度 `FindByCode` / `Consume` される時にのみ起きる lazy eviction のみ**。`/token`(`Consume`)に到達せず消費されないコードは、対応する `FindByCode` / `Consume` 呼び出しが二度と来ないため、`DeleteExpiredAuthCode` が起動せず、期限切れ後もテーブルに恒久的に残り続ける。

### 再現手順

1. app/auth を Postgres モードで起動する(`DB_HOST` 等を設定)。
2. `/authorize` を叩いて認可コードを発行させる(`Save` が `authorization_codes` に 1 行 INSERT する。`app/auth/infra/postgres/authcode_repository.go:40-55`)。**後続の `/token`(`Consume`)は実行しない**(= ユーザーが離脱したフロー)。
3. そのコードの `expires_at` 経過を待つ。
4. `SELECT count(*) FROM authorization_codes` で、消費されなかったコードが期限切れ後もテーブルに残っていることを確認する。この code に対する `FindByCode` / `Consume` が二度と呼ばれない限り `DeleteExpiredAuthCode`(`db/queries/authcodes.sql:32-40`)は起動しない。

### 環境・条件

- 対象: app/auth の Postgres 永続化(SPEC-005 の `infra/postgres`)。
- `infra/memory` 実装では該当しない: プロセス再起動でマップがリセットされるため、未消費コードは自然に回収されていた(= Postgres 化で失われた劣化パス)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: 期限切れコードの削除は `app/auth/infra/postgres/authcode_repository.go:62-88` の `FindByCode` 内で、`GetActiveAuthCode` が有効行を返さなかった時にのみ、その code を対象に `DeleteExpiredAuthCode`(`:72`)を呼ぶ形でしか起きない(lazy eviction)。
- 事実: `db/queries/authcodes.sql:32-40` の `DeleteExpiredAuthCode` は `WHERE code = $1 AND expires_at <= now()` で単一コード対象。テーブル全体を対象にした定期 bulk purge は存在しない(`purge` / `cron` / `scheduled` の grep はヒットせず、削除系は `DeleteExpiredAuthCode` と `ConsumeAuthCode` のみ)。
- 事実: `Consume`(`authcode_repository.go:108-120`)は `DELETE ... RETURNING`(`db/queries/authcodes.sql:42-59`)で消費時に行を削除するが、これは `/token` に到達したコードのみ。到達しないコードは `Save` の INSERT 行が残る。
- 事実: 個々のクエリは PK(`code`)等価検索(`GetActiveAuthCode` / `ConsumeAuthCode` / `DeleteExpiredAuthCode` いずれも `WHERE code = $1`)なので、テーブルが肥大化してもクエリ自体はインデックスで耐える。劣化するのはストレージ量と autovacuum のコスト。
- 事実: `infra/memory.AuthCodeRepository` は同じ lazy eviction 契約だが、プロセス再起動でマップが空になるため恒久蓄積しない。Postgres 化で「再起動による自然リセット」が失われた。
- 仮説: 蓄積ペースは、`/authorize` 発行のうち `/token` に到達しない割合とトラフィックに比例し、長期運用でのみ顕在化する(実測は未実施 = 未調査)。

### 根本原因

SPEC-005 は認可コードの単回使用・TTL セマンティクス(有効なコードのみ `FindByCode` で返す)を SQL で正しく表現することに焦点を当て、「有効でなくなった行の回収」は既存 `infra/memory` と同じ lazy eviction 契約をそのまま踏襲した。memory では再起動で暗黙回収されていたが、永続化により回収されない行が恒久残存するようになった。**テーブル全体を定期的にクリーンアップする実行主体(スケジュールタスク等)が存在しない**ことが根本原因。

## 4. 対応(どう解決するか)

### 対応方針

- **今回のスコープ(SPEC-005 = plan まで / 永続化実装)では対応しない。** `expires_at` を対象にしたテーブル全体の周期的 bulk purge(例: `DELETE FROM authorization_codes WHERE expires_at <= now()`)を、新しい実行主体で回す必要がある。実行主体候補: スケジュール実行の ECS タスク / pg_cron / アプリ内の終了条件付き goroutine ティッカー。いずれも SPEC-005 の範囲(plan まで、runtime 依存は pgx のみ、goose/sqlc)を超え、新しいデプロイ / 運用要素を要するためスコープ外として追跡する。
- 参照: `app/auth/infra/postgres/authcode_repository.go:62-88`(lazy eviction のみの `FindByCode`)、`app/auth/db/queries/authcodes.sql:32-40`(単一コード対象の `DeleteExpiredAuthCode`)、SPEC-005 plan §6.1(review-performance E3)。

### 実施内容(将来対応時のチェックリスト)

- [ ] `expires_at` を対象にしたテーブル全体の bulk purge クエリを追加する(`db/queries` に追記し sqlc 再生成、impl-db)
- [ ] 周期実行の実行主体を決めて実装する(スケジュール ECS タスク / pg_cron / アプリ内ティッカーのいずれか。iac 変更を伴う場合は impl-iac)
- [ ] purge 間隔と、消費済み(= `Consume` で DELETE 済み)以外に `consumed = true` で残る行が無いことを確認する(現状 `Consume` は行削除のため通常 `consumed = true` は残らないが、`GetActiveAuthCode` 経路で `consumed` を更新する設計変更時は要注意)
- [ ] `authorization_codes(expires_at)` にインデックスを付与するか、bulk purge のコストを評価する
- [ ] purge が動かない / 遅延した場合のテーブルサイズ上限・監視方針を決める

### 再発防止

- 永続化する短命エンティティ(TTL を持つ行)は、lazy eviction だけでなく「到達しないケースを回収する定期 purge」をセットで設計する、を `.claude/rules/db.md` の永続化設計チェックに加えることを検討する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-09

- 起票。SPEC-005(app/auth の Postgres 永続化)のレビュー(review-performance、E3)で Major として挙がり「今回は対応せず追跡する」と判断された、認可コードの無制限増加を記録。
- 事実確認: 期限切れ削除は `app/auth/infra/postgres/authcode_repository.go:62-88` の `FindByCode` 内で当該 code に対して `DeleteExpiredAuthCode`(`db/queries/authcodes.sql:32-40`)を呼ぶ lazy eviction のみ。テーブル全体の定期 purge は存在しない(`purge` / `cron` / `scheduled` の grep なし)。`/token` に到達しないコードは `Consume` されず、対応する `FindByCode` も来ないため恒久残存する。`infra/memory` は再起動でマップがリセットされ自然回収されていた劣化パス。
- severity は **medium** と判定。判定根拠: 個々のクエリは PK 等価検索でテーブル肥大化に耐え(個別レイテンシへの即時影響なし)、機能は失われず、手動 / 定期 `DELETE ... WHERE expires_at <= now()` という回避策が存在する(= medium)一方、ストレージ・autovacuum コストが無制限に単調増加する累積的劣化で軽微(low)には収まらないため。review-performance は Major と評価。
- 次にやること: 将来 planner が周期 purge の実行主体(スケジュール ECS タスク / pg_cron / アプリ内ティッカー)を確定し、impl-db(+ 必要なら impl-iac)が実装、tester / checker / review を通す。
