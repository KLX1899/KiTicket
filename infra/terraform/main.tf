terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
  default_tags {
    tags = { Project = "event-ticketing", ManagedBy = "Terraform" }
  }
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.13.0"

  name               = "ticketing"
  cidr               = "10.0.0.0/16"
  azs                = ["${var.region}a", "${var.region}b"]
  private_subnets    = ["10.0.1.0/24", "10.0.2.0/24"]
  public_subnets     = ["10.0.101.0/24", "10.0.102.0/24"]
  enable_nat_gateway = true
  single_nat_gateway = var.single_nat_gateway
}

resource "aws_security_group" "alb" {
  name   = "ticketing-alb"
  vpc_id = module.vpc.vpc_id
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "app" {
  name   = "ticketing-app"
  vpc_id = module.vpc.vpc_id
  ingress {
    from_port       = 3000
    to_port         = 3000
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "data" {
  name   = "ticketing-data"
  vpc_id = module.vpc.vpc_id
  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.app.id]
  }
  ingress {
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.app.id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_db_subnet_group" "postgres" {
  name       = "ticketing-postgres"
  subnet_ids = module.vpc.private_subnets
}

resource "aws_db_instance" "postgres" {
  identifier              = "ticketing"
  engine                  = "postgres"
  engine_version          = "16"
  instance_class          = var.db_instance_class
  allocated_storage       = 20
  max_allocated_storage   = 100
  storage_encrypted       = true
  db_name                 = "ticketing"
  username                = var.db_user
  password                = var.db_password
  db_subnet_group_name    = aws_db_subnet_group.postgres.name
  vpc_security_group_ids  = [aws_security_group.data.id]
  publicly_accessible     = false
  multi_az                = var.multi_az
  backup_retention_period = 7
  deletion_protection     = var.deletion_protection
  skip_final_snapshot     = !var.deletion_protection
}

resource "aws_elasticache_subnet_group" "redis" {
  name       = "ticketing-redis"
  subnet_ids = module.vpc.private_subnets
}

resource "aws_elasticache_replication_group" "locks" {
  replication_group_id       = "ticket-locks"
  description                = "Transient seat locks and waiting-room state"
  node_type                  = var.redis_node_type
  port                       = 6379
  num_cache_clusters         = var.multi_az ? 2 : 1
  automatic_failover_enabled = var.multi_az
  at_rest_encryption_enabled = true
  subnet_group_name          = aws_elasticache_subnet_group.redis.name
  security_group_ids         = [aws_security_group.data.id]
}

resource "aws_lb" "app" {
  name               = "ticketing"
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = module.vpc.public_subnets
}

resource "aws_lb_target_group" "backend" {
  name     = "ticketing-backend"
  port     = 3000
  protocol = "HTTP"
  vpc_id   = module.vpc.vpc_id
  health_check {
    path                = "/api/health/live"
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.app.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.backend.arn
  }
}

data "aws_ami" "amazon_linux" {
  most_recent = true
  owners      = ["amazon"]
  filter {
    name   = "name"
    values = ["al2023-ami-2023.*-x86_64"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_launch_template" "backend" {
  name_prefix            = "ticketing-backend-"
  image_id               = data.aws_ami.amazon_linux.id
  instance_type          = var.app_instance_type
  vpc_security_group_ids = [aws_security_group.app.id]
  user_data = base64encode(templatefile("${path.module}/user-data.sh.tftpl", {
    container_image = var.backend_container_image
    database_url    = "postgres://${var.db_user}:${var.db_password}@${aws_db_instance.postgres.address}:5432/ticketing"
    redis_url       = "redis://${aws_elasticache_replication_group.locks.primary_endpoint_address}:6379"
    jwt_secret      = var.jwt_secret
  }))
  metadata_options {
    http_tokens   = "required"
    http_endpoint = "enabled"
  }
  tag_specifications {
    resource_type = "instance"
    tags          = { Name = "ticketing-backend" }
  }
}

resource "aws_autoscaling_group" "backend" {
  name                = "ticketing-backend"
  min_size            = 2
  desired_capacity    = 2
  max_size            = 6
  vpc_zone_identifier = module.vpc.private_subnets
  target_group_arns   = [aws_lb_target_group.backend.arn]
  health_check_type   = "ELB"
  launch_template {
    id      = aws_launch_template.backend.id
    version = "$Latest"
  }
  instance_refresh {
    strategy = "Rolling"
    preferences { min_healthy_percentage = 100 }
  }
}

resource "aws_autoscaling_policy" "cpu" {
  name                   = "ticketing-cpu-target"
  autoscaling_group_name = aws_autoscaling_group.backend.name
  policy_type            = "TargetTrackingScaling"
  target_tracking_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ASGAverageCPUUtilization"
    }
    target_value = 65
  }
}
