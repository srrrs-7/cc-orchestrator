---
id: ISSUE-044
title: app/auth の IdP セッション in-memory store に TTL purge が無く、未ログイン離脱で pending が無制限増加し OOM に至る
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: high  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-044: app/auth の IdP セッション in-memory store に TTL purge が無く、pending 無制限増加で OOM に至る

**深刻度: Major(review) / severity: high**(メモリ枯渇による auth サービス停止 = 認証全体の可用性喪失)

## 1. ユーザー価値への影響(なぜ対応するか)

> **全ユーザー** の **ログイン・認可(サービス全体の利用)** が **auth プロセスの OOM 停止により断続的に失われる**。

- **影響を受けるユーザー**: app/auth に依存する全ユーザー(ログイン・認可・トークン発行が止まる)
- **損なわれる価値**: 認証・認可の可用性
- **影響範囲・頻度**: 未ログインでの `GET /authorize` が継続的に発生する環境で、時間とともに顕在化(bot/クローラ・二重クリック・戻るボタン再送でも増加)
- **回避策**: プロセス再起動で一時解消するが、根本解決にならない

## 2. 現象(何が起きているか)

### 期待する動作

IdP セッションの in-memory store は、TTL 切れの `sessions`(24h)/ `pending`(10min)エントリをバックグラウンドで定期 purge し、メモリ使用量が有界に保たれる(authcode/refreshtoken の purge ticker と同型)。

### 実際の動作

`app/auth/infra/memory/idp_session_store.go:18-125` の `sessions` / `pending` map には、バックグラウンドの一括 purge が無い。削除経路は「同一キー再読時の遅延削除(lazy)」または「明示削除」のみ。

未ログインでの `GET /authorize`(初回・二重クリック・bot/クローラ・戻るボタン再送)は毎回 `pending` に 1 件追加する(`app/auth/route/authorize_handler.go:125,159,180,207`、`app/auth/service/authentication_service.go:34-53`)。ログイン未完了で離脱すると `ConsumePendingAuthorize` が呼ばれず、同じ pending ID で再アクセスされない限りエントリが残り続ける。

持続 10 req/s では 1 日約 86 万件の純増となり、数百 MB〜GB 規模まで膨張する。dev の `auth_task_memory = 512MiB` では数時間〜数日で OOM に至る。

### 再現手順

1. 未ログイン状態で `GET /authorize?...`(有効なパラメータ)を高頻度に繰り返す(ログインは完了させない)。
2. 各リクエストで `pending` に 1 件追加され、消費されないまま残留する。
3. プロセスの RSS が単調増加し、`auth_task_memory` 上限で OOM kill される。

### 環境・条件

- 対象: in-memory の IdP セッションストアを使う構成(SPEC-011 で他集約は Postgres 化されているが idpsession は in-memory)。dev の 512MiB タスクで顕著。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/auth/infra/memory/idp_session_store.go:18-125` の `sessions` / `pending` map には purge ticker / バックグラウンドスイープが無い。
- 事実: 削除は lazy(再読時)か明示削除のみ。`pending`(TTL 10min)は未消費で離脱すると再アクセスされない限り残る。
- 事実: authcode(ISSUE-015)/ refreshtoken(ISSUE-019)は index + 15 分毎の purge ticker により解消済みだが、idpsession には同種機構が無い。
- 仮説: 認可コード / refresh token を Postgres + expires_at index + purge ticker で有界化した際、in-memory の idpsession が同じ堅牢化の対象から漏れた。

### 根本原因

IdP セッションの in-memory store に TTL ベースの定期 purge が無く、未消費エントリが単調増加する。

## 4. 対応(どう解決するか)

### 対応方針

impl-auth が in-memory store に定期 purge を追加する。恒久策として authcode/refreshtoken と同じ Postgres + `expires_at` index + purge ticker パターンへ寄せる案も検討する。

### 実施内容

- [ ] `IdPSessionStore` に `runPurgeTicker` 相当のバックグラウンドスイープを追加(sessions / pending の TTL 切れを定期削除)、または書き込み時の lazy sweep を追加(impl-auth)
- [ ] 恒久策: idpsession を Postgres 化(expires_at index + purge ticker)する案の要否を検討(impl-db 連携。要れば別 Spec/Issue 化)
- [ ] purge が効いてメモリが有界化することをテストで確認(tester)

### 再発防止

- 「TTL を持つ in-memory 集約は必ず定期 purge を持つ」を横断規約として点検する(authcode/refreshtoken/idpsession の 3 集約で一貫させる)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。パフォーマンス / セキュリティレビューで検出。`app/auth/infra/memory/idp_session_store.go:18-125` に purge 機構が無いこと、pending の追加経路(`authorize_handler.go:125,159,180,207`)と未消費残留を確認した。authcode(ISSUE-015)/ refreshtoken(ISSUE-019)の purge ticker と対比した。
- 関連: ISSUE-015(認可コード無制限増加、resolved)、ISSUE-019(refresh_token deferred hardening、resolved)、SPEC-011(Postgres 一本化)、SPEC-015。
