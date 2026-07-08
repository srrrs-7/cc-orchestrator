# db module
#
# RDS PostgreSQL instance (single-AZ, db.t4g.micro by default), its DB subnet
# group and parameter group. Master credentials are managed by RDS itself via
# AWS Secrets Manager (manage_master_user_password), never written to state
# or tfvars in plain text.

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
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
