---
id: ISSUE-030
title: "app/web: nginx の /api/ リバースプロキシが /api prefix を剥がさず web オリジン経由の全 API 呼び出しが 404(+ web コンテナ unhealthy)"
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: high  # critical | high | medium | low
created: 2026-07-11
updated: 2026-07-12
specs: []  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-030: app/web: nginx の /api/ リバースプロキシが /api prefix を剥がさず web オリジン経由の全 API 呼び出しが 404(+ web コンテナ unhealthy)

## 1. ユーザー価値への影響(なぜ対応するか)

> **compose / 本番相当構成で web を使うユーザー(および動作確認する開発者)** の **タスク管理という主要機能全体** が **web オリジン経由の API 呼び出しが常に 404 になることで完全に使えなくなっている**。

- **影響を受けるユーザー**: `make up` / `make up-d`(compose)や、`app/web/nginx.conf` を使って nginx 配信される本番相当構成で web フロントを開く全ユーザー・全開発者。dev サーバー / Vitest(MSW モック)利用時は影響を受けない。
- **損なわれる価値**: タスクの一覧取得・作成・開始・完了・単一取得(= アプリの主要機能すべて)。web フロントは同一オリジン相対 `/api` で API を呼ぶため、compose / 本番配信では API が一切通らず、画面上は読み込み失敗・操作不能になる。
- **影響範囲・頻度**: 常時(該当構成の全 API 呼び出しが対象)。特定条件下ではなく、web オリジン経由のリクエスト 100% が 404。
- **回避策**: なし(エンドユーザー側)。開発時は `VITE_API_BASE_URL` を api に直接向ける、あるいは dev サーバー / MSW を使えば露見しないが、これは compose / 本番配信そのものの不通を解消しない。
- **severity 判定根拠**: 該当構成では主要機能が全滅する(症状の重さは `critical`「主要機能が使えない」に迫る)。ただし (1) 影響は nginx 配信(compose / 本番相当)経路に限定され dev / test 経路(MSW)は無傷、(2) データ破損・情報漏えいは伴わない、(3) 未リリースの段階で発見済み・修正着手中(下記)であることを踏まえ、`high`(主要な価値が損なわれる)と判定する。critical との境界に近い機能ブロッカーである点を明記しておく。

## 2. 現象(何が起きているか)

### 期待する動作

- web オリジン(compose では `http://localhost:8080`)経由の `GET/POST /api/tasks`(および `/api/tasks/{id}`, `/api/tasks/{id}/start`, `/api/tasks/{id}/complete`)が、nginx で `/api` prefix を剥がされて api サービス(`http://api:8080/tasks` …)へ転送され、200 系で応答する。
- web コンテナの healthcheck が healthy になる。

### 実際の動作

- web オリジン経由の全 API 呼び出しが **404**。
  - host: `GET http://localhost:8080/api/tasks` → 404 / `GET http://localhost:8080/` → 200(SPA 自体は配信できている)。
- web コンテナが **unhealthy**(healthcheck の `wget http://localhost:80/` が connection refused)。

### 再現手順

第三者がそのまま実行できる形:

1. リポジトリルートで compose スタックを起動する: `make up-d`(内部で `make migrate` → web / api / auth / postgres を起動)。
2. SPA 自体が配信されていることを確認する: `curl -i http://localhost:8080/` → `200 OK`。
3. web オリジン経由の API 呼び出しを確認する: `curl -i http://localhost:8080/api/tasks` → `404`(POST も同様)。
4. ブラウザで `http://localhost:8080` を開くと、タスク一覧の取得(`fetchTasks`)が失敗し、UI が読み込みエラー / 操作不能になる。
5. 転送先の切り分け(web コンテナ内から api へ直接):
   - `docker compose exec web wget -qS -O- http://api:8080/tasks` → `200`(prefix なしなら通る)。
   - `docker compose exec web wget -qS -O- http://api:8080/api/tasks` → `404`(prefix 付きだと api 側に存在しない)。
6. `make ps`(= `docker compose ps`)で web コンテナの STATUS が `unhealthy` であることを確認する。

### 環境・条件

- compose スタック(`make up` / `up-d`)。および `app/web/nginx.conf` を使って nginx 配信される本番相当構成。
- dev サーバー / Vitest では **再現しない**: web のテスト・dev では MSW が `/api/*` を Node / ブラウザ層で横取りするため、nginx 転送段を通らず露見しない。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: web フロントは同一オリジン相対パス `/api` を API ベースに使う。`app/web/src/features/tasks/api/client.ts:17` `DEFAULT_BASE_PATH = "/api"`、同 `:30-35` `resolveBaseUrl()` が `VITE_API_BASE_URL` 未設定時に `${window.location.origin}/api` を組み立てる。したがって compose / 本番配信では web と同一オリジンの `/api/tasks` などへ発射される。
- 事実: api は `/api` prefix なしで `/tasks` 系を提供する。`app/api/route/router.go:16-21` が `mux.HandleFunc("POST /tasks", …)` / `"GET /tasks"` / `"GET /tasks/{id}"` / `"POST /tasks/{id}/start"` / `"POST /tasks/{id}/complete"` / `"POST /tasks/{id}/priority"` を登録(`@Router /tasks [post]` 等の注釈とも一致)。
- 事実(バグ版): `app/web/nginx.conf` の `location /api/` が変数付き proxy_pass を使い、`set $api_upstream http://api:8080;` + `proxy_pass $api_upstream/;` で転送していた(git 管理版 = 修正前。コミット `1c77489`)。コメントは「Trailing slash on proxy_pass strips the /api prefix so /api/tasks -> http://api:8080/tasks」と記していた。
- 事実(nginx 仕様): nginx は proxy_pass のターゲットに変数を含む場合、通常の location-prefix 自動置換(URI 部分の書き換え = `/api` 剥がし)を **行わない**。そのため `/api/tasks` がそのまま `http://api:8080/api/tasks` へ転送され、api 側に存在しないパスとなり 404 になる。コメントの「trailing slash が /api を剥がす」は、静的アドレスの proxy_pass では成り立つが、変数版では成り立たない誤り。
- 実測根拠: web コンテナ内 `http://api:8080/tasks`=200 / `http://api:8080/api/tasks`=404、host `http://localhost:8080/api/tasks`=404、`http://localhost:8080/`=200(上記再現手順 2〜5 と対応)。
- 副次(healthcheck): `app/web/Dockerfile` の HEALTHCHECK が `wget -q -O - http://localhost:80/` を叩く。`localhost` が IPv6 `::1` に解決される一方、nginx が IPv4 のみ listen していると connection refused となり unhealthy になる(**仮説**: `localhost` の名前解決先(IPv4/IPv6)と nginx の listen アドレスの不一致。IPv4 リテラルで叩けば解消する見込み)。この unhealthy は 404 とは独立した別要因だが、同じ配信構成の不具合として併せて対応する。
- 事実: 本件は SPEC-013(テストの実 DB 一本化)とは無関係。SPEC-013 はテストのみの変更で `nginx.conf` / `Dockerfile` を変更していない。既存の配信設定バグである。

### 根本原因

- 主因(404): nginx で変数ターゲットの proxy_pass を使うと location prefix の自動置換が効かないという仕様に対し、`app/web/nginx.conf` の `location /api/` が明示的な prefix 剥がしをせず `proxy_pass $api_upstream/;` していたため、`/api/tasks` が prefix 付きのまま api(`/tasks` のみ提供)へ転送され 404 になっていた。
- 副因(unhealthy・仮説): `app/web/Dockerfile` の healthcheck が `http://localhost:80/` を使い、`localhost` の解決先(IPv6 `::1` 等)と nginx の listen アドレス(IPv4)が食い違って connection refused になっていた。

## 4. 対応(どう解決するか)

### 対応方針

- 404: 変数 resolver(Docker 埋め込み DNS `127.0.0.11` によるリクエスト時解決 = api 未起動でも nginx が起動して SPA を配信できる挙動)は維持したまま、`location /api/` で `rewrite ^/api/(.*)$ /$1 break;` により `/api` prefix を明示的に剥がしてから `proxy_pass $api_upstream;` する(クエリ文字列は rewrite 既定で保持)。コメントの誤り(「trailing slash が /api を剥がす」)も変数版の実態に合わせて訂正する。
- unhealthy: healthcheck の URL を `http://localhost:80/` から IPv4 リテラル `http://127.0.0.1:80/` に変更し、`localhost` の解決先ゆらぎに依存しないようにする。
- 担当: impl-web(app/web の `nginx.conf` / `Dockerfile`。app/api 側の変更は不要)。**修正着手中**(作業ツリー上で該当 2 ファイルを修正済み・未コミット)。

### 実施内容

- [x] `app/web/nginx.conf`: `location /api/` を `rewrite ^/api/(.*)$ /$1 break;` + `proxy_pass $api_upstream;` に修正(変数 resolver は維持)。コメントを実態に合わせて訂正。
- [x] `app/web/Dockerfile`: HEALTHCHECK の URL を `http://127.0.0.1:80/` に変更。
- [x] compose スタックで再検証(2026-07-11、admin 実施): web イメージ再ビルド + コンテナ再作成後、`GET http://localhost:8080/api/tasks`=200 / `POST http://localhost:8080/api/tasks`=201 / `GET http://localhost:8080/`=200、web コンテナ Health=healthy(FailingStreak 0)。詳細は経緯参照。
- [x] 修正のコミット(現状 `app/web/nginx.conf` / `app/web/Dockerfile` は作業ツリーに未コミット)。コミット後に `resolved` → `closed` へ。
- [x] checker(format / lint 相当)・tester による確認、review-* による配信構成の妥当性確認。

### 再発防止

- nginx 転送(`/api` → api)は MSW モックの外側にあり web の Vitest では検出できない。compose 起動後にオリジン経由 API を叩くスモーク確認(`curl http://localhost:8080/api/tasks` が 404 でないこと)を、手順またはヘルスチェック / CI のスモークとして残すことを検討する。
- 変数付き proxy_pass では location prefix が自動で剥がれないという nginx の落とし穴を、`nginx.conf` のコメントに明記(修正時に反映済みの方針)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-11

- 起票。稼働中の compose スタックで web オリジン経由の全 API 呼び出しが 404 になる不具合を発見。実測で host `http://localhost:8080/api/tasks`=404 / `http://localhost:8080/`=200、web コンテナ内 `http://api:8080/tasks`=200 / `http://api:8080/api/tasks`=404 を確認し、原因を「変数付き proxy_pass では nginx が location prefix を自動置換しない」仕様に対して `app/web/nginx.conf` の `location /api/` が `proxy_pass $api_upstream/;` のまま prefix を剥がしていなかった点と特定(web は同一オリジン相対 `/api` で呼ぶ = `app/web/src/features/tasks/api/client.ts:17,34`、api は prefix なし `/tasks` で提供 = `app/api/route/router.go:16-21`)。副次で web コンテナが unhealthy(healthcheck の `wget http://localhost:80/` が connection refused。仮説: `localhost` 解決先と nginx listen アドレスの IPv4/IPv6 不一致)。impl-web が `nginx.conf`(`rewrite ^/api/(.*)$ /$1 break;` + 変数 resolver 維持)と `Dockerfile`(healthcheck を `http://127.0.0.1:80/` に)を修正着手中(作業ツリーに未コミットの変更あり)。SPEC-013 とは無関係(テストのみの変更で `nginx.conf` / `Dockerfile` は不変)。次アクション: 修正の compose 再検証と checker / tester / review-* 通過をもって resolved 判定する。

### 2026-07-11(修正適用・実挙動検証。admin 実施)

- impl-web の修正(`app/web/nginx.conf` の `rewrite ^/api/(.*)$ /$1 break;` + `proxy_pass $api_upstream;`、`app/web/Dockerfile` の healthcheck `http://localhost:80/` → `http://127.0.0.1:80/`)を適用後、web イメージを再ビルド(`docker-compose -f compose.yml build web`)し web コンテナを新イメージで再作成(`up -d --no-deps web`)して実挙動を検証した。
- 検証結果(いずれも実測。修正前 → 修正後):
  - `GET http://localhost:8080/api/tasks` → **200**(修正前 404)。
  - `POST http://localhost:8080/api/tasks` → **201**(タスク JSON 応答。ユーザーの元操作 = タスク作成が復旧)。
  - `GET http://localhost:8080/` → 200(SPA 配信は継続)。
  - healthcheck 副次: web コンテナ内 `wget http://127.0.0.1:80/` → OK、`docker inspect` の Health = **healthy(FailingStreak 0)** に復帰。
- healthcheck 真因を確定(起票時の仮説 → 事実): alpine(musl)では `localhost` が `::1`(IPv6)にも解決され得るが、nginx は `listen 80`(IPv4)のみ listen のため `::1` への接続が refused だった。healthcheck を `127.0.0.1` 固定にしたことで解消。
- 検証の副産物: dev `api` DB にスモーク用タスク 1 件(title="proxy-fix smoke test")が作成されたが、api に delete エンドポイントが無いため残存(無害)。
- 状態: 404・unhealthy とも実挙動で解消を確認済みで **resolved 相当**。ただし修正 2 ファイル(`app/web/nginx.conf` / `app/web/Dockerfile`)が**作業ツリーに未コミット**のため、完全クローズは保留し status は `fixing` とする。次アクション: 上記 2 ファイルのコミット(および checker / tester / review-* 通過確認)をもって `resolved` → `closed` に更新する。

### 2026-07-12

- リポジトリ確認: `app/web/nginx.conf` の `rewrite ^/api/(.*)$ /$1 break;` + `proxy_pass $api_upstream;`、`app/web/Dockerfile` の healthcheck `http://127.0.0.1:80/` は既にコミット済み(git 作業ツリー clean)。2026-07-11 の compose 再検証結果(200/201/healthy)をもって `resolved` とする。
