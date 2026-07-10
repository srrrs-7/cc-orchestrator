# db module
#
# RDS PostgreSQL instance (single-AZ, db.t4g.micro by default), its DB subnet
# group and parameter group. Master credentials are managed by RDS itself via
# AWS Secrets Manager (manage_master_user_password), never written to state
# or tfvars in plain text.
#
# This module also provisions the *credentials* (not the roles themselves)
# for api/auth's least-privilege runtime DB roles (ISSUE-016 R-c) -- see the
# "Least-privilege runtime DB roles" section below and README.md.

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.54"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

locals {
  parameter_group_family = "postgres${split(".", var.engine_version)[0]}"
}

resource "aws_db_subnet_group" "this" {
  name       = "${var.name_prefix}-db-subnet-group"
  subnet_ids = var.private_subnet_ids

  tags = merge(var.tags, { Name = "${var.name_prefix}-db-subnet-group" })
}

resource "aws_db_parameter_group" "this" {
  name   = "${var.name_prefix}-pg-params"
  family = local.parameter_group_family

  tags = merge(var.tags, { Name = "${var.name_prefix}-pg-params" })
}

resource "aws_db_instance" "this" {
  identifier = "${var.name_prefix}-db"

  engine         = "postgres"
  engine_version = var.engine_version
  instance_class = var.instance_class

  allocated_storage = var.allocated_storage
  storage_type      = "gp3"
  storage_encrypted = true

  db_name  = var.db_name
  username = var.master_username

  # Secrets Manager-managed master password: no plaintext password anywhere
  # in configuration, tfvars or state (see module README).
  manage_master_user_password = true

  db_subnet_group_name   = aws_db_subnet_group.this.name
  parameter_group_name   = aws_db_parameter_group.this.name
  vpc_security_group_ids = [var.rds_sg_id]

  multi_az            = var.multi_az
  publicly_accessible = false

  backup_retention_period    = var.backup_retention_period
  auto_minor_version_upgrade = true
  deletion_protection        = var.deletion_protection

  skip_final_snapshot       = var.skip_final_snapshot
  final_snapshot_identifier = var.skip_final_snapshot ? null : "${var.name_prefix}-db-final-snapshot"

  tags = merge(var.tags, { Name = "${var.name_prefix}-db" })
}

# --- Least-privilege runtime DB roles (ISSUE-016 R-c) -----------------------
#
# api/auth's runtime containers connect as a scoped, per-service PostgreSQL
# role (api_app / auth_app) instead of the RDS master user above, so that a
# leaked runtime credential cannot reach the other service's database or run
# DDL. RDS is private-subnet-only and unreachable from wherever `terraform
# apply`/`plan` runs (see README "別データベースでも権限境界ではない" and
# modules/service/README.md "database ブートストラップ"), so this module
# cannot create the roles themselves (no `cyrilgdn/postgresql` provider,
# no ROLE/GRANT resources here). It only generates and stores each role's
# *credentials*; app/migrator -- which already runs inside the VPC as the
# migrate init container -- reads the same credentials back out via
# APP_DB_USER/APP_DB_PASSWORD and is what actually issues `CREATE ROLE` /
# `ALTER ROLE ... PASSWORD` / `GRANT` (see envs/dev/main.tf's
# migration_secrets and docs/plans/ISSUE-016-plan.md).
#
# Trade-off (documented, not silently accepted): unlike the master user's
# manage_master_user_password (no password ever enters state), random_password
# writes its result to Terraform state in plain text (state is S3 +
# encrypt=true, see envs/dev/versions.tf). This is accepted in exchange for
# not adding an AWS SDK dependency to app/migrator, whose only runtime
# dependency is meant to stay pgx (see README "state にパスワードが載る
# トレードオフ" and .claude/rules/db.md).
#
# Each secret uses the same JSON shape {"username","password"} as the master
# user secret so the existing ECS `secret-arn:json-key::` valueFrom syntax
# (":username::" / ":password::") works unchanged for these secrets too.

resource "random_password" "api_app" {
  length = 32
  # Alphanumeric-only: avoids characters that would need extra escaping in
  # migrator's `ALTER ROLE ... PASSWORD '...'` SQL literal (matches the
  # alphanumeric-only rationale already used for random_password.origin_verify
  # in envs/dev/main.tf).
  special = false
}

resource "aws_secretsmanager_secret" "api_app" {
  name = "${var.name_prefix}-db-api-app-credentials"

  # 0 = delete immediately instead of the default 30-day recovery window, so
  # a disposable dev environment can be destroyed/recreated without hitting
  # "secret scheduled for deletion" name conflicts (same disposability intent
  # as aws_db_instance.this's skip_final_snapshot = true above).
  recovery_window_in_days = 0

  tags = merge(var.tags, { Name = "${var.name_prefix}-db-api-app-credentials" })
}

resource "aws_secretsmanager_secret_version" "api_app" {
  secret_id = aws_secretsmanager_secret.api_app.id

  secret_string = jsonencode({
    username = var.api_app_role_name
    password = random_password.api_app.result
  })
}

resource "random_password" "auth_app" {
  length  = 32
  special = false
}

resource "aws_secretsmanager_secret" "auth_app" {
  name = "${var.name_prefix}-db-auth-app-credentials"

  recovery_window_in_days = 0

  tags = merge(var.tags, { Name = "${var.name_prefix}-db-auth-app-credentials" })
}

resource "aws_secretsmanager_secret_version" "auth_app" {
  secret_id = aws_secretsmanager_secret.auth_app.id

  secret_string = jsonencode({
    username = var.auth_app_role_name
    password = random_password.auth_app.result
  })
}
