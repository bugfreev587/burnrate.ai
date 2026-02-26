package events

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-redis/redis/v8"
)

const reportStreamName = "tokengate:report:jobs"

// ReportQueue is a Redis Streams producer for audit report generation jobs.
type ReportQueue struct {
	rdb *redis.Client
}

// NewReportQueue creates a new ReportQueue.
func NewReportQueue(rdb *redis.Client) *ReportQueue {
	return &ReportQueue{rdb: rdb}
}

// Publish enqueues a report generation job.
func (q *ReportQueue) Publish(ctx context.Context, reportID uint) error {
	err := q.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: reportStreamName,
		Values: map[string]interface{}{
			"report_id": strconv.FormatUint(uint64(reportID), 10),
		},
	}).Err()
	if err != nil {
		return fmt.Errorf("reportqueue: XADD %s: %w", reportStreamName, err)
	}
	return nil
}
