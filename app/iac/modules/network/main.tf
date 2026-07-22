# network module
#
# Responsibilities:
#   - VPC / Internet Gateway / public & private subnets / route tables
#   - Security groups for ALB, ECS tasks and RDS, and their ingress/egress rules
#   - CloudFront origin-facing managed prefix list lookup (used to restrict ALB ingress)
#
# Security groups for all three tiers (alb / ecs / rds) are intentionally kept in this
# single module (see module README) so that cross-tier SG references (ALB -> ECS -> RDS)
# do not create circular dependencies between modules.

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.55"
    }
  }
}

# ---------------------------------------------------------------------------
# VPC / Internet Gateway
# ---------------------------------------------------------------------------

resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = merge(var.tags, { Name = "${var.name_prefix}-vpc" })
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id

  tags = merge(var.tags, { Name = "${var.name_prefix}-igw" })
}

# ---------------------------------------------------------------------------
# Subnets (for_each keyed by AZ so subnet count follows var.azs without count())
# ---------------------------------------------------------------------------

resource "aws_subnet" "public" {
  for_each = zipmap(var.azs, var.public_subnet_cidrs)

  vpc_id                  = aws_vpc.this.id
  availability_zone       = each.key
  cidr_block              = each.value
  map_public_ip_on_launch = true

  tags = merge(var.tags, { Name = "${var.name_prefix}-public-${each.key}" })
}

resource "aws_subnet" "private" {
  for_each = zipmap(var.azs, var.private_subnet_cidrs)

  vpc_id                  = aws_vpc.this.id
  availability_zone       = each.key
  cidr_block              = each.value
  map_public_ip_on_launch = false

  tags = merge(var.tags, { Name = "${var.name_prefix}-private-${each.key}" })
}

# ---------------------------------------------------------------------------
# Route tables
# ---------------------------------------------------------------------------

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  tags = merge(var.tags, { Name = "${var.name_prefix}-public-rt" })
}

resource "aws_route" "public_internet" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.this.id
}

resource "aws_route_table_association" "public" {
  for_each = aws_subnet.public

  subnet_id      = each.value.id
  route_table_id = aws_route_table.public.id
}

# Private route table has no NAT / IGW route (NAT Gateway is intentionally not used,
# see module README). Only the local VPC route (implicit, added by AWS) applies.
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.this.id

  tags = merge(var.tags, { Name = "${var.name_prefix}-private-rt" })
}

resource "aws_route_table_association" "private" {
  for_each = aws_subnet.private

  subnet_id      = each.value.id
  route_table_id = aws_route_table.private.id
}

# ---------------------------------------------------------------------------
# CloudFront origin-facing managed prefix list
# ---------------------------------------------------------------------------

data "aws_ec2_managed_prefix_list" "cloudfront" {
  name = "com.amazonaws.global.cloudfront.origin-facing"
}

# ---------------------------------------------------------------------------
# Security groups. All ingress/egress rules are declared as inline blocks
# only; separate aws_vpc_security_group_ingress_rule / _egress_rule resources
# are intentionally NOT used anywhere in this module. Mixing inline rule
# blocks and the separate rule resources on the same security group is
# unsupported by the AWS provider (the two mechanisms fight over rule
# ownership, producing a perpetual plan diff or one style silently
# overwriting the other). Inline-only avoids this entirely (see README).
#
# To keep the dependency graph acyclic without the separate rule resources,
# only *ingress* rules reference a peer security group (`security_groups =
# [...]`). *Egress* rules are expressed as CIDR blocks (VPC CIDR or
# 0.0.0.0/0) rather than a peer SG reference, even where the destination is
# conceptually another tier's SG. This makes the resource dependency
# direction strictly one-way (ecs -> alb, rds -> ecs; never alb -> ecs or
# ecs -> rds), so there is no circular dependency between the three
# `aws_security_group` resources. Actual reachability is still restricted to
# the intended peer, because the *receiving* side's ingress only accepts
# traffic from that peer's security group (see README for the trade-off this
# implies for the ecs -> rds path).
# ---------------------------------------------------------------------------

resource "aws_security_group" "alb" {
  name        = "${var.name_prefix}-alb-sg"
  description = "ALB security group: allows inbound only from CloudFront origin-facing IP ranges"
  vpc_id      = aws_vpc.this.id

  ingress {
    description     = "HTTP from CloudFront origin-facing IP ranges only"
    from_port       = 80
    to_port         = 80
    protocol        = "tcp"
    prefix_list_ids = [data.aws_ec2_managed_prefix_list.cloudfront.id]
  }

  egress {
    description = "Forward traffic to ECS tasks (CIDR-based, not an SG reference, to keep the dependency direction one-way; see README)"
    from_port   = var.container_port
    to_port     = var.container_port
    protocol    = "tcp"
    cidr_blocks = [var.vpc_cidr]
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-alb-sg" })
}

resource "aws_security_group" "ecs" {
  name        = "${var.name_prefix}-ecs-sg"
  description = "ECS task security group: allows inbound only from the ALB security group"
  vpc_id      = aws_vpc.this.id

  ingress {
    description     = "Container port from the ALB only"
    from_port       = var.container_port
    to_port         = var.container_port
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    description = "PostgreSQL traffic to RDS (CIDR-based, not an SG reference, to keep the dependency direction one-way; see README). Reachability is still limited to RDS because the RDS SG's ingress only accepts traffic from this ECS SG."
    from_port   = var.db_port
    to_port     = var.db_port
    protocol    = "tcp"
    cidr_blocks = [var.vpc_cidr]
  }

  # No NAT Gateway / VPC interface endpoints are used (see README), so ECS
  # tasks need direct outbound HTTPS via the Internet Gateway (public IP) to
  # reach ECR, CloudWatch Logs and Secrets Manager endpoints.
  egress {
    description = "HTTPS to AWS service endpoints (ECR/CloudWatch Logs/Secrets Manager) via IGW; no NAT Gateway is used"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-ecs-sg" })
}

resource "aws_security_group" "rds" {
  name        = "${var.name_prefix}-rds-sg"
  description = "RDS security group: allows inbound only from the ECS security group"
  vpc_id      = aws_vpc.this.id

  ingress {
    description     = "PostgreSQL from ECS tasks only"
    from_port       = var.db_port
    to_port         = var.db_port
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs.id]
  }

  # Explicit empty egress: this security group is managed inline-only (no
  # separate aws_vpc_security_group_egress_rule resource exists for it), so
  # `egress = []` on its own is sufficient to remove the AWS default
  # all-egress rule and make "no egress at all" the confirmed,
  # Terraform-managed state (matches design intent). No mixing of inline
  # blocks and separate rule resources occurs here.
  egress = []

  tags = merge(var.tags, { Name = "${var.name_prefix}-rds-sg" })
}
