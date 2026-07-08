---
name: github-pr
description: gh CLI で GitHub Pull Request を作成する。本文は必要最低限の概要だけを固定テンプレートで記す。PR の作成・PR 説明文の作成を求められたときに使用する。
argument-hint: "[PRの目的 | base=<branch>]"
---

# GitHub PR作成スキル

`gh pr create` で PR を作る。本文は「何を・なぜ」だけを**最小文字**で記す。
差分から読み取れる詳細(ファイル一覧・行単位の説明)は書かない。レビュアーが数秒で要点を掴めることが目標。

## 手順

1. 現状把握(参照系のみ): `git status` / `git diff <base>...HEAD --stat` / `git log <base>..HEAD --oneline` で変更範囲とコミットを確認する
   - `<base>` は既定で `main`(引数 `base=<branch>` で上書き)
2. ブランチ確認: 現在が base ブランチのときは PR を作れない。作業ブランチに分けてから進める
3. push: 未 push のコミットがあれば `git push -u origin <branch>` で push する(PR 作成の前提として実施)
4. [template.md](template.md) を読み、各セクションを 1〜数行で埋める
   - **「概要」は 1 文**。1 文で書けないなら PR の分割を検討する
   - 「変更点」は意味のある変更のみ箇条書き(ファイル名の羅列にしない)
   - 関連する SPEC-NNN / ISSUE-NNN があれば必ず結びつける(なければ「なし」)
5. 作成: `gh pr create --base <base> --title "<title>" --body "<body>"`(body は template.md のフッターまで含める)
6. 作成後、PR の URL をユーザーに報告する

## ルール

- 本文は日本語。コマンド・識別子・エラーは原文のまま
- タイトルは要点のみ(接頭辞は任意: `feat:` / `fix:` / `docs:` など)。関連 ID があれば含める
- 記載は**差分から読み取れないこと(意図・背景・影響)に絞る**。自明な列挙・冗長な説明をしない
- 本文末尾のフッター(template.md 記載)は必ず残す
