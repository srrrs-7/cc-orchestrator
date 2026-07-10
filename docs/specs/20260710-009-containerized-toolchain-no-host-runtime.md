---
id: SPEC-009
title: 開発ツールチェーンのコンテナ隔離(ホスト runtime 不要・サプライチェーン対策)
status: in-progress  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-10
updated: 2026-07-10
issues: []       # 関連Issue ID (例: [ISSUE-003])
supersedes: null # 置き換える旧Spec ID
---

# SPEC-009: 開発ツールチェーンのコンテナ隔離(ホスト runtime 不要・サプライチェーン対策)

## 1. ユーザー価値(なぜ作るか)

> **開発者・CI・multi-agent ワークフロー** が **ホストに go / bun / golangci-lint 等の言語ランタイムを一切入れずに、全開発ツールを隔離コンテナ内で実行できるようになり**、**依存インストール時の悪性コード実行(Shai-Hulud 系 npm worm 等によるホスト秘密の窃取・外部送出・自己増殖)のリスクを排除できる** 価値を得る。

- **対象ユーザー**: この repo をローカルで開発する人、CI、および `make check` / `bun run` 等を実行する subagent(checker / tester / impl-*)
- **解決する課題**: 現状、`make check` / `make test` / `bun run` / `make migrate`(`go run ./cmd/migrator`)/ `deploy-web`(`bun run build`)は **ホストの go / bun ツールチェーンで直接実行**される。依存インストール(`bun install` の postinstall、`go run tool@ver`、`go generate`)がホスト権限で走るため、悪性パッケージがホストの秘密(`~/.aws` / `~/.ssh` / `~/.npmrc` / 環境変数)を読み取り外部送出・自己増殖しうる
- **得られる価値**: ホストの前提は **Docker のみ**。ツール実行は秘密を持たない使い捨てコンテナ内に閉じ、通常実行はオフライン。ローカルと CI が同一の pinned ツールチェーンで動く
- **価値の検証方法**: ホストに go / bun / golangci-lint 等が**入っていない**状態で、`make check`(api/auth/migrator)/ `bun run`(web)/ `make openapi` / `make sqlc` / `make migrate` / iac の fmt・validate がすべてコンテナ経由で成功し、check/build/test フェーズが `--network none` で完走すること、CI が同一 toolchain イメージで green になることを確認できたら成功とみなす

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- 開発者として、ホストに go/bun を入れずに `make check` を叩きたい。なぜなら悪性依存にホストの認証情報を渡したくないから。
- subagent(checker/tester/impl)として、これまでどおり `make check` / `bun run test` を呼ぶだけで、中身がコンテナ内で走ってほしい。なぜなら呼び出し契約(`.claude/rules` のコマンド表)を変えたくないから。
- CI として、ローカルと同じ pinned ツールチェーンイメージで検査したい。なぜなら「ローカルで通ったが CI で落ちる/その逆」を無くしたいから。

### 利用フロー

1. 開発者が `make check`(または `bun run test` 等)をホストで叩く
2. ラッパーが「コンテナ内か?」を判定し、ホストなら `docker compose -f compose.tools.yml run --rm tools <cmd>` に透過的に再実行する
3. install(deps)フェーズのみ network 有効で依存を named volume キャッシュへ取得、check/build/test 等は `--network none` で実行される
4. 結果(生成物・テスト結果)は bind-mount 経由でリポジトリに反映される。ホストの `~/.bun` / `~/go` / 秘密には一切触れない

## 3. 要件(何を満たすべきか)

### 機能要件

- [ ] R1: 単一の polyglot「toolchain」イメージ(`docker/toolchain/`)を用意し、go / bun / golangci-lint / terraform / tflint / trivy と `go run` 系ツール(sqlc / goose / swag)を含める。バージョンは単一ソース(`versions.env`。現状 `cicd.yml` env を昇格)から Dockerfile・CI が共有する
- [ ] R2: `compose.tools.yml` の `tools` サービスを実行の正典にする。repo を `/workspace` に bind-mount、キャッシュは named volume(`gomodcache` / `gobuild` / `buncache`)、`user` = 実行者 UID:GID、**ホスト秘密のマウントなし**、`no-new-privileges` / `cap_drop: [ALL]` / docker socket 非マウント
- [ ] R3: 2 フェーズ network — deps(install)は network 有効(lockfile + sha512 + `minimumReleaseAge` + go.sum + pin を第一線に維持)、check / build / test / generate / sqlc / openapi は **`--network none`**
- [ ] R4: コマンド契約は不変。`make check` / `make test` / `make fmt` 等(api/auth/migrator)、`bun run <script>`(web)、`make openapi` / `make sqlc` / `make migrate` / iac の `make fmt|validate|lint|security|plan` の入口を維持し、ホスト呼び出し時に `tools` コンテナへ透過再実行する(`IN_TOOLBOX` マーカーで多重ラップ防止)。`.claude/rules/*` のコマンド表・CLAUDE.md 早見表・各 agent 呼び出しは**変えない**
- [ ] R5: devcontainer(`.devcontainer/devcontainer.json`)を同一 `tools` サービス参照で用意(非 root remoteUser・秘密マウントなし)。対話開発向けの任意レイヤ
- [ ] R6: CI(`.github/workflows/*`)を同一 toolchain イメージに寄せる。`setup-go` / `setup-bun` / golangci-lint の curl install を廃し、pinned image(container job もしくは image 実行)を使う。ローカル = CI を成立させる
- [ ] R7: AWS デプロイ系(`push-images` / `deploy-web`)は build をコンテナ・`aws` CLI を host に分割し、AWS 認証情報をコンテナに入れない

### 非機能要件

- **多層防御**: lockfile + 整合性 + `minimumReleaseAge`(既存)が第一線、コンテナ隔離(秘密非保持 + network-none)が第二線。両輪で運用する
- **CI パリティ**: toolchain バージョンは単一ソースで、ローカルと CI が一致すること
- **macOS 性能**: bind-mount は `:delegated`、依存・キャッシュは named volume に逃がす
- **既存挙動不変**: アプリの振る舞い・生成物・契約は変えない(実行環境の移設のみ)

### スコープ外(やらないこと)

- rootless Docker / Podman への完全移行(将来のハードニング候補として記録のみ)
- 既存アプリ runtime イメージ(`app/*/Dockerfile`)の設計変更
- egress プロキシ allowlist の厳密構築(deps フェーズの最小化は行うが、専用プロキシ導入は将来)

## 4. 設計(どう実現するか)

### 方針

「危険は依存インストール時 + ホスト秘密 + egress」という脅威モデルに対し、**ツール実行を『秘密を持たない使い捨てコンテナ』へ移設**し、**通常実行を `--network none`** にする。エディタ(devcontainer)や CI も**同一イメージ**を参照し、単一の pinned ツールチェーンに一本化する。呼び出し契約は Makefile ラッパーで透過化し、agent/CLI/CI すべてが同じ経路を通る。

### アーキテクチャ / データ / インターフェース

- **`versions.env`**(新規・単一ソース): `GO_VERSION` / `BUN_VERSION` / `GOLANGCI_LINT_VERSION` / `TERRAFORM_VERSION` / `TFLINT_VERSION` / `TRIVY_VERSION`(現状 `cicd.yml` env の値)を昇格。Dockerfile(build arg)と CI(env)が読む
- **`docker/toolchain/Dockerfile`**: 上記を ARG で受け、go/bun/golangci-lint/terraform/tflint/trivy + sqlc/goose/swag を焼く。非 root ユーザー
- **`compose.tools.yml`**: `tools` サービス(上記イメージ)。`/workspace` bind-mount、named cache volume、`user`、秘密非マウント、`security_opt`/`cap_drop`。deps 用と run 用でネットワーク指定を分ける(run は `network_mode: none` 相当、または `docker compose run --network none`)
- **Makefile ラッパー**: 各 stack Makefile / ルート Makefile の対象ターゲットで `IN_TOOLBOX` を判定し、未設定なら `docker compose -f compose.tools.yml run --rm [--network none] tools $(MAKE) <target>` に委譲(`<target>-native` 等の内部ターゲットで実体を分離)
- **`.devcontainer/devcontainer.json`**: `dockerComposeFile: compose.tools.yml` / `service: tools` / `workspaceFolder: /workspace` / 非 root
- **CI**: 各 job を toolchain image の container job(または image 内実行)に変更し、`setup-*`/curl を撤去。`versions.env` を参照
- **ドキュメント**: `.claude/rules/{web,api,db,iac,testing}.md` の「コマンド」表は**コマンド名を変えず**、実行がコンテナ内である旨の注記を追加。CLAUDE.md「ローカル実行」「コマンド早見表」も同様

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| devcontainer のみ | エディタ中心で、subagent/CLI/CI の実行経路をカバーしない。この repo は agent 駆動が中心 |
| スタック別 toolchain イメージ(tools-go/web/iac) | blast-radius は最小化できるが 3 イメージ + バージョン整合の保守コストが増える。単一イメージ + network-none + 秘密非保持で隔離目的は達成できる(ユーザー決定) |
| ホスト実行のまま lockfile 等だけで対処 | 第一線は維持するが、install 時のホスト秘密露出という根本リスクが残る |
| docker socket をマウントしてコンテナから docker 操作 | ソケット露出は事実上のホスト root 相当。非マウントを維持 |

## 5. 実装計画

詳細は `docs/plans/SPEC-009-plan.md`(planner が作成)。着手前に planner が以下を設計する:

- [ ] T1: planner が方針(versions.env 昇格・toolchain Dockerfile・compose.tools・Makefile 透過ラッパーの具体形・2 フェーズ network の compose 表現・CI の container 化順序)と影響範囲・手順・テスト戦略・リスクを計画化
- [ ] T2: impl-ci が `versions.env` / `docker/toolchain/Dockerfile` / `compose.tools.yml` / CI(`.github/workflows/*`)の container 化 / `.devcontainer/` を実装
- [ ] T3: impl-api / impl-auth / impl-db / impl-web が各 stack の Makefile / package.json スクリプトに透過ラッパーを適用(コマンド契約は不変)。ルート Makefile の `migrate` / `deploy-web` のコンテナ化・AWS 分割
- [ ] T4: admin が `.claude/rules/*` / CLAUDE.md をコンテナ実行前提の注記に更新(`.claude`/CLAUDE.md 整備は admin 権限)
- [ ] T5: checker / tester が「ホストに runtime を入れない状態」での全コマンド疎通と `--network none` 完走、CI green を検証
- [ ] T6: review-security(秘密非保持・egress・socket 非マウント・network-none の妥当性)/ review-spec

## 6. 経緯(時系列・追記のみ)

### 2026-07-10

- 初版作成(status: approved)。プロジェクト全体レビュー後、ユーザーから「ホストに bun/go ランタイムを入れて実行するのは Shai-Hulud 系サプライチェーン攻撃のリスクがあるため避けたい。devcontainer か `docker compose run` でコンテナ隔離 + volume マウントを検討」との方針提示を受け、admin が脅威モデル(危険は install 時 + ホスト秘密 + egress)を整理して設計を提案・対話で確定。
- **ユーザー決定**: (1) イメージ粒度は **単一 polyglot + `--network none` 実行フェーズ**(スタック別イメージは不採用)。(2) CI 統合は本 SPEC で一緒に対応(ローカル = CI を同一イメージで成立させる)。
- 採用設計は §4 のとおり。compose `run` のツールチェーンを実行の正典にし、devcontainer は同一イメージの任意上乗せ。コマンド契約(`.claude/rules` のコマンド表・CLAUDE.md 早見表・agent 呼び出し)は不変にして Makefile ラッパーで透過化する方針を確定。
- 残留リスクとして Docker デーモンのホスト権限(コンテナエスケープ)を明記。privileged 不使用・docker socket 非マウントで緩和し、将来 rootless/Podman を検討候補とする。lockfile + 整合性 + `minimumReleaseAge` を第一線、コンテナ隔離を多層防御と位置づける。
- 関連: SPEC-005(app/migrator の `go run`)/ SPEC-007(bun/TS7)/ ISSUE-020(レジストリ設定コミット)と整合を取る。実装は planner に委ねる。
