---
id: ISSUE-028
title: "SPEC-013: DB 一本化テストの実行時間・fail-fast 最適化余地(将来検討)"
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-11
updated: 2026-07-11
specs: [SPEC-013]  # 関連Spec ID
---

# ISSUE-028: SPEC-013: DB 一本化テストの実行時間・fail-fast 最適化余地(将来検討)

> **注記**: これは現時点の不具合ではなく、**既知トレードオフ / 将来の最適化候補**の記録である。以下 3 点はいずれも SPEC-013(テストの実 DB 一本化)の非機能要件「実 DB 化に伴うテスト時間増は許容する」(`docs/specs/20260711-013-unify-tests-real-db-test-databases.md` §3 非機能要件)の範囲内で、実装計画 `docs/plans/SPEC-013-plan.md` §6 が既知トレードオフとして記載済み。SPEC-013 T8 のパフォーマンスレビュー(review-performance)で「今回は対応しない・将来の最適化余地」として挙がったものを、将来の最適化時の起点として 1 件に集約する。

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/api・app/auth を開発・レビューする人と subagent(tester / impl-* / review-*)、および CI / pre-commit** の **テストのフィードバック速度** が **実 DB 一本化に伴い増加した実行時間ぶん、わずかに損なわれている**。

- **影響を受けるユーザー**: app/api・app/auth を開発・レビューする人と subagent、および CI(`cicd.yml`)/ pre-commit hook
- **損なわれる価値**: `make check` / CI / pre-commit の実行時間が増える(下記 §2 の実測どおり)。特に gofmt / go vet 級の即時失敗でも DB provisioning コストを毎回支払うため、**fail-fast(即時失敗で早く止まる)が後退**している
- **影響範囲・頻度**: 常時(`make check` / CI / pre-commit を回すたび)。ただし SPEC-013 が明示的に許容した範囲内であり、**機能・正しさには一切影響しない**(実 DB 検証によりむしろカバレッジは向上している)
- **回避策**: あり(現状の構成でも機能は正しく動作する。最適化は将来検討であり、対応しなくても支障はない)

## 2. 現象(何が起きているか)

本 Issue は不具合報告ではないため、「期待する動作 / 実際の動作」は「理想的に最適化された構成 / 現状の(単純さを優先した)未最適化な構成」の対比として記載する。数値はすべて SPEC-013 T8 の review-performance による実測。

### 期待する動作

DB を必要とするテストのみが DB のコスト(直列化・接続 open/close・provisioning)を負担し、DB 非依存の検証や即時失敗はそのコストを支払わずに済む。

### 実際の動作

#### 現象 1: `go test -p 1 ./...` が DB 非依存パッケージまで一律直列化

- 旧構成は `-p 1`(パッケージ間シリアライズ)を DB 到達パッケージ(`infra/postgres` / `route`)に限定していた。実 DB 一本化(SPEC-013)で `-p 1` を `./...` **全体**へ拡大した(`app/api/Makefile` / `app/auth/Makefile` の `test-native`)。
- **実測**: 空実行(no-op)でも直列化により、api 約 **2.3–2.7 倍**・auth 約 **2 倍**(**+400–650ms**)。増加はパッケージ数に線形。
- `domain/*`・`cmd/*`・`infra/jwt` 等の純ロジックパッケージは DB に触れないため、本来は既定の並列実行でよい。

#### 現象 2: `testsupport.OpenTestDB` の per-test 接続 open/close

- テスト / サブテストごとに `sql.Open` + `Ping` + `Close`(`t.Cleanup`)を行う(既存の per-call 設計を踏襲)。パッケージ単位での `*sql.DB` 共有はしていない。
- **実測**: 約 **1.7ms/回 × 概算 110–150 回 ≈ 全体 +200–300ms**。
- **リーク / コネクション枯渇のリスクは無い**(DB 到達 run を `-p 1` で回し、api/auth に `t.Parallel` が 0 件であることを確認済み。`docs/plans/SPEC-013-plan.md` §1.1 / §6)。純粋にオーバーヘッドの問題。

#### 現象 3: CI / pre-commit の DB provisioning が offline チェックより前に無条件実行 → fail-fast 後退

- CI(`cicd.yml` の `api` / `auth` `check` ジョブ)/ pre-commit は、gofmt / go vet 級の即時失敗でも Postgres 起動 + マイグレーション + ヘルスチェックのコストを**毎回・無条件に**先に支払う。offline チェックが即座に落ちるケースでも DB 起動を待つ。
- 加えて `make migrate-test` は常に api / auth 両方の `-target` を実行するため、単一 stack の job(例: api だけ変更)でも不要な 1 回(auth 側)が混入する。
- **注**: pre-commit hook は SPEC-013 のレビュー後修正で offline / db-test の 2 フェーズに分離済み。ただし本 Issue が指す「DB 準備を offline フェーズより**先に**走らせる」という**順序自体**の最適化は、その分離とは別論点(まだ未対応)。

### 再現手順

いずれも SPEC-013 実装後の app/api・app/auth テスト構成で観測できる。

1. **現象 1**: `app/api`(または `app/auth`)で、実 DB 経路のテストを `go test -p 1 ./...` と `go test ./...`(既定並列)の 2 通りで実行し、壁時計時間を比較する。`-p 1` により全パッケージが直列化されるぶんの増加(上記実測)が観測できる。
2. **現象 2**: `testsupport.OpenTestDB` の呼び出し回数(テスト / サブテスト数 ≈ 110–150 回)× 1 回あたりの `sql.Open`+`Ping`+`Close` コスト(約 1.7ms)として積算される。プロファイル / 計時で確認できる。
3. **現象 3**: CI ログ / pre-commit 実行で、offline チェック(fmt-check / lint / vet / build)が失敗するケースでも、その手前で Postgres 起動 + `make migrate-test`(api / auth 両 target)+ ヘルスチェックが完了するまで待たされることを確認できる。

### 環境・条件

- SPEC-013 実装後の app/api・app/auth のテスト構成(`//go:build integration` 廃止・実 DB 一本化・専用テスト DB `api_test` / `auth_test`)
- SPEC-009 の toolchain コンテナ経路(`docker compose -f .devcontainer/compose.tools.yml`。テストフェーズは `tools-db`)
- 数値は SPEC-013 T8 の review-performance による実測

## 3. 原因(なぜ起きているか)

### 調査ログ

- **事実**: 上記 §2 の数値はすべて SPEC-013 T8 の review-performance による実測報告。
- **事実**: `go test -p 1 ./...` の全体適用は、実装の単純さを優先した選択(`docs/plans/SPEC-013-plan.md` §6「実行時間の増加(許容だが監視)」で「今回は単純さ優先で `go test -p 1 ./...`」と明記済み)。
- **事実**: `OpenTestDB` の per-call open/close は既存設計の踏襲(同 §6 に「接続再利用 … プール共有の余地は review-performance が評価」と記載)。
- **事実**: リーク / 枯渇が無いことは `-p 1` + `t.Parallel` 0 件から確認済み(同 §1.1 / §6)。
- **事実**: 現象 3 の「DB provisioning を offline より先に無条件実行」は、CI / pre-commit の統合(SPEC-013 R5、`docs/plans/SPEC-013-plan.md` §1.4 / §2 impl-ci)で導入された導線。

### 根本原因

3 点とも不具合ではなく、**SPEC-013 実装時に「単純さ優先」で選んだ既知トレードオフ**。SPEC-013 の非機能要件「実行時間増は許容する。ただし接続再利用・軽量な隔離手段でオーバーヘッドを最小化する」の範囲内で、`docs/plans/SPEC-013-plan.md` §6 がリスク / 未確定事項として記載済み。

## 4. 対応(どう解決するか)

### 対応方針

**今回は対応しない。** いずれも SPEC-013 が許容した範囲内の既知トレードオフであり、機能・正しさに影響しない。将来テスト実行時間が問題化した際の最適化候補として本 Issue に集約して記録する。各現象に対する将来案は以下(いずれも review-performance の提案)。

### 実施内容

- [ ] 現象 1: DB 到達パッケージ(`infra/postgres` / `route`)のみ `-p 1` に限定し、`domain/*`・`cmd/*`・`infra/jwt` 等の純ロジックパッケージは既定の並列実行に戻す(旧構成と同様の分割)。
- [ ] 現象 2: パッケージ単位で `*sql.DB` を共有する(`TestMain` で 1 回 `Open`、各サブテストは truncate のみ)。既存の `app/{api,auth}/infra/jwt/signer_verifier_test.go` の `TestMain` パターンと対称に扱える。
- [ ] 現象 3: offline フェーズ(fmt-check / lint / vet / build)を先行させ、失敗時は DB 起動をスキップして fail-fast を回復する。併せて `make migrate-test` を single-target 化し、単一 stack job で不要な target を実行しないようにする。

### 再発防止

該当なし(意図的トレードオフの記録)。将来の最適化に着手する際は本 Issue を起点とし、SPEC-013 の非機能要件「オーバーヘッドを最小化する」への追随として扱う。将来 `t.Parallel()` を導入する場合は、現象 1 / 2 の前提(`-p 1` + truncate 隔離)が破綻するため、`docs/plans/SPEC-013-plan.md` §6「並列単位ごとの別 DB」への移行と併せて検討すること。

## 5. 経緯(時系列・追記のみ)

### 2026-07-11

- 起票。SPEC-013(テストの実 DB 一本化)の T8 パフォーマンスレビュー(review-performance)で「今回は対応しない・将来の最適化余地」として挙がった 3 点 —(1)`go test -p 1 ./...` の全体直列化(空実行でも api 約 2.3–2.7 倍・auth 約 2 倍 / +400–650ms)、(2)`OpenTestDB` の per-test 接続 open/close(約 1.7ms × 概算 110–150 回 ≈ +200–300ms)、(3)CI / pre-commit の DB provisioning が offline チェックより前に無条件実行され fail-fast が後退(+ `migrate-test` の両 target 常時実行)— を 1 件の Issue に集約。
- いずれも SPEC-013 の非機能要件「実行時間増は許容」の範囲内で `docs/plans/SPEC-013-plan.md` §6 が既知トレードオフとして記載済み。**現時点の不具合ではなく、将来の最適化候補**として記録するもの。重要度は low。
- 起点 Spec: SPEC-013(`docs/specs/20260711-013-unify-tests-real-db-test-databases.md`)と相互リンク(本 Issue frontmatter `specs: [SPEC-013]` / Spec 側 frontmatter `issues: [ISSUE-028]`)。
