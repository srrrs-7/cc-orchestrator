---
id: ISSUE-056
title: runtime コンテナの最小権限ハードニング不足(ECS タスク定義の readonlyRootFilesystem / cap_drop 未設定、公開 nginx が root 常駐、compose の cap_drop/read_only 未設定)
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-25
updated: 2026-07-25
specs: [SPEC-016]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-056: runtime コンテナの最小権限ハードニング不足(ECS / nginx / compose)

**深刻度: Major 2 件 + Minor 2 件(review) / severity: medium**(現時点で機能への影響・active exploit はないが、本番 ECS + 外部公開 nginx を含む runtime コンテナに最小権限=多層防御が未適用で、別の脆弱性が突かれた際の被害範囲(blast radius)を抑えられていない。「最小マウント」は達成済みだが「最小権限」に穴がある)

## 1. ユーザー価値への影響(なぜ対応するか)

> **本番 ECS の運用者・セキュリティ担当** の **多層防御(最小権限)による被害限定** が **runtime コンテナに cap_drop / readonlyRootFilesystem / non-root が未適用なため確保できていない**。外部公開面(web / auth-web)経由のエンドユーザーには間接的な保護低下として波及する。

- **影響を受けるユーザー**: 本番 ECS の運用者・セキュリティ担当(直接)。外部公開面(web / auth-web)を利用するエンドユーザー(間接=公開コンポーネントが root で常駐)
- **損なわれる価値**: 多層防御(最小権限)。単体では機能に影響しないが、別の脆弱性(アプリ RCE・依存の脆弱性等)が成立した際に、権限昇格・ファイル改ざん・capability 悪用を封じる防壁が無い
- **影響範囲・頻度**: 本番 ECS は常時(全タスク定義で未設定)。公開 nginx(web / auth-web)は常時 root 常駐。ローカル compose は常時。実害は「別の侵入経路が成立した場合」に顕在化する(現時点で active exploit は確認されていない)
- **回避策**: なし(設定を追加する以外に緩和策はない)。ただし現状で機能は正常に動作しており、機能的な回避策は不要

## 2. 現象(何が起きているか)

### 期待する動作

runtime コンテナは最小権限で常駐する:

- ECS Fargate タスク定義に `readonlyRootFilesystem: true` と `linuxParameters.capabilities.drop: ["ALL"]` を設定する(Fargate は両方サポートする)
- 外部公開の nginx(web / auth-web)は non-root ユーザーで、非特権ポートを listen して起動する
- ローカル compose の各サービスも `cap_drop: [ALL]`(+ 可能なら `read_only: true`)を設定する
- ローカル専用の固定値(例: `ADMIN_API_KEY`)には「非機密のローカル専用・実環境に転用しない」旨の注記を付ける

### 実際の動作

以下の 4 点で最小権限が満たされていない(危険な bind mount はなく「最小マウント」自体は達成済み):

1. **[Major] ECS タスク定義に readonlyRootFilesystem / cap_drop が未設定** — `app/iac/modules/service/ecs.tf`。`local.app_container`(66-101 行)と migration コンテナ定義に `readonlyRootFilesystem` / `linuxParameters.capabilities.drop` が一切ない。タスク定義本体は 104-121 行(`aws_ecs_task_definition.this`)で、コンテナ定義は `container_definitions = jsonencode(concat(local.migration_container_defs, [local.app_container]))`(118 行)。distroless static / nonroot の api・auth・migrator は実行時にファイル書き込みも capability も不要なため、`cap_drop ALL` + `readonlyRootFilesystem: true` を副作用なく適用できる。本番実運用に直結するため最重要。
2. **[Major] 公開 nginx が root 常駐** — `app/web/Dockerfile:16-26` / `app/auth-web/Dockerfile:12-22`。最終ステージ(`FROM nginx:alpine AS runtime`)に `USER` 指定がなく、nginx master プロセスが root で常駐する(worker のみ nginx ユーザーへ降格)。両 Dockerfile は `EXPOSE 80` + `CMD ["nginx", "-g", "daemon off;"]`。web / auth-web は外部公開面に直接晒されるコンポーネント。非特権ポート(8080)listen + non-root 起動化が推奨(80 番バインド用 `CAP_NET_BIND_SERVICE` の必要性も同時に消える)。
3. **[Minor] compose の 5 サービスに cap_drop / read_only が未設定** — ルート `compose.yml`。postgres / api / auth / web / auth-web すべてに `cap_drop: [ALL]` と `read_only: true` がない(`security_opt: ["no-new-privileges:true"]` のみ設定済み: 51-52 / 103-104 / 140-141 / 153-154 / 168-169 行)。対照的に `.devcontainer/compose.tools.yml` の toolchain コンテナ(`x-tools-base`)は `no-new-privileges` に加えて `cap_drop: ALL` も設定済み(185-188 行)で、アプリ本体側より capability 面で厳格という逆転がある。
4. **[Minor] ローカル専用固定値に非機密の注記がない** — `compose.yml:121` の `ADMIN_API_KEY: local-dev-admin-key` に「非機密のローカル専用固定値・実環境に転用しない」旨のコメントがない。DB 資格情報側には同種の注記がある(14-17 行「The credentials below are fixed, non-secret local-only defaults ... never reused for any real environment」)。

### 再現手順

1. `app/iac/modules/service/ecs.tf` の `local.app_container`(66-101 行)と `aws_ecs_task_definition.this`(104-121 行)を確認し、`readonlyRootFilesystem` / `linuxParameters` が無いことを確認する。
2. `app/web/Dockerfile:16-26` と `app/auth-web/Dockerfile:12-22` の runtime ステージに `USER` 指定が無いことを確認する。
3. ルート `compose.yml` の 5 サービスに `cap_drop` / `read_only` が無いこと、`.devcontainer/compose.tools.yml:185-188` には `cap_drop: ALL` があることを対比する。
4. `compose.yml:121` の `ADMIN_API_KEY: local-dev-admin-key` に非機密注記が無いことを確認する。

### 環境・条件

- 対象 stack: iac(ECS Fargate)/ web・auth-web(Dockerfile / nginx)/ リポジトリルート(compose.yml)
- 関連領域: 本番 ECS デプロイ(SPEC-004)、toolchain コンテナとの権限対称性(SPEC-009。上記 3 の逆転が該当)

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: ECS タスク定義(`ecs.tf` の `local.app_container` 66-101 行、`aws_ecs_task_definition.this` 104-121 行)に `readonlyRootFilesystem` / `linuxParameters.capabilities.drop` が無い(Fargate は両方サポート)。
- 事実: `app/web/Dockerfile:16-26` / `app/auth-web/Dockerfile:12-22` の nginx:alpine runtime ステージに `USER` 指定が無く、`EXPOSE 80` + `CMD ["nginx", "-g", "daemon off;"]` で root 常駐。
- 事実: `compose.yml` の 5 サービスに `cap_drop` / `read_only` が無い(`security_opt: no-new-privileges` のみ)。`.devcontainer/compose.tools.yml:185-188` の `x-tools-base` は `cap_drop: ALL` + `no-new-privileges` を設定済み(ただし workspace を read/write bind-mount するため `read_only: true` は設定していない=逆転は capability の点に限る)。
- 事実: `compose.yml:121` の `ADMIN_API_KEY: local-dev-admin-key` に非機密注記が無い一方、DB 資格情報(14-17 行)には非機密注記がある。
- 事実: web / auth-web の compose ポートマッピングはコンテナ側 80(`127.0.0.1:8080:80` / `127.0.0.1:8083:80`。152 / 167 行)。non-root 化で非特権ポート(8080)listen に変える場合、この container port と ECS 側 container port を追随させる必要がある(ホスト側 / ALB 側の公開ポートは不変)。
- 仮説: 「最小マウント」(危険な bind mount を作らない)は満たされているが、「最小権限」(cap_drop / readonly / non-root)がコンテナ設計の初期方針に含まれておらず、後追いのハードニングとして残っている。

### 根本原因

runtime コンテナ(ECS タスク定義・公開 nginx イメージ・ローカル compose)の設計に最小権限(cap_drop / readonlyRootFilesystem / non-root)が組み込まれていない。マウント面は最小化されているが権限面が未対応で、toolchain コンテナ(SPEC-009)より緩いという非対称も生じている。

## 4. 対応(どう解決するか)

### 対応方針

**実装計画: [`docs/plans/ISSUE-056-plan.md`](../plans/ISSUE-056-plan.md)**(ISSUE-051 を同梱。4 stack 分担: impl-iac=ECS `readonlyRootFilesystem`+`cap drop ALL` / impl-web=nginx を `nginx-unprivileged` で non-root + 8080 化 / impl-ci=`compose.yml` の `cap_drop`/`read_only`/tmpfs/ポート追随/注記 / impl-auth=デモログ opt-in 一本化)。コンテナ内 listen ポート=8080 を契約として固定し 4 レーン並列着手、統合検証は `make up-d` smoke + `make iac-plan` 差分で行う。確定した前提: web/auth-web は ECS 非対象(nginx のポート変更は ECS へ波及せず api/auth の container_port は 8080 不変)/ api/auth/migrator は distroless nonroot・無書き込みで `readonlyRootFilesystem` 安全 / postgres の read_only は見送り(cap_drop+最小 cap_add で担保)。

3 stack に分担して適用する(scope が独立しているため並列可)。副作用の小さい順に、api / auth / migrator は即適用可能、公開 nginx と postgres / web / auth-web の read_only は書き込み先(tmpfs 等)の設計を伴う。

- **impl-iac**: ECS タスク定義(`ecs.tf`)の app / migration コンテナに `readonlyRootFilesystem: true` + `linuxParameters.capabilities.drop: ["ALL"]` を追加。適用後 `make iac-plan ENV=dev` の差分を報告(apply はユーザー判断)。
- **impl-web**: `app/web/Dockerfile` / `app/auth-web/Dockerfile` の nginx を non-root + 非特権ポート(8080)listen 化(`nginx.conf` の listen ポート・`USER`・`EXPOSE`・`HEALTHCHECK` の URL を追随)。ECS / compose の container port 追随が必要な点を impl-iac / impl-ci と調整。
- **impl-ci**: ルート `compose.yml` の各サービスへ `cap_drop: [ALL]`(可能なら `read_only: true` + 必要な tmpfs)を追加。nginx non-root 化に伴う container port(80→8080)の追随。`ADMIN_API_KEY` に非機密のローカル専用注記を追加。

### 実施内容

- [x] [Major] ECS タスク定義(app / migration コンテナ)に `readonlyRootFilesystem: true` + `cap_drop: ["ALL"]` を追加(impl-iac。`ecs.tf` 51-56 / 100-105 行で確認)
- [x] [Major] 公開 nginx(web / auth-web)を non-root + 非特権ポート(8080)listen 化(impl-web。`nginxinc/nginx-unprivileged:alpine`(uid 101)採用・`EXPOSE 8080`・HEALTHCHECK 8080 化)。compose の container port は追随済み(impl-ci)。web/auth-web は ECS 非対象のため ECS 追随は不要
- [x] [Minor] compose の 5 サービスに `cap_drop: [ALL]` を追加、api / auth / web / auth-web は `read_only: true` + 最小 tmpfs、postgres は最小 `cap_add` で担保(impl-ci)
- [x] [Minor] `compose.yml` の `ADMIN_API_KEY` に非機密のローカル専用注記を追加(impl-ci)
- [x] 各 stack の `make check`(auth / web / auth-web / iac)green・`make up-d` の 5 サービス疎通/healthy・`make test-db`(REQUIRE_DB=1)全 pass を確認(checker / tester)。**`make iac-plan ENV=dev` は AWS 認証情報が無く未実行**(ECS への適用=plan で replace が無いことの確認 + apply はユーザー側デプロイ作業として残る)

### 再発防止

- 新規 runtime コンテナ(Dockerfile / ECS タスク定義 / compose サービス)は最小権限(cap_drop / readonlyRootFilesystem / non-root / 非特権ポート)を初期方針に含める。review-security のチェック観点に runtime 権限を明示する。
- toolchain コンテナ(SPEC-009)とアプリ本体コンテナの権限対称性(アプリ側が toolchain より緩くならない)をレビュー観点にする。

## 5. 経緯(時系列・追記のみ)

### 2026-07-25

- 起票。発見経緯: review-security によるコンテナ最小権限レビュー(2026-07-25)。「最小マウント」は達成済み(危険な bind mount なし)だが「最小権限」に Major 2 件 + Minor 2 件のギャップを確認した。
- 事実確認: ECS タスク定義(`app/iac/modules/service/ecs.tf` 66-121 行)に `readonlyRootFilesystem` / `linuxParameters.capabilities.drop` が無い / nginx runtime ステージ(`app/web/Dockerfile:16-26`・`app/auth-web/Dockerfile:12-22`)に `USER` 指定が無く root 常駐 / `compose.yml` の 5 サービスに `cap_drop`・`read_only` が無い(`.devcontainer/compose.tools.yml:185-188` の toolchain は `cap_drop: ALL` 済みで capability 面が逆転)/ `compose.yml:121` の `ADMIN_API_KEY` に非機密注記が無い、をそれぞれ現物で確認した。
- 分担予定: impl-iac(ECS)/ impl-web(Dockerfile・nginx.conf)/ impl-ci(compose.yml)。
- 実装計画を [`docs/plans/ISSUE-056-plan.md`](../plans/ISSUE-056-plan.md) に策定(planner)。ISSUE-051(デモパスワードログ opt-in 一本化)を同一 review-security 由来の一括ハードニングとして同梱。決定事項: (1) コンテナ内 listen ポート=8080 を契約固定し impl-web/impl-ci/impl-iac/impl-auth の 4 レーンを並列着手、(2) nginx は `nginxinc/nginx-unprivileged:alpine` 採用(手動 USER 化を退けた。read_only との相性・書き込みパス漏れの少なさから)、(3) **web/auth-web は本番 ECS 非対象**(`envs/dev/main.tf` は service_api/service_auth のみ)を現物確認 → nginx のポート変更は ECS に波及せず api/auth の container_port は 8080 不変・cap_add 不要、(4) api/auth/migrator は distroless static nonroot で runtime 無書き込み(`os.WriteFile` 等の全探索ヒットは keygen のみ=別バイナリ)→ `readonlyRootFilesystem:true` を副作用なく適用可、(5) **postgres は read_only 見送り**(entrypoint がボリューム外に多書き込み・ローカル DB のため便益小)= `cap_drop:[ALL]` + 最小 `cap_add:[CHOWN,DAC_OVERRIDE,FOWNER,SETGID,SETUID]` + no-new-privileges で担保。検証は `make up-d` の 5 サービス疎通/healthy + postgres 空ボリューム初回 init + `make iac-plan ENV=dev` 差分(replace でないこと)。status は実装未完のため open 維持。

### 2026-07-25(実装・検証・レビュー完了 → resolved)

- 実装(Phase 1・4 レーン並列で完了):
  - impl-web: `app/web` / `app/auth-web` の nginx を `nginxinc/nginx-unprivileged:alpine`(master を uid 101 で non-root 常駐)+ listen 8080 化。`EXPOSE 8080` / HEALTHCHECK を 8080 に追随(`app/web/Dockerfile:55,60,63` / `app/auth-web/Dockerfile:34,39,42` で確認)。
  - impl-iac: `app/iac/modules/service/ecs.tf` の app / migration 両コンテナに `readonlyRootFilesystem = true` + `linuxParameters.capabilities.drop = ["ALL"]` を追加(51-56 / 100-105 行で確認)。
  - impl-ci: ルート `compose.yml` を、5 サービス全て `cap_drop: [ALL]` / postgres は最小 `cap_add`(CHOWN, DAC_OVERRIDE, FOWNER, SETGID, SETUID)/ api・auth・web・auth-web は `read_only: true` + 最小 `tmpfs` / ポート右辺を 8080 追随(`127.0.0.1:8081:8080` 等)/ `ADMIN_API_KEY`・`DEMO_LOG_PASSWORD` に非機密ローカル専用の注記を追加。
  - impl-auth: `cmd/authz/env.go` に `LogDemoPassword`(`os.Getenv("DEMO_LOG_PASSWORD") == "1"`。113 行)を追加し、`main.go` のログ条件を `if e.LogDemoPassword {`(163 行)= `DEMO_LOG_PASSWORD == "1"` 単独 opt-in に一本化(ISSUE-051 同梱分)。
- テスト検証(事実): tester が `cmd/authz/env_test.go` に `TestNewEnv_LogDemoPassword`(413 行〜。`DEMO_PASSWORD` 非空だけでは `LogDemoPassword=false` を固定する回帰ケース含む)を追加し 15/15 pass。checker が auth / web / auth-web / iac の `make check`(iac は fmt-check + validate + lint + security)を全 green。
- 統合検証(Phase 3・事実): `make up-d` で 5 サービス healthy・全疎通 200。postgres 空ボリュームの初回 init が最小 `cap_add` で成功。nginx が uid 101・distroless(api/auth/migrator)が uid 65532 で常駐することを実測。`DEMO_PASSWORD` 単独ではパスワード非出力・`DEMO_LOG_PASSWORD=1` でのみ平文出力を実挙動で確認。`make test-db`(REQUIRE_DB=1)で api / auth の DB 依存テスト含め全 pass。
- レビュー検証(Phase 5・事実): review-security 再レビューで Blocker / Major / Minor ゼロ(Info 3 件のみ)。review-spec で要件 11/11 充足・既存 Spec(009 コンテナ toolchain / 014 CSP / 015 OIDC / 004 デプロイ)との drift なしを確認。
- 判定: Issue が対象とする「runtime コンテナへの最小権限(cap_drop / readonlyRootFilesystem / non-root / 非特権ポート)適用」というコード・設定の修正を、checker(iac 含む全 stack green)+ tester(回帰テスト追加・全 pass)+ 実挙動(`make up-d` / `make test-db`)+ review-security/spec(指摘ゼロ・要件充足)で検証できたため status を `resolved` に遷移。
- 残る未実施事項(コード修正の verification gap ではなくデプロイ作業): `make iac-plan ENV=dev` は AWS 認証情報が無く未実行のため、ECS 側の適用差分(readonlyRootFilesystem / cap_drop 追加が新タスク定義リビジョンの in-place ロールで replace を伴わないこと)の plan 確認 + apply はユーザー側デプロイで行う。CLAUDE.md の方針(`terraform apply` は実行せず apply の判断はユーザーに委ねる)どおり、Issue のコード解決スコープ外として resolved に含めず本項に明記する。
