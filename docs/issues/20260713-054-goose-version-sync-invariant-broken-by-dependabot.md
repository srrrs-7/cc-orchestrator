---
id: ISSUE-054
title: dependabot の goose bump で versions.env の GOOSE_VERSION(CLI 版)と app/migrator/go.mod(library 版)の同期 invariant が破れた
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-13
updated: 2026-07-13
specs: [SPEC-009]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-054: dependabot の goose bump で versions.env の GOOSE_VERSION(CLI 版)と app/migrator/go.mod(library 版)の同期 invariant が破れた

## 1. ユーザー価値への影響(なぜ対応するか)

> **開発者・運用者** の **マイグレーション scaffold 生成(`make migrate-create`)の一貫性** が **goose CLI(v3.24.1)と適用ライブラリ(v3.27.2)の版乖離により将来的に損なわれるリスクがある**。

- **影響を受けるユーザー**: 開発者・運用者(エンドユーザーへの直接影響はない)
- **損なわれる価値**: `GOOSE_VERSION` は toolchain イメージに `go install` される goose **CLI** の版で、用途は `make migrate-create`(マイグレーションファイルの scaffold 生成。DB 接続なし)のみ。ランタイムのマイグレーション**適用**は `app/migrator` が **library** 版(v3.27.2)を使うため、本番のマイグレーション動作には影響しない。ただし CLI(scaffold を生成する側)と library(scaffold を解釈・適用する側)の版が乖離すると、生成される scaffold の形式と適用側の解釈がずれるリスクが将来的にある。
- **影響範囲・頻度**: 常時(現在すでに乖離状態。dependabot が `app/migrator/go.mod` の goose を bump するたびに構造的に再発する)
- **回避策**: あり(`versions.env` の `GOOSE_VERSION` を手動で `go.mod` の require に合わせて bump する)

## 2. 現象(何が起きているか)

### 期待する動作

goose の CLI 版(`.devcontainer/versions.env` の `GOOSE_VERSION`)と library 版(`app/migrator/go.mod` の `github.com/pressly/goose/v3` require)が常に一致している。`versions.env` 自身のヘッダコメント(76〜80 行)がこの同期 invariant を明記している:

```
# Do not bump SQLC_VERSION/GOOSE_VERSION here without also checking
# .claude/rules/db.md's "版" section and app/migrator/go.mod's
# github.com/pressly/goose/v3 require (goose is a *library* dependency
# there, unlike the CLI usage everywhere else -- the two must stay in
# sync per that module's own Makefile comment).
```

### 実際の動作

library 版だけが bump され、CLI 版が取り残されて 2 者が乖離している(いずれも検証済みの事実):

- `app/migrator/go.mod`(7 行): `github.com/pressly/goose/v3 v3.27.2`(library 版)
- `.devcontainer/versions.env`(82 行): `GOOSE_VERSION=v3.24.1`(CLI 版)
- `.claude/rules/db.md`(45 行、「版」セクション): `goose v3.24.1`(同期対象として明記されているが同じく取り残されている)

同じ invariant を参照する `app/auth/Makefile`(123〜124 行)・`app/api/Makefile`(114 行付近)のコメントも `versions.env` の `GOOSE_VERSION` を単一ソースとして参照しており、CLI 版が v3.24.1 のまま。

### 再現手順

1. `app/migrator/go.mod` の `github.com/pressly/goose/v3` require の版を確認する → `v3.27.2`
2. `.devcontainer/versions.env` の `GOOSE_VERSION` を確認する → `v3.24.1`
3. `.claude/rules/db.md` の「版」セクション(45 行)の goose 版を確認する → `v3.24.1`
4. 3 者(library 版・CLI 版・db.md 記載)が一致していないことを確認する(現状 library 版のみ v3.27.2、他 2 者は v3.24.1)

### 環境・条件

- ブランチ: `feat/auth-oidc-foundation`
- dependabot PR #4 相当の bump(`app/migrator/go.mod` の goose を v3.24.1 → v3.27.2)が、origin/main の取り込みマージ `85d6e7e` を経て本ブランチに入った。bump 本体のコミットは `86abeb2`(`deps: bump the migrator-go-modules group in /app/migrator with 2 updates`)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 【事実】`app/migrator/go.mod:7` の goose require は `v3.27.2`(`grep` で確認)。
- 【事実】`.devcontainer/versions.env:82` の `GOOSE_VERSION` は `v3.24.1`(`grep` で確認)。
- 【事実】`.devcontainer/versions.env:76-80` に「`GOOSE_VERSION` を bump するときは `db.md` の『版』セクションと `app/migrator/go.mod` の goose require の両方を確認して同期を保て」と invariant が明記されている。
- 【事実】`.claude/rules/db.md:45` の「版」セクションも `goose v3.24.1` を記載し、「`app/migrator/go.mod` の require が単一の情報源。Makefile の `GOOSE_VERSION` もこれと同じ値に保つ」と述べているが、値が古いまま(この文書自体も同期漏れの対象になっている)。
- 【事実】dependabot は `app/migrator/go.mod`(go modules)のみを bump 対象とし、`versions.env` の `GOOSE_VERSION` 文字列や `db.md` の記述は更新しない。今回の乖離はこの構造の実例。
- 【仮説】この invariant は「人手で 3 箇所(go.mod・versions.env・db.md)を同時に更新する」ことに依存しており、dependabot のような自動 bump 経路では原理的に守られない。CI 等の自動検査が無いため、乖離が検出されずに取り込まれた。

### 根本原因

goose の版が 3 箇所(`app/migrator/go.mod` の library require / `.devcontainer/versions.env` の CLI 版 `GOOSE_VERSION` / `.claude/rules/db.md` の「版」記述)に分散し、同期はコメントで宣言された invariant のみに依存している。dependabot は go.mod しか更新しないため、この invariant は自動 bump 経路で構造的に破れる。乖離を検出する自動検査(CI・pre-commit・dependabot 設定)が存在しない。

## 4. 対応(どう解決するか)

### 対応方針

短期(乖離の解消)と恒久(再発防止)の 2 段で対応する。恒久策は本 Issue では方針提示にとどめ、実施可否は admin / ユーザーの判断に委ねる。

1. **短期**: impl-ci が `.devcontainer/versions.env` の `GOOSE_VERSION` を `v3.27.2`(= go.mod の require)に bump し、`.claude/rules/db.md` の「版」セクションの goose 版も `v3.27.2` に更新する。bump 手順は `versions.env` ヘッダコメント(76〜80 行)に従う。変更後は toolchain イメージを再ビルドする(`GOOSE_VERSION` は Dockerfile の `go install` で消費されるため)。
   - 注: 同期対象は当初想定の「versions.env と go.mod」の 2 者に加え、`.claude/rules/db.md:45` の「版」記述も含む(invariant コメント自身が db.md を同期対象として挙げている)。3 者すべてを揃える。
2. **恒久(検討)**: dependabot が `app/migrator/go.mod` の goose を bump するたびに同じ乖離が再発する。CI で「`app/migrator/go.mod` の `github.com/pressly/goose/v3` require と `.devcontainer/versions.env` の `GOOSE_VERSION` の一致」を検査する仕組み、または dependabot 側の設定(該当グループの ignore / grouping / post-update hook 等)で構造的に同期を担保する案を検討する。SPEC-009(版の一元化)のスコープに関わるため、恒久策の採否は SPEC-009 側の設計判断と合わせて決める。

### 実施内容

- [ ] `.devcontainer/versions.env` の `GOOSE_VERSION` を `v3.27.2` に bump(impl-ci)
- [ ] `.claude/rules/db.md`「版」セクションの goose 版を `v3.27.2` に更新(admin。`.claude/` メタ作業)
- [ ] toolchain イメージ再ビルド後、`make migrate-create` が想定どおり動くことを検証(tester / impl-ci)
- [ ] 恒久策(CI 一致検査 or dependabot 設定)の要否・具体案を検討(未定)

### 再発防止

- 恒久策として、goose 版の CLI ⇄ library ⇄ db.md 記述の一致を機械的に検査する仕組みを検討する(sqlc も同型の invariant を持つため、`SQLC_VERSION` を含めた汎用の版一致検査に一般化できる可能性がある。仮説)。設計・所在は SPEC-009(版の一元化)と整合させる。

## 5. 経緯(時系列・追記のみ)

### 2026-07-13

- 起票。dependabot PR #4 相当の goose bump(`app/migrator/go.mod` の `github.com/pressly/goose/v3` を v3.24.1 → v3.27.2、bump commit `86abeb2`)が origin/main 取り込みマージ `85d6e7e` で `feat/auth-oidc-foundation` に入った際、`.devcontainer/versions.env` の `GOOSE_VERSION`(v3.24.1)と `.claude/rules/db.md`「版」セクション(v3.24.1)が取り残され、`versions.env` ヘッダコメントが明記する同期 invariant が破れていることを検証で確認した。直接のユーザー影響は軽微(CLI 版は `make migrate-create` の scaffold 生成専用でランタイム適用は library 版を使う)だが、CLI と library の版乖離は将来的に scaffold 形式のずれリスクがあり、dependabot 経由で構造的に再発するため起票。SPEC-009(版の一元化)と相互リンク。
