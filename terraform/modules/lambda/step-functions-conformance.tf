# The live AWS API differential runs AWS Step Functions state machines and
# compares their output, execution history, nested executions, distributed Map
# Runs, and AWS Lambda task integrations with the simulator.
data "aws_iam_policy_document" "step_functions_conformance_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["states.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "step_functions_conformance" {
  name               = "${local.name_prefix}-step-functions-conformance-role"
  assume_role_policy = data.aws_iam_policy_document.step_functions_conformance_assume_role.json

  tags = merge(local.common_tags, {
    Name      = "${local.name_prefix}-step-functions-conformance-role"
    component = "step-functions-conformance"
  })
}

data "aws_iam_policy_document" "step_functions_conformance" {
  statement {
    sid     = "InvokeConformanceFunctions"
    effect  = "Allow"
    actions = ["lambda:InvokeFunction"]
    resources = [
      "arn:${data.aws_partition.current.partition}:lambda:${var.region}:${data.aws_caller_identity.current.account_id}:function:sockerless-*",
    ]
  }

  statement {
    sid     = "StartDistributedMapChildren"
    effect  = "Allow"
    actions = ["states:StartExecution"]
    resources = [
      "arn:${data.aws_partition.current.partition}:states:${var.region}:${data.aws_caller_identity.current.account_id}:stateMachine:sockerless-*",
    ]
  }

  statement {
    sid    = "ObserveDistributedMapChildren"
    effect = "Allow"
    actions = [
      "states:DescribeExecution",
      "states:StopExecution",
    ]
    resources = [
      "arn:${data.aws_partition.current.partition}:states:${var.region}:${data.aws_caller_identity.current.account_id}:execution:sockerless-*:*",
    ]
  }

  statement {
    sid    = "ManageSynchronousExecutionEvents"
    effect = "Allow"
    actions = [
      "events:DescribeRule",
      "events:PutRule",
      "events:PutTargets",
    ]
    resources = [
      "arn:${data.aws_partition.current.partition}:events:${var.region}:${data.aws_caller_identity.current.account_id}:rule/StepFunctionsGetEventsForStepFunctionsExecutionRule",
    ]
  }
}

resource "aws_iam_role_policy" "step_functions_conformance" {
  name   = "${local.name_prefix}-step-functions-conformance"
  role   = aws_iam_role.step_functions_conformance.id
  policy = data.aws_iam_policy_document.step_functions_conformance.json
}

output "step_functions_conformance_role_arn" {
  description = "Execution role for the live AWS Step Functions API differential"
  value       = aws_iam_role.step_functions_conformance.arn
}
