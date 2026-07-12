---
id: RETRO-003
title: CLAUDE.md と rules/auth.md の概要記述が実装の進展(集約 7 パッケージ・auth 新機能・新 Makefile ターゲット)に追随せず陳腐化した
status: addressed  # open | addressed | wontfix
severity: medium  # high(頻発・手戻り大 / タスクをブロック) | medium(回避したが非効率) | low(軽微)
source: review-spec
phase: orchestration  # spec | plan | test | impl | check | review | orchestration | other
target: CLAUDE.md
created: 2026-07-12
updated: 2026-07-12
synthesis: RETROSUM-001
tags: [doc-drift, missing-command, stale-summary]
---

# RETRO-003: CLAUDE.md / rules/auth.md の概要記述が実装の進展に追随せず陳腐化した

## 1. 遭遇した課題(何が摩擦だったか)

> **CLAUDE.md** と **rules/auth.md** の **app/auth 概要・集約数・コマンド早見表** が、機能追加(ISSUE-031〜040)後も更新されず、**常時ロードされる一次情報が実装と食い違ったまま**になっている。

- **具体的に何が起きたか**:
  - CLAUDE.md は app/auth を「client / user / authcode / refreshtoken の **4 集約** + token」と書くが、実際は `app/auth/domain/` に `consent`(ISSUE-032)・`idpsession`(ISSUE-031)が加わり **7 パッケージ**。
  - CLAUDE.md コマンド早見表に、ルート Makefile の `auth-signing-keys` / `rotate-auth-signing-keys`(ISSUE-036、`Makefile:279,283`)が載っていない。
  - rules/auth.md は AUTH-001 / SPEC-006 時点のままで、ログイン/同意 UI・RP-Initiated Logout・`/revoke`・confidential client・RSA 鍵永続化+ローテーション・audience 分離・introspection に一切触れていない。SPEC-009/013 のコンテナ実行注記も api.md / db.md にはあるが auth.md には無い。
- **どのアセットの問題か**: 誤り / 欠落(陳腐化)。常時ロードされる CLAUDE.md と、`app/auth/**` を扱う際に自動ロードされる rules/auth.md が実態とずれている。

## 2. 影響(タスクにどう響いたか)

- **症状**: 誤った前提。admin / subagent が概要や集約数・利用可能コマンドを CLAUDE.md / auth.md から把握すると実態と食い違い、担当範囲やコマンドの誤認につながる。
- **コスト**: 直接のブロックはないが、レビューや計画のたびに「記述と実体どちらが正か」を実ファイルで再確認する非効率が発生。今回のレビューでも Minor 6 件として検出。

## 3. 改善提案(どう直すか)

- **CLAUDE.md**: app/auth の概要を「client / user / authcode / refreshtoken / consent / idpsession の 6 永続化集約 +(署名ポートのみで永続化しない)token」に更新。コマンド早見表に `auth-signing-keys` / `rotate-auth-signing-keys` を追記。
- **rules/auth.md**: ISSUE-031〜040 で加わった機能(login/consent UI・logout・revoke・introspect・confidential client・鍵永続化+ローテーション・audience)を概要に反映し、SPEC-009/013 のコンテナ実行注記を api.md / db.md と揃えて追記。
- **恒久策(仮説)**: rules/orchestration.md の検収チェックに「機能追加が完了したら、その概要が CLAUDE.md / 該当 rules の常時ロード記述と一致しているか確認する」を 1 項目追加し、陳腐化を検収時に拾えるようにする。RETRO-002(Spec 回避)と同根で、機能追加時のドキュメント追随が仕組み化されていない。

## 4. 根拠 / 再現(なぜそう言えるか)

- **根拠**: review-spec 報告 Minor #3〜5, #10。CLAUDE.md の app/auth 概要行、`app/auth/domain/` の 7 パッケージ実在、`Makefile:279,283`、rules/auth.md の記述範囲。
- **再現条件**: 機能追加(新集約・新コマンド・新エンドポイント)後にドキュメント更新タスクを明示委譲しないと再現する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 記録。リポジトリ全体レビューの検収中、review-spec の Minor 群として surface。是正(CLAUDE.md / rules/auth.md の更新)は admin の `.claude` メタ作業として本タスク内で対応する予定だった。RETRO-002 と同じ「機能追加時のドキュメント追随が仕組み化されていない」パターン。
- **追記(同日、修正着手時)**: 是正しようとしたところ、作業ツリーに**当セッション外の未コミット変更**として CLAUDE.md(app/auth 概要を「6 集約」+ 署名鍵コマンド + idpsession in-memory へ更新済み)と `.claude/rules/auth.md`(集約 4→6、実装済み機能一覧、`make auth-signing-keys` / `rotate-signing-keys`、SPEC-009/013 コンテナ実行 + `auth_test` 注記を追加済み)が既に存在していた(`git diff .claude/rules/auth.md` で確認)。つまり本 entry が指摘した陳腐化は、別途の未コミット作業で既に解消されていた。admin による重複編集は行わない。**この「レビューが陳腐化を検出した時点で、既に別作業で修正が進んでいた」状況自体が、機能追加とドキュメント追随が別々に散発的に行われ、追随状況が可視化されていない(RETRO-002 と同根)ことの傍証**。status は当該修正が commit されるまで `open` 据え置き(retro の `addressed` は `.claude` 変更が commit されたときのみ)。恒久策(orchestration.md の検収チェックにドキュメント追随確認を 1 項目追加)は依然有効。

### 2026-07-12(帰属の補足)

- 上記の「セッション外の未コミット変更」の出所を補足: 別セッションの `/init`(admin メタ作業。3 並列の Explore 検証で実装との差分を確認して CLAUDE.md を更新)と、続くユーザー指示「auth.md も更新して」による admin の直接更新。散発的な無管理の修正ではなく、いずれも entry の提案 1・2 に対応する意図的な適用だった(適用内容は提案どおり + セキュリティ規約・準拠仕様の追補)。恒久策(提案 3)は未適用のまま synthesizer の統括対象として残す。status はコミット時にコミット ID を追記して `addressed` へ遷移する。

### 2026-07-12(同種摩擦の再発: rules/api.md)

- auth.md 修正の過程で **`.claude/rules/api.md` にも同種のより深い陳腐化**を発見: レイアウトセクションが「`internal/` にアプリケーションコード本体を置く」「`internal/user`, `internal/order` など」と記述していたが、実際の `app/api` はトップレベル `domain` / `service` / `infra` / `route` / `cmd` 構成で `internal/` は存在しない(CLAUDE.md・auth.md・copilot-instructions の記述とも矛盾していた)。SPEC-009/013 のコンテナ実行・実 test DB 注記も欠落していた。ユーザー指示「api.md も直して」により admin が同日修正(冒頭概要 + SPEC-015 リソースサーバー化・コマンド注記・実レイアウト反映)。**同種摩擦が 3 ファイル(CLAUDE.md / auth.md / api.md)にまたがった再発**であり、恒久策(検収時のドキュメント追随確認)の必要性を補強する。severity は medium を維持(いずれも直接ブロックはしていない)。

### 2026-07-12(addressed へ遷移)

- 是正がコミットされたため addressed に遷移: CLAUDE.md / `.claude/rules/auth.md` / `.claude/rules/api.md`(+ 本 entry の経緯)は `5b88480`、`.github/copilot-instructions.md`(同種ドリフトの隣接修正。impl-ci 実施)は `2fba5d3` に含まれてコミット済み。提案 1・2 は解消。**恒久策(提案 3: orchestration.md の検収チェックへのドキュメント追随確認の追加)は未適用のまま**であり、retro-synthesizer の統括対象として残る(RETRO-002 と同根のため統括での一括提案が適切)。なお `5b88480` / `2fba5d3` は新スタック `app/auth-web` を導入しており、更新直後の CLAUDE.md / orchestration.md 割り振り表が同スタックに未追随 = 本 entry と同種の摩擦が直ちに再発している(別 entry 化は同種のため行わず、ここに記録)。

### 2026-07-12(auth-web 追随の適用 + 同種摩擦の追加発見)

- ユーザー指示「auth-web も CLAUDE.md と割り振り表に追随させて」により admin が適用: CLAUDE.md(リポジトリ概要に `app/auth-web` bullet、コマンド早見表の web 行を web / auth-web に拡張、ローカル実行を 5 サービス + `:8083` + `auth-web-%` エイリアスに更新、CI job 列挙に auth-web 追加)、orchestration.md 割り振り表(impl-web の担当を app/web / app/auth-web に拡張)、rules/web.md(frontmatter `paths` に `app/auth-web/**` 追加 + 両 stack 適用の冒頭注記)、agents/impl-web.md(担当範囲の拡張)。
- 適用中に **agents/impl-web.md の手順に `pnpm run typecheck` 等の陳腐化**を発見(実契約は toolchain コンテナ経由の `make typecheck` / `build` / `test`。SPEC-009 のホスト直接実行禁止とも矛盾)。同時に修正。agent 定義内の具体的コマンド記述も本 entry と同じドリフト源であり、恒久策の検収チェックは agents/ 配下も対象に含めるべき。
- 別途、pre-commit hook(`.githooks/lib/detect-stacks.sh` + `run-checks.sh`)が auth-web stack 非対応であることも確認(CI の `cicd.yml` はカバー済み)。これは `.claude` ではなく `.githooks` の実装ギャップのため impl-ci への委譲対象としてユーザーに報告。
