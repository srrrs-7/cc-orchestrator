# SPEC-007 実装計画: app/web を TypeScript 7.0(ネイティブ tsc)へ移行

- 起点: `docs/specs/20260710-007-web-typescript-7-native-tsc.md`(SPEC-007、status: approved)
- 作成日: 2026-07-10 / planner
- 目的: app/web の型チェック・ビルドのコンパイラを tsgo(`@typescript/native-preview` の日次プレビュー)から stable 同梱のネイティブ `tsc`(`typescript@7.0.2`)へ移行する。**挙動不変のツールチェーン移行**であり、生成物・UI・API 契約は一切変えない。

## 方針

### 採用するアプローチ

TypeScript 7.0 stable(`typescript@7.0.2`、`bin/tsc` に Go 実装のネイティブコンパイラを同梱)へ一本化し、`@typescript/native-preview`(tsgo)を依存から落とす。tsgo は TS7 ネイティブコンパイラの日次プレビュー版、TS7 の `tsc` はその stable 版にあたるため、型チェック挙動は互換の想定。差分が出た箇所は Spec ではなく実装で吸収し、吸収できない非互換のみ Issue 化する(§リスク)。

現状(admin 調査済みの確定事実):

- registry `typescript` の `latest` = **7.0.2**(公開 2026-07-08、約2日前)。`typescript@7.0.2` は `bin/tsc`(native)を同梱
- `app/web`: `typescript@^6.0.3`(bun.lock 固定 `6.0.3`。TS6 の `bin/tsc` は旧 JS コンパイラ)、`@typescript/native-preview@^7.0.0-dev.20260707.2`(bun.lock 固定 `7.0.0-dev.20260707.2`、`bin/tsgo`)
- `package.json` scripts: `"build": "tsgo --noEmit && vite build"`, `"typecheck": "tsgo --noEmit"`
- `bunfig.toml`: `minimumReleaseAge = 1814400`(21日)、`minimumReleaseAgeExcludes = ["@typescript/native-preview"]`。`typescript` は excludes 未登録のため、7.0.2(2日前公開)は install 時に 21日ゲートで弾かれる

### lock-then-restore(サプライチェーンゲートの一時緩和)の順序

`typescript@7.0.2` は公開約2日で `minimumReleaseAge`(21日)ゲートに掛かる。恒久緩和にせず、ロック固定でゲートを通す。impl-web は**この順序を厳守**する:

1. `bunfig.toml` の `minimumReleaseAgeExcludes` を一時的に `["typescript"]` に変更(native-preview はこの移行で依存から落とすので、excludes からも同時に外す)
2. `package.json` を編集(`typescript` を `^7.0.2`、`@typescript/native-preview` を devDependencies から削除、`scripts.typecheck` = `tsc --noEmit`、`scripts.build` = `tsc --noEmit && vite build`、`vite` を `^8.1.4` に更新)
3. `bun install` を実行 → `bun.lock` に `typescript@7.0.2` を固定し、`@typescript/native-preview` 系エントリ(本体 + 7つの platform optionalDependencies)を除去
4. `bunfig.toml` の `minimumReleaseAgeExcludes` を **`[]`(空配列)** に戻す(最終状態)。合わせて excludes 上のコメント(native-preview 言及)を「現状 preview パッケージは無く、ロック済み依存はゲートを通過するため excludes は空」の趣旨に更新する
5. **検証**: 空配列化後に `bun install --frozen-lockfile` を実行し、`bun.lock` が変質しない(= `typescript@7.0.2` 固定が維持され、native-preview が復活しない)ことを確認する。`bunfig.toml` は lock の内容に含まれないため、excludes を空に戻しても frozen install は成功し、ロック済み 7.0.2 はゲートを通過する想定。`git diff app/web/bun.lock` で typescript 行が 7.0.2、native-preview 系エントリが消えていることを目視確認する

> 退けた順序: 「先に package.json を書いてから excludes を触る」順序だと、excludes 追加前に `bun install` すると 7.0.2 がゲートで解決不能になり得るため採らない。**excludes 追加 → install → excludes 解除**の順を固定する。

### 退けた代替案(Spec §4 の要約 + 計画上の判断)

| 案 | 不採用理由 |
|---|---|
| tsgo(native-preview)を維持し typescript のみ 7.0 化 | 日次プレビュー依存が残り stable 化の価値(固定・単純化)が得られない。native `tsc` があるのに二重管理 |
| 21日ゲート通過(~7/29)を待って自然導入 | いま移行できない。lock-then-restore で安全に前倒しできる |
| `typescript` を恒久的に excludes へ追加 | TS コンパイラのサプライチェーン保護を恒久的に外す。一時緩和で足りる |
| `7.0.1-rc` 採用 | RC を本番ツールチェーンに載せる必要はない。stable 7.0.2 を導入できる |
| vite 8.1.3→8.1.4 を別対応にする | パッチ更新で安全、かつ本移行と同じ `bun install` で取り込める。**本計画に含める**(R5) |

## 変更ファイル

### app/web(impl-web 担当)

| ファイル | 変更内容 |
|---|---|
| `package.json` | devDeps: `typescript` `^6.0.3`→`^7.0.2`、`@typescript/native-preview` 行を削除、`vite` `^8.1.3`→`^8.1.4`。scripts: `typecheck` `tsgo --noEmit`→`tsc --noEmit`、`build` `tsgo --noEmit && vite build`→`tsc --noEmit && vite build`(R1/R2/R3/R5) |
| `bunfig.toml` | lock-then-restore の一時操作を経て、最終状態は `minimumReleaseAgeExcludes = []`。コメントを native-preview 言及から「excludes は空」の趣旨に更新(R6 非機能・サプライチェーン) |
| `bun.lock` | `bun install` により `typescript@7.0.2` 固定、`vite@8.1.4` 固定、`@typescript/native-preview` 本体 + platform optionalDependencies(darwin/linux/win × arch)を除去 |
| `tsconfig.json` | **原則現状維持**。`moduleResolution: Bundler` / `allowImportingTsExtensions` / `noUncheckedIndexedAccess` 等は TS7 でもサポート。TS7 の `tsc` で必要になった調整のみ最小限で対応(§リスク) |
| `Dockerfile` | **変更なし想定**。`bun run build` 経由で `tsgo` リテラル参照はなく、build stage の glibc ベース(`oven/bun:1`)選択は tsc(ネイティブバイナリ)にもそのまま当てはまる(DOCKER-001 の判断)。ビルド疎通のみ確認 |

### .github/(impl-ci 担当)

| ファイル | 変更内容 |
|---|---|
| `dependabot.yml` | web ecosystem の `ignore` から `@typescript/native-preview` ルール(L39-44 のコメント + `dependency-name` エントリ)を削除。パッケージ自体が無くなるため。他ルール(cooldown 21日・groups 等)は不変 |
| `copilot-instructions.md` | L10 の web ツーリング表 `tsgo`→`tsc`、L43 の「type-check with **tsgo** (not `tsc`)」を「type-check with **tsc** (TypeScript 7 native)」に修正 |
| `workflows/deploy.yml` | L163 コメント `tsgo --noEmit && vite build`→`tsc --noEmit && vite build`。ステップ自体は `bun run build` のため変更なし |
| `workflows/cicd.yml` | L91 コメント `Bun runtime/pm, Biome, tsgo, Vitest, Vite`→`... tsc ...`。web job のステップ(`bun run typecheck` / `build`)は script 経由のため変更なし |

### .claude/ + CLAUDE.md(**admin 対応**。orchestration の権限区分で `.claude/` と CLAUDE.md の整備は admin)

| ファイル | 変更内容 |
|---|---|
| `.claude/rules/web.md` | L22 の `bun run build` = `tsgo --noEmit && vite build`→`tsc --noEmit && vite build`。L26 の「型チェックは **tsgo**(…)で行う…**`tsc` は使わない。**」を「型チェックは **tsc**(TypeScript 7.0 ネイティブコンパイラ、`typescript` 同梱)で行う(`typecheck` = `tsc --noEmit`、`build` も `tsc --noEmit && vite build`)」へ全面差し替え(「`tsc` は使わない」の記述を削除)。L32 の native-preview を excludes する例示を、preview パッケージが無くなった現状に合わせて調整(将来 preview を追加する場合の一般則として残すか、native-preview の具体例を除去) |
| `.claude/agents/impl-ci.md` | L20 の例示 `Biome / tsgo / Vitest / Vite`→`Biome / tsc / Vitest / Vite` |
| `CLAUDE.md` | L59 コマンド表 `typecheck`(tsgo、`tsc` ではない)→`typecheck`(tsc、TypeScript 7 ネイティブ)、`build`(tsgo + Vite)→`build`(tsc + Vite) |

### 変更しないファイル(明示)

- `docs/plans/DOCKER-001-plan.md` / `docs/plans/SPEC-003-plan.md` の tsgo 記述: **過去の計画記録のため書き換えない**(ドキュメント追記のみ規約)
- 起点 Spec 本文: §5 実装計画に既に本 plan への参照(T1〜T6)があるため追加編集不要
- `app/web/src/**` のアプリコード / 生成物 / `openapi-ts.config.ts`: 挙動不変のため触らない

## 手順

依存関係: **impl-web / impl-ci / admin(ドキュメント)の 3 つは相互に独立**(text/コメント更新と実 install が別ファイル群)なので**並列実行可**。checker / tester は impl-web(実 lock/scripts)の完了に依存するため後続。

### フェーズ A(並列): impl-web ∥ impl-ci ∥ admin

- **impl-web(R1/R2/R3/R5 + 非機能 lock-then-restore)**: 上記「lock-then-restore の順序」1〜5 を厳守して `package.json` / `bunfig.toml` / `bun.lock` を更新。`vite` `^8.1.4` を同時取り込み。`tsconfig.json` は必要時のみ最小調整。Dockerfile は変更せずビルド疎通のみ確認(`bun run build` がローカルで通ること)。作業後、`git diff app/web/bun.lock` で 7.0.2 固定 / native-preview 除去 / vite 8.1.4 を確認し報告する
- **impl-ci(R6 の `.github/`)**: `dependabot.yml`(native-preview ignore 削除)/ `copilot-instructions.md` / `deploy.yml` コメント / `cicd.yml` コメント を tsc 前提に更新。YAML 構文と job 論理が壊れていないこと(コメントのみ変更でステップ不変)を自己検証
- **admin(R6 の `.claude/` + CLAUDE.md)**: `.claude/rules/web.md` / `.claude/agents/impl-ci.md` / `CLAUDE.md` を tsgo→tsc に更新(委譲不可の admin 整備作業)

### フェーズ B(A の impl-web 完了後): tester ∥ checker

- **tester(R4)**: 既存 web テストスイートを実行(`bun run test` = Vitest)。**新規テストは書かない**(挙動不変・§テスト戦略)。既存が green のまま維持されることを検証。落ちた場合は原因(TS7 起因か否か)を切り分けて報告
- **checker(R4)**: `app/web` で `bun run format:check` / `bun run lint` / `bun run typecheck` / `bun run build` を実行。特に `typecheck`(`tsc --noEmit`)と `build`(`tsc --noEmit && vite build`)が **tsc(TS7)で新規型エラーを出さない**ことを確認。型エラーが出た場合は checker が事象を報告し、impl-web に差し戻す(勝手に型チェックを外さない)

### フェーズ C(B が green 後): review-security ∥ review-spec ∥ review-performance

- **review-security**: lock-then-restore のゲート扱いの妥当性(一時緩和が最終 `[]` に戻り恒久緩和になっていないか、`bun.lock` が意図どおり 7.0.2 固定か、native-preview 系が完全除去か)を重点確認
- **review-spec**: R1〜R6 が過不足なく満たされ、スコープ外(openapi-ts bump / vitest beta 変更 / 過去 plan の書き換え)に手を出していないかを確認
- **review-performance**: tsc(native)への切替でビルド/型チェック時間が退行していないか、生成物サイズ(`dist`)に想定外変化がないかを確認

### フェーズ D: 完了処理(admin + spec skill)

- Blocker / Major 指摘があれば impl agent へ差し戻し、フェーズ B→C を再実行。今回対応しない指摘は issue-creator が Issue 化
- 起点 Spec の frontmatter(status: approved→done、updated)と §6 経緯を `spec` skill の手順で更新して完了。§5 の T1〜T6 チェックボックスを反映

### 要件 → 手順 / 検証の対応表

| 要件 | 実現手順 | 検証 |
|---|---|---|
| R1 typescript 7.0.2 化 | impl-web(package.json + install) | checker `typecheck`/`build`、review-spec、`bun.lock` diff |
| R2 native-preview 削除 | impl-web(package.json + install) | `bun.lock` から native-preview 系消失、review-security |
| R3 scripts tsc 化 | impl-web(package.json scripts) | checker が `tsc --noEmit` で走ることを確認 |
| R4 既存が green 維持(挙動不変) | 実装で吸収 | tester(test)+ checker(format/lint/typecheck/build)|
| R5 安全な更新(vite 8.1.4)取り込み | impl-web(package.json + install) | `bun.lock` で vite 8.1.4、checker `build` |
| R6 ルール/ドキュメント/CI を tsc 前提へ | impl-ci(.github) + admin(.claude/CLAUDE.md)| review-spec、grep で tsgo 残存なし(スコープ外の過去 plan を除く)|

## テスト戦略

- **新規テストは書かない(TDD の先行テストなし)**。本件は生成物・振る舞いを変えないコンパイラ差し替えであり、検証すべきは「既存の品質ゲートが tsc(TS7)で green のまま」という不変性(R4)。よって既存スイートの green 維持が戦略の中心。
- **テストの実体(R4 の検証手段)**:
  - tester: `bun run test`(Vitest + RTL)= 既存ユニット/コンポーネントテストが全 pass
  - checker: `bun run format:check`(Biome)/ `bun run lint`(Biome)/ `bun run typecheck`(`tsc --noEmit`)/ `bun run build`(`tsc --noEmit && vite build`)が全 pass
- **型差分が出た場合の扱い**: tsc(TS7)が tsgo(TS7 preview)と型チェックで差分を出し新規エラーになった場合、まず impl-web が**実装(コード / 最小限の tsconfig 調整)で吸収**する。吸収できない非互換(TS7 の破壊的変更起因等)は、勝手に型チェックを緩めず issue-creator が Issue 化して本 Spec からリンクする(Spec §4 方針に準拠)。
- **契約系 CI**: 生成物(`src/features/tasks/api/generated`)を再生成しないため contract-drift / sqlc-drift への影響はなし(スコープ外)。

## リスク / 未確定事項

- **TS7 tsc と TS6/tsgo の型チェック挙動差**: `tsconfig.json` の `moduleResolution: Bundler` / `allowImportingTsExtensions` / `noUncheckedIndexedAccess` 等は TS7 でもサポートされる想定だが、TS7 stable が preview(tsgo)と診断で差分を出す可能性は残る。→ 出た場合はフェーズ B の checker が検出し、impl-web が実装/最小 tsconfig 調整で吸収、非互換は Issue 化(上記テスト戦略)。**現時点で具体差分は未確認(推測で断定しない)**。
- **lock-then-restore の最終 install で lock が再解決される懸念**: excludes を `[]` に戻した後に `bun install` すると typescript が「新規解決」扱いでゲートに掛かり 7.0.2 が外れる可能性が理論上ある。web.md の規約では「ロック済み依存はゲートを通過する」ため維持される想定だが、**impl-web は最終 `bun install --frozen-lockfile` + `git diff bun.lock` で 7.0.2 固定の維持を明示的に確認する**(手順 5)。frozen で差分が出るなら順序/挙動を再検討し報告する。
- **Docker build 内での tsc ネイティブバイナリ取得**: `bun run build` = `tsc --noEmit && vite build` は build stage(`oven/bun:1` = glibc/debian)で走る。TS7 の `tsc` は tsgo 同様プラットフォーム別ネイティブバイナリを取得しうるため、DOCKER-001 の glibc 選択判断が tsc にも当てはまる(musl ベースを避ける)。→ impl-web はローカルの `bun run build` 疎通を確認。**CI/Docker 上での実バイナリ取得可否はローカルでは完全再現できないため、cicd.yml の web job / web イメージビルドでの build 成功が最終確認点**(未確定として明記)。arm64/amd64 双方のバイナリ取得可否に懸念があれば報告する。
- **openapi-ts / msw の typescript peer 整合(確認済み・低リスク)**: `@hey-api/openapi-ts@0.98.2` の peer は `typescript: ">=5.5.3 || ..."`、`msw@2.15.0` は `typescript: ">= 4.8.x"`(optional)で、いずれも 7.0.2 を満たす。peer 警告は出ない想定。念のため impl-web が install 時の警告有無を確認する。
- **vite 8.1.3→8.1.4 の同梱**: パッチ更新で安全と判断し本計画に含める(R5)。万一 8.1.4 が build/test に影響したら vite のみ 8.1.3 に留めて typescript 移行を優先し、事象を報告する(退避策)。
- **native-preview 除去後の dependabot 挙動**: `dependabot.yml` の ignore ルール削除後、typescript は通常の web-development グループ(cooldown 21日)で管理される。7.0.x の後続 patch は 21日 cooldown 経由で PR 化される想定(意図どおり)。
