---
name: release-pr
description: main の HEAD から vX.Y.Z ブランチを切り、リリース PR を作成する。変更のユーザー影響・関連 PR / Issue・インフラのデプロイ要件をテーブルで集約する。リリースの準備・リリース PR の作成を求められたときに使用する。
argument-hint: "vX.Y.Z base=<branch>"
---

# リリースPR作成スキル

`main` の HEAD から `vX.Y.Z` ブランチを切り、`base` へ向けたリリース PR を作る。
本文は `base..main` の差分を集約し、**変更のユーザー影響・関連 PR / Issue・インフラのデプロイ要件をテーブル**でまとめる。

## 引数(どちらも必須・既定なし)

- `vX.Y.Z` — リリースバージョン。`v` + semver(`v1.4.0` など)。自動採番はしない
- `base=<branch>` — マージ先ブランチ(例: `base=production` / `base=staging`)。既定は置かない

いずれかが欠けている・形式が不正なときは、実行せずユーザーに確認する。

## 手順

1. 引数検証: `vX.Y.Z` が `v\d+\.\d+\.\d+` か、`base=` が指定されているかを確認する
2. 同期と存在確認: `git fetch origin`。`base` がリモートに在るか `git ls-remote --heads origin <base>` で確認する(無ければ中止して報告)
3. リリース内容の確定: 範囲は `origin/<base>..origin/main`
   - `git log origin/<base>..origin/main --oneline` で対象コミットを確認する。**差分が空ならリリース対象なし**として中止・報告する
4. リリースブランチ作成: main HEAD から切る
   - `git checkout -b vX.Y.Z origin/main`(未コミット変更はリリースに含めない。あれば警告)
   - `git push -u origin vX.Y.Z`(PR の前提として push する)
5. 変更の収集(範囲は 3 と同じ):
   - **PR**: コミットログの `(#NN)` / `Merge pull request #NN` から PR 番号を拾い、`gh pr view <NN> --json number,title,url` で概要とリンクを得る
   - **Issue / Spec**: 各 PR 本文と `docs/issues` / `docs/specs`(frontmatter の `status` が範囲内で resolved / closed / done のもの)から関連 ID を拾い、該当ファイルへリンクする
   - **ユーザー影響**: PR / Issue の「ユーザー価値への影響」から一言で要約する(不明なものは「要確認」と明記)
6. インフラのデプロイ要件抽出: `git diff --name-only origin/<base>..origin/main -- app/iac` を確認し、変更のあった `envs/<env>` / `modules/<module>` と必要作業(terraform plan → apply)を表にする。**`apply` はこのスキルでは実行しない**
7. 本文生成: [template.md](template.md) を埋める(該当のない欄は「なし」)
8. 作成: `gh pr create --base <base> --head vX.Y.Z --title "vX.Y.Z" --body-file <path>`
9. PR の URL を報告する

## ルール

- バージョンと base は毎回明示する(自動採番・既定 base を持たない)
- タイトルは `vX.Y.Z` のみ
- 本文は日本語。PR / Issue 番号・コマンド・ブランチ名は原文のまま
- **デプロイ(`terraform apply` を含む)はこのスキルでは実行しない。** 要件を表に記載するに留め、実施可否はユーザーが判断する
- git タグはこのスキルでは打たない(リリース確定=マージ後に別途)
- 本文末尾のフッター(template.md 記載)は必ず残す
