---
id: ISSUE-029
title: "SPEC-013/SPEC-009: pre-commit hook の offline フェーズが network 有効な tools で実行される非対称性(供給網防御の一貫性)"
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-11
updated: 2026-07-11
specs: [SPEC-013, SPEC-009]  # 関連Spec ID
---

# ISSUE-029: SPEC-013/SPEC-009: pre-commit hook の offline フェーズが network 有効な tools で実行される非対称性(供給網防御の一貫性)

> **注記**: これは現時点で悪用可能な脆弱性ではなく、**供給網防御(SPEC-009)の一貫性強化の将来課題**の記録である。SPEC-013 の Major 修正で「pre-commit hook の `go test` フェーズがインターネットに出る」という Major は既に解消済み。本 Issue は、その修正後に残る **offline フェーズ実行環境の非対称性**(Minor)と、関連する既存挙動(Info)を、SPEC-013 T8 の review-security 再検証で検出したものとして記録する。

## 1. ユーザー価値への影響(なぜ対応するか)

> **この repo の開発者・CI・multi-agent ワークフロー** の **サプライチェーン防御(SPEC-009 R3: 非依存フェーズはインターネットに出さない)の一貫性** が **pre-commit hook 経路の offline フェーズだけ network 有効な `tools` コンテナで実行される点でわずかに損なわれている**。

- **影響を受けるユーザー**: pre-commit hook を使う開発者(供給網防御を期待する立場)。CI・直接 `make check` 経路は影響を受けない
- **損なわれる価値**: SPEC-009 R3「非依存(fmt / lint / vet / build)フェーズは全てオフライン」原則の一貫性。CI・直接 `make check` はこれらを `tools-offline`(`--network none`)で実行するのに対し、**hook 経路の offline フェーズだけ `tools`(network 有効)で走る**という経路間の非対称性が残る
- **影響範囲・頻度**: pre-commit hook で Go DB stack(api / auth)をコミットするたび(特定条件下)。ただし当該フェーズ(fmt-check / lint / vet / build)は**依存の runtime コードを実行しない**(コンパイル・静的解析のみ)ため、実際に悪用可能な経路にはならない
- **回避策**: あり(供給網防御の主目的=**コードを実行する `go test` フェーズの egress 遮断**は SPEC-013 で達成済み。本件は一貫性の問題で、機能・安全性の実害はない)

## 2. 現象(何が起きているか)

本 Issue は不具合報告ではないため、「期待する動作 / 実際の動作」は「SPEC-009 R3 が理想とする一貫した offline 実行 / 現状の(設計妥協による)非対称な実行環境」の対比として記載する。

### 期待する動作(SPEC-009 R3 の原則)

依存の runtime コードを実行しない非依存フェーズ(fmt / lint / vet / build)は、実行経路(CI / 直接 `make check` / pre-commit hook)によらず一貫して `tools-offline`(`--network none`)で実行される。`go test` などコードを実行するフェーズのインターネット非到達も、経路によらず一貫する。

### 実際の動作

#### Minor: hook 経路の offline フェーズが network 有効な `tools` で実行される

- SPEC-013 の Major 修正で、pre-commit hook は 2 フェーズに分離された:
  - offline フェーズ(api / auth の fmt-check / lint / vet / build)
  - db-test フェーズ(`go test`)= `tools-db`(postgres 到達可・**internet 非到達**)
- これにより「**コードを実行する `go test` フェーズがインターネットに出る**」という Major は**解消済み**。
- しかし残る非対称性: **hook 経路の offline フェーズ(api / auth の fmt-check / lint / vet / build)は `tools`(network 有効)で実行される**一方、CI・直接 `make check` は同じ検査を `tools-offline`(`--network none`)で実行する。
- 悪用可能性は低い(fmt / lint / vet / build はコンパイル・静的解析のみで、依存の runtime コードを実行しない)。ただし SPEC-009 R3「非依存フェーズは全てオフライン」原則からの**部分的後退**。
- 原因(下記 §3): hook が `go mod download` の warm を同一コンテナ内で済ませる設計妥協。

#### Info(関連・既存挙動): migrator の hook 経由 test が network 有効な `tools` で走る

- `app/migrator` の hook 経由 `check-native` は `test-native`(`go test ./...`)を含み、network 有効な `tools` 内で実行される。
- これは **SPEC-013 以前からの既存挙動**(今回の SPEC-013 修正で新規発生したものではない)。migrator テストは DB 非依存。
- SPEC-009 R6 の適用範囲を「`go test` 実行**全般**はインターネット非到達」まで広げるなら、migrator の hook 経由 test も `tools-offline` 等へ寄せる見直し余地がある(現状の R6 は api / auth のテストフェーズを対象に緩和・network を絞った経路を用意しており、migrator は対象外のまま `tools`)。

### 再現手順

1. **Minor**: api または auth の Go ソースをステージして pre-commit hook(または `make hook-check`)を実行し、offline フェーズ(fmt-check / lint / vet / build)がどの compose サービスで起動されるかを `.githooks/` の実装で確認する。hook 経路は `tools`(network 有効)、CI(`cicd.yml`)・直接 `make check` は `tools-offline`(`--network none`)であることを対比する。
2. **Info**: `app/migrator` の Go ソースをステージして hook を実行し、`check-native` に含まれる `test-native`(`go test ./...`)が `tools`(network 有効)で走ることを確認する。

### 環境・条件

- SPEC-013 実装後の pre-commit hook 構成(offline / db-test の 2 フェーズ分離済み)
- SPEC-009 の toolchain コンテナ(`.devcontainer/compose.tools.yml` の `tools` / `tools-offline` / `tools-db` の 3 層)
- 検出: SPEC-013 T8 の review-security による再検証(Major「`go test` フェーズが egress」を確認後、Minor / Info を追加検出)

## 3. 原因(なぜ起きているか)

### 調査ログ

- **事実**: SPEC-013 の Major 修正により、hook の `go test` フェーズは `tools-db`(internet 非到達)へ移り、Major「コード実行フェーズが egress」は解消済み(`docs/specs/20260711-013-unify-tests-real-db-test-databases.md` 経緯 / `docs/plans/SPEC-013-plan.md` §1.4)。
- **事実**: hook 経路の offline フェーズ(fmt-check / lint / vet / build)は `tools`(network 有効)で実行される。CI・直接 `make check` は `tools-offline`(`--network none`)で実行する。この経路間の非対称性が Minor の本体。
- **事実**: fmt / lint / vet / build は依存の runtime コードを実行しない(コンパイル・静的解析のみ)。よって当該フェーズが network 有効でも、悪性依存コードの実行経路にはならない。
- **事実**: migrator の hook 経由 `check-native` が network 有効な `tools` で `go test` を含むのは SPEC-013 以前からの既存挙動(今回の修正で新規発生ではない)。migrator テストは DB 非依存。
- **仮説(原因)**: hook が `go mod download` の warm(依存キャッシュの準備)を offline フェーズと同一コンテナで済ませる設計妥協により、当該フェーズを network 有効な `tools` に置いている。

### 根本原因

不具合ではなく、pre-commit hook の実装が **依存 warm の簡便さを優先**し、offline フェーズを network 有効な `tools` に同居させている設計妥協。SPEC-009 R3 の理想(非依存フェーズ全 offline)と、hook 経路の実装との間の一貫性ギャップ。SPEC-013 の主目的(test フェーズの egress 遮断)は達成済みで、本件はその副産物として残った一貫性課題。

## 4. 対応(どう解決するか)

### 対応方針

**今回は対応しない。** 悪用にはコードを実行するフェーズが必要で、当該 offline フェーズ(fmt / lint / vet / build)はコードを実行しない。SPEC-013 の主目的(test フェーズの egress 遮断)は達成済み。供給網防御の一貫性強化の将来課題として本 Issue に記録する。将来案は以下。

### 実施内容

- [ ] Minor: hook の offline フェーズ内で依存 warm(`go mod download`)を済ませた後、`tools-offline`(no-network)へさらに切り替えて fmt-check / lint / vet / build を実行する。あるいは warm 専用フェーズを分離し、検査本体は一貫して `tools-offline` で回す。
- [ ] Info: SPEC-009 R6 の適用範囲を「`go test` 実行**全般**はインターネット非到達」まで広げる方針を採るなら、`app/migrator` の hook 経由 test(DB 非依存)も `tools-offline` 等へ寄せる(R6 の見直しと併せて判断)。

### 再発防止

該当なし(意図的な設計妥協の記録)。将来 SPEC-009 R3 / R6 の適用範囲を厳格化・見直しする際は、CI / 直接 `make check` / pre-commit hook の 3 経路で「非依存フェーズ = offline」「コード実行フェーズ = internet 非到達」が一貫することをチェックリスト化する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-11

- 起票。SPEC-013 の Major 修正(pre-commit hook を offline / db-test の 2 フェーズに分離し、`go test` フェーズを `tools-db`=internet 非到達へ移して Major「コード実行フェーズが egress」を解消)後に残る一貫性課題を、T8 の review-security 再検証が検出したものとして記録。
- **Minor**: hook 経路の offline フェーズ(api / auth の fmt-check / lint / vet / build)が `tools`(network 有効)で実行される一方、CI・直接 `make check` は `tools-offline`(`--network none`)で実行する非対称性。fmt / lint / vet / build は依存 runtime コードを実行しないため悪用可能性は低いが、SPEC-009 R3「非依存フェーズは全てオフライン」からの部分的後退。原因は hook が `go mod download` warm を同一コンテナで済ませる設計妥協。将来案: warm 後に `tools-offline` へ切替、または warm 専用フェーズ分離。
- **関連 Info**: `app/migrator` の hook 経由 `check-native` が `test-native`(`go test ./...`)を network 有効な `tools` で実行(SPEC-013 以前からの既存挙動・migrator テストは DB 非依存)。R6 を「`go test` 全般は internet 非到達」へ広げるなら migrator も `tools-offline` へ寄せる見直し余地。
- severity **low**(悪用にはコード実行フェーズが必要で当該 offline フェーズはコードを実行しない。SPEC-013 の主目的=test フェーズの egress 遮断は達成済み)。今回は対応せず、供給網防御の一貫性強化の将来課題として記録。
- 起点: SPEC-013(本 SPEC の Major 修正の副産物)/ SPEC-009(R3 のオフライン原則)と相互リンク(本 Issue frontmatter `specs: [SPEC-013, SPEC-009]` / 各 Spec 側 frontmatter `issues` に ISSUE-029 追記)。
