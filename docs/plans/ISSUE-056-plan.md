# ISSUE-056 実装計画: runtime コンテナ最小権限ハードニング(+ ISSUE-051 デモパスワードログ opt-in 一本化)

- 起点 Issue: `docs/issues/20260725-056-runtime-container-least-privilege-hardening-gap.md`(ISSUE-056)
- 併せて対応する関連 Issue: `docs/issues/20260712-051-auth-demo-password-plaintext-log-path-enabled-by-default.md`(ISSUE-051)
- 作成日: 2026-07-25 / 作成: planner

---

## 0. スコープと同梱理由(先に読むこと)

本計画は review-security の「コンテナ最小権限レビュー(2026-07-25)」で挙がった一連のハードニング指摘を、**4 stack に分担して一括適用**する。ISSUE-051(デモパスワード平文ログの opt-in 一本化)は同一レビュー由来で app/auth に閉じるため、往復を減らす目的で同計画に同梱する(コード変更範囲は独立しており相互依存しない)。

- **impl-iac**: ECS タスク定義に `readonlyRootFilesystem` + `capabilities.drop:["ALL"]`(ISSUE-056 §2-1)
- **impl-web**: 公開 nginx(web / auth-web)を non-root + 非特権ポート 8080 化(ISSUE-056 §2-2)
- **impl-ci**: ルート `compose.yml` の `cap_drop` / `read_only` / tmpfs / ポート追随 / 注記(ISSUE-056 §2-3,4)
- **impl-auth**: デモパスワードログを `DEMO_LOG_PASSWORD == "1"` のみの opt-in へ一本化(ISSUE-051)

### 調査で確定した前提

1. **web / auth-web は本番 ECS に存在しない。** `app/iac/envs/dev/main.tf` の service モジュールインスタンスは `service_api` と `service_auth` の 2 つだけ(web は S3+CloudFront 配信、auth-web も ECS 定義なし)。したがって **ECS 上の runtime コンテナは api / auth の app コンテナと migration(init)コンテナのみ**で、nginx は載っていない。**nginx のポート変更(80→8080)は ECS 側に一切波及せず、api / auth の `container_port` は 8080 のまま不変**(impl-iac は capability add / port 追随が不要)。
2. **api / auth / migrator は distroless static nonroot の Go バイナリ**(`app/{api,auth}/Dockerfile` は `gcr.io/distroless/static:nonroot` + `USER nonroot`、`app/migrator/Dockerfile` も同様)。**runtime のコードにファイル書き込みは無い**(`grep` で `os.Create`/`os.WriteFile`/`os.MkdirAll` 等を全探索した結果、ヒットは `app/auth/cmd/keygen/main.go` のみ。keygen は署名鍵生成 CLI で **実行コンテナ(`cmd/authz`)とは別バイナリ**。authz / api / migrator の runtime パスは無書き込み)。よって `readonlyRootFilesystem: true` を副作用なく適用できる。
3. **nginx は現状 `nginx:alpine`**(`FROM nginx:alpine AS runtime`、`USER` 指定なし、`EXPOSE 80`、`listen 80`、`HEALTHCHECK` は `http://127.0.0.1:80/`)。master プロセスが root 常駐。
4. **ISSUE-051 の該当条件は現状 `if e.DemoPassword != "" || os.Getenv("DEMO_LOG_PASSWORD") == "1"`**(`app/auth/cmd/authz/main.go:158-160`)。`e.DemoPassword`(= `DEMO_PASSWORD`)が非空なだけで平文出力され、`DEMO_LOG_PASSWORD` は実質 no-op な冗長条件。

---

## 1. 方針

### 1.1 nginx non-root 化: `nginx-unprivileged` イメージを採用する(手動 USER 化を退ける)

- **採用**: 最終ステージを `nginxinc/nginx-unprivileged:alpine` に置き換える。このイメージは既定で uid 101(non-root)起動・`listen 8080`・`pid /tmp/nginx.pid`・一時ディレクトリを書き込み可能な場所に配置済みで、**メインの `/etc/nginx/nginx.conf`(user / pid / temp_path のハードニング)がイメージ側に内蔵**されている。本リポジトリは server ブロックを `/etc/nginx/conf.d/default.conf` として供給しているだけなので、メイン設定に手を入れずに済み、変更は「ベースイメージ差し替え + `listen` を 8080 に変更 + `EXPOSE`/`HEALTHCHECK` の追随」に閉じる。
- **退けた代替案(手動 USER 化)**: `nginx:alpine` を維持し `USER nginx` + pid ファイルパス変更 + cache ディレクトリ所有権変更を手で行う。メインの `nginx.conf` に `user nginx;` / `pid /var/run/nginx.pid;` が入っているため、(a) 非 root 起動で `/var/run` への pid 書き込みが必要になり compose の `read_only:true` と衝突しやすい、(b) `client_body_temp` 等の temp path を明示的に書き込み可能な場所へ移すメイン設定の上書きが必要、と手数と漏れリスクが増える。**書き込みパス設計をイメージに委ねられる `nginx-unprivileged` の方が read_only 化との相性が良く、漏れが少ない**ため採用する。
- **補足**: ベースイメージ名が変わる(`nginx:alpine` → `nginxinc/nginx-unprivileged:alpine`)が、digest pin は現状どのアプリ Dockerfile でも行っていない(alpine タグ運用)ため、pin 方針の変更は本計画のスコープ外(現行運用を踏襲)。busybox `wget`(HEALTHCHECK が使用)は同 alpine ベースに同梱される。

### 1.2 コンテナ内 listen ポートを **8080 に固定**する(ホスト/ALB 公開ポートは不変)

- nginx の container port を 80 → **8080** に統一する。これを計画時点の**契約として固定**することで、impl-web(Dockerfile/nginx.conf)と impl-ci(compose のポートマッピング)は**互いの完了を待たずに並列着手**できる(検証だけ両者ランディング後)。
- ホスト側公開ポートは不変: compose のマッピングは `127.0.0.1:8080:80` → `127.0.0.1:8080:8080`(web)、`127.0.0.1:8083:80` → `127.0.0.1:8083:8080`(auth-web)。**左辺(ホスト 8080/8083)は変えず、右辺(コンテナ側)を 8080 に追随**させるだけ。

### 1.3 ECS: `readonlyRootFilesystem: true` + `capabilities.drop:["ALL"]` を app / migration 両コンテナへ

- `app/iac/modules/service/ecs.tf` の `local.app_container`(66-101 行)と `local.migration_container_defs`(36-64 行)の各コンテナ定義に、コンテナレベルで `readonlyRootFilesystem = true` と `linuxParameters = { capabilities = { drop = ["ALL"] } }` を追加する。**Fargate は両方をサポート**。§0-2 のとおり distroless nonroot で無書き込みのため `readonlyRootFilesystem` は副作用なし、8080 バインド(>1024)+ nonroot ユーザーで `cap_add` は**不要**(nginx が ECS に居ないため NET_BIND_SERVICE も不要)。
- 適用は service モジュールで一元化されるため、`service_api` / `service_auth` の両インスタンスに自動的に効く(モジュール内で完結・呼び出し側 `envs/dev/main.tf` の変更は原則不要)。variables 追加も原則不要(定数で埋め込む。将来的に無効化スイッチが要るなら `variable "readonly_root_filesystem"` を default=true で追加してもよいが、本計画では既定 true 固定で最小変更にする)。

### 1.4 compose: `cap_drop:[ALL]` を全 5 サービス、`read_only` は api/auth/web/auth-web、postgres は最小 cap_add + read_only 見送り

- **`cap_drop: [ALL]`**: postgres / api / auth / web / auth-web の 5 サービス全部に追加(既存の `security_opt: no-new-privileges` は維持)。toolchain コンテナ(`.devcontainer/compose.tools.yml` の `x-tools-base`)が既に `cap_drop: ALL` を持つ非対称を解消する。
- **postgres の cap_add(最小)**: 公式 postgres イメージの entrypoint は root 起動 → データディレクトリの chown → `su-exec`(alpine)で `postgres` ユーザーへ降格、という初期化を行うため、`cap_drop:[ALL]` 単独では初回 init(空ボリューム)で失敗しうる。**`cap_add: [CHOWN, DAC_OVERRIDE, FOWNER, SETGID, SETUID]`** を残す設計とする(chown=CHOWN、権限バイパス=DAC_OVERRIDE/FOWNER、uid/gid 降格=SETUID/SETGID)。impl-ci は**空ボリュームからの初回 init**(`make down` 後にボリューム破棄 → `make db-up`)で起動を確認し、過不足があれば増減する(§4・§6)。
- **`read_only: true`**:
  - **api / auth**: distroless static・無書き込み(§0-2)のため安全に付与。防御的に `tmpfs: [/tmp]` を併記(Go の既定 `TMPDIR`。実測で不要と確認できれば外してよい)。
  - **web / auth-web**(nginx-unprivileged): `read_only: true` + `tmpfs: [/tmp, /var/cache/nginx]`(pid=`/tmp/nginx.pid`、client_body/proxy 等の temp=`/var/cache/nginx`)。起動・HEALTHCHECK 疎通で不足パス(必要なら `/var/run`)を追加する。
  - **postgres**: **read_only 化は見送る**。理由: 公式イメージの entrypoint がデータボリューム外(ソケット `/var/run/postgresql`、`/tmp`、設定生成先など)に多数書き込み、全書き込み先を tmpfs 化する調整は高リスクな一方、ローカル DB は `127.0.0.1` バインド・非機密固定資格情報でセキュリティ上の追加便益が小さい。`cap_drop:[ALL]` + 最小 `cap_add` + `no-new-privileges` + 既存 `sysctls`(ip_forward=0)で最小権限は担保し、read_only だけ見送る旨をコメントに残す。
- **`ADMIN_API_KEY` 注記**: `compose.yml:121` の `ADMIN_API_KEY: local-dev-admin-key` に、DB 資格情報(14-17 行)と同種の「非機密のローカル専用固定値・実環境に転用しない」旨のコメントを付す。

### 1.5 ISSUE-051: `DEMO_LOG_PASSWORD == "1"` のみの opt-in へ一本化する

- **コード**: 条件を `if e.DemoPassword != "" || os.Getenv("DEMO_LOG_PASSWORD") == "1"` から **`DEMO_LOG_PASSWORD == "1"` 単独**に変える。`DEMO_PASSWORD`(実資格情報を入れうる)単独では平文出力しない。
- **env 読み取りの集約(採用)**: `app/auth/cmd/authz` の env 読み取りは `env.go`(`NewEnv`/`Env`)に一本化する規約(`.claude/rules/db.md` の接続 env 契約・testing.md の「`cmd/*/env.go` の env→Config 写像テスト」)に倣い、`DEMO_LOG_PASSWORD` を **`Env` の bool フィールド(例 `LogDemoPassword`)として `env.go` で読み**、`main.go` の `run()` は `if e.LogDemoPassword {` に変える。現状 `main.go` が `os.Getenv("DEMO_LOG_PASSWORD")` を直接読むインライン参照(`DEMO_PASSWORD` は既に `Env` 経由なのに片方だけ生 getenv)という非対称も解消でき、**`env.go` を単体テスト可能な seam にできる**。
  - 退けた代替案: `main.go` のインライン条件を `os.Getenv("DEMO_LOG_PASSWORD") == "1"` に縮めるだけ(Env に載せない)。動作は正しいが `run()` 内の logging guard を単体テストしづらく、env 読み取りの一本化規約からも外れるため退ける。
- **compose の `DEMO_LOG_PASSWORD: "1"` は残す(impl-ci)**: コード修正後は「明示 opt-in が唯一のトリガ」になる。ローカルの `DEMO_PASSWORD` は既知・非機密の固定値 `demo` で、ログ表示はローカルログインの利便に資する。よって compose の `DEMO_LOG_PASSWORD: "1"` は**明示的なローカル opt-in として残し**、「既知・非機密のデモ資格情報を意図的に表示する明示フラグ」である旨のコメントを添える(除去すると利便を失い、フラグの存在意義も消える)。ISSUE-051 の核である「`DEMO_PASSWORD` 単独で漏えい経路になる」危険はコード側で解消されるため、compose にフラグを残すことと矛盾しない。

---

## 2. 変更ファイル(stack ごと)

### app/web / app/auth-web(impl-web)
- 変更: `app/web/Dockerfile` — `FROM nginx:alpine` → `FROM nginxinc/nginx-unprivileged:alpine`、`EXPOSE 80` → `EXPOSE 8080`、`HEALTHCHECK` の URL を `http://127.0.0.1:8080/` に、`USER` は不要(イメージが non-root)。`CMD ["nginx","-g","daemon off;"]` は不変。
- 変更: `app/web/nginx.conf` — `listen 80;` → `listen 8080;`(server ブロックのみ。proxy upstream `api:8080` / `auth:8080` は不変)。
- 変更: `app/auth-web/Dockerfile` — web と同一の変更。
- 変更: `app/auth-web/nginx.conf` — `listen 80;` → `listen 8080;`。

### app/iac(impl-iac)
- 変更: `app/iac/modules/service/ecs.tf` — `local.app_container`(66-101 行)へ `readonlyRootFilesystem = true` と `linuxParameters = { capabilities = { drop = ["ALL"] } }` を追加。`local.migration_container_defs`(36-64 行、`merge(...)` の第 1 引数オブジェクト)にも同じ 2 キーを追加。
- 変更(必要時のみ): `variables.tf` / `envs/dev/main.tf` — 本計画では定数埋め込みで variables 追加は不要の見込み。呼び出し側変更も原則不要。

### リポジトリルート(impl-ci)
- 変更: `compose.yml` —
  - web: ポート `127.0.0.1:8080:80` → `127.0.0.1:8080:8080`。auth-web: `127.0.0.1:8083:80` → `127.0.0.1:8083:8080`。
  - 5 サービスへ `cap_drop: [ALL]` を追加。postgres へ `cap_add: [CHOWN, DAC_OVERRIDE, FOWNER, SETGID, SETUID]`。
  - api / auth / web / auth-web へ `read_only: true` + 必要 tmpfs(api/auth: `/tmp`、web/auth-web: `/tmp`,`/var/cache/nginx`(不足時 `/var/run`))。postgres は read_only 見送り(理由コメント)。
  - `ADMIN_API_KEY` に非機密ローカル専用の注記コメント。
  - `DEMO_LOG_PASSWORD: "1"` は残しつつ「明示的ローカル opt-in」コメントを付す(ISSUE-051 連携)。

### app/auth(impl-auth)
- 変更: `app/auth/cmd/authz/env.go` — `Env` に `LogDemoPassword bool` を追加し `NewEnv` で `os.Getenv("DEMO_LOG_PASSWORD") == "1"` を読む。
- 変更: `app/auth/cmd/authz/main.go` — 158-160 行の条件を `if e.LogDemoPassword {` に変更(`os`/`syscall` 等 import の未使用が生じたら整理)。
- 追加/変更(tester): `app/auth/cmd/authz/env_test.go` — `DEMO_LOG_PASSWORD` → `LogDemoPassword` 写像の table test。

---

## 3. 手順(実行 agent と順序)

> 契約: **コンテナ内 listen ポート = 8080**(§1.2)を計画時点で固定する。これにより Phase 1 の 4 agent は相互の完了を待たずに並列着手できる。**統合検証(Phase 3 の `make up-d` smoke)は 4 者すべてのランディング後**に行う。

### Phase 1 — 実装(4 レーン並列)

- **1-A impl-web**: `app/web` / `app/auth-web` の Dockerfile を `nginx-unprivileged` に差し替え・`EXPOSE`/`HEALTHCHECK` を 8080 化、各 `nginx.conf` の `listen` を 8080 化(§2 app/web)。
- **1-B impl-iac**: `modules/service/ecs.tf` の app / migration コンテナへ `readonlyRootFilesystem` + `capabilities.drop:["ALL"]` を追加(§2 app/iac)。
- **1-C impl-ci**: `compose.yml` のポート追随・`cap_drop`/`cap_add`/`read_only`/tmpfs・`ADMIN_API_KEY` 注記・`DEMO_LOG_PASSWORD` コメント(§2 ルート)。
- **1-D impl-auth**: `cmd/authz/env.go` + `main.go` のデモログ opt-in 一本化(§2 app/auth)。

> レーン間依存: 1-A と 1-C は「コンテナ側 8080」という固定契約でのみ結合(コード上の依存はなく並列可)。1-B は nginx 非対象のため完全独立。1-D は完全独立。

### Phase 2 — テスト作成/実行 + チェック(stack 単位)

- **2-1 tester(impl-auth 分)**: `env_test.go` に `DEMO_LOG_PASSWORD` 写像の table test(未設定/"1"/"0"/"true"/`DEMO_PASSWORD` 非空だけ、を網羅)を追加し `make auth-test` で green を確認(§4)。TDD 先行が望ましいが、既存 env test と同型の小変更のため後付けも可。
- **2-2 checker**: 影響 stack ごとに実行(並列可)。
  - `make auth-check`(fmt-check + lint + vet + build + test。実 test DB を要するため正規経路 `make migrate-test` → `REQUIRE_DB=1`)。
  - `make web-check` / `make auth-web-check`(format-check + lint + typecheck + test + build)。**注**: これは TS アプリのビルドのみで **Dockerfile / nginx.conf は対象外**(回帰がないことの確認に留まる。Docker イメージの検証は Phase 3)。
  - `make iac-check`(fmt-check + validate + lint + security)。

### Phase 3 — 統合検証(tester、Phase 1+2 すべてランディング後)

コンテナ/インフラ側は静的チェックでは挙動を確認できないため、tester が以下を実行する:

1. `make up-d`(前提 `make migrate` を含む。5 サービスをビルド&起動)。
2. 疎通確認:
   - web `http://localhost:8080/`(200・SPA 配信)、auth-web `http://localhost:8083/`(200)。
   - api `http://localhost:8081/health`(200)、auth `http://localhost:8082/.well-known/openid-configuration`(200)。
   - `make ps` で 5 サービスが `healthy`(nginx healthcheck の 8080 追随・read_only+tmpfs での起動を含む)であること。
3. **ISSUE-051 検証**: `make logs`(auth)で `DEMO_LOG_PASSWORD: "1"`(compose 既定)時に平文パスワード行が出ること、および **`DEMO_LOG_PASSWORD` を外した(または `"0"`)構成では `DEMO_PASSWORD=demo` があっても平文行が出ないこと**を確認(既定発火の除去=本 Issue の受け入れ)。
4. **postgres の cap 検証**: 初回 init 経路を確かめるため、`make down` + `pgdata` ボリューム破棄後に `make db-up`(または `make up-d`)で postgres が `healthy` に到達することを確認(cap_add の過不足検出)。
5. `make down` で後片付け。

### Phase 4 — iac plan 差分(impl-iac 完了後、Phase 3 と並列可)

- impl-iac または tester が `make iac-plan ENV=dev` を実行し、差分が「api / auth の app・migration コンテナ定義に `readonlyRootFilesystem`/`linuxParameters` が追加されるのみ(既存リソースの replace ではなく in-place update)」であることを報告。**apply はユーザー判断**(agent は実行しない)。`make iac-security`(trivy)で ECS の readonlyRootFilesystem 未設定系の指摘が解消/悪化しないことも確認。

### Phase 5 — レビュー(並列)

- review-security(最小権限が意図どおり効くか・cap_add が最小か・read_only 見送り理由の妥当性・ISSUE-051 の opt-in ゲート)、review-spec(ISSUE-056/051 の期待動作充足)。指摘の Blocker/Major は該当 impl へ差し戻し(Phase 2/3 再実行)。今回対応しない指摘は issue-creator が起票。

### Phase 6 — 起点 Issue の更新

- ISSUE-056 / ISSUE-051 の status(検証できたら `resolved`)と経緯を issue skill 手順で更新(§5)。

---

## 4. テスト戦略

- **方針**: コンテナ / インフラ設定は単体テスト不能のため、**Go(auth)側のみ単体テスト**を追加し、**コンテナ側は静的チェック + `make up-d` の smoke + `make iac-plan` 差分**で検証する(テスト規約の「テストなしで完了にしない」を、テスト可能な auth はユニットで、不能な部分は再現可能な検証手順で満たす)。
- **auth(ISSUE-051)ユニットテスト(tester、`cmd/authz/env_test.go`、`package main`、DB 非依存)**: `t.Setenv("DEMO_LOG_PASSWORD", ...)` で `NewEnv().LogDemoPassword` を検証する table test。
  - 正常系: `"1"` → true。
  - 異常系/境界: 未設定 → false、`"0"` → false、`"true"` → false(厳密一致 `== "1"`)。
  - **回帰の要**: `DEMO_PASSWORD` を非空にしても `DEMO_LOG_PASSWORD` 未設定なら `LogDemoPassword=false`(= 平文を出さない)ことを 1 ケースで固定し、旧条件(`DemoPassword != ""` で発火)の復活を検出する。
  - `run()` 内のログ出力自体(slog 呼び出し)は composition root のため単体化せず、この `LogDemoPassword` の写像テスト + Phase 3-3 のログ観察で代替する。
- **コンテナ側検証(Phase 3)**: `make up-d` → 4 エンドポイント疎通 + `healthy` + postgres 初回 init + デモログ有無、で「起動回帰なし + 最小権限適用で壊れない」を担保。
- **iac 側検証(Phase 4)**: `make iac-check`(validate/lint/security)+ `make iac-plan ENV=dev` の差分レビュー(replace でなく in-place・意図どおりの 2 キー追加)。破壊的変更(replace)が出たら報告。

### 要件 ↔ 手順/テスト 対応表

| 要件(Issue の期待動作) | 実装手順 | 検証 |
|---|---|---|
| ISSUE-056 ①: ECS に `readonlyRootFilesystem:true` + `cap drop ALL`(app/migration) | 1-B(ecs.tf) | Phase 4 `iac-plan` 差分 + `iac-security` |
| ISSUE-056 ②: 公開 nginx を non-root + 非特権ポート 8080 化 | 1-A(Dockerfile/nginx.conf) | Phase 3 web/auth-web 疎通 + `healthy` |
| ISSUE-056 ②: ECS/compose の container port 追随 | 1-B(ECS は不要=8080 維持)/ 1-C(compose 右辺 8080) | Phase 3 疎通(host 8080/8083 不変) |
| ISSUE-056 ③: compose 5 サービスに `cap_drop:[ALL]`(+ read_only/tmpfs) | 1-C(compose) | Phase 3 `make ps` healthy + postgres 初回 init |
| ISSUE-056 ④: `ADMIN_API_KEY` に非機密注記 | 1-C(compose) | checker/レビューで文言確認 |
| ISSUE-051: `DEMO_LOG_PASSWORD==1` のみ opt-in・`DEMO_PASSWORD` 単独で出さない | 1-D(env.go/main.go)+ 1-C(compose コメント) | 2-1 `env_test.go` + Phase 3-3 ログ観察 |

---

## 5. 起点 Issue への反映(planner が issue skill 手順で実施)

本計画の作成に合わせ、planner が以下を issue skill の更新手順(該当セクションの現状追随 + 経緯末尾への `### YYYY-MM-DD` 追記・過去エントリ不編集 + frontmatter `updated`)で反映する。status は実装未完のため両者 `open` 維持(Phase 6 で検証後に `resolved` へ)。

- **ISSUE-056**: 「4. 対応 / 対応方針」に本計画(`docs/plans/ISSUE-056-plan.md`)への参照と 4 stack 分担の要約を追記。経緯に「計画策定・8080 ポート固定契約・postgres read_only 見送り判断・nginx-unprivileged 採用・ECS は nginx 非対象で port 追随不要」を記録。
- **ISSUE-051**: 「4. 対応 / 対応方針」に本計画(ISSUE-056 と統括)への参照を追記。経緯に「ISSUE-056 の一括ハードニングに同梱・`DEMO_LOG_PASSWORD` を `Env` へ移し `== "1"` 単独 opt-in に一本化・compose のフラグは明示 opt-in として残す方針」を記録。

---

## 6. リスク / 未確定事項

1. **【要検証】postgres の `cap_add` 最小集合が初回 init を通すか。** `cap_drop:[ALL]` + `[CHOWN, DAC_OVERRIDE, FOWNER, SETGID, SETUID]` を想定するが、公式イメージのバージョン差で追加 cap(例 `FSETID`)が要る可能性がある。**空ボリュームからの初回 init**(既存 `pgdata` があると chown 経路を通らず見逃す)を Phase 3-4 で必ず確認する。過不足があれば cap を最小限で増減し、それでも通らなければ read_only 同様に「postgres は cap_drop を見送り、no-new-privileges + sysctls のみ」に後退する判断もありうる(その場合は理由を Issue 経緯に記録)。
2. **【要検証】nginx-unprivileged の read_only 化で必要な tmpfs 集合。** `/tmp` + `/var/cache/nginx` を第一候補とするが、`/var/run` 等が追加で要る可能性。Phase 3 の HEALTHCHECK/疎通で不足を検出し補う。HEALTHCHECK が `wget` を用いるため、busybox `wget` がイメージに存在することも同時に確認する。
3. **web/auth-web の `make check` は Docker を検証しない。** web の checker は TS アプリの build までで、Dockerfile/nginx.conf 変更は `make up-d`(Phase 3)でしか検証されない。**Phase 3 を必須ゲート**とし、これを飛ばして完了にしない。
4. **ECS の readonlyRootFilesystem で想定外書き込みが起きる可能性(低)。** §0-2 のコード全探索で runtime 無書き込みを確認済みだが、将来ライブラリ追加等で `/tmp` 書き込みが生じた場合、Fargate では `tmpfs`(EC2 専用)は使えず **ephemeral volume + `mountPoints`** が必要になる。現状は不要と判断(low)。apply 前の実起動確認はユーザーの apply 判断に委ねる(agent は apply しない)。
5. **iac plan の replace リスク。** `readonlyRootFilesystem`/`linuxParameters` 追加は `aws_ecs_task_definition` の新リビジョン発行(= サービスのローリング更新)であり通常は in-place 相当だが、Phase 4 の plan で replace 有無を必ず確認・報告する。
6. **`DEMO_LOG_PASSWORD` を compose に残す判断はユーザー是非の余地あり。** 既定 `demo`(非機密固定値)を表示する明示 opt-in として残す方針(§1.5)だが、「ローカルでも一切平文を出さない」を厳格にしたい場合は compose から外す選択もありうる。**外す場合は 1-C(impl-ci)の compose 変更に含める**(コード側は既に opt-in ゲート済みなので追加改修は不要)。
7. **ベースイメージ差し替えの供給網。** `nginxinc/nginx-unprivileged:alpine` は Docker 公式 nginx の姉妹イメージ(nginx 社公式)。digest pin は現行運用どおり行わない(スコープ外)。将来イメージ pin を導入するなら別途 impl-ci/impl-web で検討。
