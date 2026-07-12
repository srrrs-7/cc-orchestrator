---
id: ISSUE-007
title: app/web の vitest を beta(5.0.0-beta.6)にピン留めして回避している(Rolldown-vite + vitest4 の zod export バグ)
status: closed  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-08
updated: 2026-07-08
specs: []  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-007: app/web の vitest を beta(5.0.0-beta.6)にピン留めして回避している(Rolldown-vite + vitest4 の zod export バグ)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/web の開発者** の **依存管理の健全性(サプライチェーン衛生・保守性)** が **beta 版ツールへのピン留めによって一時的に損なわれている**。

- **影響を受けるユーザー**: app/web の開発者・CI(依存の追加/更新・クリーンインストールを行う者)。エンドユーザーへの影響はなし(本番ビルドは無影響、後述)
- **損なわれる価値**: テスト基盤の保守性とサプライチェーン衛生。安定版ではなく beta 版(`vitest@5.0.0-beta.6`)を devDependency にピン留めしている状態。加えて `bunfig.toml` の `minimumReleaseAge`(21日)ゲートを回避している側面がある
- **影響範囲・頻度**: 現状は顕在化していない(`bun.lock` に固定済みのため既存環境ではテスト・ビルドとも全て緑)。将来のクリーンインストール時、および安定版 vitest への追随時に対応が必要になる
- **回避策**: あり(`vitest@5.0.0-beta.6` にピン留め。全テスト green、build / typecheck / lint / format も緑)

## 2. 現象(何が起きているか)

### 期待する動作

- app/web のテストランナーとして安定版 `vitest@^4.1.10`(4系の最新安定)を使い、`import { z } from "zod"` が正しく解決され、テストが実行できること。

### 実際の動作

- `vitest@4.1.10` をこのリポジトリの構成で使うと、Vitest の runner 経由で `zod` を import した際に **`z` named export だけが黙って欠落**する(`import { z } from "zod"` が `undefined` になる)。zod の他の約 240 の export は `export *` 経由で正常に解決される。結果、zod を使うテストが動かない。
- `vitest@5.0.0-beta.6`(現時点で入手可能な最新。stable 5.0.0 は未公開)に上げると解消し、全テストが green になる。

### 再現手順

1. `app/web` で devDependency を `vitest@4.1.10` に設定して `bun install` する。
2. `import { z } from "zod"`(または `import("zod")` の named export 参照)を含むテスト/コードを Vitest の runner 経由で実行する。
3. `z` が `undefined` になり、zod を使うテストが失敗する。

最小再現による切り分け(事実):

- Vitest の runner 経由の `import("zod")` → `hasZ: false`(`z` が欠落)。
- Bun ネイティブ resolver での import → `hasZ: true`(正常)。
- 素の `vite.createServer().ssrLoadModule("zod")` → `hasZ: true`(正常)。
- → 欠落するのは Vitest 4.1.10 の runner 経由のときだけ。Bun や zod 側の問題ではなく、Vitest 4.1.10 + この Rolldown-vite 構成に固有の module-transform バグと確認できる。
- `test.server.deps.external: [/zod/]` を設定しても直らなかった(`app/web/vitest.config.ts` に該当設定は現状入れていない)。

### 環境・条件

- `app/web/package.json`: `vite: "^8.1.3"`(実体 8.1.3)、`zod: "^4.4.3"`、`vitest: "5.0.0-beta.6"`(回避後のピン。バグ再現版は `4.1.10`)。
- このプロジェクトの `vite@8.1.3` はコアのバンドラ/トランスフォームに **Rolldown** を使用(dependencies、optional ではない = rolldown-vite 構成)。
- runtime / package manager は Bun。`app/web/bunfig.toml` に `minimumReleaseAge = 1814400`(21日)。
- テストランナー設定は `app/web/vitest.config.ts`(production build の `vite.config.ts` とは分離)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 確認したこと・わかったこと(事実):
  - 発生源は zod のエントリモジュール(`node_modules/zod/index.js` の `export { z }`)を Vitest の SSR module runner が rolldown-vite 上で変換する際に、`z` named export のみを落とすこと。
  - 上記「再現手順」の3点の切り分けにより、原因は Vitest 4.1.10 の runner の module-transform に局在すると確定(Bun ネイティブ resolver / 素の vite SSR ローディングでは再現しない)。
  - `test.server.deps.external: [/zod/]` による external 化では回避できなかった。
  - `vitest@5.0.0-beta.6` では解消する。
- 仮説: `z` だけが選択的に落ちる Vitest 内部の正確な機序(なぜ `export { z }` の名前付き再エクスポートのみ欠落し `export *` は通るのか)は未特定。beta で解消することから、Vitest 5 系で該当の変換ロジックが修正されたものと推測する(仮説)。

### 根本原因

- 直接原因(事実): Vitest 4.1.10 の SSR module runner が rolldown-vite 構成下で zod エントリモジュールの `z` named export を変換時に脱落させる、Vitest 側の module-transform バグ。
- 内部機序の詳細(仮説): 上記のとおり未特定。今回は深追いの調査対象外とし、安定版のリリースで解消される見込みとして扱う。

## 4. 対応(どう解決するか)

### 対応方針

- 今回は恒久対応を行わず、**回避策の維持と監視のみ**を行う。
- `vitest@5.0.0-beta.6` へのピン留めで現状は全て緑。安定版 `vitest@5.x`(または 4.x の修正版)がリリースされたら追随し、beta ピンを解消して再テストする。

### 実施内容

- [x] 回避策として `app/web/package.json` の `vitest` を `5.0.0-beta.6` にピン留め(全テスト green、build / typecheck / lint / format も緑)。
- [ ] 安定版 `vitest@5.x`(または 4.x の修正版)のリリースを監視する。
- [ ] 安定版リリース後、`vitest` を安定版へ更新して beta ピンを解消し、全テスト・build を再検証する。
- [ ] 更新後、`minimumReleaseAge`(21日)ゲート下でもクリーンインストールが通ることを確認する。

### 再発防止

- 依存追加・更新時は「必要な版を確定してからゲートを効かせる」運用(`app/web/.claude/rules` の web.md / bunfig ポリシー)に従う。
- 補足(サプライチェーン衛生): `vitest@5.0.0-beta.6` は公開から 21 日未満のため、`bunfig.toml` の `minimumReleaseAge`(21日)ゲート下ではクリーンインストール時に弾かれうる。現状は `bun.lock` に固定済みのためゲートを通過している(ロック済み依存はゲートの影響を受けない)。安定版化・21日経過で解消見込み。

## 5. 経緯(時系列・追記のみ)

### 2026-07-08

- 起票。app/web のテスト導入時に `vitest@^4.1.10` がこのリポジトリの rolldown-vite(`vite@8.1.3`、Rolldown をコアに使用)構成で壊れることが判明したため、追跡課題(技術的負債)として記録。
- 事象: Vitest 4.1.10 の SSR module runner が rolldown-vite 上で zod エントリモジュールの `z` named export のみを黙って落とし、`import { z } from "zod"` が `undefined` になる。他の約 240 export は `export *` 経由で正常。
- 切り分け: Vitest runner 経由の `import("zod")` は `hasZ: false`、Bun ネイティブ resolver と素の `vite.createServer().ssrLoadModule("zod")` は `hasZ: true`。→ Vitest 4.1.10 + この Rolldown-vite 固有の module-transform バグと確認(Bun / zod 側の問題ではない)。`test.server.deps.external: [/zod/]` では直らなかった。
- 回避: `vitest@5.0.0-beta.6`(現時点で入手可能な最新。stable 5.0.0 は未公開)にピン留めして解消。全テスト green、build / typecheck / lint / format も緑。
- 影響範囲メモ: バグは Vitest の runner のみに影響し、`vite build` / `vite dev` の SSR ローディングは無影響(本番ビルドは正常)。
- severity 判定: `low`。判定根拠 — 動作する回避策があり、テスト・ビルドとも全て緑で機能的劣化はなく、エンドユーザー影響なし。残存リスクは開発側の保守・サプライチェーン衛生(beta 依存 + クリーンインストール時のゲート)に限られるため。
- 次アクション: 安定版 `vitest@5.x`(または 4.x 修正版)のリリースを監視し、出たらアップグレードして beta ピンを解消・再テストする(今回は深追い調査せず監視のみ)。

### 2026-07-12

- **closed(回避策完了・上流待ち)**。2026-07-12 時点で npm の最新 vitest は依然 `5.0.0-beta.6`(stable 5.0.0 未公開)。Vitest 4.1.10 + rolldown-vite(`vite@8.1.3`)構成での `z` export 欠落バグは再現条件として有効なまま。
- 回避策(`vitest@5.0.0-beta.6` ピン、`make -C app/web check` green)は維持。beta 除去は stable 5.x リリース後に新規 Issue または Dependabot で追随する。
- 本 Issue のスコープ(回避策の記録と監視)は達成。stable 追随は別サイクル。
