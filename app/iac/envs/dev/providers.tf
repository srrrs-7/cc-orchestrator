provider "aws" {
  region = var.region

  default_tags {
    tags = local.common_tags
  }
}

# WAFv2 CLOUDFRONT-scope Web ACLs must be created via us-east-1; this alias is
# passed explicitly into the cdn module (see main.tf).
provider "aws" {
  alias  = "us_east_1"
  region = "us-east-1"

  default_tags {
    tags = local.common_tags
  }
}
