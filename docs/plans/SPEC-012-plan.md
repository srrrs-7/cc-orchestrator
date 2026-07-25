# SPEC-012 実装計画: app/web レスポンシブデザイン対応

- 起点: `docs/specs/20260711-012-web-responsive-design.md`(SPEC-012, status: approved)
- 種別: presentation 層のみの変更(挙動・契約は不変)
- 関連 rules: `.claude/rules/web.md`(「レスポンシブ / アダプティブデザイン」節が受け入れ基準の正)/ `.claude/rules/workflow.md`
- 対象 stack: **app/web のみ**(api / auth / iac / db は対象外)

---

## 方針

既存の feature-sliced 構成(`features/tasks/{domain,api,hooks,components}` / `shared/ui`)と OpenAPI 契約・TanStack Query hooks・Zod スキーマ・ルーティングを **一切変えず**、presentation 層の Tailwind `className` のみを調整して `.claude/rules/web.md` のレスポンシブ必須要件を全画面(`/`・`/tasks/$taskId`)で充足する。

調査の結果、レスポンシブ対応は **すでに大半が実装済み**で、未充足は 3 コンポーネントの局所的なギャップに集約される:

- **維持(変更不要)**:
  - `app/App.tsx` — `max-w-3xl` + `mx-auto` + `px-4 sm:px-6`(App シェルの規範パターン)。`TaskListPage` は `flex flex-col gap-6` で縦積み。
  - `TaskItem.tsx` — `flex-col sm:flex-row`、本文側 `min-w-0` + タイトル `break-words`、ボタン群 `shrink-0 flex-wrap`。R3 / R5 を既に満たす。
  - `TaskFilters.tsx` — `flex-wrap` + 選択肢に `pointer-coarse:min-h-11 pointer-coarse:px-4`。R3 / R4 充足。
  - `TaskPager.tsx` — `flex-wrap items-center justify-between`。R3 充足。
  - `Button.tsx` — `pointer-coarse:min-h-11 pointer-coarse:px-4` + `motion-reduce:transition-none` + `disabled:cursor-not-allowed`。R4 / ユーザー設定尊重を既に満たす。
  - `index.html` — viewport meta(`width=device-width, initial-scale=1.0`)を既に持つ。**変更しない**(R6 は維持のみ)。
- **補完(変更する)**:
  - `CreateTaskForm.tsx` — `<input>` / `<select>` にタッチターゲット高さが無い(現状 `px-2 py-1` のみ)。送信 Button は `self-start` 固定で狭幅でも幅が縮む。→ 入力要素に `pointer-coarse:min-h-11`、送信ボタンを狭幅 `w-full` / `sm:` 以上で従来どおり(R2 / R4)。
  - `TaskSummary.tsx` — `grid grid-cols-3` が常に 3 列で、各セルに `min-w-0` が無いため 320px + 桁数増大時にセル内容がグリッドトラックを押し広げ横あふれを招き得る。→ 各セルに `min-w-0`(R2 / 横スクロール禁止)。
  - `router.tsx`(`TaskDetailPage`)— 詳細タイトル `h1` に `break-words`(および器側の `min-w-0`)が無く、長い連続文字列が横あふれし得る。→ `break-words` を追加(R5)。

### 各要件 → 対応箇所の対応表

| 要件 | 対応 |
|---|---|
| R1(全画面が web.md 準拠) | 下記 3 コンポーネント補完 + 既存 5 箇所維持で `/` と `/tasks/$taskId` を充足 |
| R2(モバイルファースト / 固定幅禁止 / `max-w-*`+`mx-auto`+レスポンシブ padding) | App シェルの既存パターン維持。新規に固定 px を導入しない。フォーム送信ボタンの `w-full sm:w-auto` |
| R3(横並びは狭幅で縦積み / 折り返し) | TaskItem / TaskFilters / TaskPager の既存 `flex-col sm:flex-row` / `flex-wrap` を維持 |
| R4(タッチターゲット 44×44px 目安) | Button / TaskFilters の既存 `pointer-coarse:min-h-11` 維持 + CreateTaskForm の input / select に付与 |
| R5(長文の `break-words` / `min-w-0`) | TaskItem 既存維持 + TaskDetailPage の `h1` に追加 |
| R6(viewport meta 維持) | `index.html` 変更なし。checker / review が存置を確認 |

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| `TaskSummary` の `grid-cols-3` を `grid-cols-1 sm:grid-cols-3` にリフロー | Spec §4 は 3 列維持 + `min-w-0` を指定。3 セル・短い数値なので 320px でも `min-w-0` で破綻せず、縦積みは情報密度を落とすだけ。最小変更を優先 |
| `index.css` にメディアクエリを直書き | プロジェクト規約は Tailwind ユーティリティ一本(web.md)。既存コンポーネントも `className` 方式 |
| 専用 `ResponsiveLayout` / モバイルナビ新設 | 2 画面のみで App シェルで十分(Spec スコープ外) |
| className の存在をテストで固定 | Tailwind クラス名アサートは脆く実効レイアウトを保証しない。挙動不変は既存の振る舞いテストで担保し、レスポンシブは代表幅の手動確認 + review-spec で検証(Spec §3 非機能・「E2E 導入はスコープ外」に整合) |

---

## 変更ファイル(stack ごと)

### app/web(presentation 層のみ)

**変更**

- `src/features/tasks/components/CreateTaskForm.tsx`
  - `<input id="title">` の className に `pointer-coarse:min-h-11` を追加(タッチ幅で高さ確保、R4)。
  - `<select id="priority">` の className に同上を追加(R4)。
  - 送信 `<Button className="self-start">` を狭幅で全幅・広幅で従来の左寄せに:`className="w-full sm:w-auto sm:self-start"`(R2 / R4)。フォーム自身は既に `flex flex-col gap-3` + `w-full` 入力なので追加のコンテナ変更は不要。
- `src/features/tasks/components/TaskSummary.tsx`
  - `<dl>` の 3 つの子 `<div>`(各セル)に `min-w-0` を追加し、`grid-cols-3` トラックが内容で押し広げられて横あふれするのを防ぐ(R2 / 横スクロール禁止)。`grid grid-cols-3 gap-2 text-center` の枠は維持。
- `src/app/router.tsx`(`TaskDetailPage`)
  - 詳細タイトル `<h1 className="text-xl font-semibold">` に `break-words` を追加(長い連続文字列の折り返し、R5)。必要なら親 `div`(`flex flex-col gap-4`)に `min-w-0` を付け、flex 子の縮小を許可する。
  - ロジック(`useParams` / `useTaskQuery` / 分岐)は一切変更しない。

**維持(変更しないが受け入れ確認の対象)**

- `src/app/App.tsx` / `src/features/tasks/components/{TaskItem,TaskFilters,TaskPager}.tsx` / `src/shared/ui/Button.tsx` / `index.html`

**変更なし(触れてはいけない)**

- `src/features/tasks/{domain,api,hooks}/**`、`src/features/tasks/api/generated/**`、`src/lib/**`、`src/mocks/**`、`openapi-ts.config.ts`、`../api/docs/openapi.yaml`。**`make generate` は再実行不要**(契約不変)。

### 他 stack

- app/api / app/auth / app/iac / app/migrator / DB / CI ワークフロー: **変更なし**。

---

## 手順(担当 agent・順序・並列可否)

> フェーズ間は原則直列。今回は単一 stack かつ変更が小さいため並列化の余地は限定的。

- **T2 / Phase 0(ベースライン)** — **tester**
  - `make -C app/web check`(format-check + lint + typecheck + test + build)が現状 green であることを確認し、以降の「挙動不変」の安全網を固定する。

- **T3 / Phase 1(実装)** — **impl-web**(単独)
  1. `CreateTaskForm.tsx` の input / select にタッチターゲット、送信ボタンに `w-full sm:w-auto sm:self-start` を適用。
  2. `TaskSummary.tsx` の各セルに `min-w-0` を追加。
  3. `router.tsx` の `TaskDetailPage` `h1` に `break-words`(必要なら親に `min-w-0`)を追加。
  4. 320 / 375 / 768 / 1024 / 1280px の代表幅で、横スクロール・はみ出し・重なり・切れが無いことをローカル(`make -C app/web build` 済みプレビュー等)で目視確認し、結果を報告に含める。
  - 3 ファイルは互いに独立なので順不同で可(同一 agent が一括で実施)。

- **T4 / Phase 2(テスト)** — **tester**(Phase 1 後)
  - 既存 Vitest(`CreateTaskForm.test.tsx` / `TaskItem.test.tsx` / `TaskPager.test.tsx` / `TaskList.test.tsx` / `router.test.tsx` 他)が **無改変で green** のままかを確認(className 変更のみなので role / label / 挙動は不変)。
  - 新規テストは原則不要(下記テスト戦略)。もし追加するなら「入力要素・送信ボタン・詳細 `h1` が期待どおり render される」程度の最小限に留め、Tailwind クラス名の直接アサートはしない。

- **T5 / Phase 3(チェック)** — **checker**(Phase 2 後)
  - `make -C app/web check` を実行。**green になるまでレビューに進まない。**

- **T6 / Phase 4(レビュー)** — `[並列]` **review-spec** / **review-security** / **review-performance**
  - review-spec: web.md の「レスポンシブ / アダプティブデザイン」節への準拠(固定幅なし・オーバーフローなし・狭幅で縦積み・タッチターゲット・`break-words`)、および **OpenAPI / API / hooks / domain 不変**・viewport meta 存置を確認。
  - review-security / review-performance: className のみの変更のため影響は軽微。バンドル・再レンダリングへの副作用が無いことを確認。

- **T7 / 指摘対応** — Blocker / Major は **impl-web** に差し戻し、Phase 2→4 を再実行。今回対応しない指摘は **issue-creator** が Issue 化。

- **T8(完了判定)** — admin
  - Spec §1「価値の検証方法」3 条件(代表幅で破綻なし / `make -C app/web check` green / review-spec 合格)を満たしたら Spec を `done` に更新。

---

## テスト戦略

- **方式**: 後付け型・かつ **既存テストを安全網として維持**する。変更は className(presentation)のみで、コンポーネントの role / label / 挙動・ルーティング・mutation は不変のため、既存の振る舞いテストがそのまま「挙動不変」を保証する。
- **レベル別**:
  - domain / hooks / api: 変更なし。既存テスト(`domain/*.test.ts` / `useTasks.test.tsx` / `api/*.test.ts`)は無改変で green のまま。
  - component(RTL + Vitest): 既存の `CreateTaskForm` / `TaskItem` / `TaskPager` / `TaskList` / `router` テストが無改変で green であることを確認。className 変更が role(`button` / `textbox` / `combobox`)や `getByLabelText` を壊さないことがそのまま回帰チェックになる。
  - レスポンシブ表示そのもの(横スクロール・はみ出し・重なり): 自動テストの対象外。**代表幅(320/375/768/1024/1280px)の手動目視** + review-spec の受け入れ確認でカバー(Spec §3「E2E 導入はスコープ外」に整合)。jsdom はレイアウトを計算しないため Vitest では検証できない。
- **新規テストの方針**: 原則追加しない(「minimal new tests」)。追加する場合も Tailwind クラス名の直接アサートは脆いため避け、render 可否・アクセシブルな role/label の存在確認に留める。
- **要件 → カバレッジ対応**:
  - R1〜R5: 実装 + 代表幅手動確認 + review-spec(web.md 準拠判定)。
  - R6(viewport meta 維持): `index.html` 差分ゼロを checker / review が確認。
  - 挙動不変(§3 非機能): 既存 Vitest 群が無改変で green であること。

---

## リスク / 未確定事項

1. **`TaskSummary` の 3 列維持の妥当性**: 320px で 3 セル + `gap-2` + `p-3` は内側テキスト領域が狭い。件数が 3 桁以上になると `text-xl` が窮屈になり得る。`min-w-0` で横あふれ(ページ横スクロール)は防げるが、セル内での視認性が落ちる可能性がある。→ Spec §4 は 3 列維持を指定しているため本計画は踏襲する。手動確認で明確に破綻する場合は `grid-cols-1 sm:grid-cols-3`(縦積みフォールバック)を impl-web が代替案として提示し、admin / ユーザー判断を仰ぐ。
2. **`w-full sm:w-auto` と `Button` 既存クラスの合成**: `Button` は `className` を末尾結合するため `w-full sm:w-auto sm:self-start` は問題なく効くはずだが、`px`/`min-h` の pointer-coarse ユーティリティとの併用で意図せぬ幅にならないか、impl-web が実機幅で確認する。
3. **`TaskDetailPage` の `h1` 折り返し器**: `break-words` は連続文字列(URL 様のトークン)には `break-all` 相当が必要な場合がある。日本語・空白入りタイトルは `break-words` で十分だが、極端な連続 ASCII で切れない場合は `[overflow-wrap:anywhere]` の追加を検討(手動確認で判断)。
4. **手動確認の再現性**: レスポンシブ受け入れは目視依存で、確認環境(ブラウザ / DevTools のデバイスエミュレーション)に依存する。E2E 導入はスコープ外のため、impl-web は確認した幅・ブラウザを報告に明記し、review-spec が同条件で追認できるようにする。
5. **`app/web` のスタイル基盤前提**: `pointer-coarse:` / `motion-reduce:` バリアントは Tailwind v4 の標準バリアント。既存 Button / TaskFilters で使用実績があるため利用可能と判断。ビルド警告が出ないことを checker が確認する。

---

## Spec 反映(planner 実施済み)

- `docs/specs/20260711-012-web-responsive-design.md` §5 の T1 にチェックを入れ、本 plan(`docs/plans/SPEC-012-plan.md`)への参照を明記。
- §6 経緯に「planner が実装計画を作成。変更は app/web の presentation 層のみ(CreateTaskForm / TaskSummary / router:TaskDetailPage の 3 ファイル補完 + 既存 5 箇所維持)、既存 Vitest を無改変で維持し代表幅の手動確認 + review-spec で受け入れ、`make generate` 再実行不要という方針を確定」を追記。
- 更新は spec skill の手順(経緯追記・frontmatter `updated`・過去エントリ不編集)に従う。
