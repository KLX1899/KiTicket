output "vpc_id" {
  value = module.vpc.vpc_id
}

output "application_url" {
  value = "http://${aws_lb.app.dns_name}"
}

output "database_address" {
  value     = aws_db_instance.postgres.address
  sensitive = true
}

output "redis_address" {
  value     = aws_elasticache_replication_group.locks.primary_endpoint_address
  sensitive = true
}
