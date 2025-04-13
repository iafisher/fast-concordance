package pkg

import (
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	maxRequests := 3
	maxRequestsInterval := time.Second * 2
	timeOutPenalty := time.Millisecond * 100
	rl := NewRateLimiter(maxRequests, maxRequestsInterval, timeOutPenalty)

	badIp := "1.1.1.1"
	goodIp := "2.2.2.2"

	var T int64 = 1744558472
	// ok: 1 req at T
	shouldAllow(t, &rl, badIp, T+0)

	// ok: 2 reqs at T
	shouldAllow(t, &rl, badIp, T+0)

	// ok: 3 reqs = 1 at T+1, 2 at T
	shouldAllow(t, &rl, badIp, T+1)

	// not ok: 3 reqs already
	shouldNotAllow(t, &rl, badIp, T+1)

	// not ok: 3 reqs already
	shouldNotAllow(t, &rl, badIp, T+1)

	// not ok: 3 reqs already
	shouldNotAllow(t, &rl, badIp, T+1)

	// ok: 1 req (different ip)
	shouldAllow(t, &rl, goodIp, T+1)

	// ok: 2 reqs: 1 at T+2, 1 at T+1
	shouldAllow(t, &rl, goodIp, T+2)
}

func TestRateLimitTimeOutPenalty(t *testing.T) {
	maxRequests := 2
	maxRequestsInterval := time.Second * 1
	timeOutPenalty := time.Second * 5
	rl := NewRateLimiter(maxRequests, maxRequestsInterval, timeOutPenalty)

	badIp := "1.1.1.1"

	var T int64 = 1744558472
	// ok: 1 req at T
	shouldAllow(t, &rl, badIp, T+0)

	// ok: 2 reqs at T
	shouldAllow(t, &rl, badIp, T+0)

	// not ok: 2 reqs already
	shouldNotAllow(t, &rl, badIp, T+0)

	// not ok: in penalty box until T+5
	shouldNotAllow(t, &rl, badIp, T+1)

	// not ok: in penalty box until T+5
	shouldNotAllow(t, &rl, badIp, T+2)

	// not ok: in penalty box until T+5
	shouldNotAllow(t, &rl, badIp, T+4)

	// ok: out of penalty box
	shouldAllow(t, &rl, badIp, T+5)
}

func shouldAllow(t *testing.T, rl *IpRateLimiter, ip string, sec int64) {
	t.Helper()
	ipOk := rl.IsOk(ip, time.Unix(sec, 0))
	if !ipOk {
		t.Fatalf("IP should be allowed at %d but wasn't", sec)
	}
}

func shouldNotAllow(t *testing.T, rl *IpRateLimiter, ip string, sec int64) {
	t.Helper()
	ipOk := rl.IsOk(ip, time.Unix(sec, 0))
	if ipOk {
		t.Fatalf("IP should not be allowed at %d but was", sec)
	}
}
