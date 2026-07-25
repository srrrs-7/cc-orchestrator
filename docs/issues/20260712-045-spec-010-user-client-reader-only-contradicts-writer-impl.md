---
id: ISSUE-045
title: SPEC-010 R1 の「user/client は Reader-only、空 Writer は作らない」記述が、ISSUE-039 で追加された Writer 実装と矛盾している(仕様の陳腐化)
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-010]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-045: SPEC-010 R1 の「user/client は Reader-only」記述が実装(Writer 追加)と矛盾している

**深刻度: Major(review) / severity: low**(実装は正常。仕様ドキュメントの陳腐化により将来の実装者を誤誘導するリスク)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth の開発者・レビュアー** が **一次情報である SPEC-010 を信頼して実装・レビューする際に**、**確定設計と現実装の矛盾により誤った前提で判断してしまう**。

- **影響を受けるユーザー**: 開発者・レビュアー(エンドユーザーへの直接影響はない)
- **損なわれる価値**: 仕様ドキュメントの正確性・信頼性(一次情報としての価値)
- **影響範囲・頻度**: SPEC-010 を参照して user/client の永続化層を扱うときに顕在化
- **回避策**: あり(実装を確認すれば実態は分かる)。ただし「仕様を正とする」原則(project.md)に反する状態

## 2. 現象(何が起きているか)

### 期待する動作

一次情報である SPEC-010 の記述が現実装と一致する。設計が変わった場合は経緯セクションに上書きの記録が残る。

### 実際の動作

`docs/specs/20260710-010-db-cqrs-read-write-separation.md` の R1 は「user/client は Reader-only、空 Writer は作らない」を**確定設計**として明記している。

一方、ISSUE-039(client/user 管理 API)の対応で `app/auth/infra/postgres/client_writer.go` / `user_writer.go` が実追加され、user/client が Writer を持つに至った。SPEC-010 の R1 記述と現実装が矛盾し、SPEC-010 の経緯セクションにこの上書きの記録が無い。

### 再現手順

1. `docs/specs/20260710-010-db-cqrs-read-write-separation.md` の R1 を読む(「user/client は Reader-only、空 Writer は作らない」)。
2. `app/auth/infra/postgres/` に `client_writer.go` / `user_writer.go` が存在することを確認する。
3. 記述と実装が矛盾していることを確認する。

### 環境・条件

- ドキュメント上の矛盾(実行時の不具合ではない)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: SPEC-010 R1 が user/client の Reader-only を確定設計として記述。
- 事実: ISSUE-039 で `client_writer.go` / `user_writer.go` が追加され、user/client が Writer を持つ。
- 事実: SPEC-010 の経緯セクションにこの設計変更の記録が無い。
- 仮説: ISSUE-039 の対応時に、CQRS の Reader-only 前提を覆したことを SPEC-010 側に反映し忘れた。

### 根本原因

ISSUE-039 の実装で SPEC-010 R1 の前提が覆ったが、SPEC-010 側の記述・経緯が更新されず陳腐化した(実装が正・仕様が古い)。

## 4. 対応(どう解決するか)

### 対応方針

**これは仕様側の陳腐化であり、修正は該当 Spec(SPEC-010)の更新**で行う。R1 を「ISSUE-039 により user/client が Writer を持つに至った」事実へ更新し、経緯セクションに上書きの記録を追記する。

> 注記: 本 Issue の修正は、admin が `spec` skill で SPEC-010 を更新して閉じる予定。issue-creator / impl agent によるコード変更は不要(コードは正しい)。

### 実施内容

- [ ] admin が `spec` skill で SPEC-010 R1 を現実装(user/client が Writer を持つ)へ更新
- [ ] SPEC-010 の経緯セクションに、ISSUE-039 による Reader-only 前提の上書きを追記
- [ ] 更新後、本 Issue を resolved に更新

### 再発防止

- CQRS のポート構成(Reader-only か Reader+Writer か)を変える実装は、起点 Spec(SPEC-010)の経緯更新を必須とする。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。仕様準拠レビューで検出。SPEC-010 R1 の Reader-only 記述と、ISSUE-039 で追加された `app/auth/infra/postgres/client_writer.go` / `user_writer.go` の矛盾を確認した。SPEC-010 の経緯に上書きの記録が無い。
- 関連: ISSUE-039(client/user 管理 API、resolved)、SPEC-010。修正は admin の spec skill による SPEC-010 更新で行う。

### 2026-07-12 (resolved)

- 修正(admin + spec skill): `docs/specs/20260710-010-db-cqrs-read-write-separation.md` の R1 該当 bullet に ⚠️ 注記を追加し、ISSUE-039 で user/client に Writer(`client_writer.go` / `user_writer.go`)が追加された事実を明記。§6 経緯 2026-07-12 に是正理由(実装を正としてドキュメントを現実に同期・コア設計は不変)を追記した。SPEC-010 の status は done 維持。
- 検証: 仕様側の陳腐化のみで実装は正常(ドキュメントレベルの是正)。実装と一次情報の矛盾が解消したため Major(仕様準拠)解消につき resolved。
