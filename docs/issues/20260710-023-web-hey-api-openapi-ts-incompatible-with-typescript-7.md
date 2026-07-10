---
id: ISSUE-023
title: "@hey-api/openapi-ts が TypeScript 7 ネイティブ tsc と非互換で bun run generate が失敗し、OpenAPI 契約から web 型を再生成できない(SPEC-007 が SPEC-003 の型生成を壊した回帰)"
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: high  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-10
specs: [SPEC-003, SPEC-007]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-023: @hey-api/openapi-ts が TypeScript 7 ネイティブ tsc と非互換で bun run generate が失敗し、OpenAPI 契約から web 型を再生成できない(SPEC-007 が SPEC-003 の型生成を壊した回帰)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/api の DTO を変更して web の型を追従させる開発者** の **「OpenAPI 契約(SPEC-003)から web の型 / Zod スキーマ / TanStack Query クライアントを再生成できる」という開発体験** が、**`@hey-api/openapi-ts` が TypeScript 7 ネイティブコンパイラと非互換で `bun run generate` が例外終了し、契約から web 型を再生成できないことで損なわれている**。

- **影響を受けるユーザー**: app/api ⇄ app/web の型契約(SPEC-003)を扱う開発者。とくに Go の DTO を変更する開発者(変更後の再生成ができない)
- **損なわれる価値**: OpenAPI 契約からの web 型再生成という、SPEC-003 の中核ワークフロー。コミット済みの生成物は正常なため既存の build / typecheck は通るが、**契約が変わったときに追従できない**
- **影響範囲・頻度**: 常時(`cd app/web && bun run generate` を実行するたびに失敗する)。加えて **Go の DTO を変えたとき** に CI の `.github/workflows/contract-drift.yml`(`bun run generate` を実行して差分を検査する)が必ず fail するため、DTO 変更を含む PR がマージできなくなる = CI ブロッカー
- **回避策**: 現状なし(SPEC-003 の再生成手段そのものが動かない)。緊急には手書きで生成物を更新する手もあるが、これは SPEC-003 の「手書きで二重定義しない」方針に反する

## 2. 現象(何が起きているか)

### 期待する動作

`cd app/web && bun run generate`(= `openapi-ts`)が、`app/api/docs/openapi.yaml` を入力に、`src/features/tasks/api/generated/` へ型 / Zod スキーマ / TanStack Query クライアントを正常に生成して終了する(SPEC-003)。

### 実際の動作

`cd app/web && bun run generate` が以下の例外で失敗する(タスク起票元のレビューで検証済み):

```
TypeError: undefined is not an object (evaluating 'ts.SyntaxKind.AnyKeyword')
```

`@hey-api/openapi-ts`(生成器)が内部で参照する TypeScript コンパイラ API `ts.SyntaxKind`(例: `AnyKeyword`)が、app/web が採用した TypeScript 7 ネイティブコンパイラ(SPEC-007)では従来と同じ形で公開されておらず、`undefined` を参照して例外になる。

- コミット済みの生成物(`app/web/src/features/tasks/api/generated/` = `client.gen.ts` / `sdk.gen.ts` / `types.gen.ts` / `zod.gen.ts` / `@tanstack` / `core` 等)は正常なため、`bun run typecheck`(`tsc --noEmit`)・`bun run build`(`tsc --noEmit && vite build`)は通る。**壊れているのは「再生成」のみ**。

### 再現手順

1. app/web の依存が `typescript: ^7.0.2`(TS7 ネイティブ tsc、SPEC-007)かつ `@hey-api/openapi-ts: 0.98.2`(`app/web/package.json`)である現状のリポジトリを用意する。
2. `cd app/web && bun install`(未取得なら)。
3. `cd app/web && bun run generate`(= `openapi-ts`、`app/web/package.json:13`)を実行する。
4. `TypeError: undefined is not an object (evaluating 'ts.SyntaxKind.AnyKeyword')` で失敗し、`src/features/tasks/api/generated/` が生成されないことを確認する。
5. 対比: `bun run typecheck` / `bun run build` は(コミット済み生成物を使うため)成功することを確認する。

### 環境・条件

- 対象 stack: app/web(TypeScript / React)。SPEC-003(OpenAPI 契約からの型生成)と SPEC-007(TS7 ネイティブ tsc への移行)の交差点。
- 発見文脈: プロジェクト全体レビューで `bun run generate` の失敗として検証された。
- CI: `.github/workflows/contract-drift.yml` は Go の swag v2 から `openapi.yaml` を再生成 → `bun run generate` で web 型を再生成 → コミット済みとの差分を検査するジョブ(SPEC-003)。この `bun run generate` が失敗するため、当該ジョブは Go DTO の変更が入った時点で必ず fail する。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/web/package.json` で `typescript: ^7.0.2`(`:39`)・`@hey-api/openapi-ts: 0.98.2`(`:28`)。`generate` スクリプトは `openapi-ts`(`:13`)。
- 事実: 生成設定は `app/web/openapi-ts.config.ts`。`input: "../api/docs/openapi.yaml"`、`output: "src/features/tasks/api/generated"`、plugins に `@hey-api/client-fetch` / `@hey-api/typescript` / `zod`(compatibilityVersion 4)/ `@tanstack/react-query`。
- 事実: エラーは `TypeError: undefined is not an object (evaluating 'ts.SyntaxKind.AnyKeyword')`。`@hey-api/openapi-ts` は生成物を作る際に TypeScript コンパイラ API(`ts.SyntaxKind` 等の AST ファクトリ)を利用するが、TS7 ネイティブコンパイラでは当該 API の公開形が従来(TypeScript 5.x JS 実装)と異なり、`ts.SyntaxKind.AnyKeyword` が `undefined` になる。
- 事実: コミット済み生成物は存在し正常(`app/web/src/features/tasks/api/generated/` に `client.gen.ts` / `sdk.gen.ts` / `types.gen.ts` / `zod.gen.ts` 等)。build / typecheck はこれを使うため通る。
- 事実: `.github/workflows/contract-drift.yml` は `bun run generate` を実行して差分を検査する(Go + Bun 双方を要する唯一のジョブ、SPEC-003)。generate 自体が失敗するため、このジョブは Go DTO 変更時に fail する。
- 仮説: `@hey-api/openapi-ts 0.98.2` が同梱 / peer で解決する TypeScript のバージョン想定(5.x)と、app/web が SPEC-007 で入れた TS7 ネイティブ tsc の API 差分が原因。生成器が参照する `typescript` の実体が TS7 に解決されている可能性が高い(要確認)。

### 根本原因

SPEC-007(app/web を TypeScript 7 ネイティブ tsc へ移行)が、SPEC-003 の型生成器 `@hey-api/openapi-ts` の TypeScript コンパイラ API 依存を破壊した回帰。生成器が前提とする `ts.SyntaxKind` API が TS7 ネイティブコンパイラで変わり、生成処理が例外終了する。SPEC-007 の移行時に「生成器が TS7 で動くか」の検証がスコープに含まれていなかったことが根本にある(仮説を含む。移行経緯は SPEC-007 で要確認)。

## 4. 対応(どう解決するか)

### 対応方針

- **SPEC-003 の再生成ワークフローを復旧させる。** 以下のいずれか(または組み合わせ)を planner が評価して確定する:
  - **`@hey-api/openapi-ts` を TypeScript 7 対応版へ更新する**。TS7 ネイティブ tsc に対応した生成器バージョンがあればそれに上げる(最優先候補)。
  - **生成専用に互換 TypeScript を固定する**。生成器が解決する `typescript` を TS5.x 系に固定(生成時のみ devDependency / 別解決)し、アプリの型チェック / ビルドは TS7 ネイティブ tsc を使う、という「生成器と型チェッカでコンパイラを分ける」構成にする。SPEC-007 の TS7 ネイティブ化を維持しつつ生成を復旧できる。
  - 上記いずれも難しい場合、TS7 と互換のある別の生成器 / 生成方式への切り替えを検討する(最終手段。SPEC-003 の契約単一ソース方針は維持する)。
- 対応後は `.github/workflows/contract-drift.yml` が通ること(Go DTO を変えて再生成 → 差分ゼロ)を検証条件にする。
- 参照: `app/web/package.json`(`:13` generate / `:28` @hey-api/openapi-ts / `:39` typescript)、`app/web/openapi-ts.config.ts`、`app/web/src/features/tasks/api/generated/`、`.github/workflows/contract-drift.yml`、SPEC-003 / SPEC-007。

### 実施内容

- [ ] `@hey-api/openapi-ts` の TS7 対応状況を調査し、対応版があれば更新する(impl-web)
- [ ] 対応版が無い場合、生成専用に互換 TS を固定する構成(生成器と typecheck/build でコンパイラを分離)を検討・実装する(impl-web)
- [ ] `cd app/web && bun run generate` が正常終了し、生成物がコミット済みと一致することを確認する(checker / tester)
- [ ] `.github/workflows/contract-drift.yml` の drift 検査が通ることを確認する(Go DTO 変更 → 再生成 → 差分ゼロ)(impl-ci / tester)
- [ ] SPEC-007 側に「TS7 ネイティブ化は型生成器の互換に影響する」旨の追記が必要か admin と確認する

### 再発防止

- app/web の TypeScript / ビルドツールチェーンのメジャー更新時は、`bun run generate`(SPEC-003 の再生成)が通ることを移行チェックリストに含める。生成器が内部で TypeScript コンパイラ API に依存する場合、その互換性を移行スコープに明記する。
- 生成物が正常でも「再生成コマンド」が壊れる回帰は build / typecheck では検出できない。`bun run generate` の実行成否を CI(既存の contract-drift ジョブ)で担保していることを移行時の確認観点にする。

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。プロジェクト全体レビューで「`cd app/web && bun run generate` が `TypeError: undefined is not an object (evaluating 'ts.SyntaxKind.AnyKeyword')` で失敗する」と検証された事象を記録。SPEC-007(app/web の TypeScript 7 ネイティブ tsc 化)が SPEC-003(OpenAPI 契約からの web 型生成)の生成器 `@hey-api/openapi-ts` を壊した回帰。
- 事実確認: `app/web/package.json` は `typescript: ^7.0.2`(`:39`)・`@hey-api/openapi-ts: 0.98.2`(`:28`)、`generate` は `openapi-ts`(`:13`)。生成設定は `app/web/openapi-ts.config.ts`(input `../api/docs/openapi.yaml`、plugins: client-fetch / typescript / zod(compat 4)/ react-query)。コミット済み生成物(`src/features/tasks/api/generated/`)は正常で build / typecheck は通る = 壊れているのは「再生成」のみ。CI の `.github/workflows/contract-drift.yml` は `bun run generate` を実行して差分検査するため、Go DTO 変更時に必ず fail する(CI ブロッカー)。
- 重複確認: `docs/issues` を横断し、hey-api / openapi-ts の TS7 非互換・generate 失敗を扱う既存 Issue は無いことを確認。関連する既存 web/hey-api 系 Issue(ISSUE-011 生成 TanStack Query プラグイン出力の未使用、ISSUE-012 生成 client の SSE コードがバンドルに混入、ISSUE-013 hey-api 推移依存 js-yaml のビルド時脆弱性、ISSUE-007 Vitest beta ピン)はいずれも「生成物の内容 / 依存の中身 / テスト環境」の別テーマで、本件(TS7 での再生成コマンドの実行不能)とは異なる独立事象。
- severity は **high** と判定。判定根拠: SPEC-003 の中核ワークフロー(契約からの再生成)が常時失敗し回避策が無く、Go DTO 変更を含む PR で contract-drift CI が必ず fail する(= マージをブロックする主要開発フローの毀損)。ただしコミット済み生成物・build・typecheck・実行時アプリは正常で「主要機能そのものが使えない」(critical)には至らないため high とした。
- 相互リンク: 本 Issue frontmatter の `specs` に SPEC-003・SPEC-007 を設定。SPEC-003 / SPEC-007 側 frontmatter の `issues` への ISSUE-023 追記は、Spec 編集担当(admin / spec skill)への依頼が必要(issue-creator は `docs/issues` のみ編集する。本タスクでも docs/issues 以外は変更しない指示)。
- 次にやること: planner が復旧方針(生成器の TS7 対応版更新 / 生成専用の互換 TS 固定 / 生成方式の切替)を評価・計画化し、impl-web が実装、checker / tester で `bun run generate` と contract-drift CI の成功を検証する。

### 2026-07-10

- 修正完了(status: open → resolved)。対応方針のうち「生成器を TypeScript 7 対応版へ更新する」(最優先候補)を採用。impl-web が `@hey-api/openapi-ts` を `0.98.2` → `0.0.0-next-20260708192938`(TS7 対応の next プレリリース版)へ更新した。stable 系列(最新 `0.99.0` まで)は TS7 未対応であることを実機で確認したため、やむを得ず next 版を採用。TS7 移行(`typescript: ^7.0.2`、SPEC-007)は維持。
- 依存インストールゲート(`app/web/bunfig.toml` の `minimumReleaseAge`)は SPEC-007 と同じ **lock-then-restore** 手順で対応し、`bunfig.toml` は空配列(`minimumReleaseAgeExcludes = []`)のまま維持した(ゲートに個別の除外エントリを残さない)。
- 検証(checker が独立に実施・全 pass): `bun install --frozen-lockfile` / `bun run generate`(TS7 で成功・冪等) / `typecheck` / `lint` / `format:check` / `build` / `test`(9 files, 73 tests)。`bun run generate` が TS7 ネイティブ tsc 上で例外なく完走し冪等であることを確認したため、`.github/workflows/contract-drift.yml`(Go DTO 変更 → 再生成 → 差分検査)は通る見込み。
- 生成物差分の確認: 今回の更新で生じた差分は「インデント幅(4 → 2)」と「runtime で上書きされる `baseUrl` 既定値の付与」のみで、契約(`app/api/docs/openapi.yaml`)由来の型・エンドポイント・スキーマ内容には差分なし。生成器の互換復旧が目的どおり達成され、契約の意味に影響していないことを確認。
- 残課題(deferred): stable の `@hey-api/openapi-ts` が TS7 に対応したら `package.json` を stable 版へ戻す(意図は `app/web/openapi-ts.config.ts` にコメントで記載済み)。また CI は npm レジストリミラーがこの `0.0.0-next-20260708192938`(next プレリリース版)を解決できることが前提となる。この deferred は影響が限定的で追跡先が本 Issue の経緯で足りるため、新規 Issue は切らず本 Issue の経緯に残す方針とする。
- severity(high)は起票時のまま維持(修正済みのため実害は解消)。
</content>
</invoke>
