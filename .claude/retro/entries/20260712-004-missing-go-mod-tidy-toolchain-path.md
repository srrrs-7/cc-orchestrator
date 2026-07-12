---
id: RETRO-004
title: SPEC-009 下で go mod tidy を実行する正規経路がなく 3 agent が同じ回避策を重複考案した
status: open
severity: medium
source: impl-db  # impl-api / impl-auth も同一摩擦に遭遇(検収: admin)
phase: impl
target: rules/api.md  # 同型の欠落: rules/auth.md / rules/db.md(と各 Go スタックの Makefile)
created: 2026-07-12
updated: 2026-07-12
synthesis: RETROSUM-001
tags: [missing-command, spec-009, makefile-gap, duplicated-workaround]
---

# RETRO-004: SPEC-009 下で go mod tidy を実行する正規経路がなく 3 agent が同じ回避策を重複考案した

## 1. 遭遇した課題(何が摩擦だったか)

> **rules/{api,auth,db}.md のコマンド表と各 Go スタックの Makefile** に **依存解決(`go mod tidy` / `go mod download`)を toolchain コンテナで実行する経路の記載・ターゲットが欠落**しており、**impl-api / impl-auth / impl-db** が **それぞれ独立に compose コマンドを手組みする同じ回避策を重複して考案・実行**した。

- **具体的に何が起きたか**: `git merge origin/main`(dependabot の依存 bump)の go.mod 競合解消タスクで、3 agent 全員が `go mod tidy` を必要とした。SPEC-009 により「ホストで go を直接実行しない」制約があるが、`app/{api,auth,migrator}/Makefile` に tidy 相当のターゲットがなく(auth の Makefile には「no deps/install phase exists in this Makefile that would need network」とコメントまである)、rules のコマンド表にも記載がない。結果、各 agent が `.devcontainer/compose.tools.yml` の `tools` サービス(network 有効)を直接呼ぶコマンドを自力で組み立てた:
  ```
  docker compose -f .devcontainer/compose.tools.yml run --rm --workdir /workspace/app/<stack> tools go mod tidy
  ```
  その際、`versions.env` の export・`TOOLBOX_UID`/`TOOLBOX_GID`/`TOOLBOX_WORKSPACE`/`TOOLBOX_CONTEXT`/`COMPOSE_PROJECT_NAME` の設定規約・compose plugin 不在時の standalone `docker-compose` へのフォールバック判定まで、3 agent が各自 Makefile を読み解いて再発見した
- **どのアセットの問題か**: 欠落(コマンド表に「依存を触る変更(go.mod 編集・merge 競合解消)後に何を実行するか」の正規経路がない)

## 2. 影響(タスクにどう響いたか)

- **症状**: 非効率(同一の回避策を 3 回重複考案)。回避はできたためブロックなし
- **コスト**: 各 agent が Makefile / compose.tools.yml / versions.env の読解に数ツール呼び出しずつ消費(計 10+ tool uses 相当)。手組みコマンドは環境変数の設定漏れ・network フェーズの取り違え(offline サービスで tidy 実行など)のリスクを毎回伴う

## 3. 改善提案(どう直すか)

いずれか(両方でもよい):

1. **各 Go スタックの Makefile に `tidy` ターゲットを追加**(network 有効の `tools` サービス経由。web の `install` と同じ位置づけ)し、`rules/api.md`・`rules/auth.md`・`rules/db.md`(migrator 分)のコマンド表に載せる。`make check` には含めない(生成系と同じ扱い)。Makefile の変更は各 impl(api / auth)+ impl-db(migrator)に委譲する
2. **仮説:** ワンオフの Go コマンド全般(tidy に限らない)に効く汎用策として、CLAUDE.md「実行環境(SPEC-009)」または rules 側に「Makefile にないコマンドを toolchain コンテナで実行する標準手順」(tools = network 有効 / tools-offline = 検査系、変数設定の規約、compose バイナリのフォールバック)を 1 箇所だけ記す。ただし乱用されると SPEC-009 の統制が緩むため、1 の明示ターゲット化のほうが筋が良い可能性が高い

## 4. 根拠 / 再現(なぜそう言えるか)

- **根拠**: impl-api・impl-auth・impl-db の 3 報告すべてに「Makefile に tidy 相当のターゲットが存在しないため compose を直接実行した」旨の記載。impl-db は報告の申し送り事項で `tidy` ターゲット追加を明示的に提案。impl-auth は `app/auth/Makefile` の当該コメントを引用
- **再現条件**: go.mod / go.sum に触る任意のタスク(依存追加・bump・merge 競合解消)で、SPEC-009 準拠のまま `go mod tidy` が必要になったとき。今回は `feat/auth-oidc-foundation` への `origin/main`(dependabot bump)マージで発生

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 記録。origin/main マージの go.mod 競合解消(impl-api / impl-auth / impl-db に 3 並列委譲)の検収中に、3 報告が同一の回避策を独立に記述していたことから摩擦として吸い上げた
