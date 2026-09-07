package ecs

import "testing"

func TestECSStopTimeoutFollowsDocker(t *testing.T) {
	five, zero, huge := 5, 0, 600
	cases := []struct {
		in   *int
		want int32
	}{
		{nil, 10},
		{&five, 5},
		{&zero, 2},
		{&huge, 120},
	}
	for _, c := range cases {
		if got := ecsStopTimeout(c.in); got != c.want {
			t.Errorf("ecsStopTimeout(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
