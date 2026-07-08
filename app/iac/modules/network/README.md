# network module

VPC / サブネット / ルートテーブル / セキュリティグループ(ALB・ECS・RDS の 3 種)/ CloudFront
origin-facing マネージドプレフィックスリストの参照をまとめて作成する。

## コスト上の選択理由

### NAT Gateway を使わない

- NAT Gateway は 1 台あたり固定費 約 $32/月 + データ処理料がかかる。マルチ AZ 構成にすると
  AZ 数 x $32/月 とさらに増える
- 代替の VPC Interface Endpoint(ECR API / ECR DKR / CloudWatch Logs / Secrets Manager 等)を
  複数エンドポイントで揃える案も、エンドポイントあたり 約$7〜8/月 で 3〜4 個必要となり
  $22〜29/月 かかるため、NAT 回避のメリットが薄い
- 本モジュールでは ECS タスクを **パブリックサブネットに配置し public IP を付与**、
  セキュリティグループで inbound を ALB のみに絞ることで、NAT Gateway / Interface Endpoint
  なしで安全に outbound(ECR pull、CloudWatch Logs 送信、Secrets Manager 参照)を実現する
- トレードオフ: ECS タスクに public IP が付与されるため、SG による inbound 制限(ALB からのみ)が
  唯一の防御線になる。本番相当の強度が必要な場合は NAT Gateway + プライベートサブネットへの
  切り替えを検討すること
- コスト面のトレードオフ: 2024年2月以降、Fargate タスクに付与される Public IPv4 アドレスにも
  課金される(約 $0.005/時 ≒ 約$3.65/月/アドレス)。NAT Gateway の固定費(約$32/月〜、AZ 数倍)
  とは異なり、この課金は **稼働タスク数に比例して増える**(タスクを増やす/スケールアウトすると
  Public IPv4 課金も線形に増える)。少数タスクの dev 環境では NAT Gateway 回避のメリットが勝るが、
  タスク数が多い環境では NAT Gateway + プライベートサブネットの方が相対的に有利になり得るため、
  スケール時は再評価すること

### セキュリティグループをこのモジュールに集約した理由

ALB SG・ECS SG・RDS SG は互いに関係し合う(ALB egress の宛先が ECS、ECS ingress が ALB を
参照、ECS egress の宛先が RDS、RDS ingress が ECS を参照)。これらを `app` / `db` の各所有
モジュールに分散すると、モジュール間で SG ID を相互に渡し合う必要が生じ、循環参照
(`app` が `db` の SG を必要とし、`db` が `app` の SG を必要とする、等)が発生しやすい。
本モジュールに 3 つの SG とその ingress/egress ルールをすべて集約することで、
SG 間の関係は module 内部の resource 参照に閉じ、`app` / `db` モジュールは
SG ID を outputs 経由で受け取るだけの一方向依存にできる。

### SG ルールはインラインブロックのみで管理する(分離ルールリソースは使わない)

`aws_security_group`(alb / ecs / rds)の `ingress` / `egress` はすべて **インラインブロックのみ**
で宣言し、`aws_vpc_security_group_ingress_rule` / `aws_vpc_security_group_egress_rule`
(分離ルールリソース)はこのモジュールでは一切使用しない。

- 理由: 同一 SG に対してインラインブロックと分離ルールリソースを併用することは AWS provider
  でサポートされない構成であり、両者がルールの所有権を奪い合って `terraform plan` が
  収束しない(毎回差分が出続ける)か、一方が他方のルールを黙って上書きする不整合を招く。
  `terraform validate` はこの種の競合を検出できないため、インラインのみに統一して問題そのものを
  発生させない設計とした
- SG 本体の再作成なしにルールだけを個別に追加・削除できる分離リソースの利点(AWS provider v5 系
  で推奨される書き方)は、この設計では得られない。本モジュールのルール数は固定(各 SG 1〜3 個)
  であり、頻繁な追加・削除を想定していないため、この利点よりもインライン/分離混在の禁止を守る
  ことを優先した

### 循環依存を避けるための非対称設計(ingress は SG 参照、egress は CIDR ベース)

分離ルールリソースを使わずに ALB ⇄ ECS ⇄ RDS の関係を安全に表現するため、**ingress と egress で
参照方法を非対称にしている**:

- **ingress は SG 参照のまま**(`security_groups = [...]`)にし、実際に許可する送信元を
  厳密に絞る(例: RDS SG の ingress は ECS SG のみを許可)
- **egress は SG 参照をやめ、CIDR ベース**(`cidr_blocks = [var.vpc_cidr]` または
  `["0.0.0.0/0"]`)で表現する

この非対称化により、`aws_security_group` リソース間の依存方向は **ecs → alb、rds → ecs の
一方向のみ**になり(alb → ecs、ecs → rds という逆方向の参照は存在しない)、3 つの SG 間で
循環依存は発生しない。

トレードオフ: ECS → RDS の egress は「VPC CIDR 宛の TCP 5432」という CIDR ベースのルールになり、
SG 参照ほど宛先を厳密には絞れない(VPC 内の他のリソースが同じ CIDR 帯にあれば、ECS からの
egress 自体はブロックされない)。ただし実際の到達先は RDS SG 側の ingress が「ECS SG からの
みを許可」という制約を持つため、**ECS → RDS 以外の宛先への通信は egress は通っても ingress
側で拒否され、実質的に RDS のみに限定される**。ALB → ECS の egress も同様に VPC CIDR ベース
だが、ECS SG の ingress が ALB SG のみを許可するため同じ理屈で実到達先が絞られる。

### CloudFront プレフィックスリストによる ALB 保護

ALB SG の ingress は `com.amazonaws.global.cloudfront.origin-facing` マネージドプレフィックス
リストのみを許可し、CloudFront 以外からの直接アクセスを IP レベルで遮断する。ただし
プレフィックスリストは「CloudFront のオリジンサーバー向け送信元 IP 帯」であり、
別の CloudFront ディストリビューション経由のなりすましは防げない。これを補うため、
`app` / `cdn` モジュールでカスタムヘッダによる二重検証を行う(詳細は各モジュールの README)。

## 既知の制約

- SG のルールはすべて `aws_security_group` の **インラインブロックのみ**で管理しており、
  `aws_vpc_security_group_ingress_rule` / `aws_vpc_security_group_egress_rule`(分離ルール
  リソース)はこのモジュールでは使用していない。同一 SG へのインライン/分離の混在は
  AWS provider がサポートしない構成(`terraform plan` が収束しない、あるいはルールが
  片方に上書きされる懸念がある)ため、インラインのみに統一することでこの問題自体を回避している。
  RDS SG は `egress = []` を明示しており、これはインライン専用の SG に対する単独の宣言として
  完結する(分離リソースとの併用がないため、競合の余地はない)。これにより AWS が新規
  セキュリティグループ作成時に自動付与する「デフォルトの all-egress 許可ルール
  (0.0.0.0/0 全プロトコル)」も Terraform 管理下で確実に削除される
- 上記の設計変更(インライン専用化)により、以前ここに記載していた「インライン/分離混在に伴う
  `terraform plan` 収束の懸念」は解消された。未解決の懸念としては残っていない
- egress を CIDR ベースにしたことに伴う到達範囲のトレードオフ(ECS → RDS / ALB → ECS の egress
  が VPC CIDR 全体を宛先として許可される点)は、上の「循環依存を避けるための非対称設計」節を
  参照。受信側(RDS / ECS)の ingress が送信元 SG を厳密に絞っているため、実質的な到達先は
  意図した相手に限定される
