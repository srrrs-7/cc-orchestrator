// Package service contains the application layer: use case
// orchestration that coordinates domain objects to fulfill a single
// request, without holding business rules itself.
package service

import (
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// TaskDTO is the application layer's output model for a Task. It
// exists so that domain objects (task.Task, its value objects) never
// leak into upper layers (presentation) directly; only plain data
// crosses the application layer boundary.
type TaskDTO struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// newTaskDTO converts a domain Task into its DTO representation.
func newTaskDTO(t *task.Task) TaskDTO {
	return TaskDTO{
		ID:        t.ID().String(),
		Title:     t.Title().String(),
		Status:    t.Status().String(),
		CreatedAt: t.CreatedAt(),
		UpdatedAt: t.UpdatedAt(),
	}
}
