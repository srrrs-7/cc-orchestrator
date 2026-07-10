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
| OpenAPI 契約消費・生成 | `bun run generate`(`../api/docs/openapi.yaml` から型 / Zod / TanStack Query を `src/features/tasks/api/generated` に生成。SPEC-003) |

package manager / runtime は **Bun**。依存導入は `bun install`、スクリプト実行は `bun run <name>`。バンドラは Vite(`bun run build` = `tsc --noEmit && vite build`)、テストランナーは **Vitest + React Testing Library**(`test` script の実体は `vitest run`。RTL / jsdom / MSW と組み合わせるため Bun 標準ランナーではなく Vitest を採用)。実行はすべて `app/web` ディレクトリで行う。

lint / format は **Biome** 単一ツールで行う(`lint` = `biome lint`、`format` / `format:check` = `biome format`。設定は `biome.json`)。**ESLint / Prettier は使わない。**

型チェックは **tsc**(TypeScript 7.0 のネイティブコンパイラ。`typescript` パッケージに同梱)で行う(`typecheck` = `tsc --noEmit`、`build` も `tsc --noEmit && vite build`)。TypeScript 7.0 は Go 実装のネイティブコンパイラが stable 化されたもので、旧来の tsgo(`@typescript/native-preview` の日次プレビュー)はこの stable 化により不要になった(SPEC-007 で移行)。**ESLint / Prettier を使わないのと同様、型チェックは `tsc` に一本化する。**

## 依存インストール(サプライチェーン対策)

Shai-Hulud 型の npm サプライチェーン攻撃(公開直後に汚染されたバージョンを掴む)への防御として、`app/web/bunfig.toml` の `[install]` に **`minimumReleaseAge`(最小公開経過日数)= 21 日 = `1814400` 秒** を設定する。公開から 21 日未満のバージョンはインストール時に除外される。

- 意図的に最新を追う preview パッケージ(日次 dev ビルド等)を使う場合は `minimumReleaseAgeExcludes` に列挙して除外する(除外しないと更新が 21 日ブロックされる)。**現状、該当パッケージは無く `minimumReleaseAgeExcludes` は空**(SPEC-007 で tsgo=`@typescript/native-preview` を stable の `tsc` に置き換え、除外対象が消えた)
- 既に `bun.lock` に固定済みのバージョンはゲートを通過する(ロック済み依存は影響を受けない)。ゲートは新規追加・更新時に効く。したがって依存追加は「必要な版を確定してから」ゲートを効かせる運用にする
- 公開直後(21 日未満)の stable 版をどうしても前倒しで固定したい場合は **lock-then-restore**: 対象パッケージを一時的に `minimumReleaseAgeExcludes` に追加 → `bun install` で `bun.lock` に固定 → 除外を戻す。固定後はロック済みとしてゲートを通過する(SPEC-007 で `typescript@7.0.2` の導入に使用)。恒久的な除外(ゲートの無効化)にはしない

## TypeScript

- strict mode 前提。`any` 禁止(やむを得ない場合は `unknown` + 型ガードで絞り込む)
- `as` による強制キャストは境界(API レスポンスのパース等)のみ。内部ロジックでは使わない
- 外部から来るデータ(API・localStorage・URL params)はスキーマバリデーション(zod 等)を通してから型を付ける
- export は named export のみ。default export 禁止(ルーティング規約やビルドツールの設定ファイル(`vite.config.ts` / `vitest.config.ts`)が要求する場合を除く)

## React

- function component + hooks のみ。class component 禁止
- server state(API 由来)と client state(UI 状態)を分離する。server state を useState に複製しない
- コンポーネントは feature 単位でディレクトリを切る(`features/<feature>/` 配下に domain / api / hooks / components を同居)
- **domain logic と component を分離する**: ビジネスルール(状態遷移・不変条件・派生値・フィルタ / ソート)は `features/<feature>/domain/` に React 非依存の純関数・純データとして置き、React / fetch / DOM を import しない。component と hooks はそれを呼ぶだけでロジックを内包しない。依存方向は一方向 `components → hooks → (api | domain)`、`domain` は何にも依存しない(app/api の DDD と同じく `domain` を最下層に置き、単体でテスト可能に保つ)
- 副作用は hooks に隔離し、コンポーネントは表示に専念させる
- リスト レンダリングの key に配列 index を使わない(並び替え・削除がない静的リストを除く)

## レスポンシブ / アダプティブデザイン

UI は **どのディスプレイ幅でもデザインが崩れない** ことを必須要件とする(Google / web.dev のレスポンシブ Web デザインのベストプラクティスに準拠)。新規コンポーネントは最初からこの原則で書き、既存コンポーネントを変更するときも満たす。スタイルは Tailwind CSS v4(`className` のユーティリティ)で表現する。

- **モバイルファースト**: プレフィックスなしのユーティリティを最小幅(〜320px)向けの基準にし、`sm:` / `md:` / `lg:` で広い幅に上書きする。max-width 起点のデスクトップファーストで書かない
- **横スクロールを絶対に出さない**: ページ本体(body)が横スクロールする状態を作らない。320px 幅でも内容は切れずに折り返す。折り返しに `flex-wrap`、はみ出し得るテキストを含む flex 子要素に `min-w-0` + `truncate` / `break-words`、テーブルや `pre` など本質的に広い要素は自身を `overflow-x-auto` の器で包み、その器の中だけスクロールさせる(ページはスクロールさせない)
- **固定幅を使わない**: レイアウトコンテナは `max-w-*` + `mx-auto` + レスポンシブ padding(`px-4 sm:px-6`)で作る(App シェルの既存パターン)。`w-[320px]` のような固定 px をレイアウトに使わない。画像・メディアは `max-w-full h-auto`
- **ブレークポイント間も連続的に破綻させない**: できる限り本質的に流動的なレイアウト(`flex-wrap`、`grid-cols-[repeat(auto-fill,minmax(16rem,1fr))]`、`clamp()`)を使い、リフローが必要な箇所にだけブレークポイントを置く。ブレークポイントは特定デバイス幅ではなくコンテンツ基準で決める
- **横並びは狭い幅で積む**: 広い幅で横並びの要素(ボタン群・フィルタ・フォーム項目)は、狭い幅で折り返す / 縦積みにする(`flex-col sm:flex-row`、`flex-wrap`)。潰れ・はみ出しを作らない
- **タッチターゲット**: 操作要素はタッチ幅で概ね 44×44px 以上の実効サイズを確保する(`min-h` / padding で調整)
- **可読性**: テキストは 1 行の文字数(measure)を制限し(`max-w-prose` 等)、固定サイズでのはみ出しを避け、折り返しを許可する
- **ユーザー設定の尊重**: transition / animation は `motion-reduce:` で `prefers-reduced-motion` を尊重し、`focus-visible` の可視状態を残す

**受け入れ基準**: 変更した画面は少なくとも 320 / 375 / 768 / 1024 / 1280px の代表幅で、横スクロール・要素のはみ出し・重なり・切れが無いことを確認する。`index.html` の viewport meta(`width=device-width, initial-scale=1`)は前提として常に維持する。review-* はこの節への違反(固定幅・オーバーフロー・狭幅での破綻)を指摘する。
