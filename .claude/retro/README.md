# 振り返り(retro)— orchestration 自己改善ループ

`.claude/`(agents / rules / skills / CLAUDE.md)の**精度を、実行中に遭遇した摩擦の記録から継続的に上げる**ための仕組み。
開発対象(product: app/*)の不具合は `docs/issues` に、**orchestration 自体の課題はここ**に記録する。

## なぜ必要か

multi-agent の質は `.claude/` の定義(役割分担・ルール・コマンド契約・報告形式)の質で決まる。定義の穴は「実際にタスクを回して初めて」わかる ── ルールが曖昧で agent が迷う、コマンド表と Makefile がずれる、担当範囲が不明で作業が宙に浮く、報告に必要な項目が無い、等。これらを**その場で捨てず記録として溜め、横断分析して定義に還元する**ことで、`.claude/` を経験から改善し続ける。

## ループ

```
1. 実行        subagent / skill がタスクを回す
2. 記録        admin が報告を検収する際に .claude 自体の摩擦を吸い上げ、/retro で entry を記録
3. 蓄積        entries/ に 1 件 1 ファイルで溜まる(並列 subagent でも衝突しない)
4. 統括        retro-synthesizer agent が entry を横断分析 → target/phase 別にクラスタ化
               → 頻度 × 深刻度で優先度付け → .claude/ の具体的な変更提案レポートを reports/ に出す
5. 適用        admin が提案レポートを検収し、.claude/(agents/rules/skills/CLAUDE.md)に適用
6. クローズ    適用できた entry を /retro で addressed にし、synthesis に RETROSUM-NNN を記入
   → 1 に戻る(改善された .claude で次のタスクを回す)
```

役割の分離は本プロジェクトの他のパイプラインと同型:

- **記録(`/retro` skill)** = entry の形式の単一情報源。事実の記録のみ(review が指摘するだけなのと同じ)
- **統括(`retro-synthesizer` agent)** = 分析と**提案のみ**。`.claude/` も entry も直接書き換えない(review agent がコードを変えないのと同じ)
- **適用(admin)** = 提案を検収して `.claude/` を実際に変更する(`.claude/` の整備は admin のホワイトリスト。`orchestration.md` 参照)

## ディレクトリ構成

```
.claude/retro/
  README.md            # このファイル。ループ全体の単一情報源
  entries/             # 個別の摩擦記録(1 件 1 ファイル)
    YYYYMMDD-NNN-<slug>.md      # RETRO-NNN。形式は /retro skill が正
  reports/             # retro-synthesizer の統括レポート
    YYYYMMDD-NNN-<slug>.md      # RETROSUM-NNN。形式は下記
```

- entry の形式(frontmatter / セクション)の正は [`.claude/skills/retro/template.md`](../skills/retro/template.md)。作成・更新手順は [`.claude/skills/retro/SKILL.md`](../skills/retro/SKILL.md)
- 採番は各ディレクトリ内のファイル名連番 NNN の最大値 +1(specs / issues と同じ規約)
- 経緯は追記のみ・過去エントリは編集しない(specs / issues と同じ)

## 記録は誰がするか

**基本は admin。** admin は各 subagent の報告を検収する立場にあり、orchestration の摩擦(どのルールが曖昧で agent が誤解したか、どのコマンドが間違っていたか、どの担当範囲が抜けていたか)を最もよく観測できる。admin は検収の一部として摩擦を `/retro` で記録する(`orchestration.md` の「admin の行動規範」参照)。

- subagent の報告に摩擦が表れていれば admin がそれを拾って記録する
- Write 権限を持たない agent(review-* 等)の摩擦も admin 経由で取りこぼさない
- 1 件でも「次の同種タスクで同じ所で詰まりそう」と思えば記録する価値がある。溜まった頻度が統括での優先度になる

## 何を記録するか / しないか

| 記録する(retro) | 記録しない |
|---|---|
| ルール(`.claude/rules/*`)の曖昧さ・欠落・矛盾で agent が迷った | product(app/*)のバグ・仕様課題 → **`docs/issues`(`/issue`)** |
| コマンド表と実際の Makefile / package.json のズレ | 一時的な環境障害・ネットワーク断など `.claude` と無関係な事象 |
| agent の担当範囲・割り振り表の穴、agent 新設の必要 | 特定タスク限りの些末な判断(再発しないもの) |
| 報告形式に足りない項目、モデル割り当ての不適 | 既に同一 target で記録済みの重複(既存 entry に追記する) |
| skill テンプレート・手順の不備 | |

判断に迷ったら:「`.claude/` を変えれば直るか、`app/` を変えれば直るか」。前者が retro、後者が issue。

## 統括レポート(RETROSUM-NNN)の形式

`retro-synthesizer` agent が `reports/YYYYMMDD-NNN-<slug>.md` に作成する:

```markdown
---
id: RETROSUM-NNN
title: <統括の一行要約>
date: YYYY-MM-DD
status: proposed  # proposed(提案済み)| applied(admin が .claude/ に適用済み)
entries: [RETRO-001, RETRO-004, ...]  # このレポートで扱った entry
---

# RETROSUM-NNN: <統括の一行要約>

## サマリ
対象 entry 数 / 主要クラスタ / 最優先で直すべき .claude アセット(3 行以内)。

## クラスタ分析
target / phase 別にまとめる。各クラスタで:
- パターン(共通する摩擦)/ 該当 entry(RETRO-NNN)/ 頻度 / 深刻度
- 根本原因(なぜ .claude の現状の記述だとこの摩擦が起きるか)

## 改善提案(優先度順)
各提案:
- 優先度 / 対象アセット(ファイル)/ 現状(該当記述の引用)/ 変更内容(具体的に)/ 期待効果 / 対応 RETRO-NNN

## 適用結果(admin が適用後に記入)
- 適用したコミット / 見送った提案とその理由 / addressed にした entry
```

## admin 向けチェックリスト

- [ ] タスクの検収時、`.claude/` に起因する摩擦があれば `/retro` で記録したか
- [ ] entry がある程度溜まった / 定期見直しのタイミングで `retro-synthesizer` を起動したか
- [ ] 統括レポートの提案を検収し、妥当なものを `.claude/` に適用したか
- [ ] 適用できた entry を `addressed` にし、レポートを `applied` にしたか
