---
id: ISSUE-010
title: app/api の全 HTTP ハンドラでリクエストボディサイズ上限と http.Server の防御設定が無い(緩やかな DoS への横断的堅牢化不足)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-09
updated: 2026-07-10
specs: [SPEC-002]  # 発見文脈: SPEC-002 のセキュリティレビュー(ただし SPEC-002 起因ではない既存課題)
---

# ISSUE-010: app/api の全 HTTP ハンドラでリクエストボディサイズ上限と http.Server の防御設定が無い(緩やかな DoS への横断的堅牢化不足)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/api を運用する開発者・その API 利用者** の **サービスの可用性(リソース枯渇耐性)** が **上限の無い巨大ボディや slow-loris 的リクエストでメモリ/CPU/コネクションを消費させられうる点で、堅牢化の余地が残っている**。

- **影響を受けるユーザー**: app/api を(特に認証なしのまま)ネットワークに露出して運用する開発者と、その API を利用するクライアント
- **損なわれる価値**: 可用性(defense-in-depth)。悪意ある/誤った巨大リクエストや遅延リクエストに対する耐性が、アプリ層で明示的に確保されていない
- **影響範囲・頻度**: **現時点(認証なしのサンプル・実トラフィックなし)では顕在化しない**。横断的(全 decode 経路・サーバ全体)に該当するが、実害は本番相当に露出した場合にのみ発生しうる
- **回避策**: あり(前段のインフラ/リバースプロキシでボディサイズ・タイムアウトを制限する、本 Issue の堅牢化を実装する)。サンプルのまま閉じた環境で使う限りは実害限定

## 2. 現象(何が起きているか)

> 個別の退行バグではなく、既存の全ハンドラに以前から存在する横断的な堅牢化不足。以下は「堅牢化として期待される状態」と「現状」の差分。

### 期待する動作

1. リクエストボディを読む全経路で `http.MaxBytesReader(w, r.Body, N)` 等により**ボディサイズの上限**が課され、上限超過は 413(Request Entity Too Large)相当で拒否される
2. `http.Server` に `ReadTimeout` / `ReadHeaderTimeout` / `WriteTimeout` / `IdleTimeout` / `MaxHeaderBytes` 等の**防御設定**があり、ヘッダ/ボディの読み取りやレスポンス書き込みが遅延しても一定時間・一定サイズで打ち切られる

### 実際の動作

1. JSON をデコードする全ハンドラが、ボディサイズ上限**無し**で `json.NewDecoder(r.Body).Decode(&req)` を実行している。該当は 2 経路(現状ボディを取る全ハンドラ):
   - `create`(POST /tasks): `app/api/route/task_handler.go:60`
   - `changePriority`(POST /tasks/{id}/priority、SPEC-002 で新設): `app/api/route/task_handler.go:134`
   - 参考: `list` / `get` / `start` / `complete` はリクエストボディを読まないため、現状デコード経路はこの 2 つ
2. `app/api/cmd/api/main.go:49-52` の `http.Server` は `Addr` と `Handler` のみを設定し、`ReadTimeout` / `ReadHeaderTimeout` / `WriteTimeout` / `IdleTimeout` / `MaxHeaderBytes` をいずれも**設定していない**(未設定はゼロ値 = 無制限のタイムアウトになる)

### 再現手順

第三者がコード上で決定的に確認できる(静的確認):

1. `app/api/route/task_handler.go` を開き、`create`(58-72 行)と `changePriority`(130-146 行)がいずれも `json.NewDecoder(r.Body).Decode(&req)` を **`http.MaxBytesReader` でラップせず**呼んでいることを確認する(60 行 / 134 行)
2. 同ファイル内に `http.MaxBytesReader` の呼び出しが 1 箇所も無いこと、ボディサイズ制限を課す共通ミドルウェアが存在しないことを確認する
3. `app/api/cmd/api/main.go:49-52` の `srv := &http.Server{ ... }` に `ReadTimeout` / `ReadHeaderTimeout` / `WriteTimeout` / `IdleTimeout` / `MaxHeaderBytes` が **1 つも設定されていない**ことを確認する

動的な確認(概念実証・任意):

4. `make run`(`app/api`)でサーバを起動し、`POST /tasks` に極端に大きな JSON ボディ(例: 数百 MB の `{"title": "aaaa..."}`)を送ると、上限が無いためデコード時にボディ全体を読み込もうとする(メモリ消費)。あるいはヘッダ/ボディをごく低速に送り続ける(slow-loris 的)接続を張ると、タイムアウトが無いため接続が長時間保持される
   - ※ 4 は挙動の説明であり、DoS の成立条件(必要な規模・並列数・実際の停止有無)は本 Issue では未計測(仮説)。堅牢化の要否判断には 1-3 の静的事実で十分

### 環境・条件

- 対象: `app/api`(Go 標準ライブラリのみの DDD サンプル、タスク管理)。認証は未実装
- 発見文脈: **SPEC-002(Task 優先度追加)のセキュリティレビュー**で observation として挙がった。ただし本件は SPEC-002 の差分で新設された問題ではなく、既存の全ハンドラ(および `http.Server` 配線)に以前から存在する横断的課題であり、SPEC-002 のスコープ外として切り出したもの。SPEC-002 で新設した `changePriority` も同じパターンを踏襲しているため影響面の一つに含まれる

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: JSON デコード経路(`create`:`task_handler.go:60`、`changePriority`:`task_handler.go:134`)がボディサイズ上限無しで `json.NewDecoder(r.Body).Decode(&req)` を実行している(レビュー観察)
- 事実: `http.Server`(`cmd/api/main.go:49-52`)に `ReadTimeout` / `ReadHeaderTimeout` / `WriteTimeout` / `IdleTimeout` / `MaxHeaderBytes` の設定が無い(レビュー観察)。Go の `http.Server` はこれらを未設定だとタイムアウト無制限で動作する
- 事実: この API には認証・認可が無いため、リクエストは誰でも送れる前提になる
- 仮説: 上限の無いボディや遅延リクエストを大量/大規模に送ることで、メモリ・CPU・コネクションを消費させる**緩やかな DoS**(リソース枯渇)の一因になりうる。実際に停止に至る規模は未計測
- 関連: サーバ堅牢化という観点で **ISSUE-002(SPEC-001 の本番相当移行時のセキュリティ・可用性強化チェックリスト)** と趣旨が近い。ただし ISSUE-002 は `app/iac`(Terraform: RDS/ECR/CloudFront/WAF/ECS/state)のインフラ層に閉じており、本 Issue の `app/api`(Go アプリ層の HTTP 受け口堅牢化)とは**対象コード・修正担当(impl-iac ↔ impl-api)・修正ファイルがいずれも重ならない**。同一問題ではなく、層の異なる別課題として相互参照する(重複判定の詳細は「5. 経緯」参照)

### 根本原因

**退行バグではない。** サンプル実装として、アプリ層での入力サイズ制限・サーバタイムアウト等の defense-in-depth をこれまで明示的に設定してこなかったことによる横断的な堅牢化不足。サンプル(認証なし・閉じた環境で `make run` する題材)としては動作要件を満たすが、ネットワークに露出する運用ではアプリ層でも上限・タイムアウトを課すのが望ましい。

## 4. 対応(どう解決するか)

### 対応方針

以下は候補(提案)。採否・具体値・実装方式は着手時に planner が確定する:

- ボディサイズ上限の導入: 共通ミドルウェア、または各デコード経路で `http.MaxBytesReader(w, r.Body, N)` を適用する。上限 `N` はエンドポイントの正当なペイロード上限に基づいて決める
- `http.Server` の防御設定: `cmd/api/main.go` の `http.Server` に `ReadTimeout` / `ReadHeaderTimeout` / `WriteTimeout` / `IdleTimeout` / `MaxHeaderBytes` を設定する(値は運用要件に応じて決定)
- 上限超過時のステータス: `MaxBytesReader` により上限超過で発生するエラー(`*http.MaxBytesError`)を判別し、現状の一律 400("invalid request body")ではなく **413(Request Entity Too Large)相当**で返すことを検討する
- 実装は横断的なため、既存の全デコード経路(および将来追加されるボディ受け取り経路)に一貫して適用できる形(共通ミドルウェア/ヘルパ)が望ましい

### 実施内容

- [ ] ボディサイズ上限(`http.MaxBytesReader`)を全デコード経路に適用する方式と上限値を決めて実装する
- [ ] `http.Server` に `ReadTimeout` / `ReadHeaderTimeout` / `WriteTimeout` / `IdleTimeout` / `MaxHeaderBytes` を設定する
- [ ] 上限超過を 413 相当で扱う(採用する場合)
- [ ] テスト追加(上限超過時のステータス/エラー、正常系の境界サイズ)(tester)

### 再発防止

- 新しいボディ受け取りハンドラを追加する際に、ボディサイズ上限を必ず通す仕組み(共通ミドルウェア/ヘルパの一元化)にしておき、個別ハンドラで上限付与を忘れても素通りしないようにする

## 5. 経緯(時系列・追記のみ)

### 2026-07-09

- 起票。SPEC-002(Task 優先度追加、`docs/specs/20260708-002-task-priority.md`)のセキュリティレビューで、app/api の HTTP 受け口にボディサイズ上限とサーバタイムアウトが無い点が observation として挙がった。これは SPEC-002 の差分起因ではなく既存の全ハンドラに横断する課題のため、SPEC-002 スコープ外の独立課題として本 Issue に切り出した。
- 事実確認(レビュー観察を file:line で保持): デコード経路のボディサイズ上限なし = `app/api/route/task_handler.go:60`(`create`)/ `:134`(`changePriority`)。`http.Server` の防御設定なし = `app/api/cmd/api/main.go:49-52`。`list` / `get` / `start` / `complete` はボディを読まないため現状のデコード経路はこの 2 つと確認。
- **重複判定**: 既存 ISSUE-002(SPEC-001 の本番相当移行時セキュリティ・可用性強化チェックリスト)と「サーバ堅牢化」の趣旨は近いが、**別課題と判断し新規起票**した。理由: ISSUE-002 は 7 項目すべてが `app/iac` の Terraform リソース(RDS/ECR/CloudFront/WAF/ECS/state バケット)に閉じたインフラ層のチェックリストで、修正担当は impl-iac。本 Issue は `app/api` の Go アプリ層(HTTP デコード経路と `http.Server` 配線)の堅牢化で、修正担当は impl-api。対象コード・修正ファイル・修正担当がいずれも重ならず、ISSUE-002 に含めると層とオーナーの異なる項目が混在してトレーサビリティが落ちるため、独立 Issue とし相互参照する(本 Issue「3. 調査ログ」に ISSUE-002 への参照を記載)。
- **severity は low と判定**。判定根拠: 退行バグではなく defense-in-depth の横断的な堅牢化不足で、現状は認証なしのサンプル・実トラフィックなしのため実害は限定的、回避策(前段インフラでの制限)もある。critical/high/medium ではないのは、現行スコープで機能・価値が損なわれておらず、DoS 成立の具体規模も未計測(仮説段階)なため。ISSUE-002 と同様、本番相当に露出した場合にのみ実害となる位置づけとして low とした。
- 関連 Spec: 発見文脈である SPEC-002 を frontmatter `specs` に相互リンクし、SPEC-002 側 `issues` にも本 Issue を追記した(本 Issue が SPEC-002 起因の退行ではなく、SPEC-002 レビューで検出した既存の横断課題である旨を SPEC-002 の経緯に明記)。
- 次にやること: 対応を決めた時点で planner に計画化を依頼し、impl-api が実装 → tester/checker/review-* を通す。上限値・タイムアウト値・413 採否は planner が確定する。

### 2026-07-10(env 集約リファクタの review-security で再検出 / gosec G112・app/auth との非対称)

- env 集約リファクタ中の review-security パスで、app/api の `http.Server` にサーバタイムアウトが無い点が gosec **G112(Potential Slowloris Attack)** として再び検出された。**本件はそのリファクタの差分の外にある既存(PRE-EXISTING)の指摘**であり、本 Issue が 2026-07-09 に既に記録済みの同一課題(`http.Server` のタイムアウト未設定)である。よって新規 Issue は起票せず本 Issue に追記した(重複起票の回避)。
- 現物再確認(現行コード): `app/api/cmd/api/main.go:70-73` の `srv := &http.Server{ Addr, Handler }` は依然 `Addr` と `Handler` のみを設定し、`ReadHeaderTimeout` / `ReadTimeout` / `WriteTimeout` / `IdleTimeout` を設定していない(本 Issue 起票時に記載した 49-52 行から行番号は移動したが状態は不変)。
- **新たに判明した文脈(HTTP 層防御の非対称性)**: 対照的に `app/auth/cmd/authz/main.go:124-131` は `ReadHeaderTimeout` / `ReadTimeout` / `WriteTimeout` / `IdleTimeout` の 4 つを明示設定している(定数定義 44-47 行に Slowloris 緩和の理由コメントあり)。api と auth は同一 RDS を共有し対称性を重視する方針であるのに、HTTP 受け口の防御が非対称になっている。対応時は auth と同等のサーバタイムアウト群を api にも設定して対称化することを推奨する。
- ステータスは `open` のまま(対応未着手)。`updated` を 2026-07-10 に更新。
</content>
</invoke>
