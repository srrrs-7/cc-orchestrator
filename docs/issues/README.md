# issues — 不具合・課題の時系列記録

1課題 = 1ファイル。`/issue` スキル(`.claude/skills/issue/`)で作成・更新する。

## 命名規則

`YYYYMMDD-NNN-<slug>.md` — 日付(起票日) + 通し番号(= Issue ID) + 英語スラッグ。
ファイル名順 = 起票の時系列順。

## 現状の把握方法(AI向け)

1. ファイル名順に frontmatter(`status` / `severity` / `updated`)を読む — 何が未解決かがわかる
2. 各ファイルは「1. ユーザー価値への影響」が最上位。影響 → 現象 → 原因 → 対応 の順に深まる
3. 「経緯」セクションは追記のみの時系列ログ。**下から読むと最新の調査状況がわかる**
4. `resolved` / `closed` のIssueは過去の意思決定・修正理由の記録として参照する

## ステータス

`open` → `investigating` → `fixing` → `resolved` → `closed` / `wontfix`
