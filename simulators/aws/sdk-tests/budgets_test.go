package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	btypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const budgetsAccountID = "123456789012"

func budgetsClient() *budgets.Client {
	return budgets.NewFromConfig(sdkConfig(), func(o *budgets.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestBudgetsCRUDSDK(t *testing.T) {
	c := budgetsClient()
	const name = "sdk-budget-crud"

	_, err := c.CreateBudget(ctx, &budgets.CreateBudgetInput{
		AccountId: aws.String(budgetsAccountID),
		Budget: &btypes.Budget{
			BudgetName: aws.String(name),
			BudgetLimit: &btypes.Spend{
				Amount: aws.String("100"),
				Unit:   aws.String("USD"),
			},
			TimeUnit:   btypes.TimeUnitMonthly,
			BudgetType: btypes.BudgetTypeCost,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteBudget(ctx, &budgets.DeleteBudgetInput{
			AccountId:  aws.String(budgetsAccountID),
			BudgetName: aws.String(name),
		})
	})

	desc, err := c.DescribeBudget(ctx, &budgets.DescribeBudgetInput{
		AccountId:  aws.String(budgetsAccountID),
		BudgetName: aws.String(name),
	})
	require.NoError(t, err)
	require.NotNil(t, desc.Budget)
	assert.Equal(t, name, aws.ToString(desc.Budget.BudgetName))
	assert.Equal(t, btypes.TimeUnitMonthly, desc.Budget.TimeUnit)
	assert.Equal(t, btypes.BudgetTypeCost, desc.Budget.BudgetType)
	require.NotNil(t, desc.Budget.BudgetLimit)
	assert.Equal(t, "100", aws.ToString(desc.Budget.BudgetLimit.Amount))
	assert.Equal(t, "USD", aws.ToString(desc.Budget.BudgetLimit.Unit))

	_, err = c.UpdateBudget(ctx, &budgets.UpdateBudgetInput{
		AccountId: aws.String(budgetsAccountID),
		NewBudget: &btypes.Budget{
			BudgetName: aws.String(name),
			BudgetLimit: &btypes.Spend{
				Amount: aws.String("250"),
				Unit:   aws.String("USD"),
			},
			TimeUnit:   btypes.TimeUnitMonthly,
			BudgetType: btypes.BudgetTypeCost,
		},
	})
	require.NoError(t, err)

	desc2, err := c.DescribeBudget(ctx, &budgets.DescribeBudgetInput{
		AccountId:  aws.String(budgetsAccountID),
		BudgetName: aws.String(name),
	})
	require.NoError(t, err)
	assert.Equal(t, "250", aws.ToString(desc2.Budget.BudgetLimit.Amount))

	listed, err := c.DescribeBudgets(ctx, &budgets.DescribeBudgetsInput{
		AccountId: aws.String(budgetsAccountID),
	})
	require.NoError(t, err)
	var seen bool
	for _, b := range listed.Budgets {
		if aws.ToString(b.BudgetName) == name {
			seen = true
		}
	}
	assert.True(t, seen, "DescribeBudgets must include created budget")

	_, err = c.DeleteBudget(ctx, &budgets.DeleteBudgetInput{
		AccountId:  aws.String(budgetsAccountID),
		BudgetName: aws.String(name),
	})
	require.NoError(t, err)

	_, err = c.DescribeBudget(ctx, &budgets.DescribeBudgetInput{
		AccountId:  aws.String(budgetsAccountID),
		BudgetName: aws.String(name),
	})
	assert.Error(t, err)
}

func TestBudgetsNotificationsAndSubscribersSDK(t *testing.T) {
	c := budgetsClient()
	const name = "sdk-budget-notif"

	_, err := c.CreateBudget(ctx, &budgets.CreateBudgetInput{
		AccountId: aws.String(budgetsAccountID),
		Budget: &btypes.Budget{
			BudgetName: aws.String(name),
			BudgetLimit: &btypes.Spend{
				Amount: aws.String("500"),
				Unit:   aws.String("USD"),
			},
			TimeUnit:   btypes.TimeUnitMonthly,
			BudgetType: btypes.BudgetTypeCost,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteBudget(ctx, &budgets.DeleteBudgetInput{
			AccountId:  aws.String(budgetsAccountID),
			BudgetName: aws.String(name),
		})
	})

	notif := &btypes.Notification{
		NotificationType:   btypes.NotificationTypeActual,
		ComparisonOperator: btypes.ComparisonOperatorGreaterThan,
		Threshold:          80,
		ThresholdType:      btypes.ThresholdTypePercentage,
	}

	_, err = c.CreateNotification(ctx, &budgets.CreateNotificationInput{
		AccountId:    aws.String(budgetsAccountID),
		BudgetName:   aws.String(name),
		Notification: notif,
		Subscribers: []btypes.Subscriber{
			{
				SubscriptionType: btypes.SubscriptionTypeEmail,
				Address:          aws.String("alerts@example.com"),
			},
		},
	})
	require.NoError(t, err)

	listed, err := c.DescribeNotificationsForBudget(ctx, &budgets.DescribeNotificationsForBudgetInput{
		AccountId:  aws.String(budgetsAccountID),
		BudgetName: aws.String(name),
	})
	require.NoError(t, err)
	require.Len(t, listed.Notifications, 1)
	assert.Equal(t, btypes.NotificationTypeActual, listed.Notifications[0].NotificationType)
	assert.Equal(t, 80.0, listed.Notifications[0].Threshold)

	subs, err := c.DescribeSubscribersForNotification(ctx, &budgets.DescribeSubscribersForNotificationInput{
		AccountId:    aws.String(budgetsAccountID),
		BudgetName:   aws.String(name),
		Notification: notif,
	})
	require.NoError(t, err)
	require.Len(t, subs.Subscribers, 1)
	assert.Equal(t, "alerts@example.com", aws.ToString(subs.Subscribers[0].Address))

	_, err = c.CreateSubscriber(ctx, &budgets.CreateSubscriberInput{
		AccountId:    aws.String(budgetsAccountID),
		BudgetName:   aws.String(name),
		Notification: notif,
		Subscriber: &btypes.Subscriber{
			SubscriptionType: btypes.SubscriptionTypeSns,
			Address:          aws.String("arn:aws:sns:us-east-1:123456789012:budget-alerts"),
		},
	})
	require.NoError(t, err)

	subs2, err := c.DescribeSubscribersForNotification(ctx, &budgets.DescribeSubscribersForNotificationInput{
		AccountId:    aws.String(budgetsAccountID),
		BudgetName:   aws.String(name),
		Notification: notif,
	})
	require.NoError(t, err)
	assert.Len(t, subs2.Subscribers, 2)

	_, err = c.DeleteSubscriber(ctx, &budgets.DeleteSubscriberInput{
		AccountId:    aws.String(budgetsAccountID),
		BudgetName:   aws.String(name),
		Notification: notif,
		Subscriber: &btypes.Subscriber{
			SubscriptionType: btypes.SubscriptionTypeEmail,
			Address:          aws.String("alerts@example.com"),
		},
	})
	require.NoError(t, err)

	subs3, err := c.DescribeSubscribersForNotification(ctx, &budgets.DescribeSubscribersForNotificationInput{
		AccountId:    aws.String(budgetsAccountID),
		BudgetName:   aws.String(name),
		Notification: notif,
	})
	require.NoError(t, err)
	assert.Len(t, subs3.Subscribers, 1)

	_, err = c.DeleteNotification(ctx, &budgets.DeleteNotificationInput{
		AccountId:    aws.String(budgetsAccountID),
		BudgetName:   aws.String(name),
		Notification: notif,
	})
	require.NoError(t, err)

	listed2, err := c.DescribeNotificationsForBudget(ctx, &budgets.DescribeNotificationsForBudgetInput{
		AccountId:  aws.String(budgetsAccountID),
		BudgetName: aws.String(name),
	})
	require.NoError(t, err)
	assert.Empty(t, listed2.Notifications)
}
