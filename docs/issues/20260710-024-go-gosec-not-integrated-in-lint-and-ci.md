---
id: ISSUE-024
title: gosec がプロジェクトの lint / CI に組み込まれておらず(.golangci.yml 不在、golangci-lint はデフォルト linter のみ)、既存 gosec 由来 Issue(G704 / G112 / G710)の再現・回帰検出ができない
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-10
specs: []  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-024: gosec がプロジェクトの lint / CI に組み込まれておらず、既存 gosec 由来 Issue(G704 / G112 / G710)の再現・回帰検出ができない

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/api / app/auth / app/migrator のセキュリティ品質を担保する開発者・レビュー担当** の **「セキュリティ静的解析(gosec)の指摘が再現でき、修正後に回帰しないことを CI で継続検証できる」というプロセスの信頼性** が、**gosec がリポジトリの lint / CI にどこにも組み込まれておらず(`.golangci.yml` 不在、`golangci-lint run ./...` はデフォルト linter のみで gosec 系ルールを含まない)、gosec 由来として起票された既存 Issue(G704 / G112 / G710)の再現・回帰検出ができないことで損なわれている**。

- **影響を受けるユーザー**: app/api / app/auth / app/migrator のセキュリティレビュー担当・実装者。gosec 指摘に対処する / 抑制する担当者
- **損なわれる価値**: セキュリティ静的解析の再現可能性・監査可能性・回帰防止。gosec 指摘に対する修正(緩和 or `#nosec` 抑制)が「本当に解消したか」「以後再発しないか」を CI で検証できない
- **影響範囲・頻度**: 常時(gosec を回す設定・経路がそもそも存在しない)。app/api・app/auth・app/migrator の Go 3 スタック横断。ただし現時点でランタイムの脆弱性が新たに生じているわけではなく、**プロセス / 再現性の欠如**である
- **回避策**: あり(担当者がローカルで `gosec ./...` を手動実行する)。ただし手動のため再現性・強制力が無く、CI での回帰検出はできない

## 2. 現象(何が起きているか)

### 期待する動作

gosec のセキュリティルール(G704 SSRF / G112 Slowloris / G710 オープンリダイレクト等)が、リポジトリの lint / CI の一部として実行され、指摘が再現でき、修正 / 抑制後は CI で回帰検出される。既存の gosec 由来 Issue(ISSUE-021 / ISSUE-010 / ISSUE-004)が指す gosec 指摘を、誰でも同じ手順で再現・確認できる。

### 実際の動作

gosec を実行する設定・経路がリポジトリのどこにも無い。

- `.golangci.yml`(および `app/api` / `app/auth` / `app/migrator` 配下の同等ファイル)が **存在しない**。golangci-lint は設定ファイルが無いとデフォルト有効 linter のみを実行し、**gosec は golangci-lint のデフォルト linter に含まれない**(明示有効化が必要)。
- `make lint` の実体は各 Go スタックの Makefile の `golangci-lint run ./...`(`app/api/Makefile:76-77` / `app/auth/Makefile:66-67` / `app/migrator/Makefile:37-38`)で、設定ファイルが無いため gosec 系ルールを一切実行しない。
- `.github/workflows/cicd.yml` は golangci-lint を install して実行するが(`:153-155` / `:185-187` / `:336-338` 他)、これも `.golangci.yml` が無いためデフォルト linter のみで、gosec を含まない。gosec 単体を実行するステップも無い。
- にもかかわらず、既存 Issue は gosec 指摘を根拠に起票されている: ISSUE-021(healthcheck の `client.Get` に **G704** SSRF)、ISSUE-010(`http.Server` タイムアウト未設定に **G112** Slowloris、2026-07-10 追記)、ISSUE-004(`http.Redirect` に **G710** オープンリダイレクト相当)。これらの指摘を出したツール・設定がリポジトリに残っていないため、第三者が同じ結果を再現できない。

### 再現手順

1. `.golangci.yml`(および `app/api/.golangci.yml` / `app/auth/.golangci.yml` / `app/migrator/.golangci.yml`)がリポジトリに存在しないことを確認する(`ls` で No such file)。
2. `app/api/Makefile:76-77`(および `app/auth/Makefile:66-67` / `app/migrator/Makefile:37-38`)の `lint` ターゲットが `golangci-lint run ./...` のみで、gosec を有効化する設定・引数を持たないことを確認する。
3. `.github/workflows/cicd.yml` の golangci-lint 実行(`:153-155` 他)に gosec を有効化する設定・gosec 単体ステップが無いことを確認する。
4. `cd app/api && make lint`(または `golangci-lint run ./...`)を実行しても、gosec ルール(G704 / G112 / G710 等)が一切報告されないことを確認する(デフォルト linter のみ)。
5. 対比: 既存 Issue(ISSUE-021 / ISSUE-010 / ISSUE-004)は gosec の G704 / G112 / G710 を根拠に起票されているのに、それを出す設定・ツールがリポジトリに無いことを確認する。

### 環境・条件

- 対象 stack: app/api・app/auth・app/migrator(Go)横断。lint(各スタックの Makefile)と CI(`.github/workflows/cicd.yml`)。
- 発見文脈: プロジェクト全体レビューで、gosec 由来 Issue が複数ある一方でリポジトリに gosec の設定・実行経路が無いことが確認された。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `.golangci.yml` はリポジトリルート・各 Go スタック(app/api / app/auth / app/migrator)配下のいずれにも存在しない。
- 事実: `make lint` = `golangci-lint run ./...`(`app/api/Makefile:76-77`、`app/auth/Makefile:66-67`、`app/migrator/Makefile:37-38`)。golangci-lint は設定ファイルが無いとデフォルトの有効 linter セットのみを実行し、gosec はそのデフォルトに含まれない(明示的な `linters.enable` が必要)。
- 事実: `.github/workflows/cicd.yml` は複数箇所で golangci-lint を install / 実行するが(`:153-155`、`:185-187`、`:336-338`)、`.golangci.yml` が無いためデフォルト linter のみ。gosec 単体を実行するステップも無い。
- 事実: 既存 Issue は gosec 指摘を根拠にしている — ISSUE-021 は healthcheck の `client.Get(url)` に **gosec G704**(SSRF)、ISSUE-010 は `http.Server` タイムアウト未設定に対する **gosec G112**(Potential Slowloris Attack、2026-07-10 追記)、ISSUE-004 は `http.Redirect` にユーザー由来値が渡る経路に対する **gosec G710**(オープンリダイレクト)相当。
- 事実: これらの gosec 指摘は「env 集約リファクタ中の review-security パス」等でレビュー agent が検出したものとして記録されている(ISSUE-021 の起票文脈等)。つまり **レビュー時にアドホックに gosec を回した結果**が起票され、その実行設定はリポジトリに永続化されていない。
- 仮説: レビュー agent が手元で `gosec` を単発実行した結果を起票に用いたが、その実行手段を lint / CI の恒久設定として残す作業がスコープに含まれていなかった。

### 根本原因

gosec のセキュリティ静的解析が、リポジトリの品質ゲート(`make lint` / CI)に恒久的に組み込まれていない。golangci-lint を使う土台はあるが、gosec を有効化する `.golangci.yml` が無く、デフォルト linter のみで運用されている。その結果、レビューでアドホックに検出された gosec 指摘(G704 / G112 / G710)を、同じ設定で再現したり、修正 / 抑制後に回帰しないことを CI で継続検証したりできない。

## 4. 対応(どう解決するか)

### 対応方針

- **gosec を Go 3 スタック(app/api / app/auth / app/migrator)の lint / CI に恒久的に組み込む。** 以下のいずれか(または組み合わせ):
  - **golangci-lint の gosec linter を有効化する**(推奨。既存の `golangci-lint run ./...` 経路にそのまま乗る)。リポジトリに `.golangci.yml` を追加し `linters.enable` に `gosec` を含める。3 スタックで共通設定を共有するか、スタックごとに置くかを決める(既存の Makefile が各スタック基点で `golangci-lint run ./...` を実行する構成に合わせる)。
  - **gosec を単体で `make lint` / CI に追加する**(`gosec ./...`)。golangci-lint とは別ツールとして回す構成。
- 既存の gosec 由来 Issue(ISSUE-021 G704 / ISSUE-010 G112 / ISSUE-004 G710)の指摘が、組み込み後の gosec で再現することを確認し、各 Issue の対応(緩和 or `#nosec` + 根拠コメントでの抑制)がこの恒久設定下で「解消 / 抑制済み」と判定できる状態にする。
- 担当: 設定ファイル / lint 経路の追加は Go 品質ツーリング横断のため impl-ci(CI/CD・リポジトリツーリング)を軸に、各スタックの Makefile 調整が要る場合は impl-api / impl-auth / impl-db と分担する(概念で担当を切る)。
- 参照: `app/api/Makefile:76-77`、`app/auth/Makefile:66-67`、`app/migrator/Makefile:37-38`、`.github/workflows/cicd.yml:153-155`(golangci-lint install/run)、既存 gosec 由来 Issue: ISSUE-021 / ISSUE-010 / ISSUE-004。

### 実施内容

- [ ] gosec 有効化方式を決める(golangci-lint の `.golangci.yml` で gosec を enable / gosec 単体を lint・CI に追加)
- [ ] app/api・app/auth・app/migrator の 3 スタックで gosec が実行される状態にする(設定の共有 or スタック別配置を決定)
- [ ] `make lint` および `.github/workflows/cicd.yml` で gosec が回ることを確認する
- [ ] 既存の gosec 由来 Issue(ISSUE-021 G704 / ISSUE-010 G112 / ISSUE-004 G710)の指摘が新設定で再現し、各 Issue の対応が「解消 / 抑制済み」と判定できることを確認する
- [ ] `#nosec` による抑制を許す場合、抑制には必ず理由コメントを付ける運用(ISSUE-021 の方針と整合)を明記する

### 再発防止

- セキュリティ静的解析(gosec 等)は「レビューでアドホックに回して指摘する」だけで終わらせず、lint / CI の恒久設定として残し、指摘の再現と回帰検出を CI で担保することを、セキュリティレビューのプロセス規約(`.claude/rules/` の該当箇所)に明記することを検討する。
- gosec 指摘を根拠に起票する際は、それを出したツール・設定がリポジトリに恒久的に存在するかを起票時に確認する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。プロジェクト全体レビューで「gosec 由来の Issue(G704 / G112 / G710)が複数あるのに、gosec を実行する設定・経路がリポジトリに無い」という再現性・回帰検出の欠如を記録。
- 事実確認: `.golangci.yml`(ルート / app/api / app/auth / app/migrator 配下いずれも)不在。`make lint` = `golangci-lint run ./...`(`app/api/Makefile:76-77`、`app/auth/Makefile:66-67`、`app/migrator/Makefile:37-38`)で、設定不在ゆえデフォルト linter のみ(gosec 非含有)。`.github/workflows/cicd.yml` は golangci-lint を install / 実行するが(`:153-155` 他)同様にデフォルトのみ、gosec 単体ステップも無い。既存 Issue の gosec 由来: ISSUE-021(G704 SSRF)、ISSUE-010(G112 Slowloris、2026-07-10 追記)、ISSUE-004(G710 オープンリダイレクト相当)。
- 重複確認: `docs/issues` を横断し、「gosec を lint / CI に組み込む」というプロセス課題を扱う既存 Issue は無いことを確認。ISSUE-021 / ISSUE-010 / ISSUE-004 はいずれも **個別の gosec 指摘への対応**(SSRF / Slowloris / オープンリダイレクト)であり、本 Issue は **それらを再現・回帰検出するための解析ツール自体の組み込み** という上位のプロセス課題で、対象が異なる(相互参照はするが重複ではない)。
- severity は **medium** と判定。判定根拠: 現時点でランタイムの新たな脆弱性が生じているわけではなく、手動 `gosec ./...` という回避策もある(= critical/high ではない)。一方で、認証基盤を含む Go 3 スタックのセキュリティ静的解析が品質ゲートに恒久的に組み込まれておらず、既存のセキュリティ指摘を再現・回帰検出できないという、軽微(low)には収まらないプロセス / 再現性の欠如のため medium。
- 相互リンク: 直接ひもづく Spec は無いため frontmatter `specs` は空。既存 gosec 由来 Issue(ISSUE-021 / ISSUE-010 / ISSUE-004)と本文で相互参照する。
- 次にやること: planner が gosec の組み込み方式(golangci-lint の gosec 有効化 / gosec 単体)を確定し、impl-ci を軸に(必要に応じ impl-api / impl-auth / impl-db と分担して)3 スタックの lint / CI に組み込み、既存 gosec 由来 Issue の再現・回帰検出ができることを確認する。
</content>
