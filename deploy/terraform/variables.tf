variable "aws_region" {
  description = "AWS region to deploy into"
  type        = string
  default     = "us-east-1"
}

variable "instance_type" {
  description = "EC2 instance type for data nodes and proxies"
  type        = string
  default     = "t3.micro"
}

variable "nlb_instance_type" {
  description = "EC2 instance type for the NLB (HAProxy) instance"
  type        = string
  default     = "m7a.xlarge"
}

variable "benchmark_instance_type" {
  description = "EC2 instance type for the benchmark runner"
  type        = string
  default     = "m7a.xlarge"
}
variable "manipulator_instance_type" {
  description = "EC2 instance type for the manipulator"
  type        = string
  default     = "t3.micro"
}

variable "public_key_path" {
  description = "Path to the SSH public key to install on instances"
  type        = string
  default     = "~/.ssh/id_ed25519.pub"
}

variable "allowed_cidr" {
  description = "CIDR block allowed to SSH into instances"
  type        = string
  default     = "0.0.0.0/0"
}

variable "node_count" {
  description = "Number of data node instances"
  type        = number
  default     = 2
}

variable "proxy_count" {
  description = "Number of proxy instances"
  type        = number
  default     = 1
}

variable "shards" {
  description = "Number of hash ring shards written into config.yaml"
  type        = number
  default     = 128
}

variable "node_base_port" {
  description = "Data node i listens on node_base_port + i"
  type        = number
  default     = 8000
}

variable "proxy_base_port" {
  description = "Proxy i listens on proxy_base_port + i"
  type        = number
  default     = 8100
}

variable "manipulator_port" {
  description = "Port the single manipulator listens on"
  type        = number
  default     = 8080
}

variable "nlb_port" {
  description = "Port HAProxy (NLB) listens on for client traffic"
  type        = number
  default     = 9000
}
