output "db_endpoint" {
  description = "RDS instance hostname (no port)."
  value       = aws_db_instance.this.address
}

output "db_port" {
  description = "RDS instance port."
  value       = aws_db_instance.this.port
}

output "db_name" {
  description = "Database name."
  value       = aws_db_instance.this.db_name
}

output "master_user_secret_arn" {
  description = "ARN of the Secrets Manager secret holding the master user credentials (created by manage_master_user_password). Still used by app/migrator's migrate init container (CREATE DATABASE + CREATE ROLE/GRANT), no longer by api/auth's app containers directly (ISSUE-016 R-c; see api_app_secret_arn / auth_app_secret_arn)."
  value       = aws_db_instance.this.master_user_secret[0].secret_arn
}

output "api_app_role_name" {
  description = "PostgreSQL role name provisioned for app/api's runtime connection (matches var.api_app_role_name; also embedded as the \"username\" JSON key in api_app_secret_arn)."
  value       = var.api_app_role_name
}

output "api_app_secret_arn" {
  description = "ARN of the dedicated Secrets Manager secret holding api_app's scoped runtime credentials ({\"username\",\"password\"} JSON, same shape as master_user_secret_arn). app/api's app container reads DB_USER/DB_PASSWORD from this via ECS \"secret-arn:json-key::\" valueFrom; app/migrator additionally reads it as APP_DB_USER/APP_DB_PASSWORD to provision/sync the role."
  value       = aws_secretsmanager_secret.api_app.arn
}

output "auth_app_role_name" {
  description = "PostgreSQL role name provisioned for app/auth's runtime connection (matches var.auth_app_role_name; also embedded as the \"username\" JSON key in auth_app_secret_arn)."
  value       = var.auth_app_role_name
}

output "auth_app_secret_arn" {
  description = "ARN of the dedicated Secrets Manager secret holding auth_app's scoped runtime credentials, same shape/usage as api_app_secret_arn but for app/auth."
  value       = aws_secretsmanager_secret.auth_app.arn
}
