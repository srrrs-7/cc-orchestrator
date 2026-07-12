---
id: RETROSUM-001
title: 機能完了ゲート・維持系タスクの定義・報告形式が orchestration 定義の主要な穴
date: 2026-07-13
status: proposed  # proposed(提案済み)| applied(admin が .claude/ に適用済み)
entries: [RETRO-001, RETRO-002, RETRO-003, RETRO-004, RETRO-005, RETRO-006]
---

# RETROSUM-001: 機能完了ゲート・維持系タスクの定義・報告形式が orchestration 定義の主要な穴

## サマリ

対象 entry 6 件(RETRO-001〜006、うち 003 は addressed だが恒久策未適用)。最大クラスタは **orchestration phase(002 / 003 / 005 の 3 件)**で、いずれも「反復するタスク類型に対して admin の検収ゲート・フローが定義されていない」ことが根。最優先で直すべきは **`rules/orchestration.md` の「admin の行動規範」** と **`rules/workflow.md` のパイプライン**(機能完了ゲート + 維持系タスクフロー)。次点は `rules/{api,auth,db}.md` + Go Makefile(依存解決コマンド)、`skills/spec`、`agents/checker.md`。

## クラスタ分析

### クラスタ 1 — 機能完了時の「Spec 追随・ドキュメント追随」ゲートの欠落(最優先・systemic)

- **パターン**: 機能追加が完了する局面で、admin の検収が「(a) その機能を Spec として一次情報化する / (b) 確定済み Spec・常時ロード docs を実装に追随させる」を強制しない。結果、Spec を回避したまま確定契約が陳腐化し(RETRO-002)、CLAUDE.md / rules が実装から drift する(RETRO-003)。
- **該当 entry**: RETRO-002(high)/ RETRO-003(medium・addressed だが恒久策 open)
- **頻度 / 深刻度**: 頻度 2 / 深刻度 high。RETRO-003 本文が「RETRO-002 と同根」「機能追加とドキュメント追随が別々に散発的に行われ、追随状況が可視化されていない」と 3 回明示。RETRO-003 は表層(CLAUDE.md/auth.md/api.md/copilot-instructions の drift)を `5b88480`/`2fba5d3` で修正済みだが、恒久策=検収ゲートは未適用のまま。
- **根本原因**: 
  - `rules/workflow.md`「## パイプライン」冒頭は **「機能開発は Spec を、不具合対応は Issue を起点とする。」** とだけ書き、**何をもって「機能開発」とし Issue で済ませてよいかの線引きが無い**。ロードマップ plan(AUTH-002)が機能群を Issue 分割だけで着手指示しても止まらなかった。
  - `rules/orchestration.md`「## admin の行動規範」の 1〜5 は、5 で retro 記録を入れたが、**「機能完了時に Spec/docs が実装に追随しているかを確認する」ゲートを持たない**。追随は「別セッションの /init やユーザーの都度指示」に依存し、仕組み化されていない(RETRO-003 経緯)。追随対象には agents/ 配下も含む(RETRO-003 経緯で `agents/impl-web.md` の `pnpm run typecheck` 陳腐化を発見)。
  - 傍証(同種 drift の現存): `agents/checker.md:14` は参照先を **`.claude/rules/{web,api,iac}.md`** と書くが、実際は auth.md / db.md も存在する。ゲートがあれば拾えたはずの drift が今も残っている。

### クラスタ 2 — 維持系タスク(依存 bump / main 取り込みマージ)の役割・フロー・コマンドの未定義

- **パターン**: origin/main 取り込みマージ + go.mod 競合という反復維持タスクで、(a) admin がフロー全体(merge 実行可否・委譲単位・パイプライン適用範囲・commit 授権)を即興解釈し(RETRO-005)、(b) その中の `go mod tidy` を impl-api / impl-auth / impl-db が独立に手組みした(RETRO-004)。両 entry は同一トリガタスク(`feat/auth-oidc-foundation` への dependabot bump マージ)由来で相互補強。
- **該当 entry**: RETRO-005(medium)/ RETRO-004(medium)
- **頻度 / 深刻度**: 頻度 2 / 深刻度 medium。dependabot 稼働のため反復ほぼ確実(両 entry が明記)。今回はブロック無しだが session ごとの一貫性リスク + 毎回 10+ tool uses の重複読解コスト。
- **根本原因**:
  - `rules/workflow.md` のパイプラインは Spec/Issue 起点前提で、**維持タスク類型の軽量フローが無い**(planner/tester/review-* をどこまで適用するか不明)。
  - `rules/orchestration.md`「admin が直接行ってよいこと」は **「git の commit / push(ユーザーが指示したときのみ)」のみで merge / config --unset が未列挙**、かつ割り振り表に git 操作を担う行が無い。
  - 3 Go スタックの Makefile に **`tidy` ターゲットが存在しない**(全 Makefile を grep で確認)。`rules/api.md`(14〜22 行)/ `auth.md`(27〜35 行)/ `db.md`(30〜36 行)のコマンド表にも依存解決(go mod tidy / download)の正規経路が無い。web は `install`(network 有効)を既に持つのと対照的。

### クラスタ 3 — テスト戦略変更 Spec の起票チェック観点の欠落(単発・手戻り実績あり)

- **パターン**: 既存ダブルを実基盤へ置換するテスト戦略 Spec で、ダブルを「実 DB で観測可能か」で先に分類しないと要件・例外条項が狭くなり、レビュー段階の逸脱指摘 → Spec 改訂 → 計画差し戻しの手戻りになる。
- **該当 entry**: RETRO-001(medium)
- **頻度 / 深刻度**: 頻度 1 / 深刻度 medium。実際に SPEC-013 で計画確定が 1 往復遅れた。
- **根本原因**: `skills/spec/SKILL.md`「新規作成の手順」4 のチェック項目はユーザー価値・未確定明記に限られ、テスト戦略・テスト基盤変更 Spec 向けの「ダブルを DB 観測可能性で分類する」観点が無い。

### クラスタ 4 — checker 報告形式の項目欠落(DB テスト実行環境)(単発・低)

- **パターン**: checker が test を含む `make check` を回して「全テスト成功(DB 依存テストも含む)」と報告したが、`REQUIRE_DB` の値・DB 到達性・skip 件数が報告に無く、admin が主張を検収時に検証できなかった(過大主張か事実か判別不能)。
- **該当 entry**: RETRO-006(low)
- **頻度 / 深刻度**: 頻度 1 / 深刻度 low。今回は直後の pre-commit hook が権威的に DB テストを回したため実害ゼロ。ただし hook/CI が挟まらない中間チェックでは誤前提でフェーズ前進する手戻りリスク。
- **根本原因**: `agents/checker.md`「## 報告形式」は「チェック項目 × 結果 / 自動修正内容 / 残った fail」のみで、test 結果の信頼範囲を確定させる実行環境情報を要求しない。背景に、割り振り表の checker = 「format / lint / type check」に対し Go スタックの `make check` が build + test まで含むという境界の曖昧さがある。

## 改善提案(優先度順)

### 提案 1【P1・systemic】機能完了ゲートを orchestration.md + workflow.md に追加(RETRO-002 / RETRO-003)

- **優先度**: 最優先(頻度 2・深刻度 high・複数の確定契約破壊と全 rules 横断 drift の共通根)
- **対象アセット**: `rules/workflow.md` /  `rules/orchestration.md` /(補助)`skills/issue/SKILL.md`
- **現状(引用)**:
  - `rules/workflow.md`「## パイプライン」: 「機能開発は Spec を、不具合対応は Issue を起点とする。」— 線引き定義なし
  - `rules/orchestration.md`「## admin の行動規範」の 1〜5: 5 は retro 記録のみ。機能完了時の追随確認ゲートなし
- **変更内容(具体的に)**:
  1. `rules/workflow.md` のパイプライン節に「**Spec 起点 / Issue 起点の判定基準**」を追記する: 「新しい HTTP エンドポイント・新しいドメイン集約・確定済み公開契約(env / OpenAPI / ドメインポート / DB スキーマ)の追加・変更を伴うものは **Spec 必須**。既存挙動の不具合修正・内部改善は Issue。**ロードマップ plan が機能群を Issue 分割だけで着手指示している場合は、着手前に Spec 化を先行させる**」。
  2. `rules/orchestration.md`「admin の行動規範」に新ステップ(例: 6.)を追加してゲート化する: 「機能追加を検収するとき、(i) それが確定済み Spec の記述(env 契約・CQRS ポート方針・集約構成)を破っていないか。破るなら Spec を先行更新してから進める。(ii) 完了した機能の概要が **CLAUDE.md / 該当 rules(常時ロード)/ 該当 agents 定義** の記述(集約数・コマンド・エンドポイント・担当範囲・参照先 rules 一覧)と一致しているかを確認し、drift があればドキュメント追随を適用/委譲する」。追随対象に agents/ を含める旨を明記(RETRO-003 経緯の impl-web.md 陳腐化・下記 checker.md 参照先 drift の根拠)。
  3. `skills/issue/SKILL.md`「新規作成の手順」に分岐注記を 1 行追加: 「新機能(新エンドポイント / 新集約 / 確定契約の変更)なら Issue でなく Spec に回す(判定基準の正は `rules/workflow.md`)」。判定基準そのものは workflow.md 単一情報源のまま参照に留める(二重定義回避)。
- **期待効果**: Spec 回避(002)と docs drift(003)を、発生源=検収時の 1 ゲートで両方止める。「機能追加とドキュメント追随が別々・散発・無可視」という RETRO-003 が指摘した構造そのものを解消。
- **対応 RETRO**: RETRO-002, RETRO-003(恒久策)
- **副次アクション(admin へ委譲)**: `agents/checker.md:14` の参照先 `{web,api,iac}.md` を `{web,api,auth,iac,db,testing}.md`(実在する rules 一覧)へ更新。これはゲート(ii)が拾うべき現存 drift の一例。

### 提案 2【P2】維持系 git タスクの役割とフローを orchestration.md + workflow.md に定義(RETRO-005)

- **優先度**: 高(頻度・深刻度 medium だが dependabot で反復確実)
- **対象アセット**: `rules/orchestration.md`(ホワイトリスト)/ `rules/workflow.md`(維持作業フロー)
- **現状(引用)**:
  - `rules/orchestration.md`「admin が直接行ってよいこと」: 「git の commit / push(ユーザーが指示したときのみ)」のみ。merge / config は未列挙、割り振り表に git 操作の行なし
  - `rules/workflow.md` のパイプラインは Spec/Issue 起点前提で維持タスクのフローがない
- **変更内容(具体的に)**:
  1. ホワイトリストに 1 項目追記: 「ユーザーが明示的に指示・実行した git 操作(pull / merge / branch 操作等)の完了と、それに付随する git 設定の修正。ただし競合解消が `app/` 配下の編集に及ぶ場合、解消の編集はファイル所有 stack の impl agent に委譲する(例: `app/api/go.mod` → impl-api、`app/migrator/**` → impl-db)」。
  2. `rules/workflow.md` に「維持作業の軽量フロー」節を追記: 「依存 bump / main 取り込みマージ等: admin が merge を実行 → 競合解消を stack 所有 impl に委譲(解消方針は admin が決めて指示)→ checker(対象 stack の `make check`)→ merge commit。ドメインロジックの実質的な競合のみ planner / review-* を挟む。Go スタックは `make check` が test を含むため tester を別に挟まなくてよい」。
- **期待効果**: dependabot 由来の反復タスクで、session ごとの即興ぶれ(merge 実行可否・委譲単位・パイプライン適用・commit 授権の 4 判断)を消す。
- **対応 RETRO**: RETRO-005
- **判断ポイント**: RETRO-005 提案 3 の「git-ops agent 新設」は頻度に対し過剰の可能性が高い。まず whitelist + workflow 追記で様子見が妥当(entry も同見解)。

### 提案 3【P2】Go スタックに tidy ターゲット追加 + rules コマンド表に依存解決経路を記載(RETRO-004)

- **優先度**: 高(頻度 medium・提案 2 と同じ維持タスクの一部)
- **対象アセット**: `app/{api,auth,migrator}/Makefile`(**委譲**: impl-api / impl-auth / impl-db)/ `rules/api.md`・`auth.md`・`db.md` のコマンド表
- **現状(引用)**: 3 Go Makefile に `tidy` ターゲットなし(grep 済み。`app/auth/Makefile` には「no deps/install phase exists…would need network」コメントすらある)。コマンド表(`api.md:14-22` / `auth.md:27-35` / `db.md:30-36`)に依存解決経路なし。web は `install`(network 有効 = `tools` サービス)を precedent として保有。
- **変更内容(具体的に)**:
  1. 各 Go Makefile に **`tidy` ターゲット**を追加(`go mod tidy` を network 有効の `tools` サービス経由で実行。web の `install` と同位置づけ、`--network none` の offline 検査系とは別レーン)。**Makefile 編集は admin 不可**なので impl-api / impl-auth / impl-db に委譲する。
  2. `rules/api.md` / `auth.md` のコマンド表に 1 行追加: 「依存解決(go.mod 編集・merge 競合解消後)| `make tidy`(network 到達フェーズ。生成系と同様 `make check` には含めない)」。`db.md` のコマンド表には migrator 分(`app/migrator` で `make tidy`)を追記。
- **期待効果**: go.mod に触る任意のタスクで、3 agent が compose コマンド(`versions.env` export・`TOOLBOX_*` 変数・standalone フォールバック判定)を各自再発見する重複を排除。network フェーズ取り違え(offline サービスで tidy 実行)のリスクも消える。
- **対応 RETRO**: RETRO-004
- **判断ポイント(admin)**: 本提案は admin 単独で完結しない(Makefile は impl 委譲)。RETRO-004 提案 2 の「Makefile にないコマンドの汎用実行手順を CLAUDE.md に記す」案は SPEC-009 の統制を緩めるため採らず、明示ターゲット化(提案 1)を推す(entry も同見解)。

### 提案 4【P3】skills/spec にテスト戦略変更 Spec 向けのチェック観点を追加(RETRO-001)

- **優先度**: 中(頻度 1・手戻り実績あり)
- **対象アセット**: `skills/spec/SKILL.md`(必要なら `skills/spec/template.md`)
- **現状(引用)**: `SKILL.md`「新規作成の手順」4 のチェックは「1. ユーザー価値を最初に」「未確定は明記」に限られ、テスト戦略変更 Spec 向けの分類観点がない
- **変更内容(具体的に)**: 「新規作成の手順」4(または「ルール」)に条件付きチェックを追加: 「テスト戦略・テスト基盤を変更する Spec では、既存ダブル(fake/stub/spy)を remove/replace の要件にする前に、**各ダブルが検証する性質が実基盤(実 DB 等)で観測可能かで分類**する。DB 非観測な seam(service→ポート振り分け・compile-time 型証明・実装が到達し得ないエラー分岐)を検証するダブルは、最初から**例外カテゴリ**に置き、単純置換の対象にしない。分類は `.claude/rules/testing.md` の『残してよいダブルの類型』を相互参照する」。
- **期待効果**: 同種 Spec で「狭すぎる例外条項 → レビュー逸脱指摘 → Spec 改訂 + planner 差し戻し」の 1 往復手戻り(SPEC-013 で実際に発生)を起票時に予防。
- **対応 RETRO**: RETRO-001

### 提案 5【P4】agents/checker.md 報告形式に DB テスト実行環境の必須項目を追加(RETRO-006)

- **優先度**: 低(頻度 1・severity low・実害なし)だが低コストで適用可
- **対象アセット**: `agents/checker.md`(報告形式)/(補足)`rules/orchestration.md` 割り振り表 checker 行
- **現状(引用)**: `agents/checker.md`「## 報告形式」は「チェック項目 × 結果 / 自動修正内容 / 残った fail」のみ。test を含む `make check` を回しても `REQUIRE_DB` / skip を報告する義務がない
- **変更内容(具体的に)**:
  1. checker.md 報告形式に必須項目を追加: 「test を含む check を実行した場合、**テスト実行環境**(`REQUIRE_DB` の値=未設定なら『未設定』と明記 / DB 依存テストの実行・skip の別 / 可能なら skip 件数)を報告に含める。`未設定 = skip`(意味論の正は `.claude/rules/testing.md`)の下では、この情報なしに『全テスト成功』は検収不能である」。
  2. `rules/orchestration.md` 割り振り表 checker 行に一言補足: 「Go スタックの `make check` は build + test を含むため、test 結果の報告要件も checker に適用される(詳細は `agents/checker.md`)」。tester との二重定義を避け、参照に留める。
- **期待効果**: hook/CI が挟まらない中間チェックでも「DB テスト済み」の誤前提でフェーズ前進する手戻りリスクを消す。
- **対応 RETRO**: RETRO-006

## 提案 ⇄ entry 対応表

| 提案 | 対象アセット | 対応 RETRO | 種別 |
|---|---|---|---|
| 提案 1(P1) | rules/workflow.md + rules/orchestration.md + skills/issue | RETRO-002, RETRO-003 | systemic(束ね) |
| 提案 2(P2) | rules/orchestration.md + rules/workflow.md | RETRO-005 | 維持タスク定義 |
| 提案 3(P2) | app/{api,auth,migrator}/Makefile(委譲) + rules/{api,auth,db}.md | RETRO-004 | コマンド欠落 |
| 提案 4(P3) | skills/spec/SKILL.md | RETRO-001 | チェック観点欠落 |
| 提案 5(P4) | agents/checker.md + rules/orchestration.md | RETRO-006 | 報告形式欠落 |

全 6 entry を本レポートで扱った(RETRO-003 は addressed だが恒久策=検収ゲートが未適用のため提案 1 に含めた)。各 entry の `synthesis` は `RETROSUM-001` を記入する(admin が /retro で更新)。

## 適用結果(admin が適用後に記入)

- 適用したコミット:
- 見送った提案とその理由:
- addressed にした entry:
