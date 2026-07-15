// Package application coordinates catalog use cases and repeats authorization at the service boundary.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/KLX1899/KiTicket/internal/catalog/domain"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
)

type Repository interface {
	CreateVenue(context.Context, domain.Venue) error
	CreateEvent(context.Context, domain.Event) error
	EventOwner(context.Context, string) (string, error)
	CreateSchedule(context.Context, domain.Schedule) error
	PublishEvent(context.Context, string) error
	Search(context.Context, domain.SearchFilter) ([]domain.DiscoveryItem, int, error)
	ScheduleDetail(context.Context, string, string) (domain.ScheduleDetail, error)
}

type IDGenerator interface{ New() (string, error) }

type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	repository Repository
	ids        IDGenerator
	clock      Clock
}

func New(repository Repository, ids IDGenerator) (*Service, error) {
	if repository == nil || ids == nil {
		return nil, errors.New("catalog service requires repository and ID generator")
	}
	return &Service{repository: repository, ids: ids, clock: realClock{}}, nil
}

func (s *Service) CreateVenue(ctx context.Context, principal auth.Principal, venue domain.Venue) (domain.Venue, error) {
	if principal.Role != "organizer" && principal.Role != "admin" {
		return domain.Venue{}, domain.ErrForbidden
	}
	if principal.Role == "organizer" {
		venue.OwnerID = principal.UserID
	} else if venue.OwnerID == "" {
		venue.OwnerID = principal.UserID
	}
	if err := assignVenueIDs(&venue, s.ids); err != nil {
		return domain.Venue{}, err
	}
	if err := venue.Validate(); err != nil {
		return domain.Venue{}, err
	}
	if err := s.repository.CreateVenue(ctx, venue); err != nil {
		return domain.Venue{}, err
	}
	return venue, nil
}

func (s *Service) CreateEvent(ctx context.Context, principal auth.Principal, event domain.Event) (domain.Event, error) {
	if principal.Role != "organizer" && principal.Role != "admin" {
		return domain.Event{}, domain.ErrForbidden
	}
	if principal.Role == "organizer" {
		event.OrganizerID = principal.UserID
	} else if event.OrganizerID == "" {
		event.OrganizerID = principal.UserID
	}
	var err error
	event.ID, err = s.ids.New()
	if err != nil {
		return domain.Event{}, err
	}
	if err := event.Validate(); err != nil {
		return domain.Event{}, err
	}
	if err := s.repository.CreateEvent(ctx, event); err != nil {
		return domain.Event{}, err
	}
	return event, nil
}

func (s *Service) CreateSchedule(ctx context.Context, principal auth.Principal, schedule domain.Schedule) (domain.Schedule, error) {
	if err := s.authorizeEvent(ctx, principal, schedule.EventID); err != nil {
		return domain.Schedule{}, err
	}
	var err error
	schedule.ID, err = s.ids.New()
	if err != nil {
		return domain.Schedule{}, err
	}
	for index := range schedule.Categories {
		schedule.Categories[index].ID, err = s.ids.New()
		if err != nil {
			return domain.Schedule{}, err
		}
	}
	if err := schedule.Validate(s.clock.Now()); err != nil {
		return domain.Schedule{}, err
	}
	if err := s.repository.CreateSchedule(ctx, schedule); err != nil {
		return domain.Schedule{}, err
	}
	return schedule, nil
}

func (s *Service) Publish(ctx context.Context, principal auth.Principal, eventID string) error {
	if err := s.authorizeEvent(ctx, principal, eventID); err != nil {
		return err
	}
	return s.repository.PublishEvent(ctx, eventID)
}

func (s *Service) Search(ctx context.Context, filter domain.SearchFilter) ([]domain.DiscoveryItem, int, error) {
	if err := filter.Validate(); err != nil {
		return nil, 0, err
	}
	return s.repository.Search(ctx, filter)
}

func (s *Service) ScheduleDetail(ctx context.Context, eventID, scheduleID string) (domain.ScheduleDetail, error) {
	if eventID == "" || scheduleID == "" {
		return domain.ScheduleDetail{}, domain.ErrNotFound
	}
	return s.repository.ScheduleDetail(ctx, eventID, scheduleID)
}

func (s *Service) authorizeEvent(ctx context.Context, principal auth.Principal, eventID string) error {
	if (principal.Role != "organizer" && principal.Role != "admin") || principal.UserID == "" || eventID == "" {
		return domain.ErrForbidden
	}
	owner, err := s.repository.EventOwner(ctx, eventID)
	if err != nil {
		return err
	}
	if principal.Role != "admin" && owner != principal.UserID {
		return domain.ErrForbidden
	}
	return nil
}

func assignVenueIDs(venue *domain.Venue, ids IDGenerator) error {
	var err error
	venue.ID, err = ids.New()
	if err != nil {
		return err
	}
	for hallIndex := range venue.Halls {
		hall := &venue.Halls[hallIndex]
		hall.ID, err = ids.New()
		if err != nil {
			return err
		}
		for sectionIndex := range hall.Sections {
			section := &hall.Sections[sectionIndex]
			section.ID, err = ids.New()
			if err != nil {
				return err
			}
			for rowIndex := range section.Rows {
				row := &section.Rows[rowIndex]
				row.ID, err = ids.New()
				if err != nil {
					return err
				}
				for seatIndex := range row.Seats {
					row.Seats[seatIndex].ID, err = ids.New()
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
