package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCloudTrail_LookupEventsPaginationStableCursorSDK proves the LookupEvents
// NextToken is a stable cursor, not an absolute offset: an event ingested
// mid-pagination (head-insertion) must not cause a previously-returned EventId
// to reappear on a later page. With the old offset token, the prepend shifted
// every offset and page 2 overlapped page 1; the cursor resumes after the
// last-seen (EventTime, EventId) key, so each event is returned exactly once.
func TestCloudTrail_LookupEventsPaginationStableCursorSDK(t *testing.T) {
	ct := cloudTrailClient()
	ec2c := ec2Client()

	// Seed several recorded events at the head of the trail.
	for i := 0; i < 8; i++ {
		_, err := ec2c.CreateVolume(ctx, &ec2.CreateVolumeInput{
			AvailabilityZone: aws.String("us-east-1a"), Size: aws.Int32(1),
		})
		require.NoError(t, err)
	}

	seen := map[string]int{}
	var token *string
	pages := 0
	for pages < 12 {
		out, err := ct.LookupEvents(ctx, &cloudtrail.LookupEventsInput{
			MaxResults: aws.Int32(3),
			NextToken:  token,
		})
		require.NoError(t, err)
		for _, e := range out.Events {
			seen[aws.ToString(e.EventId)]++
		}
		pages++

		// Inject one head-insertion after the first page — the exact race the
		// old absolute-offset token mishandled.
		if pages == 1 {
			_, err := ec2c.CreateVolume(ctx, &ec2.CreateVolumeInput{
				AvailabilityZone: aws.String("us-east-1a"), Size: aws.Int32(1),
			})
			require.NoError(t, err)
		}

		if out.NextToken == nil || len(out.Events) == 0 {
			break
		}
		token = out.NextToken
	}

	require.GreaterOrEqual(t, pages, 2, "test must span multiple pages")
	for id, n := range seen {
		assert.Equalf(t, 1, n, "EventId %s returned on %d pages — the cursor must yield each event exactly once", id, n)
	}
}
