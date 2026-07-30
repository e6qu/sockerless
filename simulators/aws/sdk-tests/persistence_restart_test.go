package aws_sdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
	amplifytypes "github.com/aws/aws-sdk-go-v2/service/amplify/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	wafv2types "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func persistentSimulatorPorts(t *testing.T) (int, int) {
	t.Helper()
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tcpPort := tcpListener.Addr().(*net.TCPAddr).Port
	require.NoError(t, tcpListener.Close())

	udpListener, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	udpPort := udpListener.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, udpListener.Close())
	return tcpPort, udpPort
}

func startPersistentSimulator(t *testing.T, stateDir string, tcpPort, udpPort int, runtimeName string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(),
		"SIM_AWS_PORT="+strconv.Itoa(tcpPort),
		"SIM_DNS_PORT="+strconv.Itoa(udpPort),
		"SIM_RUNTIME="+runtimeName,
		"SIM_PERSIST=true",
		"SIM_DATA_DIR="+stateDir,
		"SIM_LOG_LEVEL=warn",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	require.NoError(t, waitForHealth(fmt.Sprintf("http://127.0.0.1:%d/health", tcpPort)))
	return cmd
}

func persistentSDKConfig() aws.Config {
	return sdkConfig()
}

func TestAWSServiceStateSurvivesSimulatorRestart_SDK(t *testing.T) {
	stateDir := t.TempDir()
	tcpPort, udpPort := persistentSimulatorPorts(t)
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", tcpPort)

	cmd := startPersistentSimulator(t, stateDir, tcpPort, udpPort, "process")
	t.Cleanup(func() { shutdownSimulator(cmd) })

	cfg := persistentSDKConfig()
	amplifyAPI := amplify.NewFromConfig(cfg, func(o *amplify.Options) { o.BaseEndpoint = aws.String(endpoint) })
	wafAPI := wafv2.NewFromConfig(cfg, func(o *wafv2.Options) { o.BaseEndpoint = aws.String(endpoint) })
	route53API := route53.NewFromConfig(cfg, func(o *route53.Options) { o.BaseEndpoint = aws.String(endpoint) })
	iamAPI := iam.NewFromConfig(cfg, func(o *iam.Options) { o.BaseEndpoint = aws.String(endpoint) })
	dynamoDBAPI := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(endpoint) })
	stepFunctionsAPI := sfn.NewFromConfig(cfg, func(o *sfn.Options) { o.BaseEndpoint = aws.String(endpoint) })
	lambdaAPI := lambda.NewFromConfig(cfg, func(o *lambda.Options) { o.BaseEndpoint = aws.String(endpoint) })
	sqsAPI := sqs.NewFromConfig(cfg, func(o *sqs.Options) { o.BaseEndpoint = aws.String(endpoint) })
	s3API := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	testCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	app, err := amplifyAPI.CreateApp(testCtx, &amplify.CreateAppInput{Name: aws.String("persistent-app")})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	appARN := aws.ToString(app.App.AppArn)

	webACL, err := wafAPI.CreateWebACL(testCtx, &wafv2.CreateWebACLInput{
		Name:  aws.String("persistent-web-acl"),
		Scope: wafv2types.ScopeCloudfront,
		DefaultAction: &wafv2types.DefaultAction{
			Allow: &wafv2types.AllowAction{},
		},
		VisibilityConfig: &wafv2types.VisibilityConfig{
			CloudWatchMetricsEnabled: true,
			MetricName:               aws.String("persistent-web-acl"),
			SampledRequestsEnabled:   true,
		},
	})
	require.NoError(t, err)
	webACLARN := aws.ToString(webACL.Summary.ARN)
	_, err = wafAPI.AssociateWebACL(testCtx, &wafv2.AssociateWebACLInput{
		ResourceArn: aws.String(appARN),
		WebACLArn:   aws.String(webACLARN),
	})
	require.NoError(t, err)

	hostedZone, err := route53API.CreateHostedZone(testCtx, &route53.CreateHostedZoneInput{
		CallerReference: aws.String("persistent-zone"),
		Name:            aws.String("persistent.example"),
		HostedZoneConfig: &r53types.HostedZoneConfig{
			PrivateZone: true,
		},
		VPC: &r53types.VPC{
			VPCId:     aws.String("vpc-persistent-primary"),
			VPCRegion: r53types.VPCRegionUsEast1,
		},
	})
	require.NoError(t, err)
	hostedZoneID := strings.TrimPrefix(aws.ToString(hostedZone.HostedZone.Id), "/hostedzone/")
	_, err = route53API.ChangeTagsForResource(testCtx, &route53.ChangeTagsForResourceInput{
		ResourceId:   aws.String(hostedZoneID),
		ResourceType: r53types.TagResourceTypeHostedzone,
		AddTags: []r53types.Tag{{
			Key:   aws.String("persistence"),
			Value: aws.String("verified"),
		}},
	})
	require.NoError(t, err)
	_, err = route53API.CreateVPCAssociationAuthorization(testCtx, &route53.CreateVPCAssociationAuthorizationInput{
		HostedZoneId: aws.String(hostedZoneID),
		VPC: &r53types.VPC{
			VPCId:     aws.String("vpc-persistent-authorized"),
			VPCRegion: r53types.VPCRegionUsWest2,
		},
	})
	require.NoError(t, err)

	serviceRole, err := iamAPI.CreateServiceLinkedRole(testCtx, &iam.CreateServiceLinkedRoleInput{
		AWSServiceName: aws.String("amplify.amazonaws.com"),
		CustomSuffix:   aws.String("persistence"),
	})
	require.NoError(t, err)
	deletion, err := iamAPI.DeleteServiceLinkedRole(testCtx, &iam.DeleteServiceLinkedRoleInput{
		RoleName: serviceRole.Role.RoleName,
	})
	require.NoError(t, err)
	deletionTaskID := aws.ToString(deletion.DeletionTaskId)

	_, err = dynamoDBAPI.CreateTable(testCtx, &dynamodb.CreateTableInput{
		TableName:   aws.String("persistent-table"),
		BillingMode: ddbtypes.BillingModePayPerRequest,
		AttributeDefinitions: []ddbtypes.AttributeDefinition{{
			AttributeName: aws.String("id"),
			AttributeType: ddbtypes.ScalarAttributeTypeS,
		}},
		KeySchema: []ddbtypes.KeySchemaElement{{
			AttributeName: aws.String("id"),
			KeyType:       ddbtypes.KeyTypeHash,
		}},
	})
	require.NoError(t, err)
	_, err = dynamoDBAPI.PutItem(testCtx, &dynamodb.PutItemInput{
		TableName: aws.String("persistent-table"),
		Item: map[string]ddbtypes.AttributeValue{
			"id":    &ddbtypes.AttributeValueMemberS{Value: "item-1"},
			"value": &ddbtypes.AttributeValueMemberS{Value: "survived"},
		},
	})
	require.NoError(t, err)

	stateMachine, err := stepFunctionsAPI.CreateStateMachine(testCtx, &sfn.CreateStateMachineInput{
		Name:       aws.String("persistent-state-machine"),
		Definition: aws.String(`{"StartAt":"Persisted","States":{"Persisted":{"Type":"Pass","Result":{"value":"survived"},"End":true}}}`),
		RoleArn:    aws.String("arn:aws:iam::123456789012:role/persistent-sfn-role"),
		Type:       sfntypes.StateMachineTypeStandard,
	})
	require.NoError(t, err)
	execution, err := stepFunctionsAPI.StartExecution(testCtx, &sfn.StartExecutionInput{
		StateMachineArn: stateMachine.StateMachineArn,
		Name:            aws.String("persistent-execution"),
		Input:           aws.String(`{"request":"restart"}`),
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		out, describeErr := stepFunctionsAPI.DescribeExecution(testCtx, &sfn.DescribeExecutionInput{
			ExecutionArn: execution.ExecutionArn,
		})
		return describeErr == nil && out.Status == sfntypes.ExecutionStatusSucceeded
	}, 10*time.Second, 50*time.Millisecond)

	waitMachine, err := stepFunctionsAPI.CreateStateMachine(testCtx, &sfn.CreateStateMachineInput{
		Name:       aws.String("restart-wait-state-machine"),
		Definition: aws.String(`{"StartAt":"WaitAcrossRestart","States":{"WaitAcrossRestart":{"Type":"Wait","Seconds":3,"Next":"Done"},"Done":{"Type":"Succeed"}}}`),
		RoleArn:    aws.String("arn:aws:iam::123456789012:role/persistent-sfn-role"),
		Type:       sfntypes.StateMachineTypeStandard,
	})
	require.NoError(t, err)
	waitExecution, err := stepFunctionsAPI.StartExecution(testCtx, &sfn.StartExecutionInput{
		StateMachineArn: waitMachine.StateMachineArn,
		Name:            aws.String("restart-wait-execution"),
	})
	require.NoError(t, err)
	time.Sleep(1200 * time.Millisecond)
	beforeRestart, err := stepFunctionsAPI.DescribeExecution(testCtx, &sfn.DescribeExecutionInput{
		ExecutionArn: waitExecution.ExecutionArn,
	})
	require.NoError(t, err)
	require.Equal(t, sfntypes.ExecutionStatusRunning, beforeRestart.Status)

	const functionName = "persistent-lambda"
	_, err = lambdaAPI.CreateFunction(testCtx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(functionName),
		Role:         aws.String("arn:aws:iam::123456789012:role/persistent-lambda-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String("example.invalid/persistent:1")},
	})
	require.NoError(t, err)
	version, err := lambdaAPI.PublishVersion(testCtx, &lambda.PublishVersionInput{
		FunctionName: aws.String(functionName),
	})
	require.NoError(t, err)
	versionNumber := aws.ToString(version.Version)
	_, err = lambdaAPI.CreateAlias(testCtx, &lambda.CreateAliasInput{
		FunctionName: aws.String(functionName), Name: aws.String("live"),
		FunctionVersion: aws.String(versionNumber),
	})
	require.NoError(t, err)
	_, err = lambdaAPI.AddPermission(testCtx, &lambda.AddPermissionInput{
		FunctionName: aws.String(functionName), StatementId: aws.String("persistent-invoke"),
		Action: aws.String("lambda:InvokeFunction"), Principal: aws.String("events.amazonaws.com"),
	})
	require.NoError(t, err)
	functionURL, err := lambdaAPI.CreateFunctionUrlConfig(testCtx, &lambda.CreateFunctionUrlConfigInput{
		FunctionName: aws.String(functionName), AuthType: lambdatypes.FunctionUrlAuthTypeNone,
	})
	require.NoError(t, err)
	_, err = lambdaAPI.PutFunctionEventInvokeConfig(testCtx, &lambda.PutFunctionEventInvokeConfigInput{
		FunctionName: aws.String(functionName), MaximumRetryAttempts: aws.Int32(1),
		MaximumEventAgeInSeconds: aws.Int32(120),
	})
	require.NoError(t, err)
	_, err = lambdaAPI.PutProvisionedConcurrencyConfig(testCtx, &lambda.PutProvisionedConcurrencyConfigInput{
		FunctionName: aws.String(functionName), Qualifier: aws.String(versionNumber),
		ProvisionedConcurrentExecutions: aws.Int32(2),
	})
	require.NoError(t, err)
	codeSigning, err := lambdaAPI.CreateCodeSigningConfig(testCtx, &lambda.CreateCodeSigningConfigInput{
		AllowedPublishers: &lambdatypes.AllowedPublishers{
			SigningProfileVersionArns: []string{"arn:aws:signer:us-east-1:123456789012:/signing-profiles/persistent/1"},
		},
	})
	require.NoError(t, err)
	codeSigningARN := aws.ToString(codeSigning.CodeSigningConfig.CodeSigningConfigArn)
	_, err = lambdaAPI.PutFunctionCodeSigningConfig(testCtx, &lambda.PutFunctionCodeSigningConfigInput{
		FunctionName: aws.String(functionName), CodeSigningConfigArn: aws.String(codeSigningARN),
	})
	require.NoError(t, err)
	_, err = lambdaAPI.PutRuntimeManagementConfig(testCtx, &lambda.PutRuntimeManagementConfigInput{
		FunctionName: aws.String(functionName), UpdateRuntimeOn: lambdatypes.UpdateRuntimeOnManual,
		RuntimeVersionArn: aws.String("arn:aws:lambda:us-east-1::runtime:persistent"),
	})
	require.NoError(t, err)
	_, err = lambdaAPI.PutFunctionRecursionConfig(testCtx, &lambda.PutFunctionRecursionConfigInput{
		FunctionName: aws.String(functionName), RecursiveLoop: lambdatypes.RecursiveLoopAllow,
	})
	require.NoError(t, err)
	layer, err := lambdaAPI.PublishLayerVersion(testCtx, &lambda.PublishLayerVersionInput{
		LayerName: aws.String("persistent-layer"),
		Content:   &lambdatypes.LayerVersionContentInput{ZipFile: lambdaDeploymentZip(t)},
	})
	require.NoError(t, err)
	_, err = lambdaAPI.AddLayerVersionPermission(testCtx, &lambda.AddLayerVersionPermissionInput{
		LayerName: aws.String("persistent-layer"), VersionNumber: aws.Int64(layer.Version),
		StatementId: aws.String("persistent-share"), Action: aws.String("lambda:GetLayerVersion"),
		Principal: aws.String("123456789012"),
	})
	require.NoError(t, err)
	queue, err := sqsAPI.CreateQueue(testCtx, &sqs.CreateQueueInput{
		QueueName: aws.String("persistent-event-source"),
	})
	require.NoError(t, err)
	queueAttributes, err := sqsAPI.GetQueueAttributes(testCtx, &sqs.GetQueueAttributesInput{
		QueueUrl: queue.QueueUrl, AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	require.NoError(t, err)
	eventSource, err := lambdaAPI.CreateEventSourceMapping(testCtx, &lambda.CreateEventSourceMappingInput{
		FunctionName:   aws.String(functionName),
		EventSourceArn: aws.String(queueAttributes.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]),
		BatchSize:      aws.Int32(5),
	})
	require.NoError(t, err)
	eventSourceUUID := aws.ToString(eventSource.UUID)
	const bucketName = "persistent-s3-state"
	_, err = s3API.CreateBucket(testCtx, &s3.CreateBucketInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err)
	_, err = s3API.PutBucketVersioning(testCtx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucketName),
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: s3types.BucketVersioningStatusEnabled,
		},
	})
	require.NoError(t, err)
	_, err = s3API.PutObject(testCtx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName), Key: aws.String("tagged.txt"),
		Body: bytes.NewReader([]byte("persisted object")),
	})
	require.NoError(t, err)
	_, err = s3API.PutObjectTagging(testCtx, &s3.PutObjectTaggingInput{
		Bucket: aws.String(bucketName), Key: aws.String("tagged.txt"),
		Tagging: &s3types.Tagging{TagSet: []s3types.Tag{{
			Key: aws.String("persistence"), Value: aws.String("verified"),
		}}},
	})
	require.NoError(t, err)
	multipart, err := s3API.CreateMultipartUpload(testCtx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(bucketName), Key: aws.String("multipart.txt"),
	})
	require.NoError(t, err)
	uploadedPart, err := s3API.UploadPart(testCtx, &s3.UploadPartInput{
		Bucket: aws.String(bucketName), Key: aws.String("multipart.txt"), UploadId: multipart.UploadId,
		PartNumber: aws.Int32(1), Body: bytes.NewReader([]byte("multipart survived restart")),
	})
	require.NoError(t, err)

	shutdownSimulator(cmd)
	cmd = startPersistentSimulator(t, stateDir, tcpPort, udpPort, "process")

	amplifyAPI = amplify.NewFromConfig(cfg, func(o *amplify.Options) { o.BaseEndpoint = aws.String(endpoint) })
	wafAPI = wafv2.NewFromConfig(cfg, func(o *wafv2.Options) { o.BaseEndpoint = aws.String(endpoint) })
	route53API = route53.NewFromConfig(cfg, func(o *route53.Options) { o.BaseEndpoint = aws.String(endpoint) })
	iamAPI = iam.NewFromConfig(cfg, func(o *iam.Options) { o.BaseEndpoint = aws.String(endpoint) })
	dynamoDBAPI = dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(endpoint) })
	stepFunctionsAPI = sfn.NewFromConfig(cfg, func(o *sfn.Options) { o.BaseEndpoint = aws.String(endpoint) })
	lambdaAPI = lambda.NewFromConfig(cfg, func(o *lambda.Options) { o.BaseEndpoint = aws.String(endpoint) })
	sqsAPI = sqs.NewFromConfig(cfg, func(o *sqs.Options) { o.BaseEndpoint = aws.String(endpoint) })
	s3API = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	persistedApp, err := amplifyAPI.GetApp(testCtx, &amplify.GetAppInput{AppId: aws.String(appID)})
	require.NoError(t, err)
	require.NotNil(t, persistedApp.App.WafConfiguration)
	assert.Equal(t, amplifytypes.WafStatusAssociationSuccess, persistedApp.App.WafConfiguration.WafStatus)
	assert.Equal(t, webACLARN, aws.ToString(persistedApp.App.WafConfiguration.WebAclArn))

	persistedAssociation, err := wafAPI.GetWebACLForResource(testCtx, &wafv2.GetWebACLForResourceInput{
		ResourceArn: aws.String(appARN),
	})
	require.NoError(t, err)
	require.NotNil(t, persistedAssociation.WebACL)
	assert.Equal(t, webACLARN, aws.ToString(persistedAssociation.WebACL.ARN))
	persistedResources, err := wafAPI.ListResourcesForWebACL(testCtx, &wafv2.ListResourcesForWebACLInput{
		WebACLArn: aws.String(webACLARN),
	})
	require.NoError(t, err)
	assert.Contains(t, persistedResources.ResourceArns, appARN)

	persistedZone, err := route53API.GetHostedZone(testCtx, &route53.GetHostedZoneInput{
		Id: aws.String(hostedZoneID),
	})
	require.NoError(t, err)
	require.Len(t, persistedZone.VPCs, 1)
	assert.Equal(t, "vpc-persistent-primary", aws.ToString(persistedZone.VPCs[0].VPCId))
	persistedTags, err := route53API.ListTagsForResource(testCtx, &route53.ListTagsForResourceInput{
		ResourceId:   aws.String(hostedZoneID),
		ResourceType: r53types.TagResourceTypeHostedzone,
	})
	require.NoError(t, err)
	require.Len(t, persistedTags.ResourceTagSet.Tags, 1)
	assert.Equal(t, "persistence", aws.ToString(persistedTags.ResourceTagSet.Tags[0].Key))
	persistedAuthorizations, err := route53API.ListVPCAssociationAuthorizations(testCtx, &route53.ListVPCAssociationAuthorizationsInput{
		HostedZoneId: aws.String(hostedZoneID),
	})
	require.NoError(t, err)
	require.Len(t, persistedAuthorizations.VPCs, 1)
	assert.Equal(t, "vpc-persistent-authorized", aws.ToString(persistedAuthorizations.VPCs[0].VPCId))

	persistedDeletion, err := iamAPI.GetServiceLinkedRoleDeletionStatus(testCtx, &iam.GetServiceLinkedRoleDeletionStatusInput{
		DeletionTaskId: aws.String(deletionTaskID),
	})
	require.NoError(t, err)
	assert.Equal(t, iamtypes.DeletionTaskStatusTypeSucceeded, persistedDeletion.Status)
	_, err = iamAPI.GetServiceLinkedRoleDeletionStatus(testCtx, &iam.GetServiceLinkedRoleDeletionStatusInput{
		DeletionTaskId: aws.String("task_unknown"),
	})
	require.Error(t, err)
	var unknownTask *iamtypes.NoSuchEntityException
	require.ErrorAs(t, err, &unknownTask)

	persistedItem, err := dynamoDBAPI.GetItem(testCtx, &dynamodb.GetItemInput{
		TableName: aws.String("persistent-table"),
		Key: map[string]ddbtypes.AttributeValue{
			"id": &ddbtypes.AttributeValueMemberS{Value: "item-1"},
		},
	})
	require.NoError(t, err)
	value, ok := persistedItem.Item["value"].(*ddbtypes.AttributeValueMemberS)
	require.True(t, ok)
	assert.Equal(t, "survived", value.Value)

	persistedMachine, err := stepFunctionsAPI.DescribeStateMachine(testCtx, &sfn.DescribeStateMachineInput{
		StateMachineArn: stateMachine.StateMachineArn,
	})
	require.NoError(t, err)
	assert.Equal(t, "persistent-state-machine", aws.ToString(persistedMachine.Name))
	persistedExecution, err := stepFunctionsAPI.DescribeExecution(testCtx, &sfn.DescribeExecutionInput{
		ExecutionArn: execution.ExecutionArn,
	})
	require.NoError(t, err)
	assert.Equal(t, sfntypes.ExecutionStatusSucceeded, persistedExecution.Status)
	persistedHistory, err := stepFunctionsAPI.GetExecutionHistory(testCtx, &sfn.GetExecutionHistoryInput{
		ExecutionArn: execution.ExecutionArn,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, persistedHistory.Events)
	require.Eventually(t, func() bool {
		out, describeErr := stepFunctionsAPI.DescribeExecution(testCtx, &sfn.DescribeExecutionInput{
			ExecutionArn: waitExecution.ExecutionArn,
		})
		return describeErr == nil && out.Status == sfntypes.ExecutionStatusSucceeded
	}, 2500*time.Millisecond, 50*time.Millisecond, "the persisted Wait state did not resume with its original deadline")

	persistedVersions, err := lambdaAPI.ListVersionsByFunction(testCtx, &lambda.ListVersionsByFunctionInput{
		FunctionName: aws.String(functionName),
	})
	require.NoError(t, err)
	require.Len(t, persistedVersions.Versions, 2)
	persistedAlias, err := lambdaAPI.GetAlias(testCtx, &lambda.GetAliasInput{
		FunctionName: aws.String(functionName), Name: aws.String("live"),
	})
	require.NoError(t, err)
	assert.Equal(t, versionNumber, aws.ToString(persistedAlias.FunctionVersion))
	persistedPolicy, err := lambdaAPI.GetPolicy(testCtx, &lambda.GetPolicyInput{
		FunctionName: aws.String(functionName),
	})
	require.NoError(t, err)
	assert.Contains(t, aws.ToString(persistedPolicy.Policy), "persistent-invoke")
	persistedURL, err := lambdaAPI.GetFunctionUrlConfig(testCtx, &lambda.GetFunctionUrlConfigInput{
		FunctionName: aws.String(functionName),
	})
	require.NoError(t, err)
	assert.Equal(t, aws.ToString(functionURL.FunctionUrl), aws.ToString(persistedURL.FunctionUrl))
	persistedInvokeConfig, err := lambdaAPI.GetFunctionEventInvokeConfig(testCtx, &lambda.GetFunctionEventInvokeConfigInput{
		FunctionName: aws.String(functionName),
	})
	require.NoError(t, err)
	assert.EqualValues(t, 1, aws.ToInt32(persistedInvokeConfig.MaximumRetryAttempts))
	persistedConcurrency, err := lambdaAPI.GetProvisionedConcurrencyConfig(testCtx, &lambda.GetProvisionedConcurrencyConfigInput{
		FunctionName: aws.String(functionName), Qualifier: aws.String(versionNumber),
	})
	require.NoError(t, err)
	assert.EqualValues(t, 2, aws.ToInt32(persistedConcurrency.AllocatedProvisionedConcurrentExecutions))
	persistedCodeSigning, err := lambdaAPI.GetFunctionCodeSigningConfig(testCtx, &lambda.GetFunctionCodeSigningConfigInput{
		FunctionName: aws.String(functionName),
	})
	require.NoError(t, err)
	assert.Equal(t, codeSigningARN, aws.ToString(persistedCodeSigning.CodeSigningConfigArn))
	persistedRuntime, err := lambdaAPI.GetRuntimeManagementConfig(testCtx, &lambda.GetRuntimeManagementConfigInput{
		FunctionName: aws.String(functionName),
	})
	require.NoError(t, err)
	assert.Equal(t, lambdatypes.UpdateRuntimeOnManual, persistedRuntime.UpdateRuntimeOn)
	persistedRecursion, err := lambdaAPI.GetFunctionRecursionConfig(testCtx, &lambda.GetFunctionRecursionConfigInput{
		FunctionName: aws.String(functionName),
	})
	require.NoError(t, err)
	assert.Equal(t, lambdatypes.RecursiveLoopAllow, persistedRecursion.RecursiveLoop)
	persistedLayerPolicy, err := lambdaAPI.GetLayerVersionPolicy(testCtx, &lambda.GetLayerVersionPolicyInput{
		LayerName: aws.String("persistent-layer"), VersionNumber: aws.Int64(layer.Version),
	})
	require.NoError(t, err)
	assert.Contains(t, aws.ToString(persistedLayerPolicy.Policy), "persistent-share")
	persistedEventSource, err := lambdaAPI.GetEventSourceMapping(testCtx, &lambda.GetEventSourceMappingInput{
		UUID: aws.String(eventSourceUUID),
	})
	require.NoError(t, err)
	assert.EqualValues(t, 5, aws.ToInt32(persistedEventSource.BatchSize))
	_, err = sqsAPI.GetQueueAttributes(testCtx, &sqs.GetQueueAttributesInput{
		QueueUrl: queue.QueueUrl, AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	require.NoError(t, err)
	persistedVersioning, err := s3API.GetBucketVersioning(testCtx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err)
	assert.Equal(t, s3types.BucketVersioningStatusEnabled, persistedVersioning.Status)
	persistedObjectTags, err := s3API.GetObjectTagging(testCtx, &s3.GetObjectTaggingInput{
		Bucket: aws.String(bucketName), Key: aws.String("tagged.txt"),
	})
	require.NoError(t, err)
	require.Len(t, persistedObjectTags.TagSet, 1)
	assert.Equal(t, "verified", aws.ToString(persistedObjectTags.TagSet[0].Value))
	persistedParts, err := s3API.ListParts(testCtx, &s3.ListPartsInput{
		Bucket: aws.String(bucketName), Key: aws.String("multipart.txt"), UploadId: multipart.UploadId,
	})
	require.NoError(t, err)
	require.Len(t, persistedParts.Parts, 1)
	_, err = s3API.CompleteMultipartUpload(testCtx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(bucketName), Key: aws.String("multipart.txt"), UploadId: multipart.UploadId,
		MultipartUpload: &s3types.CompletedMultipartUpload{Parts: []s3types.CompletedPart{{
			PartNumber: aws.Int32(1), ETag: uploadedPart.ETag,
		}}},
	})
	require.NoError(t, err)
	persistedMultipart, err := s3API.GetObject(testCtx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName), Key: aws.String("multipart.txt"),
	})
	require.NoError(t, err)
	persistedMultipartBody, err := io.ReadAll(persistedMultipart.Body)
	persistedMultipart.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, "multipart survived restart", string(persistedMultipartBody))
}

func TestAmplifyBuildResumesAfterSimulatorRestart_SDK(t *testing.T) {
	stateDir := t.TempDir()
	tcpPort, udpPort := persistentSimulatorPorts(t)
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", tcpPort)

	cmd := startPersistentSimulator(t, stateDir, tcpPort, udpPort, "docker")
	t.Cleanup(func() { shutdownSimulator(cmd) })

	repository := amplifyServeGitRepo(t, "main", map[string]string{
		"index.html": "<html>persistent Amplify build</html>",
	})
	buildSpec := `version: 1
frontend:
  phases:
    build:
      commands:
        - sleep 4
        - mkdir -p dist
        - cp index.html dist/index.html
  artifacts:
    baseDirectory: dist
    files:
      - '**/*'
`
	cfg := persistentSDKConfig()
	amplifyAPI := amplify.NewFromConfig(cfg, func(o *amplify.Options) { o.BaseEndpoint = aws.String(endpoint) })
	lambdaAPI := lambda.NewFromConfig(cfg, func(o *lambda.Options) { o.BaseEndpoint = aws.String(endpoint) })
	testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	app, err := amplifyAPI.CreateApp(testCtx, &amplify.CreateAppInput{
		Name:       aws.String("persistent-build-app"),
		Repository: aws.String(repository),
		BuildSpec:  aws.String(buildSpec),
	})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	_, err = amplifyAPI.CreateBranch(testCtx, &amplify.CreateBranchInput{
		AppId:      aws.String(appID),
		BranchName: aws.String("main"),
	})
	require.NoError(t, err)
	job, err := amplifyAPI.StartJob(testCtx, &amplify.StartJobInput{
		AppId:      aws.String(appID),
		BranchName: aws.String("main"),
		JobType:    amplifytypes.JobTypeRelease,
		CommitId:   aws.String("HEAD"),
	})
	require.NoError(t, err)
	jobID := aws.ToString(job.JobSummary.JobId)
	require.Eventually(t, func() bool {
		out, getErr := amplifyAPI.GetJob(testCtx, &amplify.GetJobInput{
			AppId: aws.String(appID), BranchName: aws.String("main"), JobId: aws.String(jobID),
		})
		return getErr == nil && out.Job.Summary.Status == amplifytypes.JobStatusRunning
	}, 30*time.Second, 100*time.Millisecond)

	const durableFunction = "persistent-durable-lambda"
	_, err = lambdaAPI.CreateFunction(testCtx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(durableFunction),
		Role:         aws.String("arn:aws:iam::123456789012:role/persistent-durable-role"),
		Runtime:      lambdatypes.RuntimeNodejs20x,
		Handler:      aws.String("index.handler"),
		Code: &lambdatypes.FunctionCode{
			ZipFile: lambdaNodeDeploymentZip(t, `({Status:"SUCCEEDED",Result:JSON.stringify({persisted:true})})`),
		},
		DurableConfig: &lambdatypes.DurableConfig{
			ExecutionTimeout: aws.Int32(60), RetentionPeriodInDays: aws.Int32(1),
		},
	})
	require.NoError(t, err)
	durable, err := lambdaAPI.Invoke(testCtx, &lambda.InvokeInput{
		FunctionName:         aws.String(durableFunction + ":$LATEST"),
		DurableExecutionName: aws.String("restart-journal"),
		Payload:              []byte(`{"request":"persist"}`),
	})
	require.NoError(t, err)
	durableARN := aws.ToString(durable.DurableExecutionArn)
	require.NotEmpty(t, durableARN)

	shutdownSimulator(cmd)
	cmd = startPersistentSimulator(t, stateDir, tcpPort, udpPort, "docker")
	amplifyAPI = amplify.NewFromConfig(cfg, func(o *amplify.Options) { o.BaseEndpoint = aws.String(endpoint) })
	lambdaAPI = lambda.NewFromConfig(cfg, func(o *lambda.Options) { o.BaseEndpoint = aws.String(endpoint) })

	require.Eventually(t, func() bool {
		out, getErr := amplifyAPI.GetJob(testCtx, &amplify.GetJobInput{
			AppId: aws.String(appID), BranchName: aws.String("main"), JobId: aws.String(jobID),
		})
		return getErr == nil && out.Job.Summary.Status == amplifytypes.JobStatusSucceed
	}, 45*time.Second, 200*time.Millisecond, "the persisted Amplify build did not resume")

	hostingRequest, err := http.NewRequestWithContext(testCtx, http.MethodGet, endpoint+"/index.html", nil)
	require.NoError(t, err)
	hostingRequest.Host = "main." + appID + ".amplifyapp.com"
	artifactResponse, err := http.DefaultClient.Do(hostingRequest)
	require.NoError(t, err)
	defer artifactResponse.Body.Close()
	require.Equal(t, http.StatusOK, artifactResponse.StatusCode)
	artifact, err := io.ReadAll(artifactResponse.Body)
	require.NoError(t, err)
	assert.Equal(t, "<html>persistent Amplify build</html>", string(artifact))
	persistedDurable, err := lambdaAPI.GetDurableExecution(testCtx, &lambda.GetDurableExecutionInput{
		DurableExecutionArn: aws.String(durableARN),
	})
	require.NoError(t, err)
	assert.Equal(t, lambdatypes.ExecutionStatusSucceeded, persistedDurable.Status)
	assert.JSONEq(t, `{"persisted":true}`, aws.ToString(persistedDurable.Result))
	persistedDurableHistory, err := lambdaAPI.GetDurableExecutionHistory(testCtx, &lambda.GetDurableExecutionHistoryInput{
		DurableExecutionArn: aws.String(durableARN),
	})
	require.NoError(t, err)
	require.Len(t, persistedDurableHistory.Events, 2)
}

func TestLambdaDurableCallbackResumesAfterSimulatorRestart_SDK(t *testing.T) {
	stateDir := t.TempDir()
	tcpPort, udpPort := persistentSimulatorPorts(t)
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", tcpPort)
	cmd := startPersistentSimulator(t, stateDir, tcpPort, udpPort, "docker")
	t.Cleanup(func() { shutdownSimulator(cmd) })

	cfg := persistentSDKConfig()
	lambdaAPI := lambda.NewFromConfig(cfg, func(o *lambda.Options) { o.BaseEndpoint = aws.String(endpoint) })
	testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const functionName = "persistent-callback-lambda"
	type runtimeCoordinates struct {
		CheckpointToken string `json:"checkpointToken"`
		DurableARN      string `json:"durableArn"`
	}
	coordinates := make(chan runtimeCoordinates, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var received runtimeCoordinates
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		coordinates <- received
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	containerReceiverURL := strings.Replace(receiver.URL, "127.0.0.1", "host.docker.internal", 1)
	source := fmt.Sprintf(`
exports.handler = async (event) => {
  const callback = event.InitialExecutionState.Operations.find(
    (operation) => operation.Type === "CALLBACK"
  );
  if (callback?.Status === "SUCCEEDED") {
    return {Status:"SUCCEEDED",Result:JSON.stringify({
      resumed:true,
      callback:JSON.parse(callback.CallbackDetails.Result)
    })};
  }
  await fetch(%q, {
    method:"POST",
    headers:{"content-type":"application/json"},
    body:JSON.stringify({
      checkpointToken:event.CheckpointToken,
      durableArn:event.DurableExecutionArn
    })
  });
  console.log("CHECKPOINT_TOKEN=" + event.CheckpointToken);
  console.log("DURABLE_ARN=" + event.DurableExecutionArn);
  return {Status:"PENDING"};
};`, containerReceiverURL)
	_, err := lambdaAPI.CreateFunction(testCtx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(functionName),
		Role:         aws.String("arn:aws:iam::123456789012:role/persistent-callback-role"),
		Runtime:      lambdatypes.RuntimeNodejs20x,
		Handler:      aws.String("index.handler"),
		Code:         &lambdatypes.FunctionCode{ZipFile: lambdaNodeSourceZip(t, source)},
		DurableConfig: &lambdatypes.DurableConfig{
			ExecutionTimeout: aws.Int32(60), RetentionPeriodInDays: aws.Int32(1),
		},
	})
	require.NoError(t, err)
	invocation, err := lambdaAPI.Invoke(testCtx, &lambda.InvokeInput{
		FunctionName:         aws.String(functionName + ":$LATEST"),
		DurableExecutionName: aws.String("restart-callback"),
		InvocationType:       lambdatypes.InvocationTypeEvent,
		Payload:              []byte(`{"request":"persist-callback"}`),
	})
	require.NoError(t, err)
	durableARN := aws.ToString(invocation.DurableExecutionArn)
	require.NotEmpty(t, durableARN)

	var received runtimeCoordinates
	select {
	case received = <-coordinates:
	case <-time.After(30 * time.Second):
		t.Fatal("the durable runtime did not publish its checkpoint coordinates")
	}
	require.Equal(t, durableARN, received.DurableARN)
	require.NotEmpty(t, received.CheckpointToken)

	checkpoint, err := lambdaAPI.CheckpointDurableExecution(testCtx, &lambda.CheckpointDurableExecutionInput{
		DurableExecutionArn: aws.String(durableARN), CheckpointToken: aws.String(received.CheckpointToken),
		Updates: []lambdatypes.OperationUpdate{{
			Id: aws.String("approval"), Name: aws.String("approval"),
			Type: lambdatypes.OperationTypeCallback, Action: lambdatypes.OperationActionStart,
			CallbackOptions: &lambdatypes.CallbackOptions{
				HeartbeatTimeoutSeconds: 30, TimeoutSeconds: 45,
			},
		}},
	})
	require.NoError(t, err)
	var callbackID string
	for _, operation := range checkpoint.NewExecutionState.Operations {
		if operation.CallbackDetails != nil {
			callbackID = aws.ToString(operation.CallbackDetails.CallbackId)
		}
	}
	require.NotEmpty(t, callbackID)

	shutdownSimulator(cmd)
	cmd = startPersistentSimulator(t, stateDir, tcpPort, udpPort, "docker")
	lambdaAPI = lambda.NewFromConfig(cfg, func(o *lambda.Options) { o.BaseEndpoint = aws.String(endpoint) })

	_, err = lambdaAPI.SendDurableExecutionCallbackHeartbeat(testCtx, &lambda.SendDurableExecutionCallbackHeartbeatInput{
		CallbackId: aws.String(callbackID),
	})
	require.NoError(t, err)
	_, err = lambdaAPI.SendDurableExecutionCallbackSuccess(testCtx, &lambda.SendDurableExecutionCallbackSuccessInput{
		CallbackId: aws.String(callbackID), Result: []byte(`{"by":"restart-test"}`),
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		execution, getErr := lambdaAPI.GetDurableExecution(testCtx, &lambda.GetDurableExecutionInput{
			DurableExecutionArn: aws.String(durableARN),
		})
		return getErr == nil && execution.Status == lambdatypes.ExecutionStatusSucceeded &&
			strings.Contains(aws.ToString(execution.Result), `"resumed":true`)
	}, 30*time.Second, 100*time.Millisecond, "the persisted callback did not resume the durable execution")
}
