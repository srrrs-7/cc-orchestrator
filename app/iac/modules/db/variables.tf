variable "name_prefix" {
  type        = string
  description = "Prefix applied to all resource names/tags created by this module."
}

variable "private_subnet_ids" {
  type        = list(string)
  description = "Private subnet IDs for the DB subnet group (RDS is not placed in public subnets)."
}

variable "rds_sg_id" {
  type        = string
  description = "Security group ID to attach to the RDS instance (must allow inbound only from the ECS security group)."
}

variable "instance_class" {
  type        = string
  description = "RDS instance class."
  default     = "db.t4g.micro"
}

variable "allocated_storage" {
  type        = number
  description = "Allocated storage for the RDS instance, in GiB."
  default     = 20
}

variable "engine_version" {
  type        = string
  description = "PostgreSQL engine version (major.minor, e.g. \"16.4\")."
  default     = "16.4"
}

variable "db_name" {
  type        = string
  description = "Initial database name created on the RDS instance."
  default     = "app"
}

variable "master_username" {
  type        = string
  description = "Master username for the RDS instance. Not a secret by itself; the password is managed by RDS/Secrets Manager (manage_master_user_password)."
  default     = "app_admin"

  # ISSUE-016 review-security Major-2: mutual-distinctness of this variable
  # vs. api_app_role_name/auth_app_role_name is enforced by
  # auth_app_role_name's validation below, in one place, to avoid a
  # dependency cycle (Terraform 1.9+ cross-variable validation only allows a
  # one-directional reference graph; each of the three variables validating
  # against the other two forms a cycle -- see that variable's comment for
  # the full rationale).
}

variable "multi_az" {
  type        = bool
  description = "Whether to enable Multi-AZ deployment. Defaults to false (single-AZ) to minimize dev cost; see module README for the availability trade-off."
  default     = false
}

variable "deletion_protection" {
  type        = bool
  description = "Whether to enable RDS deletion protection."
  default     = false
}

variable "skip_final_snapshot" {
  type        = bool
  description = "Whether to skip the final snapshot on destroy. Defaults to true for a disposable dev environment."
  default     = true
}

variable "backup_retention_period" {
  type        = number
  description = "Number of days to retain automated backups."
  default     = 1
}

variable "api_app_role_name" {
  type        = string
  description = "PostgreSQL role name used by app/api's runtime connection (least-privilege scoped role, ISSUE-016 R-c). This module only generates/stores this role's credentials (random_password + a dedicated Secrets Manager secret); the role itself is created by app/migrator, not by Terraform (RDS is unreachable from where terraform apply/plan runs, see README)."
  default     = "api_app"

  # ISSUE-016 review-security Major-2: mutual-distinctness enforced by
  # auth_app_role_name's validation below (see that variable's comment).
}

variable "auth_app_role_name" {
  type        = string
  description = "PostgreSQL role name used by app/auth's runtime connection (least-privilege scoped role, ISSUE-016 R-c). Same caveat as api_app_role_name: only the credentials are provisioned here, the role itself is created by app/migrator."
  default     = "auth_app"

  # ISSUE-016 review-security Major-2: master_username and the two scoped
  # runtime role names (api_app_role_name, this variable) must be pairwise
  # distinct. If any two collided, app/migrator's idempotent role-credential
  # sync (`ALTER ROLE <name> PASSWORD ...`, run by the migrate init
  # container to keep each scoped role's password in sync with its Secrets
  # Manager secret) would resolve the colliding name to a single PostgreSQL
  # role and overwrite that role's password with the wrong value -- most
  # dangerously, overwriting the RDS *master* password with a scoped app
  # role's randomly generated one, silently locking out master access. Fail
  # closed here at plan/apply time instead of at migrate-init runtime.
  #
  # All three pairwise comparisons are checked from this single variable's
  # validation (not from each of the three) because Terraform 1.9+
  # cross-variable validation forms a dependency graph from the reference,
  # and three variables each validating against the other two creates a
  # cycle ("Cycle: module.db.var.auth_app_role_name (validation), ...
  # (validation)" at `terraform validate`). A one-directional reference
  # graph (only this variable references the other two; they don't
  # reference back) avoids that cycle while still covering all three pairs.
  #
  # This module-level guard holds even when module.db is called directly
  # from a future env root module; it duplicates the equivalent check on
  # envs/dev's db_auth_app_role_name (envs/dev/variables.tf) so the failure
  # surfaces regardless of which layer's defaults/tfvars introduce the
  # collision.
  validation {
    condition = (
      var.master_username != var.api_app_role_name &&
      var.master_username != var.auth_app_role_name &&
      var.api_app_role_name != var.auth_app_role_name
    )
    error_message = "master_username, api_app_role_name and auth_app_role_name must all be pairwise distinct (ISSUE-016 Major-2): a collision would make app/migrator's role-credential sync overwrite the colliding role's password (most dangerously, the RDS master password) with the wrong value."
  }
}

variable "tags" {
  type        = map(string)
  description = "Common tags applied to all resources created by this module."
  default     = {}
}
