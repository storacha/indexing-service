locals {
  azs      = slice(data.aws_availability_zones.available.names, 0, 3)
}

data "aws_availability_zones" "available" {}

resource "aws_vpc" "vpc" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "${var.environment}-${var.app}-vpc"
  }
}

resource "aws_internet_gateway" "vpc_internet_gateway" {
  vpc_id = aws_vpc.vpc.id

  tags = {
    Name = "${var.environment}-${var.app}-vpc-internet-gateway"
  }
}

resource "aws_subnet" "vpc_public_subnet" {
  count = length(local.azs)
  
  vpc_id            = aws_vpc.vpc.id
  availability_zone = local.azs[count.index]
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index)

  tags = {
    Name = "${var.environment}-${var.app}-vpc-public-subnet-${local.azs[count.index]}"
  }
}

resource "aws_subnet" "vpc_private_subnet" {
  count = length(local.azs)
  
  vpc_id            = aws_vpc.vpc.id
  availability_zone = local.azs[count.index]
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index + 10)

  tags = {
    Name = "${var.environment}-${var.app}-vpc-private-subnet-${local.azs[count.index]}"
  }
}

resource "aws_route_table" "vpc_public_route_table" {
  count = length(local.azs)

  vpc_id = aws_vpc.vpc.id

  tags = {
    Name = "${var.environment}-${var.app}-vpc-public-route-table-${local.azs[count.index]}"
  }
}

resource "aws_route_table" "vpc_private_route_table" {
  count = length(local.azs)

  vpc_id = aws_vpc.vpc.id

  tags = {
    Name = "${var.environment}-${var.app}-vpc-private-route-table-${local.azs[count.index]}"
  }
}

resource "aws_route_table_association" "vpc_public_route_table_association" {
  count = length(local.azs)

  subnet_id      = aws_subnet.vpc_public_subnet[count.index].id
  route_table_id = aws_route_table.vpc_public_route_table[count.index].id
}

resource "aws_route_table_association" "vpc_private_route_table_association" {
  count = length(local.azs)

  subnet_id      = aws_subnet.vpc_private_subnet[count.index].id
  route_table_id = aws_route_table.vpc_private_route_table[count.index].id
}

resource "aws_route" "vpc_public_internet_gateway" {
  count = length(local.azs)

  route_table_id         = aws_route_table.vpc_public_route_table[count.index].id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.vpc_internet_gateway.id

  timeouts {
    create = "5m"
  }
}

resource "aws_route" "vpc_private_nat_gateway" {
  count = length(local.azs)

  route_table_id         = aws_route_table.vpc_private_route_table[count.index].id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.vpc_nat[count.index].id

  timeouts {
    create = "5m"
  }
}

resource "aws_eip" "vpc_nat_ip" {
  count = length(local.azs)

  domain = "vpc"

  tags = {
    Name = "${var.environment}-${var.app}-vpc-nat-ip-${local.azs[count.index]}"
  }
}

resource "aws_nat_gateway" "vpc_nat" {
  count = length(local.azs)

  subnet_id     = aws_subnet.vpc_public_subnet[count.index].id
  allocation_id = aws_eip.vpc_nat_ip[count.index].id
  depends_on    = [aws_internet_gateway.vpc_internet_gateway]

  tags = {
    Name = "${var.environment}-${var.app}-vpc-nat-${local.azs[count.index]}"
  }
}
