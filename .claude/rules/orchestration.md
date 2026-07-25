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
5. 検収の一部として、orchestration 自体(`.claude/` の rules / agents / skills / CLAUDE.md)の摩擦(曖昧さ・欠落・誤り・非効率)に気づいたら `retro` skill で `.claude/retro/entries/` に記録する。subagent 報告に摩擦が表れていれば拾う(product の不具合は対象外 → issue-creator)。溜まった記録は随時 `retro-synthesizer` で統括し、提案を `.claude/` に適用する。ループの正は [`.claude/retro/README.md`](../retro/README.md)
6. 機能追加の検収では**完了ゲート**として次を確認する: (i) 確定済み Spec の記述(env 契約・ポート方針・集約構成など)を破っていないか。破るなら Spec を先行更新してから進める。(ii) 完了した機能が **CLAUDE.md・該当 rules(常時ロード含む)・該当 agents 定義**の記述(集約数・コマンド表・エンドポイント・担当範囲・参照先 rules 一覧)と一致しているか。drift があればドキュメント追随を適用または委譲する(`agents/` 配下も追随対象)

## 割り振り表

| タスク | 実行者 | 補足 |
|---|---|---|
| 機能仕様(Spec)の作成・更新 | admin + `spec` skill | ユーザーとの対話が必要なため、唯一 admin が直接行う実務 |
| Issue の起票・更新 | issue-creator | `issue` skill の規約に従う |
| 実装計画の作成 | planner | |
| 実装・修正(app/web / app/auth-web) | impl-web | TypeScript / React の 2 SPA(タスク UI / IdP 管理 UI)を担当。規約は `.claude/rules/web.md` で共通 |
| 実装・修正(app/api) | impl-api | |
| 実装・修正(app/auth) | impl-auth | domain / service / route。永続化(infra/postgres)は impl-db |
| 実装・修正(app/iac) | impl-iac | |
| CI/CD・リポジトリルート/横断ツーリング設定(`.github/` + リポジトリルート) | impl-ci | GitHub Actions workflow / dependabot / copilot-instructions、およびルート `Makefile` / `compose.yml` / `.devcontainer/`(toolchain / compose.tools.yml / versions.env 等) / `.gitignore` / `.env` など特定 stack に属さない横断ツーリング(SPEC-009)。`app/<stack>` 内のコード・各 stack の Makefile/package.json の中身は各 impl が担当 |
| 実装・修正(DB/永続化層: migrations / sqlc / infra/postgres, および `app/migrator`(独立 Go モジュール。対象 DB 作成 + goose 適用)。app/api・app/auth 横断) | impl-db | ポート(`Repository` interface)は domain 側(impl-api / auth)、実装は `infra/postgres` 側で分担。`app/migrator` 一式(main / config / database / Dockerfile / Makefile / go.mod)も impl-db 所有(SPEC-005)。概念で担当を切る(impl-ci と同型) |
| リファクタリング(挙動不変のコード内部改善) | refactor | 対象 stack / スコープを明示して割り振る。既存テストを**無改変で緑**に保つ。公開契約(HTTP / DTO / OpenAPI・ドメインポート・env 契約)・DB スキーマは変えない(変えるなら Spec 起票 → 該当 impl)。機能追加・バグ修正はしない |
| テスト作成・実行 | tester | |
| format / lint / type check | checker | Go スタックの `make check` は build + test を含むため、test 結果の報告要件も checker に適用される(詳細は `agents/checker.md` の報告形式) |
| セキュリティレビュー | review-security | |
| パフォーマンスレビュー | review-performance | |
| 仕様準拠レビュー | review-spec | |
| orchestration の摩擦記録(retro entry)の記録・更新 | admin + `retro` skill | `.claude/` 自体の課題。ユーザー対話は不要だが `.claude/` メタ作業のため admin が直接記録する(spec skill 行と同型)。product の不具合は issue-creator |
| 振り返りの統括・`.claude/` 改善提案 | retro-synthesizer | 溜まった retro entry を横断分析し改善提案レポートを出す。**提案のみ**で `.claude/` の適用は admin |

## admin が直接行ってよいこと(ホワイトリスト)

- 状況把握のための読み取り(Read / Glob / Grep、`git status`・`git log`・`git diff` などの参照系コマンド)
- `spec` skill の対話的実行(割り振り表のとおり)
- subagent の起動・停止、報告の統合、ユーザーへの報告
- git の commit / push(ユーザーが指示したときのみ)
- ユーザーが明示的に指示・実行した git 操作(pull / merge / branch 操作等)の完了と、それに付随する git 設定の修正。ただし競合解消が `app/` 配下の編集に及ぶ場合、解消の編集はファイル所有 stack の impl agent に委譲する(例: `app/api/go.mod` → impl-api、`app/migrator/**` → impl-db。フローの正は `workflow.md` の「維持作業」)
- `.claude/` 配下(agents / rules / skills)と CLAUDE.md の整備(orchestration 自体のメタ作業)
- `.claude/retro/` への振り返り記録(retro entry)の記録・更新(`retro` skill 経由)と、`retro-synthesizer` の提案に基づく `.claude/` への適用(orchestration 自体のメタ作業)

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
  上流(issue-creator / planner / retro-synthesizer)= opus、中流(impl-*(impl-web / impl-api / impl-auth / impl-iac / impl-ci / impl-db)/ refactor / tester / review-*)= sonnet、下流(checker)= haiku
- モデル割り当てを変える場合は agent 定義の frontmatter を書き換える(このファイルの方針も併せて更新する)
