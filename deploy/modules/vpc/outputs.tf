output "id" {
  value = aws_vpc.vpc.id
}

output "cidr_block" {
  value = aws_vpc.vpc.cidr_block
}

output "subnets" {
  value = {
      private = aws_subnet.vpc_private_subnet[*].id
      public  = aws_subnet.vpc_public_subnet[*].id
    }
}
