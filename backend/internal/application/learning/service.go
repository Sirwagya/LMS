package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/clock"
	"vedam/lms/internal/domain/errs"
	domain "vedam/lms/internal/domain/learning"
	syncdomain "vedam/lms/internal/domain/sync"
	"vedam/lms/internal/domain/uow"
)

const MaxAttachmentBytes = 10 * 1024 * 1024

type Service struct {
	tx         uow.TxManager
	catRead    catalog.CatalogReader
	assignRead assessment.StudentAssignmentReader
	subRepo    assessment.SubmissionRepository
	progRepo   domain.ProgressRepository
	discRepo   domain.DiscussionRepository
	assetMeta  catalog.AssetMetadataRepository
	assetBlob  catalog.AssetBlobStore
	outbox     syncdomain.OutboxWriter
	clk        clock.Clock
	newID      func() string
}

func NewService(
	tx uow.TxManager,
	catRead catalog.CatalogReader,
	assignRead assessment.StudentAssignmentReader,
	subRepo assessment.SubmissionRepository,
	progRepo domain.ProgressRepository,
	discRepo domain.DiscussionRepository,
	assetMeta catalog.AssetMetadataRepository,
	assetBlob catalog.AssetBlobStore,
	outbox syncdomain.OutboxWriter,
	clk clock.Clock,
	newID func() string,
) *Service {
	return &Service{
		tx:         tx,
		catRead:    catRead,
		assignRead: assignRead,
		subRepo:    subRepo,
		progRepo:   progRepo,
		discRepo:   discRepo,
		assetMeta:  assetMeta,
		assetBlob:  assetBlob,
		outbox:     outbox,
		clk:        clk,
		newID:      newID,
	}
}

func (s *Service) ToggleProgress(ctx context.Context, userID string, lectureID int64, complete bool) error {
	return s.SetProgress(ctx, userID, lectureID, complete, s.clk.Now())
}

func (s *Service) SetProgress(ctx context.Context, userID string, lectureID int64, complete bool, clientTime time.Time) error {
	if _, err := s.catRead.GetLecture(ctx, lectureID); err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		if complete {
			if err := s.progRepo.MarkComplete(txCtx, userID, lectureID); err != nil {
				return err
			}
		} else {
			if err := s.progRepo.MarkIncomplete(txCtx, userID, lectureID); err != nil {
				return err
			}
		}
		payload, _ := json.Marshal(map[string]any{
			"userId":    userID,
			"lectureId": lectureID,
			"complete":  complete,
		})
		target := userID
		if _, err := s.outbox.AppendChangelog(txCtx, "user:"+userID, &target, "lecture_progress", strconv.FormatInt(lectureID, 10), "upsert", payload); err != nil {
			return err
		}
		return s.outbox.EnqueueEvent(txCtx, syncdomain.OutboxEvent{
			ID:          s.newID(),
			AggregateID: userID,
			Topic:       "cache.invalidate.bootstrap",
			Payload:     payload,
			CreatedAt:   s.clk.Now().UTC(),
		})
	})
}

type SubmitWrittenInput struct {
	UserID       string
	Role         string
	AssignmentID string
	Answer       string
	Filename     string
	MimeType     string
	FileBytes    []byte
}

func (s *Service) SubmitWrittenAssignment(ctx context.Context, in SubmitWrittenInput) error {
	if in.Role != "student" {
		return errs.New(errs.ErrForbidden, "STUDENT_ONLY", "Student access required.")
	}
	a, err := s.assignRead.GetPublicByID(ctx, in.AssignmentID)
	if err != nil {
		return err
	}
	if a.Kind == assessment.KindCoding {
		return errs.New(errs.ErrValidation, "CODING_WRONG_ENDPOINT", "Use the coding workspace to submit this assignment.")
	}
	answer := strings.TrimSpace(in.Answer)
	if answer == "" || len(answer) > 20000 {
		return errs.New(errs.ErrValidation, "INVALID_ANSWER", "Submission answer must be between 1 and 20000 characters.")
	}
	if len(in.FileBytes) > MaxAttachmentBytes {
		return errs.New(errs.ErrValidation, "FILE_TOO_LARGE", "Maximum attachment size is 10 MB.")
	}

	return s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		existing, err := s.subRepo.GetByUserAndAssignment(txCtx, in.UserID, a.ID)
		if err != nil && !errors.Is(err, errs.ErrNotFound) {
			return err
		}
		if existing != nil && existing.Score != nil && *existing.Score != "" {
			return errs.New(errs.ErrAlreadyGraded, "ALREADY_GRADED", "Graded submissions cannot be changed.")
		}

		var assetID *string
		filename := strings.TrimSpace(in.Filename)
		if len(in.FileBytes) > 0 {
			if filename == "" || len(filename) > 200 {
				return errs.New(errs.ErrValidation, "INVALID_FILENAME", "Provide a valid attachment filename.")
			}
			id := s.newID()
			storageKey := "submissions/" + id
			digest := sha256.Sum256(in.FileBytes)
			if err := s.assetBlob.Put(txCtx, storageKey, in.FileBytes); err != nil {
				return err
			}
			mime := in.MimeType
			if mime == "" {
				mime = "application/octet-stream"
			}
			if err := s.assetMeta.Create(txCtx, &catalog.Asset{
				ID:         id,
				Filename:   filename,
				MimeType:   mime,
				ByteSize:   int64(len(in.FileBytes)),
				StorageKey: storageKey,
				SHA256:     hex.EncodeToString(digest[:]),
				CreatedAt:  s.clk.Now().UTC(),
			}); err != nil {
				return err
			}
			assetID = &id
		}

		subID := s.newID()
		if existing != nil {
			subID = existing.ID
		}
		return s.subRepo.Upsert(txCtx, &assessment.Submission{
			ID:           subID,
			UserID:       in.UserID,
			AssignmentID: a.ID,
			Course:       a.CourseCode,
			Answer:       answer,
			Filename:     filename,
			AssetID:      assetID,
			Created:      s.clk.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	})
}

func (s *Service) ListDiscussions(ctx context.Context, lectureID int64) ([]domain.Discussion, error) {
	return s.discRepo.ListForLecture(ctx, lectureID)
}

func (s *Service) PostDiscussion(ctx context.Context, userID string, lectureID int64, bodyText string) (string, error) {
	if _, err := s.catRead.GetLecture(ctx, lectureID); err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(bodyText)
	if trimmed == "" || len(trimmed) > 2000 {
		return "", errs.New(errs.ErrValidation, "INVALID_BODY", "Message must be between 1 and 2000 characters.")
	}
	now := s.clk.Now().UTC()
	id := s.newID()
	return id, s.discRepo.Create(ctx, &domain.Discussion{
		ID:        id,
		UserID:    userID,
		LectureID: lectureID,
		Body:      trimmed,
		Created:   now.Format("2006-01-02T15:04:05.000Z"),
		CreatedAt: now,
	})
}
