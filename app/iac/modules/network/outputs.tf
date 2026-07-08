output "vpc_id" {
  description = "ID of the created VPC."
  value       = aws_vpc.this.id
}

output "public_subnet_ids" {
  description = "IDs of the public subnets (one per AZ)."
  value       = [for s in aws_subnet.public : s.id]
}

output "private_subnet_ids" {
  description = "IDs of the private subnets (one per AZ)."
  value       = [for s in aws_subnet.private : s.id]
}

output "alb_sg_id" {
  description = "ID of the ALB security group."
  value       = aws_security_group.alb.id
}

output "ecs_sg_id" {
  description = "ID of the ECS task security group."
  value       = aws_security_group.ecs.id
}

output "rds_sg_id" {
  description = "ID of the RDS security group."
  value       = aws_security_group.rds.id
}
