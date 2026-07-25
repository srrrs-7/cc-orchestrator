---
id: ISSUE-053
title: app/auth の RP-initiated logout(GET /logout)に CSRF 検証が無く、強制ログアウト CSRF が可能
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-053: app/auth の RP-initiated logout(GET /logout)に強制ログアウト CSRF がある

**深刻度: Minor(review) / severity: low**(セッション終了のみの nuisance レベル。資格情報漏えいは伴わない)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth にログイン済みのユーザー** の **セッション継続** が **攻撃者の仕込んだリンク経由で意図せず終了させられる(強制ログアウト)**。

- **影響を受けるユーザー**: IdP セッションを保持している全ユーザー
- **損なわれる価値**: セッションの継続性(利便性)。※データ漏えい・乗っ取りは伴わない
- **影響範囲・頻度**: 攻撃者リンクを踏んだ場合に発生(nuisance レベル)
- **回避策**: なし(GET + Cookie のみで成立するため利用者側の回避は困難)

## 2. 現象(何が起きているか)

### 期待する動作

ログアウトは CSRF に耐性を持つ(確認画面を挟む、または POST + CSRF トークン検証)。第三者サイトからのリンク誘導だけでは強制ログアウトできない。

### 実際の動作

`app/auth/route/logout_handler.go` の `/logout` は GET + Cookie のみで、CSRF トークン検証が無い。セッション Cookie は `SameSite=Lax`(`app/auth/route/session_cookie.go`)で、Lax はトップレベル GET ナビゲーションに Cookie を同送するため、`<a href="https://.../auth/logout">`(または画像等のトップレベル遷移)で victim を強制ログアウトさせられる。影響はセッション終了のみ(nuisance レベル)。

### 再現手順

1. `app/auth/route/logout_handler.go` が GET + Cookie のみで CSRF 検証が無いことを確認する。
2. `app/auth/route/session_cookie.go` の Cookie が `SameSite=Lax` であることを確認する。
3. ログイン済み victim に `<a href="https://<domain>/auth/logout">` を踏ませると(トップレベル GET)、Cookie が同送され強制ログアウトされる。

### 環境・条件

- 対象: app/auth の RP-initiated logout(GET /logout)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/auth/route/logout_handler.go` の /logout は GET + Cookie のみで CSRF トークン検証が無い。
- 事実: `app/auth/route/session_cookie.go` の Cookie は `SameSite=Lax`(トップレベル GET に同送)。
- 仮説: RP-initiated logout を GET ナビゲーションで受ける OIDC の慣習に沿ったが、CSRF 対策(確認画面 / POST 化)を未導入。

### 根本原因

/logout が GET + Cookie のみで CSRF 検証を持たず、SameSite=Lax がトップレベル GET に Cookie を同送するため、第三者リンクで強制ログアウトが成立する。

## 4. 対応(どう解決するか)

### 対応方針

impl-auth が、ログアウトに確認画面を挟む、または POST + CSRF トークン化する。OIDC RP-initiated logout(end-session)の仕様(state / 確認)との整合も考慮する。

### 実施内容

- [ ] /logout に確認画面を追加、または POST + CSRF token 検証に変更(impl-auth)
- [ ] 第三者リンクの GET だけでは強制ログアウトできないことをテストで確認(tester)

### 再発防止

- 状態変更を伴うエンドポイントは GET + Cookie のみで受けない(CSRF 観点をレビューに追加)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。セキュリティレビューで検出。`app/auth/route/logout_handler.go` が GET + Cookie のみで CSRF 検証が無いこと、`session_cookie.go` が SameSite=Lax であることを確認した。影響はセッション終了のみ(nuisance)。
- 関連: SPEC-015(RP-initiated logout は ISSUE-033 で実装)。
