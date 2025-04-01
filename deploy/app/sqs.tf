resource "aws_sqs_queue" "caching" {
  name = "${terraform.workspace}-${var.app}-caching"
  visibility_timeout_seconds = 300
  message_retention_seconds = 86400
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.caching_deadletter.arn
    maxReceiveCount     = 4
  })
  tags = {
    Name = "${terraform.workspace}-${var.app}-caching"
  }
}

resource "aws_sqs_queue" "caching_deadletter" {
  name = "${terraform.workspace}-${var.app}-caching-deadletter"
}

resource "aws_sqs_queue_redrive_allow_policy" "caching" {
  queue_url = aws_sqs_queue.caching_deadletter.id

  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue",
    sourceQueueArns   = [aws_sqs_queue.caching.arn]
  })
}