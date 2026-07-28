# Sim surface — aws-budgets

Surface registered in `simulators/aws/budgets.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AWSBudgetServiceGateway.CreateBudget` | ✓ `simulators/aws/budgets.go:77::handleBudgetsCreateBudget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.DescribeBudget` | ✓ `simulators/aws/budgets.go:78::handleBudgetsDescribeBudget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.DeleteBudget` | ✓ `simulators/aws/budgets.go:79::handleBudgetsDeleteBudget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.UpdateBudget` | ✓ `simulators/aws/budgets.go:80::handleBudgetsUpdateBudget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.DescribeBudgets` | ✓ `simulators/aws/budgets.go:81::handleBudgetsDescribeBudgets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.CreateNotification` | ✓ `simulators/aws/budgets.go:82::handleBudgetsCreateNotification` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.DescribeNotificationsForBudget` | ✓ `simulators/aws/budgets.go:83::handleBudgetsDescribeNotificationsForBudget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.DeleteNotification` | ✓ `simulators/aws/budgets.go:84::handleBudgetsDeleteNotification` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.CreateSubscriber` | ✓ `simulators/aws/budgets.go:85::handleBudgetsCreateSubscriber` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.DescribeSubscribersForNotification` | ✓ `simulators/aws/budgets.go:86::handleBudgetsDescribeSubscribersForNotification` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.DeleteSubscriber` | ✓ `simulators/aws/budgets.go:87::handleBudgetsDeleteSubscriber` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.ListTagsForResource` | ✓ `simulators/aws/budgets.go:88::handleBudgetsListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.TagResource` | ✓ `simulators/aws/budgets.go:89::handleBudgetsTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSBudgetServiceGateway.UntagResource` | ✓ `simulators/aws/budgets.go:90::handleBudgetsUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
