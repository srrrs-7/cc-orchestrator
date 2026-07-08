# app/api — DDD サンプル実装(タスク管理)

エリック・エヴァンス『Domain-Driven Design』のレイヤ化アーキテクチャと戦術的パターンを、
タスク管理(Task 集約)を題材に Go(標準ライブラリのみ)で実装したサンプル。

## レイヤ構成

```
                 ┌───────────┐
                 │  route    │  プレゼンテーション層 (HTTP ハンドラ / ルーティング)
                 └─────┬─────┘
                       │ 依存
                 ┌─────▼─────┐
                 │  service  │  アプリケーション層 (ユースケース)
                 └─────┬─────┘
                       │ 依存
                 ┌─────▼─────┐
                 │  domain   │  ドメイン層 (Task 集約・値オブジェクト・ドメインサービス)
                 └─────▲─────┘
                       │ 実装 (依存性逆転)
                 ┌─────┴─────┐
                 │  infra    │  インフラ層 (リポジトリ実装)
                 └───────────┘
```

- 依存の向きは `route → service → domain` の一方向。`domain` は他のどの層にも依存しない。
- `domain` はデータの永続化方法を知らない。代わりに `domain/task/repository.go` で
  `Repository` インターフェースを定義し、`infra/memory` がそれを実装する
  (**依存性逆転の原則 / DIP**)。図の矢印が domain → infra ではなく infra → domain
  (実装が interface に依存する)方向になっているのはこのため。
- `cmd/api` はコンポジションルート。各層の実装を組み立てて配線するだけで、
  ビジネスロジックを一切持たない。

## 戦術的パターンと本実装の対応

| パターン | 説明 | 本実装での対応 |
|---|---|---|
| エンティティ (Entity) | 同一性(ID)によって区別され、ライフサイクルを通じて可変な状態を持つオブジェクト | `domain/task/task.go` の `Task` |
| 値オブジェクト (Value Object) | 属性の値そのもので同一性が決まる、不変なオブジェクト | `domain/task/id.go` の `ID`、`title.go` の `Title`、`status.go` の `Status` |
| 集約 (Aggregate) / 集約ルート (Aggregate Root) | 不変条件を守る単位としてまとめられたオブジェクト群。外部からは集約ルート経由でのみ操作する | `Task` が集約ルート。フィールドはすべて非公開で、`Start()` / `Complete()` / `Rename()` などの振る舞いメソッドを通してのみ状態が変わる |
| ファクトリ (Factory) | 複雑な生成ロジック・不変条件の充足をカプセル化する生成手段 | `task.New(title)`(新規生成、必ず `StatusTodo` から開始)、`task.Reconstruct(...)`(永続化層からの再構築専用) |
| リポジトリ (Repository) | 集約の永続化・再構築をコレクションのように抽象化するインターフェース | `domain/task/repository.go` の `Repository`(定義はドメイン層、実装は `infra/memory`) |
| ドメインサービス (Domain Service) | 単一のエンティティ・値オブジェクトに自然に属さない、複数集約にまたがる知識・処理 | `domain/task/service.go` の `DuplicateChecker`(タイトル重複の判定は特定の `Task` 一つに属さない知識のためドメインサービスに配置) |
| アプリケーションサービス (Application Service) | ユースケースを実現するために、ドメインオブジェクトを協調させる薄い層。ビジネスルール自体は持たない | `service/task_service.go` の `TaskService`(`Create` / `Get` / `List` / `Start` / `Complete`) |

その他の設計判断:

- ドメインエラーは sentinel error(`ErrNotFound` など)とカスタム型(`*TransitionError`)で表現し、
  呼び出し側は `errors.Is` / `errors.As` で判定する(`domain/task/errors.go`)。
- アプリケーション層はドメインオブジェクトを直接返さず、`TaskDTO` に変換してから返す
  (`service/dto.go`)。これにより `domain` の内部表現がプレゼンテーション層に漏れない。
- `route/response.go` にドメインエラー → HTTP ステータスの変換を集約し、
  ハンドラごとの分岐ロジックの重複を避けている。

## ディレクトリ

```
app/api/
├── cmd/api/main.go          コンポジションルート(配線のみ)
├── domain/task/             ドメイン層(他層に非依存)
│   ├── id.go                値オブジェクト ID
│   ├── title.go              値オブジェクト Title
│   ├── status.go             値オブジェクト Status
│   ├── task.go                集約ルート Task
│   ├── errors.go              ドメインエラー
│   ├── repository.go          Repository インターフェース
│   └── service.go             ドメインサービス DuplicateChecker
├── service/                 アプリケーション層
│   ├── dto.go                 TaskDTO と変換関数
│   └── task_service.go        ユースケース(Create/Get/List/Start/Complete)
├── infra/memory/             インフラ層
│   └── task_repository.go     in-memory Repository 実装
└── route/                    プレゼンテーション層
    ├── router.go               ルーティング定義
    ├── task_handler.go         HTTP ハンドラ
    └── response.go             JSON ヘルパー・エラー変換
```

## 開発コマンド

汎用コマンドは `Makefile` にまとめている(`make help` で一覧表示)。

| ターゲット | 内容 |
|---|---|
| `make fmt` | フォーマット自動修正(gofmt + goimports) |
| `make fmt-check` | フォーマット差分チェック |
| `make lint` | golangci-lint |
| `make vet` / `make build` | go vet / go build |
| `make test` / `make test-race` | テスト実行(race detector 付きは test-race) |
| `make check` | 上記チェックを一括実行(fmt-check + lint + vet + build + test) |
| `make run` | API サーバー起動 |

## 起動方法

```sh
cd app/api
make run  # または go run ./cmd/api
```

デフォルトでは `:8080` で待ち受ける。ポートを変えたい場合は `PORT` 環境変数を指定する。

```sh
PORT=9000 go run ./cmd/api
```

`Ctrl+C`(SIGINT)または SIGTERM で graceful shutdown する。

## API 一覧

| メソッド | パス | 説明 | 成功時ステータス |
|---|---|---|---|
| POST | `/tasks` | タスクを作成する | 201 |
| GET | `/tasks` | タスク一覧を取得する | 200 |
| GET | `/tasks/{id}` | タスクを 1 件取得する | 200 |
| POST | `/tasks/{id}/start` | タスクを todo → doing に遷移する | 200 |
| POST | `/tasks/{id}/complete` | タスクを doing → done に遷移する | 200 |

主なエラーレスポンス:

| ステータス | 条件 |
|---|---|
| 400 | タイトルが空 / 100 文字超過、ID が不正、リクエストボディが不正な JSON |
| 404 | 指定した ID のタスクが存在しない |
| 409 | タイトルが重複している / 許可されていない状態遷移(例: todo → done) |
| 500 | 上記以外の予期しないエラー(詳細はレスポンスに含めずサーバーログに出力) |

### curl 例

```sh
# 作成
curl -s -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"write report"}'

# 一覧取得
curl -s http://localhost:8080/tasks

# 1件取得
curl -s http://localhost:8080/tasks/<id>

# 開始 (todo -> doing)
curl -s -X POST http://localhost:8080/tasks/<id>/start

# 完了 (doing -> done)
curl -s -X POST http://localhost:8080/tasks/<id>/complete
```
