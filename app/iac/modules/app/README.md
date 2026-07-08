# app module

ALB(Target Group / Listener / カスタムヘッダ検証ルール)、ECR リポジトリ、ECS
クラスタ/タスク定義/サービス、IAM ロール、CloudWatch Logs をまとめて作成する。

## コスト上の選択理由

### Fargate(ARM64/Graviton)+ Fargate Spot 併用を採用し EC2 起動タイプを退けた理由

- ECS の EC2 起動タイプは、コンテナ以外にホスト EC2 インスタンスの管理(パッチ適用・
  スケーリング・キャパシティプランニング)という運用負荷がかかる。小規模なサンプルでは
  Fargate のサーバーレス運用のメリットがコスト差を上回る
- Fargate は ARM64(Graviton)の方が x86_64 より vCPU/メモリ単価が 約20% 安い。
  `runtime_platform.cpu_architecture = "ARM64"` を採用しているため、ECR に push する
  イメージは **必ず linux/arm64 でビルド**する必要がある(README 注記)
- dev 環境では `use_fargate_spot = true`(既定)とし、`capacity_provider_strategy` で
  `FARGATE`(on-demand, 既定 weight=0/base=0)と `FARGATE_SPOT`(既定 weight=1)を
  併用する。既定値では **タスクは実質すべて Spot 容量で起動**し、on-demand Fargate 比で
  最大 70% 程度のコスト削減が見込める(R5)。可用性を優先したい場合は
  `fargate_base` を 1 以上に設定し、最低 1 タスクを on-demand で確保できる

#### トレードオフ: 既定値は単一障害点(SPOF)であることを明示

- 既定値(`fargate_base = 0`, `fargate_weight = 0`, `fargate_spot_weight = 1`,
  `desired_count = 1`)を組み合わせると、実質 **100% Spot・タスク数 1** の構成になる。
  これは意図的な設定であり、dev サンプルではコスト最小化を可用性より優先する方針による
- この既定構成では、AWS が Spot 容量を回収(interrupt)した瞬間に稼働中の唯一のタスクが
  失われ、後続タスクが起動するまで API が全断する。つまり **既定値は単一障害点(SPOF)を
  許容する設計**であり、可用性が必要な用途にそのまま使うべきではない
- 単一障害点を解消する選択肢(いずれか、または両方を組み合わせる):
  - `fargate_base = 1` に設定し、最低 1 タスクを on-demand(Spot 回収の影響を受けない)で
    確保する。on-demand 1 タスク分の増分コストは概算 約$6/月(ARM64, 256/512 の場合)
  - `desired_count = 2` 以上に設定し、Spot タスクが同時に複数稼働するようにする(Spot でも
    同時に全タスクが回収される確率は下がるが、ゼロにはならない点に注意)
  - 本番相当の可用性が必要な環境では、上記に加えて `fargate_base = 1` かつ
    `desired_count >= 2` の組み合わせを推奨する
- タスクサイズは既定 `task_cpu=256`(0.25 vCPU)/ `task_memory=512`(0.5GiB)の最小構成。
  実際のワークロードに応じて `envs/dev` の tfvars で調整する

### カスタムヘッダ検証(fixed-response 403 + forward ルール)の仕組み

- ALB の HTTP リスナーの **default action は `fixed-response` で 403** を返す
- CloudFront が生成するカスタムオリジンヘッダ(`origin_verify_header_name` /
  `origin_verify_header_value`。値は `envs/dev` で `random_password` により 1 度だけ
  生成され、`app` と `cdn` の両モジュールへ変数として配布される)が一致するリクエストのみ、
  Listener ルールの `condition.http_header` にマッチし Target Group へ forward される
- これにより、`network` モジュールのプレフィックスリスト SG(IP レベルの制限)に加えて
  アプリケーション層でも CloudFront 経由であることを検証する **二重防御**になる(R3)。
  プレフィックスリストは「CloudFront 全体の送信元 IP 帯」であり別ディストリビューション
  からのアクセスも通過し得るため、ヘッダ検証がこれを補完する

### CloudFront ⇔ ALB 間が HTTP である点のトレードオフ

- 本サンプルはカスタムドメイン / ACM 証明書を使わない(Spec スコープ外)ため、ALB の
  リスナーは HTTP(80)のみで HTTPS 化していない
- CloudFront → ALB 間の通信は AWS のバックボーンネットワーク内を通るが、TLS 終端は
  CloudFront までであり、ALB までの区間は暗号化されない。カスタムヘッダの値も
  この区間では平文で流れる。ACM 証明書とカスタムドメインを用意できる環境では、
  ALB に HTTPS リスナーを追加し `cdn` モジュールの `custom_origin_config` を
  `https-only` に変更することを推奨する

### ヘルスチェックに `GET /tasks` を使う理由

- `app/api` には現状専用のヘルスチェックエンドポイント(`/healthz` 等)が存在せず、
  `route/router.go` のルートは `/tasks` 系のみ(ISSUE-001 参照)
- ALB Target Group のヘルスチェックは暫定で `GET /tasks`(matcher=200)を使用する。
  `/tasks` は一覧取得 API のため、通常は認証なしで 200 を返す想定だが、将来
  認証やレート制限が追加されると健全性判定に影響する可能性がある
- 将来 `app/api` に `/healthz` を追加した際は、この変数(`health_check_path`)を
  切り替えるだけでよい(モジュールインターフェースは変更不要)

### IAM ロールを実行ロール/タスクロールに分離した理由

- 実行ロール(`task_execution`)は ECS エージェントが image pull・ログ送信・
  シークレット取得を行うために必要な `AmazonECSTaskExecutionRolePolicy` に加え、
  DB マスターシークレットのみを読める最小権限のインラインポリシーを付与
- タスクロール(`task`)はアプリケーションが実行時に引き受けるロールで、現状
  `app/api` はランタイムで AWS API を呼ばない(in-memory リポジトリ)ため、
  権限を一切付与しない最小権限構成としている。将来 S3 等を使う場合はここに追加する
