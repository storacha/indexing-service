locals {
  vpc_cidr = "10.0.0.0/16"
  azs      = slice(data.aws_availability_zones.available.names, 0, 3)
}

data "aws_availability_zones" "available" {}

resource "aws_vpc" "dev_vpc" {  
  cidr_block           = local.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true
  
  tags = {
    Name = "dev-${var.app}-vpc"
  }
}

resource "aws_subnet" "dev_vpc_private_subnet" {
  count = length(local.azs)

  vpc_id            = aws_vpc.dev_vpc.id
  availability_zone = local.azs[count.index]
  cidr_block        = cidrsubnet(local.vpc_cidr, 8, count.index + 10)

  tags = {
    Name = "dev-${var.app}-vpc-private-subnet-${local.azs[count.index]}"
  }
}


resource "aws_subnet" "dev_vpc_public_subnet" {
  count = length(local.azs)
  
  vpc_id            = aws_vpc.dev_vpc.id
  availability_zone = local.azs[count.index]
  cidr_block        = cidrsubnet(local.vpc_cidr, 8, count.index)

  tags = {
    Name = "dev-${var.app}-vpc-public-subnet-${local.azs[count.index]}"
  }
}

resource "aws_internet_gateway" "dev_vpc_igw" {
  vpc_id = aws_vpc.dev_vpc.id
  
  tags = {
    Name = "dev-${var.app}-vpc-internet-gateway"
  }
}

resource "aws_nat_gateway" "dev_vpc_nat" {
  count = length(local.azs)

  subnet_id     = aws_subnet.dev_vpc_public_subnet[count.index].id
  allocation_id = aws_eip.dev_vpc_nat[count.index].id
  depends_on    = [aws_internet_gateway.dev_vpc_igw]
  
  tags = {
    Name = "dev-${var.app}-vpc-nat-${local.azs[count.index]}"
  }
}

resource "aws_eip" "dev_vpc_nat" {
  count = length(local.azs)

  domain = "vpc"
  
  tags = {
    Name = "dev-${var.app}-vpc-nat-ip-${local.azs[count.index]}"
  }
}

resource "aws_route_table" "dev_vpc_public_route_table" {
  count = length(local.azs)

  vpc_id = aws_vpc.dev_vpc.id

  tags = {
    Name = "dev-${var.app}-vpc-public-route-table-${local.azs[count.index]}"
  }
}

resource "aws_route_table" "dev_vpc_private_route_table" {
  count = length(local.azs)

  vpc_id = aws_vpc.dev_vpc.id

  tags = {
    Name = "dev-${var.app}-vpc-private-route-table-${local.azs[count.index]}"
  }
}

resource "aws_route_table_association" "dev_vpc_public_route_table_association" {
  count = length(local.azs)

  subnet_id      = aws_subnet.dev_vpc_public_subnet[count.index].id
  route_table_id = aws_route_table.dev_vpc_public_route_table[count.index].id
}

resource "aws_route_table_association" "dev_vpc_private_route_table_association" {
  count = length(local.azs)

  subnet_id      = aws_subnet.dev_vpc_private_subnet[count.index].id
  route_table_id = aws_route_table.dev_vpc_private_route_table[count.index].id
}

resource "aws_route" "dev_vpc_public_internet_gateway" {
  count = length(local.azs)

  route_table_id         = aws_route_table.dev_vpc_public_route_table[count.index].id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.dev_vpc_igw.id
}

resource "aws_route" "dev_vpc_private_nat_gateway" {
  count = length(local.azs)

  route_table_id         = aws_route_table.dev_vpc_private_route_table[count.index].id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.dev_vpc_nat[count.index].id
}

output "dev_vpc" {
  value = {
    id         = aws_vpc.dev_vpc.id
    cidr_block = aws_vpc.dev_vpc.cidr_block
    azs        = local.azs
    subnets = {
      private = aws_subnet.dev_vpc_private_subnet[*].id
      public  = aws_subnet.dev_vpc_public_subnet[*].id
    }
  }
}