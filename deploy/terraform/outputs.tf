output "manipulator_public_ip" {
  description = "Public IP of the manipulator instance"
  value       = aws_instance.manipulator.public_ip
}

output "manipulator_private_ip" {
  description = "Private IP of the manipulator instance"
  value       = aws_instance.manipulator.private_ip
}

output "node_public_ips" {
  description = "Public IPs of data node instances"
  value       = aws_instance.node[*].public_ip
}

output "node_private_ips" {
  description = "Private IPs of data node instances (used in config.yaml)"
  value       = aws_instance.node[*].private_ip
}

output "proxy_public_ips" {
  description = "Public IPs of proxy instances"
  value       = aws_instance.proxy[*].public_ip
}

output "proxy_private_ips" {
  description = "Private IPs of proxy instances (used in config.yaml)"
  value       = aws_instance.proxy[*].private_ip
}

output "nlb_dns_name" {
  description = "DNS name of the AWS Network Load Balancer"
  value       = aws_lb.nlb.dns_name
}

output "nlb_zone_id" {
  description = "Route 53 hosted zone ID of the AWS Network Load Balancer"
  value       = aws_lb.nlb.zone_id
}

output "benchmark_public_ip" {
  description = "Public IP of the first Locust benchmark node"
  value       = aws_instance.benchmark[0].public_ip
}

output "benchmark_public_ips" {
  description = "Public IPs of Locust benchmark nodes"
  value       = aws_instance.benchmark[*].public_ip
}

output "benchmark_private_ip" {
  description = "Private IP of the first Locust benchmark node"
  value       = aws_instance.benchmark[0].private_ip
}

output "benchmark_private_ips" {
  description = "Private IPs of Locust benchmark nodes"
  value       = aws_instance.benchmark[*].private_ip
}
