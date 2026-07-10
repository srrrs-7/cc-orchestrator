---
id: SPEC-007
title: app/web を TypeScript 7.0(ネイティブ tsc)へ移行
status: done  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-10
updated: 2026-07-10
issues: [ISSUE-020, ISSUE-013, ISSUE-023]       # 関連Issue ID (例: [ISSUE-003])
supersedes: null # 置き換える旧Spec ID
---

# SPEC-007: app/web を TypeScript 7.0(ネイティブ tsc)へ移行

## 1. ユーザー価値(なぜ作るか)

> **app/web の開発者** が **stable な TypeScript 7.0 のネイティブコンパイラ(tsc)で型チェック・ビルドできるようになり**、**日次プレビュー(tsgo)依存を捨てて再現性と保守性が上がる** という価値を得る。

- **対象ユーザー**: app/web を開発・CI で回す開発者(および将来のメンテナ)
- **解決する課題**: 現状、型チェック/ビルドは `@typescript/native-preview`(tsgo)= **日次の dev プレビュービルド**に依存している。これはネイティブコンパイラを 7.0 stable 以前に先取りするための暫定手段だった。TypeScript 7.0 が stable リリース(registry `latest` = 7.0.2、`typescript@7` は自前の native `tsc` バイナリを同梱)された今、プレビュー専用パッケージを使い続ける理由がなくなり、むしろ「毎日中身が変わる依存」を抱え続ける保守負債になっている
- **得られる価値**: stable でバージョン固定された単一の TypeScript(`typescript@7.0.2`)に一本化。プレビューパッケージと dependabot/bunfig の特例扱いが不要になり、ツールチェーンが理解しやすくなる
- **価値の検証方法**: `@typescript/native-preview` を依存から外し、`bun run typecheck` / `bun run build` が `typescript@7.0.2` 同梱の `tsc` で実行され、既存の型チェック・lint・format・テスト・ビルドがすべて green のままであることを確認できたら成功とみなす

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- app/web 開発者として、`bun run typecheck` / `bun run build` を stable な TypeScript 7.0 の `tsc` で実行したい。なぜなら日次で中身が変わるプレビュービルドではなく、固定・再現可能なコンパイラで型健全性を担保したいから。

### 利用フロー

1. 開発者が `app/web` で `bun install` する(`typescript@7.0.2` が `bun.lock` に固定され、`@typescript/native-preview` は消える)
2. 開発者が `bun run typecheck` を実行する → `tsc --noEmit` が走る
3. 開発者が `bun run build` を実行する → `tsc --noEmit && vite build` が走る
4. CI・ドキュメント・ルールも「tsgo」ではなく「tsc(TypeScript 7.0 ネイティブ)」を前提として記述されている

## 3. 要件(何を満たすべきか)

### 機能要件

- [x] R1: `app/web` の `typescript` を `6.0.3` から `7.0.2`(stable、native tsc 同梱)へ更新する
- [x] R2: `@typescript/native-preview`(tsgo)を `app/web` の依存から削除する
- [x] R3: `package.json` の scripts を tsgo から tsc に変更する(`typecheck` = `tsc --noEmit`、`build` = `tsc --noEmit && vite build`)
- [x] R4: 既存の `typecheck` / `lint` / `format:check` / `test` / `build` がすべて green のまま(挙動不変)であること
- [x] R5(一般化): 関連ライブラリのうち **21日ゲートを通過済みで安全に取り込める更新** を取り込む(既に latest のものは対象外、該当なければ実施しない)。※当初 `vite` 8.1.3→8.1.4 を想定したが、8.1.4 は本作業時点で公開約1日=`minimumReleaseAge`(21日)ゲート対象。patch のためにサプライチェーンゲートを緩和しない方針(非機能要件の趣旨)に従い **意図的に見送り**、ゲート通過後 or dependabot 経由で取り込む
- [x] R6: tsgo/native-preview を参照するルール・ドキュメント・CI を tsc 前提に更新する(`.claude/rules/web.md` の「`tsc` は使わない」削除、`CLAUDE.md` コマンド表、`.claude/agents/impl-ci.md`、`.github/` の `deploy.yml`/`cicd.yml` コメント・`copilot-instructions.md`・`dependabot.yml` の native-preview ignore 削除)

### 非機能要件

- **サプライチェーン**: `typescript@7.0.2` は公開約2日で `bunfig.toml` の `minimumReleaseAge`(21日)ゲートに掛かる(`typescript` は excludes 未登録)。**lock-then-restore** で導入する: `typescript` を一時的に `minimumReleaseAgeExcludes` に追加 → `bun install` で `7.0.2` を `bun.lock` に固定 → 除外を解除する。ロック済みバージョンはゲートを通過するため、恒久的なゲート緩和にはしない。`@typescript/native-preview` を落とすので `minimumReleaseAgeExcludes` からも native-preview を削除する
- **挙動不変**: 生成物・アプリの振る舞いを変えない純粋なツールチェーン移行。UI・API 契約・生成コードに変更を持ち込まない

### スコープ外(やらないこと)

- `@hey-api/openapi-ts` の bump(0.98.2→0.99.0)。生成物の再生成 = contract-drift 影響が本移行と直交するため別途対応
- `vitest` の beta ライン変更(現状 `5.0.0-beta.6`。stable `latest` は 4.x で、beta 採用は意図的)
- 既に latest のライブラリ(react/@types/react/@tanstack/*/zod/biome/msw/tailwindcss 等)の変更
- `docs/plans/` の過去記録(DOCKER-001, SPEC-003 等)の tsgo 記述の書き換え(追記のみ規約のため過去エントリは変更しない)
- `terraform apply` や本番デプロイ等の運用操作

## 4. 設計(どう実現するか)

### 方針

TypeScript 7.0 stable(`typescript@7.0.2`)へ一本化し、コンパイラをプレビュー配布(`@typescript/native-preview` の `tsgo` バイナリ)から stable 同梱のネイティブ `tsc` に切り替える。TS7 の `tsc` は Go 実装のネイティブバイナリで、tsgo と同じネイティブコンパイラの stable 版にあたるため、型チェック挙動は互換の想定。差分が出た箇所は Spec ではなく実装で吸収し、吸収できない非互換は Issue 化する。

### アーキテクチャ / データ / インターフェース

- **package.json**: `devDependencies` から `@typescript/native-preview` を削除、`typescript` を `^7.0.2` に更新。`scripts.typecheck` = `tsc --noEmit`、`scripts.build` = `tsc --noEmit && vite build`
- **bunfig.toml**: `minimumReleaseAgeExcludes` から `@typescript/native-preview` を削除。lock-then-restore の一連の操作後、最終状態では `typescript` は excludes に**残さない**(`bun.lock` 固定で通過)
- **bun.lock**: `typescript@7.0.2` 固定、`@typescript/native-preview` 系エントリ削除
- **tsconfig.json**: 原則現状維持(`moduleResolution: Bundler` / `allowImportingTsExtensions` 等は TS7 でもサポート)。TS7 で必要になった調整があれば最小限で対応
- **Dockerfile(app/web)**: `bun run build` 経由のため `tsgo` リテラル参照はなし。`tsc`(TS7)も tsgo 同様ネイティブバイナリのため、build stage の glibc ベース(`oven/bun:1`)選択は引き続き妥当(DOCKER-001 の判断が tsc にもそのまま当てはまる)。原則変更なし・ビルド疎通のみ確認
- **ドキュメント/ルール/CI**: R6 の各所を tsc 前提に更新

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| tsgo(native-preview)を維持し `typescript` のみ 7.0 に上げる | プレビュー(日次 dev ビルド)への依存が残り、stable 化の価値(固定・単純化)が得られない。stable 同梱の `tsc` があるのに二重管理になる |
| `7.0.2` の 21日ゲート通過(~7/29)を待つ | いま移行できない。lock-then-restore でロック固定すれば安全に前倒しできる |
| `typescript` を恒久的に `minimumReleaseAgeExcludes` へ追加 | TS コンパイラのサプライチェーン保護を恒久的に外すことになる。一時的な lock-then-restore で足りる |
| `7.0.1-rc` を採用 | RC を本番ツールチェーンに載せる必要はない。lock-then-restore で stable `7.0.2` を導入できる |

## 5. 実装計画

詳細は `docs/plans/SPEC-007-plan.md`(planner が作成)。概要:

- [x] T1: planner が移行手順(特に lock-then-restore の順序)と影響範囲を計画化
- [x] T2: impl-web が `package.json` / `bunfig.toml` / `bun.lock`(install)を更新(R1/R2/R3 + R6 の非機能要件 lock-then-restore。R5=vite はゲート対象で見送り)
- [x] T3: impl-ci が `.github/`(deploy.yml/cicd.yml コメント・copilot-instructions.md・dependabot.yml)を tsc 前提に更新(R6)
- [x] T4: admin が `.claude/rules/web.md` / `.claude/agents/impl-ci.md` / `CLAUDE.md` を tsc 前提に更新(R6、`.claude`/CLAUDE.md 整備は admin 権限)
- [x] T5: tester が既存テストスイート実行、checker が format/lint/typecheck/build 実行(R4)
- [x] T6: review-security(lock-then-restore のゲート扱い)/ review-spec / review-performance がレビュー

## 6. 経緯(時系列・追記のみ)

### 2026-07-10

- 初版作成。TypeScript 7.0(ネイティブコンパイラ)が stable リリース(registry `latest` = `typescript@7.0.2`、`typescript@7` は native `tsc` バイナリを `bin/tsc` として同梱)されたのを機に、app/web を tsgo(`@typescript/native-preview` 日次プレビュー)から stable の native `tsc` へ移行する方針を承認(status: approved)。`7.0.2` は公開約2日で 21日サプライチェーンゲートに掛かるため lock-then-restore で導入する非機能要件を明記。openapi-ts bump / vitest beta 変更はスコープ外とした。
- レビューでスコープ外の追跡課題を 2 件整理し、frontmatter `issues` に相互リンクした:
  - **ISSUE-020**(新規起票): SPEC-007 の `typescript` 6.0.3→7.0.2 更新に伴い、`app/web/bun.lock` の `typescript@7.0.2` 系 21 エントリだけが社内ミラー URL 未記録(空)になった cosmetic 退行 + レジストリ設定が非コミットで解決経路が環境依存という再現性ハイジーンの穴。severity low(sha512 ピン済み・実害限定的)。ミラー役割の確認とレジストリ設定コミットを追跡。
  - **ISSUE-013**(既存へ追記): `js-yaml@4.1.1` の moderate 脆弱性(GHSA-h67p-54hq-rp68)。SPEC-007 のレビューで再検出したが、`js-yaml` のバージョンは SPEC-007 diff で不変(4.1.1→4.1.1、URL のみ変化)で SPEC-007 とは無関係な既存状態。既存 ISSUE-013 が同一 advisory・同一依存経路を既にカバーするため新規起票せず追記のみ。openapi-ts bump(本 Spec スコープ外)で修正版に上がるかを別スコープで追跡。
- **実装完了・検収**: planner→(impl-web ∥ impl-ci ∥ admin)→(tester ∥ checker)→(review-security ∥ review-spec ∥ review-performance)のパイプラインを完走し、価値の検証方法を満たしたため status を done とした。
  - impl-web: `typescript` 6.0.3→7.0.2 / `@typescript/native-preview` 削除 / scripts を `tsc --noEmit`・`tsc --noEmit && vite build` 化を **lock-then-restore**(excludes へ typescript 一時追加→`bun install` で固定→`[]` へ復元→`bun install --frozen-lockfile` で固定維持を確認)で実施。`tsconfig.json` / `Dockerfile` は無変更(TS7 tsc が新規型エラーを出さず)。R1/R2/R3 達成。
  - impl-ci: `.github/`(dependabot の native-preview ignore 削除・copilot-instructions・deploy/cicd コメント)を tsc 前提に更新。admin: `.claude/rules/web.md`(「`tsc` は使わない」削除・tsc 一本化・lock-then-restore を明文化)/ `.claude/agents/impl-ci.md` / `CLAUDE.md` を更新。tsgo/native-preview の残存はスコープ外の過去 plan(DOCKER-001 / SPEC-003)のみ。R6 達成。
  - tester: 既存 Vitest 9 files / 73 tests 全 pass。checker: `format:check` / `lint` / `typecheck`(TS7 `tsc`)/ `build` 全 green。R4 達成(挙動不変)。
  - review: **performance** = 退行なし(typecheck 0.38s / build 0.54s、`dist` 不変、native バイナリ 1→1 の置換)。**security** = Major「`bun.lock` の `typescript@7.0.2` 系 21 エントリだけ URL が空」は、調査で既定レジストリ=社内ミラー(`~/.npmrc`、repo に registry 設定コミットなし)かつ sha512 ピン済みと判明し、実バイパスではなく cosmetic + 再現性ハイジーンと再評価 → ISSUE-020 で追跡。**spec** = Major「R5(vite)未達・未記録」は R5 を一般化し本経緯に見送り理由を記録して吸収。
  - 残課題: ISSUE-020(bun.lock URL 正規化 + レジストリ設定コミット、`npm.flatt.tech` の役割確認が前提)、R5 の vite 8.1.4(ゲート通過後 or dependabot)。
- **移行後レビュー(プロジェクト全体レビュー)での回帰検出と解消**: 本移行がスコープ外(§3 スコープ外)とした `@hey-api/openapi-ts`(0.98.2)が、TS7 ネイティブ tsc の `ts.SyntaxKind` API と非互換で `cd app/web && bun run generate` が `TypeError: undefined is not an object (evaluating 'ts.SyntaxKind.AnyKeyword')` で失敗する回帰を検出。コミット済み生成物・typecheck・build・test は正常だが**再生成が不能**で、`.github/workflows/contract-drift.yml` が Go DTO 変更時に fail する状態だった。設計方針「吸収できない非互換は Issue 化する」(§4)に沿って **ISSUE-023** として起票し、frontmatter `issues` に追加。
- 解消: impl-web が `@hey-api/openapi-ts` を TS7 対応の next プレリリース `0.0.0-next-20260708192938` へピン留め(stable は最新 `0.99.0` まで TS7 未対応を実機確認)。lock-then-restore で `bunfig.toml` は `minimumReleaseAgeExcludes = []` を維持し、`typescript@^7.0.2`(TS7)も維持。checker が `bun run generate`(成功・冪等)/ typecheck / lint / format:check / build / test を独立検証して全 pass。stable が TS7 対応したら stable へ戻す follow-up は ISSUE-023 の経緯で追跡(新規 Issue は切らない)。
