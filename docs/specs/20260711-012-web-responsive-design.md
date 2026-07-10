---
id: SPEC-012
title: app/web レスポンシブデザイン対応
status: done
created: 2026-07-11
updated: 2026-07-11
issues: []
supersedes: null
---

# SPEC-012: app/web レスポンシブデザイン対応

## 1. ユーザー価値(なぜ作るか)

> **Task Manager を使うユーザー** が **スマートフォン・タブレット・デスクトップのどの画面幅でも快適にタスクを閲覧・作成・操作できるようになり**、**デバイスを選ばず一貫した体験** を得る。

- **対象ユーザー**: Task Manager Web UI を利用するエンドユーザー(モバイル・タブレット・デスクトップ)
- **解決する課題**: 現状、一部コンポーネントはレスポンシブ対応済みだが、フォーム入力・詳細画面・サマリー等で狭い幅でのはみ出し・タッチ操作のしづらさ・テキスト切れのリスクが残っている。`.claude/rules/web.md` の「レスポンシブ / アダプティブデザイン」節が必須要件として定義されているが、全画面で未充足
- **得られる価値**: 320px〜1280px の代表幅で横スクロール・要素のはみ出し・重なり・切れがなく、タッチターゲットが十分な UI
- **価値の検証方法**: タスク一覧・作成フォーム・詳細画面が 320 / 375 / 768 / 1024 / 1280px で破綻なく表示されること。`make -C app/web check` が green。review-spec が `.claude/rules/web.md` の受け入れ基準を満たすと判定すること

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- モバイルユーザーとして、片手でタスク一覧を閲覧し、フィルタ・ページ送り・Start/Complete を快適に操作したい。なぜなら通勤中や会議の合間にスマホでタスクを確認・更新することが多いから。
- デスクトップユーザーとして、広い画面では情報が横に整理され、狭い画面では縦積みに自然にリフローするレイアウトを期待する。なぜなら同じ URL を PC とモバイルの両方で使うから。

### 利用フロー

1. ユーザーが任意のデバイス幅で Task Manager を開く(`index.html` の viewport meta は維持)
2. タスク一覧ページ: サマリー・作成フォーム・フィルタ・リスト・ページャが画面幅に応じて折り返し・縦積みし、横スクロールが発生しない
3. タスク詳細ページ: 長いタイトルが折り返し、操作ボタンがタッチしやすいサイズで表示される
4. ユーザーがフィルタ・ページ送り・Start/Complete・タスク作成を、デバイスに応じたタッチターゲットで操作する

## 3. 要件(何を満たすべきか)

### 機能要件

- [x] R1: 全画面(タスク一覧 `/`、タスク詳細 `/tasks/$taskId`)が `.claude/rules/web.md` の「レスポンシブ / アダプティブデザイン」節のルールに準拠する
- [x] R2: モバイルファーストの Tailwind ユーティリティでスタイルを表現する(固定 px 幅のレイアウト禁止、`max-w-*` + `mx-auto` + レスポンシブ padding)
- [x] R3: 横並び要素(タスクアイテムの本文とボタン群、ページャ、フィルタ)は狭い幅で縦積み / 折り返しする
- [x] R4: 操作要素(Button、フィルタボタン、フォーム入力)はタッチ幅で概ね 44×44px 以上の実効サイズを確保する(`pointer-coarse:min-h-11` 等)
- [x] R5: 長いテキスト(タスクタイトル等)は `break-words` / `min-w-0` で切れずに折り返す
- [x] R6: `index.html` の viewport meta(`width=device-width, initial-scale=1`)を維持する

### 非機能要件

- **アーキテクチャ維持**: feature-sliced 構成(`features/tasks/domain|api|hooks|components`、`shared/ui`)を変えない。ビジネスロジックは `domain/` に残し、変更は presentation 層(コンポーネント・App シェル)に限定する
- **バックエンド整合**: OpenAPI 契約・API 呼び出し・Zod スキーマ・TanStack Query hooks に変更を持ち込まない。`make generate` の再実行は不要(契約不変)
- **挙動不変**: ルーティング・URL search params・mutation の振る舞いは変えない。スタイル(className)のみの変更

### スコープ外(やらないこと)

- 新規 API エンドポイントやサーバーサイドフィルタ(SPEC-008 R-5 の既知制限の解消)
- ダークモード・テーマ切替
- 専用のモバイルナビゲーション(ハンバーガーメニュー等)の追加
- ビジュアルリデザイン(配色・タイポグラフィの全面刷新)
- E2E ブラウザテストの導入(代表幅の手動確認 + 既存 Vitest で十分)

## 4. 設計(どう実現するか)

### 方針

既存の feature-sliced アーキテクチャと OpenAPI 契約を維持し、presentation 層の Tailwind `className` のみを調整する。`.claude/rules/web.md` のルールを各コンポーネントへ適用し、部分対応済みの箇所(App シェル、TaskItem、TaskFilters、Button、TaskPager)は維持・強化し、未対応箇所(CreateTaskForm、TaskSummary、TaskDetailPage)を補完する。

### アーキテクチャ / データ / インターフェース

| 対象 | 変更内容 |
|---|---|
| `app/App.tsx` | 既存パターン維持(`max-w-3xl`, `px-4 sm:px-6`) |
| `TaskItem.tsx` | 既存の `flex-col sm:flex-row`, `min-w-0`, `break-words` 維持 |
| `TaskFilters.tsx` | 既存の `flex-wrap`, タッチターゲット維持 |
| `TaskPager.tsx` | 既存の `flex-wrap` 維持 |
| `Button.tsx` | 既存の `pointer-coarse:min-h-11`, `motion-reduce:` 維持 |
| `CreateTaskForm.tsx` | 入力・select にタッチターゲット追加。送信ボタンは狭い幅で `w-full sm:w-auto` |
| `TaskSummary.tsx` | `grid-cols-3` に `min-w-0` を各セルへ追加し、極小幅でもセル内テキストがはみ出さないようにする |
| `router.tsx` (TaskDetailPage) | 詳細タイトル `h1` に `break-words` を追加 |
| `index.html` | viewport meta 維持(変更なし想定) |

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| CSS メディアクエリを `index.css` に直書き | プロジェクト規約は Tailwind ユーティリティ一本。既存コンポーネントも `className` 方式 |
| 専用の `ResponsiveLayout` コンポーネント新設 | 現状 2 画面のみで App シェルで十分。過剰抽象化 |
| OpenAPI / API 層の変更 | レスポンシブは presentation のみ。バックエンド契約に触れる必要なし |

## 5. 実装計画

詳細は `docs/plans/SPEC-012-plan.md` を参照。変更は app/web の presentation 層のみで、`CreateTaskForm`(入力/送信ボタンのタッチターゲット)・`TaskSummary`(各セル `min-w-0`)・`router.tsx` の `TaskDetailPage`(`h1` の `break-words`)の 3 ファイルを補完し、既に対応済みの App シェル / TaskItem / TaskFilters / TaskPager / Button は維持する。既存 Vitest を無改変で保ち、レスポンシブ受け入れは代表幅の手動確認 + review-spec で検証する(`make generate` 再実行は不要)。

- [x] T1: planner が実装計画を作成
- [x] T2: impl-web が presentation 層の className を調整
- [x] T3: tester が既存テスト実行 + 必要なら presentation テスト追加
- [x] T4: checker が `make -C app/web check` を実行
- [x] T5: review-spec / review-security / review-performance がレビュー

## 6. 経緯(時系列・追記のみ)

### 2026-07-11

- 初版作成。`.claude/rules/web.md` のレスポンシブ必須要件を全画面で充足するための Spec。バックエンド契約・アーキテクチャは不変、presentation 層のみ変更する方針で approved。
- planner が実装計画を作成(`docs/plans/SPEC-012-plan.md`)。調査の結果、未充足は 3 コンポーネント(CreateTaskForm / TaskSummary / TaskDetailPage の `h1`)の局所的ギャップに集約されると確定。App シェル・TaskItem・TaskFilters・TaskPager・Button は既に対応済みで維持。変更は className のみ・挙動不変で、既存 Vitest を無改変の安全網とし、代表幅(320/375/768/1024/1280px)の手動確認 + review-spec で受け入れ、`make generate` 再実行は不要とする方針を確定。
- 実装完了。impl-web が 3 ファイルの className を調整。tester: 106 テスト全件 green(無改変)。checker: `make -C app/web check` green。review-spec: R1–R6 充足・Blocker/Major 0 件。review-security / review-performance: 問題なし。status を `done` に更新。
