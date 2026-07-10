---
id: SPEC-009
title: 開発ツールチェーンのコンテナ隔離(ホスト runtime 不要・サプライチェーン対策)
status: in-progress  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-10
updated: 2026-07-10
issues: [ISSUE-026]  # 関連Issue ID (例: [ISSUE-003])
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
- **実装(Phase A–C)完了**: Phase A(`versions.env` + `docker/toolchain` イメージ + `compose.tools.yml` の tools/tools-offline + devcontainer、commit `999eed0`)/ Phase B(全 Makefile + `bin/bun` の透過ラッパー、root の db-up/migrate compose 整合、`.env` 追跡除外、impl-ci charter を repo-root/横断ツーリングへ拡張、commit `f627d39`)/ Phase C(CI 3 workflow を toolchain イメージ実行へ + deploy.yml の build/creds ジョブ分離 + dependabot base-image 追跡 + provider lock の Linux hash + CLAUDE.md 注記、commit `ab1f677`)。
- **Phase D 検証**: ホストから go/bun/golangci-lint/terraform/tflint/trivy/node を PATH 除外した状態で全スタック(api/auth/migrator/iac/web)の check がコンテナ経由 green、`tools-offline` の network 遮断(fetch 失敗)を実証。**ローカル「ホスト runtime 不要」= 達成(前提は Docker のみ)**。
- **Phase E レビュー**: review-security = Blocker 0・隔離設計は実効的と評価。Major-1(deploy.yml が AWS 資格情報と同一ジョブで `bun install`)を **build-web ジョブ分離(id-token 権限なし=構造的に creds 取得不可)** で解消。Major-2(iac の net+creds+provider fetch 同時性)を **lock file の Linux hash 追加**で緩和 + 本残留リスクを §4 に記載すべき事項として記録。review-spec = R2/R3/R4/R5 充足、R1/R6/R7 は deploy.yml 修正により充足へ。
- **ユーザー報告バグの修正(portability)**: 一部の `docker compose` 実装(nerdctl/Rancher 系, v5.3.1)がトップレベル `--env-file` を拒否し(`unknown flag: --env-file`)`make lint` 等が失敗していた。全 wrapper(root + api/auth/migrator/iac Makefile + `bin/bun`)で `--env-file` を廃し、`include` 済み `versions.env` の版変数を `export` してプロセス環境から compose の `${VAR}` 展開に渡す方式へ変更(commit `ab1f677`)。sandbox 検証が standalone `docker-compose` を使い実機の plugin と乖離していたことが見逃しの原因(教訓: compose 実装差の検証)。
- **残留リスク(§4 補足)**: (a) iac の `plan`/`apply` は net 有効コンテナに AWS 資格情報を透過し provider を fetch する(lock file hash で緩和)。(b) named cache volume は単一テナント前提の 0777。(c) monorepo 全体を単一 bind-mount(単一 install 侵害の blast-radius は repo 全体、ただしホスト秘密は非漏洩)。(d) bun/golangci-lint は curl|sh の TOFU インストール。(e) devcontainer は UID 自動注入なしで固定 uid 1000。いずれも主目的(install 時のホスト秘密窃取の防止)は満たしつつ受容。
- **残タスク(follow-up)**: (1) 実 PR での CI green 確認(GHA build cache / container job の実機初回実行。ローカル未確認のため status は in-progress を維持)。(2) `.github/copilot-instructions.md` のコンテナ実行注記・`.claude/rules/*` の各コマンド表への注記(CLAUDE.md 中央注記は反映済み)。(3) SPEC-009-plan の「CI 配布方式」節を実決定(各 job build + gha cache)に更新。将来ハードニング: rootless/Podman、action の SHA pin、cache volume の権限厳格化。
- **ISSUE-026 起票(本 SPEC 由来の混入バグ)**: SPEC-010 の tester 作業中に、`app/auth/Makefile` の `test-integration` が `docker compose run` の引数順違反(`DB_ONLINE` に `tools` を内包した上で後ろに `-e ...` を追記 → `-e` が in-container コマンドに回る)で `exec: "-e": executable file not found in $PATH` になる既存バグを発見。api 側(正常)と非対称で、SPEC-009 Phase B(Makefile の toolbox ラッパー化)で混入したと推定。CI の `auth-integration` ジョブも同じ `make test-integration` を呼ぶため fail の可能性が高い(要確認)。ISSUE-026 として起票(修正は impl-auth、本 SPEC 側は相互リンクのみ)。
