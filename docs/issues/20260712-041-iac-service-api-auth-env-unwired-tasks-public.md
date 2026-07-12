---
id: ISSUE-041
title: app/iac の service_api に認証 env 3 変数(AUTH_ISSUER/AUTH_JWKS_URL/AUTH_AUDIENCE)が未配線で、terraform apply された Task API が無認証で公開される
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: critical  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-041: app/iac の service_api に認証 env 3 変数が未配線で、terraform apply された Task API が無認証で公開される

**深刻度: Blocker(review) / severity: critical**(本番デプロイで全 task エンドポイントが認証なしに公開される)

## 1. ユーザー価値への影響(なぜ対応するか)

> **Task Manager を利用する全ユーザー** の **自分のタスクの機密性・完全性(他人に見られない・改ざんされない)** が **本番相当デプロイでは完全に失われる**。

- **影響を受けるユーザー**: AWS(ECS/ALB/CloudFront)にデプロイされた環境の全ユーザー
- **損なわれる価値**: タスクデータの機密性・完全性・アクセス制御。誰でも他人のタスクの閲覧・作成・変更・削除が可能
- **影響範囲・頻度**: 常時(terraform apply された環境で全 `/tasks*` エンドポイントが公開 ALB/CloudFront 経由で無認証到達可能)
- **回避策**: なし(env を配線して再デプロイしない限り常時公開)

## 2. 現象(何が起きているか)

### 期待する動作

SPEC-015 R12/R13 の設計どおり、本番相当デプロイでも app/api の全 task エンドポイントが Bearer JWT 必須(AuthMiddleware 経由)になる。認証設定が欠けていれば起動エラー等で気付ける。

### 実際の動作

`app/iac/envs/dev/main.tf` の `module "service_api"` の `environment` には `PORT` / `DB_HOST` / `DB_PORT` / `DB_NAME` / `DB_SSLMODE` のみが設定され、`AUTH_ISSUER` / `AUTH_JWKS_URL` / `AUTH_AUDIENCE` が一切設定されていない(確認済み: main.tf:135-141 の environment ブロック)。

app/api の `Env.validate()`(`app/api/cmd/api/env.go:157-186`)は 3 変数が「全て設定」または「全て未設定」のいずれかなら正常系として許容する(`noneSet` を許可する dev opt-out)。3 変数とも未設定のため validate は通り、`authEnabled()`(env.go:183-186)は false を返す。結果、ECS タスクは起動エラーにならず、AuthMiddleware を経由せずに起動する(`app/api/cmd/api/main.go:85-96`)。

その結果、`/tasks*` 全エンドポイントが CloudFront → ALB → app/api 経由で **無認証で到達可能**になる。terraform plan の diff にも認証 env の欠落は現れないため気付きにくい。

### 再現手順

1. `app/iac/envs/dev/main.tf` の `module "service_api"` の `environment` を確認する(AUTH_* が無いことを確認)。
2. `app/iac/envs/dev` を apply 相当でデプロイする(本タスクでは apply は行わないため、`make plan` で environment に AUTH_* が含まれないことを確認する)。
3. デプロイ後、`https://<cloudfront_domain>/api/tasks` に **Authorization ヘッダなし**で GET/POST/PATCH/DELETE すると 200 系で処理される(認証拒否 401 が返らない)。

### 環境・条件

- 対象: AWS(ECS/ALB/CloudFront)にデプロイされた環境。ローカル compose では auth env が別途配線されている場合があるため顕在化しない。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/iac/envs/dev/main.tf:135-141` の service_api environment に `AUTH_ISSUER` / `AUTH_JWKS_URL` / `AUTH_AUDIENCE` が存在しない(DB_* のみ)。
- 事実: `app/api/cmd/api/env.go:164-176` は 3 変数の all-or-nothing 検証で、全未設定(`noneSet`)を正常系として許容する。
- 事実: `app/api/cmd/api/env.go:183-186` の `authEnabled()` は 3 変数すべてが非空のときのみ true。全未設定では false。
- 事実: `app/api/cmd/api/main.go:85-96` は authEnabled 判定に基づき AuthMiddleware を配線する。無効時はミドルウェアを通さない。
- 仮説: dev の利便性のために設けた「3 変数未設定なら認証オフ」という opt-out が、iac 側の配線漏れと組み合わさって「本番相当デプロイでも無認証で起動する」という fail-open を招いている。

### 根本原因

IaC(service_api の environment)に認証 env が未配線であり、かつ app/api 側が「全未設定 = 認証オフ」を正常系として許容する(fail-open)ため、配線漏れが起動エラーにならず素通りする。

## 4. 対応(どう解決するか)

### 対応方針

impl-iac が `module.service_api` の environment に認証 3 変数を追加する。加えて「3 変数が揃っているか」を plan/CI で検証する仕組みを検討する(fail-open の是正は別途 app/api 側の判断が必要になり得るため、まず iac 配線を優先)。

### 実施内容

- [ ] `app/iac/envs/dev/main.tf` の `module "service_api"` の environment に以下を追加:
  - [ ] `AUTH_ISSUER`(auth の `ISSUER` と同値。main.tf:248 参照)
  - [ ] `AUTH_JWKS_URL`(auth の JWKS エンドポイント URL)
  - [ ] `AUTH_AUDIENCE`(auth の `API_AUDIENCE` と一致する値)
- [ ] 必要なら `app/iac/envs/dev/variables.tf` に対応する変数を追加
- [ ] 3 変数が揃っていることを plan/CI で検証する仕組みを検討(欠落時に fail させる)
- [ ] apply は行わず plan 結果を報告し、apply 判断はユーザーに委ねる

### 再発防止

- 認証 env の欠落を検出する plan/CI ゲート(all-or-nothing かつ本番では all-set 必須)を検討する。
- app/api 側の「全未設定 = 認証オフ」を本番相当環境で許容しない方向(fail-closed 化)も別 Issue として検討する余地がある。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。リポジトリ全体のセキュリティ / spec 準拠レビューで検出。`app/iac/envs/dev/main.tf:135-141` の service_api environment に `AUTH_ISSUER` / `AUTH_JWKS_URL` / `AUTH_AUDIENCE` が無いこと、`app/api/cmd/api/env.go:157-186` が全未設定を正常系として許容すること(authEnabled() が false になる)を実地確認した。
- 関連: ISSUE-014(app/iac の auth/web デプロイ経路整備、resolved)。SPEC-015 R12/R13(Bearer JWT 必須設計)。
