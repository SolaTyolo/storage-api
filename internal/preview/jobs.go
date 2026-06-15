package preview

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// JobStatus tracks async preview progress.
type JobStatus string

const (
	JobPending JobStatus = "pending"
	JobReady   JobStatus = "ready"
	JobFailed  JobStatus = "failed"
)

// Job is an async preview result.
type Job struct {
	ID          string
	Status      JobStatus
	ContentType string
	Body        []byte
	Err         string
	CreatedAt   time.Time
}

// JobStore holds in-memory async preview jobs.
type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
	ttl  time.Duration
}

func NewJobStore(ttl time.Duration) *JobStore {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &JobStore{jobs: make(map[string]*Job), ttl: ttl}
}

func (s *JobStore) Create() *Job {
	job := &Job{
		ID:        uuid.NewString(),
		Status:    JobPending,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.pruneLocked()
	s.mu.Unlock()
	return job
}

func (s *JobStore) Get(id string) (*Job, bool) {
	s.mu.RLock()
	job, ok := s.jobs[id]
	s.mu.RUnlock()
	return job, ok
}

func (s *JobStore) Complete(id string, contentType string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = JobReady
		job.ContentType = contentType
		job.Body = body
	}
}

func (s *JobStore) Fail(id, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = JobFailed
		job.Err = msg
	}
}

func (s *JobStore) pruneLocked() {
	cutoff := time.Now().Add(-s.ttl)
	for id, job := range s.jobs {
		if job.CreatedAt.Before(cutoff) && job.Status != JobPending {
			delete(s.jobs, id)
		}
	}
}
