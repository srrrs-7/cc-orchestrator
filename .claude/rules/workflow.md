# 開発ワークフロー

ドキュメントの形式は skill が単一の情報源として管理する。テンプレートをここに重複させない:

- **機能仕様(Spec)**: `docs/specs` — `spec` skill(SPEC-NNN、固定テンプレート)
- **不具合・課題(Issue)**: `docs/issues` — `issue` skill(ISSUE-NNN、固定テンプレート)

## パイプライン

機能開発は Spec を、不具合対応は Issue を起点とする。orchestrator(メインの Claude)は各フェーズを対応する subagent に委譲する。

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

## Plan(実装計画)

- ファイル名: `docs/plans/<ID>-plan.md`(例: `SPEC-001-plan.md`、`ISSUE-003-plan.md`)
- 起点ドキュメント側(Spec の「5. 実装計画」/ Issue の「対応方針」)にはタスクの要約と plan ファイルへの参照を書き、詳細は plan ファイルに書く
- 必須セクション:
  - **方針**: 採用するアプローチと、退けた代替案の理由
  - **変更ファイル**: stack ごとの追加・変更ファイル一覧
  - **手順**: どの agent が何をどの順で行うか(並列可能な箇所を明示)
  - **テスト戦略**: 先行作成(TDD)か後付けか、何をどのレベルでテストするか
  - **リスク / 未確定事項**
