---
id: ISSUE-052
title: RSA 署名鍵永続化(ISSUE-036)の IaC 配線が未完了のまま resolved になっており、本番相当デプロイは依然プロセス起動毎に使い捨て鍵を生成する
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-052: RSA 署名鍵永続化(ISSUE-036)の IaC 配線が未完了のまま resolved

**深刻度: Minor(review) / severity: medium**(ISSUE-036(high)の必須項目 IaC 配線が未完で、本番相当では鍵永続化が実質未達)

## 1. ユーザー価値への影響(なぜ対応するか)

> **本番相当環境の全ユーザー** の **ログインセッションの継続性** が **auth プロセス再起動 / デプロイ毎の RSA 署名鍵入れ替えにより失われ(発行済みトークンが一斉に検証不能になり)得る**。

- **影響を受けるユーザー**: 本番相当環境(ECS)の全ユーザー
- **損なわれる価値**: 発行済みトークンの継続的な検証可能性(セッション継続性)。鍵ローテーション設計の前提
- **影響範囲・頻度**: auth プロセスの再起動 / 再デプロイ / スケールアウト(複数タスクが別々の鍵を持つ)ごとに顕在化
- **回避策**: なし(IaC 配線が入るまで本番相当では使い捨て鍵のまま)

## 2. 現象(何が起きているか)

### 期待する動作

ISSUE-036 の設計どおり、本番相当デプロイでは Secrets Manager 等に格納した鍵リングを app/auth に注入し、プロセス再起動・スケールアウトを跨いで同一の RSA 署名鍵(および JWKS 複数鍵ローテーション)を用いる。

### 実際の動作

`app/iac/envs/dev/main.tf` の `module.service_auth` に `SIGNING_KEYS_FILE` 相当の env / Secrets Manager 参照が無い(確認: service_auth の environment/secrets に鍵リング注入が存在しない)。

そのため本番相当デプロイは依然として **プロセス起動毎に使い捨ての RSA 鍵を生成する**(app/auth の `buildKeyRingLoader` フォールバック経路)。

加えて ISSUE-036 の Issue ファイル(`docs/issues/20260712-036-*.md`)は、実施内容チェックリストの「compose / IaC env 契約」項目が**未チェックのまま `status: resolved`** になっている。IaC 配線という必須項目を残したまま resolved 扱いされている。

### 再現手順

1. `app/iac/envs/dev/main.tf` の `module.service_auth` に `SIGNING_KEYS_FILE` 相当の env / Secrets Manager 参照が無いことを確認する。
2. `docs/issues/20260712-036-*.md` の実施内容チェックリストで「compose / IaC env 契約」が未チェックかつ `status: resolved` であることを確認する。
3. 本番相当デプロイで auth を再起動すると、以前発行したトークンの署名検証が失敗する(鍵が入れ替わる)。

### 環境・条件

- 対象: app/iac(service_auth)+ ISSUE-036 の未完了項目。本番相当デプロイで顕在化。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/iac/envs/dev/main.tf` の `module.service_auth` に鍵リング注入(SIGNING_KEYS_FILE 相当)が無い。
- 事実: app/auth は鍵リング未注入時に `buildKeyRingLoader` フォールバックで使い捨て鍵を生成する。
- 事実: ISSUE-036 の実施内容チェックリストの「compose / IaC env 契約」が未チェックのまま resolved。
- 仮説: ISSUE-036 のアプリ層(永続化ローダ)は実装したが、IaC 配線(Secrets Manager + env 注入)が残ったまま resolved にされた。

### 根本原因

鍵永続化のアプリ実装は入ったが、本番で鍵リングを供給する IaC 配線が未完了で、フォールバック(使い捨て鍵)経路が本番相当で発火する。

## 4. 対応(どう解決するか)

### 対応方針

impl-iac が Secrets Manager に鍵リング JSON を格納し、`module.service_auth` の secrets に `SIGNING_KEYS_FILE`(相当)を注入する。あわせて ISSUE-036 の未完了必須項目としてトラッキングする。

### 実施内容

- [ ] Secrets Manager に鍵リング JSON を格納するリソースを追加(impl-iac)
- [ ] `module.service_auth` の secrets / environment に `SIGNING_KEYS_FILE` 相当を注入(impl-iac)
- [ ] app/auth が起動時に注入された鍵リングをロードし、フォールバック(使い捨て鍵)に落ちないことを確認
- [ ] ISSUE-036 の実施内容チェックリストの「compose / IaC env 契約」を本 Issue の完了に合わせて更新(admin / issue-creator)
- [ ] apply は行わず plan 結果を報告する

### 再発防止

- 「アプリ層の永続化」実装は、対応する IaC 配線が済むまで resolved にしない(チェックリスト未チェック項目がある間は resolved 禁止)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。セキュリティ / 仕様準拠レビューで検出。`app/iac/envs/dev/main.tf` の service_auth に鍵リング注入が無いこと、ISSUE-036 の実施内容チェックリスト「compose / IaC env 契約」が未チェックのまま resolved であることを確認した。本番相当では buildKeyRingLoader フォールバックで使い捨て鍵になる。
- 関連: ISSUE-036(RSA 署名鍵の永続化と JWKS 複数鍵ローテーション、resolved だが IaC 配線が未完)、SPEC-015。
