---
id: ISSUE-055
title: CI の build-toolchain-image action が versions.env をコメント行ごと $GITHUB_ENV に流し全 make ジョブが失敗
status: resolved
severity: critical
created: 2026-07-13
updated: 2026-07-13
specs: [SPEC-009]
---

# ISSUE-055: CI の build-toolchain-image action が versions.env をコメント行ごと $GITHUB_ENV に流し全 make ジョブが失敗

## 1. ユーザー価値への影響(なぜ対応するか)

> **この repo で PR / push を出す開発者・multi-agent ワークフロー** の **CI による品質ゲート(check / test / contract-drift / sqlc-drift)** が **全ジョブの「Build toolchain image」ステップで即失敗し、一切通らない状態**になっている。

- **影響を受けるユーザー**: PR / push を出す全開発者、および CI green を前提に検収する admin / subagent ワークフロー
- **損なわれる価値**: CI が全 `make <target>` ジョブで機能せず、変更の自動検証が得られない。green を要件とするマージ運用が止まる
- **影響範囲・頻度**: 常時。`.github/actions/build-toolchain-image/action.yml` を使う `cicd.yml` / `contract-drift.yml` / `sqlc-drift.yml` の該当ジョブすべて(全 stack)
- **回避策**: なし(CI 経路。手元 `make check` は toolchain コンテナで通るが、この action を経由しないため症状は出ない)

## 2. 現象(何が起きているか)

### 期待する動作

`.github/actions/build-toolchain-image/action.yml` の「Load versions.env」step が `.devcontainer/versions.env` の `KEY=value` 行だけを `$GITHUB_ENV` に流し込み、後続の「Build (cache-from/to: gha)」step が `${{ env.GO_VERSION }}` 等を build-args として参照できる。

### 実際の動作

「Load versions.env」step が次のエラーで失敗し、ジョブが中断する:

```
Error: Unable to process file command 'env' successfully.
Error: Invalid format '# versions.env — single source of truth for pinned tool / runtime'
```

エラーメッセージ中の文字列 `# versions.env — single source of truth for pinned tool / runtime` は `.devcontainer/versions.env` の 1 行目(コメント)そのもの。

### 再現手順

1. `.github/actions/build-toolchain-image/action.yml` を使う任意のジョブ(例: `cicd.yml` の web / api / auth / iac などの check job)を GitHub Actions 上で走らせる
2. 「Build toolchain image」→「Load versions.env」step で上記エラーが出てジョブが失敗する

### 環境・条件

- GitHub Actions runner(GitHub-hosted)。`$GITHUB_ENV` ファイルコマンドを使う環境全般
- SPEC-009 Phase C で導入された共有 composite action 経路

## 3. 原因(なぜ起きているか)

### 調査ログ

- 確認したこと(事実): `action.yml` の「Load versions.env」step(56 行目)は次を実行する:

  ```
  run: cat "${{ github.workspace }}/.devcontainer/versions.env" >> "$GITHUB_ENV"
  ```

- 確認したこと(事実): GitHub Actions runner の `$GITHUB_ENV` ファイルコマンドは、追記される各行を `KEY=value`(または heredoc の `KEY<<EOF` 形式)として解釈する。コメント行(`# ...`)・空行は不正な行として `Invalid format` を返す。
- 確認したこと(事実): `.devcontainer/versions.env` は 1 行目からコメントブロックで始まり、途中にも空行・コメント行(例: 21・37・57 行目付近)を含む。`cat` で全行を `$GITHUB_ENV` へ流すため、最初のコメント行(1 行目)で即エラーになる。エラーメッセージの文字列は 1 行目と一致する。
- 確認したこと(事実・付随): `.devcontainer/versions.env` のヘッダーコメント 7 行目が消費方法として `#   - GitHub Actions:     cat .devcontainer/versions.env >> "$GITHUB_ENV"` を記載しており、これは実際には動作しない(ドキュメントの誤り)。action.yml の実装はこの誤った案内どおりに書かれている。

### 根本原因

`cat versions.env >> "$GITHUB_ENV"` が、`$GITHUB_ENV` が受け付けない**コメント行・空行を含むファイル全体**をそのまま流し込んでいること。`$GITHUB_ENV` は `KEY=value` 行のみを受け付けるため、コメント/空行を除外せずに追記した時点で `Invalid format` になる。`versions.env` のヘッダーコメント(7 行目)が、この動かない消費方法を「正しい方法」としてドキュメント化していたことが実装ミスを誘発した。

## 4. 対応(どう解決するか)

### 対応方針

- composite action 側の「Load versions.env」step を、`KEY=value` 行のみ抽出して `$GITHUB_ENV` に追記する形へ修正する(例: `grep -E '^[A-Za-z_][A-Za-z0-9_]*=' "${{ github.workspace }}/.devcontainer/versions.env" >> "$GITHUB_ENV"`)。コメント行・空行が入っても壊れないようにする。
- `.devcontainer/versions.env` のヘッダーコメント(7 行目)の GitHub Actions 消費例を、実際に動く形(`grep` フィルタ付き)へ追随修正する。
- 修正は impl-ci が担当(action.yml は `.github/` 配下・横断ツーリングのため impl-ci の所有。`.devcontainer/versions.env` も同様)。

### 実施内容

- [x] `.github/actions/build-toolchain-image/action.yml` の「Load versions.env」step を `KEY=value` 行のみ抽出する形へ修正(impl-ci。2026-07-13 適用・未コミット)
- [x] `.devcontainer/versions.env` の 7 行目ヘッダーコメントを動作する消費方法へ修正(impl-ci。2026-07-13 適用・未コミット)
- [x] CI(`cicd.yml` / `contract-drift.yml` / `sqlc-drift.yml`)の「Build toolchain image」step が通ることを検証(2026-07-13 push 後、3 workflow すべて conclusion: success)

### 再発防止

- `versions.env` の「shell / GitHub Actions / docker compose / make すべてから読める」という設計意図に対し、GitHub Actions 消費だけがコメント除外を要する点をヘッダーコメントに明記する。
- 仮説: 将来 `versions.env` にコメントや新規行を足しても壊れないよう、抽出は行フォーマットに依存しない `KEY=value` フィルタで固定するのが望ましい(現行の `cat` 全流し込みは脆い)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-13

- 起票。GitHub Actions CI が全 `make <target>` ジョブの「Build toolchain image」→「Load versions.env」step で `Error: Unable to process file command 'env' successfully. / Error: Invalid format '# versions.env — single source of truth for pinned tool / runtime'` を出して失敗する事象を調査。
- 原因を特定: `action.yml`(56 行目)の `cat versions.env >> "$GITHUB_ENV"` が、`$GITHUB_ENV` の受け付けない**コメント行(1 行目〜)・空行**を含むファイル全体を流し込んでおり、最初のコメント行で `Invalid format` になる。エラー文字列は `.devcontainer/versions.env` 1 行目と一致。
- 付随して `.devcontainer/versions.env` 7 行目のヘッダーコメントが、動作しない消費方法 `cat .devcontainer/versions.env >> "$GITHUB_ENV"` を案内している(ドキュメントの誤り)ことを確認。SPEC-009 Phase C で導入された共有 composite action 経路の不具合として SPEC-009 と相互リンク。
- 対応方針: `KEY=value` 行のみ抽出して追記する形へ action を修正し、versions.env のヘッダーコメントも追随修正する。impl-ci が並行で修正実施中のため status は `fixing` で起票。

### 2026-07-13(追記: impl-ci が修正を適用)

- impl-ci が修正を適用(working tree のみ・未コミット):
  1. `.github/actions/build-toolchain-image/action.yml` の「Load versions.env」step を `cat` から `grep -E '^[A-Za-z_][A-Za-z0-9_]*=' "${{ github.workspace }}/.devcontainer/versions.env" >> "$GITHUB_ENV"` に変更(コメント/空行を除外する理由の説明コメントも追加)。
  2. `.devcontainer/versions.env` ヘッダーコメントの GitHub Actions 向け消費方法の記述を grep フィルタ形へ追随修正(値の行は無変更)。
- 検証結果(事実): grep フィルタのローカル実行で 11 個の `KEY=value` 行(`GO_VERSION` 〜 `GOIMPORTS_VERSION`)のみが出力され、コメント/空行を含まないことを確認。action.yml の YAML パース妥当性も確認済み。
- 残り: commit/push 後に CI(`cicd` / `contract-drift` / `sqlc-drift`)の green を確認して `resolved` へ遷移する。CI 未確認のため status は `fixing` を維持。

### 2026-07-13(追記: 修正を commit・push し CI green を確認 → resolved)

- 修正を commit `979672c`(branch `feat/auth-oidc-foundation`、2026-07-13 push)。内容: action.yml の grep フィルタ化 + versions.env ヘッダーコメント追随 + ISSUE-055 / SPEC-009 の記録(4 ファイル)。
- 検証(事実): push 後の GitHub Actions 3 workflow(CI = `cicd.yml` / OpenAPI Contract Drift = `contract-drift.yml` / sqlc Drift = `sqlc-drift.yml`)がすべて `conclusion: success` で完了。いずれも修正対象の `build-toolchain-image` composite action を使用しており、「Load versions.env」step が通過したことを確認。以て症状(全 make ジョブが Load versions.env で失敗)の解消を検証できたため status を `resolved` に遷移。
- 補足(ローカル検証の限界): 本 commit のローカル pre-commit hook は DB 依存テストフェーズでポート 5432 の環境競合(別プロジェクトのコンテナが占有)により実行できず `SKIP_PRE_COMMIT=1` でスキップした。ただし hook の他フェーズ(web / iac / api・auth offline check / migrator / contract drift / sqlc drift)は commit 試行で全 green を確認済みで、DB 依存テストは CI 側で green を確認した。
