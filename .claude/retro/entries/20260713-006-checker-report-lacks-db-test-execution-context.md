---
id: RETRO-006
title: checker の報告に DB 依存テストの実行環境(REQUIRE_DB / skip 有無)がなく検収で主張を検証できない
status: open
severity: low
source: checker
phase: check
target: agents/checker.md  # 併記: orchestration.md 割り振り表の checker/tester 境界(make check が test を含む)
created: 2026-07-13
updated: 2026-07-13
synthesis:
tags: [report-format, missing-field, db-tests, require-db]
---

# RETRO-006: checker の報告に DB 依存テストの実行環境(REQUIRE_DB / skip 有無)がなく検収で主張を検証できない

## 1. 遭遇した課題(何が摩擦だったか)

> **agents/checker.md の報告形式** に **テスト実行環境(`REQUIRE_DB` の値・DB 依存テストの実行 / skip)の明示義務が欠落**しており、**admin** が **checker 報告の「全テスト成功(DB 依存テストも含む)」という主張を検収時に検証できなかった**。

- **具体的に何が起きたか**: origin/main マージの commit 前検証で checker に api / auth / migrator の `make check` を委譲した(指示で「REQUIRE_DB は設定しない(DB 依存テストは skip でよい)」と明記)。返ってきた報告は「全テスト成功(**DB 依存テストも含む**)」と主張したが、報告には `REQUIRE_DB` の値・DB への到達性・skip されたテスト数が一切含まれず、この主張が事実(たまたま postgres コンテナが稼働中で実行された)なのか過大主張(実際は skip)なのか、報告からは判別できなかった。仮説: 当時 `cc-orchestrator-postgres-1` は稼働中だったため実際に実行された可能性はあるが、未確認
- **どのアセットの問題か**: 欠落(報告形式に、テスト結果の信頼範囲を確定させる実行環境情報がない)。背景に、割り振り表の checker = 「format / lint / type check」に対して Go スタックの `make check` 契約が build + test まで含むという**境界の曖昧さ**もある(checker が test 結果を報告する立場になるが、その報告要件は tester ほど定義されていない)

## 2. 影響(タスクにどう響いたか)

- **症状**: 検収時の検証不能(誤った前提のリスク)。今回は直後の pre-commit hook が db-test フェーズ(`api_test` / `auth_test` + no-internet)を権威的に実行したため実害なし
- **コスト**: 今回ゼロ。ただし hook / CI が挟まらない検証(レビュー前の中間チェック等)で同じ過大主張が起きると、「DB 依存テスト済み」という誤った前提でフェーズを進める手戻りリスクがある

## 3. 改善提案(どう直すか)

1. **agents/checker.md の報告形式に必須項目を追加**: test を含む check を実行した場合、「テスト実行環境: `REQUIRE_DB` の値(未設定なら未設定と書く)/ DB 依存テストの実行・skip の別(可能なら skip 件数)」を報告に含める。`未設定 = skip` の意味論(`.claude/rules/testing.md` が正)の下では、この情報なしに「全テスト成功」は検収不能であることを明記する
2. 仮説: orchestration.md の割り振り表 checker 行の補足に「Go スタックの `make check` は build + test を含むため、test 結果の報告要件(上記)も checker に適用される」と境界を一言明記する(tester との二重定義は避け、checker.md への参照でよい)

## 4. 根拠 / 再現(なぜそう言えるか)

- **根拠**: checker 報告の原文「全テスト成功(DB 依存テストも含む)」(origin/main マージ検証タスク、2026-07-12)。admin の委譲指示には「REQUIRE_DB は設定しない(DB 依存テストは skip でよい。CI で検証される)」と明記していた。`REQUIRE_DB=1 = fail / 未設定 = skip` の意味論は CLAUDE.md / `.claude/rules/testing.md` に定義
- **再現条件**: checker に Go スタックの `make check`(test 含む)を委譲し、DB 依存テストの成否が検収の関心事であるとき

## 5. 経緯(時系列・追記のみ)

### 2026-07-13

- 記録。origin/main マージタスクの振り返り(/retro)で、checker 報告の主張と委譲指示(REQUIRE_DB 未設定)の不整合に気づき吸い上げた
