---
id: ISSUE-057
title: runtime イメージビルドがサプライチェーンゲートを迂回していた(bunfig.toml 未 COPY / lifecycle script 有効 / auth-web の .dockerignore 欠落 / build context に .secrets)+ runtime のホスト実行が許容されていた
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: high  # critical | high | medium | low
created: 2026-07-25
updated: 2026-07-25
specs: [SPEC-016]  # 関連Spec ID
---

# ISSUE-057: runtime イメージビルドのサプライチェーンゲート迂回 + runtime のホスト実行許容

**深刻度: high**(Shai-Hulud 型 npm ワームへの防御としてリポジトリに導入済みのゲート(社内ミラー固定 + `minimumReleaseAge` 21 日)が、**リポジトリ内で唯一 runtime イメージのビルド経路だけ効いていなかった**。加えて runtime のホスト実行が Makefile レベルで許容されており、SPEC-009 が閉じたはずのホスト権限面が常駐プロセスに対して開いていた。active exploit は確認されていないが、防御の穴が「効いていると誤認されうる」形で存在していた点が重い)

## 1. ユーザー価値への影響(なぜ対応するか)

> **このリポジトリの開発者と本番 ECS の運用者** の **「汚染された依存パッケージが混入・実行されない」「侵害時にホストと本番へ横展開されない」という防御** が **イメージビルド経路でゲートが無効化されていたこと、および runtime をホストで起動できたことにより確保できていない**。

- **影響を受けるユーザー**: このリポジトリの開発者(直接。イメージビルドは日常操作)、本番 ECS の運用者・セキュリティ担当(直接。同じイメージが本番へ push される)、web / auth-web の利用者(間接)
- **損なわれる価値**: サプライチェーン防御(公開直後の汚染バージョンを掴まない / lifecycle script を実行しない / 資格情報をビルド面に置かない)と、権限分離(runtime に開発者のホスト権限を与えない)
- **影響範囲・頻度**: 常時。`make up` / `up-d` / `build` / `push-images` はすべて該当ビルド経路を通る。ホスト実行は `make -C app/api run` / `make -C app/auth run` を使う開発者に常時
- **回避策**: なし(設定を直す以外にない)。機能は正常に動作していたため機能的な回避策は不要

## 2. 現象(何が起きているか)

### 期待する動作

- `bun install` が走るすべての経路(toolchain コンテナの `make install`、および web / auth-web の **Dockerfile build ステージ**)で、`app/{web,auth-web}/bunfig.toml` のゲート(社内ミラー `registry` 固定 + `minimumReleaseAge = 1814400` = 21 日)が適用される
- 依存の lifecycle script(postinstall 等)はイメージビルド中に実行されない
- build context にホストの依存ツリー(`node_modules`)・ビルド成果物(`dist`)・資格情報(`.env*` / `.secrets/`)が含まれない
- アプリの runtime はコンテナでのみ起動でき、ホストや toolchain コンテナでは起動できない
- runtime のベースイメージは immutable digest で固定されている

### 実際の動作

1. **[Major] build ステージでゲートが無効**: `app/web/Dockerfile` / `app/auth-web/Dockerfile` は `COPY package.json bun.lock ./` の直後に `bun install --frozen-lockfile` を実行しており、**`bunfig.toml` が context 内に存在しない状態でインストールしていた**。したがって社内ミラー固定と `minimumReleaseAge` ゲートはこのビルドに一切効かず、既定の公開レジストリで解決される(`bun.lock` の integrity は効くが、URL を持たないエントリの解決先とゲートは失われる)。リポジトリ内で唯一のゲート無しインストール経路。
2. **[Major] lifecycle script が有効**: 同じ `bun install` に `--ignore-scripts` がなく、Bun の既定 trusted 集合に含まれるパッケージの postinstall はビルド中に実行され得た。Shai-Hulud 型ワームの実行・資格情報流出はまさにこの経路。
3. **[Major] `app/auth-web/.dockerignore` が存在しない**: build context が `app/auth-web` であるため、ルート `.dockerignore` は効かない。結果、後段の `COPY . .` が**ホストの `node_modules` をイメージへ持ち込み、直前の `bun install --frozen-lockfile` の結果を上書き**していた(lockfile ピンの実質的な無効化)。`app/web` には `.dockerignore` があり、auth-web だけ穴が空いていた。
4. **[Minor] ルート `.dockerignore` に `.secrets/` がない**: `app/migrator/Dockerfile` の build context はリポジトリルートで、`.secrets/auth-signing-keys.json`(auth の RSA 秘密鍵、ISSUE-036)がある環境では**鍵ディレクトリごと Docker daemon へ転送**されていた(イメージへ COPY はされない)。
5. **[Major] runtime のホスト実行が許容**: `app/{api,auth}/Makefile` の `run` は `go run ./cmd/api` / `go run ./cmd/authz`(ホスト実行)で、SPEC-009 の「ホストで go / bun を直接実行しない」から明示的に除外されていた(「host-side convenience、out of this Phase's scope」)。ホストのユーザー権限・ファイルシステム・資格情報を常駐プロセスに与える経路。
6. **[Minor] マイグレーションが toolchain レーン**: `make migrate` / `migrate-test` / `make -C app/migrator run` は toolchain コンテナ内で `go run ./cmd/migrator` していた。runtime バイナリなのにビルドツール一式 + リポジトリ read/write bind mount を持つ面で実行され、かつ本番 ECS の init コンテナ(runtime イメージ)とアーティファクトが異なっていた。
7. **[Minor] ベースイメージが浮動タグ**: `oven/bun:1`(major 浮動。`versions.env` の `BUN_VERSION=1.3.14` とも乖離)、`nginxinc/nginx-unprivileged:alpine`、`golang:1.26-alpine`、`gcr.io/distroless/static:nonroot`、`postgres:17.5-alpine` はいずれも再 push されうるタグで、digest 固定がなかった。

### 再現手順

1. `app/web/Dockerfile` の build ステージを見る → `COPY package.json bun.lock ./` の次に `bun install --frozen-lockfile` があり、`bunfig.toml` が COPY されていないことを確認する(= そのビルドではゲート設定ファイルが存在しない)。
2. `ls app/auth-web/.dockerignore` → 存在しない。`docker build app/auth-web` の context にホストの `node_modules` が含まれ、`COPY . .` でイメージ内の依存ツリーを上書きすることを確認する。
3. `grep -n "secrets" .dockerignore` → ヒットなし(ルート context のビルドに `.secrets/` が含まれる)。
4. `grep -n "go run" app/api/Makefile app/auth/Makefile` → `run` ターゲットがホストで `go run` することを確認する。
5. `grep -n "^FROM" app/*/Dockerfile` → digest 固定がないことを確認する。

### 環境・条件

- 対象: `app/web` / `app/auth-web` の Dockerfile と `.dockerignore`、ルート `.dockerignore`、`app/{api,auth,migrator}/Makefile`、ルート `Makefile`、`compose.yml`、`.github/workflows/cicd.yml`、`.devcontainer/versions.env`
- 関連: SPEC-009(toolchain コンテナ化・ホスト直接実行の排除)、ISSUE-020(bunfig のサプライチェーンゲート導入)、ISSUE-056(runtime コンテナの最小権限。並行レーンで実施)、ISSUE-036(署名鍵の永続化)

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/{web,auth-web}/bunfig.toml` には `registry = "https://npm.flatt.tech/"` と `minimumReleaseAge = 1814400`(21 日)が設定されている(ISSUE-020 / `.claude/rules/web.md`)。一方、両 Dockerfile の build ステージは `bunfig.toml` を COPY せずに `bun install` していた。
- 事実: `app/web/.dockerignore` は存在するが `app/auth-web/.dockerignore` は存在しなかった。Docker の `.dockerignore` は **context のルート**にあるものしか効かないため、ルートの `.dockerignore` は `app/auth-web` context のビルドには無関係。
- 事実: ルート `.dockerignore` は `.env` / `.env.*` / `**/node_modules/` / `.terraform` などを除外していたが `.secrets/` は対象外だった。
- 事実: `app/{api,auth}/Makefile` の `run` は SPEC-009 plan §手順 B1/B2 で「`docker compose run` はポートを publish しないため、ホスト側の便宜として残す(このフェーズのスコープ外)」と明記されたうえでホスト実行のまま残されていた。
- 事実: 実測(2026-07-25)で `--ignore-scripts` + `bunfig.toml` COPY を入れても両 stack のビルドは通る(245 パッケージ install → `tsc --noEmit && vite build` 成功)。ネイティブ依存(Tailwind oxide / esbuild)は optionalDependencies のプリビルドバイナリで供給されるため lifecycle script を必要としない。
- 仮説ではなく設計上の帰結: SPEC-009 は「ホストの言語ツールチェーン実行」を対象に据えたため、**イメージビルド内部の依存インストール**と**runtime の起動**という 2 つの面が視野の外に残った。ゲートは「開発者が叩く `bun install`」にだけ効く形で導入され(ISSUE-020)、Dockerfile 側に同じ契約が適用されているかを検査する仕組みがなかった。

### 根本原因

「どこで何が実行され、その面にどの防御が適用されるか」を**面(レーン)単位で定義した契約が存在しなかった**。SPEC-009 は toolchain 面のみを定義し、runtime 面(常駐プロセス・イメージビルド内の依存インストール・マイグレーション実行)は暗黙のまま残った。その結果、(a) ゲートが 1 経路だけ抜け、(b) ホスト実行が「便宜」として温存され、(c) 同一バイナリが目的別に 3 通りの権限で実行される状態になった。

## 4. 対応(どう解決するか)

### 対応方針

SPEC-016 として**実行境界(2 レーン)を明文化**し、runtime 面の契約(最小権限・最小マウント・サプライチェーンゲート・digest 固定)を定義したうえで、本 Issue の 7 点をその契約に合わせて修正する。ISSUE-056(compose / ECS / nginx の権限設定)とはレーンが分かれるため並行実施し、本 Issue は「実行経路とビルド面」を担当する。

### 実施内容

- [x] [Major] web / auth-web の Dockerfile build ステージで `bunfig.toml` を `bun install` より前に COPY(`COPY package.json bun.lock bunfig.toml ./`)
- [x] [Major] 同 build ステージを `bun install --frozen-lockfile --ignore-scripts` に変更
- [x] [Major] `app/auth-web/.dockerignore` を新設(`node_modules` / `dist` / `.env*` ほか。理由をコメントで明記)、`app/web/.dockerignore` に同趣旨のヘッダコメント
- [x] [Minor] ルート `.dockerignore` に `.secrets/` を追加(理由コメント付き)
- [x] [Major] `app/{api,auth}/Makefile` の `run` を compose の該当サービス(runtime イメージ)起動へ置換し、`IN_TOOLBOX=1` では禁止エラーで落とす
- [x] [Minor] マイグレーションを runtime レーンへ: `compose.yml` に profile 付き `migrator` サービスを追加し、ルート `make migrate` / `migrate-test`、`app/migrator` の `make run`、CI の 5 箇所をその経路へ統一(`run-native` は削除 + 禁止ガード)
- [x] [Minor] ベースイメージを tag + immutable digest で固定(api / auth / migrator / web / auth-web の build+runtime ステージ、compose の postgres は `versions.env` の `POSTGRES_VERSION` + `POSTGRES_IMAGE_DIGEST`)
- [x] ドキュメント同期: CLAUDE.md「実行境界: runtime はコンテナのみ」節、`.claude/rules/{api,auth,web,db}.md`、`app/{api,auth}/README.md`、`versions.env` ヘッダ、`compose.yml` の egress 実測コメント
- [x] ローカル検証(空ボリューム postgres 初期化 → migrator runtime イメージで 4 DB → 5 サービス healthy → エンドポイント疎通 → `docker inspect` の権限・マウント確認 → 禁止ガード 3 本)
- [ ] CI 検証(push 時に `cicd.yml` が緑。特に api / auth の migration health check と `migrator-integration`)

### 再発防止

- **契約を面単位で持つ**: SPEC-016 が「toolchain / runtime の 2 レーンと各レーンの権限・マウント・ゲート」を唯一の正として定義した。新しい実行経路を足すときは、どちらのレーンに属するか(= どの防御が適用されるか)を先に決める
- **ゲートは「`bun install` が走る全経路」で確認する**: `.claude/rules/web.md` に「runtime イメージのビルドにも同じゲートを効かせる」節を追加し、`bunfig.toml` の COPY 位置・`--ignore-scripts`・`.dockerignore` の 3 点をレビュー観点として明記した
- **`.dockerignore` は context ごとに必要**: ルートの `.dockerignore` が per-stack context に効かないことを両ファイルのヘッダコメントに残した(「掃除」で消されないよう、サイズ最適化ではなく防御であることを明記)
- **review-security の観点追加**: runtime 権限(ISSUE-056)に加え、「イメージビルド面のサプライチェーンゲート」「実行レーンの逸脱(ホスト / toolchain での runtime 起動)」「ベースイメージの digest 固定」をチェック観点にする

## 5. 経緯(時系列・追記のみ)

### 2026-07-25

- 起票。発見経緯: 「runtime 部分のみコンテナで実行できる構成にし、最小権限・最小マウント、Shai-Hulud 対策を行う」というユーザー要求に対する調査中に、ISSUE-056(並行レーンで実装中)がカバーしていない 7 点を確認した。特に「bunfig.toml が build ステージに存在しない」ことは、ゲートが導入済み(ISSUE-020)であるがゆえに**効いていると誤認されうる**穴であり、Major と判断した。
- 事実確認: 各項目を現物で確認(`app/{web,auth-web}/Dockerfile` の COPY 順、`app/auth-web/.dockerignore` の不在、ルート `.dockerignore` の `.secrets/` 不在、`app/{api,auth}/Makefile` の `run`、`FROM` の浮動タグ)。
- 修正・検証: SPEC-016 の T1〜T8 として実施。build ステージの修正は専用タグ(`cc-orch-verify/*-build:shai`、検証後削除)で `--target build` ビルドし、245 パッケージ install + `tsc && vite build` 成功を確認。フルスタックは空 `pgdata` から `postgres`(cap_drop ALL + 最小 cap_add)→ `migrator` runtime イメージで `api` / `auth` / `api_test` / `auth_test` 作成 → 5 サービス healthy → `localhost:8080/`・`8083/`・`8081/health`・`8082/.well-known/openid-configuration`・`8080/auth/.well-known/openid-configuration` が 200、`8080/api/tasks` が 401 を確認。`docker inspect` で全サービス `cap_drop=[ALL]` + `no-new-privileges`、api / auth = `read_only` + `nonroot`、web / auth-web = `read_only` + uid 101、マウントは `pgdata` のみを確認。`IN_TOOLBOX=1` の `run` は api / auth / migrator すべて非ゼロ終了。
- status を `resolved` にした根拠は上記ローカル検証。CI(`cicd.yml` の migrator 経路変更を含む)は push 時に確認するため `closed` にはせず CI 緑を待つ。
- 検証環境メモ: ホストの 5432 が別プロジェクトの Postgres に占有されていたため、smoke ではリポジトリ外のコピーで postgres の host publish のみ `15432` に差し替えて実行した(`compose.yml` 自体は未変更)。`make up-d` をそのまま使う場合はホスト側 5432 の解放が必要。
