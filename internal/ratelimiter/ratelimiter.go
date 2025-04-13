package ratelimiter

import (
	"time"
)

type IpRateLimiterRecord struct {
	RequestsInInterval []time.Time
	TimeOutUntil       time.Time
}

type IpRateLimiter struct {
	MaxRequests         int
	MaxRequestsInterval time.Duration
	TimeOutPenalty      time.Duration
	records             map[string]*IpRateLimiterRecord
}

func NewRateLimiter(maxRequests int, maxRequestsInterval time.Duration, timeOutPenalty time.Duration) IpRateLimiter {
	return IpRateLimiter{
		MaxRequests:         maxRequests,
		MaxRequestsInterval: maxRequestsInterval,
		TimeOutPenalty:      timeOutPenalty,
		records:             make(map[string]*IpRateLimiterRecord),
	}
}

func (rl *IpRateLimiter) IsOk(ip string, now time.Time) bool {
	record := rl.getOrCreateRecord(ip)
	if record.inPenaltyBox(now) {
		return false
	}

	record.appendRequest(rl.MaxRequestsInterval, now)
	if rl.exceedsLimits(record) {
		record.timeOut(now.Add(rl.TimeOutPenalty))
		return false
	}
	return true
}

func (rl *IpRateLimiter) exceedsLimits(record *IpRateLimiterRecord) bool {
	return len(record.RequestsInInterval) > rl.MaxRequests
}

func (rl *IpRateLimiter) getOrCreateRecord(ip string) *IpRateLimiterRecord {
	record, ok := rl.records[ip]
	if ok {
		return record
	} else {
		record := &IpRateLimiterRecord{
			RequestsInInterval: []time.Time{},
		}
		rl.records[ip] = record
		return record
	}
}

func (r *IpRateLimiterRecord) appendRequest(interval time.Duration, now time.Time) {
	r.removeStaleRequests(interval, now)
	r.RequestsInInterval = append(r.RequestsInInterval, now)
}

func (r *IpRateLimiterRecord) removeStaleRequests(interval time.Duration, now time.Time) {
	minStartTime := now.Add(-interval)

	firstGoodIndex := -1
	for i, t := range r.RequestsInInterval {
		if t.After(minStartTime) {
			firstGoodIndex = i
			break
		}
	}

	if firstGoodIndex == -1 {
		r.RequestsInInterval = []time.Time{}
	} else {
		r.RequestsInInterval = r.RequestsInInterval[firstGoodIndex:]
	}
}

func (r *IpRateLimiterRecord) timeOut(until time.Time) {
	r.TimeOutUntil = until
}

func (r *IpRateLimiterRecord) inPenaltyBox(now time.Time) bool {
	return r.TimeOutUntil.After(now)
}
