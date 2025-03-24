

resource "aws_sqs_queue" "caching" {
  name = "${terraform.workspace}-${var.app}-caching.fifo"
  fifo_queue = true
  content_based_deduplication = true
  visibility_timeout_seconds = 300
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.caching_deadletter.arn
    maxReceiveCount     = 4
  })
  tags = {
    Name = "${terraform.workspace}-${var.app}-caching"
  }
}

resource "aws_sqs_queue" "caching_deadletter" {
  fifo_queue = true
  content_based_deduplication = true
  name = "${terraform.workspace}-${var.app}-caching-deadletter.fifo"
}

resource "aws_sqs_queue_redrive_allow_policy" "caching" {
  queue_url = aws_sqs_queue.caching_deadletter.id

  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue",
    sourceQueueArns   = [aws_sqs_queue.caching.arn]
  })
}