---
id: ISSUE-021
title: healthcheck バイナリの client.Get に SSRF 検出(gosec G704)— 悪用可能性の検証と抑制/制限が必要
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-10
specs: []  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-021: healthcheck バイナリの client.Get に SSRF 検出(gosec G704)— 悪用可能性の検証と抑制/制限が必要

種別: セキュリティ静的解析の指摘トリアージ(要検証。現時点では悪用可能性は未確認)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/api / app/auth を運用する開発者・セキュリティ担当** の **静的解析(gosec)結果の信頼性・監査可能性** が **healthcheck バイナリの `client.Get(url)` に SSRF(G704)が検出されたまま、悪用可能性の判定も抑制の根拠づけも行われていない** ことで損なわれている。

- **影響を受けるユーザー**: 主に app/api / app/auth の開発者・セキュリティレビュー担当。gosec の SSRF 検出が未処理のまま残ると、真の脆弱性と偽陽性の区別がつかず、以後のセキュリティスキャンのシグナル対ノイズ比が下がる
- **損なわれる価値**: セキュリティ静的解析の信頼性・監査可能性(defense-in-depth の一部)。仮に悪用可能性が実在する場合は、コンテナ内プローブを踏み台にした内部ネットワークへの SSRF 可能性
- **影響範囲・頻度**: **エンドユーザー向けランタイム挙動への直接影響は現時点で未確認**。healthcheck はコンテナ内の自己プローブ用の別バイナリで、URL 入力は運用者制御(Dockerfile の `HEALTHCHECK` 命令 / compose の引数 / `HEALTHCHECK_URL` env)であり、外部インバウンドリクエストからの注入経路は現状のコード上は見当たらない(下記「3. 原因」参照)。ただし review では実際の悪用可能性まで検証できていない(**未検証**)
- **回避策**: あり(URL 入力は運用者が Dockerfile / compose で固定するため、運用上は任意 URL を渡さない限り問題は顕在化しない)。恒久対応は本 Issue の検証と抑制/制限

## 2. 現象(何が起きているか)

> 個別の退行バグではなく、gosec の taint analysis(G704)が healthcheck バイナリの外向き HTTP リクエストを SSRF として検出しているもの。「静的解析が期待する状態」と「現状」の差分。

### 期待する動作

- healthcheck バイナリの外向き HTTP リクエスト(`client.Get`)について、gosec G704(SSRF)の検出が「偽陽性であることを根拠づけて抑制(`#nosec G704` + 理由コメント等)」されているか、または「URL の scheme/host を制限する」等の緩和が入っており、セキュリティスキャンにノイズが残らない状態

### 実際の動作

- `app/api/cmd/healthcheck/main.go:34` と `app/auth/cmd/healthcheck/main.go:34` の両方で、`url` を検証・制限せずに `resp, err := client.Get(url)` を実行している(2 ファイルは実質同一実装で、defaultURL のみ異なる)
- `url` は次の優先順で決まる(両ファイル 25-30 行):
  1. `os.Args[1]`(コマンド引数)があればそれ
  2. なければ `HEALTHCHECK_URL` env
  3. どちらも無ければ const `defaultURL`(api: `http://localhost:8080/tasks` / auth: `http://localhost:8080/.well-known/openid-configuration`)
- gosec の G704(taint analysis)は `os.Args` / `os.Getenv` を汚染源とみなし、その値がそのまま `http.Client.Get` の URL に渡る経路を SSRF として検出している

### 再現手順

第三者がコード上で決定的に確認できる(静的確認):

1. `app/api/cmd/healthcheck/main.go` を開き、25-30 行で `url` が `os.Args[1]` / `os.Getenv("HEALTHCHECK_URL")` / `defaultURL` の順で決まり、34 行で `client.Get(url)` に scheme/host の制限なしで渡っていることを確認する
2. `app/auth/cmd/healthcheck/main.go` でも同一構造(34 行の `client.Get(url)`、defaultURL のみ差異)であることを確認する
3. gosec(security スキャン)を実行し、両ファイルの `client.Get(url)` 行に G704(SSRF)が報告されることを確認する

悪用可能性の検証(**未実施・本 Issue の対応で行う**):

4. デプロイ済みコンテナ環境で、この healthcheck の URL 入力(`HEALTHCHECK_URL` env / 引数)を外部インバウンドリクエストから注入できる経路が実在するかを検証する。現状のコードからはその経路は見当たらないが、review では確定していない

### 環境・条件

- 対象: `app/api/cmd/healthcheck/main.go` / `app/auth/cmd/healthcheck/main.go`(distroless ランタイムイメージに curl/wget が無いため用意された Docker HEALTHCHECK 用の自己プローブバイナリ。各スタックに 1 つずつ存在)
- 発見文脈: **env 集約リファクタ中の review-security パス**で gosec G704 として検出された。**本件はそのリファクタの差分の外にある既存(PRE-EXISTING)の指摘**で、healthcheck バイナリに以前から存在する構造に対する検出であり、リファクタが新設した問題ではない
- 参照ブランチ: `feat/auth-oidc-foundation`

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `client.Get(url)` の `url` は `os.Args[1]` / `HEALTHCHECK_URL` env / const のいずれか由来(`app/api/cmd/healthcheck/main.go:25-30,34`、`app/auth/cmd/healthcheck/main.go:25-30,34`)。scheme/host のバリデーション・allowlist は無い
- 事実: gosec G704 は taint analysis で `os.Args` / `os.Getenv` を汚染源とみなすため、その値が HTTP クライアントの宛先 URL に流入する本経路を SSRF として一律に検出する
- 仮説: healthcheck はコンテナランタイム(Docker HEALTHCHECK / compose)が起動する自己プローブであり、URL 入力は運用者が Dockerfile / compose で固定する運用者制御の値である。外部インバウンドリクエストがこの env / 引数を書き換える経路は現状のコード・配線上は存在しないと考えられ、その場合 G704 は**偽陽性**(汚染源が攻撃者ではなく運用者)。ただし review では実際の悪用可能性まで検証できておらず、これは**仮説**にとどまる

### 根本原因

未調査(要検証)。gosec G704 が「運用者制御の env/引数を汚染源として一律に SSRF 検出している偽陽性」なのか、「外部から URL を注入しうる実在の経路」なのかは本 Issue の対応で確定する。現時点の心証は前者(偽陽性)だが確定していない。

## 4. 対応(どう解決するか)

### 対応方針

- まず**悪用可能性を検証する**: healthcheck の URL 入力(`HEALTHCHECK_URL` env / `os.Args[1]`)を外部リクエストから注入できる経路が実在するかを、コンテナの配線(Dockerfile の `HEALTHCHECK`・compose・ECS タスク定義等)まで含めて確認する
- 検証結果に応じて:
  - **問題なし(偽陽性と確定)の場合**: 理由コメント付きで抑制する(該当行に `#nosec G704` + 「URL は運用者制御の env/引数で、外部注入経路が無い」旨の根拠コメント)。抑制の根拠を本 Issue にも記録する
  - **問題あり(注入経路が実在)の場合**: URL の scheme(http/https のみ許可)・host(想定ホストへの allowlist)を検証してから `client.Get` に渡すよう制限を入れる
- api・auth の 2 ファイルは実質同一のため、**両方に同じ対応を適用**して非対称を作らない
- テスト: 制限を入れる場合は、許可 URL は通り不正 scheme/host は拒否されることを検証する(tester)。抑制のみの場合は挙動不変

### 実施内容

- [ ] healthcheck の URL 入力に外部注入経路が実在するかを検証する(impl-api / impl-auth)
- [ ] 偽陽性なら `#nosec G704` + 根拠コメントで抑制、実在するなら scheme/host 制限を追加する
- [ ] api・auth の両 healthcheck に同一対応を適用する
- [ ] (制限を入れる場合)許可/拒否 URL のテストを追加する(tester)
- [ ] gosec スキャンで G704 が解消/抑制されたことを確認する(checker / review-security)

### 再発防止

- 抑制する場合は理由コメントを必須にし、なぜ安全かをコード上に残す(将来の監査で「なぜ抑制したか」がわかる状態にする)
- healthcheck のような自己プローブ用の外向きリクエストを追加する際の URL 検証方針を、api/auth 双方で統一する

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。env 集約リファクタ中の review-security パスで、healthcheck バイナリの `client.Get(url)` に gosec G704(SSRF)が検出された。**本件はそのリファクタの差分の外にある既存(PRE-EXISTING)の指摘**であり、healthcheck バイナリに以前から存在する構造に対する検出である旨をここに明記する。
- 事実確認(file:line を保持): `app/api/cmd/healthcheck/main.go:34` / `app/auth/cmd/healthcheck/main.go:34` の `client.Get(url)`。`url` は両ファイル 25-30 行で `os.Args[1]` → `HEALTHCHECK_URL` env → const `defaultURL` の順に決まり、scheme/host の検証は無い。2 ファイルは defaultURL のみ異なる実質同一実装。
- 悪用可能性の評価: healthcheck は distroless に curl/wget が無いために用意されたコンテナ自己プローブで、URL 入力は運用者制御(Dockerfile `HEALTHCHECK` / compose / env)。外部インバウンドリクエストからの注入経路は現状のコード上は見当たらず、G704 は偽陽性の可能性が高い。ただし review では実際の悪用可能性まで検証できていない(**未検証**)ため、根本原因は「未調査」とした。
- severity は **low** と判定。判定根拠: 入力が運用者制御で外部注入経路が現状のコードに見当たらず、healthcheck は内部自己プローブであること、退行バグではなく静的解析指摘のトリアージであること、回避策(運用上任意 URL を渡さない)があること。critical/high/medium ではないのは、エンドユーザー向けランタイム挙動への実害が現時点で確認されていないため。**ただし本 severity は「偽陽性」という仮説を前提とした暫定値であり、検証で外部注入経路が見つかった場合は引き上げる**。
- 重複判定: `docs/issues` を再走査し、healthcheck バイナリの SSRF/G704 を扱う既存 Issue は無いことを確認(`20260708-001` は `/healthz` エンドポイント + postgres リポジトリの別件)。よって新規起票とした。
- 次にやること: impl-api / impl-auth が URL 入力の外部注入経路の有無を検証し、偽陽性なら根拠コメント付きで抑制、実在するなら scheme/host 制限を追加する。両スタックの healthcheck に同一対応を適用する。

### 2026-07-10(gosec 統合〈ISSUE-024〉の実測で CI pin 1.64.8 では本指摘が非検出と判明 / open 維持)

- ISSUE-024(gosec を Go 3 スタックの lint / CI に恒久組み込み)の実装・実測で、healthcheck の `client.Get(url)`(`app/api/cmd/healthcheck/main.go:34` / `app/auth/cmd/healthcheck/main.go:34`)は **CI pin の gosec(golangci-lint 1.64.8 バンドル)では検出されない**ことを確認した。理由は 2 点: (1) 1.64.8 の gosec には taint-analysis 系の **G704(SSRF)ルールが存在しない**(G704 は golangci-lint **v2 系**〈ローカル v2.12.2 で実測〉でのみ検出される)、(2) 近縁の **G107**(Url provided to HTTP request as taint input)は `http.Get` 等のパッケージ関数呼び出しのみを対象とし、本コードのような `*http.Client` のメソッド呼び出し(`client.Get`)は対象外。したがって現状 CI(1.64.8)では本指摘は gosec ゲートに一切掛からない。
- runtime 評価は据え置き: URL 入力は運用者制御(`os.Args[1]` / `HEALTHCHECK_URL` env / 固定の `defaultURL`)で、外部からの注入経路が現状のコードに見当たらないという偽陽性の心証(2026-07-10 起票時の評価)は変更なし。
- **open 維持**。理由: 機械検出したい場合は golangci-lint を v2 系へ上げる必要があり(ISSUE-024 の follow-up)、その際に根拠付き `//nolint:gosec`(理由コメント付き抑制)または scheme/host 制限の実修正を判断する。severity は low のまま(検出ツールの制約が判明しただけで、悪用可能性の評価自体は変わっていないため)。
- ステータスは `open` のまま。`updated` は 2026-07-10。
