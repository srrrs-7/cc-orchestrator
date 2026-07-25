# 開発ワークフロー

ドキュメントの形式は skill が単一の情報源として管理する。テンプレートをここに重複させない:

- **機能仕様(Spec)**: `docs/specs` — `spec` skill(SPEC-NNN、固定テンプレート)
- **不具合・課題(Issue)**: `docs/issues` — `issue` skill(ISSUE-NNN、固定テンプレート)

## パイプライン

機能開発は Spec を、不具合対応は Issue を起点とする。admin(メインセッションの Claude。役割と強制事項は `orchestration.md` 参照)は各フェーズを対応する subagent に委譲する。

**Spec 起点 / Issue 起点の判定基準**: 新しい HTTP エンドポイント・新しいドメイン集約・確定済み公開契約(env 契約 / OpenAPI 契約 / ドメインポート / DB スキーマ)の追加・変更を伴うものは **Spec 必須**。既存挙動の不具合修正・内部改善は Issue でよい。ロードマップ plan が機能群を Issue 分割だけで着手指示している場合でも、着手前に Spec 化を先行させる。

```
機能開発: spec skill で Spec を作成(status: approved にしてから着手)
不具合  : issue-creator agent で Issue を起票
  → 1. planner  : 実装計画を docs/plans に作成し、起点の Spec / Issue に反映
  → 2. tester   : 要件からテストを先に作成(TDD。計画で後付けを指定した場合は 3 の後)
  → 3. impl-web / impl-api / impl-iac : 実装(scope が独立していれば並列可)
  → 4. tester   : テスト実行・不足テストの追加
  → 5. checker  : format / lint / type check
  → 6. review-security / review-performance / review-spec : レビュー(並列)
  → 7. 指摘対応  : Blocker / Major は impl agent に差し戻し、4→6 を再実行。
                  今回対応しない指摘は issue-creator が Issue として起票する
  → 8. 起点の Spec / Issue のステータスと経緯を skill の手順に従って更新し、完了
```

- フェーズを飛ばさない。特に 5(checker)が通らない状態で 6(レビュー)に進まない
- レビュー agent はコードを変更しない。修正は必ず impl agent が行う
- Spec / Issue を更新するときは必ず各 skill の更新手順(経緯セクションへの追記・frontmatter の status / updated 更新・過去エントリの編集禁止)に従う

## リファクタリング(挙動不変の内部改善)

機能追加・バグ修正を伴わない、コード内部品質(重複・命名・関数の粒度・可読性・凝集度)の改善は refactor agent が担当する。起点は Spec / Issue に限らず、技術的負債・コードの臭い・レビューの純粋なクリーンアップ指摘など随時。**外部から観測できる挙動と公開契約を変えないこと**が絶対条件で、既存テストがその安全網となる。

```
リファクタリング:
  → 0. スコープ確定 : admin が対象 stack / 範囲と狙いを決めて refactor に割り振る
  → 1. ベースライン : checker / tester で対象が緑であることを確認(特性化。テストが手薄なら tester が先に補う)
  → 2. refactor    : 挙動不変の内部改善を適用(既存テストは変更しない)
  → 3. tester      : 同一テストが無改変で緑のままかを確認。落ちたら挙動変更の証拠として refactor に差し戻す
  → 4. checker     : format / lint / type check
  → 5. review-*    : 必要に応じ(特に review-spec)挙動・契約の非変更を確認
```

- 不変条件: HTTP API / DTO / OpenAPI 契約・ドメインポートのシグネチャ・env 契約・DB スキーマ / マイグレーション / sqlc の結果を変えない。変えたくなったら refactor を止め、Spec / Issue を起こして該当 impl agent に回す
- **テストを変えて通すのは禁止**(手順 2 の refactor と 3 の tester を分離するのが安全網)。リファクタでテストが落ちたら、テストではなく変更を戻す
- 大きなリファクタ(モジュール構成の変更・広範な再配置)は planner で計画し、Spec / Issue に記録する。stack / モジュール境界を越える共通化はアーキテクチャ変更として Spec 化する

## 維持作業(依存 bump / main 取り込みマージ)

dependabot の依存更新の取り込みや feature ブランチへの main 取り込みマージなど、Spec / Issue を起点としない反復的な維持作業は次の軽量フローで行う:

```
維持作業:
  → 1. admin   : ユーザーが指示・実行した git 操作(merge 等)を完了させる
                 (orchestration.md のホワイトリスト参照)
  → 2. impl-*  : 競合が app/ 配下に及ぶ場合、admin が解消方針を決めて指示し、
                 編集はファイル所有 stack の impl agent に委譲する(独立していれば並列可)
  → 3. checker : 影響 stack の make check
  → 4. admin   : merge commit(手順 1 の git 操作の完了として)
```

- ドメインロジックの実質的な競合(コードの意味が衝突している場合)のみ planner / review-* を挟む
- Go スタックは `make check` が build + test を含むため、tester を別途挟まなくてよい

## Plan(実装計画)

- ファイル名: `docs/plans/<ID>-plan.md`(例: `SPEC-001-plan.md`、`ISSUE-003-plan.md`)
- 起点ドキュメント側(Spec の「5. 実装計画」/ Issue の「対応方針」)にはタスクの要約と plan ファイルへの参照を書き、詳細は plan ファイルに書く
- 必須セクション:
  - **方針**: 採用するアプローチと、退けた代替案の理由
  - **変更ファイル**: stack ごとの追加・変更ファイル一覧
  - **手順**: どの agent が何をどの順で行うか(並列可能な箇所を明示)
  - **テスト戦略**: 先行作成(TDD)か後付けか、何をどのレベルでテストするか
  - **リスク / 未確定事項**
