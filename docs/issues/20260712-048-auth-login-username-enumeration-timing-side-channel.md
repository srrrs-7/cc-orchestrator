---
id: ISSUE-048
title: app/auth の /login にユーザー名列挙を許すタイミングサイドチャネルがある(未知ユーザーは bcrypt を経由せず即返す)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-048: app/auth の /login にユーザー名列挙を許すタイミングサイドチャネルがある

**深刻度: Minor(review) / severity: low**(応答時間差によるユーザー名存在推測。前段の情報漏えい)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth のユーザー** の **アカウント存在の非開示** が **ログイン応答の時間差により推測され、標的型攻撃の下地を与える**。

- **影響を受けるユーザー**: app/auth に登録された全ユーザー
- **損なわれる価値**: ユーザー名(アカウント存在)の秘匿性
- **影響範囲・頻度**: /login に対する計測攻撃で顕在化(直接の資格情報漏えいではない)
- **回避策**: なし(応答時間差はコード構造に依存)

## 2. 現象(何が起きているか)

### 期待する動作

ユーザーが存在するか否かにかかわらず、`/login` の応答時間が概ね一定になり、時間差からユーザー名の存在を推測できない。

### 実際の動作

`app/auth/service/authentication_service.go:34-54`(`Login`)では、未知ユーザーは即座に `ErrInvalidCredentials` を返す一方、既存ユーザーの誤パスワードは bcrypt 比較(数十〜百 ms)を経由する。この応答時間差により、ユーザー名の存在を推測できる。

refreshTokenGrant は ISSUE-019 で定数時間フロア(10ms)を持つが、Login には同種の対策が無い。

### 再現手順

1. 存在しないユーザー名で `/login` に POST し、応答時間を計測する(短い)。
2. 存在するユーザー名 + 誤パスワードで `/login` に POST し、応答時間を計測する(bcrypt 分だけ長い)。
3. 時間差からユーザー名の存在を判別できることを確認する。

### 環境・条件

- 対象: app/auth の /login(ログイン認証)経路。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/auth/service/authentication_service.go:34-54` の `Login` は未知ユーザーで bcrypt を経由せず即返す。
- 事実: 既存ユーザー誤 PW は bcrypt(数十〜百 ms)を経由する。
- 事実: refreshTokenGrant(ISSUE-019)は定数時間フロアを持つが Login には無い。
- 仮説: ユーザー未存在時に早期 return する自然な実装が、タイミング差を生んでいる。

### 根本原因

未知ユーザーのパスに bcrypt 相当の計算コストが無く、既存ユーザーパスとの応答時間差が生じる。

## 4. 対応(どう解決するか)

### 対応方針

impl-auth が `ErrNotFound` パスにダミー bcrypt 比較を入れる、または定数時間フロアを追加してユーザー有無で時間差が出ないようにする。

### 実施内容

- [ ] `Login` の未知ユーザーパスにダミー bcrypt 比較 or 定数時間フロアを追加(impl-auth)
- [ ] 未知/既存ユーザーで応答時間差が有意に縮まることをテストで確認(tester)

### 再発防止

- 資格情報検証パスは「存在有無で分岐せず一定コスト」を規約にする(refreshTokenGrant と一貫させる)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。セキュリティレビューで検出。`app/auth/service/authentication_service.go:34-54` の Login が未知ユーザーで bcrypt を経由しないことを確認した。refreshTokenGrant(ISSUE-019)の定数時間フロアと対比した。
- 関連: ISSUE-019(refresh_token deferred hardening、resolved)、SPEC-015。
