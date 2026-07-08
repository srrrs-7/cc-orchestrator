---
paths:
  - "app/web/**"
---

# app/web — TypeScript / React 規約

## コマンド(package.json scripts 契約)

`app/web/package.json` は必ず以下の scripts を提供する。checker / tester はこれを実行する。

| 目的 | コマンド |
|---|---|
| format(チェック) | `bun run format:check` |
| format(自動修正) | `bun run format` |
| lint | `bun run lint` |
| type check | `bun run typecheck` |
| test | `bun run test` |
| build | `bun run build` |

package manager / runtime は **Bun**。依存導入は `bun install`、スクリプト実行は `bun run <name>`。バンドラは Vite(`bun run build` = `vite build`)、テストランナーは Bun 標準(`test` script の実体は `bun test`)。実行はすべて `app/web` ディレクトリで行う。

lint / format は **Biome** 単一ツールで行う(`lint` = `biome lint`、`format` / `format:check` = `biome format`。設定は `biome.json`)。**ESLint / Prettier は使わない。**

型チェックは **tsgo**(`@typescript/native-preview`、TypeScript ネイティブ移植)で行う(`typecheck` = `tsgo --noEmit`、`build` も `tsgo --noEmit && vite build`)。**`tsc` は使わない。**

## 依存インストール(サプライチェーン対策)

Shai-Hulud 型の npm サプライチェーン攻撃(公開直後に汚染されたバージョンを掴む)への防御として、`app/web/bunfig.toml` の `[install]` に **`minimumReleaseAge`(最小公開経過日数)= 21 日 = `1814400` 秒** を設定する。公開から 21 日未満のバージョンはインストール時に除外される。

- 意図的に最新を追う preview パッケージ(例: `@typescript/native-preview` = tsgo の日次ビルド)は `minimumReleaseAgeExcludes` に列挙して除外する(除外しないと更新が 21 日ブロックされる)
- 既に `bun.lock` に固定済みのバージョンはゲートを通過する(ロック済み依存は影響を受けない)。ゲートは新規追加・更新時に効く。したがって依存追加は「必要な版を確定してから」ゲートを効かせる運用にする

## TypeScript

- strict mode 前提。`any` 禁止(やむを得ない場合は `unknown` + 型ガードで絞り込む)
- `as` による強制キャストは境界(API レスポンスのパース等)のみ。内部ロジックでは使わない
- 外部から来るデータ(API・localStorage・URL params)はスキーマバリデーション(zod 等)を通してから型を付ける
- export は named export のみ。default export 禁止(ルーティング規約が要求する場合を除く)

## React

- function component + hooks のみ。class component 禁止
- server state(API 由来)と client state(UI 状態)を分離する。server state を useState に複製しない
- コンポーネントは feature 単位でディレクトリを切る(`features/<feature>/` 配下に domain / api / hooks / components を同居)
- **domain logic と component を分離する**: ビジネスルール(状態遷移・不変条件・派生値・フィルタ / ソート)は `features/<feature>/domain/` に React 非依存の純関数・純データとして置き、React / fetch / DOM を import しない。component と hooks はそれを呼ぶだけでロジックを内包しない。依存方向は一方向 `components → hooks → (api | domain)`、`domain` は何にも依存しない(app/api の DDD と同じく `domain` を最下層に置き、単体でテスト可能に保つ)
- 副作用は hooks に隔離し、コンポーネントは表示に専念させる
- リスト レンダリングの key に配列 index を使わない(並び替え・削除がない静的リストを除く)
