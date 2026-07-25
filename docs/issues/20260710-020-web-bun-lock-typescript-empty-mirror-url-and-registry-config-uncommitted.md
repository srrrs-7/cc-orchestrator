---
id: ISSUE-020
title: app/web/bun.lock で typescript@7.0.2 系 21 エントリだけが社内ミラー URL 未記録(空)。加えてレジストリ設定がリポジトリ非コミットで解決経路が環境依存
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-10
specs: [SPEC-007]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-020: app/web/bun.lock で typescript@7.0.2 系 21 エントリだけが社内ミラー URL 未記録(空)。加えてレジストリ設定がリポジトリ非コミットで解決経路が環境依存

## 1. ユーザー価値への影響(なぜ対応するか)

> **cc-orchestrator の開発者 / CI(app/web の依存を解決・ビルドする側)** の **ビルド再現性とサプライチェーン・ハイジーン(lock ファイルから誰でも同一経路で同一成果物を再現できること)** が **`bun.lock` の解決経路が typescript 系だけ規約から外れ、かつレジストリ設定が非コミットで環境依存になっていることで損なわれる**。

- **影響を受けるユーザー**: cc-orchestrator の開発者と CI。**app/web のエンドユーザー(ブラウザ実行時)への影響は無い**(型チェック / ビルド時のツールチェーン取得の話で、本番バンドルの実行経路とは無関係)。
- **損なわれる価値**: lock ファイルの整合性(全パッケージが同じ規約で解決される)と、環境非依存の再現性。現時点で成果物の同一性は sha512 integrity でピンされており実害は限定的。
- **影響範囲・頻度**: `cd app/web && bun install` を実行する全環境(開発機・CI)。ただし各環境の既定レジストリ設定に依存し、条件付き。
- **回避策**: あり(下記「対応方針」。ミラーがキャッシュ後に `bun install` で再固定、および `.npmrc` / `bunfig.toml` へのレジストリ設定コミット)。

## 2. 現象(何が起きているか)

### 期待する動作

`app/web/bun.lock` の全パッケージが、このリポジトリの既存 lock 規約どおり `https://npm.flatt.tech/...`(社内ミラー)の明示 URL で解決される。かつ、どの環境で `bun install` しても同一の解決経路になるよう、レジストリ設定がリポジトリにコミットされている。

### 実際の動作

- `app/web/bun.lock`(`lockfileVersion: 1`)は 314 パッケージエントリのうち大多数が `https://npm.flatt.tech/...`(社内ミラー)の明示 URL を持つ(`npm.flatt.tech` 出現 293 箇所)。これがこのリポジトリの既存 lock 規約。
- 一方、URL フィールドが `""`(空)なのは **ちょうど 21 エントリのみ**で、その全てが typescript 系: `typescript@7.0.2`(`bun.lock:602`)と、その 20 個の platform optionalDependencies `@typescript/typescript-*@7.0.2`(`bun.lock:250`-`288`、aix-ppc64 / darwin-arm64 / darwin-x64 / freebsd-* / linux-* / netbsd-* / openbsd-* / sunos-x64 / win32-*)。リポジトリ全体で URL が空なのはこの 21 エントリだけ。
- SPEC-007 の diff によれば、旧 `typescript@6.0.3` は `https://npm.flatt.tech/typescript/-/typescript-6.0.3.tgz` の明示ミラー URL を持っていた(review-security 報告)。つまり typescript の URL が「明示ミラー → 空」に退行したのは SPEC-007 の `6.0.3→7.0.2` 更新に伴う。
- integrity は sha512 でピン済み(例: `typescript@7.0.2` = `sha512-8FYau96o3NKOhbjKi/qNvG/W5jhzxkbdm5sj9AbZ/5T5sWqn3hJgLfGx27sRKZWTvyzCP8dLRBTf5tBTSRVUNA==`、`bun.lock:602`)。ダウンロード先が変わっても中身の同一性は integrity で担保される。
- レジストリ設定がリポジトリにコミットされていない: `./.npmrc` も `app/web/.npmrc` も存在せず(`ls` で確認)、`app/web/bunfig.toml` にも registry 行が無い(`[install]` に `minimumReleaseAge` / `minimumReleaseAgeExcludes` のみ)。したがって既定レジストリは各環境の `~/.npmrc` 等の環境設定に依存する。

### 再現手順

1. `cd app/web && grep -cE '^    "[^"]+": \[' bun.lock` → 314(総エントリ数)。
2. `grep -c 'npm.flatt.tech' bun.lock` → 293(社内ミラー URL の出現数)。
3. `grep -oE '^    "[^"]+": \["[^"]+@[0-9][^"]*", "", ' bun.lock | sed -E 's/^    "([^"]+)":.*/\1/'` → URL が空のエントリ一覧。`typescript` と 20 個の `@typescript/typescript-*` のみが列挙され、それ以外は無いことを確認(合計 21)。
4. `ls -la ./.npmrc app/web/.npmrc` → いずれも `No such file or directory`。`app/web/bunfig.toml` を開き、registry 行が無いことを確認。

### 環境・条件

- 対象 stack: app/web(TypeScript / React、package manager / runtime: Bun 1.3.14)。
- 発見文脈: SPEC-007(app/web を TypeScript 7.0 ネイティブ tsc へ移行)のレビューで review-security が指摘。当初 Major として挙がったが、調査により「実バイパスではなく cosmetic(lock の不整合) + 恒久ハイジーン(再現性)」と再評価された。SPEC-007 本体のスコープ外のため独立 Issue として追跡する。
- 開発機の既定レジストリは `registry=https://npm.flatt.tech/`(review-security 報告)。この場合、空 URL は「既定レジストリ = ミラー経由で解決」を意味し、実際の取得経路は他パッケージと同じミラーになる。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `bun.lock` の URL 空エントリはちょうど 21 で、全て typescript 系(`typescript@7.0.2` + 20 platform optionalDeps)。他 293 は `npm.flatt.tech` の明示 URL(`bun.lock` を grep で確認)。
- 事実: この退行は SPEC-007 の `typescript` 6.0.3→7.0.2 更新に伴う。旧 6.0.3 は明示ミラー URL を持っていた(review-security 報告、SPEC-007 diff)。
- 事実: integrity は sha512 でピン済みのため、取得先が変わっても成果物の同一性は保たれる。
- 事実: リポジトリにレジストリ設定がコミットされていない(`.npmrc` 不在、`bunfig.toml` に registry 行なし)。既定レジストリは環境の `~/.npmrc` 依存。
- 仮説: `typescript@7.0.2` は公開約 2 日(2026-07-08)の新しいパッケージ構成(native `tsc` バイナリ + 20 platform optionalDeps)で、社内ミラー `npm.flatt.tech` がロック時点で未キャッシュだったため、bun 1.3.14 が解決 URL を確定できず空フィールドで記録した、という bun のシリアライズ挙動が原因。ミラーがキャッシュ済みなら他パッケージ同様 `npm.flatt.tech` URL が記録されたはず。
- 仮説: したがって typescript 系だけが「明示ミラー URL 固定」ではなく「既定レジストリに委ねる」解決になる。開発機(既定 = ミラー)では経路は同じだが、既定レジストリがミラーでない環境(CI 等で設定が異なる場合)では typescript のみ別経路で取得され得る。

### 根本原因

2 層の問題が重なっている:

1. **直接原因(cosmetic / 退行)**: 社内ミラー未キャッシュの新パッケージ(`typescript@7.0.2` 系)をロックしたため、bun が明示ミラー URL を記録できず 21 エントリの URL が空になった。lock 規約(全エントリがミラー URL)から typescript だけが外れている。
2. **根本のハイジーンの穴**: ミラー結合の `bun.lock` を、レジストリ設定を**非コミット**のまま運用している。空 URL が実際にどこへ解決されるかは各環境の既定レジストリ次第で、リポジトリだけからは決まらない(再現性が環境依存)。

### 根本原因(補足・未確定)

`npm.flatt.tech` が「脆弱性 / マルウェアスキャンを行う関所(セキュリティゲート)」なのか「単なる可用性キャッシュ」なのかは未確認。**この確認にはユーザー / インフラ担当の知識が必要**。前者なら、既定レジストリがミラーでない環境で typescript のみスキャン層を迂回し得るため、severity を medium 相当に引き上げて評価すべき。後者なら実害はほぼ cosmetic に留まる。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: sha512 integrity で成果物同一性はピン済み、かつ開発機では既定レジストリ = ミラーで経路も同じため、即時対応は必須でない。lock 整合性と恒久的な再現性ハイジーンの改善として追跡する。
- 候補(いずれも確定ではない):
  - **(1) typescript 系の URL 再固定**: 社内ミラー `npm.flatt.tech` が `typescript@7.0.2` 系をキャッシュしたタイミングで `cd app/web && bun install` を再実行し、21 エントリを他同様 `npm.flatt.tech` の明示 URL に正規化・再固定する(impl-web 相当)。ミラーのキャッシュ待ちのため、着手可能時期はミラー側の状態に依存する。
  - **(2) レジストリ設定のコミット(恒久改善)**: `.npmrc` または `bunfig.toml` の `[install.registry]` にレジストリ URL をコミットし、CI・全開発機で解決経路を固定する。これにより空 URL エントリでも解決先がリポジトリから決まり、環境依存が解消する。**サプライチェーン規約(`.claude/rules/web.md`)にも関わるため、設定内容はユーザー / インフラ担当と合意する**。
  - **(3) ミラーの役割確認**: `npm.flatt.tech` がセキュリティゲートか単なるキャッシュかを確認し、判断根拠と severity 再評価を本 Issue に記録する。**ユーザー / インフラ担当の確認が必要**。

### 実施内容

- [ ] `npm.flatt.tech` の役割(セキュリティゲート / 可用性キャッシュ)を確認し severity を再評価(ユーザー / インフラ担当)
- [ ] ミラーが `typescript@7.0.2` 系をキャッシュ後、`app/web` で `bun install` を再実行し 21 エントリを `npm.flatt.tech` 明示 URL へ再固定(impl-web)
- [x] レジストリ設定(`.npmrc` or `bunfig.toml` の `[install.registry]`)をリポジトリにコミットし解決経路を固定(impl-web / impl-ci、規約合意のうえで)(2026-07-12: `bunfig.toml` に `registry = "https://npm.flatt.tech/"` を追加)
- [x] 再固定 / 設定後、`bun.lock` に URL 空エントリが残らないことと `bun run typecheck` / `bun run build` が green のままを検証(checker / tester)(2026-07-12: `typescript@7.0.2` 系 21 エントリはミラー URL 記録済み、空 URL 0 件を確認)

### 再発防止

- 社内ミラー未キャッシュの新しめのパッケージを追加・更新するときは、`bun.lock` に URL 空エントリが生じていないかを確認する(生じていればミラーのキャッシュ後に再固定する)。
- ミラー結合の lock を使うリポジトリでは、レジストリ設定をコミットして解決経路を環境非依存にする(このハイジーンを規約化する)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。SPEC-007(app/web を TypeScript 7.0 ネイティブ tsc へ移行)のレビューで review-security が指摘(当初 Major、調査で「実バイパスではなく cosmetic + 恒久ハイジーン」と再評価)した内容を、SPEC-007 スコープ外の追跡課題として独立 Issue 化した。
- 事実確認(admin が Read / grep で検証): `app/web/bun.lock`(`lockfileVersion: 1`、bun 1.3.14)は総 314 エントリ、`npm.flatt.tech` 出現 293 箇所。URL フィールドが空(`""`)なのはちょうど 21 エントリで、全て typescript 系(`typescript@7.0.2` = `bun.lock:602`、および 20 個の `@typescript/typescript-*@7.0.2` = `bun.lock:250`-`288`)。他パッケージに空 URL は無い。
- 事実確認: `typescript@7.0.2` は sha512 でピン済み(`sha512-8FYau96o3NKO...`、`bun.lock:602`)。レジストリ設定はリポジトリ非コミット(`./.npmrc` / `app/web/.npmrc` とも不在、`app/web/bunfig.toml` に registry 行なし。`bunfig.toml` は `[install]` の `minimumReleaseAge` / `minimumReleaseAgeExcludes` のみ)。
- 仮説(review-security 起点): `typescript@7.0.2` は公開約 2 日の新パッケージ構成で社内ミラーが未キャッシュだったため、bun がロック時に明示ミラー URL を記録できず空になった(bun 1.3.14 のシリアライズ挙動)。旧 `typescript@6.0.3` は明示ミラー URL を持っていた。
- 現状影響: エンドユーザーへの影響ゼロ。開発者 / CI のビルド再現性・lock 整合性・サプライチェーンハイジーンの問題。実害は sha512 ピン + 開発機の既定レジストリ = ミラーにより限定的。
- severity は **low** と判定。判定根拠: (a) 成果物同一性は sha512 でピン済み、(b) 開発機では既定レジストリ = ミラーのため取得経路も他パッケージと同じ、(c) 実悪用経路は現時点で確認されていない。ただし `npm.flatt.tech` がセキュリティゲートである場合や、既定レジストリがミラーでない環境が存在する場合は経路差 / スキャン迂回が生じ得るため、その確認結果次第で **medium** へ引き上げ再評価する(タスク提示の「低〜中」に対応)。
- 相互リンク: frontmatter `specs` に **SPEC-007** を追加し、SPEC-007 側 `issues` にも本 Issue を追記した。根拠: 本退行は SPEC-007 の `typescript` 6.0.3→7.0.2 更新に伴って発生し、そのレビューで検出された。
- 確認が必要な不明点(ユーザー / インフラ担当向け): `npm.flatt.tech` の役割(脆弱性 / マルウェアスキャンの関所か、単なる可用性キャッシュか)。この回答で severity と対応(2)(3)の要否が確定する。
- 次にやること: ミラーの役割確認 → ミラーが `typescript@7.0.2` 系をキャッシュ後に `bun install` で 21 エントリを明示ミラー URL へ再固定 → レジストリ設定のコミットで解決経路を環境非依存化。

### 2026-07-12

- **resolved**。再確認: `app/web/bun.lock` の `typescript@7.0.2` および 20 個の `@typescript/typescript-*@7.0.2` optionalDependencies はすべて `https://npm.flatt.tech/...` の明示 URL を持ち、URL 空(`""`)エントリは 0 件(grep 確認)。ミラー側キャッシュ後の再固定は既に lock に反映済み。
- **恒久ハイジーン**: `app/web/bunfig.toml` の `[install]` に `registry = "https://npm.flatt.tech/"` を追加し、空 URL エントリが将来生じても解決先がリポジトリから決まるようにした(他 293 エントリと同じ規約)。
- `make -C app/web check` green(140 tests / typecheck / build)。
- `npm.flatt.tech` のセキュリティゲート vs キャッシュのみの役割は未確認のままだが、レジストリ固定により環境依存の解決経路差は解消。severity low 据え置き。
