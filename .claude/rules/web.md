---
paths:
  - "app/web/**"
  - "app/auth-web/**"
---

# app/web / app/auth-web — TypeScript / React 規約

本規約は `app/web`(タスク UI)と `app/auth-web`(IdP 管理 UI。auth の admin API を操作)の両 TypeScript / React stack に適用する。コマンド・レイアウト・コーディング規約は共通で、本文の `app/web` は原則 `app/auth-web` にも読み替える。差分は 2 点のみ: `generate`(OpenAPI 型生成)は app/web だけが持ち、auth-web は admin API の型を zod スキーマで直接定義する。実行は各 stack 自身のディレクトリ / `Makefile` で行う。

## コマンド(app/web/Makefile 契約)

`app/web/Makefile`(auth-web は `app/auth-web/Makefile`)は必ず以下のターゲットを提供する。checker / tester はこれを実行する。**ホストで bun / go を直接実行しない**(SPEC-009: toolchain コンテナ内のみ。`make` が `docker compose run` へ委譲する)。

| 目的 | コマンド (`app/web` で実行) |
|---|---|
| 依存インストール | `make install` (`INSTALL_FLAGS=--frozen-lockfile` で CI 固定) |
| format(チェック) | `make format-check` |
| format(自動修正) | `make format` |
| lint | `make lint` |
| type check | `make typecheck` |
| test | `make test` (単一ファイルは `make test TEST=<path>`) |
| build | `make build` |
| 全チェック | `make check` (= format-check + lint + typecheck + test + build) |
| OpenAPI 契約消費・生成 | `make generate` (`../api/docs/openapi.yaml` から型 / Zod / TanStack Query を `src/features/tasks/api/generated` に生成。SPEC-003) |

ルートからは `make web-<target>` / `make auth-web-<target>` でも同じターゲットを呼べる(例: `make web-check` / `make auth-web-check`)。

`app/web/package.json` の scripts はコンテナ内で `bun run <script>` として実行される実体。scripts 名・内容は変更しない。**`dev`(Vite dev server)は Makefile 契約に含めない** — 画面の動作確認は compose の `web` / `auth-web` サービス(ルート `make up-d` → http://localhost:8080 / http://localhost:8083)で行う。ホストや toolchain コンテナで dev server を常駐させない(CLAUDE.md「実行境界: runtime はコンテナのみ」)。

package manager / runtime は **Bun**(コンテナ内)。依存導入は `make install`、スクリプト実行は上表の `make` ターゲット経由。バンドラは Vite(`build` = `tsc --noEmit && vite build`)、テストランナーは **Vitest + React Testing Library**。

lint / format は **Biome** 単一ツールで行う(`lint` = `biome lint`、`format` / `format:check` = `biome format`。設定は `biome.json`)。**ESLint / Prettier は使わない。**

型チェックは **tsc**(TypeScript 7.0 のネイティブコンパイラ。`typescript` パッケージに同梱)で行う(`typecheck` = `tsc --noEmit`、`build` も `tsc --noEmit && vite build`)。TypeScript 7.0 は Go 実装のネイティブコンパイラが stable 化されたもので、旧来の tsgo(`@typescript/native-preview` の日次プレビュー)はこの stable 化により不要になった(SPEC-007 で移行)。**ESLint / Prettier を使わないのと同様、型チェックは `tsc` に一本化する。**

## 依存インストール(サプライチェーン対策)

Shai-Hulud 型の npm サプライチェーン攻撃(公開直後に汚染されたバージョンを掴む)への防御として、`app/web/bunfig.toml` の `[install]` に **`minimumReleaseAge`(最小公開経過日数)= 21 日 = `1814400` 秒** を設定する。公開から 21 日未満のバージョンはインストール時に除外される。

- 意図的に最新を追う preview パッケージ(日次 dev ビルド等)を使う場合は `minimumReleaseAgeExcludes` に列挙して除外する(除外しないと更新が 21 日ブロックされる)。**現状、該当パッケージは無く `minimumReleaseAgeExcludes` は空**(SPEC-007 で tsgo=`@typescript/native-preview` を stable の `tsc` に置き換え、除外対象が消えた)
- 既に `bun.lock` に固定済みのバージョンはゲートを通過する(ロック済み依存は影響を受けない)。ゲートは新規追加・更新時に効く。したがって依存追加は「必要な版を確定してから」ゲートを効かせる運用にする
- 公開直後(21 日未満)の stable 版をどうしても前倒しで固定したい場合は **lock-then-restore**: 対象パッケージを一時的に `minimumReleaseAgeExcludes` に追加 → `bun install` で `bun.lock` に固定 → 除外を戻す。固定後はロック済みとしてゲートを通過する(SPEC-007 で `typescript@7.0.2` の導入に使用)。恒久的な除外(ゲートの無効化)にはしない

### runtime イメージのビルドにも同じゲートを効かせる

`bun install` は `make install`(toolchain コンテナ)だけでなく **`app/web/Dockerfile` / `app/auth-web/Dockerfile` の build ステージ**でも走る。ここが抜けるとリポジトリ内で唯一ゲート無しのインストール経路になるため、以下を必須とする(CLAUDE.md「実行境界: runtime はコンテナのみ」の Shai-Hulud 対策部と同一の契約)。

- **`bunfig.toml` を `bun install` より前に COPY する**: `COPY package.json bun.lock bunfig.toml ./`。`bunfig.toml` はソースではなく「解決に影響するファイル」の一員として扱う(これを忘れると、そのビルドだけが既定の公開レジストリ・ゲート無しで解決される)
- **`bun install --frozen-lockfile --ignore-scripts`**: lifecycle script(postinstall 等)は当該ワーム類の実行・流出経路そのもので、このビルドでは不要(ネイティブ依存は optionalDependencies のプリビルドバイナリで供給される)。Bun は既定でも lifecycle script を trusted パッケージに限定するが、`--ignore-scripts` は既定 trusted 集合まで塞ぐ
- **build context に依存ツリーと資格情報を入れない**: 各 stack 直下の `.dockerignore`(ルートの `.dockerignore` は per-stack context には効かない)で `node_modules` / `dist` / `.env*` を除外する。`node_modules` を通すと後段の `COPY . .` がゲート済みインストール結果をホストの木で上書きしてしまう。`~/.npmrc` / クラウド資格情報 / docker.sock / SSH agent はビルドに渡さない
- **runtime ステージに npm エコシステムを残さない**: 配信は nginx + `dist` の静的ファイルだけで、Node / Bun / パッケージマネージャを含めない。依存コードが動くのは build ステージのみに閉じる
- **ベースイメージは digest 固定**(SPEC-016): build ステージは `oven/bun:<version>@sha256:...`(version は `versions.env` の `BUN_VERSION` と揃える。旧 `oven/bun:1` は major 浮動でツールチェーンとの版ズレも生んでいた)、runtime ステージは `nginxinc/nginx-unprivileged:alpine@sha256:...`。更新手順は各 Dockerfile のコメント(`docker pull` → `docker inspect --format '{{index .RepoDigests 0}}'`)

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
