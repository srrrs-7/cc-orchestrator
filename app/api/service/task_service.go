package service

import (
	"context"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// TaskService implements the Task-related application use cases. It
// orchestrates domain objects (the Task aggregate, the Repository
// and the DuplicateChecker domain service) but does not itself hold
// any business rule.
type TaskService struct {
	repo   task.Repository
	dupChk *task.DuplicateChecker
}

// NewTaskService builds a TaskService.
func NewTaskService(repo task.Repository, dupChk *task.DuplicateChecker) *TaskService {
	return &TaskService{repo: repo, dupChk: dupChk}
}

// Create builds a new Task from the given title and priority,
// rejecting duplicate titles, and persists it. An empty priority
// defaults to task.PriorityMedium; a non-empty priority is validated
// via task.ParsePriority and rejected (ErrInvalidPriority) if unknown.
func (s *TaskService) Create(ctx context.Context, title, priority string) (TaskDTO, error) {
	t, err := task.NewTitle(title)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: create task: %w", err)
	}

	p := task.PriorityMedium
	if priority != "" {
		p, err = task.ParsePriority(priority)
		if err != nil {
			return TaskDTO{}, fmt.Errorf("service: create task: %w", err)
		}
	}

	duplicated, err := s.dupChk.IsDuplicated(ctx, t)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: create task: %w", err)
	}
	if duplicated {
		return TaskDTO{}, fmt.Errorf("service: create task: %w", task.ErrDuplicateTitle)
	}

	newTask := task.New(t, p)
	if err := s.repo.Save(ctx, newTask); err != nil {
		return TaskDTO{}, fmt.Errorf("service: create task: %w", err)
	}

	return newTaskDTO(newTask), nil
}

// Get retrieves a single Task by its ID.
func (s *TaskService) Get(ctx context.Context, id string) (TaskDTO, error) {
	taskID, err := task.ParseID(id)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: get task: %w", err)
	}

	t, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: get task: %w", err)
	}

	return newTaskDTO(t), nil
}

// List retrieves all Tasks.
func (s *TaskService) List(ctx context.Context) ([]TaskDTO, error) {
	tasks, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("service: list tasks: %w", err)
	}

	dtos := make([]TaskDTO, 0, len(tasks))
	for _, t := range tasks {
		dtos = append(dtos, newTaskDTO(t))
	}
	return dtos, nil
}

// Start transitions the Task identified by id from todo to doing.
func (s *TaskService) Start(ctx context.Context, id string) (TaskDTO, error) {
	taskID, err := task.ParseID(id)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: start task: %w", err)
	}

	t, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: start task: %w", err)
	}

	if err := t.Start(); err != nil {
		return TaskDTO{}, fmt.Errorf("service: start task: %w", err)
	}

	if err := s.repo.Save(ctx, t); err != nil {
		return TaskDTO{}, fmt.Errorf("service: start task: %w", err)
	}

	return newTaskDTO(t), nil
}

// ChangePriority updates the priority of the Task identified by id.
// It never touches status: priority changes are orthogonal to the
// todo/doing/done state machine.
func (s *TaskService) ChangePriority(ctx context.Context, id, priority string) (TaskDTO, error) {
	taskID, err := task.ParseID(id)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: change priority: %w", err)
	}

	p, err := task.ParsePriority(priority)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: change priority: %w", err)
	}

	t, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: change priority: %w", err)
	}

	t.ChangePriority(p)

	if err := s.repo.Save(ctx, t); err != nil {
		return TaskDTO{}, fmt.Errorf("service: change priority: %w", err)
	}

	return newTaskDTO(t), nil
}

// Complete transitions the Task identified by id from doing to done.
func (s *TaskService) Complete(ctx context.Context, id string) (TaskDTO, error) {
	taskID, err := task.ParseID(id)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: complete task: %w", err)
	}

	t, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("service: complete task: %w", err)
	}

	if err := t.Complete(); err != nil {
		return TaskDTO{}, fmt.Errorf("service: complete task: %w", err)
	}

	if err := s.repo.Save(ctx, t); err != nil {
		return TaskDTO{}, fmt.Errorf("service: complete task: %w", err)
	}

	return newTaskDTO(t), nil
}
