---
id: ISSUE-004
title: /authorize エラーリダイレクトのオープンリダイレクト不変条件がコメント規約のみで担保されている(型 or 回帰テストで機械強制すべき)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-08
updated: 2026-07-08
specs: []  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-004: /authorize エラーリダイレクトのオープンリダイレクト不変条件がコメント規約のみで担保されている(型 or 回帰テストで機械強制すべき)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth を実運用化し `/authorize` を拡張する開発者(および将来のエンドユーザー)** の **オープンリダイレクトからの安全性** が **「未検証の client_id/redirect_uri のときだけ直接エラー、それ以外は redirect」という不変条件がコメント規約のみで担保されている** ことで、将来の改修時に損なわれるおそれがある。

- **影響を受けるユーザー**: `/authorize` のエラー処理・検証順序に手を入れる将来の開発者と、その結果に晒されるエンドユーザー
- **損なわれる価値(将来条件下)**: リダイレクト先の安全性。検証前に発生しうるエラーを増やしたのに `isUnverifiedAuthorizeError` の更新を忘れると、未検証(=未登録/攻撃者制御下でありうる)の `redirect_uri` へエラーリダイレクトし、`/authorize` がオープンリダイレクタになりうる
- **影響範囲・頻度**: **現状の実装は正しく、現時点の実害はない。** 将来 `Authorize` に検証前の早期リターンを追加し、かつ `isUnverifiedAuthorizeError` の同期更新を怠った場合にのみ顕在化する退行リスク(=回帰の予防が目的)
- **回避策**: あり(不変条件を型または回帰テストで機械的に強制する。現状はコードレビューとコメント遵守で担保)

## 2. 現象(何が起きているか)

### 期待する動作

`/authorize` のエラー応答は RFC 6749 §4.1.2.1 に従い、**client_id / redirect_uri が検証できるまでは絶対に redirect せず直接エラーを表示**し、検証済みになった後のエラーのみ `redirect_uri` へエラーリダイレクトする。この不変条件が、将来 `Authorize` の検証順序を変更しても**機械的に**破れないよう保証されている状態。

### 実際の動作(現状は正しいが担保が弱い)

不変条件は次の 2 箇所の整合(=「検証前に起きうるエラー sentinel の集合」と「直接エラー扱いにする sentinel の集合」の一致)を、**コメントによる規約と目視レビュー**でのみ担保している:

- `app/auth/service/authorization_service.go` の `Authorize`(80-151 行): 「client_id と redirect_uri を最初に検証し、それ以降のエラーは redirect で報告して安全」という順序をコメント(67-79 行、98 行の `// --- client_id and redirect_uri are now verified. ---`)で規定
- `app/auth/route/response.go` の `isUnverifiedAuthorizeError`(58-63 行): 直接エラー扱いにする 4 つの sentinel(`client.ErrNotFound` / `client.ErrInvalidClientID` / `client.ErrInvalidRedirectURI` / `client.ErrRedirectURIMismatch`)を列挙

現状はこの 4 sentinel が「検証前に発生しうるエラー」と過不足なく一致しており実装は正しい。しかしコンパイラも既存テストもこの一致を強制していないため、片方だけの変更を検出できない。

### 再現手順(リスクの確認)

1. `app/auth/service/authorization_service.go:80-102` を開き、`Authorize` が (a) `ParseClientID`、(b) `FindByID`、(c) `NewRedirectURI`、(d) `ValidateRedirectURI` の順で検証し、98 行の「client_id/redirect_uri 検証済み」コメント以降でのみ他のエラー(scope・PKCE 等)を返すことを確認する。
2. `app/auth/route/response.go:58-63` の `isUnverifiedAuthorizeError` が、上記 (a)〜(d) が返す 4 sentinel(`client.ErrNotFound` / `client.ErrInvalidClientID` / `client.ErrInvalidRedirectURI` / `client.ErrRedirectURIMismatch`)と一致することを確認する。
3. 思考実験として「`Authorize` の 98 行より前に、別の新しいエラーを返す検証を挿入し、`isUnverifiedAuthorizeError` を更新しない」変更を想定する。この場合 `writeAuthorizeError`(102-134 行)は当該エラーを「検証済み後のエラー」とみなし、未検証の `redirect_uri` へリダイレクトしうる(= オープンリダイレクト)ことを確認する。

### 環境・条件

- 対象: `app/auth`(OAuth 2.0 認可サーバー / OpenID Provider 基盤サンプル、Go)
- 発見文脈: AUTH-001 基盤(`docs/plans/AUTH-001-plan.md`)のレビュー(review-security)で Minor(低優先)として挙がった、不変条件の担保強度に関する指摘
- 静的解析: review-security の報告によれば、gosec の G710(オープンリダイレクト)相当のルールが該当箇所(`writeAuthorizeError` の `http.Redirect` にユーザー由来値が渡る経路)を検出している

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: 不変条件を担保する 2 箇所(`Authorize` の検証順序 / `isUnverifiedAuthorizeError` の sentinel 集合)が別ファイルに分かれ、両者の一致はコメント規約(`authorization_service.go:67-79`、`response.go:45-54`)でのみ表現されている(`app/auth/service/authorization_service.go`、`app/auth/route/response.go:58-63`)。
- 事実: `writeAuthorizeError`(`response.go:102-134`)は `redirectURI` が非空かつ `!isUnverifiedAuthorizeError(err)` のときに `http.Redirect` する。redirect 先の妥当性はサービス層の検証済みであることを前提にしており、その前提を型システムやテストで検証していない。
- 事実: 現状の 4 sentinel は `Authorize` の検証前エラーと過不足なく一致しており、**現時点でオープンリダイレクトは発生しない**(review-security も現行実装は正しいと報告)。
- 事実: review-security の報告では gosec G710(オープンリダイレクト)相当が当該 `http.Redirect` 経路を検出している。これは現行が脆弱という意味ではなく、ユーザー由来値がリダイレクト先に流入する経路を静的解析が拾っているもの。

### 根本原因

**現行のバグではない。** オープンリダイレクト防止の不変条件(「検証前のエラーは redirect しない」)が、コンパイラやテストではなく**人間が守るコメント規約**として実装間に分散していること。将来 `Authorize` の検証順序に手を入れた際、`isUnverifiedAuthorizeError` の同期更新漏れを機械的に検出できない構造が根本原因。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: 本件は AUTH-001(`docs/plans/AUTH-001-plan.md`)の**基盤サンプル**における担保強度の指摘であり、現行実装は正しく実害はない。**今回のスコープでは対応せず**、実運用化・拡張時のハードニング候補として記録・追跡する。
- 対応する場合の候補(いずれか、または併用。仮説含む):
  - (A) **型で表現する**: 検証済みの `redirect_uri` を専用型(例: `VerifiedRedirectURI`)で表し、`writeAuthorizeError`(相当のリダイレクト経路)がその型でしかリダイレクトできないようにする。検証を経ないと型が得られないため、順序の不変条件をコンパイル時に強制できる。
  - (B) **回帰テストで不変条件を機械検証する**: 検証前に発生しうる全 sentinel を列挙し、それらがすべて `isUnverifiedAuthorizeError` で true になること(= 未検証エラーが redirect されないこと)、および検証後エラーが redirect されることを route テストで固定する。sentinel を増やしたのに列挙を更新しないとテストが落ちるようにする。
- 参照: `docs/plans/AUTH-001-plan.md`、`app/auth/service/authorization_service.go:67-151`(検証順序・コメント規約)、`app/auth/route/response.go:58-63`(`isUnverifiedAuthorizeError`)・`:95-134`(`writeAuthorizeError`)。
- 手順: 対応時は planner が方針(A/B)を選定・計画化し、impl-api が実装、tester が不変条件テストを追加、checker(`make check`)・review-security を通す。

### 実施内容

- [ ] 不変条件の機械強制方式(型 / 回帰テスト)を決定する
- [ ] (A の場合)検証済み redirect_uri を型で表現し、リダイレクト経路を型で縛る
- [ ] (B の場合)検証前 sentinel を全列挙し、未検証エラーが redirect されないことを検証する回帰テストを追加する
- [ ] gosec G710 相当の検出について、対応後の状態(抑制の妥当性 or 解消)を記録する

### 再発防止

- 「未検証の redirect_uri へリダイレクトしない」というセキュリティ不変条件を、コメントではなく型または落ちるテストで表現し、検証順序の改修時に自動で気づける状態を維持する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-08

- 起票。AUTH-001 基盤(`docs/plans/AUTH-001-plan.md`)のレビュー(review-security)で Minor(低優先)として挙がった、`/authorize` のオープンリダイレクト不変条件がコメント規約のみで担保されている点を記録。
- 事実確認: `app/auth/service/authorization_service.go:80-151` の検証順序(client_id/redirect_uri を先に検証)と、`app/auth/route/response.go:58-63` の `isUnverifiedAuthorizeError` の 4 sentinel が現状は過不足なく一致し、現行実装は正しいことを確認。将来 `Authorize` に検証前エラーを追加して列挙更新を怠るとオープンリダイレクトになりうる退行リスクとして整理。
- review-security 報告として、gosec G710(オープンリダイレクト)相当が `writeAuthorizeError` の `http.Redirect` 経路を検出している旨を記載(現行が脆弱の意味ではなく、ユーザー由来値の流入経路の検出)。
- severity は **low** と判定。判定根拠: 現行実装は正しく現時点の実害はゼロで、将来の改修時にのみ顕在化しうる予防的ハードニング。回避策(型/回帰テストでの機械強制)ありのため low(critical/high/medium ではないのは、現に安全性が損なわれていないため)。
- 次にやること: 実運用化・拡張を決めた時点で planner が型(A)/回帰テスト(B)の方式を選定・計画化し、impl-api/tester/checker/review-security で実施する。
