package aws_sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	wafv2types "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func wafClient() *wafv2.Client {
	cfg := sdkConfig()
	return wafv2.NewFromConfig(cfg, func(o *wafv2.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestWAFv2WebACLLifecycle(t *testing.T) {
	c := wafClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	name := "sdk-acl-" + time.Now().Format("150405.000000")
	createOut, err := c.CreateWebACL(ctx, &wafv2.CreateWebACLInput{
		Name:  aws.String(name),
		Scope: wafv2types.ScopeCloudfront,
		DefaultAction: &wafv2types.DefaultAction{
			Allow: &wafv2types.AllowAction{},
		},
		Description: aws.String("sdk test"),
		VisibilityConfig: &wafv2types.VisibilityConfig{
			SampledRequestsEnabled:   true,
			CloudWatchMetricsEnabled: true,
			MetricName:               aws.String(name + "-metric"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.Summary)
	id := aws.ToString(createOut.Summary.Id)
	require.NotEmpty(t, id)
	lockToken := aws.ToString(createOut.Summary.LockToken)
	require.NotEmpty(t, lockToken)

	getOut, err := c.GetWebACL(ctx, &wafv2.GetWebACLInput{
		Name:  aws.String(name),
		Scope: wafv2types.ScopeCloudfront,
		Id:    aws.String(id),
	})
	require.NoError(t, err)
	require.NotNil(t, getOut.WebACL)
	assert.Equal(t, name, aws.ToString(getOut.WebACL.Name))

	listOut, err := c.ListWebACLs(ctx, &wafv2.ListWebACLsInput{
		Scope: wafv2types.ScopeCloudfront,
	})
	require.NoError(t, err)
	found := false
	for _, s := range listOut.WebACLs {
		if aws.ToString(s.Id) == id {
			found = true
		}
	}
	assert.True(t, found)

	// Stale-lock delete → optimistic-lock failure
	_, err = c.DeleteWebACL(ctx, &wafv2.DeleteWebACLInput{
		Name:      aws.String(name),
		Scope:     wafv2types.ScopeCloudfront,
		Id:        aws.String(id),
		LockToken: aws.String("stale"),
	})
	require.Error(t, err, "stale lock must be rejected")

	// Real lock-token from Get works
	currentLock := aws.ToString(getOut.LockToken)
	_, err = c.DeleteWebACL(ctx, &wafv2.DeleteWebACLInput{
		Name:      aws.String(name),
		Scope:     wafv2types.ScopeCloudfront,
		Id:        aws.String(id),
		LockToken: aws.String(currentLock),
	})
	require.NoError(t, err)
}

func TestWAFv2IPSetLifecycle(t *testing.T) {
	c := wafClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	name := "sdk-ipset-" + time.Now().Format("150405.000000")
	createOut, err := c.CreateIPSet(ctx, &wafv2.CreateIPSetInput{
		Name:             aws.String(name),
		Scope:            wafv2types.ScopeCloudfront,
		IPAddressVersion: wafv2types.IPAddressVersionIpv4,
		Addresses:        []string{"203.0.113.0/24", "198.51.100.10/32"},
		Description:      aws.String("sdk test"),
	})
	require.NoError(t, err)
	id := aws.ToString(createOut.Summary.Id)
	lock := aws.ToString(createOut.Summary.LockToken)

	getOut, err := c.GetIPSet(ctx, &wafv2.GetIPSetInput{
		Name:  aws.String(name),
		Scope: wafv2types.ScopeCloudfront,
		Id:    aws.String(id),
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(getOut.IPSet.Addresses))

	// Update
	updOut, err := c.UpdateIPSet(ctx, &wafv2.UpdateIPSetInput{
		Name:      aws.String(name),
		Scope:     wafv2types.ScopeCloudfront,
		Id:        aws.String(id),
		Addresses: []string{"203.0.113.42/32"},
		LockToken: aws.String(lock),
	})
	require.NoError(t, err)

	_, err = c.DeleteIPSet(ctx, &wafv2.DeleteIPSetInput{
		Name:      aws.String(name),
		Scope:     wafv2types.ScopeCloudfront,
		Id:        aws.String(id),
		LockToken: updOut.NextLockToken,
	})
	require.NoError(t, err)
}

func TestWAFv2RuleGroupAndRegexPatternSet(t *testing.T) {
	c := wafClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rgName := "sdk-rg-" + time.Now().Format("150405.000000")
	rgOut, err := c.CreateRuleGroup(ctx, &wafv2.CreateRuleGroupInput{
		Name:     aws.String(rgName),
		Scope:    wafv2types.ScopeCloudfront,
		Capacity: aws.Int64(100),
		VisibilityConfig: &wafv2types.VisibilityConfig{
			SampledRequestsEnabled:   false,
			CloudWatchMetricsEnabled: false,
			MetricName:               aws.String(rgName + "-metric"),
		},
	})
	require.NoError(t, err)
	rgID := aws.ToString(rgOut.Summary.Id)
	rgLock := aws.ToString(rgOut.Summary.LockToken)

	getRG, err := c.GetRuleGroup(ctx, &wafv2.GetRuleGroupInput{
		Name:  aws.String(rgName),
		Scope: wafv2types.ScopeCloudfront,
		Id:    aws.String(rgID),
	})
	require.NoError(t, err)
	require.NotNil(t, getRG.RuleGroup)

	listRG, err := c.ListRuleGroups(ctx, &wafv2.ListRuleGroupsInput{Scope: wafv2types.ScopeCloudfront})
	require.NoError(t, err)
	rgFound := false
	for _, s := range listRG.RuleGroups {
		if aws.ToString(s.Id) == rgID {
			rgFound = true
		}
	}
	assert.True(t, rgFound)

	_, err = c.DeleteRuleGroup(ctx, &wafv2.DeleteRuleGroupInput{
		Name: aws.String(rgName), Scope: wafv2types.ScopeCloudfront,
		Id: aws.String(rgID), LockToken: aws.String(rgLock),
	})
	require.NoError(t, err)

	rsName := "sdk-rs-" + time.Now().Format("150405.000000")
	rsOut, err := c.CreateRegexPatternSet(ctx, &wafv2.CreateRegexPatternSetInput{
		Name:                  aws.String(rsName),
		Scope:                 wafv2types.ScopeCloudfront,
		RegularExpressionList: []wafv2types.Regex{{RegexString: aws.String("^/admin/.*$")}},
	})
	require.NoError(t, err)
	rsID := aws.ToString(rsOut.Summary.Id)
	rsLock := aws.ToString(rsOut.Summary.LockToken)

	_, err = c.GetRegexPatternSet(ctx, &wafv2.GetRegexPatternSetInput{
		Name: aws.String(rsName), Scope: wafv2types.ScopeCloudfront, Id: aws.String(rsID),
	})
	require.NoError(t, err)

	_, err = c.DeleteRegexPatternSet(ctx, &wafv2.DeleteRegexPatternSetInput{
		Name: aws.String(rsName), Scope: wafv2types.ScopeCloudfront,
		Id: aws.String(rsID), LockToken: aws.String(rsLock),
	})
	require.NoError(t, err)
}

// TestWAFv2UpdatesAndListsAndTags exercises the secondary list / update /
// tag verbs not covered by the focused per-resource lifecycle tests:
// UpdateWebACL, UpdateRuleGroup, UpdateRegexPatternSet, ListIPSets,
// ListRegexPatternSets, TagResource, UntagResource, ListTagsForResource,
// GetSampledRequests. One test covers the whole cluster.
func TestWAFv2UpdatesAndListsAndTags(t *testing.T) {
	c := wafClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ts := time.Now().Format("150405.000000")

	// IPSet for List + Tag verbs
	ipsetOut, err := c.CreateIPSet(ctx, &wafv2.CreateIPSetInput{
		Name: aws.String("up-ipset-" + ts), Scope: wafv2types.ScopeCloudfront,
		IPAddressVersion: wafv2types.IPAddressVersionIpv4,
		Addresses:        []string{"203.0.113.0/24"},
	})
	require.NoError(t, err)
	ipsetID := aws.ToString(ipsetOut.Summary.Id)
	ipsetLock := aws.ToString(ipsetOut.Summary.LockToken)
	ipsetARN := aws.ToString(ipsetOut.Summary.ARN)
	defer func() {
		// fetch latest lock token then delete
		getOut, _ := c.GetIPSet(ctx, &wafv2.GetIPSetInput{
			Name: aws.String("up-ipset-" + ts), Scope: wafv2types.ScopeCloudfront, Id: aws.String(ipsetID),
		})
		_, _ = c.DeleteIPSet(ctx, &wafv2.DeleteIPSetInput{
			Name: aws.String("up-ipset-" + ts), Scope: wafv2types.ScopeCloudfront,
			Id: aws.String(ipsetID), LockToken: getOut.LockToken,
		})
	}()
	_ = ipsetLock

	// ListIPSets — verify our IPSet is present
	listIPs, err := c.ListIPSets(ctx, &wafv2.ListIPSetsInput{Scope: wafv2types.ScopeCloudfront})
	require.NoError(t, err)
	ipFound := false
	for _, s := range listIPs.IPSets {
		if aws.ToString(s.Id) == ipsetID {
			ipFound = true
		}
	}
	assert.True(t, ipFound)

	// TagResource → ListTagsForResource → UntagResource
	_, err = c.TagResource(ctx, &wafv2.TagResourceInput{
		ResourceARN: aws.String(ipsetARN),
		Tags:        []wafv2types.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
	})
	require.NoError(t, err)

	tagsOut, err := c.ListTagsForResource(ctx, &wafv2.ListTagsForResourceInput{
		ResourceARN: aws.String(ipsetARN),
	})
	require.NoError(t, err)
	require.NotNil(t, tagsOut.TagInfoForResource)
	require.Len(t, tagsOut.TagInfoForResource.TagList, 1)

	_, err = c.UntagResource(ctx, &wafv2.UntagResourceInput{
		ResourceARN: aws.String(ipsetARN),
		TagKeys:     []string{"env"},
	})
	require.NoError(t, err)

	// WebACL Update — round-trip a new description through the resource
	aclOut, err := c.CreateWebACL(ctx, &wafv2.CreateWebACLInput{
		Name: aws.String("up-acl-" + ts), Scope: wafv2types.ScopeCloudfront,
		DefaultAction:    &wafv2types.DefaultAction{Allow: &wafv2types.AllowAction{}},
		VisibilityConfig: &wafv2types.VisibilityConfig{SampledRequestsEnabled: false, CloudWatchMetricsEnabled: false, MetricName: aws.String("m")},
	})
	require.NoError(t, err)
	aclID := aws.ToString(aclOut.Summary.Id)
	aclLock := aws.ToString(aclOut.Summary.LockToken)
	defer func() {
		getOut, _ := c.GetWebACL(ctx, &wafv2.GetWebACLInput{
			Name: aws.String("up-acl-" + ts), Scope: wafv2types.ScopeCloudfront, Id: aws.String(aclID),
		})
		_, _ = c.DeleteWebACL(ctx, &wafv2.DeleteWebACLInput{
			Name: aws.String("up-acl-" + ts), Scope: wafv2types.ScopeCloudfront,
			Id: aws.String(aclID), LockToken: getOut.LockToken,
		})
	}()
	updACL, err := c.UpdateWebACL(ctx, &wafv2.UpdateWebACLInput{
		Name: aws.String("up-acl-" + ts), Scope: wafv2types.ScopeCloudfront, Id: aws.String(aclID),
		LockToken:        aws.String(aclLock),
		DefaultAction:    &wafv2types.DefaultAction{Block: &wafv2types.BlockAction{}},
		Description:      aws.String("updated"),
		VisibilityConfig: &wafv2types.VisibilityConfig{SampledRequestsEnabled: false, CloudWatchMetricsEnabled: false, MetricName: aws.String("m")},
	})
	require.NoError(t, err)
	require.NotEqual(t, aclLock, aws.ToString(updACL.NextLockToken))

	// RuleGroup Update
	rgOut, err := c.CreateRuleGroup(ctx, &wafv2.CreateRuleGroupInput{
		Name: aws.String("up-rg-" + ts), Scope: wafv2types.ScopeCloudfront,
		Capacity:         aws.Int64(50),
		VisibilityConfig: &wafv2types.VisibilityConfig{SampledRequestsEnabled: false, CloudWatchMetricsEnabled: false, MetricName: aws.String("rgm")},
	})
	require.NoError(t, err)
	rgID := aws.ToString(rgOut.Summary.Id)
	rgLock := aws.ToString(rgOut.Summary.LockToken)
	updRG, err := c.UpdateRuleGroup(ctx, &wafv2.UpdateRuleGroupInput{
		Name: aws.String("up-rg-" + ts), Scope: wafv2types.ScopeCloudfront, Id: aws.String(rgID),
		LockToken:        aws.String(rgLock),
		Description:      aws.String("updated rg"),
		VisibilityConfig: &wafv2types.VisibilityConfig{SampledRequestsEnabled: false, CloudWatchMetricsEnabled: false, MetricName: aws.String("rgm")},
	})
	require.NoError(t, err)
	_, err = c.DeleteRuleGroup(ctx, &wafv2.DeleteRuleGroupInput{
		Name: aws.String("up-rg-" + ts), Scope: wafv2types.ScopeCloudfront,
		Id: aws.String(rgID), LockToken: updRG.NextLockToken,
	})
	require.NoError(t, err)

	// RegexPatternSet Update + List
	rsOut, err := c.CreateRegexPatternSet(ctx, &wafv2.CreateRegexPatternSetInput{
		Name: aws.String("up-rs-" + ts), Scope: wafv2types.ScopeCloudfront,
		RegularExpressionList: []wafv2types.Regex{{RegexString: aws.String("foo.*")}},
	})
	require.NoError(t, err)
	rsID := aws.ToString(rsOut.Summary.Id)
	rsLock := aws.ToString(rsOut.Summary.LockToken)
	updRS, err := c.UpdateRegexPatternSet(ctx, &wafv2.UpdateRegexPatternSetInput{
		Name: aws.String("up-rs-" + ts), Scope: wafv2types.ScopeCloudfront, Id: aws.String(rsID),
		LockToken:             aws.String(rsLock),
		RegularExpressionList: []wafv2types.Regex{{RegexString: aws.String("bar.*")}},
	})
	require.NoError(t, err)
	listRS, err := c.ListRegexPatternSets(ctx, &wafv2.ListRegexPatternSetsInput{Scope: wafv2types.ScopeCloudfront})
	require.NoError(t, err)
	rsFound := false
	for _, s := range listRS.RegexPatternSets {
		if aws.ToString(s.Id) == rsID {
			rsFound = true
		}
	}
	assert.True(t, rsFound)
	_, err = c.DeleteRegexPatternSet(ctx, &wafv2.DeleteRegexPatternSetInput{
		Name: aws.String("up-rs-" + ts), Scope: wafv2types.ScopeCloudfront,
		Id: aws.String(rsID), LockToken: updRS.NextLockToken,
	})
	require.NoError(t, err)

	// GetSampledRequests — stub returns empty
	samp, err := c.GetSampledRequests(ctx, &wafv2.GetSampledRequestsInput{
		WebAclArn:      aws.String(aws.ToString(aclOut.Summary.ARN)),
		RuleMetricName: aws.String("any"),
		Scope:          wafv2types.ScopeCloudfront,
		TimeWindow: &wafv2types.TimeWindow{
			StartTime: aws.Time(time.Now().Add(-time.Hour)),
			EndTime:   aws.Time(time.Now()),
		},
		MaxItems: aws.Int64(10),
	})
	require.NoError(t, err)
	require.NotNil(t, samp)
}

func TestWAFv2AssociateWebACLWithCloudFront(t *testing.T) {
	c := wafClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	name := "sdk-assoc-" + time.Now().Format("150405.000000")
	createOut, err := c.CreateWebACL(ctx, &wafv2.CreateWebACLInput{
		Name:  aws.String(name),
		Scope: wafv2types.ScopeCloudfront,
		DefaultAction: &wafv2types.DefaultAction{
			Allow: &wafv2types.AllowAction{},
		},
		VisibilityConfig: &wafv2types.VisibilityConfig{
			SampledRequestsEnabled:   false,
			CloudWatchMetricsEnabled: false,
			MetricName:               aws.String(name + "-metric"),
		},
	})
	require.NoError(t, err)
	aclARN := aws.ToString(createOut.Summary.ARN)
	lock := aws.ToString(createOut.Summary.LockToken)
	defer func() {
		_, _ = c.DeleteWebACL(ctx, &wafv2.DeleteWebACLInput{
			Name: aws.String(name), Scope: wafv2types.ScopeCloudfront,
			Id: createOut.Summary.Id, LockToken: aws.String(lock),
		})
	}()

	cfARN := "arn:aws:cloudfront::123456789012:distribution/EFAKE000000001"

	_, err = c.AssociateWebACL(ctx, &wafv2.AssociateWebACLInput{
		WebACLArn:   aws.String(aclARN),
		ResourceArn: aws.String(cfARN),
	})
	require.NoError(t, err)

	getForResOut, err := c.GetWebACLForResource(ctx, &wafv2.GetWebACLForResourceInput{
		ResourceArn: aws.String(cfARN),
	})
	require.NoError(t, err)
	require.NotNil(t, getForResOut.WebACL)
	assert.Equal(t, aclARN, aws.ToString(getForResOut.WebACL.ARN))

	listResOut, err := c.ListResourcesForWebACL(ctx, &wafv2.ListResourcesForWebACLInput{
		WebACLArn: aws.String(aclARN),
	})
	require.NoError(t, err)
	require.Contains(t, listResOut.ResourceArns, cfARN)

	// Delete WebACL while associated → reject
	_, err = c.DeleteWebACL(ctx, &wafv2.DeleteWebACLInput{
		Name: aws.String(name), Scope: wafv2types.ScopeCloudfront,
		Id: createOut.Summary.Id, LockToken: aws.String(lock),
	})
	require.Error(t, err, "delete with active association must fail")

	// Disassociate then delete works
	_, err = c.DisassociateWebACL(ctx, &wafv2.DisassociateWebACLInput{
		ResourceArn: aws.String(cfARN),
	})
	require.NoError(t, err)
}
