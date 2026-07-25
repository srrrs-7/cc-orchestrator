---
id: SPEC-016
title: 実行境界(runtime はコンテナのみ / ホスト実行禁止)と runtime コンテナの最小権限・サプライチェーン契約
status: in-progress  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-25
updated: 2026-07-25
issues: [ISSUE-056, ISSUE-057]  # 関連Issue ID
supersedes: null # 置き換える旧Spec ID
---

# SPEC-016: 実行境界(runtime はコンテナのみ / ホスト実行禁止)と runtime コンテナの最小権限・サプライチェーン契約

## 1. ユーザー価値(なぜ作るか)

> **このリポジトリの開発者と本番 ECS の運用者** が **アプリを常に「最小権限・最小マウントの runtime コンテナ」でだけ動かせるようになり**、**汚染された依存パッケージ(Shai-Hulud 型 npm ワーム)や侵害されたプロセスが、開発マシンの資格情報・リポジトリ・本番へ横展開できない** という価値を得る。

- **対象ユーザー**: このリポジトリの開発者(直接)、本番 ECS の運用者・セキュリティ担当(直接)、web / auth-web の利用者(間接=公開面が非 root・読み取り専用になる)
- **解決する課題**: SPEC-009 は「検査・ビルド・依存解決」をコンテナに閉じたが、**アプリの起動(runtime)は対象外**として明示的にホスト実行を許していた(`app/{api,auth}/Makefile` の `run` = `go run ./cmd/...`)。またマイグレーションは runtime バイナリなのに toolchain コンテナ内で `go run` されており、依存インストール面(web / auth-web の Dockerfile build ステージ)にはリポジトリのサプライチェーンゲート(社内ミラー固定 + `minimumReleaseAge`)が効いていなかった。結果として「runtime をどこで動かすか」「その面にどの権限とマウントを与えるか」が契約として存在しなかった
- **得られる価値**: どの経路で動かしても runtime は同一アーティファクト(= 本番 ECS と同じイメージ)・同一の最小権限になる。ホストの資格情報とリポジトリは runtime から構造的に到達不能になり、npm 由来コードの実行はビルド時の 1 点に閉じてゲート下に置かれる
- **価値の検証方法**: 以下がすべて満たされたら成功とみなす。(a) ホスト / toolchain コンテナで runtime を起動しようとすると明示エラーで落ちる。(b) `make up-d` 後、5 サービスすべてが healthy で 4 エンドポイントが応答し、`docker inspect` で全サービスが `cap_drop: ALL` + `no-new-privileges`、api / auth / web / auth-web が `read_only: true`、web / auth-web が非 root(uid 101)である。(c) runtime コンテナのマウントが `pgdata`(+ 任意の署名鍵 read_only bind)だけである。(d) マイグレーションがローカル・CI・ECS で同一イメージから適用される。(e) CI(`cicd.yml`)が緑で通る

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- 開発者として、`make up-d` か `make api-run` だけでアプリを起動したい。なぜならホストに go / bun / nginx / postgres を入れたくないし、起動したプロセスに自分のホーム配下や `~/.aws` を触らせたくないからだ。
- 開発者として、ホストで `go run ./cmd/api` を打ったら**止められたい**。なぜなら「手元だけの例外」が積み重なると本番と違う経路が常態化し、権限境界が絵に描いた餅になるからだ。
- 運用者として、ローカルで適用しているマイグレーションと本番 ECS の init コンテナが同一イメージであってほしい。なぜなら「ローカルでは通るが本番で違う挙動」を構造的に消せるからだ。
- セキュリティ担当として、汚染された npm パッケージが混入しうる面が「web / auth-web のイメージビルド 1 箇所」に限定され、そこにレジストリ固定・公開経過日数ゲート・lifecycle script 無効化が効いていてほしい。

### 利用フロー

1. 開発者が `make up-d` を実行する(または単一サービスだけなら `make api-run` / `make auth-run`)
2. `make migrate` が compose の `migrator` サービス(= `app/migrator/Dockerfile` の runtime イメージ)で DB を作成・マイグレーション適用する
3. 5 サービスが最小権限(non-root / read-only rootfs / `cap_drop: ALL` / `no-new-privileges`)で起動し、`127.0.0.1` にのみ publish される
4. 開発者がホストで `go run ./cmd/api` を打つ、または toolchain コンテナ内で `make run` を打つと、**理由と正規経路を示すエラーで落ちる**
5. 依存を追加して `make -C app/web install` → イメージビルド時も同じ `bunfig.toml` のゲート(社内ミラー + 21 日隔離)と `--ignore-scripts` が適用される

## 3. 要件(何を満たすべきか)

### 機能要件

- [x] R1: 実行経路を **toolchain レーン**(検査・ビルド・生成・依存解決 + リポジトリへ書き込む生成 CLI)と **runtime レーン**(常駐プロセス + DB マイグレーション)の 2 つだけに定義し、CLAUDE.md に唯一の正として明記する
- [x] R2: ホストでの runtime 起動を禁止する。`app/{api,auth}/Makefile` の `run` は compose の該当サービス(= その stack の runtime イメージ)を起動する形に置き換える
- [x] R3: toolchain コンテナ内での runtime 起動を禁止する。`IN_TOOLBOX=1` で `run` を叩いた場合は理由と正規経路を示して非ゼロ終了する(api / auth / migrator の 3 Makefile)
- [x] R4: DB マイグレーションを runtime レーンへ移す。`make migrate` / `migrate-test` / `make -C app/migrator run` は compose の profile 付き `migrator` サービス(`app/migrator/Dockerfile` の runtime イメージ、マイグレーション SQL は焼き込み、`dbnet` のみ、`read_only` + `cap_drop: ALL`)で実行する。CI も同一経路にする
- [x] R5: runtime コンテナは最小権限とする。全サービス `cap_drop: ALL` + `no-new-privileges`、api / auth / web / auth-web は `read_only: true` + 必要最小 tmpfs、公開 nginx は非 root + 非特権ポート 8080、ECS は `readonlyRootFilesystem: true` + `capabilities.drop: ["ALL"]`(ISSUE-056 で実施)
- [x] R6: runtime コンテナのマウントを最小にする。`pgdata`(postgres)と auth の署名鍵ファイル(read_only bind、既定無効)以外を持たない。リポジトリ・docker.sock・ホスト資格情報は渡さない
- [x] R7: サプライチェーンゲートをイメージビルドにも効かせる。build ステージは `bunfig.toml` を `bun install` の前に COPY し、`--frozen-lockfile --ignore-scripts` で実行する。build context から `node_modules` / `dist` / `.env*` / `.secrets/` を除外する
- [x] R8: runtime のベースイメージを **tag + immutable digest** で固定する(api / auth / migrator / web / auth-web の build+runtime ステージ、compose の postgres)。multi-arch index digest を使い ECS の linux/arm64 を維持する

### 非機能要件

- 既存のコマンド契約(`make check` / `test` / `up` / `up-d` / `migrate` …)の**名前と意味を変えない**。変わるのは実行される場所と権限だけ(SPEC-009 R4 と同じ原則)
- ホスト前提は Docker のみを維持する
- ローカル起動のホスト側公開ポート(8080 / 8081 / 8082 / 8083 / 5432)を変えない
- マイグレーション適用の所要時間がイメージビルド分だけ増えるが、レイヤキャッシュにより 2 回目以降は数秒に収まること

### スコープ外(やらないこと)

- **runtime サービスのインターネット egress 遮断**: `internal: true` ネットワークが唯一の手段だが、実測(2026-07-25、Docker 29.2.1)で publish ポートと両立しないため採らない(§4 参照)。publish しない面(`tools-offline` / `tools-db` / `migrator`)のみ egress を落とす
- **toolchain イメージのベースイメージ digest 固定**: `versions.env` のタグ運用のまま(build args 経由で解決しており、runtime 面より優先度が低い)。将来やるなら別 Spec
- **postgres の `read_only: true`**: ローカル専用のため見送り(理由は `compose.yml` のコメントが正。ISSUE-056 の判断を踏襲)
- ホストでの `aws` CLI 実行(`push-images` / `deploy-web`)の変更。SPEC-009 R7 のまま、資格情報をコンテナへ渡さない方針を維持する

## 4. 設計(どう実現するか)

### 方針

**「実行面 = レーン」を 2 つに固定し、レーンごとに権限・マウント・ネットワークを定義する。** どちらのレーンにも属さない実行(ホスト直実行)は Makefile レベルで存在させない。

| レーン | 動かすもの | 実体 | 権限・マウント |
|---|---|---|---|
| toolchain(SPEC-009) | 検査・ビルド・生成・依存解決、リポジトリへ書き込む生成 CLI(`cmd/keygen`) | `.devcontainer/compose.tools.yml` の `tools` / `tools-offline` / `tools-db` | `/workspace` に repo を read/write bind、`cap_drop: ALL`、資格情報・docker.sock なし、既定オフライン |
| runtime(本 Spec) | 常駐プロセス(api / auth / web / auth-web / postgres)+ DB マイグレーション | ローカル `compose.yml`(`app/*/Dockerfile`)、本番 ECS Fargate | non-root / `read_only` / `cap_drop: ALL` / `no-new-privileges` / マウントは `pgdata` と署名鍵のみ / publish は `127.0.0.1` |

`cmd/keygen` を toolchain レーンに残すのは「リポジトリの `.secrets/` へ書く」ことが目的だからで、書き込み可能な bind mount を持つ面が正しい置き場になる。逆にマイグレーションは「DB に対して働き、リポジトリには書かない」ため runtime レーンが正しい。**判定基準は「常駐か否か」ではなく「リポジトリへ書くか / 本番と同一アーティファクトであるべきか」**。

### アーキテクチャ / データ / インターフェース

- **`run` の置き換え**: `app/{api,auth}/Makefile` の `run` は `$(MAKE) -C $(REPO_ROOT) migrate` → `docker compose -f compose.yml up --build <svc>`。`ifdef IN_TOOLBOX` 側には同名の禁止ターゲット(echo + `exit 1`)を置き、toolbox 内で叩いた場合に正規経路を案内する
- **`migrator` サービス**: `compose.yml` に `profiles: [migrate]` 付きで定義(`up` / `ps` / `build` から除外)。`build.context: .` + `dockerfile: app/migrator/Dockerfile`、`networks: [dbnet]`、`read_only: true`、`cap_drop: ALL`、`depends_on: postgres(service_healthy)`。呼び出しは `--profile migrate run --rm --build migrator -target api|auth [-command …]`。`--build` が新規マイグレーション反映のトリガ(SQL はイメージに焼き込まれる)、`-migrations-dir` は既定 `/migrations/<target>` に任せる
- **CI**: `cicd.yml` の 5 箇所の `tools go run ./cmd/migrator …` を同じ `--profile migrate run` 形に置換。`migrator-integration` の権限境界テスト(`go test -tags=integration`)は Go テストなので toolchain レーンに残す
- **サプライチェーンゲート**: `COPY package.json bun.lock bunfig.toml ./` → `RUN bun install --frozen-lockfile --ignore-scripts`。`app/auth-web/.dockerignore` を新設(欠落していた)、ルート `.dockerignore` に `.secrets/` を追加
- **digest 固定**: `FROM <image>:<tag>@sha256:…`。postgres だけは compose 経由なので `versions.env` の `POSTGRES_VERSION` + `POSTGRES_IMAGE_DIGEST`(2 つ同時更新が必須。digest が優先されるため)。更新手順は `docker pull` → `docker inspect --format '{{index .RepoDigests 0}}'` を各所コメントに記載

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| `run` を toolchain コンテナで動かす(`docker compose run tools go run ./cmd/api`) | toolbox はビルドツール一式 + repo の read/write bind mount を持つため、常駐プロセスに与える権限が runtime コンテナより広い。`docker compose run` はポートも publish しない |
| `run` を削除してルート `make up` に一本化 | 単一サービスだけ再起動したい日常操作を失う。名前を残して中身を差し替える方が既存の習慣を壊さない |
| ホスト実行を「非推奨」の文書化だけで済ませる | SPEC-009 が同じ形(「スコープ外の便宜」)で残した結果が今回の穴。実行時に落ちるガードがなければ再発する |
| runtime サービスを `internal: true` ネットワークに載せて egress を遮断 | 実測で publish ポートと両立しない(通常 bridge → HTTP 200 / internal → connection refused、同一コンテナでの対照実験)。ホストからの `localhost:8080` 系アクセスを失う代償が大きい |
| api / auth の host publish を捨てて internal 化(web nginx 経由のみに) | 8081 / 8082 直アクセスは README・CLAUDE.md・smoke 手順で使われている documented な導線。Go サービスの egress 遮断という便益に対して失うものが大きい |
| マイグレーションを `up` の init コンテナ(profile なし)として常時起動 | `up` / `ps` に exited コンテナが混じり、`make up` の healthy 判定と出力が読みにくくなる。profile 付き `run` なら「タスク」の意味論が明示される |
| `bun install` を廃してホスト/toolchain で作った `dist` をイメージに COPY | イメージビルドの hermeticity が落ち、ビルド前に別コマンドを要求する導線になる。ゲートを効かせれば build ステージ内で完結できる |
| digest を `versions.env` に集約し全 Dockerfile へ build arg 経由で渡す | `make push-images` の `docker buildx build` は compose を経由せず、`--build-arg` を全呼び出しに配線する必要がある。`FROM` の直上にコメント付きで書く方が参照点が近い(postgres だけは compose 経由なので versions.env に置く) |

## 5. 実装計画

- [x] T1: `app/{api,auth}/Makefile` の `run` を runtime コンテナ経路へ置換 + `IN_TOOLBOX` 禁止ガード
- [x] T2: web / auth-web の Dockerfile build ステージに `bunfig.toml` COPY + `--ignore-scripts` を追加、`app/auth-web/.dockerignore` 新設、`app/web/.dockerignore` に理由コメント、ルート `.dockerignore` に `.secrets/`
- [x] T3: `compose.yml` に profile 付き `migrator` サービスを追加(`dbnet` / `read_only` / `cap_drop: ALL`)
- [x] T4: ルート `Makefile` の `migrate` / `migrate-test` を `migrator` サービス経路へ、`app/migrator/Makefile` の `run` も同様に(`run-native` 削除 + 禁止ガード)
- [x] T5: `cicd.yml` の migrator 実行 5 箇所を同経路へ置換
- [x] T6: ベースイメージ digest 固定(5 Dockerfile + `versions.env` の `POSTGRES_IMAGE_DIGEST` + 全 Makefile の export)
- [x] T7: ドキュメント同期 — CLAUDE.md に「実行境界」節、`.claude/rules/{api,auth,web,db}.md`、`app/{api,auth}/README.md`、`versions.env` ヘッダ、`compose.yml` の egress 実測コメント
- [x] T8: ローカル検証(空ボリュームからの postgres 初期化 → migrator runtime イメージで 4 DB 作成 → 5 サービス healthy → 4 エンドポイント + nginx proxy 疎通 → `docker inspect` で権限・マウント確認 → 禁止ガード 3 本)
- [ ] T9: CI 検証(push 時に `cicd.yml` が緑であること。特に api / auth の migration health check と `migrator-integration`)
- [ ] T10: iac 側の `make iac-plan ENV=dev` 差分確認(ISSUE-056 のレーン。apply はユーザー判断)

## 6. 経緯(時系列・追記のみ)

### 2026-07-25

- 初版作成。きっかけ: 「runtime 部分のみコンテナで実行できる構成にし、最小権限・最小ボリュームマウント、Shai-Hulud 対策を行い、ホストでの runtime 実行禁止を CLAUDE.md に明記する」というユーザー要求。並行して ISSUE-056(runtime コンテナの最小権限ハードニング)が別レーンで実装中だったため、本 Spec は「実行境界の定義 + マイグレーションのレーン移動 + ビルド面のサプライチェーンゲート + digest 固定」を担当し、compose / ECS の権限設定そのものは ISSUE-056 の成果を前提として参照する形にした。
- SPEC-009 との関係: SPEC-009 は toolchain レーンを定義し、runtime を明示的にスコープ外(「host-side convenience」)としていた。本 Spec はその除外を撤回する。マイグレーションについては SPEC-009 のネットワーク切り分け表が `tools`(+compose.yml)としていたのを、runtime レーン(`migrator` サービス)へ移す形で更新した。
- 実測により決定: runtime サービスの egress 遮断は不採用(§4)。Docker 29.2.1 で `internal: true` ネットワークのみに接続したコンテナは publish ポートへホストから到達できない(busybox httpd の対照実験: 通常 bridge → 200 / internal → connection refused、in-container の DNS/egress は期待どおり不通)。代わりに publish しない面(`tools-offline` / `tools-db` / `migrator`)だけ egress を落とす方針を `compose.yml` の `networks:` コメントに残した。
- ローカル検証結果(T8、すべて pass): 空ボリュームからの postgres 初回 init が `cap_drop: ALL` + 最小 `cap_add`(CHOWN / DAC_OVERRIDE / FOWNER / SETGID / SETUID)で healthy。`migrator` runtime イメージで `api` / `auth` / `api_test` / `auth_test` の 4 DB を作成・適用(`migrations_dir=/migrations/<target>` = 焼き込みパス、`-command status` も動作)。5 サービス healthy、`http://localhost:8080/`・`:8083/`・`:8081/health`・`:8082/.well-known/openid-configuration`・`:8080/auth/.well-known/openid-configuration` が 200、`:8080/api/tasks` が 401(認証ミドルウェア動作)。`docker inspect`: 全サービス `cap_drop=[ALL]` + `no-new-privileges`、api / auth = `read_only` + `nonroot`、web / auth-web = `read_only` + uid 101(`ps` でも nginx master が uid 101)、マウントは `pgdata` のみ。`IN_TOOLBOX=1` の `run` は api / auth / migrator の 3 つとも非ゼロ終了。
- 検証時の環境メモ: ホストの 5432 が別プロジェクトの Postgres に占有されていたため、smoke ではリポジトリ外のコピーに postgres の host publish のみ `15432` へ差し替えて実行した(compose ファイル自体は変更していない)。`make up-d` をそのまま使うにはホスト側 5432 の解放が必要。
