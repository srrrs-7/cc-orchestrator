# SPEC-009 実装計画: 開発ツールチェーンのコンテナ隔離(ホスト runtime 不要・サプライチェーン対策)

- 起点: `docs/specs/20260710-009-containerized-toolchain-no-host-runtime.md`(status: **approved**)
- 対象範囲: リポジトリ全体のツーリング層(`versions.env` / `docker/toolchain/` / `compose.tools.yml` / `.devcontainer/` / 各 `Makefile` / web 実行入口 / `.github/workflows/*`)。**`app/**` のアプリコードと `app/*/Dockerfile`(runtime イメージ)は変更しない**(SPEC §3 スコープ外)
- 成果物: ホストの前提を **Docker のみ** にし、`make check` / `bun run <script>` / `make openapi` / `make sqlc` / `make migrate` / iac の fmt・validate 等が単一 pinned toolchain イメージ内で走る。通常実行(check/build/test/generate)は `--network none`。CI・devcontainer も同一イメージ。コマンド契約(`.claude/rules` の表・CLAUDE.md 早見表・agent 呼び出し)は不変

---

## ⚠️ 冒頭: 着手前に必要なユーザー / admin 判断

以下は planner が調査で確定できず、着手前に決めが要る点。**1 は必須の決定**(versions.env が単一値のため)。

1. **BUN バージョンの一本化(必須決定)**: 現状 pin が割れている — `cicd.yml`=`1.2.21` / `contract-drift.yml`=`1.3.14`(「意図的に cicd と違える」旨のコメント付き)/ `app/web/Dockerfile`=`oven/bun:1`(浮動 major)。`versions.env` は **単一の `BUN_VERSION`** に集約するため、どれに合わせるかを決める必要がある。**planner 推奨は `1.3.14`**(contract-drift が既に前進させており、単一 polyglot イメージなら 2 系統を持てない)。※ `app/web/Dockerfile` の runtime イメージ pin(`oven/bun:1`)は SPEC スコープ外なので今回は触らないが、toolchain と乖離する点はリスクに記載。
2. **web の host 実行入口の方式**: `bun run <script>` を「ホストに bun 無し」で維持するには、ホスト PATH 上に `bun` シム(POSIX シェル。docker へ委譲)を置く必要がある(§方針 5)。**PATH への追加という 1 度きりのセットアップ**が発生する(CLAUDE.md に手順を明記)。この方式で進めてよいか(代替は web を `make` 経由にする案だが SPEC が `bun run <script>` 維持を明記しているため不採用)。
3. **CI の toolchain イメージ配布方式**: GHCR に `toolchain` を build & push して各 job が `container:` 参照する方式を推奨(§方針 7)。プライベート GHCR 利用 / `packages: write` 権限付与の可否を確認。不可なら「各 job で毎回イメージを build(キャッシュ付き)」にフォールバック(遅いが権限不要)。

いずれも設計の骨格は変えず、確定後に impl-ci が反映できる。**T2 以降の着手は 1 の決定を待つ**(他の設計・準備は先行可)。

---

## 方針

### 採用アプローチ(全体像)

脅威モデルは「危険は依存 install 時 + ホスト秘密 + egress」。対策は **ツール実行を『秘密を持たない使い捨てコンテナ』に移設** + **通常実行を `--network none`** + **install/整合性ゲート(lockfile・sha512・`minimumReleaseAge`・go.sum・pin)を第一線として維持**。呼び出し契約は透過ラッパーで不変にし、ローカル / devcontainer / CI が **同一の単一 polyglot イメージ** を通る。

1. **単一 polyglot イメージ**(`docker/toolchain/Dockerfile`): go / bun / golangci-lint / terraform / tflint / trivy + `go run` 系ツール(sqlc / goose / swag / goimports)を 1 つに焼く(スタック別 3 イメージ案は SPEC の代替案検討で不採用済み)。
2. **バージョン単一ソース**(`versions.env`): `cicd.yml` の env pin を昇格。Dockerfile(build ARG)と CI(env)が同じファイルを読む。
3. **compose.tools.yml で 2 サービス**(`tools` = network 有効 / `tools-offline` = `network_mode: none`)を YAML アンカーで共通化し、**2 フェーズ network を「サービス選択」で表現**する(`docker compose run --network` フラグの可搬性に依存しない)。
4. **Makefile 透過ラッパー**(`IN_TOOLBOX` ガード + `*-native` 実体): ホスト呼び出しを `docker compose -f compose.tools.yml run --rm <tools|tools-offline> $(MAKE) <target>-native IN_TOOLBOX=1` へ委譲。network の要否でサービスを出し分ける(下表)。
5. **web は `bin/bun` シム**で `bun run <script>` を透過化(package.json は不変)。
6. **CI を同一イメージへ寄せる**: `setup-go` / `setup-bun` / golangci curl / setup-terraform 等を撤去し `container:` job 化。postgres service は container job と同一 job network で `DB_HOST=postgres` に。
7. **AWS 系は build=コンテナ / `aws` CLI=host に分割**(認証情報をコンテナに入れない)。

### versions.env の設計(単一ソース)

`versions.env`(リポジトリ直下、`KEY=value` 形式・引用符/空白/`export` なし)。この形式は (a) シェル `set -a; . ./versions.env; set +a`、(b) GitHub Actions `cat versions.env >> "$GITHUB_ENV"`、(c) `docker compose --env-file versions.env` の変数展開、(d) Makefile の `include versions.env` すべてで読める。

```
# CI env から昇格(SPEC R1)
GO_VERSION=1.24
BUN_VERSION=1.3.14          # ← 冒頭判断 1 で確定
GOLANGCI_LINT_VERSION=2.12.2
TERRAFORM_VERSION=1.10.5
TFLINT_VERSION=v0.55.1
TRIVY_VERSION=v0.58.1
POSTGRES_VERSION=17.5-alpine
# go run 系ツール(現状 Makefile が SoT。オフライン化のためイメージに焼くので集約)
SQLC_VERSION=v1.31.1
GOOSE_VERSION=v3.24.1
SWAG_VERSION=v2.0.0-rc5
GOIMPORTS_VERSION=<pin>     # ← 現状 @latest。offline/再現性のため要 pin(下記リスク)
```

- **消費経路**: Dockerfile は `ARG` で受け(compose の `build.args: ${...}` 経由で `--env-file versions.env` 展開)、各 Makefile は先頭で `include $(REPO_ROOT)/versions.env` してツール版を参照、CI は `versions.env` を `$GITHUB_ENV` に流し込む。
- **`go run` 系ツール版の SoT を versions.env に移す**理由: オフライン実行(`--network none`)のためこれらをイメージに焼く必要があり、Dockerfile と Makefile の両方が同じ版を要する。二重管理を避けるため versions.env へ集約する(現状 Makefile の `SQLC`/`GOOSE`/`SWAG`/`GOIMPORTS` を `include` 参照へ置換)。**退けた代替**: Makefile を SoT に残し Dockerfile へ sync コメントで複製(既存 `GOOSE_VERSION` の運用に倣う)。単一ソース原則(OpenAPI/sqlc と同思想)を優先し集約案を採る。
- **`goimports@latest` の pin**: 現状 3 つの Go Makefile が `goimports@latest`。`@latest` は解決に network を要し再現性も崩すため、`GOIMPORTS_VERSION` として pin する(impl 時に resolvable な最新 stable を確定)。

### docker/toolchain/Dockerfile の設計

- ベースは Go を入れやすい `golang:${GO_VERSION}-bookworm`(bun/terraform/tflint/trivy を追加インストール)を第一候補。musl/alpine より glibc の方が各種 CLI バイナリの互換が安定。**ビルドコンテキストは `docker/toolchain/` のみ**(リポジトリ source は焼かない。source は実行時に bind-mount)→ 巨大コンテキスト回避のため `.dockerignore` 不要。
- **ツール導入(現行 CI の install 方法を踏襲)**:
  - go: ベースイメージ同梱
  - bun: 公式 install script を `BUN_VERSION` 指定で(`oven-sh/setup-bun` 相当)
  - golangci-lint: 現行 CI と同じ curl install(`GOLANGCI_LINT_VERSION`)
  - terraform / tflint / trivy: 各公式配布物を `*_VERSION` 指定で(`setup-terraform`/`setup-tflint`/`setup-trivy` 相当)
  - `go run` 系(sqlc/goose/swag/goimports): `go install pkg@${VERSION}` で **モジュールキャッシュを温め、バイナリも生成**。Makefile は `go run pkg@ver` を使い続けるが、モジュールが GOMODCACHE に在れば `GOPROXY=off` でも offline で走る(バイナリ生成は副次的な温め目的)。
- **非 root**: 固定 UID(例 1000)の `tools` ユーザーを作成。ただし実行は host UID で行う(下記)。キャッシュディレクトリ(`GOMODCACHE`/`GOCACHE`/`GOPATH`/`BUN_INSTALL_CACHE`/`HOME` 相当・`TF_PLUGIN_CACHE_DIR`)を named volume マウント点に定め、任意 UID が書けるよう `0777`(または適切な group 権限)にしておく。
- **イメージ env**: `GOFLAGS`/`GOPROXY` は既定を素直に保ち、offline 実行時のみ `tools-offline` サービス側で `GOPROXY=off` を効かせる(誤 fetch を明示的に fail させ「キャッシュ未温め」を早期検知)。
- **ビルド/参照**: ローカルは `docker compose --env-file versions.env -f compose.tools.yml build`(ラッパーが必要時に自動 build も可)。CI は GHCR に build & push(冒頭判断 3)。

### compose.tools.yml の設計(2 フェーズ network)

```yaml
# 擬似(詳細は impl-ci)
x-tools-base: &tools-base
  build: { context: ./docker/toolchain, args: { GO_VERSION: ${GO_VERSION}, BUN_VERSION: ${BUN_VERSION}, ... } }
  image: cc-orchestrator/toolchain:local
  user: "${TOOLBOX_UID:-1000}:${TOOLBOX_GID:-1000}"   # ラッパーが id -u/-g を注入
  working_dir: /workspace
  volumes:
    - .:/workspace:delegated            # macOS 性能(VirtioFS 既定下では no-op ヒント)
    - gomodcache:/cache/go/mod
    - gobuild:/cache/go/build
    - buncache:/cache/bun
    - tfplugincache:/cache/tf-plugins
  environment: { GOMODCACHE: /cache/go/mod, GOCACHE: /cache/go/build, BUN_INSTALL_CACHE_DIR: /cache/bun, TF_PLUGIN_CACHE_DIR: /cache/tf-plugins, ... }
  security_opt: ["no-new-privileges:true"]
  cap_drop: ["ALL"]
  # ホスト秘密(~/.aws ~/.ssh ~/.npmrc 等)・docker socket は一切マウントしない

services:
  tools:                      # network 有効(install / migrate / integration 用)
    <<: *tools-base
  tools-offline:              # 通常実行用
    <<: *tools-base
    network_mode: none
volumes: { gomodcache: , gobuild: , buncache: , tfplugincache: }
```

- **2 フェーズ network を「サービス選択」で表現**: `--network none` フラグの `docker compose run` 対応は環境差があるため、**`network_mode: none` を焼いた別サービス `tools-offline`** を用意し、ラッパーが用途でサービス名を選ぶ(deterministic)。
- **統合テスト / migrate の例外(network + postgres が必要)**: これらは既存 `compose.yml` の `postgres` に到達する必要があるため **network-none にしない**。実行時は 2 ファイルを重ねる `docker compose -f compose.yml -f compose.tools.yml run --rm tools ...` とし、`tools` を merged プロジェクトの既定 network に載せて `postgres` をサービス名で解決(`DB_HOST=postgres`)。これは「install と並ぶ network 要フェーズ」として明示的に例外扱いする(下表)。
- **ハードニング(任意)**: `read_only: true` + `/tmp` を tmpfs 化は将来強化候補として記載(今回は必須にしない)。

### network 要否の切り分け(実装契約)

| 入口(コマンド) | フェーズ | サービス | network | 備考 |
|---|---|---|---|---|
| `bun install` / `add` / `remove` / `update` | deps | `tools` | 有 | lockfile + `minimumReleaseAge` ゲートが第一線 |
| go モジュール warm(`go mod download`) | deps | `tools` | 有 | GOMODCACHE volume を温める |
| `terraform init`(provider 取得) / `tflint --init` | deps | `tools` | 有 | TF_PLUGIN_CACHE_DIR / tflint plugin を温める |
| `make fmt` / `fmt-check` / `lint` / `vet` / `build` / `test` / `test-race` / `check` | exec | `tools-offline` | none | api / auth / migrator |
| `bun run format` / `format:check` / `lint` / `typecheck` / `test` / `build` / `generate` | exec | `tools-offline` | none | web(`generate` はローカル openapi.yaml 読取のみ) |
| `make openapi` / `make sqlc` / `make migrate-create` | exec | `tools-offline` | none | 生成・scaffold(DB 接続なし) |
| iac `make fmt` / `fmt-check` / `validate` / `lint` / `security` / `init-local` | exec | `tools-offline` | none | provider/plugin 温め済み前提 |
| iac `make plan` | exec | `tools` | 有 | remote state / provider 参照のため例外 |
| root `make migrate`(app/migrator 実行) | exec+DB | `tools`(+`compose.yml`) | 有 | postgres 到達が必要な例外 |
| `make test-integration`(api/auth/migrator) | exec+DB | `tools`(+`compose.yml`) | 有 | 同上。統合テストは network-none にしない |
| `make deploy-web` の build 部 / `push-images` の image build | exec | `tools-offline` / buildx | none/— | build のみコンテナ。§AWS 分割参照 |
| `aws s3 sync` / `cloudfront` / `ecr login`(deploy) | host | — | host | `aws` CLI は host 実行(認証情報をコンテナに渡さない) |

### Makefile 透過ラッパーの形(全 stack 共通パターン)

```makefile
# 各 Makefile 先頭(擬似)
REPO_ROOT := $(shell git rev-parse --show-toplevel)
include $(REPO_ROOT)/versions.env
COMPOSE  := docker compose --env-file $(REPO_ROOT)/versions.env -f $(REPO_ROOT)/compose.tools.yml
RUNENV   := -e IN_TOOLBOX=1 -e TOOLBOX_UID=$(shell id -u) -e TOOLBOX_GID=$(shell id -g)
OFFLINE  := $(COMPOSE) run --rm $(RUNENV) tools-offline
ONLINE   := $(COMPOSE) run --rm $(RUNENV) tools      # + -f compose.yml を重ねる版は migrate/integration 用

ifdef IN_TOOLBOX      # ← コンテナ内: 実体を実行(多重ラップ防止)
check-native: fmt-check-native lint-native vet-native build-native test-native
fmt-native: ; gofmt -w . && $(GOIMPORTS) -w .
# ... 既存レシピを *-native にリネームして温存 ...
else                  # ← ホスト: コンテナへ委譲
check: ; $(OFFLINE) $(MAKE) check-native
test:  ; $(OFFLINE) $(MAKE) test-native
test-integration: ; $(DB_ONLINE) $(MAKE) test-integration-native   # compose.yml を重ねる
# ... 各 public target を対応するサービスへ委譲 ...
endif
```

- **コマンド契約は不変**: 外部から見える target 名(`check`/`test`/`fmt`/`lint`/`vet`/`build`/`sqlc`/`openapi`/`migrate-create`/`test-integration`、iac の `fmt`/`validate`/`lint`/`security`/`plan`/`init-local` 等)は現状のまま。実体は `*-native` に退避。
- **`IN_TOOLBOX` マーカー**: コンテナ内では set 済み → `*-native` 側(実体)が走り、再委譲しない。
- **既存の pin コメント / DB_* 既定 / `?=` は温存**(env override 契約を壊さない)。`SQLC`/`GOOSE`/`SWAG`/`GOIMPORTS` の版参照だけ `versions.env` の変数へ差し替える。

### web の透過ラッパー(bin/bun シム)

`bin/bun`(POSIX sh・実行ビット付き):

```sh
#!/bin/sh
# ホストに bun 無しで `bun run <script>` 等を維持する透過シム。
# IN_TOOLBOX 下(コンテナ内)は本物の bun を exec(多重ラップ防止)。
[ -n "$IN_TOOLBOX" ] && exec /usr/local/bin/bun "$@"
# サブコマンドで network 要否を判定(install/add/remove/update → tools、それ以外 → tools-offline)
case "$1" in install|add|remove|update|pm) SVC=tools ;; *) SVC=tools-offline ;; esac
exec docker compose --env-file "$ROOT/versions.env" -f "$ROOT/compose.tools.yml" \
  run --rm -e IN_TOOLBOX=1 -e TOOLBOX_UID="$(id -u)" -e TOOLBOX_GID="$(id -g)" \
  -w /workspace/app/web "$SVC" bun "$@"
```

- **package.json は不変**(scripts はコンテナ内の本物 bun がそのまま実行)。
- **セットアップ**: `<repo>/bin` を PATH 前方に追加(CLAUDE.md「ローカル実行」に明記)。checker/tester・admin のシェルもこの PATH 前提で疎通確認する(冒頭判断 2)。
- **退けた代替**: web に Makefile を新設して `make` 経由にする案 → SPEC R4 が `bun run <script>` 維持を明記しているため不採用。

### CI の container 化(R6)

- 新 job `toolchain`: `versions.env` を読み、`docker/toolchain` を build して **GHCR に push**(タグは `versions.env` 内容のハッシュ)。`outputs.image` に ref を出す。
- 各 job(`web` / `api` / `auth` / `migrator` / `api-integration` / `auth-integration` / `iac`)を `container: { image: ${{ needs.toolchain.outputs.image }} }` 化。`setup-go` / `setup-bun` / golangci curl / `setup-terraform` / `setup-tflint` / `setup-trivy` を **撤去**(すべてイメージ同梱)。
- **postgres service との組合せ**: container job では service container が同一 job network に載り、サービス名 `postgres` で到達可能。統合系 job の `DB_HOST` を `127.0.0.1` → `postgres` に変更(container job の標準)。`services.postgres.image` は `env` context を参照できない制約が現状あるため、`POSTGRES_VERSION` は job step 側で参照しつつ image はリテラル維持(既存コメントの方針踏襲)。
- **contract-drift.yml / sqlc-drift.yml も同一イメージ**へ。特に contract-drift は go+bun 双方を要するが、単一 polyglot イメージがそのまま解になる(=**BUN pin 二系統問題も自然解消**)。
- **キャッシュ**: `actions/cache` で GOMODCACHE / GOCACHE / bun cache を go.sum / bun.lock キーで維持(container job 内のキャッシュ dir を指定)。
- **network-none は CI では必須にしない**(runner 自体が ephemeral・隔離済み)。CI の価値は「同一 pinned イメージ = ローカルと一致」。将来 exec step を `--network none` 化する余地は残す。
- `dependabot.yml` に `docker/toolchain/Dockerfile` の base image 追跡(docker ecosystem)を追加。`copilot-instructions.md` にコマンド実体がコンテナ内である旨を追記。

### AWS 分割(R7)

- root `Makefile` `deploy-web`: `bun run build`(= `bin/bun` 経由で `tools-offline` 内 build)→ 生成 `dist/` を **host の `aws s3 sync` / `cloudfront`** で配布(`aws` はコンテナに入れない)。
- root `Makefile` `push-images`: image build(`docker buildx`, host の docker)→ `aws ecr get-login` / push は host。**AWS 認証情報は host のみ**、toolchain コンテナへは渡らない(元々 `app/*/Dockerfile` の build であり toolchain とは別。ここは「build=docker(host) / 認証=host aws」の整理を明文化)。
- `.github/workflows/deploy.yml`: web build を toolchain イメージ(container job)で行い、`aws-actions/configure-aws-credentials`(OIDC)は別 step/job(host runner)に保つ。認証情報が toolchain コンテナに露出しない配置にする。

### devcontainer(R5)

`.devcontainer/devcontainer.json`: `dockerComposeFile: ["../compose.tools.yml"]` / `service: tools` / `workspaceFolder: /workspace` / `remoteUser`(非 root)/ ホスト秘密マウントなし。対話開発向けの任意上乗せ(実行の正典は compose run。SPEC §4)。

---

## 変更ファイル

### 新規

| パス | 内容 | 主担当 |
|---|---|---|
| `versions.env` | ツール版の単一ソース | impl-ci |
| `docker/toolchain/Dockerfile` | 単一 polyglot イメージ | impl-ci |
| `docker/toolchain/README.md` | イメージの目的・build・版の出所 | impl-ci |
| `compose.tools.yml` | `tools` / `tools-offline` サービス + cache volumes | impl-ci |
| `.devcontainer/devcontainer.json` | tools サービス参照の devcontainer | impl-ci |
| `bin/bun` | web の host 実行シム | impl-web |

### 変更

| パス | 内容 | 主担当 |
|---|---|---|
| `Makefile`(root) | `migrate` を container(+compose.yml)経由へ / `deploy-web`・`push-images` を build=コンテナ・aws=host に分割 / 必要なら `toolbox-build`・`deps` 補助 target | impl-ci(migrate 意味論は impl-db がレビュー) |
| `app/api/Makefile` | 透過ラッパー(`IN_TOOLBOX`/`*-native`)/ ツール版を versions.env 参照へ / goimports pin | impl-api |
| `app/auth/Makefile` | 同上 | impl-auth |
| `app/migrator/Makefile` | 同上(統合テストは network 要サービス) | impl-db |
| `app/iac/Makefile` | 透過ラッパー / provider・plugin の init(deps)フェーズ分離 / plan は network 要 | impl-iac |
| `.github/workflows/cicd.yml` | `toolchain` job 追加・各 job の container 化・setup-*/curl 撤去・integration の DB_HOST 調整 | impl-ci |
| `.github/workflows/contract-drift.yml` | 同一イメージ化(go+bun) | impl-ci |
| `.github/workflows/sqlc-drift.yml` | 同一イメージ化 | impl-ci |
| `.github/workflows/deploy.yml` | build=コンテナ / aws=host の分割 | impl-ci |
| `.github/dependabot.yml` | `docker/toolchain` の base image 追跡追加 | impl-ci |
| `.github/copilot-instructions.md` | コマンド実体がコンテナ内である旨の注記 | impl-ci |
| `app/web/package.json` | **原則不変**(scripts はコンテナ内 bun がそのまま実行)。必要が判明した場合のみ最小調整 | impl-web |
| `.claude/rules/{web,api,db,iac,testing}.md` | コマンド表はコマンド名不変のまま「実体はコンテナ内実行」注記追加 | admin |
| `CLAUDE.md` | 「ローカル実行」「コマンド早見表」にコンテナ実行前提 + `bin` PATH セットアップを注記 | admin |

> **注**: root `Makefile` は 1 ファイルに複数の変更(migrate / deploy / push)が集中するため **単一オーナー(impl-ci)** に集約し、`migrate` の DB 意味論だけ impl-db がレビューする(親メッセージの「impl-db が migrate、impl-web が deploy 部」を、同一ファイルの同時編集による衝突回避のため単一オーナーへ寄せる。役割の趣旨は維持)。

---

## 手順(担当 agent・順序・並列可否)

依存の基本線: **(A) 基盤(イメージ+compose)→ (B) 各 stack ラッパー(並列)→ (C) CI/AWS + docs(並列)→ (D) 検証 → (E) レビュー**。

### フェーズ A — 基盤(impl-ci、最優先・単独)

- A1. `versions.env` 作成(冒頭判断 1 の BUN 版・`GOIMPORTS_VERSION` の pin を確定して記載)。
- A2. `docker/toolchain/Dockerfile` + `README.md`: go/bun/golangci-lint/terraform/tflint/trivy + `go install` によるツール温めを実装。ローカル build が通ることを確認(impl-ci の作業範囲内)。
- A3. `compose.tools.yml`: `tools` / `tools-offline`(+ cache volumes・security_opt・cap_drop・user・秘密非マウント)。
- A4. `.devcontainer/devcontainer.json`(A3 に依存。A5 と並列可)。

> A は後続すべての前提。A1–A3 完了を Phase B のゲートにする。

### フェーズ B — 各 stack の透過ラッパー(A 完了後・**相互に並列**)

- B1. **impl-api**: `app/api/Makefile` にラッパー適用 + ツール版を versions.env 参照 + goimports pin。`make check`(→ tools-offline)/ `make openapi`・`make sqlc`(offline)/ `make test-integration`(tools+compose.yml)/ `make migrate-create`(offline)を委譲。
- B2. **impl-auth**: `app/auth/Makefile` を B1 と同形で。
- B3. **impl-db**: `app/migrator/Makefile` を同形で(`test-integration` は network 要サービス)。root `Makefile` の `migrate` 変更の**意味論レビュー**(実体編集は impl-ci)。
- B4. **impl-web**: `bin/bun` シム作成(install→tools / それ以外→tools-offline の出し分け・IN_TOOLBOX ガード)。`package.json` は原則不変であることを確認。
- B5. **impl-iac**: `app/iac/Makefile` にラッパー適用。`terraform init`/`tflint --init` を deps(network 要)フェーズに分離、`fmt/validate/lint/security/init-local` は offline、`plan` は network 要サービス。fmt はルート全体・validate 系は `envs/<env>` 基点の既存規約を維持。

### フェーズ C — CI・AWS・ドキュメント(B 完了後・**並列**)

- C1. **impl-ci**: `cicd.yml` に `toolchain` build&push job 追加 → 各 job を container 化・setup-*/curl 撤去・`versions.env` 参照・integration の `DB_HOST=postgres` 調整。
- C2. **impl-ci**: `contract-drift.yml` / `sqlc-drift.yml` を同一イメージへ(BUN 二系統解消)。
- C3. **impl-ci**: root `Makefile` の `deploy-web` / `push-images` と `deploy.yml` を build=コンテナ / aws=host に分割。`dependabot.yml` / `copilot-instructions.md` 更新。
- C4. **admin**: `.claude/rules/{web,api,db,iac,testing}.md` + `CLAUDE.md` にコンテナ実行前提の注記追加(コマンド名不変・`bin` PATH セットアップ手順)。C1–C3 と並列可。

### フェーズ D — 検証(C 完了後)

- D1. **tester**: 「ホストに go/bun/golangci-lint 等が無い」前提での疎通(§テスト戦略)。exec 系が `--network none`(=`tools-offline`)で完走、deps/migrate/integration が network 要サービスで成立することを確認。不足あれば該当 impl へ差し戻し。
- D2. **checker**: 全 stack の `make check` / `bun run *`(= コンテナ経由)と CI のローカル再現(可能な範囲)を確認。format/lint/type check が toolbox 内で green。

### フェーズ E — レビュー(D green 後・並列)

- E1. **review-security**: 秘密非マウント・docker socket 非マウント・`cap_drop: ALL` / `no-new-privileges` ・network-none の適用範囲・deps フェーズの network 最小化・AWS 認証情報がコンテナに渡らないことを検証。
- E2. **review-spec**: R1–R7 と非機能(多層防御・CI パリティ・macOS 性能・既存挙動不変)への準拠を確認。

指摘の Blocker/Major は該当 impl へ差し戻し、D→E を再実行。今回対応しない指摘は issue-creator が起票。

---

## テスト戦略

インフラ・ツーリング変更のため、ユニットテストではなく **「ホストに runtime 無し」の疑似環境での疎通 + `--network none` 完走 + CI green** を受け入れ条件にする(TDD ではなく実装と同時に検証手順を確立)。

- **ホスト runtime 非在の擬似**: 検証環境で go/bun/golangci-lint を実際にアンインストールできないため、**PATH を絞って擬似する**(例: `env -i PATH=/usr/bin:/bin HOME=$HOME <cmd>` で go/bun が見つからない状態を作り、`make check` / `<repo>/bin/bun run test` が **docker + make のみ**で成功することを示す)。追加で `command -v go bun golangci-lint` が絞った PATH 下で無いことをアサート。
- **`--network none` 完走**: exec 系(`make check` / `bun run test` / `make sqlc` / `make openapi` / iac `validate`)を `tools-offline`(=`network_mode: none`)で走らせ完走することを確認。さらに `GOPROXY=off` 下で go build/test が warm cache のみで通る=誤 fetch が無いことを確認(温め漏れの検知)。
- **network 例外の成立**: deps(`bun install` / `go mod download` / `terraform init` / `tflint --init`)・`make migrate`・`make test-integration`(api/auth/migrator)が network 要サービス(+`compose.yml` の postgres)で成立することを確認。統合テストが postgres に到達し既存と同結果になること。
- **生成物の同一性(既存挙動不変)**: `make openapi` / `make sqlc` / `bun run generate` の生成物が現行 commit と一致(drift 無し)。既存 `contract-drift` / `sqlc-drift` が同一イメージ下でも green。
- **CI パリティ**: PR を上げ、container 化後の `cicd.yml` / `contract-drift` / `sqlc-drift` が green。ローカルと同一イメージ tag で走ることを確認。
- **UID / 権限**: Linux(CI/一般開発機)で bind-mount へ生成したファイルが root 所有にならない(host UID で作られる)ことを確認。named volume への書き込みが任意 UID で可能なこと。
- **要件カバレッジ**(下表)で各 R が手順・テストのどこで担保されるかを追跡。

### 要件 → 手順 / テスト 対応表

| 要件 | 手順 | テスト |
|---|---|---|
| R1 単一 polyglot イメージ + versions.env | A1・A2 | D2(イメージ build)・生成物同一性 |
| R2 tools サービス(bind/cache/user/秘密非マウント/cap_drop 等) | A3 | E1・UID/権限確認 |
| R3 2 フェーズ network(deps=有 / exec=none) | A3・B1–B5(表の切り分け) | `--network none` 完走・network 例外の成立 |
| R4 コマンド契約不変(透過ラッパー・IN_TOOLBOX) | B1–B5・C4 | ホスト runtime 非在疑似・D2 |
| R5 devcontainer | A4 | 手動起動確認(devcontainer で tools 参照) |
| R6 CI 同一イメージ化 | C1・C2 | CI パリティ(green) |
| R7 AWS build=コンテナ/aws=host 分割 | C3 | E1(認証情報がコンテナに渡らない) |
| NFR 多層防御 | 既存ゲート維持 + A3 隔離 | E1 |
| NFR CI パリティ / macOS 性能 / 既存挙動不変 | A1・A3(`:delegated`+volume) | CI パリティ・生成物同一性 |

---

## リスク / 未確定事項

- **BUN pin 二系統(要決定・冒頭 1)**: `1.2.21` / `1.3.14` / `oven/bun:1` の不一致。単一 `BUN_VERSION` に集約する必要。`app/web/Dockerfile`(runtime, `oven/bun:1`)は SPEC スコープ外で今回据え置くため、toolchain(build/test)と runtime image で bun 版が乖離しうる点は残存リスク(将来 SPEC で整合)。
- **`goimports@latest` の pin 必須**: offline/再現性のため版固定が要る。impl 時に resolvable な stable を確定(見つからない/衝突時は報告)。
- **`go run tool@ver` の offline 化**: sqlc/goose/swag/goimports を `go install` でイメージに温めても、Makefile が別途 `go run` で解決する経路がキャッシュを外すと network を引く。`GOPROXY=off` + GOMODCACHE 温めで担保するが、go のバージョン解決挙動(`@ver` の再検証)次第で温め漏れが起きうる。D1 の「誤 fetch 無し」確認で検知する。**万一 offline 実行できない場合の代替**: Makefile を `go run` から `go install` 済みバイナリ直呼びへ切替(ただしコマンド実体の変更になるため最終手段)。
- **統合テスト / migrate と network-none の両立**: 統合系は postgres 到達に network が要るため network-none にできない(明示的例外)。この 2 系統(offline exec / online DB)をラッパーが取り違えないことが要。表を単一契約として厳守する。
- **compose 2 ファイル重ね(`-f compose.yml -f compose.tools.yml`)**: サービス名・network・volume の衝突可能性。tools サービス名が既存(postgres/api/auth/web)と衝突しないことは確認済み。merged プロジェクト名の固定(`-p`)で意図しない別プロジェクト起動を避ける。
- **UID マッピング / named volume 権限**: 任意 host UID で named volume(root 所有既定)に書けるよう、イメージ側でキャッシュ dir を world/group 書込可にする必要。Linux で顕在、macOS(VirtioFS)は緩い。E1・UID 確認で担保。
- **macOS bind-mount 性能**: 大量ファイル(node_modules・go build)で遅延。`:delegated` + cache を named volume に逃がすが、node_modules は bind 上に残る(必要なら `/workspace/app/web/node_modules` に volume を被せる案。今回は任意)。
- **イメージサイズ / ビルド時間**: polyglot は肥大化しやすい。multi-stage / 不要ファイル削除で抑制。CI は GHCR キャッシュで初回以外を短縮。
- **CI イメージ配布(冒頭 3)**: GHCR push には `packages: write` が要る。不可なら各 job build(キャッシュ付き)にフォールバック(遅延増)。
- **web host シムの PATH 依存(冒頭 2)**: `<repo>/bin` を PATH に入れ忘れると `bun` が解決されない。CLAUDE.md に明記し、checker/tester・admin のシェルで前提を満たす。
- **Docker デーモンのホスト権限(残存)**: コンテナエスケープは privileged 不使用・socket 非マウントで緩和。rootless/Podman は将来候補(SPEC §6 記載どおり)。
- **`services.<id>.image` が env context 非対応**: CI の postgres image は引き続きリテラル pin + コメントで `POSTGRES_VERSION` と同期(既存方針踏襲)。
