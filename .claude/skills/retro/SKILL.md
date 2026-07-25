---
name: retro
description: .claude/retro にorchestration自己改善の振り返り記録(retro entry)を固定テンプレートで時系列に記録・更新する。agent/skill/rule/CLAUDE.md の曖昧さ・欠落・誤り・非効率など「実行中に遭遇した .claude 自体の摩擦」を記録するとき、既存 entry の追記・ステータス変更、entry 一覧の確認を求められたときに使用する。product の不具合は /issue、こちらは orchestration(.claude/)の課題専用。
argument-hint: "[摩擦の説明 | update RETRO-NNN | list]"
---

# 振り返り(retro)記録スキル

`.claude/retro/entries/` 配下で、**orchestration 自体(`.claude/` の agents / rules / skills / CLAUDE.md)の摩擦**を時系列に記録する。1 記録 = 1 ファイル。
これは「実行してみて初めてわかった `.claude` の改善点」を溜める場所で、溜まった記録は `retro-synthesizer` agent が横断分析して `.claude/` の改善提案に変える(ループ全体の正は [`.claude/retro/README.md`](../../retro/README.md))。

**scope の境界(重要)**:

- **ここ(retro)** = orchestration の課題。ルールが曖昧で agent が迷った / コマンド表と実際の Makefile がずれていた / agent の担当範囲が不明確だった / 報告形式に必要な項目が無かった、など「`.claude/` を直せば次から楽になる」もの。
- **docs/issues(`/issue`)** = 開発対象(product: app/web・api・auth・iac)の不具合・課題。**retro に product のバグを書かない。**
- どちらか迷ったら「`.claude/` を変更して直るか、`app/` を変更して直るか」で切り分ける。

## モード判定

引数・文脈から判定する:

- **新規記録**(デフォルト): 実行中に `.claude/` の摩擦が見つかった
- **更新**: 既存 entry(ID・target・文脈で特定)への追記、ステータス変更(`open` → `addressed` / `wontfix`)
- **一覧**: 「一覧」「状況」など → 全 entry の frontmatter を読み、`status` 別・`target` 別・`severity` 順に要約して報告する(ファイルは作成しない)

## 新規記録の手順

1. 採番: `ls .claude/retro/entries/` で既存ファイル名の連番 NNN の最大値を確認して +1(3 桁ゼロ埋め、初回は `001`)
2. 日付: `date +%Y-%m-%d` で今日の日付を取得する(推測しない)
3. ファイル名: `.claude/retro/entries/YYYYMMDD-NNN-<slug>.md`(日付はハイフンなし 8 桁、slug は英小文字ケバブケース)
4. 同ディレクトリの skill 直下 [template.md](template.md) を読み、全セクションを埋めて作成する
   - **`target` に「どの `.claude` アセットの問題か」を必ず 1 つ以上書く**(統括のクラスタ化キー)。複数なら主因を 1 つに絞り、他は本文に書く
   - **「3. 改善提案」はどのファイルに何を変えるかまで具体的に書く。** わからなければ「仮説:」を付けて方向性だけ残す(空欄・セクション削除は不可)
   - `severity` は頻度 × 手戻りの大きさで付け、根拠を本文に添える
   - 「5. 経緯」に記録時のエントリを書く
5. 同一 target・同種の摩擦が既にあれば**重複記録せず**、その entry の「経緯」に再発として追記し、`severity` を必要なら引き上げる(頻度は統括の重要シグナル)

## 更新の手順

1. 対象ファイルを特定して読む
2. 該当セクション(改善提案・根拠など)を現状に合わせて更新する
3. 「5. 経緯」の**末尾**に `### YYYY-MM-DD` 見出しで「何がわかったか / どう対応したか」を追記する(過去エントリは編集禁止)
4. frontmatter の `status` と `updated` を更新する。統括レポートで扱われたら `synthesis` に `RETROSUM-NNN` を記入する
5. **`addressed` にするのは、対応する `.claude/` の変更が実際にコミットされたときのみ。** どのコミット / どの提案(RETROSUM-NNN)で解消したかを経緯に書く

## ルール

- 本文は日本語。ファイル名・コマンド・アセットのパス・エラーメッセージは原文のまま引用する
- **記録の起点は admin。** admin が各 subagent の報告を検収する際に orchestration の摩擦を吸い上げ、この skill で記録する(subagent 報告に摩擦が表れていれば拾う)。詳細は README の「記録は誰がするか」
- ステータス遷移: `open` → `addressed`(`.claude/` 変更で解消)/ `wontfix`(対応しない判断。理由を経緯に)
- 事実と推測を区別する(推測には「仮説:」を付ける)
- **統括(analysis)はこの skill ではやらない。** 溜まった entry の横断分析と改善提案は `retro-synthesizer` agent の担当。admin がそれを起動し、提案レポートを検収して `.claude/` に適用する
