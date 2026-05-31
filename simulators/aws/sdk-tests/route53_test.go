package aws_sdk_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func r53Client() *route53.Client {
	cfg := sdkConfig()
	return route53.NewFromConfig(cfg, func(o *route53.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestRoute53HostedZoneLifecycle(t *testing.T) {
	c := r53Client()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	caller := "sdk-test-" + time.Now().Format("150405.000000")
	createOut, err := c.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
		Name:            aws.String("sdk-route53-test.local"),
		CallerReference: aws.String(caller),
		HostedZoneConfig: &r53types.HostedZoneConfig{
			Comment:     aws.String("sdk test"),
			PrivateZone: false,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.HostedZone)
	zoneID := strings.TrimPrefix(aws.ToString(createOut.HostedZone.Id), "/hostedzone/")
	require.NotEmpty(t, zoneID)
	assert.Equal(t, "INSYNC", string(createOut.ChangeInfo.Status))

	// List
	listOut, err := c.ListHostedZones(ctx, &route53.ListHostedZonesInput{})
	require.NoError(t, err)
	found := false
	for _, z := range listOut.HostedZones {
		if strings.TrimPrefix(aws.ToString(z.Id), "/hostedzone/") == zoneID {
			found = true
		}
	}
	assert.True(t, found, "expected new zone %q in list", zoneID)

	// Get
	getOut, err := c.GetHostedZone(ctx, &route53.GetHostedZoneInput{Id: aws.String(zoneID)})
	require.NoError(t, err)
	assert.Equal(t, "sdk-route53-test.local.", aws.ToString(getOut.HostedZone.Name))

	// ListResourceRecordSets — initial zone has NS + SOA
	rrOut, err := c.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
	})
	require.NoError(t, err)
	require.Len(t, rrOut.ResourceRecordSets, 2)

	// Delete
	_, err = c.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{Id: aws.String(zoneID)})
	require.NoError(t, err)

	// Get after delete should fail
	_, err = c.GetHostedZone(ctx, &route53.GetHostedZoneInput{Id: aws.String(zoneID)})
	require.Error(t, err)
}

func TestRoute53ListHostedZonesByName(t *testing.T) {
	c := r53Client()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := strings.ReplaceAll(time.Now().Format("150405.000000"), ".", "-")
	alphaName := "alpha.r53-by-name-" + suffix + ".example.com"
	betaName := "beta.r53-by-name-" + suffix + ".example.com"

	createZone := func(name string) string {
		out, err := c.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
			Name:            aws.String(name),
			CallerReference: aws.String("sdk-list-by-name-" + name),
		})
		require.NoError(t, err)
		require.NotNil(t, out.HostedZone)
		return strings.TrimPrefix(aws.ToString(out.HostedZone.Id), "/hostedzone/")
	}

	alphaID := createZone(alphaName)
	betaID := createZone(betaName)
	defer func() {
		_, _ = c.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{Id: aws.String(alphaID)})
		_, _ = c.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{Id: aws.String(betaID)})
	}()

	startAtBeta, err := c.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
		DNSName:  aws.String(betaName),
		MaxItems: aws.Int32(10),
	})
	require.NoError(t, err)
	require.NotEmpty(t, startAtBeta.HostedZones)
	assert.Equal(t, betaName+".", aws.ToString(startAtBeta.HostedZones[0].Name))

	firstPage, err := c.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
		DNSName:  aws.String(alphaName),
		MaxItems: aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, firstPage.HostedZones, 1)
	assert.Equal(t, alphaName+".", aws.ToString(firstPage.HostedZones[0].Name))
	require.True(t, firstPage.IsTruncated)
	require.Equal(t, betaName+".", aws.ToString(firstPage.NextDNSName))
	require.Equal(t, betaID, aws.ToString(firstPage.NextHostedZoneId))

	secondPage, err := c.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
		DNSName:      firstPage.NextDNSName,
		HostedZoneId: firstPage.NextHostedZoneId,
		MaxItems:     aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, secondPage.HostedZones, 1)
	assert.Equal(t, betaName+".", aws.ToString(secondPage.HostedZones[0].Name))
}

func TestRoute53RecordCRUD(t *testing.T) {
	c := r53Client()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	caller := "sdk-rr-" + time.Now().Format("150405.000000")
	zoneOut, err := c.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
		Name:            aws.String("rr-test.local"),
		CallerReference: aws.String(caller),
	})
	require.NoError(t, err)
	zoneID := strings.TrimPrefix(aws.ToString(zoneOut.HostedZone.Id), "/hostedzone/")
	defer func() {
		// Cleanup A record, then zone.
		_, _ = c.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(zoneID),
			ChangeBatch: &r53types.ChangeBatch{
				Changes: []r53types.Change{{
					Action: r53types.ChangeActionDelete,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name:            aws.String("api.rr-test.local."),
						Type:            r53types.RRTypeA,
						TTL:             aws.Int64(300),
						ResourceRecords: []r53types.ResourceRecord{{Value: aws.String("203.0.113.1")}},
					},
				}},
			},
		})
		_, _ = c.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{Id: aws.String(zoneID)})
	}()

	// CREATE
	changeOut, err := c.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{{
				Action: r53types.ChangeActionCreate,
				ResourceRecordSet: &r53types.ResourceRecordSet{
					Name:            aws.String("api.rr-test.local."),
					Type:            r53types.RRTypeA,
					TTL:             aws.Int64(300),
					ResourceRecords: []r53types.ResourceRecord{{Value: aws.String("203.0.113.1")}},
				},
			}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, r53types.ChangeStatusInsync, changeOut.ChangeInfo.Status)

	// Duplicate CREATE → error
	_, err = c.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{{
				Action: r53types.ChangeActionCreate,
				ResourceRecordSet: &r53types.ResourceRecordSet{
					Name:            aws.String("api.rr-test.local."),
					Type:            r53types.RRTypeA,
					TTL:             aws.Int64(300),
					ResourceRecords: []r53types.ResourceRecord{{Value: aws.String("203.0.113.2")}},
				},
			}},
		},
	})
	require.Error(t, err, "duplicate CREATE must fail")

	// UPSERT
	_, err = c.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{{
				Action: r53types.ChangeActionUpsert,
				ResourceRecordSet: &r53types.ResourceRecordSet{
					Name:            aws.String("api.rr-test.local."),
					Type:            r53types.RRTypeA,
					TTL:             aws.Int64(600),
					ResourceRecords: []r53types.ResourceRecord{{Value: aws.String("203.0.113.2")}},
				},
			}},
		},
	})
	require.NoError(t, err)

	// Verify via List
	rrOut, err := c.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
	})
	require.NoError(t, err)
	found := false
	for _, r := range rrOut.ResourceRecordSets {
		if aws.ToString(r.Name) == "api.rr-test.local." && r.Type == r53types.RRTypeA {
			found = true
			assert.Equal(t, int64(600), aws.ToInt64(r.TTL))
			require.Len(t, r.ResourceRecords, 1)
			assert.Equal(t, "203.0.113.2", aws.ToString(r.ResourceRecords[0].Value))
		}
	}
	assert.True(t, found, "expected updated A record in list")

	// GetChange — sim returns INSYNC for any change ID
	getChangeOut, err := c.GetChange(ctx, &route53.GetChangeInput{Id: changeOut.ChangeInfo.Id})
	require.NoError(t, err)
	assert.Equal(t, r53types.ChangeStatusInsync, getChangeOut.ChangeInfo.Status)
}

func TestRoute53ListResourceRecordSetsSortedCursor(t *testing.T) {
	c := r53Client()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	zoneOut, err := c.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
		Name:            aws.String("cursor-sort.example.com."),
		CallerReference: aws.String("sdk-rr-cursor-" + time.Now().Format("150405.000000")),
	})
	require.NoError(t, err)
	zoneID := strings.TrimPrefix(aws.ToString(zoneOut.HostedZone.Id), "/hostedzone/")
	defer func() {
		_, _ = c.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(zoneID),
			ChangeBatch: &r53types.ChangeBatch{
				Changes: []r53types.Change{{
					Action: r53types.ChangeActionDelete,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name:            aws.String("api.cursor-sort.example.com."),
						Type:            r53types.RRTypeA,
						TTL:             aws.Int64(300),
						ResourceRecords: []r53types.ResourceRecord{{Value: aws.String("192.0.2.10")}},
					},
				}},
			},
		})
		_, _ = c.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{Id: aws.String(zoneID)})
	}()

	_, err = c.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{{
				Action: r53types.ChangeActionUpsert,
				ResourceRecordSet: &r53types.ResourceRecordSet{
					Name:            aws.String("api.cursor-sort.example.com."),
					Type:            r53types.RRTypeA,
					TTL:             aws.Int64(300),
					ResourceRecords: []r53types.ResourceRecord{{Value: aws.String("192.0.2.10")}},
				},
			}},
		},
	})
	require.NoError(t, err)

	page, err := c.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(zoneID),
		StartRecordName: aws.String("api.cursor-sort.example.com."),
		StartRecordType: r53types.RRTypeA,
		MaxItems:        aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, page.ResourceRecordSets, 1)
	require.Equal(t, "api.cursor-sort.example.com.", aws.ToString(page.ResourceRecordSets[0].Name))
	require.Equal(t, r53types.RRTypeA, page.ResourceRecordSets[0].Type)
	require.False(t, page.IsTruncated)

	firstApexPage, err := c.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(zoneID),
		StartRecordName: aws.String("cursor-sort.example.com."),
		MaxItems:        aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, firstApexPage.ResourceRecordSets, 1)
	require.Equal(t, "cursor-sort.example.com.", aws.ToString(firstApexPage.ResourceRecordSets[0].Name))
	require.Equal(t, r53types.RRTypeNs, firstApexPage.ResourceRecordSets[0].Type)
	require.True(t, firstApexPage.IsTruncated)
	require.Equal(t, "cursor-sort.example.com.", aws.ToString(firstApexPage.NextRecordName))
	require.Equal(t, r53types.RRTypeSoa, firstApexPage.NextRecordType)

	secondApexPage, err := c.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(zoneID),
		StartRecordName: firstApexPage.NextRecordName,
		StartRecordType: firstApexPage.NextRecordType,
		MaxItems:        aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, secondApexPage.ResourceRecordSets, 1)
	require.Equal(t, "cursor-sort.example.com.", aws.ToString(secondApexPage.ResourceRecordSets[0].Name))
	require.Equal(t, r53types.RRTypeSoa, secondApexPage.ResourceRecordSets[0].Type)
}

func TestRoute53AliasTargetForCloudFront(t *testing.T) {
	c := r53Client()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	caller := "alias-cf-" + time.Now().Format("150405.000000")
	zoneOut, err := c.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
		Name:            aws.String("alias-cf-test.local"),
		CallerReference: aws.String(caller),
	})
	require.NoError(t, err)
	zoneID := strings.TrimPrefix(aws.ToString(zoneOut.HostedZone.Id), "/hostedzone/")
	defer func() {
		_, _ = c.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(zoneID),
			ChangeBatch: &r53types.ChangeBatch{
				Changes: []r53types.Change{{
					Action: r53types.ChangeActionDelete,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String("cdn.alias-cf-test.local."),
						Type: r53types.RRTypeA,
						AliasTarget: &r53types.AliasTarget{
							HostedZoneId:         aws.String("Z2FDTNDATAQYW2"),
							DNSName:              aws.String("d111111abcdef8.cloudfront.net."),
							EvaluateTargetHealth: false,
						},
					},
				}},
			},
		})
		_, _ = c.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{Id: aws.String(zoneID)})
	}()

	// ALIAS record targeting a CloudFront distribution domain name. The
	// HostedZoneId Z2FDTNDATAQYW2 is the CloudFront-fixed zone for every
	// distribution; the DNSName is the value the SDK would resolve from
	// `aws_cloudfront_distribution.x.domain_name` in Terraform.
	_, err = c.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{{
				Action: r53types.ChangeActionCreate,
				ResourceRecordSet: &r53types.ResourceRecordSet{
					Name: aws.String("cdn.alias-cf-test.local."),
					Type: r53types.RRTypeA,
					AliasTarget: &r53types.AliasTarget{
						HostedZoneId:         aws.String("Z2FDTNDATAQYW2"),
						DNSName:              aws.String("d111111abcdef8.cloudfront.net."),
						EvaluateTargetHealth: false,
					},
				},
			}},
		},
	})
	require.NoError(t, err)

	// Verify the AliasTarget round-trips through XML decode/encode
	rrOut, err := c.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
	})
	require.NoError(t, err)
	found := false
	for _, r := range rrOut.ResourceRecordSets {
		if aws.ToString(r.Name) == "cdn.alias-cf-test.local." {
			require.NotNil(t, r.AliasTarget, "AliasTarget must round-trip")
			assert.Equal(t, "Z2FDTNDATAQYW2", aws.ToString(r.AliasTarget.HostedZoneId))
			assert.Equal(t, "d111111abcdef8.cloudfront.net.", aws.ToString(r.AliasTarget.DNSName))
			found = true
		}
	}
	assert.True(t, found)
}
