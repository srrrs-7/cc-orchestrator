---
id: RETRO-002
title: 新機能(AUTH-002 ロードマップ)が Spec を起こさず Issue パイプラインだけで実装され workflow.md の「機能開発は Spec 起点」原則を回避した
status: addressed  # open | addressed | wontfix
severity: high  # high(頻発・手戻り大 / タスクをブロック) | medium(回避したが非効率) | low(軽微)
source: review-spec
phase: orchestration  # spec | plan | test | impl | check | review | orchestration | other
target: rules/workflow.md
created: 2026-07-12
updated: 2026-07-13
synthesis: RETROSUM-001
tags: [ambiguous-rule, spec-vs-issue, process-bypass, doc-drift]
---

# RETRO-002: 新機能が Spec を起こさず Issue パイプラインだけで実装され「機能開発は Spec 起点」原則を回避した

## 1. 遭遇した課題(何が摩擦だったか)

> **rules/workflow.md**(および CLAUDE.md)の **「機能開発は Spec を、不具合対応は Issue を起点とする」という分離原則** が拘束力を持たず、**明らかな新機能追加が Issue パイプラインだけで実装され**、Spec が一切起票されないまま `resolved` になった。

- **具体的に何が起きたか**: `docs/plans/AUTH-002-oauth-oidc-gap-roadmap-plan.md` が「各フェーズは独立 Issue + 個別 plan(必要時)で着手可能にする」と明記し、ログイン UI / 同意 UI / RP-Initiated Logout / `/revoke` / confidential client / RSA 鍵永続化 + JWKS ローテーション / audience 分離 / client・user 管理 API という**明らかな新機能群**(ISSUE-031〜040)を、SPEC-016 以降を一切起こさずに実装・`resolved` 化した。ISSUE-031・037 などは Issue 自身のチェックリストに `- [ ] Spec 起票または後継 Spec で要件確定` と書きながら、未チェックのまま `resolved` になっている。
- **どのアセットの問題か**: 曖昧 / 欠落。workflow.md は「機能開発 = Spec 起点」と書くが、(a) 何をもって「機能開発」とし Issue で済ませてよい線引き、(b) 原則を破る plan(ロードマップ)を admin が検収で止める手順、が欠落している。

## 2. 影響(タスクにどう響いたか)

- **症状**: 誤った前提 / 仕様の陳腐化。要件・受け入れ条件が Spec として一次情報化されず、実装が唯一の真実になった。結果として SPEC-010(「user/client は Reader-only」)が ISSUE-039 の Writer 追加で矛盾し、SPEC-015 の env 契約(R13)が ISSUE-037 の AUTH_AUDIENCE 追加で陳腐化した(いずれも今回のレビューで Major として検出)。
- **コスト**: 10 件の機能(ISSUE-031〜040)分の要件が Spec に残らず、後追いレビューで仕様準拠を判定するコストが発生。確定した公開契約(env 契約・CQRS ポート方針)の破壊が Spec 経由の設計レビューを経ずに入った。

## 3. 改善提案(どう直すか)

- **rules/workflow.md**: 「Spec 起点 / Issue 起点」の判定基準を明文化する。仮説として「新しい HTTP エンドポイント・新しいドメイン集約・確定済み公開契約(env / OpenAPI / ドメインポート / DB スキーマ)の変更を伴うものは Spec 必須。既存挙動の不具合・内部改善は Issue」という線引きを追記する。
- **rules/orchestration.md**: admin の検収チェックに「plan / Issue が確定済み Spec の記述を破る場合、Spec 更新(または新 Spec)を先行させる」を明示的なゲートとして追加する。
- **skills/issue**: Issue 起票時に「これは新機能では? → Spec に回す」を促す確認を入れる(仮説: テンプレートに『新機能なら Spec へ』の分岐注記)。
- 今回の後始末として、SPEC-010 / SPEC-015 の陳腐化は admin が spec skill で更新し(ISSUE として別途起票済み)、AUTH-002 の要件を事後的に Spec 化するかは retro-synthesizer の統括で方針決定する。

## 4. 根拠 / 再現(なぜそう言えるか)

- **根拠**: `docs/plans/AUTH-002-oauth-oidc-gap-roadmap-plan.md:9`(Spec 経由を明示回避)、`docs/issues/20260712-031-auth-login-ui-idp-session.md` 末尾および `docs/issues/20260712-037-auth-resource-server-audience.md:49`(未チェックの Spec 起票項目)、`docs/specs/` に SPEC-016 以降が存在しないこと。review-spec 報告の Major #1。
- **再現条件**: 大きめの機能群をロードマップ化して着手する局面で、plan が Issue 分割だけを指示すると再現する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 記録。リポジトリ全体レビュー(security / performance / spec)の検収中、review-spec の Major #1 として surface。AUTH-002 ロードマップが Spec を経ずに 10 機能を実装し、確定済み Spec(SPEC-010 / SPEC-015)を陳腐化させていた。関連する陳腐化そのものの是正は docs/issues に起票して impl / spec skill で対応する。

### 2026-07-13(addressed へ遷移)

- RETROSUM-001 提案 1 として統括・適用(コミット `9a9de7e`): `rules/workflow.md` のパイプライン節に Spec 起点 / Issue 起点の判定基準(新エンドポイント・新集約・確定済み公開契約の変更は Spec 必須。ロードマップ plan が Issue 分割だけで着手指示していても Spec 化を先行)を追記、`rules/orchestration.md` の行動規範に機能完了ゲート(手順 6)を新設、`skills/issue/SKILL.md` の新規作成手順に「新機能なら Spec に回す」分岐注記を追加。
