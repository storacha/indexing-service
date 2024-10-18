resource "aws_sns_topic" "published_advertisememt_head_change" {
  name = "${terraform.workspace}-${var.app}-published-advertisement-head-change"
}
