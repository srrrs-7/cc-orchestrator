---
id: ISSUE-027
title: Go を 1.26 に bump(全スタック横断・pin 箇所が ~8 箇所に分散)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-10
specs: [SPEC-009]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-027: Go を 1.26 に bump(全スタック横断・pin 箇所が ~8 箇所に分散)

## 1. ユーザー価値への影響(なぜ対応するか)

> **開発者・CI・保守者** の **「サポート中の Go で開発・ビルドし、言語 / 標準ライブラリ / toolchain のセキュリティ修正を受け取れる」価値** が、**旧バージョン(1.24)に固定されたままで徐々に損なわれつつある**。

- **影響を受けるユーザー**: この repo を開発・保守する人、CI、および `make check` / `make build` / `make test` を実行する subagent(checker / tester / impl-*)。**エンドユーザー(app/web / app/api / app/auth の利用者)への直接影響は現時点でなし**(1.24 で正常にビルド・稼働している)
- **損なわれる価値**: 新しい言語機能 / 標準ライブラリ改善 / go toolchain・stdlib のセキュリティ修正を受け取れない。また後述のとおり sqlc(Go≥1.26 要求)を入れるための回避策を toolchain イメージに抱え続ける負債がある
- **影響範囲・頻度**: 常時(ビルド / CI / ローカル開発の全経路)。ただし機能破壊は現状なし(**予防的なアップグレード / 技術的負債の解消**であり不具合ではない)
- **回避策**: あり(1.24 のまま運用継続)。ただし将来の Go リリースサイクルで 1.24 がサポート外になれば value 低下は進む

## 2. 現象(何が起きているか)

### 期待する動作

- Go のバージョン pin が**単一ソース(`versions.env` の `GO_VERSION`)から一貫して駆動**され、1 箇所の編集で全スタックが追随する
- 全スタック(api / auth / migrator / toolchain / app ビルドイメージ)が Go 1.26 でビルド・lint・test を通す

### 実際の動作

- Go の pin は現状すべて `1.24` で**値は一貫している**が、`versions.env` だけでは駆動されず **~8 箇所に独立して存在**する。bump は各所を手編集する必要がある
- SPEC-009 の toolchain イメージは、sqlc(その go.mod が `go >= 1.26.0` を要求)を入れるために `GOTOOLCHAIN=auto` の回避策で go1.26.x を都度取得している(`docker/toolchain/Dockerfile` の sqlc install ステップ)。base が 1.24 であることに起因する負債

### 再現手順

（不具合ではなく分散状態の確認手順。第三者が現状を再現・検証できる形で記載）

1. リポジトリルートで以下を実行し、Go 1.24 の pin 箇所を列挙する:
   ```
   grep -rnE '1\.24|GO_VERSION' --include=Dockerfile --include=go.mod --include='*.env' . | grep -iE '1\.24|GO_VERSION'
   ```
2. `versions.env` の `GO_VERSION` を変えても、`app/{api,auth,migrator}/go.mod` の `go 1.24` 行・各 app `Dockerfile` の `FROM golang:1.24-alpine`・`docker/toolchain/Dockerfile` の `ARG GO_VERSION=1.24` 既定値が連動しないことを確認する(go.mod と app Dockerfile は versions.env 非連動、toolchain 既定値は build-arg で上書きされる限りにおいてのみ連動)

### 環境・条件

- 対象コミット: 現行 HEAD(本 Issue 起票時点)
- SPEC-009(コンテナ化 toolchain)適用後の構成。toolchain 実行環境は `docker/toolchain/Dockerfile`(`FROM golang:${GO_VERSION}-bookworm`)、app ビルドは各 `Dockerfile`(`FROM golang:1.24-alpine`)

## 3. 原因(なぜ起きているか)

### 調査ログ

- 確認したこと(いずれも事実。起票時に repo を grep / read で検証済み):
  - `versions.env:32` に `GO_VERSION=1.24`。SPEC-009 の単一ソースで、**toolchain イメージ(build-arg 経由)+ `compose.tools.yml`(`compose.tools.yml:84` `GO_VERSION: ${GO_VERSION}`)+ CI(`versions.env` を `$GITHUB_ENV` にロード)+ `.github/actions/build-toolchain-image/action.yml:83`** を駆動する
  - `app/api/go.mod:3` / `app/auth/go.mod:3` / `app/migrator/go.mod:3` はいずれも `go 1.24`。**toolchain directive(`toolchain go1.xx`)行は無し**。各 go.mod は `versions.env` と非連動(手編集が必要)
  - `app/api/Dockerfile:6` / `app/auth/Dockerfile:6` / `app/migrator/Dockerfile:22` はいずれも `FROM golang:1.24-alpine AS build`。app ビルドイメージで SPEC-009 の toolchain スコープ外(手編集が必要)。各 Dockerfile 冒頭コメント(例 `app/api/Dockerfile:2,4`)にも「Go 1.24 on Alpine」と記述があり、併せて更新対象
  - `docker/toolchain/Dockerfile:65` に `ARG GO_VERSION=1.24`(`:66` で `FROM golang:${GO_VERSION}-bookworm`)。実ビルドは `compose.tools.yml` の build-arg(`versions.env` 由来)で上書きされるが、**素の `docker build` と Dependabot はこの既定値を使う**(`.github/dependabot.yml:82-83` がこの `ARG GO_VERSION=1.24` 既定を base image 追跡の起点として参照)ので、既定値も 1.26 へ合わせる
  - `docker/toolchain/Dockerfile` の sqlc install ステップ(sqlc の go.mod が `go >= 1.26.0` を要求)は、base の `golang:*-bookworm` が `GOTOOLCHAIN=local`(auto-download 無効)なため、その 1 ステップだけ `GOTOOLCHAIN=auto` に切り替えて go1.26.x を取得している(コメントに詳細)
- 仮説(未検証。§4「確認事項」で検証する):
  - 仮説: base を go1.26 にすれば、sqlc install の `GOTOOLCHAIN=auto` 回避策は不要になり撤去できる(要検証)

### 根本原因

- Go の pin が単一ソース化されておらず、`versions.env`(SPEC-009 の toolchain スコープ)/ 各 `go.mod`(モジュール宣言)/ 各 app `Dockerfile`(ランタイムビルドイメージ)/ toolchain の `ARG` 既定値、という**役割の異なる 4 系統に分散**しているため。これは各値の性質上(go.mod のバージョンは Go ツール自身が読む宣言で env 展開不可、app Dockerfile は SPEC-009 のスコープ外)ある程度は不可避で、bump は横断編集になる

## 4. 対応(どう解決するか)

### 対応方針

全 pin を **1.24 → 1.26** へ横断で bump する。stack 境界に沿って担当を分割し、変更後に build / lint / test で検証する。**最大の不確定要素は golangci-lint の Go 1.26 対応可否**で、これを先に確認してから着手すること。

**bump 対象(現状値はいずれも 1.24 → 1.26):**

| 対象ファイル | 箇所 | 担当 |
|---|---|---|
| `versions.env` | `GO_VERSION=1.24`(SPEC-009 単一ソース。toolchain + compose + CI を駆動) | impl-ci |
| `docker/toolchain/Dockerfile` | `ARG GO_VERSION=1.24` 既定値(build-arg 上書き時も素の docker build / Dependabot が使う) | impl-ci |
| `app/api/go.mod` | `go 1.24` 行 + `app/api/Dockerfile` の `FROM golang:1.24-alpine`(+ 冒頭コメント) | impl-api |
| `app/auth/go.mod` | `go 1.24` 行 + `app/auth/Dockerfile` の `FROM golang:1.24-alpine`(+ 冒頭コメント) | impl-auth |
| `app/migrator/go.mod` | `go 1.24` 行 + `app/migrator/Dockerfile` の `FROM golang:1.24-alpine`(+ 冒頭コメント) | impl-db(migrator 所有) |
| `.github/dependabot.yml` | `ARG GO_VERSION=1.24` を参照するコメント(`:82-83`)の追随 | impl-ci |

- 実行体制(担当分担):
  - `versions.env` / `docker/toolchain/Dockerfile` / compose 系 / dependabot コメント = **impl-ci**
  - `app/api/go.mod` + `app/api/Dockerfile` = **impl-api**
  - `app/auth/go.mod` + `app/auth/Dockerfile` = **impl-auth**
  - `app/migrator/go.mod` + `app/migrator/Dockerfile` = **impl-db**(migrator は impl-db 所有)
  - 検証(build / lint / test)= **checker / tester**
- 進め方の推奨: まず「確認事項」の golangci-lint 対応可否を確定 → toolchain イメージ(impl-ci)を先に更新 → 各 stack の go.mod / Dockerfile(impl-api / auth / db を並列)→ checker / tester で全スタック検証。planner に実装計画作成を委ねてもよい

### 確認事項(着手前に検証。§ リスク)

- **[最大の不確定要素] golangci-lint 2.12.2 が Go 1.26 を解析可能か未確認**(`versions.env:GOLANGCI_LINT_VERSION=2.12.2`)。golangci-lint は内部で Go の型チェッカ / SSA を使うため、**言語バージョンが解析器より新しいと lint が失敗しうる**。非対応なら `GOLANGCI_LINT_VERSION` も同時に bump が必要。**この可否確認が本 Issue 最大のリスク**であり、着手前に確定させる
- **sqlc の `GOTOOLCHAIN=auto` 回避策の撤去可否**。sqlc は既に Go≥1.26 を要求しており、現状 `docker/toolchain/Dockerfile` の sqlc install ステップで `GOTOOLCHAIN=auto` を使って go1.26.x を都度取得している。base を 1.26 にすればこの回避策を撤去できる**可能性**があるが、要検証(撤去は必須ではない。撤去できれば toolchain の負債が減る)
- **`GOIMPORTS_VERSION=v0.42.0` の更新余地**。これは go1.24 に合わせた pin(v0.43.0 以降は `go 1.25.0` を要求、と `versions.env` のコメントに記載)。1.26 化後は新しい goimports へ更新余地があるが**必須ではない**(現状 pin のままでも動作する。任意対応)

### 実施内容

- [ ] golangci-lint 2.12.2 の Go 1.26 対応可否を確認(非対応なら `GOLANGCI_LINT_VERSION` bump をスコープに追加)
- [ ] impl-ci: `versions.env` の `GO_VERSION` を 1.26 へ / `docker/toolchain/Dockerfile` の `ARG GO_VERSION` 既定値を 1.26 へ / dependabot コメント追随 / (可能なら)sqlc の `GOTOOLCHAIN=auto` 回避策の撤去可否を検証
- [ ] impl-api: `app/api/go.mod` の `go` 行 + `app/api/Dockerfile` の `FROM golang:...-alpine`(+ コメント)を 1.26 へ
- [ ] impl-auth: `app/auth/go.mod` の `go` 行 + `app/auth/Dockerfile` の `FROM golang:...-alpine`(+ コメント)を 1.26 へ
- [ ] impl-db: `app/migrator/go.mod` の `go` 行 + `app/migrator/Dockerfile` の `FROM golang:...-alpine`(+ コメント)を 1.26 へ
- [ ] checker / tester: 全 Go スタックで `make check`(fmt-check + lint + vet + build + test)/ 必要に応じ `make test-integration` を通す。toolchain イメージ再ビルドを含む
- [ ] (任意)`GOIMPORTS_VERSION` の更新可否を判断

### 再発防止

- pin の分散状態自体は各値の役割上ある程度不可避だが、**bump 時に横断編集が必要な箇所の一覧(本 Issue の対応方針テーブル)を参照点として残す**。将来 `go.mod` へ `toolchain go1.xx` directive を導入するかは別途検討(現状 directive 行は無し)

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。プロジェクトの Go バージョン currency 見直しにあたり、Go 1.24 の pin が単一ソース化されておらず bump に横断編集が要る点を課題として起票。起票にあたり repo を grep / read で検証し、bump 対象(`versions.env` / 各 `go.mod` / 各 app `Dockerfile` / `docker/toolchain/Dockerfile` の `ARG` 既定値 / dependabot コメント)と現状値(すべて 1.24)、および 3 つの確認事項(golangci-lint 2.12.2 の 1.26 対応可否=最大の不確定要素 / sqlc の `GOTOOLCHAIN=auto` 回避策撤去可否 / goimports pin の更新余地)を確認・記載。severity は **low**(現状 1.24 で正常稼働し機能破壊なし、予防的アップグレード / 技術的負債の解消のため)と判定。SPEC-009(コンテナ化 toolchain。`versions.env` / `docker/toolchain` の所有 Spec)と相互リンク。実装は本 Issue では未着手(ドキュメント作成のみ)。
