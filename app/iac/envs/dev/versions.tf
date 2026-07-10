terraform {
  required_version = ">= 1.10.0"

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

  # Remote state backend (S3 + native S3 locking via use_lockfile, no
  # DynamoDB table required). Values below are PLACEHOLDERS: the S3 bucket
  # must be created out-of-band before `terraform init` can succeed, and the
  # bucket name / key / region must be replaced with real values by whoever
  # runs this configuration (see envs/dev/README.md for the bootstrap
  # procedure). Do not put account-specific values here in version control
  # without team agreement.
  backend "s3" {
    bucket       = "REPLACE_WITH_TERRAFORM_STATE_BUCKET_NAME"
    key          = "cc-orchestrator/dev/terraform.tfstate"
    region       = "ap-northeast-1"
    encrypt      = true
    use_lockfile = true
  }
}
