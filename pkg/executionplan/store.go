package executionplan

import (
	"sync"
)

// Store handles storage and retrieval of execution plans
type Store struct {
	plans      map[string]*ExecutionPlan
	plansMutex sync.RWMutex
}

// NewStore creates a new execution plan store
func NewStore() *Store {
	return &Store{
		plans: make(map[string]*ExecutionPlan),
	}
}

// StorePlan stores an execution plan
func (s *Store) StorePlan(plan *ExecutionPlan) {
	s.plansMutex.Lock()
	defer s.plansMutex.Unlock()
	s.plans[plan.TaskID] = plan
}

// GetPlanByTaskID retrieves an execution plan by its task ID
func (s *Store) GetPlanByTaskID(taskID string) (*ExecutionPlan, bool) {
	s.plansMutex.RLock()
	defer s.plansMutex.RUnlock()
	plan, exists := s.plans[taskID]
	return plan, exists
}

// ListPlans returns a list of all plans
func (s *Store) ListPlans() []*ExecutionPlan {
	s.plansMutex.RLock()
	defer s.plansMutex.RUnlock()

	plans := make([]*ExecutionPlan, 0, len(s.plans))
	for _, plan := range s.plans {
		plans = append(plans, plan)
	}
	return plans
}

// DeletePlan deletes a plan by its task ID
func (s *Store) DeletePlan(taskID string) bool {
	s.plansMutex.Lock()
	defer s.plansMutex.Unlock()

	_, exists := s.plans[taskID]
	if exists {
		delete(s.plans, taskID)
	}
	return exists
}
