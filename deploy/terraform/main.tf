terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# ── Default VPC/subnets ───────────────────────────────────────────────────────
data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

# ── Key pair ──────────────────────────────────────────────────────────────────
resource "aws_key_pair" "utterdb" {
  key_name   = "utterdb-key"
  public_key = file(var.public_key_path)
}

# ── Security group ────────────────────────────────────────────────────────────
resource "aws_security_group" "utterdb" {
  name        = "utterdb-sg"
  description = "utterdb cluster traffic"

  # SSH
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.allowed_cidr]
  }

  # Manipulator
  ingress {
    from_port   = var.manipulator_port
    to_port     = var.manipulator_port
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Data nodes  — reserve a generous range so any node_count fits
  ingress {
    from_port   = var.node_base_port
    to_port     = var.node_base_port + 99
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Proxy nodes — same idea
  ingress {
    from_port   = var.proxy_base_port
    to_port     = var.proxy_base_port + 99
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Cluster-internal traffic, including distributed Locust master/worker ports.
  ingress {
    from_port = 0
    to_port   = 0
    protocol  = "-1"
    self      = true
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# ── AMI lookup (latest Amazon Linux 2023 x86_64) ──────────────────────────────
data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# ── Manipulator (singleton) ───────────────────────────────────────────────────
resource "aws_instance" "manipulator" {
  ami                    = data.aws_ami.al2023.id
  instance_type          = var.manipulator_instance_type
  key_name               = aws_key_pair.utterdb.key_name
  vpc_security_group_ids = [aws_security_group.utterdb.id]

  tags = { Name = "utterdb-manipulator" }
}

# ── Data nodes (arbitrary count) ──────────────────────────────────────────────
resource "aws_instance" "node" {
  count                  = var.node_count
  ami                    = data.aws_ami.al2023.id
  instance_type          = var.instance_type
  key_name               = aws_key_pair.utterdb.key_name
  vpc_security_group_ids = [aws_security_group.utterdb.id]

  tags = { Name = "utterdb-node-${count.index}" }
}

# ── Proxy nodes (arbitrary count) ─────────────────────────────────────────────
resource "aws_instance" "proxy" {
  count                  = var.proxy_count
  ami                    = data.aws_ami.al2023.id
  instance_type          = var.instance_type
  key_name               = aws_key_pair.utterdb.key_name
  vpc_security_group_ids = [aws_security_group.utterdb.id]

  tags = { Name = "utterdb-proxy-${count.index}" }
}

# ── AWS Network Load Balancer ─────────────────────────────────────────────────
resource "aws_lb" "nlb" {
  name                             = "utterdb-nlb"
  internal                         = false
  load_balancer_type               = "network"
  subnets                          = data.aws_subnets.default.ids
  enable_cross_zone_load_balancing = true

  tags = { Name = "utterdb-nlb" }
}

resource "aws_lb_target_group" "proxy" {
  name        = "utterdb-proxy-tg"
  port        = var.proxy_base_port
  protocol    = "TCP"
  target_type = "instance"
  vpc_id      = data.aws_vpc.default.id

  health_check {
    enabled             = true
    protocol            = "TCP"
    port                = "traffic-port"
    healthy_threshold   = 2
    unhealthy_threshold = 2
    interval            = 10
  }
}

resource "aws_lb_target_group_attachment" "proxy" {
  count            = var.proxy_count
  target_group_arn = aws_lb_target_group.proxy.arn
  target_id        = aws_instance.proxy[count.index].id
  port             = var.proxy_base_port + count.index
}

resource "aws_lb_listener" "nlb" {
  load_balancer_arn = aws_lb.nlb.arn
  port              = var.nlb_port
  protocol          = "TCP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.proxy.arn
  }
}

# ── Benchmark runners (same VPC, hit the NLB DNS name) ────────────────────────
resource "aws_instance" "benchmark" {
  count                  = var.benchmark_count
  ami                    = data.aws_ami.al2023.id
  instance_type          = var.benchmark_instance_type
  key_name               = aws_key_pair.utterdb.key_name
  vpc_security_group_ids = [aws_security_group.utterdb.id]

  tags = { Name = "utterdb-benchmark-${count.index}" }
}
