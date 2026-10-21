package notifications

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
)

type suggestionJob struct {
	build    func(context.Context) models.LeadSuggestion
	deliver  func(models.LeadSuggestion)
	fallback models.LeadSuggestion
	timeout  time.Duration
	expires  time.Time
	timer    *time.Timer
}

// Queueing is opt-in per org, bounded to 32 pending jobs and one minute of
// waiting. Active provider concurrency and per-job budgets stay unchanged.
func NewQueuedSuggestionRunner(maxConcurrent int, timeout time.Duration, scope func(int64) bool, policies ...func(int64) time.Duration) *SuggestionRunner {
	r := NewSuggestionRunner(maxConcurrent, timeout, policies...)
	r.queueForOrg, r.queueLimit, r.queueWait = scope, 32, time.Minute
	return r
}

func (r *SuggestionRunner) submit(orgID int64, job *suggestionJob) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case r.slots <- struct{}{}:
		r.run(job)
		return true
	default:
		if r.queueForOrg == nil || !r.queueForOrg(orgID) || r.queueUsed >= r.queueLimit {
			return false
		}
		job.expires = time.Now().Add(r.queueWait)
		r.queueUsed++
		r.pending = append(r.pending, job)
		job.timer = time.AfterFunc(r.queueWait, func() { r.expire(job) })
		return true
	}
}

func (r *SuggestionRunner) finished() {
	r.mu.Lock()
	<-r.slots
	if len(r.pending) == 0 {
		r.mu.Unlock()
		return
	}
	job := r.pending[0]
	r.pending[0] = nil
	r.pending = r.pending[1:]
	r.queueUsed--
	job.timer.Stop()
	r.slots <- struct{}{}
	r.mu.Unlock()
	r.run(job)
}

func (r *SuggestionRunner) expire(job *suggestionJob) {
	r.mu.Lock()
	for i, pending := range r.pending {
		if pending != job {
			continue
		}
		copy(r.pending[i:], r.pending[i+1:])
		r.pending[len(r.pending)-1] = nil
		r.pending = r.pending[:len(r.pending)-1]
		r.mu.Unlock()
		defer func() {
			r.mu.Lock()
			r.queueUsed--
			r.mu.Unlock()
		}()
		job.deliver(job.expiredFallback())
		return
	}
	r.mu.Unlock() // It already started; its active deadline owns delivery.
}

func (job *suggestionJob) expiredFallback() models.LeadSuggestion {
	out := job.fallback
	out.ReplyStatus = "queue_expired"
	out.ReplyNote = "Gợi ý chưa được phân tích do hết thời gian chờ; sale cần kiểm tra thủ công."
	return out
}

// ExplainUnavailableSuggestion is shared by notification callers when work is
// busy, timed out or failed. No analysis verdict is implied by an empty draft.
func ExplainUnavailableSuggestion(out models.LeadSuggestion) models.LeadSuggestion {
	out.ReplyStatus = "processing_unavailable"
	out.ReplyNote = "Gợi ý chưa xử lý kịp trong lượt này; sale cần xem bài và kiểm tra thủ công."
	return out
}
