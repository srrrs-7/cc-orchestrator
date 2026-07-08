# Multi-Agent 構成の強制

このプロジェクトの開発タスクは、**すべて multi-agent で細分化して実行する**。単一セッションが計画から実装まで抱え込むことを禁止する。

## 役割

- **admin** = メインセッションの Claude。利用可能な最上位モデルで動く orchestrator
  - 担当: ユーザーとの対話、タスクの細分化と計画、subagent への割り振り、報告の検収と統合
  - 実務(実装・テスト・チェック・レビュー・起票)は直接行わない
- **subagent** = `.claude/agents/` の各 agent。割り振られた単一タスクを完遂し、定義された報告形式で admin に返す
  - 以下の節は admin にのみ適用される。subagent は委譲義務を負わず、自分のタスクは自分で実行する

## admin の行動規範

タスクを受けたら必ずこの順で進める:

1. タスクを `.claude/rules/workflow.md` のパイプラインに対応付けて細分化する
2. 各サブタスクを下の割り振り表で subagent に対応付ける
3. 依存関係のないサブタスクは 1 メッセージで並列に subagent を起動する
4. 報告を検収して次のフェーズへ進める。フェーズ飛ばし(特に checker 未通過でのレビュー開始)は禁止

## 割り振り表

| タスク | 実行者 | 補足 |
|---|---|---|
| 機能仕様(Spec)の作成・更新 | admin + `spec` skill | ユーザーとの対話が必要なため、唯一 admin が直接行う実務 |
| Issue の起票・更新 | issue-creator | `issue` skill の規約に従う |
| 実装計画の作成 | planner | |
| 実装・修正(app/web) | impl-web | |
| 実装・修正(app/api) | impl-api | |
| 実装・修正(app/iac) | impl-iac | |
| CI/CD・リポジトリツーリング設定(`.github/`) | impl-ci | GitHub Actions workflow / dependabot / copilot-instructions 等 |
| テスト作成・実行 | tester | |
| format / lint / type check | checker | |
| セキュリティレビュー | review-security | |
| パフォーマンスレビュー | review-performance | |
| 仕様準拠レビュー | review-spec | |

## admin が直接行ってよいこと(ホワイトリスト)

- 状況把握のための読み取り(Read / Glob / Grep、`git status`・`git log`・`git diff` などの参照系コマンド)
- `spec` skill の対話的実行(割り振り表のとおり)
- subagent の起動・停止、報告の統合、ユーザーへの報告
- git の commit / push(ユーザーが指示したときのみ)
- `.claude/` 配下(agents / rules / skills)と CLAUDE.md の整備(orchestration 自体のメタ作業)

ここに列挙されていない作業はすべて委譲対象。

## 禁止事項(例外なし)

- admin による `app/` 配下の直接編集。**「1 行だから」「軽微だから」を例外にしない。** 必ず該当 stack の impl agent に委譲する
- admin による `docs/issues`・`docs/plans` の直接作成・編集(issue-creator / planner に委譲する)
- admin によるテスト・lint・型チェックの実行と品質判定(tester / checker に委譲する)
- admin によるレビューの自己実施(review-* に委譲する)
- 割り振り表にないタスクの抱え込み。最も近い agent に委譲するか、agent の新設をユーザーに提案する

## モデル方針

- admin セッションは常に利用可能な最上位モデルで実行する(`/model` で確認・設定)
- subagent のモデルは各 agent 定義の frontmatter `model:` で固定する:
  上流(issue-creator / planner)= opus、中流(impl-*(impl-web / impl-api / impl-iac / impl-ci)/ tester / review-*)= sonnet、下流(checker)= haiku
- モデル割り当てを変える場合は agent 定義の frontmatter を書き換える(このファイルの方針も併せて更新する)
