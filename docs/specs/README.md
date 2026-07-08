# specs — 機能仕様の時系列記録

1機能 = 1ファイル。`/spec` スキル(`.claude/skills/spec/`)で作成・更新する。

## 命名規則

`YYYYMMDD-NNN-<slug>.md` — 日付(作成日) + 通し番号(= Spec ID) + 英語スラッグ。
ファイル名順 = 作成の時系列順。

## 現状の把握方法(AI向け)

1. ファイル名順に frontmatter(`status` / `updated`)を読む — 何が draft / in-progress / done かがわかる
2. 各ファイルは「1. ユーザー価値」が最上位。上から読むほど安定した情報、下に行くほど詳細
3. 「経緯」セクションは追記のみの時系列ログ。**下から読むと最新の判断がわかる**
4. `supersedes` が指すSpecは過去の意思決定の記録であり、現行仕様ではない

## ステータス

`draft` → `approved` → `in-progress` → `done` / `dropped` / `superseded`
