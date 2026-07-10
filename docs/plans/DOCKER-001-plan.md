# DOCKER-001 実装計画: ローカル実行環境のコンテナ化(api / auth / web + compose)

- 種別: ツーリング(ローカル実行環境)。プロダクト機能ではないため **Spec / Issue は起点にしない**(本 plan が唯一の一次ドキュメント)
- 対象 stack: `app/api`(Go)/ `app/auth`(Go)/ `app/web`(React・nginx 配信)/ リポジトリルート(compose・Makefile)
- 成果物: 3 アプリの Dockerfile・.dockerignore(+ Go 2 つのヘルスチェックヘルパー)、ルート `compose.yml`、ルート `Makefile`
- ゴール: `make up`(ルート)で 3 サービスが起動し、下記スモーク(§テスト戦略)が全て通ること

---

## 方針

### 採用アプローチ

1. **api / auth = multi-stage + distroless 最小イメージ**
   - build stage: `golang:1.24-alpine`(musl)で `CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -trimpath` により静的バイナリを生成。標準ライブラリのみ・CGO 不要なので追加ツールは不要(`CGO_ENABLED=0` の静的ビルドのため build stage の glibc/musl 差は生成バイナリに影響せず無害。両 Dockerfile 内コメントにも意図的選択として明記)。
   - runtime stage: `gcr.io/distroless/static:nonroot`(非 root / shell 無し / 数 MB)。アプリバイナリ 1 個 + ヘルスチェックバイナリ 1 個だけを配置する。
   - アプリは `:8080`(>1024)で待受けるため nonroot でもポートバインドに特権不要。
2. **web = multi-stage(Bun ビルド)+ nginx 静的配信 + `/api` リバースプロキシ**
   - build stage: `oven/bun:1`(debian ベース / glibc)で `bun install --frozen-lockfile` → `bun run build` を実行し `dist/` を生成。
   - runtime stage: `nginx:alpine` で `dist/` を配信。nginx が `/api/` を api サービスへプロキシ(`proxy_pass http://api:8080/;` の末尾スラッシュで `/api` プレフィックスを剥がし、api の `/tasks` にマッチさせる)。SPA フォールバックは `try_files $uri $uri/ /index.html;`。
   - **app/web のコードは一切変更しない。** `VITE_API_BASE_URL` を未設定にしておけば `features/tasks/api/client.ts` の `resolveBaseUrl()` が `<origin>/api`(`DEFAULT_BASE_PATH = "/api"`)にフォールバックし `client.setConfig({ baseUrl })` で生成クライアントに適用されるため、実リクエストは `<origin>/api/tasks` → nginx が `/api` を剥がして api の `/tasks` へプロキシする(確認済み: `app/web/src/features/tasks/api/client.ts:16-37`(SPEC-003 移行後)。旧 `shared/api/http.ts` は SPEC-003 の `@hey-api/openapi-ts` 移行で削除され、同等の `/api` フォールバックはこのファイルへ移行済み。挙動は等価)。MSW は `import.meta.env.DEV` ガードで本番ビルドから tree-shake される(確認済み: `src/main.tsx` L13-17)ため、本番イメージにモックは載らない。
3. **ヘルスチェック = Go の最小静的ヘルパーバイナリ**(distroless に shell / wget / curl が無いため)
   - `app/api/cmd/healthcheck/main.go` と `app/auth/cmd/healthcheck/main.go` を新設(標準ライブラリ `net/http` のみ)。URL を引数で受け、`http.Get` して 2xx なら exit 0、それ以外 / エラーは exit 1。
   - build stage で `CGO_ENABLED=0` で静的ビルドし、最終イメージに `/healthcheck` としてコピー。
   - `HEALTHCHECK` は Dockerfile に **exec 形式**で記述(shell 不要): 例 `HEALTHCHECK CMD ["/healthcheck", "http://localhost:8080/tasks"]`。
4. **集約 = ルート `compose.yml`(compose v2、`version:` キー無し)+ ルート `Makefile`**
   - `make` の実体は `$(COMPOSE)` 変数(`docker compose` プラグイン優先・無ければ standalone `docker-compose` へ自動フォールバック)。
   - web は api の `service_healthy` に `depends_on` して起動順序を保証。

### 退けた代替案

| 案 | 退けた理由 |
|---|---|
| web を MSW スタンドアロン(モックを本番同梱)で配信 | MSW は dev 専用(`import.meta.env.DEV` ガードで prod から除去)。「本物の api に繋がる実行環境」という本タスクの目的に反し、api コンテナが無意味になる。nginx プロキシで実 api に繋ぐ。 |
| Go を `alpine`(shell あり)ベースにして `wget` でヘルスチェック | distroless は攻撃面(shell / パッケージマネージャ)が無く数 MB で最小・非 root。ヘルスチェックは 15 行の静的バイナリで代替でき、alpine を採る利点が無い。 |
| `/healthz` エンドポイントをアプリに新設 | 既存の GET `/tasks`(200)/ `/.well-known/openid-configuration`(200)で存在確認は十分。業務コードに監視専用エンドポイントを足すより、Docker 側のヘルパーで完結させる方が app 本体を汚さない。 |
| web build stage を `oven/bun:alpine`(musl) | tsgo(`@typescript/native-preview`)はネイティブバイナリを取得する。musl / glibc の齟齬を避けるため debian ベース(glibc)の `oven/bun:1` を使う(§リスク参照)。 |
| compose を `docker compose` 決め打ち | 本環境ではプラグインが無く standalone `docker-compose` のみの場合がある。Makefile で自動検出する。 |
| web に `VITE_API_BASE_URL` を build ARG で注入 | 未設定時の `/api` フォールバックで要件を満たすため不要。将来別 origin の api を指す場合の拡張余地として残すのみ(実装しない)。 |

---

## impl agent 向け「契約」(この 3 者はこの表に従えば並列実装可能)

後続の impl-api / impl-web / impl-ci は互いのファイルを見ずとも、以下の固定値だけ守れば結線が成立する。

| 項目 | 値 |
|---|---|
| compose サービス名 | `api` / `auth` / `web` |
| イメージ名(compose `image:`) | `cc-orchestrator/api:local` / `cc-orchestrator/auth:local` / `cc-orchestrator/web:local` |
| build context | `./app/api` / `./app/auth` / `./app/web`(各 context の `Dockerfile`) |
| コンテナ待受ポート | api `8080` / auth `8080` / web `80`(nginx) |
| host 公開ポート(`host:container`) | web `8080:80` / api `8081:8080` / auth `8082:8080` |
| web → api 参照先(nginx `proxy_pass`) | `http://api:8080/`(**末尾スラッシュ必須**。内部 DNS でサービス名 `api` を解決) |
| auth 環境変数(compose `environment`) | `ISSUER=http://localhost:8082`(host ポートに一致させ discovery の issuer を揃える) |
| api / auth ヘルスチェックバイナリパス | `/healthcheck`(引数に URL を渡す exec 形式) |
| api ヘルスチェック URL | `http://localhost:8080/tasks`(GET 200) |
| auth ヘルスチェック URL | `http://localhost:8080/.well-known/openid-configuration`(GET 200) |
| 起動順序 | web は `depends_on: { api: { condition: service_healthy } }` |
| restart ポリシー | 全サービス `unless-stopped` |

注: api / auth はコンテナ内で常に `8080` を待受ける(`PORT` 未設定でデフォルト 8080)。host 側の 8081 / 8082 との対応付けは compose の `ports` だけで行い、`PORT` 環境変数は上書きしない。

---

## 変更ファイル

### app/api(impl-api)
- `app/api/Dockerfile`(新規): multi-stage(golang:1.24-alpine → distroless static:nonroot)。`./cmd/api` と `./cmd/healthcheck` を静的ビルドし、後者を `/healthcheck` として配置。`EXPOSE 8080`、`HEALTHCHECK CMD ["/healthcheck", "http://localhost:8080/tasks"]`、`ENTRYPOINT ["/api"]`。
- `app/api/.dockerignore`(新規): `.git` / `docs/` / `README.md` / `Makefile` 等ビルド不要物を除外(Go ソース・go.mod は残す)。
- `app/api/cmd/healthcheck/main.go`(新規): 標準ライブラリのみ。URL を `os.Args[1]`(未指定時はデフォルト `http://localhost:8080/tasks`)で受け、`http.Get` → 2xx で exit 0 / それ以外・エラーで exit 1。**golangci-lint に通るクリーンな実装**(エラー処理・レスポンス body の close を行う)。

### app/auth(impl-api)
- `app/auth/Dockerfile`(新規): api と同型。ビルド対象は `./cmd/authz` と `./cmd/healthcheck`。`HEALTHCHECK CMD ["/healthcheck", "http://localhost:8080/.well-known/openid-configuration"]`、`ENTRYPOINT ["/authz"]`。
- `app/auth/.dockerignore`(新規): api と同方針。
- `app/auth/cmd/healthcheck/main.go`(新規): api と同一実装(デフォルト URL のみ `.../openid-configuration` に変更)。

### app/web(impl-web)
- `app/web/Dockerfile`(新規): multi-stage(oven/bun:1 → nginx:alpine)。build stage で `bun install --frozen-lockfile`(`bun.lock` が存在するため frozen 可)→ `bun run build`。runtime stage で `dist/` を `/usr/share/nginx/html` へ、`nginx.conf` を `/etc/nginx/conf.d/default.conf` へコピー。`EXPOSE 80`。web のヘルスチェックは nginx:alpine の busybox `wget` が使えるため任意で `HEALTHCHECK CMD ["wget","-q","-O","-","http://localhost:80/"]` を付与可(必須ではない)。
- `app/web/.dockerignore`(新規): **`node_modules` と `dist` を必ず除外**(コンテキスト肥大 / キャッシュ汚染防止)。加えて `.git` / `*.local` / `.env*`。`bun.lock` / `bunfig.toml` / `package.json` / `src` / `public` / `index.html` / 各 config は残す。
- `app/web/nginx.conf`(新規): server ブロック 1 つ。`location / { try_files $uri $uri/ /index.html; }` と `location /api/ { proxy_pass http://api:8080/; }`(末尾スラッシュで `/api` を剥がす)。`/etc/nginx/conf.d/default.conf` を置換する用途。

### ルート(impl-ci)
- `compose.yml`(新規): service `api` / `auth` / `web`。各 `build.context`・`image`・`ports`・`restart` を契約表どおりに。auth に `environment: ISSUER=http://localhost:8082`。web に `depends_on: api: condition: service_healthy`。healthcheck は各 Dockerfile の `HEALTHCHECK` に委ねる(compose で上書きしない)。`version:` キーは書かない。
- `Makefile`(新規): 既存 `app/api/Makefile` と同スタイル(`.DEFAULT_GOAL := help`、`## ` コメントで help 生成、`.PHONY`、日本語説明)。冒頭に `COMPOSE := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")`。ターゲット: `help` / `build` / `up`(ビルドしてフォアグラウンド) / `up-d`(デタッチ) / `down` / `logs`(追従) / `ps` / `restart` / `clean`(`down -v`)。各実体は `$(COMPOSE) ...`。

---

## 手順

前提: 3 者は上記「契約」に従うため **相互に依存せず並列実装可能**。impl-ci の compose / Makefile は Dockerfile の中身に依存せず(context とサービス名・ポートのみ参照)、Go のヘルスチェックバイナリ実装も web と独立。

### フェーズ 1(並列): 各 stack の実装
1. **impl-api**(app/api・app/auth をまとめて担当)
   - `app/api/cmd/healthcheck/main.go`・`app/auth/cmd/healthcheck/main.go` を追加。
   - `app/api/Dockerfile`・`app/auth/Dockerfile`・両 `.dockerignore` を追加。
   - 追加した Go パッケージが lint / build / test を壊さないことを確認できる状態にする(実行検証は tester / checker が担当)。
2. **impl-web**(app/web を担当)
   - `app/web/Dockerfile`・`app/web/.dockerignore`・`app/web/nginx.conf` を追加。
   - `bun install --frozen-lockfile` が Docker build stage で通る前提を確認(`bun.lock` を context に含める)。
3. **impl-ci**(ルートを担当)
   - `compose.yml`・`Makefile` を追加。

### フェーズ 2(直列): 品質チェック
4. **checker**: `app/api` と `app/auth` で `make check`(= fmt-check + lint + vet + build + test)を実行し、追加した `cmd/healthcheck` が fmt / lint / vet / build を通ることを確認。app/web はコード変更が無い(設定ファイル追加のみ)ため既存 `bun run typecheck` / `lint` は影響を受けないが、念のため実行。
   - Dockerfile / nginx.conf / compose.yml / Makefile はこれらの lint 対象外(必要なら hadolint 等は本タスクの範囲外)。

### フェーズ 3(直列): コンテナ結合スモーク(§テスト戦略)
5. **tester**: `make build` → `make up-d` → §テスト戦略のスモーク項目を curl で検証 → `make down`。失敗時は該当 impl agent に差し戻し、フェーズ 1→3 を再実行。

---

## テスト戦略

このタスクの検証対象は単体テストではなく **イメージのビルド成功とコンテナ結合スモーク**。tester が以下を上から順に確認する(ルートで実行)。

| # | 手順 | 期待 |
|---|---|---|
| S0 | `make build`(= `$(COMPOSE) build`) | api / auth / web の 3 イメージが全てビルド成功 |
| S1 | `make up-d` 後、api / auth が `healthy` になるまで待機し `make ps` | 3 サービスが Up、api / auth が `(healthy)` |
| S2 | `curl -fsS http://localhost:8081/tasks` | HTTP 200 + JSON(タスク一覧。空でも 200)。api コンテナへの直接疎通 |
| S3 | `curl -fsS http://localhost:8082/.well-known/openid-configuration` | HTTP 200 + discovery JSON。`issuer` が `http://localhost:8082` になっている(ISSUER 反映の確認) |
| S4 | `curl -fsS http://localhost:8080/` | HTTP 200 + `index.html`(nginx が dist を配信) |
| S5 | `curl -fsS http://localhost:8080/api/tasks` | HTTP 200 + JSON(nginx が `/api` を剥がし api の `/tasks` へプロキシ。S2 と同等レスポンス) |
| S6 | 存在しない SPA パス `curl -fsS http://localhost:8080/some/spa/route` | HTTP 200 + `index.html`(`try_files` フォールバック) |
| S7 | `make down` | 3 コンテナが停止・削除される |

補足:
- Go ヘルパー(`cmd/healthcheck`)は `make check`(build / vet / lint / test)で健全性を担保する。ヘルパー自体の振る舞い(2xx→0 / それ以外→1)は S1 のコンテナ healthy 遷移で間接検証される。単体テストを付けるかは impl-api / tester の判断に委ねる(std-lib のみ・十数行のため必須とはしない)。
- 各 curl の期待レスポンス本文の厳密一致までは求めず、HTTP ステータス(`-f` で 4xx/5xx を失敗扱い)と JSON/HTML の種別が一致すれば合格とする。

---

## リスク / 未確定事項

- **`docker compose` プラグイン非対応環境**: 本環境ではプラグインが無く standalone `docker-compose` のみの可能性。→ Makefile の `COMPOSE` 自動検出で吸収する方針だが、どちらのバイナリも無い環境では tester の S0 以降が実行不能。その場合は tester が「docker 実行環境が無い」旨を報告し、成果物(ファイル一式)のレビューに切り替える。
- **tsgo(`@typescript/native-preview`)の Docker 内挙動**: tsgo はネイティブバイナリをプラットフォーム別に取得する。build stage を glibc(`oven/bun:1` debian)にして musl 齟齬を避ける方針。ビルド対象アーキ(arm64 / amd64)の tsgo バイナリが取得できない場合 `bun run build` の `tsgo --noEmit` が失敗しうる。→ impl-web は build が通らない場合、フォールバックとして build を `vite build` 単体にせず、事象を報告する(型チェックを落とすのは web 規約に反するため勝手に外さない)。
- **`bun install --frozen-lockfile` と minimumReleaseAge ゲート**: `bunfig.toml` に 21 日ゲートがあるが、`bun.lock` 済み依存はゲートを通過する(web 規約記載)。frozen なら新規解決は起きない想定。ロックと `package.json` が乖離していると frozen が失敗するため、impl-web は Docker build 前に整合を確認する。
- **auth ISSUER と host ポートの整合**: `ISSUER=http://localhost:8082` は「host からアクセスする」前提。コンテナ間・別ホストからアクセスする運用に変わると discovery の issuer / エンドポイント URL が実アクセス経路と食い違う。今回はローカル実行前提で固定とし、本番相当は別途(https issuer)対応する。
- **distroless での HEALTHCHECK**: exec 形式(`["/healthcheck","<url>"]`)で shell 非依存にする。shell 形式(`CMD /healthcheck ...`)にすると distroless では動かないため、impl-api は必ず exec 配列形式で書くこと。
- **nginx `proxy_pass` の末尾スラッシュ**: `http://api:8080/`(スラッシュ有り)が必須。無しだと `/api` プレフィックスが剥がれず api 側で 404 になる。S5 がこの回帰を検出する。
- **healthcheck ヘルパー追加による `go build ./...` / lint 対象増**: `make check` の対象が増える。impl-api はレスポンス body の `Close`・エラー処理を含む lint クリーンな実装にする(未処理エラーや `errcheck` 違反に注意)。
- **web の api クライアント契約ドリフト(参考・本タスク非対象)**: `features/tasks/api/client.ts` は `PATCH /tasks/{id}/status` を叩くが、api 実装は `POST /tasks/{id}/start|complete` のみ(SPEC-003 で既知)。本コンテナ化タスクのスモーク(GET /tasks・POST /tasks 系)には影響しないため対象外とし、ここに事実のみ記録する。
