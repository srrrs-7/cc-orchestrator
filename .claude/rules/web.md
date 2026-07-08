---
paths:
  - "app/web/**"
---

# app/web — TypeScript / React 規約

## コマンド(npm scripts 契約)

`app/web/package.json` は必ず以下の scripts を提供する。checker / tester はこれを実行する。

| 目的 | コマンド |
|---|---|
| format(チェック) | `pnpm run format:check` |
| format(自動修正) | `pnpm run format` |
| lint | `pnpm run lint` |
| type check | `pnpm run typecheck` |
| test | `pnpm run test` |
| build | `pnpm run build` |

package manager は pnpm。実行はすべて `app/web` ディレクトリで行う。

## TypeScript

- strict mode 前提。`any` 禁止(やむを得ない場合は `unknown` + 型ガードで絞り込む)
- `as` による強制キャストは境界(API レスポンスのパース等)のみ。内部ロジックでは使わない
- 外部から来るデータ(API・localStorage・URL params)はスキーマバリデーション(zod 等)を通してから型を付ける
- export は named export のみ。default export 禁止(ルーティング規約が要求する場合を除く)

## React

- function component + hooks のみ。class component 禁止
- server state(API 由来)と client state(UI 状態)を分離する。server state を useState に複製しない
- コンポーネントは feature 単位でディレクトリを切る(`features/<feature>/` 配下に components / hooks / api を同居)
- 副作用は hooks に隔離し、コンポーネントは表示に専念させる
- リスト レンダリングの key に配列 index を使わない(並び替え・削除がない静的リストを除く)
