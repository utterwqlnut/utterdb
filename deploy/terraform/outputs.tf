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

output "nlb_public_ip" {
  description = "Public IP of the NLB (HAProxy) instance — send load-test traffic here"
  value       = aws_instance.nlb.public_ip
}

output "nlb_private_ip" {
  description = "Private IP of the NLB instance"
  value       = aws_instance.nlb.private_ip
}
