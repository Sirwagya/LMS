package contest

import (
	"context"
	"strings"

	"vedam/lms/internal/domain/clock"
	domain "vedam/lms/internal/domain/contest"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/uow"
)

type Service struct {
	repo  domain.Repository
	tx    uow.TxManager
	clock clock.Clock
	id    func() string
}

func New(repo domain.Repository, tx uow.TxManager, clock clock.Clock, id func() string) *Service {
	return &Service{repo: repo, tx: tx, clock: clock, id: id}
}

func (s *Service) Save(ctx context.Context, actor domain.Actor, id string, revision int64, draft domain.Draft) (domain.Record, error) {
	if err := draft.Validate(); err != nil {
		return domain.Record{}, err
	}
	var result domain.Record
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if id == "" {
			id = s.id()
			if err := s.repo.Create(ctx, actor, id, draft); err != nil {
				return err
			}
		} else if err := s.repo.Update(ctx, actor, id, revision, draft); err != nil {
			return err
		}
		var err error
		result, err = s.repo.GetStaff(ctx, actor, id)
		return err
	})
	return result, err
}

func (s *Service) Publish(ctx context.Context, actor domain.Actor, id string, revision int64) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		record, err := s.repo.GetStaff(ctx, actor, id)
		if err != nil {
			return err
		}
		if record.Revision != revision || record.Status != "draft" {
			return errs.New(errs.ErrConflict, "CONTEST_REVISION", "The draft changed. Reload it before publishing.")
		}
		if len(record.Questions) == 0 || len(record.StudentIDs) == 0 {
			return domain.Invalid("Add at least one question and one student before publishing.")
		}
		if !record.OpensAt.After(s.clock.Now()) {
			return domain.Invalid("Choose an opening time in the future before publishing.")
		}
		return s.repo.Publish(ctx, actor, id, revision)
	})
}

func (s *Service) Cancel(ctx context.Context, actor domain.Actor, id string, revision int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 1000 {
		return domain.Invalid("Provide a cancellation reason up to 1,000 bytes.")
	}
	return s.repo.Cancel(ctx, actor, id, revision, reason)
}
