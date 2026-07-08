---
id: ISSUE-003
title: /authorize が response_type の欠落と非対応値を区別せず一律 unsupported_response_type を返す(RFC 6749 4.1.2.1 の厳密区別)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-08
updated: 2026-07-08
specs: []  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-003: /authorize が response_type の欠落と非対応値を区別せず一律 unsupported_response_type を返す(RFC 6749 4.1.2.1 の厳密区別)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth を OAuth 2.0 / OIDC クライアントで実運用化する開発者** の **公式仕様(RFC 6749)への準拠の厳密さ** が **`/authorize` が必須パラメータ `response_type` の欠落と非対応値を同じエラーコードで返すことで、わずかに損なわれる**。

- **影響を受けるユーザー**: app/auth を基盤として実運用化し、仕様準拠のクライアント(エラーコードで分岐する RP / 適合性テスト)を接続する開発者
- **損なわれる価値**: OAuth エラーレスポンスの仕様適合性。`response_type` を省いた不正リクエストと、`token` 等の非対応値を指定したリクエストが同じ `unsupported_response_type` になるため、クライアントが「パラメータ不足(直すべき)」と「フローが非対応(サーバが対応していない)」を区別できない
- **影響範囲・頻度**: **現時点(基盤サンプル)では実害なし。** 稼働中のバグではなく、公式仕様の厳密区別としての将来対応候補。基盤サンプルの目的(拡張可能な認可サーバーの提示)は本件が未対応でも達成される
- **回避策**: あり(クライアント側は `error_description` の文言で区別可能。サーバ側は本 Issue の対応で厳密化)

## 2. 現象(何が起きているか)

### 期待する動作

RFC 6749 §4.1.2.1 のエラーコード規定:

- 必須パラメータ(`response_type`)が **欠落** している場合 → `invalid_request`
- `response_type` の値が **サーバで非対応** の場合(例: `token` / `id_token`)→ `unsupported_response_type`

したがって、空の `response_type`(パラメータ欠落)に対しては `invalid_request` を返すのが期待動作。

### 実際の動作

`response_type` が `"code"` 以外(空文字を含む)であれば、値の欠落・非対応を区別せず一律 `client.ErrUnsupportedResponseType` を返し、`route/response.go` がこれを `unsupported_response_type` にマップする。空の `response_type` でも `unsupported_response_type` が返る。

### 再現手順

1. `app/auth/service/authorization_service.go:100` を開き、次の分岐を確認する:
   ```go
   if req.ResponseType != responseTypeCode || !c.SupportsResponseType(responseTypeCode) {
       return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", client.ErrUnsupportedResponseType)
   }
   ```
   `req.ResponseType` が空("")の場合も `!= responseTypeCode`(= `"code"`)が真になり、`ErrUnsupportedResponseType` を返すことを確認する。
2. `app/auth/route/response.go:80-81` の `authorizeErrorCode` を開き、`client.ErrUnsupportedResponseType` が `("unsupported_response_type", "only response_type=code is supported")` にマップされることを確認する。
3. 結果として、`response_type` を省いたリクエストと `response_type=token` のリクエストがいずれも `unsupported_response_type` になり、`invalid_request` にならないことを確認する。

### 環境・条件

- 対象: `app/auth`(OAuth 2.0 認可サーバー / OpenID Provider 基盤サンプル、Go)
- 発見文脈: AUTH-001 基盤(`docs/plans/AUTH-001-plan.md`)のレビュー(review-spec)で Minor として挙がった、公式仕様との厳密区別の乖離点

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `authorization_service.go:100` は `req.ResponseType != responseTypeCode || !c.SupportsResponseType(responseTypeCode)` を単一の分岐で扱い、欠落と非対応値の両方を `client.ErrUnsupportedResponseType` にまとめている(`app/auth/service/authorization_service.go:100-102`)。
- 事実: `route/response.go:80-81` はこの sentinel を `unsupported_response_type` にマップするのみで、欠落を `invalid_request` に振り分けるパスがない(`app/auth/route/response.go:70-93`)。
- 事実: 計画は基盤簡略化として `response_type=code` のみ対応・それ以外を非対応とする方針を採用済み(`docs/plans/AUTH-001-plan.md` の「退けた代替案」で Implicit grant を不採用と記載、トレーサビリティ表 #1 で認可リクエスト必須パラメータを定義)。現状の一律マップはこの簡略化の範囲内である。
- 事実: RFC 6749 §4.1.2.1 は「欠落 = `invalid_request`」「非対応値 = `unsupported_response_type`」を明確に区別している。

### 根本原因

**退行バグではない。** 基盤サンプルとして「`response_type=code` 以外は受理しない」を単一分岐で簡潔に表現した設計判断により、必須パラメータの「欠落(`invalid_request`)」と「非対応値(`unsupported_response_type`)」という RFC 6749 §4.1.2.1 の 2 分類が 1 つに畳まれている。基盤サンプルとしては許容範囲だが、公式仕様の厳密準拠としては区別が必要になる。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: 本件は AUTH-001(`docs/plans/AUTH-001-plan.md`)の**基盤サンプル**における仕様簡略化であり、稼働中の実害はない。**今回のスコープでは対応せず**、実運用化・仕様厳密化の際の将来対応候補として記録・追跡する。
- 対応する場合の方針(仮説含む): `app/auth/service/authorization_service.go:100` の分岐を、`req.ResponseType == ""`(欠落)と `req.ResponseType != "code"`(非対応値)で分け、前者に新規 sentinel(例: `client.ErrMissingResponseType` 相当)を割り当てる。`app/auth/route/response.go` の `authorizeErrorCode` で欠落 sentinel を `invalid_request` に、非対応値を従来どおり `unsupported_response_type` にマップする。
- 参照: `docs/plans/AUTH-001-plan.md`(トレーサビリティ表 #1「認可リクエスト REQUIRED」)、`app/auth/service/authorization_service.go:100-102`、`app/auth/route/response.go:80-81`。
- 手順: 対応時は planner が計画化し、impl-api が実装、tester が「空 `response_type` → `invalid_request`」「非対応値 → `unsupported_response_type`」の両ケースを route の httptest で追加、checker(`make check`)・review-spec を通す。

### 実施内容

- [ ] `response_type` 欠落用の sentinel を追加し、非対応値と分離する
- [ ] `route/response.go` で欠落 → `invalid_request`、非対応値 → `unsupported_response_type` にマップ
- [ ] 空 `response_type` → `invalid_request` を検証する route テストを追加

### 再発防止

- OAuth / OIDC のエラーレスポンスは「欠落 = `invalid_request`」「値が非対応 = 各専用コード」という RFC 6749 §5.2 / §4.1.2.1 の区別を、エラーマッピングのテストで固定する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-08

- 起票。AUTH-001 基盤(`docs/plans/AUTH-001-plan.md`)のレビュー(review-spec)で Minor として挙がった、`/authorize` が `response_type` の欠落と非対応値を区別しない点を記録。
- 事実確認: `app/auth/service/authorization_service.go:100-102` が欠落・非対応値を一律 `client.ErrUnsupportedResponseType` にまとめ、`app/auth/route/response.go:80-81` が `unsupported_response_type` にマップすることを確認。RFC 6749 §4.1.2.1 では欠落 = `invalid_request`、非対応値 = `unsupported_response_type`。
- severity は **low** と判定。判定根拠: 稼働中の実害はなく(基盤サンプルは `response_type=code` のみを主軸に提示)、クライアントは `error_description` で区別可能なため回避策あり。基盤サンプルの目的達成を妨げない仕様厳密化のため low(critical/high/medium ではないのは機能・価値が現に損なわれていないため)。
- 次にやること: 実運用化・仕様厳密化を決めた時点で planner に計画化を依頼し、欠落/非対応値の分離と route テスト追加を impl-api/tester/checker/review-spec で実施する。
