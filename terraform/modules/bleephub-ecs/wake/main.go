package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apitypes "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	schedulertypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
)

func main() { lambda.Start(wake) }

func wake(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	cluster, service, apiID, serviceIntegrationID, targetGroupARN := os.Getenv("ECS_CLUSTER"), os.Getenv("ECS_SERVICE"), os.Getenv("API_ID"), os.Getenv("SERVICE_INTEGRATION_ID"), os.Getenv("SERVICE_TARGET_GROUP_ARN")
	dqliteServices, dqliteTargetGroups := csvEnv("DQLITE_SERVICES"), csvEnv("DQLITE_TARGET_GROUP_ARNS")
	if os.Getenv("IDLE_ARM") != "true" && (cluster == "" || service == "" || apiID == "" || serviceIntegrationID == "" || targetGroupARN == "" || len(dqliteServices) != 3 || len(dqliteTargetGroups) != 3) {
		return events.APIGatewayV2HTTPResponse{}, fmt.Errorf("ECS_CLUSTER, ECS_SERVICE, API_ID, SERVICE_INTEGRATION_ID, SERVICE_TARGET_GROUP_ARN, DQLITE_SERVICES, and DQLITE_TARGET_GROUP_ARNS are required")
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{}, fmt.Errorf("load AWS configuration: %w", err)
	}
	ecsClient := ecs.NewFromConfig(cfg)
	apiClient := apigatewayv2.NewFromConfig(cfg)
	if os.Getenv("IDLE_ARM") == "true" {
		if err := armIdleAlarm(ctx, cloudwatch.NewFromConfig(cfg), awslambda.NewFromConfig(cfg), os.Getenv("IDLE_ALARM_NAME"), os.Getenv("IDLE_SHUTDOWN_FUNCTION_ARN")); err != nil {
			return events.APIGatewayV2HTTPResponse{}, err
		}
		return response(http.StatusNoContent, ""), nil
	}
	if os.Getenv("IDLE_SHUTDOWN") != "true" {
		if err := deferIdleAlarm(ctx, cloudwatch.NewFromConfig(cfg), scheduler.NewFromConfig(cfg), os.Getenv("IDLE_ALARM_NAME"), os.Getenv("IDLE_ARM_FUNCTION_ARN"), os.Getenv("IDLE_ARM_SCHEDULER_ROLE_ARN"), os.Getenv("IDLE_ARM_SCHEDULE_NAME")); err != nil {
			return events.APIGatewayV2HTTPResponse{}, err
		}
	}
	services, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(cluster), Services: []string{service}})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{}, fmt.Errorf("describe ECS service: %w", err)
	}
	if len(services.Services) != 1 {
		return events.APIGatewayV2HTTPResponse{}, fmt.Errorf("ECS service %q was not found", service)
	}
	if os.Getenv("IDLE_SHUTDOWN") == "true" {
		if err := quiesceIdleAlarm(ctx, cloudwatch.NewFromConfig(cfg), os.Getenv("IDLE_ALARM_NAME")); err != nil {
			return events.APIGatewayV2HTTPResponse{}, err
		}
		if err := routeToWake(ctx, apiClient, apiID); err != nil {
			return events.APIGatewayV2HTTPResponse{}, err
		}
		if services.Services[0].DesiredCount != 0 {
			if _, err := ecsClient.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: aws.String(cluster), Service: aws.String(service), DesiredCount: aws.Int32(0)}); err != nil {
				return events.APIGatewayV2HTTPResponse{}, fmt.Errorf("stop ECS service after idle timeout: %w", err)
			}
		}
		appTasks, err := stopServiceTasks(ctx, ecsClient, cluster, service)
		if err != nil {
			return events.APIGatewayV2HTTPResponse{}, err
		}
		if err := waitForTasksStopped(ctx, ecsClient, cluster, appTasks); err != nil {
			return events.APIGatewayV2HTTPResponse{}, err
		}
		if err := setDesiredCount(ctx, ecsClient, cluster, dqliteServices, 0); err != nil {
			return events.APIGatewayV2HTTPResponse{}, fmt.Errorf("stop dqlite quorum after application drain: %w", err)
		}
		for _, dqliteService := range dqliteServices {
			tasks, err := stopServiceTasks(ctx, ecsClient, cluster, dqliteService)
			if err != nil {
				return events.APIGatewayV2HTTPResponse{}, err
			}
			if err := waitForTasksStopped(ctx, ecsClient, cluster, tasks); err != nil {
				return events.APIGatewayV2HTTPResponse{}, fmt.Errorf("wait for dqlite voter %s to stop: %w", dqliteService, err)
			}
		}
		return response(http.StatusAccepted, "Bleephub stopped after five minutes without requests."), nil
	}
	loadBalancerClient := elasticloadbalancingv2.NewFromConfig(cfg)
	ready, err := ensureDqlite(ctx, ecsClient, loadBalancerClient, cluster, dqliteServices, dqliteTargetGroups)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{}, err
	}
	if !ready {
		return response(http.StatusServiceUnavailable, "Bleephub database quorum is starting; retry shortly."), nil
	}
	if services.Services[0].DesiredCount == 0 {
		if _, err := ecsClient.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: aws.String(cluster), Service: aws.String(service), DesiredCount: aws.Int32(1)}); err != nil {
			return events.APIGatewayV2HTTPResponse{}, fmt.Errorf("wake ECS service: %w", err)
		}
		return response(http.StatusServiceUnavailable, "Bleephub is starting; retry shortly."), nil
	}

	health, err := loadBalancerClient.DescribeTargetHealth(ctx, &elasticloadbalancingv2.DescribeTargetHealthInput{TargetGroupArn: aws.String(targetGroupARN)})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{}, fmt.Errorf("describe Bleephub task health: %w", err)
	}
	for _, target := range health.TargetHealthDescriptions {
		if target.TargetHealth != nil && target.TargetHealth.State == elbtypes.TargetHealthStateEnumHealthy {
			if err := routeTo(ctx, apiClient, apiID, "integrations/"+serviceIntegrationID); err != nil {
				return events.APIGatewayV2HTTPResponse{}, err
			}
			location := "https://" + request.RequestContext.DomainName + request.RawPath
			if request.RawQueryString != "" {
				location += "?" + request.RawQueryString
			}
			return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusTemporaryRedirect, Headers: map[string]string{"location": location, "retry-after": "1"}, Body: "Bleephub is ready."}, nil
		}
	}
	return response(http.StatusServiceUnavailable, "Bleephub is starting; retry shortly."), nil
}

func quiesceIdleAlarm(ctx context.Context, client *cloudwatch.Client, alarmName string) error {
	if alarmName == "" {
		return fmt.Errorf("IDLE_ALARM_NAME is required")
	}
	if _, err := client.DisableAlarmActions(ctx, &cloudwatch.DisableAlarmActionsInput{AlarmNames: []string{alarmName}}); err != nil {
		return fmt.Errorf("disable Bleephub idle alarm before shutdown: %w", err)
	}
	if _, err := client.SetAlarmState(ctx, &cloudwatch.SetAlarmStateInput{
		AlarmName:   aws.String(alarmName),
		StateValue:  cloudwatchtypes.StateValueOk,
		StateReason: aws.String("Bleephub shutdown completed; wait for the next request to begin a new idle window."),
	}); err != nil {
		return fmt.Errorf("clear Bleephub idle alarm before shutdown: %w", err)
	}
	return nil
}

// deferIdleAlarm clears a stale alarm then uses Amazon EventBridge Scheduler
// to enable its actions after the full idle window. This leaves the actual
// inactivity decision to Amazon API Gateway's native request metric while
// preventing an old zero-value window from stopping a newly awakened stack.
func deferIdleAlarm(ctx context.Context, cloudWatchClient *cloudwatch.Client, schedulerClient *scheduler.Client, alarmName, armFunctionARN, schedulerRoleARN, scheduleName string) error {
	if alarmName == "" || armFunctionARN == "" || schedulerRoleARN == "" || scheduleName == "" {
		return fmt.Errorf("IDLE_ALARM_NAME, IDLE_ARM_FUNCTION_ARN, IDLE_ARM_SCHEDULER_ROLE_ARN, and IDLE_ARM_SCHEDULE_NAME are required")
	}
	if _, err := cloudWatchClient.DisableAlarmActions(ctx, &cloudwatch.DisableAlarmActionsInput{AlarmNames: []string{alarmName}}); err != nil {
		return fmt.Errorf("disable idle alarm while Bleephub starts: %w", err)
	}
	if _, err := cloudWatchClient.SetAlarmState(ctx, &cloudwatch.SetAlarmStateInput{
		AlarmName:   aws.String(alarmName),
		StateValue:  cloudwatchtypes.StateValueOk,
		StateReason: aws.String("Bleephub received an API Gateway request; begin a fresh inactivity window."),
	}); err != nil {
		return fmt.Errorf("clear idle alarm after Bleephub request: %w", err)
	}
	when := time.Now().UTC().Add(5 * time.Minute).Format("2006-01-02T15:04:05")
	input := &scheduler.CreateScheduleInput{
		Name:                       aws.String(scheduleName),
		ActionAfterCompletion:      schedulertypes.ActionAfterCompletionDelete,
		FlexibleTimeWindow:         &schedulertypes.FlexibleTimeWindow{Mode: schedulertypes.FlexibleTimeWindowModeOff},
		ScheduleExpression:         aws.String("at(" + when + ")"),
		ScheduleExpressionTimezone: aws.String("UTC"),
		Target: &schedulertypes.Target{
			Arn:     aws.String(armFunctionARN),
			RoleArn: aws.String(schedulerRoleARN),
			Input:   aws.String(`{"source":"bleephub-idle-arm"}`),
		},
	}
	if _, err := schedulerClient.CreateSchedule(ctx, input); err == nil {
		return nil
	} else {
		var conflict *schedulertypes.ConflictException
		if !errors.As(err, &conflict) {
			return fmt.Errorf("schedule Bleephub idle alarm re-arm: %w", err)
		}
	}
	_, err := schedulerClient.UpdateSchedule(ctx, &scheduler.UpdateScheduleInput{
		Name:                       input.Name,
		ActionAfterCompletion:      input.ActionAfterCompletion,
		FlexibleTimeWindow:         input.FlexibleTimeWindow,
		ScheduleExpression:         input.ScheduleExpression,
		ScheduleExpressionTimezone: input.ScheduleExpressionTimezone,
		Target:                     input.Target,
	})
	if err != nil {
		return fmt.Errorf("replace Bleephub idle alarm re-arm: %w", err)
	}
	return nil
}

func armIdleAlarm(ctx context.Context, cloudWatchClient *cloudwatch.Client, lambdaClient *awslambda.Client, alarmName, shutdownFunctionARN string) error {
	if alarmName == "" || shutdownFunctionARN == "" {
		return fmt.Errorf("IDLE_ALARM_NAME and IDLE_SHUTDOWN_FUNCTION_ARN are required")
	}
	if _, err := cloudWatchClient.EnableAlarmActions(ctx, &cloudwatch.EnableAlarmActionsInput{AlarmNames: []string{alarmName}}); err != nil {
		return fmt.Errorf("enable Bleephub idle alarm: %w", err)
	}
	alarms, err := cloudWatchClient.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{AlarmNames: []string{alarmName}})
	if err != nil {
		return fmt.Errorf("read Bleephub idle alarm after enabling actions: %w", err)
	}
	if len(alarms.MetricAlarms) != 1 {
		return fmt.Errorf("bleephub idle alarm %q was not found", alarmName)
	}
	if alarms.MetricAlarms[0].StateValue == cloudwatchtypes.StateValueAlarm {
		if _, err := lambdaClient.Invoke(ctx, &awslambda.InvokeInput{FunctionName: aws.String(shutdownFunctionARN), InvocationType: "Event", Payload: []byte(`{}`)}); err != nil {
			return fmt.Errorf("invoke Bleephub idle shutdown for an already-alarming metric: %w", err)
		}
	}
	return nil
}

func stopServiceTasks(ctx context.Context, ecsClient *ecs.Client, cluster, service string) ([]string, error) {
	tasks, err := ecsClient.ListTasks(ctx, &ecs.ListTasksInput{Cluster: aws.String(cluster), ServiceName: aws.String(service), DesiredStatus: "RUNNING"})
	if err != nil {
		return nil, fmt.Errorf("list running ECS tasks for %s: %w", service, err)
	}
	for _, task := range tasks.TaskArns {
		if _, err := ecsClient.StopTask(ctx, &ecs.StopTaskInput{Cluster: aws.String(cluster), Task: aws.String(task), Reason: aws.String("Bleephub idle shutdown")}); err != nil {
			return nil, fmt.Errorf("stop ECS task %s for %s: %w", task, service, err)
		}
	}
	return tasks.TaskArns, nil
}

func csvEnv(name string) []string {
	values := []string{}
	for _, value := range strings.Split(os.Getenv(name), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func setDesiredCount(ctx context.Context, ecsClient *ecs.Client, cluster string, services []string, count int32) error {
	for _, service := range services {
		if _, err := ecsClient.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: aws.String(cluster), Service: aws.String(service), DesiredCount: aws.Int32(count)}); err != nil {
			return fmt.Errorf("set ECS service %s desired count to %d: %w", service, count, err)
		}
	}
	return nil
}

func ensureDqlite(ctx context.Context, ecsClient *ecs.Client, loadBalancerClient *elasticloadbalancingv2.Client, cluster string, services, targetGroups []string) (bool, error) {
	described, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(cluster), Services: services})
	if err != nil {
		return false, fmt.Errorf("describe dqlite ECS services: %w", err)
	}
	if len(described.Services) != len(services) {
		return false, fmt.Errorf("dqlite ECS services were not all found")
	}
	for _, service := range described.Services {
		if service.DesiredCount == 0 {
			if err := setDesiredCount(ctx, ecsClient, cluster, services, 1); err != nil {
				return false, err
			}
			return false, nil
		}
	}
	healthyVoters := 0
	for _, targetGroup := range targetGroups {
		health, err := loadBalancerClient.DescribeTargetHealth(ctx, &elasticloadbalancingv2.DescribeTargetHealthInput{TargetGroupArn: aws.String(targetGroup)})
		if err != nil {
			return false, fmt.Errorf("describe dqlite target health: %w", err)
		}
		healthy := false
		for _, target := range health.TargetHealthDescriptions {
			if target.TargetHealth != nil && target.TargetHealth.State == elbtypes.TargetHealthStateEnumHealthy {
				healthy = true
				break
			}
		}
		if !healthy {
			continue
		}
		healthyVoters++
	}
	// dqlite commits with a majority of its configured voters. Requiring every
	// voter here turns a recoverable single-voter restart into a full outage.
	return healthyVoters >= len(targetGroups)/2+1, nil
}

func waitForTasksStopped(ctx context.Context, ecsClient *ecs.Client, cluster string, tasks []string) error {
	if len(tasks) == 0 {
		return nil
	}
	for {
		described, err := ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: aws.String(cluster), Tasks: tasks})
		if err != nil {
			return fmt.Errorf("describe draining ECS tasks: %w", err)
		}
		if len(described.Tasks) != len(tasks) {
			return fmt.Errorf("describe draining ECS tasks returned %d tasks, expected %d", len(described.Tasks), len(tasks))
		}
		stopped := true
		for _, task := range described.Tasks {
			if aws.ToString(task.LastStatus) != "STOPPED" {
				stopped = false
				break
			}
		}
		if stopped {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Bleephub ECS tasks to stop: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func routeToWake(ctx context.Context, client *apigatewayv2.Client, apiID string) error {
	integrations, err := client.GetIntegrations(ctx, &apigatewayv2.GetIntegrationsInput{ApiId: aws.String(apiID)})
	if err != nil {
		return fmt.Errorf("list API Gateway integrations: %w", err)
	}
	for _, integration := range integrations.Items {
		if integration.IntegrationType == apitypes.IntegrationTypeAwsProxy {
			return routeTo(ctx, client, apiID, "integrations/"+aws.ToString(integration.IntegrationId))
		}
	}
	return fmt.Errorf("wake integration was not found")
}

func routeTo(ctx context.Context, client *apigatewayv2.Client, apiID, target string) error {
	routes, err := client.GetRoutes(ctx, &apigatewayv2.GetRoutesInput{ApiId: aws.String(apiID)})
	if err != nil {
		return fmt.Errorf("list API Gateway routes: %w", err)
	}
	for _, route := range routes.Items {
		if aws.ToString(route.RouteKey) == "$default" {
			if _, err := client.UpdateRoute(ctx, &apigatewayv2.UpdateRouteInput{ApiId: aws.String(apiID), RouteId: route.RouteId, Target: aws.String(target)}); err != nil {
				return fmt.Errorf("switch API Gateway route to %s: %w", target, err)
			}
			return nil
		}
	}
	return fmt.Errorf("API Gateway default route was not found")
}

func response(status int, body string) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{StatusCode: status, Headers: map[string]string{"content-type": "text/plain; charset=utf-8", "retry-after": "2"}, Body: body}
}
